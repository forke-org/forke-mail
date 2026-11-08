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
	"forkemail/internal/transactional"
	"forkemail/internal/webhooks"
	"github.com/jackc/pgx/v5"
)

type fixtureEngine struct{}

func (fixtureEngine) Enqueue(ctx context.Context, tx pgx.Tx, j transactional.Job) error {
	var campaign string
	if err := tx.QueryRow(ctx, `INSERT INTO campaigns(workspace_id,name) VALUES($1,$2) RETURNING id`, j.WorkspaceID, j.ID).Scan(&campaign); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO emails(id,workspace_id,campaign_id,subject,from_address,text) VALUES($1,$2,$3,$4,$5,$6)`, j.ID, j.WorkspaceID, campaign, j.Message.Subject, j.Message.From, j.Message.Text)
	return err
}

type receiverTransport func(*http.Request) (*http.Response, error)

func (f receiverTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDeveloperContractEndToEndWithFakeEngine(t *testing.T) {
	f := newAPIFixture(t)
	v, err := webhooks.NewVault(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("v", 32))))
	if err != nil {
		t.Fatal(err)
	}
	f.developer(&developerServices{hooks: &webhooks.Handler{Pool: f.pool, Vault: v, Network: webhooks.Network{Resolver: publicDNS{}}}, transaction: &transactional.Handler{Pool: f.pool, Enqueuer: fixtureEngine{}}})
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
		t.Fatalf("send %d", w.Code)
	}
	message := "fixture-ses-" + f.ws
	dbtest.SentRecipient(t, f.pool, f.ws, receipt.ID, "person@example.com", message)
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
				EmailID string `json:"email_id"`
			}
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Error(err)
		}
		if payload.Type != "email.delivered" || payload.Data.EmailID != receipt.ID {
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
