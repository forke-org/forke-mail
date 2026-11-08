package events

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var types = []string{"queued", "sent", "delivered", "bounced", "soft_bounced", "complained", "unsubscribed", "failed", "rejected", "delayed", "sending_paused", "opened", "clicked"}

type Handler struct {
	Pool        *pgxpool.Pool
	SendingMode func() string
}

type eventView struct {
	ID           int64           `json:"id"`
	EmailID      *string         `json:"email_id"`
	MessageID    *string         `json:"message_id"`
	Subject      *string         `json:"subject"`
	CampaignName *string         `json:"campaign_name"`
	Recipient    *string         `json:"recipient"`
	Type         string          `json:"type"`
	Detail       json.RawMessage `json:"detail"`
	CreatedAt    time.Time       `json:"created_at"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	q := r.URL.Query()
	eventType := q.Get("type")
	if eventType != "" && !slices.Contains(types, eventType) {
		httpx.Error(w, http.StatusBadRequest, "invalid_type")
		return
	}
	limit, offset := httpx.Page(r)
	rows, err := h.Pool.Query(r.Context(), `
		SELECT ev.id, ev.email_id, ev.message_id, COALESCE(e.subject, tm.subject), c.name,
			COALESCE(er.email, tm.to_address, ev.detail->>'email'), ev.type, ev.detail, ev.created_at
		FROM email_events ev
		LEFT JOIN email_recipients er ON er.id = ev.recipient_id AND er.workspace_id = ev.workspace_id
		LEFT JOIN emails e ON e.id = ev.email_id AND e.workspace_id = ev.workspace_id
		LEFT JOIN campaigns c ON c.id = e.campaign_id
		LEFT JOIN transactional_messages tm ON tm.id = ev.message_id AND tm.workspace_id = ev.workspace_id
		WHERE ev.workspace_id = $1 AND ($2 = '' OR ev.type = $2)
		ORDER BY ev.id DESC
		LIMIT $3 OFFSET $4`, ws, eventType, limit, offset)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (eventView, error) {
		var e eventView
		return e, row.Scan(&e.ID, &e.EmailID, &e.MessageID, &e.Subject, &e.CampaignName, &e.Recipient, &e.Type, &e.Detail, &e.CreatedAt)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"events": list})
}
