package emails

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

type messageLog struct {
	ID        int64           `json:"id"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if err := h.missingOr(r, id, ws, nil); writeError(w, r, err) {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := httpx.Page(r)
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, payload, created_at FROM message_logs
		WHERE email_id = $1 AND workspace_id = $2 AND id > $3
		ORDER BY id LIMIT $4`, id, ws, after, limit)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (messageLog, error) {
		var m messageLog
		return m, row.Scan(&m.ID, &m.Payload, &m.CreatedAt)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": list})
}
