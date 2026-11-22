// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

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
		SELECT u.id, w.id, m.role, u.forke_admin_id::text, w.plan, w.status
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN workspace_members m ON m.user_id = u.id AND m.workspace_id = COALESCE(s.workspace_id, u.workspace_id)
		JOIN workspaces w ON w.id = m.workspace_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, HashKey(token),
	).Scan(&id.UserID, &id.WorkspaceID, &id.Role, &adminID, &id.Plan, &status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != "active") {
		return Identity{}, errUnauthorized
	}
	if err != nil {
		return Identity{}, err
	}
	if adminID != nil {
		if id.Role, err = a.syncAdminRole(ctx, id, *adminID, id.Role); err != nil {
			return Identity{}, err
		}
	}
	id.Scope = id.Role
	return id, nil
}

func (a *Authenticator) syncAdminRole(ctx context.Context, id Identity, adminID, current string) (string, error) {
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
	var platform bool
	if err := a.Pool.QueryRow(ctx, `SELECT platform FROM workspaces WHERE id = $1`, id.WorkspaceID).Scan(&platform); err != nil {
		return "", err
	}
	if !platform {
		return current, nil
	}
	if role != current {
		if _, err := a.Pool.Exec(ctx, `UPDATE workspace_members SET role = $3 WHERE user_id = $1 AND workspace_id = $2`, id.UserID, id.WorkspaceID, role); err != nil {
			return "", err
		}
	}
	return role, nil
}
