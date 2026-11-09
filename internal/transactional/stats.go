package transactional

import (
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"
)

type latency struct {
	P50 *float64 `json:"p50_ms"`
	P95 *float64 `json:"p95_ms"`
	P99 *float64 `json:"p99_ms"`
	Max *float64 `json:"max_ms"`
}

type stats struct {
	WindowHours int     `json:"window_hours"`
	Total       int     `json:"total"`
	Queued      int     `json:"queued"`
	Sending     int     `json:"sending"`
	Sent        int     `json:"sent"`
	Failed      int     `json:"failed"`
	Delivered   int     `json:"delivered"`
	Bounced     int     `json:"bounced"`
	Complained  int     `json:"complained"`
	Opened      int     `json:"opened"`
	Clicked     int     `json:"clicked"`
	Tracked     int     `json:"tracked_opens"`
	TrackedLink int     `json:"tracked_clicks"`
	Dispatch    latency `json:"dispatch"`
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	s := stats{WindowHours: 24}
	d := &s.Dispatch
	err := h.Pool.QueryRow(r.Context(), `
		WITH recent AS (
			SELECT * FROM transactional_messages WHERE workspace_id = $1 AND created_at > now() - interval '24 hours'
		), latency AS (
			SELECT EXTRACT(EPOCH FROM (sent_at - created_at)) * 1000 AS ms FROM recent WHERE sent_at IS NOT NULL
		)
		SELECT COUNT(*),
			COUNT(*) FILTER (WHERE status = 'queued'), COUNT(*) FILTER (WHERE status = 'sending'),
			COUNT(*) FILTER (WHERE status = 'sent'), COUNT(*) FILTER (WHERE status = 'failed'),
			COUNT(delivered_at), COUNT(bounced_at), COUNT(complained_at), COUNT(opened_at), COUNT(clicked_at),
			COUNT(*) FILTER (WHERE track_opens AND status = 'sent'), COUNT(*) FILTER (WHERE track_clicks AND status = 'sent'),
			(SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY ms) FROM latency),
			(SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY ms) FROM latency),
			(SELECT percentile_cont(0.99) WITHIN GROUP (ORDER BY ms) FROM latency),
			(SELECT MAX(ms)::float8 FROM latency)
		FROM recent`, ws).Scan(&s.Total, &s.Queued, &s.Sending, &s.Sent, &s.Failed,
		&s.Delivered, &s.Bounced, &s.Complained, &s.Opened, &s.Clicked, &s.Tracked, &s.TrackedLink,
		&d.P50, &d.P95, &d.P99, &d.Max)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}
