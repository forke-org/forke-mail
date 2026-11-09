package tracking

import (
	"net/http/httptest"
	"strings"
	"testing"

	"forkemail/internal/db/dbtest"

	"github.com/go-chi/chi/v5"
)

func TestBroadcastClickMarksContactEngaged(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	email := dbtest.DraftEmail(t, pool, ws)
	contact := dbtest.QueryString(t, pool, `INSERT INTO contacts (workspace_id, email, reengage_sent_at) VALUES ($1, 'reader@x.io', now() - interval '3 days') RETURNING id::text`, ws)
	recipient := dbtest.QueryString(t, pool, `INSERT INTO email_recipients (email_id, workspace_id, contact_id, email, status, sent_at) VALUES ($1, $2, $3, 'reader@x.io', 'sent', now()) RETURNING id::text`, email, ws, contact)
	tr := NewTracker([]byte(strings.Repeat("k", 32)), "https://mail.test")
	h := &Handler{Pool: pool, Tracker: tr}
	router := chi.NewRouter()
	router.Get("/t/o/{token}", h.Open)
	router.Get("/t/c/{token}", h.Click)
	out := tr.InstrumentRecipient(`<a href="https://forke.space/blog/x">Read</a></body>`, recipient, true, true)
	visit := func(prefix string) *httptest.ResponseRecorder {
		start := strings.Index(out, "https://mail.test"+prefix)
		end := start + strings.Index(out[start:], `"`)
		req := httptest.NewRequest("GET", strings.TrimPrefix(out[start:end], "https://mail.test"), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	visit("/t/o/")
	visit("/t/o/")
	if w := visit("/t/c/"); w.Code != 302 || w.Header().Get("Location") != "https://forke.space/blog/x" {
		t.Fatalf("click redirect %d %s", w.Code, w.Header().Get("Location"))
	}
	if got := dbtest.QueryString(t, pool, `SELECT open_count || ':' || click_count FROM email_recipients WHERE id = $1`, recipient); got != "2:1" {
		t.Fatalf("counts %s", got)
	}
	if got := dbtest.QueryString(t, pool, `SELECT (last_engaged_at IS NOT NULL AND reengage_sent_at IS NULL)::text FROM contacts WHERE id = $1`, contact); got != "true" {
		t.Fatal("click did not mark the contact engaged")
	}
	if got := dbtest.QueryString(t, pool, `SELECT string_agg(type, ',' ORDER BY id) FROM email_events WHERE recipient_id = $1`, recipient); got != "opened,clicked" {
		t.Fatalf("events %s", got)
	}
}
