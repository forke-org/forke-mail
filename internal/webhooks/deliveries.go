package webhooks

import (
	"forkemail/internal/auth"
	"forkemail/internal/httpx"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type DeliveryView struct {
	ID            string     `json:"id"`
	EventID       string     `json:"event_id"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	ResponseCode  *int       `json:"response_code"`
	LastError     *string    `json:"last_error"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	CreatedAt     time.Time  `json:"created_at"`
	FinishedAt    *time.Time `json:"finished_at"`
}

func (h *Handler) Deliveries(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ws := auth.FromContext(r.Context()).WorkspaceID
	var exists bool
	if err := h.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM webhook_endpoints WHERE id=$1 AND workspace_id=$2)`, id, ws).Scan(&exists); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if !exists {
		httpx.Error(w, 404, "not_found")
		return
	}
	limit, offset := httpx.Page(r)
	rows, err := h.Pool.Query(r.Context(), `SELECT id,event_id::text,status,attempts,response_code,last_error,next_attempt_at,created_at,finished_at FROM webhook_deliveries WHERE workspace_id=$1 AND endpoint_id=$2 ORDER BY created_at DESC,id LIMIT $3 OFFSET $4`, ws, id, limit, offset)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (DeliveryView, error) {
		var d DeliveryView
		err := row.Scan(&d.ID, &d.EventID, &d.Status, &d.Attempts, &d.ResponseCode, &d.LastError, &d.NextAttemptAt, &d.CreatedAt, &d.FinishedAt)
		return d, err
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if items == nil {
		items = []DeliveryView{}
	}
	httpx.JSON(w, 200, map[string]any{"deliveries": items})
}
