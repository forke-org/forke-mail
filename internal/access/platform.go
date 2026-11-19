package access

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/domains"
	"forkemail/internal/httpx"
	"forkemail/internal/transactional"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const inviteTTL = 7 * 24 * time.Hour

type Mailer interface {
	Accept(ctx context.Context, who auth.Identity, key string, req transactional.Submission) (transactional.Acceptance, error)
}

type Platform struct {
	Pool        *pgxpool.Pool
	Token       string
	Mail        Mailer
	WorkspaceID string
	From        string
	AppURL      string
	Identities  domains.Identities
}

type Request struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	DecidedAt       *time.Time `json:"decided_at"`
	JoinedAt        *time.Time `json:"joined_at"`
	InviteExpiresAt *time.Time `json:"invite_expires_at"`
	InvitesSent     int        `json:"invites_sent"`
	Workspace       *string    `json:"workspace"`
	Username        *string    `json:"username"`
	Sent            int        `json:"sent"`
}

func (p *Platform) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !httpx.TokenMatches(got, p.Token) {
			httpx.Error(w, http.StatusNotFound, "not_found")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (p *Platform) List(w http.ResponseWriter, r *http.Request) {
	rows, err := p.Pool.Query(r.Context(), `
		SELECT a.id, a.email, CASE WHEN a.status = 'approved' AND a.invite_expires_at < now() THEN 'expired' ELSE a.status END,
			a.created_at, a.decided_at, a.joined_at, a.invite_expires_at, a.invites_sent, w.name,
			(SELECT u.username FROM users u WHERE u.workspace_id = a.workspace_id ORDER BY u.created_at LIMIT 1),
			COALESCE((SELECT COUNT(*) FROM transactional_messages t WHERE t.workspace_id = a.workspace_id AND t.sent_at IS NOT NULL), 0)
				+ COALESCE((SELECT COUNT(*) FROM email_recipients e WHERE e.workspace_id = a.workspace_id AND e.sent_at IS NOT NULL), 0)
		FROM access_requests a LEFT JOIN workspaces w ON w.id = a.workspace_id
		ORDER BY a.status = 'pending' DESC, a.created_at DESC
		LIMIT 500`)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Request, error) {
		var a Request
		return a, row.Scan(&a.ID, &a.Email, &a.Status, &a.CreatedAt, &a.DecidedAt, &a.JoinedAt, &a.InviteExpiresAt, &a.InvitesSent, &a.Workspace, &a.Username, &a.Sent)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"requests": list})
}

func (p *Platform) Approve(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	token, hash := newToken()
	var email string
	err := p.Pool.QueryRow(r.Context(), `
		UPDATE access_requests SET status = 'approved', decided_at = COALESCE(decided_at, now()), invite_token_hash = $2,
			invite_expires_at = now() + $3::interval, invites_sent = invites_sent + 1
		WHERE id = $1 AND status IN ('pending', 'approved', 'rejected') RETURNING email`,
		id, hash, fmt.Sprintf("%d seconds", int(inviteTTL.Seconds()))).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if err := p.sendInvite(r.Context(), id, email, token); err != nil {
		httpx.Error(w, http.StatusBadGateway, "invite_email_failed")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (p *Platform) Reject(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	tag, err := p.Pool.Exec(r.Context(), `
		UPDATE access_requests SET status = 'rejected', decided_at = now(), invite_token_hash = NULL, invite_expires_at = NULL
		WHERE id = $1 AND status IN ('pending', 'approved')`, id)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

func (p *Platform) sendInvite(ctx context.Context, id, email, token string) error {
	if p.Mail == nil || p.WorkspaceID == "" {
		return errors.New("invite mailer not configured")
	}
	link := strings.TrimRight(p.AppURL, "/") + "/welcome/?token=" + token
	subject := "Your Forke Mail early access is ready"
	text := "You're in. Choose a username and password to open your Forke Mail workspace:\n\n" + link +
		"\n\nThe link works for 7 days. Forke Mail sends through Amazon SES. Add your own domain once you're in.\n"
	body := `<div style="font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.6;color:#111;max-width:520px">` +
		`<h2 style="font-size:20px;margin:0 0 12px">You're in.</h2>` +
		`<p style="margin:0 0 16px">Your early access to Forke Mail is approved. Choose a username and password to open your workspace.</p>` +
		`<p style="margin:0 0 20px"><a href="` + html.EscapeString(link) + `" style="display:inline-block;background:#FF7A00;color:#111;text-decoration:none;font-weight:600;padding:10px 18px;border-radius:6px">Set up your account</a></p>` +
		`<p style="margin:0 0 8px;color:#555;font-size:13px">The link works for 7 days. Forke Mail sends through Amazon SES. Add your own domain once you're in.</p></div>`
	_, err := p.Mail.Accept(ctx, auth.Identity{WorkspaceID: p.WorkspaceID}, fmt.Sprintf("forke-invite-%s-%s", id, token[:12]), transactional.Submission{
		To: email, FromName: "Forke Mail", FromAddress: p.From, Subject: subject, HTML: body, Text: text,
		Tags: map[string]string{"category": "invite"},
	})
	return err
}

func (p *Platform) Routes(r chi.Router) {
	r.Group(func(g chi.Router) {
		g.Use(p.Guard)
		g.Get("/api/platform/access", p.List)
		g.Post("/api/platform/access/{id}/approve", p.Approve)
		g.Post("/api/platform/access/{id}/reject", p.Reject)
		g.Delete("/api/platform/access/{id}", p.DeleteRequest)
		g.Get("/api/platform/accounts", p.Accounts)
		g.Post("/api/platform/accounts/{id}/credits", p.AddCredits)
		g.Post("/api/platform/accounts/{id}/status", p.SetStatus)
		g.Delete("/api/platform/accounts/{id}", p.DeleteAccount)
		g.Post("/api/platform/accounts/{id}/limits", p.SetLimits)
		g.Delete("/api/platform/accounts/{id}/members/{user}", p.RemoveMember)
		g.Delete("/api/platform/users/{id}", p.DeleteUser)
	})
}
