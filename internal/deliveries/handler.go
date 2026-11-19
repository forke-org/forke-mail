package deliveries

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var statuses = []string{"problems", "pending", "sent", "delivered", "bounced", "complained", "failed", "skipped"}
var kinds = []string{"broadcast", "transactional"}

type Handler struct {
	Pool *pgxpool.Pool
}

type Delivery struct {
	Kind        string     `json:"kind"`
	ID          string     `json:"id"`
	EmailID     *string    `json:"email_id"`
	To          string     `json:"to"`
	Subject     string     `json:"subject"`
	Status      string     `json:"status"`
	Reason      *string    `json:"reason"`
	At          time.Time  `json:"at"`
	DeliveredAt *time.Time `json:"delivered_at"`
}

const query = `
	WITH d AS (
		SELECT 'broadcast' AS kind, er.id, er.email_id, er.email AS to_address, em.subject,
			CASE WHEN er.complained_at IS NOT NULL THEN 'complained' WHEN er.bounced_at IS NOT NULL THEN 'bounced'
				WHEN er.delivered_at IS NOT NULL THEN 'delivered' WHEN er.status IN ('failed', 'skipped') THEN er.status
				WHEN er.status = 'sent' THEN 'sent' ELSE 'pending' END AS status,
			CASE WHEN er.bounced_at IS NOT NULL THEN 'bounce: ' || COALESCE(er.bounce_type, 'permanent') ELSE er.last_error END AS reason,
			COALESCE(er.sent_at, em.started_at, em.created_at) AS at, er.delivered_at
		FROM email_recipients er JOIN emails em ON em.id = er.email_id
		WHERE er.workspace_id = $1 AND $3 <> 'transactional'
			AND ($2 = '' OR lower(er.email) LIKE $2 OR lower(em.subject) LIKE $2)
		UNION ALL
		SELECT 'transactional', tm.id, NULL, tm.to_address, tm.subject,
			CASE WHEN tm.complained_at IS NOT NULL THEN 'complained' WHEN tm.bounced_at IS NOT NULL THEN 'bounced'
				WHEN tm.delivered_at IS NOT NULL THEN 'delivered' WHEN tm.status = 'failed' THEN 'failed'
				WHEN tm.status = 'sent' THEN 'sent' ELSE 'pending' END,
			CASE WHEN tm.bounced_at IS NOT NULL THEN 'bounce: ' || COALESCE(tm.bounce_type, 'permanent') ELSE tm.last_error END,
			COALESCE(tm.sent_at, tm.created_at), tm.delivered_at
		FROM transactional_messages tm
		WHERE tm.workspace_id = $1 AND $3 <> 'broadcast'
			AND ($2 = '' OR lower(tm.to_address) LIKE $2 OR lower(tm.subject) LIKE $2)
	)
	SELECT kind, id, email_id, to_address, subject, status, reason, at, delivered_at, COUNT(*) OVER ()
	FROM d
	WHERE $4 = '' OR status = $4 OR ($4 = 'problems' AND status IN ('bounced', 'complained', 'failed'))
	ORDER BY at DESC, id
	LIMIT $5 OFFSET $6`

func pattern(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return ""
	}
	q = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q)
	return "%" + q + "%"
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	q := r.URL.Query()
	status, kind := q.Get("status"), q.Get("kind")
	if status != "" && !slices.Contains(statuses, status) {
		httpx.Error(w, http.StatusBadRequest, "invalid_status")
		return
	}
	if kind != "" && !slices.Contains(kinds, kind) {
		httpx.Error(w, http.StatusBadRequest, "invalid_kind")
		return
	}
	search := q.Get("q")
	if len(search) > 254 {
		httpx.Error(w, http.StatusBadRequest, "invalid_query")
		return
	}
	limit, offset := httpx.Page(r)
	rows, err := h.Pool.Query(r.Context(), query, ws, pattern(search), kind, status, limit, offset)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	total := 0
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Delivery, error) {
		var d Delivery
		return d, row.Scan(&d.Kind, &d.ID, &d.EmailID, &d.To, &d.Subject, &d.Status, &d.Reason, &d.At, &d.DeliveredAt, &total)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"deliveries": list, "total": total})
}
