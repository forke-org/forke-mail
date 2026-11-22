package events

import (
	"context"
	"time"
)

type Component struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	LagSeconds *float64 `json:"lag_seconds"`
}

type threshold struct {
	degraded time.Duration
	down     time.Duration
}

func (t threshold) judge(lag *float64) string {
	switch {
	case lag == nil || *lag < t.degraded.Seconds():
		return "operational"
	case *lag < t.down.Seconds():
		return "degraded"
	}
	return "down"
}

const componentQuery = `
	SELECT
		(SELECT EXTRACT(EPOCH FROM now() - MIN(LEAST(next_attempt_at, COALESCE(claimed_at, next_attempt_at))))
			FROM transactional_messages WHERE status IN ('queued', 'sending') AND next_attempt_at <= now()),
		(SELECT CASE WHEN MIN(r.next_attempt_at) IS NOT NULL THEN EXTRACT(EPOCH FROM now() - GREATEST(MIN(r.next_attempt_at),
				(SELECT MAX(sent_at) FROM email_recipients WHERE sent_at >= now() - interval '1 day'))) END
			FROM email_recipients r JOIN emails e ON e.id = r.email_id JOIN workspaces w ON w.id = e.workspace_id
			WHERE r.status = 'pending' AND r.next_attempt_at <= now() AND e.status = 'sending'
				AND w.status = 'active' AND NOT w.sending_paused AND NOT workspace_held(w.id)),
		(SELECT EXTRACT(EPOCH FROM now() - MIN(next_run_at)) FROM workflow_runs WHERE status = 'running' AND next_run_at <= now()),
		(SELECT EXTRACT(EPOCH FROM now() - MIN(next_attempt_at)) FROM webhook_deliveries WHERE status = 'pending' AND next_attempt_at <= now()),
		(SELECT CASE WHEN expected >= 5 THEN EXTRACT(EPOCH FROM now() - COALESCE(
				(SELECT MAX(received_at) FROM processed_sns_messages), now() - interval '1 day')) END
			FROM (SELECT (SELECT COUNT(*) FROM email_recipients WHERE sent_at BETWEEN now() - interval '1 hour' AND now() - interval '10 minutes')
				+ (SELECT COUNT(*) FROM transactional_messages WHERE sent_at BETWEEN now() - interval '1 hour' AND now() - interval '10 minutes') AS expected) s)`

var componentThresholds = []struct {
	id string
	threshold
}{
	{"transactional", threshold{2 * time.Minute, 10 * time.Minute}},
	{"broadcasts", threshold{5 * time.Minute, 20 * time.Minute}},
	{"workflows", threshold{5 * time.Minute, 30 * time.Minute}},
	{"webhooks", threshold{5 * time.Minute, 30 * time.Minute}},
	{"email_events", threshold{15 * time.Minute, 45 * time.Minute}},
}

func (p *PublicStatus) components(ctx context.Context) ([]Component, error) {
	lags := make([]*float64, len(componentThresholds))
	dest := make([]any, len(lags))
	for i := range lags {
		dest[i] = &lags[i]
	}
	if err := p.Pool.QueryRow(ctx, componentQuery).Scan(dest...); err != nil {
		return nil, err
	}
	out := make([]Component, 0, len(componentThresholds)+1)
	for i, c := range componentThresholds {
		out = append(out, Component{ID: c.id, Status: c.judge(lags[i]), LagSeconds: lags[i]})
	}
	smtp := "not_enabled"
	if p.SMTPEnabled {
		smtp = "operational"
	}
	return append(out, Component{ID: "smtp", Status: smtp}), nil
}

func worst(list []Component) string {
	rank := map[string]int{"degraded": 1, "down": 2}
	status := "operational"
	for _, c := range list {
		if rank[c.Status] > rank[status] {
			status = c.Status
		}
	}
	return status
}
