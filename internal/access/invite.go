// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package access

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)

var errInviteInvalid = errors.New("invite_invalid")

func (p *Public) lookup(ctx context.Context, q pgx.Tx, token string, lock bool) (string, string, error) {
	var id, email string
	query := `SELECT id, email FROM access_requests WHERE invite_token_hash = $1 AND status = 'approved' AND invite_expires_at > now()`
	if lock {
		query += ` FOR UPDATE`
	}
	err := q.QueryRow(ctx, query, hashToken(token)).Scan(&id, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", errInviteInvalid
	}
	return id, email, err
}

func (p *Public) Invite(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	var email string
	err := pgx.BeginFunc(r.Context(), p.Pool, func(tx pgx.Tx) error {
		var err error
		_, email, err = p.lookup(r.Context(), tx, token, false)
		return err
	})
	if errors.Is(err, errInviteInvalid) {
		httpx.Error(w, http.StatusNotFound, "invite_invalid")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"email": email})
}

type acceptBody struct {
	Token     string `json:"token"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	Workspace string `json:"workspace"`
	Password  string `json:"password"`
}

func (b *acceptBody) validate() string {
	b.Name = strings.TrimSpace(b.Name)
	b.Username = strings.ToLower(strings.TrimSpace(b.Username))
	b.Workspace = strings.TrimSpace(b.Workspace)
	switch {
	case b.Name == "" || utf8.RuneCountInString(b.Name) > 100:
		return "invalid_name"
	case !usernamePattern.MatchString(b.Username):
		return "invalid_username"
	case b.Workspace == "" || utf8.RuneCountInString(b.Workspace) > 100:
		return "invalid_workspace_name"
	case len(b.Password) < 10 || len(b.Password) > 72:
		return "weak_password"
	}
	return ""
}

func (p *Public) Accept(w http.ResponseWriter, r *http.Request) {
	if !httpx.SameOrigin(r) {
		httpx.Error(w, http.StatusForbidden, "cross_origin_request")
		return
	}
	var body acceptBody
	if !httpx.Decode(w, r, &body) {
		return
	}
	if code := body.validate(); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	if p.ReservedName != "" && strings.EqualFold(body.Workspace, p.ReservedName) {
		httpx.Error(w, http.StatusUnprocessableEntity, "workspace_name_reserved")
		return
	}
	var userID string
	err := pgx.BeginFunc(r.Context(), p.Pool, func(tx pgx.Tx) error {
		requestID, email, err := p.lookup(r.Context(), tx, body.Token, true)
		if err != nil {
			return err
		}
		var taken bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM users WHERE lower(username) = $1)`, body.Username).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return errUsernameTaken
		}
		ws, err := accounts.CreateWorkspace(r.Context(), tx, accounts.NewWorkspace{
			Name: body.Workspace, Plan: "trial", DailyLimit: p.Workspace.DailyLimit, RatePerSec: p.Workspace.RatePerSec, TrialSendLimit: p.Workspace.SendLimit, Warmup: true,
		})
		if err != nil {
			return err
		}
		userID, err = accounts.CreateUser(r.Context(), tx, ws, body.Name, email, body.Password)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE users SET username = $2, role = 'super_admin' WHERE id = $1`, userID, body.Username); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `
			UPDATE access_requests SET status = 'joined', joined_at = now(), workspace_id = $2, invite_token_hash = NULL
			WHERE id = $1`, requestID, ws)
		return err
	})
	if err == nil {
		err = auth.StartSession(r.Context(), p.Pool, w, userID, p.SecureCookies)
	}
	switch {
	case errors.Is(err, errInviteInvalid):
		httpx.Error(w, http.StatusNotFound, "invite_invalid")
	case errors.Is(err, errUsernameTaken):
		httpx.Error(w, http.StatusConflict, "username_taken")
	case errors.Is(err, accounts.ErrEmailTaken):
		httpx.Error(w, http.StatusConflict, "email_taken")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		httpx.JSON(w, http.StatusCreated, map[string]bool{"ok": true})
	}
}

var errUsernameTaken = errors.New("username_taken")
