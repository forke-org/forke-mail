package workspaces

import (
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"
	"forkemail/internal/ratelimit"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Pool           *pgxpool.Pool
	APILimit       func(plan string) ratelimit.Rule
	GlobalSendRate func() float64
	TrialSendRate  float64
}

type meResponse struct {
	User       *meUser      `json:"user"`
	Workspace  meWorkspace  `json:"workspace"`
	Key        *meKey       `json:"key"`
	Domains    []string     `json:"domains"`
	Quota      meQuota      `json:"quota"`
	RateLimits meRateLimits `json:"rate_limits"`
}

type meWorkspace struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Plan          string  `json:"plan"`
	Status        string  `json:"status"`
	SendingPaused bool    `json:"sending_paused"`
	PauseReason   *string `json:"pause_reason"`
}

type meUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type meKey struct {
	Scope  string `json:"scope"`
	Prefix string `json:"prefix"`
}

type meQuota struct {
	TrialUsed  int `json:"trial_used"`
	TrialLimit int `json:"trial_limit"`
	DailyLimit int `json:"daily_limit"`
	SentToday  int `json:"sent_today"`
}

type meRateLimits struct {
	APIPerSecond  float64 `json:"api_per_second"`
	APIBurst      int     `json:"api_burst"`
	SendPerSecond float64 `json:"send_per_second"`
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	resp := meResponse{}
	if id.UserID != "" {
		resp.User = &meUser{}
		if err := h.Pool.QueryRow(r.Context(), `SELECT name, email FROM users WHERE id = $1`, id.UserID).Scan(&resp.User.Name, &resp.User.Email); err != nil {
			httpx.Internal(w, r, err)
			return
		}
	} else {
		resp.Key = &meKey{Scope: id.Scope, Prefix: id.KeyPrefix}
	}
	var configuredRate int
	err := h.Pool.QueryRow(r.Context(), `
		SELECT w.id, w.name, w.plan, w.status, w.sending_paused, w.pause_reason, w.trial_sent_count, w.trial_send_limit, w.daily_limit, w.rate_limit_per_sec,
			COALESCE((SELECT sent_count FROM usage_daily u WHERE u.workspace_id = w.id AND u.date = current_date), 0)
		FROM workspaces w WHERE w.id = $1`, id.WorkspaceID,
	).Scan(&resp.Workspace.ID, &resp.Workspace.Name, &resp.Workspace.Plan, &resp.Workspace.Status,
		&resp.Workspace.SendingPaused, &resp.Workspace.PauseReason,
		&resp.Quota.TrialUsed, &resp.Quota.TrialLimit, &resp.Quota.DailyLimit, &configuredRate, &resp.Quota.SentToday)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT name FROM domains WHERE workspace_id = $1 AND status = 'verified' ORDER BY name`, id.WorkspaceID)
	if err == nil {
		resp.Domains, err = pgx.CollectRows(rows, pgx.RowTo[string])
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rule := h.APILimit(resp.Workspace.Plan)
	resp.RateLimits = meRateLimits{
		APIPerSecond:  rule.PerSecond,
		APIBurst:      rule.Burst,
		SendPerSecond: SendRate(resp.Workspace.Plan, configuredRate, h.GlobalSendRate(), h.TrialSendRate),
	}
	httpx.JSON(w, http.StatusOK, resp)
}
