package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/transactional"
	"forkemail/internal/webhooks"
	"github.com/jackc/pgx/v5"
)

type countingEnqueuer struct {
	calls atomic.Int32
	fail  bool
}

func (e *countingEnqueuer) Enqueue(_ context.Context, _ pgx.Tx, _ transactional.Job) error {
	e.calls.Add(1)
	if e.fail {
		return errors.New("test_enqueue_failed")
	}
	return nil
}

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
	f := newAPIFixture(t)
	domain := f.ws + ".example.com"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,kind,status) VALUES($1,$2,'domain','verified')`, f.ws, domain)
	e := &countingEnqueuer{}
	f.developer(&developerServices{hooks: &webhooks.Handler{Pool: f.pool}, transaction: &transactional.CompatibilityHandler{Pool: f.pool, Enqueuer: e}})
	body := `{"from":"noreply@` + domain + `","to":"person@example.com","subject":"Welcome","text":"Hello"}`
	first := f.sendRequest(body, "signup-1")
	if first.Code != 202 {
		t.Fatalf("send %d", first.Code)
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
	if e.calls.Load() != 1 {
		t.Fatal("duplicate enqueue")
	}
	if code := f.sendRequest(strings.Replace(body, "Welcome", "Changed", 1), "signup-1").Code; code != 409 {
		t.Fatalf("conflict %d", code)
	}
	if code := f.sendRequest(body, "").Code; code != 400 {
		t.Fatalf("missing idempotency %d", code)
	}
	if code := f.sendRequest(`{"from":"bad","to":"bad","subject":"x","text":"x"}`, "bad").Code; code != 422 {
		t.Fatalf("validation %d", code)
	}
	f.developer(&developerServices{hooks: &webhooks.Handler{Pool: f.pool}, transaction: &transactional.CompatibilityHandler{Pool: f.pool, Enqueuer: &countingEnqueuer{}}})
	if w := f.sendRequest(body, "signup-1"); w.Code != 202 || !strings.Contains(w.Body.String(), receipt.ID) {
		t.Fatal("idempotency lost on handler restart")
	}
	dbtest.Exec(t, f.pool, `INSERT INTO suppressions(workspace_id,email,reason) VALUES($1,'person@example.com','manual')`, f.ws)
	if code := f.sendRequest(body, "blocked").Code; code != 422 {
		t.Fatalf("suppressed recipient %d", code)
	}
}

func TestTransactionalRollbackAndStub(t *testing.T) {
	f := newAPIFixture(t)
	domain := f.ws + ".example.com"
	dbtest.Exec(t, f.pool, `INSERT INTO domains(workspace_id,name,kind,status) VALUES($1,$2,'domain','verified')`, f.ws, domain)
	body := `{"from":"noreply@` + domain + `","to":"person@example.com","subject":"Welcome","text":"Hello"}`
	f.developer(&developerServices{hooks: &webhooks.Handler{Pool: f.pool}, transaction: &transactional.CompatibilityHandler{Pool: f.pool, Enqueuer: &countingEnqueuer{fail: true}}})
	if code := f.sendRequest(body, "rollback").Code; code != 500 {
		t.Fatalf("enqueue failure %d", code)
	}
	if n := dbtest.QueryString(t, f.pool, `SELECT count(*)::text FROM transactional_requests WHERE workspace_id=$1`, f.ws); n != "0" {
		t.Fatal("failed enqueue committed receipt")
	}
	f.developer(&developerServices{hooks: &webhooks.Handler{Pool: f.pool}, transaction: &transactional.CompatibilityHandler{Pool: f.pool, Enqueuer: transactional.DevelopmentStub{}, Development: true}})
	w := f.sendRequest(body, "stub")
	if w.Code != 202 || !strings.Contains(w.Body.String(), "development_stub") {
		t.Fatal("stub not clearly marked")
	}
	f.developer(&developerServices{hooks: &webhooks.Handler{Pool: f.pool}})
	if code := f.sendRequest(body, "unavailable").Code; code != 503 {
		t.Fatalf("missing engine %d", code)
	}
	t.Setenv("TRANSACTIONAL_STUB_ENABLED", "true")
	if _, err := newDeveloperServices(f.pool, false); err == nil {
		t.Fatal("production stub allowed")
	}
}

func TestTransactionalBearerAuthentication(t *testing.T) {
	f := newAPIFixture(t)
	r := httptest.NewRequest(http.MethodPost, "/api/emails", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("anonymous send %d", w.Code)
	}
	w = f.request("POST", "/api/keys", `{"name":"app","scope":"sending_access","password":"test-password"}`, "")
	var key struct{ Key string }
	if err := json.Unmarshal(w.Body.Bytes(), &key); err != nil {
		t.Fatal(err)
	}
	if w.Code != 201 || key.Key == "" {
		t.Fatal("missing API key")
	}
	if code := f.request("POST", "/api/emails", `{}`, key.Key).Code; code != 503 {
		t.Fatalf("sending key cannot reach send adapter %d", code)
	}
}
