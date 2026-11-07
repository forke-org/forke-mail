package emails

import (
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"
)

func (h *Handler) Pause(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, `
		UPDATE emails SET status = 'paused', pause_reason = 'manual'
		WHERE id = $1 AND workspace_id = $2 AND status = 'sending'`)
}

func (h *Handler) Resume(w http.ResponseWriter, r *http.Request) {
	var paused bool
	ws := auth.FromContext(r.Context()).WorkspaceID
	if err := h.Pool.QueryRow(r.Context(), `SELECT sending_paused FROM workspaces WHERE id = $1`, ws).Scan(&paused); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if paused {
		writeError(w, r, errSendingPaused)
		return
	}
	h.transition(w, r, `
		UPDATE emails SET status = 'sending', pause_reason = NULL
		WHERE id = $1 AND workspace_id = $2 AND status = 'paused'`)
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request, sql string) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	tag, err := h.Pool.Exec(r.Context(), sql, id, ws)
	if err == nil && tag.RowsAffected() == 0 {
		err = h.missingOr(r, id, ws, errInvalidState)
	}
	if writeError(w, r, err) {
		return
	}
	h.Get(w, r)
}
