// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package team

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/contacts"
	"forkemail/internal/httpx"
	"forkemail/internal/transactional"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const inviteTTL = 7 * 24 * time.Hour

type Mailer interface {
	Accept(ctx context.Context, who auth.Identity, key string, req transactional.Submission) (transactional.Acceptance, error)
}

type Handler struct {
	Pool        *pgxpool.Pool
	Mail        Mailer
	WorkspaceID string
	From        string
	AppURL      string
}

type Member struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Username  *string   `json:"username"`
	Role      string    `json:"role"`
	You       bool      `json:"you"`
	CreatedAt time.Time `json:"created_at"`
}

type Invite struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newToken() (string, string) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	t := base64.RawURLEncoding.EncodeToString(b)
	return t, hashToken(t)
}

func (h *Handler) forkeManaged(ctx context.Context, ws string) (bool, error) {
	var managed bool
	err := h.Pool.QueryRow(ctx, `SELECT platform FROM workspaces WHERE id = $1`, ws).Scan(&managed)
	return managed, err
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	rows, err := h.Pool.Query(r.Context(), `
		SELECT u.id, u.name, u.email, u.username, m.role, m.created_at
		FROM workspace_members m JOIN users u ON u.id = m.user_id
		WHERE m.workspace_id = $1 ORDER BY m.created_at`, id.WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	members, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Member, error) {
		var m Member
		err := row.Scan(&m.ID, &m.Name, &m.Email, &m.Username, &m.Role, &m.CreatedAt)
		m.You = m.ID == id.UserID
		return m, err
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err = h.Pool.Query(r.Context(), `
		SELECT id, email, role, expires_at, created_at FROM team_invites WHERE workspace_id = $1 AND expires_at > now() ORDER BY created_at DESC`, id.WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	invites, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Invite])
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	managed, err := h.forkeManaged(r.Context(), id.WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"members": members, "invites": invites, "managed_by_forke": managed})
}

func (h *Handler) InviteMember(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	email := contacts.NormalizeEmail(body.Email)
	if !contacts.ValidEmail(email) {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_email")
		return
	}
	if body.Role != "super_admin" && body.Role != "admin" {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_role")
		return
	}
	managed, err := h.forkeManaged(r.Context(), id.WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if managed {
		httpx.Error(w, http.StatusConflict, "team_managed_by_forke")
		return
	}
	var existing string
	var member bool
	err = h.Pool.QueryRow(r.Context(), `
		SELECT u.id, EXISTS (SELECT 1 FROM workspace_members m WHERE m.user_id = u.id AND m.workspace_id = $2)
		FROM users u WHERE lower(u.email) = $1`, email, id.WorkspaceID).Scan(&existing, &member)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.Internal(w, r, err)
		return
	}
	if member {
		httpx.Error(w, http.StatusConflict, "already_member")
		return
	}
	if existing != "" {
		h.addExisting(w, r, existing, email, body.Role)
		return
	}
	token, hash := newToken()
	var invite Invite
	var workspace, inviter string
	err = h.Pool.QueryRow(r.Context(), `
		WITH ins AS (
			INSERT INTO team_invites (workspace_id, email, role, token_hash, invited_by, expires_at)
			VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid, now() + interval '7 days')
			ON CONFLICT (workspace_id, lower(email)) DO UPDATE SET role = EXCLUDED.role, token_hash = EXCLUDED.token_hash,
				invited_by = EXCLUDED.invited_by, expires_at = EXCLUDED.expires_at, created_at = now()
			RETURNING id, email, role, expires_at, created_at
		)
		SELECT ins.*, w.name, COALESCE((SELECT name FROM users WHERE id = NULLIF($5, '')::uuid), w.name)
		FROM ins, workspaces w WHERE w.id = $1`, id.WorkspaceID, email, body.Role, hash, id.UserID).
		Scan(&invite.ID, &invite.Email, &invite.Role, &invite.ExpiresAt, &invite.CreatedAt, &workspace, &inviter)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if err := h.send(r.Context(), invite, token, workspace, inviter); err != nil {
		httpx.Error(w, http.StatusBadGateway, "invite_email_failed")
		return
	}
	httpx.JSON(w, http.StatusCreated, invite)
}

