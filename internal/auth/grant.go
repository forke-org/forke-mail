// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

const GrantPrefix = "fm_oauth_"

var ErrUnauthorized = errUnauthorized

func (a *Authenticator) IdentifyBearer(ctx context.Context, header string) (Identity, error) {
	plain, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return Identity{}, errUnauthorized
	}
	if strings.HasPrefix(plain, GrantPrefix) {
		return a.identifyGrant(ctx, plain)
	}
	return a.identifyKey(ctx, header)
}

func (a *Authenticator) identifyGrant(ctx context.Context, token string) (Identity, error) {
	if len(token) != len(GrantPrefix)+64 {
		return Identity{}, errUnauthorized
	}
	id := Identity{Delegated: true, KeyPrefix: "oauth"}
	var status, grantID string
	var adminID *string
	err := a.Pool.QueryRow(ctx, `
		SELECT g.id, u.id, w.id, m.role, u.forke_admin_id::text, w.plan, w.status
		FROM oauth_grants g
		JOIN users u ON u.id = g.user_id
		JOIN workspace_members m ON m.user_id = u.id AND m.workspace_id = g.workspace_id
		JOIN workspaces w ON w.id = g.workspace_id
		WHERE g.access_hash = $1 AND g.access_expires_at > now()`, HashKey(token),
	).Scan(&grantID, &id.UserID, &id.WorkspaceID, &id.Role, &adminID, &id.Plan, &status)
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
	_, err = a.Pool.Exec(ctx, `UPDATE oauth_grants SET last_used_at = now() WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, grantID)
	return id, err
}

func (a *Authenticator) IdentifySession(ctx context.Context, token string) (Identity, error) {
	return a.identifySession(ctx, token)
}
