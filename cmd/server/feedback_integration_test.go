package main

import (
	"context"
	"encoding/json"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/feedback"
	"forkemail/internal/sns"
	"forkemail/internal/sns/snstest"
	"forkemail/internal/webhooks"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTransactionalSignedSNSAndReceiverFailureIsolation(t *testing.T) {
	f, v := newIntegratedFixture(t)
	domain := f.ws + ".sns.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	w := f.request("POST", "/api/webhooks", `{"url":"https://hooks.example.com","events":["email.bounced"]}`, "")
	var endpoint struct{ Secret string }
	json.Unmarshal(w.Body.Bytes(), &endpoint)
	if w.Code != 201 {
		t.Fatal("hook create failed")
	}
	send := func(to string) string {
		t.Helper()
		w := f.request("POST", "/api/transactional", `{"from_address":"hello@`+domain+`","to":"`+to+`","subject":"SNS","text":"Hi"}`, "")
		var m struct{ ID string }
		json.Unmarshal(w.Body.Bytes(), &m)
		if w.Code != 202 {
			t.Fatal("send failed")
		}
		eventually(t, func() bool {
			return dbtest.QueryString(t, f.pool, `SELECT status FROM transactional_messages WHERE workspace_id=$1 AND id=$2`, f.ws, m.ID) == "sent"
		})
		return m.ID
	}
	id := send("user@example.com")
	provider := dbtest.QueryString(t, f.pool, `SELECT ses_message_id FROM transactional_messages WHERE workspace_id=$1 AND id=$2`, f.ws, id)
	signer := snstest.NewSigner(t)
	h := &feedback.Webhook{Verifier: signer.Verifier(), Processor: &feedback.Processor{Pool: f.pool, Reputation: feedback.Thresholds{MaxBounceRate: 0.05, MaxComplaintRate: 0.001, MinSample: 100}}}
	post := func(m sns.Message) int {
		raw, _ := json.Marshal(m)
		out := httptest.NewRecorder()
		h.ServeHTTP(out, httptest.NewRequest("POST", "/webhooks/ses", strings.NewReader(string(raw))))
		return out.Code
	}
	event := func(ws string) string {
		raw, _ := json.Marshal(map[string]any{"eventType": "Bounce", "mail": map[string]any{"messageId": provider, "tags": map[string][]string{"workspace_id": {ws}, "message_id": {id}}}, "bounce": map[string]any{"bounceType": "Permanent", "bounceSubType": "General", "bouncedRecipients": []map[string]string{{"emailAddress": "user@example.com"}}}})
		return string(raw)
	}
	foreign := dbtest.Workspace(t, f.pool, "paid", 10)
	bad := signer.Sign(t, sns.Message{Type: sns.TypeNotification, MessageID: "mismatch-" + id, Message: event(foreign)})
	if post(bad) != 200 || dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM suppressions WHERE workspace_id=$1`, f.ws) != "0" {
		t.Fatal("tenant mismatch changed state")
	}
	m := signer.Sign(t, sns.Message{Type: sns.TypeNotification, MessageID: "bounce-" + id, Message: event(f.ws)})
	for range 2 {
		if post(m) != 200 {
			t.Fatal("SNS ack failed")
		}
	}
	if dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM email_events WHERE workspace_id=$1 AND message_id=$2 AND type='bounced'`, f.ws, id) != "1" {
		t.Fatal("SNS duplicate created event")
	}
	if dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM webhook_deliveries WHERE workspace_id=$1`, f.ws) != "1" {
		t.Fatal("duplicate outgoing delivery")
	}
	m.Message += " "
	if post(m) != 403 {
		t.Fatal("tampered SNS accepted")
	}
	d := webhooks.Dispatcher{Pool: f.pool, Vault: v, Client: &http.Client{Transport: receiverTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if !webhooks.Verify(endpoint.Secret, r.Header.Get("Webhook-Id"), r.Header.Get("Webhook-Timestamp"), r.Header.Get("Webhook-Signature"), body, time.Now()) {
			t.Error("signature failed")
		}
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	if ok, err := d.Once(context.Background()); !ok || err != nil {
		t.Fatal("failed receiver was not isolated", err)
	}
	if dbtest.QueryString(t, f.pool, `SELECT status FROM webhook_deliveries WHERE workspace_id=$1`, f.ws) != "pending" {
		t.Fatal("receiver failure not retried")
	}
	if send("another@example.com") == "" {
		t.Fatal("receiver failure blocked dispatch")
	}
	if post(signer.Sign(t, sns.Message{Type: sns.TypeNotification, MessageID: "another-bounce-" + id, Message: event(f.ws)})) != 200 {
		t.Fatal("receiver failure blocked provider acknowledgement")
	}
}
