package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/transactional"
)

func (f apiFixture) sendRequest(body, key string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/emails", strings.NewReader(body))
	r.AddCookie(f.cookie)
	if key != "" {
		r.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}

func TestTransactionalIdempotency(t *testing.T) {
	f, _ := newIntegratedFixture(t)
	domain := f.ws + ".example.com"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,kind,status) VALUES($1,$2,'domain','verified')`, f.ws, domain)
	body := `{"from":"noreply@` + domain + `","to":"person@example.com","subject":"Welcome","text":"Hello"}`
	first := f.sendRequest(body, "signup-1")
	if first.Code != 202 {
		t.Fatalf("send %d %s", first.Code, first.Body.String())
	}
	var receipt transactional.Receipt
	if err := json.Unmarshal(first.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			w := f.sendRequest(body, "signup-1")
			var replay transactional.Receipt
			if err := json.Unmarshal(w.Body.Bytes(), &replay); err != nil {
				t.Error(err)
				return
			}
			if w.Code != 202 || replay.ID != receipt.ID || w.Header().Get("Idempotency-Replayed") != "true" {
				t.Error("unstable replay")
			}
		})
	}
	wg.Wait()
	if n := dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM transactional_messages WHERE workspace_id=$1`, f.ws); n != "1" {
		t.Fatal("duplicate jobs")
	}
	native := strings.Replace(body, `"from":`, `"from_address":`, 1)
	r := httptest.NewRequest("POST", "/api/transactional", strings.NewReader(native))
	r.AddCookie(f.cookie)
	r.Header.Set("Idempotency-Key", "signup-1")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Idempotent-Replayed") != "true" || !strings.Contains(w.Body.String(), receipt.ID) {
		t.Fatalf("cross-route replay %d %s", w.Code, w.Body.String())
	}
	if f.sendRequest(strings.Replace(body, "Welcome", "Changed", 1), "signup-1").Code != 409 {
		t.Fatal("changed payload accepted")
	}
	if f.sendRequest(body, "").Code != 400 {
		t.Fatal("missing key accepted")
	}
	if f.sendRequest(`{"from":"bad","to":"bad","subject":"x","text":"x"}`, "bad").Code != 422 {
		t.Fatal("invalid request accepted")
	}
	eventually(t, func() bool {
		return dbtest.QueryString(t, f.pool, `SELECT status FROM transactional_messages WHERE id=$1 AND workspace_id=$2`, receipt.ID, f.ws) == "sent"
	})
	if n := dbtest.QueryString(t, f.pool, `SELECT sent_count::text FROM usage_daily WHERE workspace_id=$1`, f.ws); n != "1" {
		t.Fatalf("quota reserved %s times", n)
	}
	dbtest.Exec(t, f.pool, `INSERT INTO suppressions(workspace_id,email,reason) VALUES($1,'person@example.com','manual')`, f.ws)
	if f.sendRequest(body, "blocked").Code != 422 {
		t.Fatal("suppression bypassed")
	}
	if n := dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM transactional_requests WHERE workspace_id=$1`, f.ws); n != "0" {
		t.Fatal("legacy idempotency store used")
	}
}

func TestTransactionalLegacyKeysAndAtomicRejection(t *testing.T) {
	f, _ := newIntegratedFixture(t)
	domain := f.ws + ".example.com"
	body := `{"from":"noreply@` + domain + `","to":"person@example.com","subject":"Welcome","text":"Hello"}`
	if f.sendRequest(body, "unverified").Code != 422 {
		t.Fatal("unverified sender accepted")
	}
	if n := dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM transactional_messages WHERE workspace_id=$1`, f.ws); n != "0" {
		t.Fatal("rejected job persisted")
	}
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,kind,status) VALUES($1,$2,'domain','verified')`, f.ws, domain)
	dbtest.Exec(t, f.pool, `INSERT INTO transactional_requests(workspace_id,idempotency_key,request_hash,mode) VALUES($1,'legacy',repeat('0',64),'development_stub')`, f.ws)
	response := f.sendRequest(body, "legacy")
	if response.Code != 409 || !strings.Contains(response.Body.String(), "legacy_stub_key_requires_new_key") {
		t.Fatal("legacy receipt suppressed real send")
	}
	response = f.sendRequest(body, "fresh")
	if response.Code != 202 || !strings.Contains(response.Body.String(), `"mode":"engine"`) {
		t.Fatal("real engine not wired")
	}
	t.Setenv("TRANSACTIONAL_STUB_ENABLED", "true")
	if _, err := newDeveloperServices(f.pool); err == nil {
		t.Fatal("obsolete stub flag silently enabled")
	}
}

func TestTransactionalBearerAuthentication(t *testing.T) {
	f := newAPIFixture(t)
	r := httptest.NewRequest(http.MethodPost, "/api/emails", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("anonymous send accepted")
	}
	w = f.request("POST", "/api/keys", `{"name":"app","scope":"sending_access","password":"test-password"}`, "")
	var key struct{ Key string }
	if err := json.Unmarshal(w.Body.Bytes(), &key); err != nil {
		t.Fatal(err)
	}
	if f.request("POST", "/api/emails", `{}`, key.Key).Code != 400 {
		t.Fatal("sending key cannot reach compatibility API")
	}
}
