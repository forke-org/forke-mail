package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/transactional"
	"forkemail/internal/webhooks"
)

type receiverTransport func(*http.Request) (*http.Response, error)

func (f receiverTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDeveloperContractEndToEndWithRealLogEngine(t *testing.T) {
	f, v := newIntegratedFixture(t)
	w := f.request("POST", "/api/keys", `{"name":"e2e","scope":"sending_access","password":"test-password"}`, "")
	var key struct{ Key string }
	if err := json.Unmarshal(w.Body.Bytes(), &key); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 {
		t.Fatal("key creation failed")
	}
	domain := f.ws + ".example.test"
	w = f.request("POST", "/api/domains", `{"name":"`+domain+`"}`, "")
	var identity struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 {
		t.Fatal("domain create failed")
	}
	if f.request("POST", "/api/domains/"+identity.ID+"/verify", "", "").Code != 200 {
		t.Fatal("domain verify failed")
	}
	w = f.request("POST", "/api/webhooks", `{"url":"https://hooks.example.com","events":["email.delivered"]}`, "")
	var endpoint struct{ ID, Secret string }
	if err := json.Unmarshal(w.Body.Bytes(), &endpoint); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 {
		t.Fatal("hook create failed")
	}
	r := httptest.NewRequest("POST", "/api/emails", strings.NewReader(`{"from":"sender@`+domain+`","to":"person@example.com","subject":"Receipt","text":"Hello"}`))
	r.Header.Set("Authorization", "Bearer "+key.Key)
	r.Header.Set("Idempotency-Key", "e2e-1")
	w = httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	var receipt transactional.Receipt
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if w.Code != 202 {
		t.Fatalf("send %d %s", w.Code, w.Body.String())
	}
	eventually(t, func() bool {
		return dbtest.QueryString(t, f.pool, `SELECT (delivered_at IS NOT NULL)::text FROM transactional_messages WHERE id=$1 AND workspace_id=$2`, receipt.ID, f.ws) == "true"
	})
	if n := dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM contacts WHERE workspace_id=$1`, f.ws); n != "0" {
		t.Fatal("transactional send created a marketing contact")
	}
	verified := false
	dispatcher := webhooks.Dispatcher{Pool: f.pool, Vault: v, Client: &http.Client{Transport: receiverTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if !webhooks.Verify(endpoint.Secret, r.Header.Get("Webhook-Id"), r.Header.Get("Webhook-Timestamp"), r.Header.Get("Webhook-Signature"), body, time.Now()) {
			t.Error("invalid outgoing signature")
		}
		var payload struct{ ID, Type string }
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Error(err)
		}
		if payload.Type != "email.delivered" {
			t.Error("wrong event")
		}
		if actual := dbtest.QueryString(t, f.pool, `SELECT message_id::text FROM email_events WHERE id=$1 AND workspace_id=$2`, payload.ID, f.ws); actual != receipt.ID {
			t.Error("engine/event message identity lost")
		}
		verified = true
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	if _, err := dispatcher.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !verified {
		t.Fatal("customer webhook not dispatched")
	}
}
