package keys

import (
	"context"
	"errors"
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		scope, err := lockActiveKey(r.Context(), tx, ws, id)
		if err != nil {
			return err
		}
		if scope == auth.FullAccess {
			var others int
			if err := tx.QueryRow(r.Context(), `
				SELECT COUNT(*) FROM api_keys
				WHERE workspace_id = $1 AND scope = 'full_access' AND revoked_at IS NULL AND id <> $2`, ws, id).Scan(&others); err != nil {
				return err
			}
			if others == 0 {
				return errLastFullAccess
			}
		}
		_, err = tx.Exec(r.Context(), `UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND workspace_id = $2`, id, ws)
		return err
	})
	switch {
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.Is(err, errLastFullAccess):
		httpx.Error(w, http.StatusConflict, "last_full_access_key")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func lockActiveKey(ctx context.Context, tx pgx.Tx, ws, id string) (string, error) {
	if _, err := tx.Exec(ctx, `SELECT 1 FROM workspaces WHERE id = $1 FOR UPDATE`, ws); err != nil {
		return "", err
	}
	var scope string
	err := tx.QueryRow(ctx, `
		SELECT scope FROM api_keys
		WHERE id = $1 AND workspace_id = $2 AND revoked_at IS NULL`, id, ws).Scan(&scope)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errNotFound
	}
	return scope, err
}
