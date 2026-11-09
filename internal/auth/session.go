package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"forkemail/internal/db"
	"forkemail/internal/forke"

	"github.com/jackc/pgx/v5"
)

const (
	SessionCookie   = "fm_session"
	sessionLifetime = 14 * 24 * time.Hour
)

func StartSession(ctx context.Context, q db.Querier, w http.ResponseWriter, userID string, secure bool) error {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	expires := time.Now().Add(sessionLifetime)
	if _, err := q.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		HashKey(token), userID, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func EndSession(ctx context.Context, q db.Querier, w http.ResponseWriter, r *http.Request, secure bool) error {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return nil
	}
	_, err = q.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, HashKey(c.Value))
	return err
}

func (a *Authenticator) identifySession(ctx context.Context, token string) (Identity, error) {
	if len(token) != 64 {
		return Identity{}, errUnauthorized
	}
	var id Identity
	var status string
	var adminID *string
	err := a.Pool.QueryRow(ctx, `
		SELECT u.id, u.workspace_id, u.role, u.forke_admin_id::text, w.plan, w.status
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN workspaces w ON w.id = u.workspace_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, HashKey(token),
	).Scan(&id.UserID, &id.WorkspaceID, &id.Role, &adminID, &id.Plan, &status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "active") {
		return Identity{}, errUnauthorized
	}
	if err != nil {
		return Identity{}, err
	}
	if adminID != nil {
		if id.Role, err = a.syncAdminRole(ctx, id, *adminID); err != nil {
			return Identity{}, err
		}
	}
	id.Scope = id.Role
	return id, nil
}

func (a *Authenticator) syncAdminRole(ctx context.Context, id Identity, adminID string) (string, error) {
	if a.Admins == nil {
		return "", errUnauthorized
	}
	role, err := a.Admins.Role(ctx, adminID)
	if errors.Is(err, forke.ErrInactive) {
		if _, err := a.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id.UserID); err != nil {
			return "", err
		}
		return "", errUnauthorized
	}
	if err != nil {
		return "", err
	}
	if role != id.Role {
		if _, err := a.Pool.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, id.UserID, role); err != nil {
			return "", err
		}
	}
	return role, nil
}
