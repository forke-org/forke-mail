// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package access

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/transactional"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type fakeMailer struct{ sent []transactional.Submission }

func (f *fakeMailer) Accept(_ context.Context, _ auth.Identity, _ string, req transactional.Submission) (transactional.Acceptance, error) {
	f.sent = append(f.sent, req)
	return transactional.Acceptance{ID: "x"}, nil
}

func TestEarlyAccessFlow(t *testing.T) {
	pool := dbtest.Pool(t)
	sender := dbtest.Workspace(t, pool, "paid", 0)
	mail := &fakeMailer{}
	email := "early-" + sender[:8] + "@example.com"
	username := "early" + sender[:8]
	t.Cleanup(func() {
		dbtest.Exec(t, pool, `DELETE FROM workspaces WHERE id IN (SELECT workspace_id FROM access_requests WHERE lower(email) = $1)`, email)
		dbtest.Exec(t, pool, `DELETE FROM access_requests WHERE lower(email) = $1`, email)
	})
	pub := &Public{Pool: pool, Workspace: WorkspaceDefaults{SendLimit: 3000, DailyLimit: 300, RatePerSec: 5}}
	plat := &Platform{Pool: pool, Token: "secret-token", Mail: mail, WorkspaceID: sender, From: "hello@forke.space", AppURL: "https://mail.example.com"}
	r := chi.NewRouter()
	r.Post("/api/access/request", pub.Request)
	r.Get("/api/access/invite", pub.Invite)
	r.Post("/api/access/invite", pub.Accept)
	plat.Routes(r)
	do := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	if w := do("POST", "/api/access/request", `{"email":"`+strings.ToUpper(email)+`"}`, ""); w.Code != 202 {
		t.Fatalf("request %d %s", w.Code, w.Body)
	}
	if w := do("POST", "/api/access/request", `{"email":"`+email+`"}`, ""); w.Code != 202 {
		t.Fatalf("repeat request %d", w.Code)
	}
	if w := do("GET", "/api/platform/access", "", "wrong"); w.Code != 404 {
		t.Fatalf("unguarded list %d", w.Code)
	}
	id := dbtest.QueryString(t, pool, `SELECT id::text FROM access_requests WHERE lower(email) = $1`, email)
	if w := do("POST", "/api/platform/access/"+id+"/approve", "", "secret-token"); w.Code != 200 {
		t.Fatalf("approve %d %s", w.Code, w.Body)
	}
	if len(mail.sent) != 1 || mail.sent[0].To != email {
		t.Fatalf("invite mail %+v", mail.sent)
	}
	link := mail.sent[0].Text[strings.Index(mail.sent[0].Text, "https://"):]
	token := strings.Fields(link[strings.Index(link, "token=")+6:])[0]

	if w := do("GET", "/api/access/invite?token="+token, "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), email) {
		t.Fatalf("invite lookup %d %s", w.Code, w.Body)
	}
	if w := do("POST", "/api/access/invite", `{"token":"`+token+`","name":"Ada","username":"`+username+`","workspace":"Ada Co","password":"short"}`, ""); w.Code != 422 {
		t.Fatalf("weak password %d", w.Code)
	}
	w := do("POST", "/api/access/invite", `{"token":"`+token+`","name":"Ada","username":"`+username+`","workspace":"Ada Co","password":"long-enough-password"}`, "")
	if w.Code != 201 || len(w.Result().Cookies()) == 0 {
		t.Fatalf("accept %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, pool, `
		SELECT w.plan || ':' || w.trial_send_limit || ':' || u.role || ':' || u.username || ':' || a.status
		FROM access_requests a JOIN workspaces w ON w.id = a.workspace_id JOIN users u ON u.workspace_id = w.id
		WHERE a.id = $1`, id); got != "trial:3000:super_admin:"+username+":joined" {
		t.Fatalf("account = %s", got)
	}
	if w := do("GET", "/api/access/invite?token="+token, "", ""); w.Code != 404 {
		t.Fatalf("token reused %d", w.Code)
	}
	var list struct{ Requests []Request }
	if err := json.Unmarshal(do("GET", "/api/platform/access", "", "secret-token").Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range list.Requests {
		found = found || (a.ID == id && a.Status == "joined" && a.Workspace != nil && *a.Workspace == "Ada Co")
	}
	if !found {
		t.Fatal("joined request missing from list")
	}

	ws := dbtest.QueryString(t, pool, `SELECT workspace_id::text FROM access_requests WHERE id = $1`, id)
	var accounts struct{ Accounts []Account }
	if err := json.Unmarshal(do("GET", "/api/platform/accounts", "", "secret-token").Body.Bytes(), &accounts); err != nil {
		t.Fatal(err)
	}
	var mine *Account
	for i := range accounts.Accounts {
		if accounts.Accounts[i].ID == ws {
			mine = &accounts.Accounts[i]
		}
	}
	if mine == nil || mine.Credits != 3000 || len(mine.Members) != 1 || mine.Members[0].Email != email {
		t.Fatalf("account %+v", mine)
	}
	if w := do("POST", "/api/platform/accounts/"+ws+"/credits", `{"amount":2000}`, "secret-token"); w.Code != 200 || !strings.Contains(w.Body.String(), "5000") {
		t.Fatalf("credits %d %s", w.Code, w.Body)
	}
	if w := do("POST", "/api/platform/accounts/"+ws+"/status", `{"status":"paused"}`, "secret-token"); w.Code != 200 {
		t.Fatalf("pause %d", w.Code)
	}
	if got := dbtest.QueryString(t, pool, `SELECT status || ':' || (SELECT COUNT(*) FROM sessions s JOIN users u ON u.id = s.user_id WHERE u.workspace_id = $1) FROM workspaces WHERE id = $1`, ws); got != "paused:0" {
		t.Fatalf("paused = %s", got)
	}
	if w := do("DELETE", "/api/platform/accounts/"+ws, "", "secret-token"); w.Code != 204 {
		t.Fatalf("delete %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, pool, `SELECT (SELECT COUNT(*) FROM workspaces WHERE id = $1) + (SELECT COUNT(*) FROM users WHERE lower(email) = $2) + (SELECT COUNT(*) FROM access_requests WHERE lower(email) = $2)`, ws, email); got != "0" {
		t.Fatalf("leftovers %s", got)
	}
}

func TestReservedWorkspaceName(t *testing.T) {
	b := acceptBody{Token: "x", Name: "A", Username: "abc", Workspace: "forke", Password: "long-enough-password"}
	if code := b.validate(); code != "" {
		t.Fatal(code)
	}
	p := &Public{ReservedName: "Forke"}
	r := chi.NewRouter()
	r.Post("/", p.Accept)
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"token":"x","name":"A","username":"abc","workspace":"FORKE","password":"long-enough-password"}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 422 || !strings.Contains(w.Body.String(), "workspace_name_reserved") {
		t.Fatalf("reserved %d %s", w.Code, w.Body)
	}
}

func TestPlatformLimitsMembersAndUsers(t *testing.T) {
	pool := dbtest.Pool(t)
	shared := dbtest.Workspace(t, pool, "trial", 100)
	solo := dbtest.Workspace(t, pool, "trial", 100)
	ctx := context.Background()
	var owner, mate string
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var err error
		if owner, err = accounts.CreateUser(ctx, tx, shared, "Owner", "own-"+shared[:8]+"@example.com", "long-enough-password"); err != nil {
			return err
		}
		if mate, err = accounts.CreateUser(ctx, tx, solo, "Mate", "mate-"+solo[:8]+"@example.com", "long-enough-password"); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'admin')`, shared, mate)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	plat := &Platform{Pool: pool, Token: "secret-token"}
	r := chi.NewRouter()
	plat.Routes(r)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	if w := do("POST", "/api/platform/accounts/"+shared+"/limits", `{"plan":"paid","daily_limit":5000,"rate_limit_per_sec":10}`); w.Code != 200 {
		t.Fatalf("limits %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, pool, `SELECT plan || ':' || daily_limit || ':' || rate_limit_per_sec FROM workspaces WHERE id = $1`, shared); got != "paid:5000:10" {
		t.Fatalf("limits = %s", got)
	}
	if w := do("POST", "/api/platform/accounts/"+shared+"/limits", `{"plan":"gold","daily_limit":1,"rate_limit_per_sec":1}`); w.Code != 422 {
		t.Fatalf("bad plan %d", w.Code)
	}
	if w := do("DELETE", "/api/platform/users/"+mate, ""); w.Code != 204 {
		t.Fatalf("delete user %d %s", w.Code, w.Body)
	}
	if got := dbtest.QueryString(t, pool, `SELECT (SELECT COUNT(*) FROM workspaces WHERE id = $1)::text || ':' || (SELECT COUNT(*) FROM workspaces WHERE id = $2) || ':' || (SELECT COUNT(*) FROM workspace_members WHERE workspace_id = $2)`, solo, shared); got != "0:1:1" {
		t.Fatalf("after delete solo:shared:members = %s", got)
	}
	if w := do("DELETE", "/api/platform/accounts/"+shared+"/members/"+owner, ""); w.Code != 204 {
		t.Fatalf("remove member %d", w.Code)
	}
}
