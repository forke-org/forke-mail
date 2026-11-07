package emails

import (
	"net/http"
	"slices"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

var recipientStatuses = []string{"pending", "sending", "sent", "failed", "skipped"}

type recipientView struct {
	ID             string     `json:"id"`
	Email          string     `json:"email"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	LastError      *string    `json:"last_error"`
	SESMessageID   *string    `json:"ses_message_id"`
	SentAt         *time.Time `json:"sent_at"`
	DeliveredAt    *time.Time `json:"delivered_at"`
	BouncedAt      *time.Time `json:"bounced_at"`
	BounceType     *string    `json:"bounce_type"`
	ComplainedAt   *time.Time `json:"complained_at"`
	UnsubscribedAt *time.Time `json:"unsubscribed_at"`
}

func (h *Handler) Recipients(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if status != "" && !slices.Contains(recipientStatuses, status) {
		httpx.Error(w, http.StatusBadRequest, "invalid_status")
		return
	}
	if err := h.missingOr(r, id, ws, nil); writeError(w, r, err) {
		return
	}
	limit, offset := httpx.Page(r)
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, email, status, attempts, last_error, ses_message_id, sent_at,
			delivered_at, bounced_at, bounce_type, complained_at, unsubscribed_at
		FROM email_recipients
		WHERE email_id = $1 AND workspace_id = $2 AND ($3 = '' OR status = $3)
		ORDER BY email LIMIT $4 OFFSET $5`, id, ws, status, limit, offset)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (recipientView, error) {
		var v recipientView
		return v, row.Scan(&v.ID, &v.Email, &v.Status, &v.Attempts, &v.LastError, &v.SESMessageID, &v.SentAt,
			&v.DeliveredAt, &v.BouncedAt, &v.BounceType, &v.ComplainedAt, &v.UnsubscribedAt)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"recipients": list})
}
