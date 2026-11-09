package main

import (
	"context"
	"encoding/json"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/webhooks"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestVerificationLiveHTTPContract(t *testing.T) {
	f, v := newIntegratedFixture(t)
	dbtest.Exec(t, f.pool, `UPDATE workspaces SET plan='trial',trial_send_limit=10 WHERE id=$1`, f.ws)
	server := httptest.NewServer(f.router)
	t.Cleanup(server.Close)
	call := func(method, path, body, key, idem string, want int) []byte {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		if key == "" {
			r.AddCookie(f.cookie)
		} else {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		if idem != "" {
			r.Header.Set("Idempotency-Key", idem)
		}
		res, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("%s %s status %d, expected %d", method, path, res.StatusCode, want)
		}
		return data
	}
	var key struct{ Key string }
	if err := json.Unmarshal(call("POST", "/api/keys", `{"name":"verify","scope":"sending_access","password":"test-password"}`, "", "", 201), &key); err != nil {
		t.Fatal(err)
	}
	var domain struct{ ID string }
	name := f.ws + ".verify.test"
	if err := json.Unmarshal(call("POST", "/api/domains", `{"name":"`+name+`","kind":"domain"}`, "", "", 201), &domain); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/domains/"+domain.ID+"/verify", "", "", "", 200)
	var hook struct{ Secret string }
	if err := json.Unmarshal(call("POST", "/api/webhooks", `{"url":"https://hooks.example.com/local","events":["email.delivered"]}`, "", "", 201), &hook); err != nil {
		t.Fatal(err)
	}
	var first, second struct{ ID string }
	if err := json.Unmarshal(call("POST", "/api/emails", `{"from":"hello@`+name+`","to":"user@example.com","subject":"Local verifier","text":"Hello"}`, key.Key, "verify-once", 202), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(call("POST", "/api/transactional", `{"from_address":"hello@`+name+`","to":"user@example.com","subject":"Local verifier","text":"Hello"}`, key.Key, "verify-once", 200), &second); err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID {
		t.Fatal("cross-route message IDs differ")
	}
	call("POST", "/api/campaigns", `{"name":"must be denied"}`, key.Key, "", 403)
	eventually(t, func() bool {
		return dbtest.QueryString(t, f.pool, `SELECT count(delivered_at)::text FROM transactional_messages WHERE workspace_id=$1`, f.ws) == "1"
	})
	if dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM transactional_messages WHERE workspace_id=$1`, f.ws) != "1" {
		t.Fatal("multiple jobs")
	}
	if dbtest.QueryString(t, f.pool, `SELECT trial_sent_count::text FROM workspaces WHERE id=$1`, f.ws) != "1" {
		t.Fatal("quota reserved more than once")
	}
	received := make(chan bool, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		valid := webhooks.Verify(hook.Secret, r.Header.Get("Webhook-Id"), r.Header.Get("Webhook-Timestamp"), r.Header.Get("Webhook-Signature"), raw, time.Now())
		var event struct {
			Type string
			Data struct {
				MessageID string `json:"message_id"`
				Kind      string `json:"message_type"`
			}
		}
		json.Unmarshal(raw, &event)
		received <- valid && event.Type == "email.delivered" && event.Data.MessageID == first.ID && event.Data.Kind == "transactional"
		w.WriteHeader(204)
	}))
	defer receiver.Close()
	target, _ := url.Parse(receiver.URL)
	client := &http.Client{Transport: receiverTransport(func(r *http.Request) (*http.Response, error) {
		clone := r.Clone(r.Context())
		u := *r.URL
		u.Scheme = target.Scheme
		u.Host = target.Host
		clone.URL = &u
		return http.DefaultTransport.RoundTrip(clone)
	})}
	d := &webhooks.Dispatcher{Pool: f.pool, Vault: v, Client: client}
	if ok, err := d.Once(context.Background()); !ok || err != nil {
		t.Fatal("customer webhook not dispatched", err)
	}
	select {
	case ok := <-received:
		if !ok {
			t.Fatal("local receiver rejected signature or canonical ID")
		}
	case <-time.After(time.Second):
		t.Fatal("local receiver not called")
	}
	t.Log("local (log driver): real HTTP key/domain setup, same cross-route message ID, one row/quota, signed local receiver, sending scope 403")
}
