package unsub

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"forkemail/internal/contacts"
	"forkemail/internal/events"
	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed page.html
var pageFS embed.FS

var page = template.Must(template.New("page.html").Funcs(template.FuncMap{
	"date": func(t *time.Time) string { return t.Format("2 January 2006") },
}).ParseFS(pageFS, "page.html"))

var (
	errContactGone       = errors.New("contact_gone")
	errNotResubscribable = errors.New("not_resubscribable")
)

type Handler struct {
	Pool   *pgxpool.Pool
	Signer Signer
}

type view struct {
	State  string
	Notice string
	Token  string
	Email  string
	Prefs  preferences
}

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	h.show(w, r, "")
}

func (h *Handler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	t, err := h.verify(r)
	if err != nil {
		render(w, http.StatusNotFound, view{State: "invalid"})
		return
	}
	source := "unsubscribe_link"
	if r.FormValue("from") == "preferences" {
		source = "preference_center"
	}
	err = pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		return apply(r.Context(), tx, t, source)
	})
	if errors.Is(err, errContactGone) {
		render(w, http.StatusNotFound, view{State: "invalid"})
		return
	}
	if err != nil {
		slog.Error("unsubscribe failed", "err", err)
		render(w, http.StatusInternalServerError, view{State: "invalid"})
		return
	}
	h.show(w, r, "unsubscribed")
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	t, err := h.verify(r)
	if err != nil {
		render(w, http.StatusNotFound, view{State: "invalid"})
		return
	}
	notice := ""
	err = pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		switch r.FormValue("action") {
		case "topics":
			notice = "saved"
			return saveTopics(r.Context(), tx, t, r.Form["topic"])
		case "pause":
			notice = "paused"
			until := time.Now().Add(pauseLength)
			return setPause(r.Context(), tx, t, &until)
		case "resume":
			notice = "resumed"
			return setPause(r.Context(), tx, t, nil)
		case "resubscribe":
			notice = "resubscribed"
			return resubscribe(r.Context(), tx, t)
		}
		return errNotResubscribable
	})
	if errors.Is(err, errContactGone) {
		render(w, http.StatusNotFound, view{State: "invalid"})
		return
	}
	if errors.Is(err, errNotResubscribable) {
		notice = "unchanged"
	} else if err != nil {
		slog.Error("update preferences failed", "err", err)
		render(w, http.StatusInternalServerError, view{State: "invalid"})
		return
	}
	h.show(w, r, notice)
}

func (h *Handler) show(w http.ResponseWriter, r *http.Request, notice string) {
	t, err := h.verify(r)
	if err != nil {
		render(w, http.StatusNotFound, view{State: "invalid"})
		return
	}
	var p preferences
	err = pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		p, err = loadPreferences(r.Context(), tx, t)
		return err
	})
	if errors.Is(err, errContactGone) {
		render(w, http.StatusNotFound, view{State: "invalid"})
		return
	}
	if err != nil {
		slog.Error("load preferences failed", "err", err)
		render(w, http.StatusInternalServerError, view{State: "invalid"})
		return
	}
	render(w, http.StatusOK, view{State: "preferences", Notice: notice, Token: chi.URLParam(r, "token"), Email: maskEmail(p.Email), Prefs: p})
}

func (h *Handler) verify(r *http.Request) (Target, error) {
	t, err := h.Signer.Verify(chi.URLParam(r, "token"))
	if err != nil || !httpx.ValidID(t.WorkspaceID) || !httpx.ValidID(t.ContactID) || !httpx.ValidID(t.EmailID) {
		return Target{}, errInvalidToken
	}
	return t, nil
}

func apply(ctx context.Context, tx pgx.Tx, t Target, source string) error {
	var email string
	err := tx.QueryRow(ctx, `SELECT email FROM contacts WHERE id = $1 AND workspace_id = $2`, t.ContactID, t.WorkspaceID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return errContactGone
	}
	if err != nil {
		return err
	}
	changed, err := contacts.Suppress(ctx, tx, contacts.Suppression{
		WorkspaceID: t.WorkspaceID, Email: email, Reason: "unsubscribed", Source: source,
		Detail: map[string]any{"email_id": t.EmailID},
	})
	if err != nil || !changed {
		return err
	}
	var recipientID string
	err = tx.QueryRow(ctx, `
		UPDATE email_recipients SET unsubscribed_at = COALESCE(unsubscribed_at, now())
		WHERE email_id = $1 AND contact_id = $2 AND workspace_id = $3
		RETURNING id`, t.EmailID, t.ContactID, t.WorkspaceID).Scan(&recipientID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	emailID := t.EmailID
	if recipientID == "" {
		emailID = ""
	}
	return events.Record(ctx, tx, events.Event{
		WorkspaceID: t.WorkspaceID, EmailID: emailID, RecipientID: recipientID,
		Type: "unsubscribed", Detail: map[string]any{"email": email, "source": source},
	})
}

func render(w http.ResponseWriter, status int, v view) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = page.Execute(w, v)
}
