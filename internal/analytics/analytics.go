// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package analytics

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var periods = []int{1, 7, 30, 90}
var streams = []string{"all", "transactional", "broadcast"}

type Handler struct {
	Pool *pgxpool.Pool
}

type Totals struct {
	Sent         int `json:"sent"`
	Delivered    int `json:"delivered"`
	Opened       int `json:"opened"`
	Clicked      int `json:"clicked"`
	Bounced      int `json:"bounced"`
	Complained   int `json:"complained"`
	Unsubscribed int `json:"unsubscribed"`
}

type Day struct {
	Date          string `json:"date"`
	Sent          int    `json:"sent"`
	Delivered     int    `json:"delivered"`
	Opened        int    `json:"opened"`
	Bounced       int    `json:"bounced"`
	Broadcast     int    `json:"broadcast"`
	Transactional int    `json:"transactional"`
}

type Stream struct {
	Stream string `json:"stream"`
	Totals
}

const base = `
	WITH m AS (
		SELECT 'broadcast' AS stream, sent_at, delivered_at, opened_at, clicked_at, bounced_at, complained_at, unsubscribed_at
		FROM email_recipients WHERE workspace_id = $1 AND sent_at >= $2 AND sent_at < $3 AND $4 <> 'transactional'
		UNION ALL
		SELECT 'transactional', sent_at, delivered_at, opened_at, clicked_at, bounced_at, complained_at, NULL
		FROM transactional_messages WHERE workspace_id = $1 AND sent_at >= $2 AND sent_at < $3 AND $4 <> 'broadcast'
	)`

const counts = `COUNT(*)::int, COUNT(delivered_at)::int, COUNT(opened_at)::int, COUNT(clicked_at)::int, COUNT(bounced_at)::int, COUNT(complained_at)::int, COUNT(unsubscribed_at)::int`

func (h *Handler) totals(r *http.Request, ws string, from, to time.Time, stream string) (Totals, error) {
	var t Totals
	err := h.Pool.QueryRow(r.Context(), base+` SELECT `+counts+` FROM m`, ws, from, to, stream).
		Scan(&t.Sent, &t.Delivered, &t.Opened, &t.Clicked, &t.Bounced, &t.Complained, &t.Unsubscribed)
	return t, err
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	days, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil {
		days = 30
	}
	stream := r.URL.Query().Get("stream")
	if stream == "" {
		stream = "all"
	}
	if !slices.Contains(periods, days) || !slices.Contains(streams, stream) {
		httpx.Error(w, http.StatusBadRequest, "invalid_period")
		return
	}
	now := time.Now().UTC()
	to := now
	from := now.Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))
	prevFrom := from.AddDate(0, 0, -days)

	current, err := h.totals(r, ws, from, to, stream)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	previous, err := h.totals(r, ws, prevFrom, from, stream)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err := h.Pool.Query(r.Context(), base+`
		SELECT to_char(d, 'YYYY-MM-DD'), COUNT(m.sent_at)::int, COUNT(m.delivered_at)::int, COUNT(m.opened_at)::int, COUNT(m.bounced_at)::int,
			COUNT(m.sent_at) FILTER (WHERE m.stream = 'broadcast')::int, COUNT(m.sent_at) FILTER (WHERE m.stream = 'transactional')::int
		FROM generate_series($2::timestamptz, $3::timestamptz - interval '1 second', interval '1 day') d
		LEFT JOIN m ON m.sent_at >= d AND m.sent_at < d + interval '1 day'
		GROUP BY d ORDER BY d`, ws, from, to, stream)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	daily, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Day])
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err = h.Pool.Query(r.Context(), base+` SELECT stream, `+counts+` FROM m GROUP BY stream ORDER BY stream DESC`, ws, from, to, "all")
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	byStream, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Stream, error) {
		var s Stream
		return s, row.Scan(&s.Stream, &s.Sent, &s.Delivered, &s.Opened, &s.Clicked, &s.Bounced, &s.Complained, &s.Unsubscribed)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"days": days, "stream": stream, "from": from, "to": to,
		"current": current, "previous": previous, "daily": daily, "streams": byStream,
	})
}
