package seed

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/httpx"
	"forkemail/internal/ratelimit"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errAlreadySeeded = errors.New("already_seeded")

type Handler struct {
	Pool         *pgxpool.Pool
	Token        string
	Failures     ratelimit.FailureGuard
	SES          bool
	SESTestEmail string
}

type spec struct {
	workspace accounts.NewWorkspace
	domain    string
	owner     string
	email     string
	scope     string
	testEmail bool
}

var specs = []spec{
	{accounts.NewWorkspace{Name: "Forke", Plan: "paid", DailyLimit: 100000, RatePerSec: 14}, "forke.space", "Forke Owner", "owner@forke.space", auth.FullAccess, true},
	{accounts.NewWorkspace{Name: "Acme Trial", Plan: "trial", RatePerSec: 1, TrialSendLimit: 100}, "acme-trial.test", "Acme Owner", "owner@acme-trial.test", auth.SendingAccess, false},
	{accounts.NewWorkspace{Name: "Beta Trial", Plan: "trial", RatePerSec: 1, TrialSendLimit: 5}, "beta-trial.test", "Beta Owner", "owner@beta-trial.test", auth.SendingAccess, false},
}

type seededWorkspace struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Plan       string   `json:"plan"`
	Identities []string `json:"identities"`
	Email      string   `json:"login_email"`
	Password   string   `json:"login_password"`
	Scope      string   `json:"key_scope"`
	Key        string   `json:"api_key"`
}

func (h *Handler) Seed(w http.ResponseWriter, r *http.Request) {
	if h.Failures.Reject(w, r) {
		return
	}
	if !httpx.TokenMatches(r.Header.Get("X-Seed-Token"), h.Token) {
		h.Failures.Fail(r)
		httpx.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var out []seededWorkspace
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = h.seed(r.Context(), tx, r.URL.Query().Get("reset") == "true")
		return err
	})
	if errors.Is(err, errAlreadySeeded) {
		httpx.Error(w, http.StatusConflict, "already_seeded")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"warning":    "These passwords and API keys are shown once and cannot be retrieved again.",
		"workspaces": out,
	})
}

func (h *Handler) seed(ctx context.Context, tx pgx.Tx, reset bool) ([]seededWorkspace, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('forkemail_seed'))`); err != nil {
		return nil, err
	}
	if reset {
		if _, err := tx.Exec(ctx, `TRUNCATE workspaces CASCADE`); err != nil {
			return nil, err
		}
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspaces)`).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, errAlreadySeeded
	}
	var out []seededWorkspace
	for _, s := range specs {
		id, err := accounts.CreateWorkspace(ctx, tx, s.workspace)
		if err != nil {
			return nil, err
		}
		identities := h.identities(s)
		for _, name := range identities {
			kind := "domain"
			if strings.Contains(name, "@") {
				kind = "email"
			}
			if err := accounts.AddVerifiedIdentity(ctx, tx, id, kind, name); err != nil {
				return nil, err
			}
		}
		password := randomPassword()
		if _, err := accounts.CreateUser(ctx, tx, id, s.owner, s.email, password); err != nil {
			return nil, err
		}
		key := auth.GenerateKey()
		if _, err := tx.Exec(ctx, `
			INSERT INTO api_keys (workspace_id, name, scope, prefix, key_hash) VALUES ($1, 'Seed key', $2, $3, $4)`,
			id, s.scope, key.Prefix, key.Hash); err != nil {
			return nil, err
		}
		out = append(out, seededWorkspace{ID: id, Name: s.workspace.Name, Plan: s.workspace.Plan, Identities: identities,
			Email: s.email, Password: password, Scope: s.scope, Key: key.Plain})
	}
	return out, nil
}

func (h *Handler) identities(s spec) []string {
	switch {
	case !h.SES:
		return []string{s.domain}
	case h.SESTestEmail != "" && s.testEmail:
		return []string{h.SESTestEmail}
	}
	return []string{}
}

func randomPassword() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