func (h *Handler) send(ctx context.Context, inv Invite, token, workspace, inviter string) error {
	if h.Mail == nil || h.WorkspaceID == "" {
		return errors.New("team invites need a mail sender")
	}
	link := strings.TrimRight(h.AppURL, "/") + "/welcome/?team=" + token
	subject := inviter + " invited you to " + workspace + " on Forke Mail"
	text := inviter + " invited you to join " + workspace + " on Forke Mail.\n\nChoose a username and password here:\n" + link + "\n\nThe link works for 7 days.\n"
	body := `<div style="font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.6;color:#111;max-width:520px">` +
		`<h2 style="font-size:20px;margin:0 0 12px">Join ` + html.EscapeString(workspace) + `</h2>` +
		`<p style="margin:0 0 16px">` + html.EscapeString(inviter) + ` invited you to their team on Forke Mail.</p>` +
		`<p style="margin:0 0 20px"><a href="` + html.EscapeString(link) + `" style="display:inline-block;background:#FF7A00;color:#111;text-decoration:none;font-weight:600;padding:10px 18px;border-radius:6px">Accept invite</a></p>` +
		`<p style="margin:0;color:#555;font-size:13px">The link works for 7 days.</p></div>`
	_, err := h.Mail.Accept(ctx, auth.Identity{WorkspaceID: h.WorkspaceID}, fmt.Sprintf("forke-team-%s-%s", inv.ID, token[:12]), transactional.Submission{
		To: inv.Email, FromName: "Forke Mail", FromAddress: h.From, Subject: subject, HTML: body, Text: text, Tags: map[string]string{"category": "team_invite"},
	})
	return err
}

func (h *Handler) addExisting(w http.ResponseWriter, r *http.Request, userID, email, role string) {
	id := auth.FromContext(r.Context())
	var workspace, inviter string
	err := h.Pool.QueryRow(r.Context(), `
		WITH ins AS (INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, $3) RETURNING workspace_id)
		SELECT w.name, COALESCE((SELECT name FROM users WHERE id = NULLIF($4, '')::uuid), w.name) FROM ins JOIN workspaces w ON w.id = ins.workspace_id`,
		id.WorkspaceID, userID, role, id.UserID).Scan(&workspace, &inviter)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if h.Mail != nil && h.WorkspaceID != "" {
		link := strings.TrimRight(h.AppURL, "/") + "/"
		text := inviter + " added you to " + workspace + " on Forke Mail. Switch to it from the project menu after you log in:\n" + link + "\n"
		body := `<div style="font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.6;color:#111;max-width:520px">` +
			`<p>` + html.EscapeString(inviter) + ` added you to <b>` + html.EscapeString(workspace) + `</b> on Forke Mail. Switch to it from the project menu after you log in.</p>` +
			`<p><a href="` + html.EscapeString(link) + `" style="color:#111">Open Forke Mail</a></p></div>`
		_, _ = h.Mail.Accept(r.Context(), auth.Identity{WorkspaceID: h.WorkspaceID}, fmt.Sprintf("forke-added-%s-%s", id.WorkspaceID, userID), transactional.Submission{
			To: email, FromName: "Forke Mail", FromAddress: h.From, Subject: inviter + " added you to " + workspace, HTML: body, Text: text, Tags: map[string]string{"category": "team_added"},
		})
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"added": true, "email": email, "role": role})
}

func (h *Handler) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	invID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM team_invites WHERE id = $1 AND workspace_id = $2`, invID, auth.FromContext(r.Context()).WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	id := auth.FromContext(r.Context())
	if userID == id.UserID {
		httpx.Error(w, http.StatusConflict, "cannot_remove_yourself")
		return
	}
	var forke bool
	err := h.Pool.QueryRow(r.Context(), `
		SELECT w.platform FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 AND m.workspace_id = $2`, userID, id.WorkspaceID).Scan(&forke)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if forke {
		httpx.Error(w, http.StatusConflict, "team_managed_by_forke")
		return
	}
	if err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error { return accounts.RemoveMember(r.Context(), tx, id.WorkspaceID, userID) }); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
