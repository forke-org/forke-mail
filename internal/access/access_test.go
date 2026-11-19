package access

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"forkemail/internal/auth"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/transactional"

	"github.com/go-chi/chi/v5"
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
}
