package main

import (
	"bytes"
	"context"
	"encoding/json"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/sender"
	"forkemail/internal/webhooks"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func apiKey(t *testing.T, f apiFixture, scope string) string {
	t.Helper()
	w := f.request("POST", "/api/keys", `{"name":"test","scope":"`+scope+`","password":"test-password"}`, "")
	var k struct{ Key string }
	if err := json.Unmarshal(w.Body.Bytes(), &k); err != nil || w.Code != 201 {
		t.Fatalf("key %d %v", w.Code, err)
	}
	return k.Key
}

func TestBroadcastAPIToSignedWebhook(t *testing.T) {
	f, v := newIntegratedFixture(t)
	full := apiKey(t, f, "full_access")
	sending := apiKey(t, f, "sending_access")
	f.worker.SetLimits(sender.Limits{Max24HourSend: 100, SentLast24Hours: 90})
	domain := f.ws + ".broadcast.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	for _, email := range []string{"unsub@example.com", "bounced@example.com"} {
		reason := "unsubscribed"
		if strings.HasPrefix(email, "bounced") {
			reason = "bounced"
		}
		dbtest.Exec(t, f.pool, `INSERT INTO suppressions(workspace_id,email,reason) VALUES($1,$2,$3)`, f.ws, email, reason)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("text", "email,first_name\ngood@example.com,Good\nunsub@example.com,Unsub\nbounced@example.com,Bounced\n"); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	r := httptest.NewRequest("POST", "/api/contacts/import", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+full)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("import %d %s", w.Code, w.Body.String())
	}
	if f.request("POST", "/api/campaigns", `{"name":"denied"}`, sending).Code != 403 {
		t.Fatal("sending scope broadened")
	}
	w = f.request("POST", "/api/campaigns", `{"name":"Broadcast integration"}`, full)
	var campaign struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &campaign)
	if w.Code != 201 {
		t.Fatalf("campaign %d", w.Code)
	}
	if f.request("GET", "/api/campaigns/"+campaign.ID, "", full).Code != 200 {
		t.Fatal("campaign read failed")
	}
	w = f.request("POST", "/api/campaigns/"+campaign.ID+"/emails", `{"subject":"Update","from_address":"sender@`+domain+`","text":"Hello","audience":{"all":true}}`, full)
	var email struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &email)
	if w.Code != 201 {
		t.Fatalf("draft %d %s", w.Code, w.Body.String())
	}
	if f.request("PATCH", "/api/emails/"+email.ID, `{"subject":"Updated"}`, full).Code != 200 {
		t.Fatal("draft update failed")
	}
	w = f.request("POST", "/api/webhooks", `{"url":"https://hooks.example.com","events":["email.delivered"]}`, full)
	var endpoint struct{ Secret string }
	json.Unmarshal(w.Body.Bytes(), &endpoint)
	if w.Code != 201 {
		t.Fatal("webhook registration failed")
	}
	if f.request("POST", "/api/emails/"+email.ID+"/send", "", sending).Code != 200 {
		t.Fatal("launch failed")
	}
	if f.request("POST", "/api/emails/"+email.ID+"/pause", "", full).Code != 200 {
		t.Fatal("pause failed")
	}
	if f.request("POST", "/api/emails/"+email.ID+"/resume", "", full).Code != 200 {
		t.Fatal("resume failed")
	}
	f.worker.SetLimits(sender.Limits{Max24HourSend: 1000})
	eventually(t, func() bool {
		return dbtest.QueryString(t, f.pool, `SELECT count(delivered_at)::text FROM email_recipients WHERE workspace_id=$1 AND email_id=$2`, f.ws, email.ID) == "1"
	})
	if n := dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM email_recipients WHERE workspace_id=$1 AND email_id=$2`, f.ws, email.ID); n != "1" {
		t.Fatal("blocked recipients included", n)
	}
	for _, suffix := range []string{"/report", "/recipients"} {
		if f.request("GET", "/api/emails/"+email.ID+suffix, "", full).Code != 200 {
			t.Fatal("report access failed")
		}
	}
	verified := false
	d := webhooks.Dispatcher{Pool: f.pool, Vault: v, Client: &http.Client{Transport: receiverTransport(func(r *http.Request) (*http.Response, error) {
		payload, _ := io.ReadAll(r.Body)
		if !webhooks.Verify(endpoint.Secret, r.Header.Get("Webhook-Id"), r.Header.Get("Webhook-Timestamp"), r.Header.Get("Webhook-Signature"), payload, time.Now()) {
			t.Error("invalid signature")
		}
		var event struct {
			Data struct {
				EmailID string `json:"email_id"`
			}
		}
		json.Unmarshal(payload, &event)
		if event.Data.EmailID != email.ID {
			t.Error("broadcast ID lost")
		}
		verified = true
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}}
	if _, err := d.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !verified {
		t.Fatal("broadcast notification missing")
	}
}
