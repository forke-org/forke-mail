package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/feedback"
	"forkemail/internal/webhooks"
)

type receiverTransport func(*http.Request) (*http.Response, error)

func (f receiverTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDeveloperContractEndToEnd(t *testing.T) {
	f := newAPIFixture(t)
	v, err := webhooks.NewVault(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("v", 32))))
	if err != nil {
		t.Fatal(err)
	}
	f.developer(&developerServices{hooks: &webhooks.Handler{Pool: f.pool, Vault: v, Network: webhooks.Network{Resolver: publicDNS{}}}})
	w := f.request("POST", "/api/keys", `{"name":"e2e","scope":"sending_access","password":"test-password"}`, "")
	var key struct{ Key string }
	if err := json.Unmarshal(w.Body.Bytes(), &key); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 {
		t.Fatalf("key %d", w.Code)
	}
	domain := f.ws + ".example.com"
	w = f.request("POST", "/api/domains", `{"name":"`+domain+`"}`, "")
	var identity struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 {
		t.Fatalf("domain %d", w.Code)
	}
	if code := f.request("POST", "/api/domains/"+identity.ID+"/verify", "", "").Code; code != 200 {
		t.Fatalf("verify %d", code)
	}
	w = f.request("POST", "/api/webhooks", `{"url":"https://hooks.example.com","events":["email.delivered"]}`, "")
	var endpoint struct{ ID, Secret string }
	if err := json.Unmarshal(w.Body.Bytes(), &endpoint); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 {
		t.Fatalf("webhook %d", w.Code)
	}
	r := httptest.NewRequest("POST", "/api/transactional", strings.NewReader(`{"from_address":"sender@`+domain+`","to":"person@example.com","subject":"Receipt","text":"Hello"}`))
	r.Header.Set("Authorization", "Bearer "+key.Key)
	r.Header.Set("Idempotency-Key", "e2e-1")
	w = httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	var receipt struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if w.Code != 202 {
		t.Fatalf("send %d", w.Code)
	}
	message := "fixture-ses-" + f.ws
	dbtest.Exec(t, f.pool, `UPDATE transactional_messages SET status='sent', sent_at=now(), ses_message_id=$2 WHERE id=$1`, receipt.ID, message)
	p := feedback.Processor{Pool: f.pool}
	if _, err := p.Handle(context.Background(), "fixture-sns-"+f.ws, []byte(`{"eventType":"Delivery","mail":{"messageId":"`+message+`"}}`)); err != nil {
		t.Fatal(err)
	}
	verified := false
	dispatcher := webhooks.Dispatcher{Pool: f.pool, Vault: v, Client: &http.Client{Transport: receiverTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if !webhooks.Verify(endpoint.Secret, r.Header.Get("Webhook-Id"), r.Header.Get("Webhook-Timestamp"), r.Header.Get("Webhook-Signature"), body, time.Now()) {
			t.Error("signature invalid")
		}
		var payload struct {
			Type string
			Data struct {
				MessageID string `json:"message_id"`
			}
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Error(err)
		}
		if payload.Type != "email.delivered" || payload.Data.MessageID != receipt.ID {
			t.Error("message identity lost")
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
