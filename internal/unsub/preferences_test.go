package unsub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"forkemail/internal/db/dbtest"

	"github.com/go-chi/chi/v5"
)

type prefFixture struct {
	t      *testing.T
	router http.Handler
	token  string
	ws     string
}

func newPrefFixture(t *testing.T) (*prefFixture, func(string) string) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	email := dbtest.DraftEmail(t, pool, ws)
	contact := dbtest.QueryString(t, pool, `INSERT INTO contacts (workspace_id, email) VALUES ($1, 'reader@example.com') RETURNING id::text`, ws)
	dbtest.Exec(t, pool, `INSERT INTO topics (workspace_id, key, label) VALUES ($1, 'blog', 'Blog posts'), ($1, 'changelog', 'Changelog')`, ws)
	h := &Handler{Pool: pool, Signer: Signer{Secret: []byte(strings.Repeat("s", 32))}}
	r := chi.NewRouter()
	r.Get("/unsubscribe/{token}", h.Confirm)
	r.Post("/unsubscribe/{token}", h.Unsubscribe)
	r.Post("/preferences/{token}", h.Update)
	f := &prefFixture{t: t, router: r, token: h.Signer.Sign(Target{WorkspaceID: ws, ContactID: contact, EmailID: email}), ws: ws}
	query := func(sql string) string { return dbtest.QueryString(t, pool, sql, contact) }
	return f, query
}

func (f *prefFixture) do(method, path string, form url.Values) string {
	f.t.Helper()
	r := httptest.NewRequest(method, path+f.token, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r.WithContext(context.Background()))
	if w.Code != 200 {
		f.t.Fatalf("%s %s: %d", method, path, w.Code)
	}
	return w.Body.String()
}

func TestPreferenceCentre(t *testing.T) {
	f, query := newPrefFixture(t)
	page := f.do("GET", "/unsubscribe/", nil)
	if !strings.Contains(page, "Blog posts") || !strings.Contains(page, "r•••••@example.com") {
		t.Fatalf("page missing topics or masked email:\n%s", page)
	}
	f.do("POST", "/preferences/", url.Values{"action": {"topics"}, "topic": {"changelog"}})
	if got := query(`SELECT array_to_string(topics_opted_out, ',') FROM contacts WHERE id = $1`); got != "blog" {
		t.Fatalf("opted out = %q", got)
	}
	f.do("POST", "/preferences/", url.Values{"action": {"pause"}})
	if got := query(`SELECT (paused_until > now() + interval '29 days')::text FROM contacts WHERE id = $1`); got != "true" {
		t.Fatal("pause not set to 30 days")
	}
	f.do("POST", "/preferences/", url.Values{"action": {"resume"}})
	if got := query(`SELECT (paused_until IS NULL)::text FROM contacts WHERE id = $1`); got != "true" {
		t.Fatal("resume did not clear pause")
	}
	page = f.do("POST", "/unsubscribe/", url.Values{"from": {"preferences"}})
	if !strings.Contains(page, "Subscribe again") {
		t.Fatal("resubscribe not offered after unsubscribing")
	}
	if got := query(`SELECT c.status || '/' || s.source FROM contacts c JOIN suppressions s ON lower(s.email) = lower(c.email) AND s.workspace_id = c.workspace_id WHERE c.id = $1`); got != "unsubscribed/preference_center" {
		t.Fatalf("after unsubscribe = %s", got)
	}
	f.do("POST", "/preferences/", url.Values{"action": {"resubscribe"}})
	if got := query(`SELECT status FROM contacts WHERE id = $1`); got != "active" {
		t.Fatalf("after resubscribe = %s", got)
	}
}

func TestResubscribeClosesAfterThirtyDaysAndNeverForComplaints(t *testing.T) {
	f, query := newPrefFixture(t)
	f.do("POST", "/unsubscribe/", nil)
	query(`UPDATE suppressions SET created_at = now() - interval '31 days' WHERE workspace_id = (SELECT workspace_id FROM contacts WHERE id = $1) RETURNING 'x'`)
	if page := f.do("GET", "/unsubscribe/", nil); strings.Contains(page, "Subscribe again") {
		t.Fatal("resubscribe offered after 30 days")
	}
	f.do("POST", "/preferences/", url.Values{"action": {"resubscribe"}})
	if got := query(`SELECT status FROM contacts WHERE id = $1`); got != "unsubscribed" {
		t.Fatalf("late resubscribe changed status to %s", got)
	}
	query(`UPDATE suppressions SET reason = 'complained', created_at = now() WHERE workspace_id = (SELECT workspace_id FROM contacts WHERE id = $1) RETURNING 'x'`)
	query(`UPDATE contacts SET status = 'complained' WHERE id = $1 RETURNING 'x'`)
	if page := f.do("GET", "/unsubscribe/", nil); strings.Contains(page, "Subscribe again") {
		t.Fatal("resubscribe offered after a spam complaint")
	}
}
