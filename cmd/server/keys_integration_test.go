package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/ratelimit"
	"forkemail/internal/sender"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type apiFixture struct {
	worker *sender.Worker
	pool   *pgxpool.Pool
	ws     string
	cookie *http.Cookie
	router http.Handler
}

func newAPIFixture(t *testing.T) apiFixture {
	t.Helper()
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	rec := httptest.NewRecorder()
	err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
		user, err := accounts.CreateUser(context.Background(), tx, ws, "Test", ws+"@example.com", "test-password")
		if err != nil {
			return err
		}
		return auth.StartSession(context.Background(), tx, rec, user, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	lim := limiters{public: ratelimit.NewStore(), keys: ratelimit.NewStore(), failures: ratelimit.NewStore()}
	return apiFixture{pool: pool, ws: ws, cookie: rec.Result().Cookies()[0], router: newRouter(testConfig("development"), pool, lim, services{sendRate: func() float64 { return 20 }})}
}

func (f apiFixture) request(method, path, body, key string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if key == "" {
		r.AddCookie(f.cookie)
	} else {
		r.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}

func TestAPIKeyLifecycle(t *testing.T) {
	f := newAPIFixture(t)
	created := f.request("POST", "/api/keys", `{"name":"app","scope":"sending_access","password":"test-password"}`, "")
	if created.Code != 201 {
		t.Fatalf("create status %d", created.Code)
	}
	var first struct{ ID, Key string }
	if err := json.Unmarshal(created.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if first.Key == "" {
		t.Fatal("missing one-time key")
	}
	if created.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("secret response is cacheable")
	}
	listed := f.request("GET", "/api/keys", "", "")
	if listed.Code != 200 || strings.Contains(listed.Body.String(), first.Key) || strings.Contains(listed.Body.String(), "key_hash") || strings.Contains(listed.Body.String(), `"key":`) {
		t.Fatal("list leaked a key or failed")
	}
	if got := f.request("GET", "/api/me", "", first.Key).Code; got != 200 {
		t.Fatalf("authenticate %d", got)
	}
	for _, tc := range []struct{ method, path string }{{"GET", "/api/keys"}, {"POST", "/api/keys"}, {"POST", "/api/keys/" + first.ID + "/rotate"}, {"DELETE", "/api/keys/" + first.ID}} {
		if got := f.request(tc.method, tc.path, `{}`, first.Key).Code; got != 403 {
			t.Fatalf("sending key management %s: %d", tc.path, got)
		}
	}
	other := newAPIFixture(t)
	for _, method := range []string{"DELETE", "POST"} {
		path := "/api/keys/" + first.ID
		if method == "POST" {
			path += "/rotate"
		}
		if got := other.request(method, path, `{"password":"test-password"}`, "").Code; got != 404 {
			t.Fatalf("foreign key %d", got)
		}
	}
	rotated := f.request("POST", "/api/keys/"+first.ID+"/rotate", `{"password":"test-password"}`, "")
	if rotated.Code != 201 {
		t.Fatalf("rotate %d", rotated.Code)
	}
	var second struct{ ID, Key string }
	if err := json.Unmarshal(rotated.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if f.request("GET", "/api/me", "", first.Key).Code != 401 {
		t.Fatal("old key remained usable")
	}
	if f.request("GET", "/api/me", "", second.Key).Code != 200 {
		t.Fatal("new key rejected")
	}
	if f.request("DELETE", "/api/keys/"+second.ID, "", "").Code != 204 {
		t.Fatal("revoke failed")
	}
	if f.request("GET", "/api/me", "", second.Key).Code != 401 {
		t.Fatal("revoked key remained usable")
	}
	stored := dbtest.QueryString(t, f.pool, `SELECT key_hash FROM api_keys WHERE id=$1 AND workspace_id=$2`, second.ID, f.ws)
	if stored == second.Key || stored != auth.HashKey(second.Key) {
		t.Fatal("key not stored as hash")
	}
}
