// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package statusmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/transactional"

	"github.com/go-chi/chi/v5"
)

type fakeMail struct{ sent []transactional.Submission }

func (f *fakeMail) Accept(_ context.Context, _ auth.Identity, _ string, s transactional.Submission) (transactional.Acceptance, error) {
	f.sent = append(f.sent, s)
	return transactional.Acceptance{}, nil
}

func linkIn(t *testing.T, body, prefix string) string {
	t.Helper()
	i := strings.Index(body, prefix)
	if i < 0 {
		t.Fatalf("no %s link in %s", prefix, body)
	}
	end := strings.IndexAny(body[i:], "\"\n ")
	return body[i : i+end]
}

func TestSubscribeConfirmNotifyUnsubscribe(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	mail := &fakeMail{}
	h := &Handler{Pool: pool, Mail: mail, WorkspaceID: ws, From: "status@example.com", BaseURL: "https://mail.test", PageURL: "https://status.test", Secret: []byte("secret")}
	r := chi.NewRouter()
	r.Post("/subscribe", h.Subscribe)
	r.Get("/status/confirm/{token}", h.Confirm)
	r.Get("/status/unsubscribe/{token}", h.Unsubscribe)
	do := func(method, target, form string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	if w := do("POST", "/subscribe", url.Values{"email": {"not-an-email"}}.Encode()); w.Code != 422 {
		t.Fatalf("invalid email %d", w.Code)
	}
	if w := do("POST", "/subscribe", url.Values{"email": {"Reader@Example.com"}}.Encode()); w.Code != 202 || len(mail.sent) != 1 {
		t.Fatalf("subscribe %d %d", w.Code, len(mail.sent))
	}
	tagged := `SELECT COUNT(*)::text FROM contact_tags ct JOIN tags t ON t.id = ct.tag_id WHERE t.workspace_id = $1 AND t.name = 'Status subscribers'`
	if dbtest.QueryString(t, pool, tagged, ws) != "0" {
		t.Fatal("subscribed before confirming")
	}
	confirm := strings.TrimPrefix(linkIn(t, mail.sent[0].Text, "https://mail.test/status/confirm/"), "https://mail.test")
	if w := do("GET", confirm, ""); w.Code != http.StatusFound || !strings.HasSuffix(w.Header().Get("Location"), "subscribe=confirmed") {
		t.Fatalf("confirm %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := do("GET", confirm+"x", ""); !strings.HasSuffix(w.Header().Get("Location"), "subscribe=expired") {
		t.Fatal("tampered token accepted")
	}
	if dbtest.QueryString(t, pool, tagged, ws) != "1" {
		t.Fatal("not tagged after confirming")
	}

	n := &Notifier{Handler: h}
	now := time.Now()
	resolved := now.Add(-time.Minute)
	incident := Incident{ID: ws + "-api", Name: "Forke Mail API", Impact: "down", StartedAt: now.Add(-20 * time.Minute)}
	old := Incident{ID: ws + "-old", Name: "Old", StartedAt: now.Add(-48 * time.Hour)}
	for range 2 {
		if err := n.Notify(t.Context(), []Incident{incident, old}, now); err != nil {
			t.Fatal(err)
		}
	}
	if len(mail.sent) != 2 || !strings.Contains(mail.sent[1].Subject, "is not working") {
		t.Fatalf("opened notice %d %v", len(mail.sent), mail.sent[len(mail.sent)-1].Subject)
	}
	incident.ResolvedAt = &resolved
	if err := n.Notify(t.Context(), []Incident{incident}, now); err != nil {
		t.Fatal(err)
	}
	if len(mail.sent) != 3 || !strings.Contains(mail.sent[2].Subject, "back to normal") {
		t.Fatalf("resolved notice %d", len(mail.sent))
	}

	unsub := strings.TrimPrefix(linkIn(t, mail.sent[2].Text, "https://mail.test/status/unsubscribe/"), "https://mail.test")
	if w := do("GET", unsub, ""); !strings.HasSuffix(w.Header().Get("Location"), "subscribe=removed") {
		t.Fatalf("unsubscribe %s", w.Header().Get("Location"))
	}
	if dbtest.QueryString(t, pool, tagged, ws) != "0" {
		t.Fatal("still subscribed")
	}
	dbtest.Exec(t, pool, `DELETE FROM status_notifications WHERE incident_id LIKE $1`, ws+"%")
}
