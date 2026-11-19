package team

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)

var (
	errInvalid       = errors.New("invite_invalid")
	errUsernameTaken = errors.New("username_taken")
)

type Join struct {
	Pool          *pgxpool.Pool
	SecureCookies bool
}

func (j *Join) Lookup(w http.ResponseWriter, r *http.Request) {
	var email, workspace string
	err := j.Pool.QueryRow(r.Context(), `
		SELECT i.email, w.name FROM team_invites i JOIN workspaces w ON w.id = i.workspace_id
		WHERE i.token_hash = $1 AND i.expires_at > now() AND w.status = 'active'`, hashToken(r.URL.Query().Get("token"))).Scan(&email, &workspace)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "invite_invalid")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"email": email, "workspace": workspace})
}

func (j *Join) Accept(w http.ResponseWriter, r *http.Request) {
	if !httpx.SameOrigin(r) {
		httpx.Error(w, http.StatusForbidden, "cross_origin_request")
		return
	}
	var body struct {
		Token    string `json:"token"`
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Username = strings.ToLower(strings.TrimSpace(body.Username))
	switch {
	case body.Name == "" || utf8.RuneCountInString(body.Name) > 100:
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_name")
		return
	case !usernamePattern.MatchString(body.Username):
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_username")
		return
	case len(body.Password) < 10 || len(body.Password) > 72:
		httpx.Error(w, http.StatusUnprocessableEntity, "weak_password")
		return
	}
	var userID string
	err := pgx.BeginFunc(r.Context(), j.Pool, func(tx pgx.Tx) error {
		var inviteID, ws, email, role string
		err := tx.QueryRow(r.Context(), `
			SELECT i.id, i.workspace_id, i.email, i.role FROM team_invites i JOIN workspaces w ON w.id = i.workspace_id
			WHERE i.token_hash = $1 AND i.expires_at > now() AND w.status = 'active' FOR UPDATE OF i`, hashToken(body.Token)).Scan(&inviteID, &ws, &email, &role)
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalid
		}
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
		if userID, err = accounts.CreateUser(r.Context(), tx, ws, body.Name, email, body.Password); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE users SET username = $2, role = $3 WHERE id = $1`, userID, body.Username, role); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `DELETE FROM team_invites WHERE id = $1`, inviteID)
		return err
	})
	if err == nil {
		err = auth.StartSession(r.Context(), j.Pool, w, userID, j.SecureCookies)
	}
	switch {
	case errors.Is(err, errInvalid):
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
