package keys

import (
	"errors"
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

func (h *Handler) Rotate(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !httpx.Decode(w, r, &body) || !h.confirmPassword(w, r, body.Password) {
		return
	}
	var created createdKey
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		scope, err := lockActiveKey(r.Context(), tx, ws, id)
		if err != nil {
			return err
		}
		var name string
		if err := tx.QueryRow(r.Context(), `SELECT name FROM api_keys WHERE id = $1`, id).Scan(&name); err != nil {
			return err
		}
		if created, err = insertKey(r.Context(), tx, ws, name, scope); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND workspace_id = $2`, id, ws)
		return err
	})
	switch {
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		httpx.JSON(w, http.StatusCreated, created)
	}
}
