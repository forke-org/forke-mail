// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package statusmail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/contacts"
	"forkemail/internal/httpx"
	"forkemail/internal/transactional"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SubscriberTag = "Status subscribers"

type Mailer interface {
	Accept(ctx context.Context, who auth.Identity, key string, req transactional.Submission) (transactional.Acceptance, error)
}

type Handler struct {
	Pool        *pgxpool.Pool
	Mail        Mailer
	WorkspaceID string
	From        string
	BaseURL     string
	PageURL     string
	Secret      []byte
}

func (h *Handler) Subscribe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	email := contacts.NormalizeEmail(r.FormValue("email"))
	if !contacts.ValidEmail(email) {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_email")
		return
	}
	if h.WorkspaceID == "" || h.Mail == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "subscriptions_unavailable")
		return
	}
	link := h.BaseURL + "/status/confirm/" + signer(h.Secret).sign(confirmKind, email, time.Now().Add(confirmLifetime))
	subject := "Confirm Forke status updates"
	lines := []string{
		"Someone asked to get an email whenever a Forke service has a problem and when it is fixed.",
		"If that was you, confirm below. If not, ignore this email and nothing will be sent.",
	}
	body, text := render(subject, lines, link, "Confirm subscription", "")
	sum := sha256.Sum256([]byte(email + time.Now().UTC().Format("2006010215")))
	_, err := h.Mail.Accept(r.Context(), auth.Identity{WorkspaceID: h.WorkspaceID}, "status-confirm-"+hex.EncodeToString(sum[:12]), transactional.Submission{
		To: email, FromName: "Forke Status", FromAddress: h.From, Subject: subject, HTML: body, Text: text,
		Tags: map[string]string{"category": "status"},
	})
	if err != nil {
		slog.Warn("status confirm email", "err", err)
	}
	httpx.JSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	email, err := signer(h.Secret).verify(chi.URLParam(r, "token"), confirmKind, time.Now())
	if err != nil || h.WorkspaceID == "" {
		http.Redirect(w, r, h.PageURL+"/?subscribe=expired", http.StatusFound)
		return
	}
	err = pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO contacts (workspace_id, email) VALUES ($1, $2)
			ON CONFLICT (workspace_id, lower(email)) DO UPDATE SET email = contacts.email
			RETURNING id`, h.WorkspaceID, email).Scan(&id); err != nil {
			return err
		}
		return contacts.AttachTags(r.Context(), tx, h.WorkspaceID, id, []string{SubscriberTag})
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	http.Redirect(w, r, h.PageURL+"/?subscribe=confirmed", http.StatusFound)
}

func (h *Handler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	email, err := signer(h.Secret).verify(chi.URLParam(r, "token"), unsubscribeKind, time.Now())
	if err != nil {
		http.Redirect(w, r, h.PageURL+"/?subscribe=expired", http.StatusFound)
		return
	}
	if _, err := h.Pool.Exec(r.Context(), `
		DELETE FROM contact_tags ct USING contacts c, tags t
		WHERE ct.contact_id = c.id AND ct.tag_id = t.id AND c.workspace_id = $1 AND t.workspace_id = $1
			AND lower(c.email) = lower($2) AND lower(t.name) = lower($3)`, h.WorkspaceID, email, SubscriberTag); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	http.Redirect(w, r, h.PageURL+"/?subscribe=removed", http.StatusFound)
}

func (h *Handler) unsubscribeURL(email string) string {
	return h.BaseURL + "/status/unsubscribe/" + signer(h.Secret).sign(unsubscribeKind, email, time.Time{})
}

func render(subject string, lines []string, link, label, footer string) (string, string) {
	var b strings.Builder
	b.WriteString(`<div style="background:#151414;padding:32px 16px"><div style="font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.6;color:#e7e5e4;max-width:520px;margin:0 auto;background:#1c1b1b;border:1px solid #2a2828;border-radius:8px;padding:28px">`)
	b.WriteString(`<p style="margin:0 0 18px;font-size:12px;letter-spacing:.08em;text-transform:uppercase;color:#a8a29e">Forke status</p>`)
	b.WriteString(`<h1 style="font-size:20px;line-height:1.3;margin:0 0 14px;color:#fff">` + html.EscapeString(subject) + `</h1>`)
	for _, l := range lines {
		b.WriteString(`<p style="margin:0 0 12px">` + html.EscapeString(l) + `</p>`)
	}
	b.WriteString(`<p style="margin:22px 0 0"><a href="` + html.EscapeString(link) + `" style="display:inline-block;background:#FF7A00;color:#151414;text-decoration:none;font-weight:600;padding:10px 18px;border-radius:6px">` + html.EscapeString(label) + `</a></p>`)
	if footer != "" {
		b.WriteString(`<p style="margin:26px 0 0;font-size:12px;color:#a8a29e"><a href="` + html.EscapeString(footer) + `" style="color:#a8a29e">Stop status emails</a></p>`)
	}
	b.WriteString(`</div></div>`)
	text := subject + "\n\n" + strings.Join(lines, "\n\n") + "\n\n" + label + ": " + link + "\n"
	if footer != "" {
		text += "\nStop status emails: " + footer + "\n"
	}
	return b.String(), text
}
