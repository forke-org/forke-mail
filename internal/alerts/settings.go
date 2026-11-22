// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package alerts

import (
	"net/http"
	"strings"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Settings struct {
	Emails   bool   `json:"emails"`
	SlackURL string `json:"slack_url"`
}

type Recent struct {
	Kind      string         `json:"kind"`
	Detail    map[string]any `json:"detail"`
	CreatedAt time.Time      `json:"created_at"`
	Subject   string         `json:"subject"`
}

type Handler struct {
	Pool              *pgxpool.Pool
	WarnBounceRate    float64
	WarnComplaintRate float64
	MaxBounceRate     float64
	MaxComplaintRate  float64
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var s Settings
	var name string
	if err := h.Pool.QueryRow(r.Context(), `SELECT alert_emails, alert_slack_url, name FROM workspaces WHERE id = $1`, ws).Scan(&s.Emails, &s.SlackURL, &name); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT kind, detail, created_at FROM workspace_alerts WHERE workspace_id = $1 ORDER BY id DESC LIMIT 10`, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	recent, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Recent, error) {
		var a Recent
		err := row.Scan(&a.Kind, &a.Detail, &a.CreatedAt)
		a.Subject, _ = Message(Alert{Kind: a.Kind, Detail: a.Detail, Workspace: name})
		return a, err
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"emails": s.Emails, "slack_url": s.SlackURL, "recent": recent,
		"thresholds": map[string]float64{
			"warn_bounce_rate": h.WarnBounceRate, "warn_complaint_rate": h.WarnComplaintRate,
			"pause_bounce_rate": h.MaxBounceRate, "pause_complaint_rate": h.MaxComplaintRate,
		},
	})
}

func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var s Settings
	if !httpx.Decode(w, r, &s) {
		return
	}
	s.SlackURL = strings.TrimSpace(s.SlackURL)
	if s.SlackURL != "" && (!strings.HasPrefix(s.SlackURL, "https://hooks.slack.com/") || len(s.SlackURL) > 500) {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_slack_url")
		return
	}
	if _, err := h.Pool.Exec(r.Context(), `UPDATE workspaces SET alert_emails = $2, alert_slack_url = $3 WHERE id = $1`, ws, s.Emails, s.SlackURL); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	h.Get(w, r)
}
