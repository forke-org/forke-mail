package accounts

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/contacts"
	"forkemail/internal/httpx"
	"forkemail/internal/ratelimit"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var errEmailTaken = errors.New("email_taken")

type Handler struct {
	Pool           *pgxpool.Pool
	SecureCookies  bool
	SandboxDomains bool
	Failures       ratelimit.FailureGuard
}

func (h *Handler) Signup(w http.ResponseWriter, r *http.Request) {
	if !httpx.SameOrigin(r) {
		httpx.Error(w, http.StatusForbidden, "cross_origin_request")
		return
	}
	var body struct {
		Name      string `json:"name"`
		Email     string `json:"email"`
		Password  string `json:"password"`
		Workspace string `json:"workspace"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Email = contacts.NormalizeEmail(body.Email)
	body.Workspace = strings.TrimSpace(body.Workspace)
	if body.Workspace == "" {
		body.Workspace = body.Name
	}
	switch {
	case body.Name == "" || utf8.RuneCountInString(body.Name) > 100:
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_name")
		return
	case !contacts.ValidEmail(body.Email):
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_email")
		return
	case !validPassword(body.Password):
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_password")
		return
	case utf8.RuneCountInString(body.Workspace) > 200:
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_workspace")
		return
	}
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		wsID, err := CreateWorkspace(r.Context(), tx, NewWorkspace{Name: body.Workspace, Plan: "trial", RatePerSec: 1, TrialSendLimit: 100})
		if err != nil {
			return err
		}
		if h.SandboxDomains {
			if err := AddVerifiedIdentity(r.Context(), tx, wsID, "domain", SandboxDomain(wsID)); err != nil {
				return err
			}
		}
		userID, err := CreateUser(r.Context(), tx, wsID, body.Name, body.Email, body.Password)
		if err != nil {
			return err
		}
		return auth.StartSession(r.Context(), tx, w, userID, h.SecureCookies)
	})
	if errors.Is(err, errEmailTaken) {
		httpx.Error(w, http.StatusConflict, "email_taken")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]bool{"ok": true})
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
	var userID, hash, status string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT u.id, u.password_hash, w.status FROM users u JOIN workspaces w ON w.id = u.workspace_id
		WHERE lower(u.email) = $1`, contacts.NormalizeEmail(body.Email)).Scan(&userID, &hash, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		hash = string(dummyHash)
	} else if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	match := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)) == nil
	if !match || userID == "" || status != "active" {
		h.Failures.Fail(r)
		httpx.Error(w, http.StatusUnauthorized, "invalid_credentials")
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

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if err := auth.EndSession(r.Context(), h.Pool, w, r, h.SecureCookies); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
