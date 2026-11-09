package main

import (
	"context"
	"encoding/json"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/webhooks"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestTrackingEventsReachSignedCustomerWebhook(t *testing.T) {
	f, v := newIntegratedFixture(t)
	domain := f.ws + ".tracking.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	w := f.request("POST", "/api/webhooks", `{"url":"https://hooks.example.com","events":["email.opened","email.clicked"]}`, "")
	var endpoint struct{ Secret string }
	json.Unmarshal(w.Body.Bytes(), &endpoint)
	if w.Code != 201 {
		t.Fatalf("hook %d", w.Code)
	}
	w = f.request("POST", "/api/transactional", `{"from_address":"hello@`+domain+`","to":"user@example.com","subject":"Track","html":"<a href=\"https://example.com/reset\">Reset</a>","track_opens":true,"track_clicks":true}`, "")
	var msg struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &msg)
	if w.Code != 202 {
		t.Fatalf("send %d %s", w.Code, w.Body.String())
	}
	eventually(t, func() bool {
		return dbtest.QueryString(t, f.pool, `SELECT status FROM transactional_messages WHERE id=$1 AND workspace_id=$2`, msg.ID, f.ws) == "sent"
	})
	html := dbtest.QueryString(t, f.pool, `SELECT payload->'message'->>'html' FROM message_logs WHERE message_id=$1 AND workspace_id=$2`, msg.ID, f.ws)
	if strings.Contains(html, "Unsubscribe") {
		t.Fatal("newsletter footer added to transactional message")
	}
	for _, kind := range []string{"o", "c"} {
		match := regexp.MustCompile(`/t/` + kind + `/([^"<> ]+)`).FindString(html)
		if match == "" {
			t.Fatal("tracking link missing")
		}
		r := httptest.NewRequest("GET", match, nil)
		r.Header.Set("User-Agent", "Mozilla/5.0 Firefox/140.0")
		out := httptest.NewRecorder()
		f.router.ServeHTTP(out, r)
		if (kind == "o" && out.Code != 200) || (kind == "c" && out.Code != 302) {
			t.Fatal("tracking route failed")
		}
	}
	before := dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM email_events WHERE workspace_id=$1 AND type IN ('opened','clicked')`, f.ws)
	for _, path := range []string{"/t/o/invalid", "/t/c/invalid"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("User-Agent", "Mozilla/5.0")
		f.router.ServeHTTP(httptest.NewRecorder(), r)
	}
	if after := dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM email_events WHERE workspace_id=$1 AND type IN ('opened','clicked')`, f.ws); after != before || after != "2" {
		t.Fatal("invalid token created event")
	}
	seen := map[string]bool{}
	d := webhooks.Dispatcher{Pool: f.pool, Vault: v, Client: &http.Client{Transport: receiverTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if !webhooks.Verify(endpoint.Secret, r.Header.Get("Webhook-Id"), r.Header.Get("Webhook-Timestamp"), r.Header.Get("Webhook-Signature"), body, time.Now()) {
			t.Error("signature invalid")
		}
		var event struct {
			Type string
			Data struct {
				MessageID string `json:"message_id"`
				Kind      string `json:"message_type"`
			}
		}
		json.Unmarshal(body, &event)
		if event.Data.MessageID != msg.ID || event.Data.Kind != "transactional" {
			t.Error("tracking identity missing")
		}
		seen[event.Type] = true
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	for range 2 {
		if _, err := d.Once(context.Background()); err != nil {
			t.Fatal(err)
		}
		dbtest.Exec(t, f.pool, `UPDATE webhook_endpoints SET next_dispatch_at=now() WHERE workspace_id=$1`, f.ws)
	}
	if !seen["email.opened"] || !seen["email.clicked"] {
		t.Fatal("tracking webhooks missing")
	}
}
