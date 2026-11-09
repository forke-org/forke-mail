package main

import (
	"encoding/json"
	"forkemail/internal/db/dbtest"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIntegratedTenantIsolationAndSendingScopes(t *testing.T) {
	f := newAPIFixture(t)
	other := newAPIFixture(t)
	domain := f.ws + ".isolated.test"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, f.ws, domain)
	key := apiKey(t, f, "sending_access")
	w := f.request("POST", "/api/transactional", `{"from_address":"hello@`+domain+`","to":"user@example.com","subject":"Isolation","text":"Hello"}`, key)
	var msg struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &msg)
	if w.Code != 202 {
		t.Fatalf("send %d", w.Code)
	}
	if f.request("GET", "/api/transactional/"+msg.ID, "", key).Code != 200 {
		t.Fatal("sending key cannot inspect its message")
	}
	for _, path := range []string{"/api/transactional", "/api/transactional/stats", "/api/webhooks", "/api/keys"} {
		if f.request("GET", path, "", key).Code != 403 {
			t.Fatalf("sending scope broadened: %s", path)
		}
	}
	if other.request("GET", "/api/transactional/"+msg.ID, "", "").Code != 404 {
		t.Fatal("foreign message exposed")
	}
	listed := other.request("GET", "/api/transactional", "", "")
	if listed.Code != 200 || strings.Contains(listed.Body.String(), msg.ID) {
		t.Fatal("foreign list leak")
	}
	stats := other.request("GET", "/api/transactional/stats", "", "")
	var counts struct{ Total int }
	json.Unmarshal(stats.Body.Bytes(), &counts)
	if stats.Code != 200 || counts.Total != 0 {
		t.Fatal("foreign stats leak")
	}
	email := dbtest.DraftEmail(t, f.pool, f.ws)
	for _, suffix := range []string{"", "/report", "/recipients", "/messages"} {
		if other.request("GET", "/api/emails/"+email+suffix, "", "").Code != 404 {
			t.Fatalf("foreign broadcast exposed: %s", suffix)
		}
	}
	if f.request("POST", "/api/campaigns", `{"name":"denied"}`, key).Code != 403 {
		t.Fatal("sending key created campaign")
	}
}

func TestRotatedAndRevokedKeysRejectedByBothSendAPIs(t *testing.T) {
	f := newAPIFixture(t)
	w := f.request("POST", "/api/keys", `{"name":"lifecycle","scope":"sending_access","password":"test-password"}`, "")
	var first struct{ ID, Key string }
	json.Unmarshal(w.Body.Bytes(), &first)
	if w.Code != 201 {
		t.Fatal("create failed")
	}
	w = f.request("POST", "/api/keys/"+first.ID+"/rotate", `{"password":"test-password"}`, "")
	var next struct{ ID, Key string }
	json.Unmarshal(w.Body.Bytes(), &next)
	if w.Code != 201 {
		t.Fatal("rotation failed")
	}
	for _, path := range []string{"/api/emails", "/api/transactional"} {
		if f.request("POST", path, `{}`, first.Key).Code != 401 {
			t.Fatal("old key accepted", path)
		}
		r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+next.Key)
		r.Header.Set("Idempotency-Key", "fresh")
		out := httptest.NewRecorder()
		f.router.ServeHTTP(out, r)
		if out.Code != 422 {
			t.Fatalf("replacement did not reach validation %s %d", path, out.Code)
		}
	}
	if f.request("DELETE", "/api/keys/"+next.ID, "", "").Code != 204 {
		t.Fatal("revoke failed")
	}
	for _, path := range []string{"/api/emails", "/api/transactional"} {
		if f.request("POST", path, `{}`, next.Key).Code != 401 {
			t.Fatal("revoked key accepted", path)
		}
	}
}
