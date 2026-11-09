package accounts

import (
	"context"
	"errors"
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/contacts"
	"forkemail/internal/forke"
	"forkemail/internal/httpx"
	"forkemail/internal/ratelimit"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	Pool          *pgxpool.Pool
	Admins        *forke.Directory
	WorkspaceName string
	SecureCookies bool
	Failures      ratelimit.FailureGuard
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if !httpx.SameOrigin(r) {
		httpx.Error(w, http.StatusForbidden, "cross_origin_request")
		return
	}
	if h.Failures.Reject(w, r) {
		return
	}
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	userID, err := h.authenticate(r.Context(), body.Email, body.Password)
	if errors.Is(err, forke.ErrInvalidCredentials) {
		h.Failures.Fail(r)
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if _, err := h.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if err := auth.StartSession(r.Context(), h.Pool, w, userID, h.SecureCookies); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) authenticate(ctx context.Context, login, password string) (string, error) {
	if h.Admins == nil {
		return localLogin(ctx, h.Pool, login, password)
	}
	admin, err := h.Admins.Authenticate(ctx, login, password)
	if err != nil {
		return "", err
	}
	var userID string
	err = pgx.BeginFunc(ctx, h.Pool, func(tx pgx.Tx) error {
		wsID, err := ForkeWorkspace(ctx, tx, h.WorkspaceName)
		if err != nil {
			return err
		}
		userID, err = UpsertAdmin(ctx, tx, wsID, admin)
		return err
	})
	return userID, err
}

func localLogin(ctx context.Context, pool *pgxpool.Pool, login, password string) (string, error) {
	var userID, status string
	var hash *string
	err := pool.QueryRow(ctx, `
		SELECT u.id, u.password_hash, w.status FROM users u JOIN workspaces w ON w.id = u.workspace_id
		WHERE lower(u.email) = $1`, contacts.NormalizeEmail(login)).Scan(&userID, &hash, &status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	compare := dummyHash
	if hash != nil {
		compare = []byte(*hash)
	}
	if bcrypt.CompareHashAndPassword(compare, []byte(password)) != nil || hash == nil || status != "active" {
		return "", forke.ErrInvalidCredentials
	}
	return userID, nil
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := auth.EndSession(r.Context(), h.Pool, w, r, h.SecureCookies); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
