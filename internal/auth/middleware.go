package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"forkemail/internal/httpx"
	"forkemail/internal/ratelimit"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errUnauthorized = errors.New("unauthorized")

type Authenticator struct {
	Pool     *pgxpool.Pool
	Failures ratelimit.FailureGuard
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.Failures.Reject(w, r) {
			return
		}
		id, err := a.identify(r)
		if errors.Is(err, errUnauthorized) {
			if r.Header.Get("Authorization") != "" {
				a.Failures.Fail(r)
			}
			httpx.Error(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err != nil {
			httpx.Internal(w, r, err)
			return
		}
		if id.Scope == Session && !httpx.SameOrigin(r) {
			httpx.Error(w, http.StatusForbidden, "cross_origin_request")
			return
		}
		httpx.SetLogFields(r.Context(), id.WorkspaceID, id.KeyPrefix)
		next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), id)))
	})
}

func (a *Authenticator) identify(r *http.Request) (Identity, error) {
	if header := r.Header.Get("Authorization"); header != "" {
		return a.identifyKey(r.Context(), header)
	}
	if c, err := r.Cookie(SessionCookie); err == nil {
		return a.identifySession(r.Context(), c.Value)
	}
	return Identity{}, errUnauthorized
}

func (a *Authenticator) identifyKey(ctx context.Context, header string) (Identity, error) {
	plain, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || !validKeyFormat(plain) {
		return Identity{}, errUnauthorized
	}
	var id Identity
	var status string
	var lastUsed *time.Time
	err := a.Pool.QueryRow(ctx, `
		SELECT k.id, k.prefix, k.scope, k.last_used_at, w.id, w.plan, w.status
		FROM api_keys k
		JOIN workspaces w ON w.id = k.workspace_id
		WHERE k.key_hash = $1 AND k.revoked_at IS NULL`, HashKey(plain),
	).Scan(&id.KeyID, &id.KeyPrefix, &id.Scope, &lastUsed, &id.WorkspaceID, &id.Plan, &status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "active") {
		return Identity{}, errUnauthorized
	}
	if err != nil {
		return Identity{}, err
	}
	if lastUsed == nil || time.Since(*lastUsed) > time.Minute {
		_, err = a.Pool.Exec(ctx, `
			UPDATE api_keys SET last_used_at = now()
			WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, id.KeyID)
		if err != nil {
			return Identity{}, err
		}
	}
	return id, nil
}
