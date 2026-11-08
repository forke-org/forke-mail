package unsub

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"

	"forkemail/internal/contacts"
	"forkemail/internal/events"
	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed page.html
var pageFS embed.FS

var page = template.Must(template.ParseFS(pageFS, "page.html"))

var errContactGone = errors.New("contact_gone")

type Handler struct {
	Pool   *pgxpool.Pool
	Signer Signer
}

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	if _, err := h.verify(r); err != nil {
		render(w, http.StatusNotFound, "invalid")
		return
	}
	render(w, http.StatusOK, "confirm")
}

func (h *Handler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	t, err := h.verify(r)
	if err != nil {
		render(w, http.StatusNotFound, "invalid")
		return
	}
	err = pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		return apply(r.Context(), tx, t)
	})
	if errors.Is(err, errContactGone) {
		render(w, http.StatusNotFound, "invalid")
		return
	}
	if err != nil {
		slog.Error("unsubscribe failed", "err", err)
		render(w, http.StatusInternalServerError, "invalid")
		return
	}
	render(w, http.StatusOK, "done")
}

func (h *Handler) verify(r *http.Request) (Target, error) {
	t, err := h.Signer.Verify(chi.URLParam(r, "token"))
	if err != nil || !httpx.ValidID(t.WorkspaceID) || !httpx.ValidID(t.ContactID) || !httpx.ValidID(t.EmailID) {
		return Target{}, errInvalidToken
	}
	return t, nil
}

func apply(ctx context.Context, tx pgx.Tx, t Target) error {
	var email string
	err := tx.QueryRow(ctx, `SELECT email FROM contacts WHERE id = $1 AND workspace_id = $2`, t.ContactID, t.WorkspaceID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return errContactGone
	}
	if err != nil {
		return err
	}
	changed, err := contacts.Suppress(ctx, tx, contacts.Suppression{
		WorkspaceID: t.WorkspaceID, Email: email, Reason: "unsubscribed", Source: "unsubscribe_link",
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
		Type: "unsubscribed", Detail: map[string]any{"email": email},
	})
}

func render(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = page.Execute(w, map[string]string{"State": state})
}
