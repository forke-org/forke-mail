package feedback

import (
	"context"
	"log/slog"

	"forkemail/internal/events"

	"github.com/jackc/pgx/v5"
)

type Thresholds struct {
	MaxBounceRate     float64
	MaxComplaintRate  float64
	WarnBounceRate    float64
	WarnComplaintRate float64
	MinSample         int
}

type Stats struct {
	Sent       int
	Bounced    int
	Complained int
}

func (t Thresholds) Evaluate(s Stats) string {
	if s.Sent == 0 || s.Sent < t.MinSample {
		return ""
	}
	if float64(s.Bounced)/float64(s.Sent) > t.MaxBounceRate {
		return "bounce_rate_exceeded"
	}
	if float64(s.Complained)/float64(s.Sent) > t.MaxComplaintRate {
		return "complaint_rate_exceeded"
	}
	return ""
}

func (t Thresholds) Warn(s Stats) string {
	if s.Sent == 0 || s.Sent < t.MinSample {
		return ""
	}
	if t.WarnBounceRate > 0 && float64(s.Bounced)/float64(s.Sent) > t.WarnBounceRate {
		return "bounce_rate_high"
	}
	if t.WarnComplaintRate > 0 && float64(s.Complained)/float64(s.Sent) > t.WarnComplaintRate {
		return "complaint_rate_high"
	}
	return ""
}

func (s Stats) detail(reason string, t Thresholds) map[string]any {
	return map[string]any{
		"reason": reason, "sent": s.Sent, "bounced": s.Bounced, "complained": s.Complained,
		"bounce_rate": float64(s.Bounced) / float64(s.Sent), "complaint_rate": float64(s.Complained) / float64(s.Sent),
		"pause_bounce_rate": t.MaxBounceRate, "pause_complaint_rate": t.MaxComplaintRate, "window_days": 30,
	}
}

func (p *Processor) warnReputation(ctx context.Context, tx pgx.Tx, workspaceID string, s Stats) error {
	reason := p.Reputation.Warn(s)
	if reason == "" {
		return nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE workspaces SET reputation_warned_at = now()
		WHERE id = $1 AND NOT sending_paused AND (reputation_warned_at IS NULL OR reputation_warned_at < now() - interval '24 hours')`, workspaceID)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	detail := s.detail(reason, p.Reputation)
	slog.Warn("workspace reputation warning", "workspace_id", workspaceID, "reason", reason, "sent", s.Sent, "bounced", s.Bounced, "complained", s.Complained)
	if _, err := tx.Exec(ctx, `INSERT INTO workspace_alerts (workspace_id, kind, detail) VALUES ($1, 'reputation_warning', $2)`, workspaceID, detail); err != nil {
		return err
	}
	return events.Record(ctx, tx, events.Event{WorkspaceID: workspaceID, Type: "reputation_warning", Detail: detail})
}

func (p *Processor) checkReputation(ctx context.Context, tx pgx.Tx, workspaceID string) error {
	var s Stats
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(bounced_at), COUNT(complained_at) FROM (
			SELECT bounced_at, complained_at FROM email_recipients
			WHERE workspace_id = $1 AND sent_at > now() - interval '30 days'
			UNION ALL
			SELECT bounced_at, complained_at FROM transactional_messages
			WHERE workspace_id = $1 AND sent_at > now() - interval '30 days'
		) sent`, workspaceID).Scan(&s.Sent, &s.Bounced, &s.Complained); err != nil {
		return err
	}
	reason := p.Reputation.Evaluate(s)
	if reason == "" {
		return p.warnReputation(ctx, tx, workspaceID, s)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE workspaces SET sending_paused = true, pause_reason = $2
		WHERE id = $1 AND NOT sending_paused`, workspaceID, reason)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE emails SET status = 'paused', pause_reason = 'reputation'
		WHERE workspace_id = $1 AND status = 'sending'`, workspaceID); err != nil {
		return err
	}
	slog.Warn("workspace sending paused", "workspace_id", workspaceID, "reason", reason,
		"sent", s.Sent, "bounced", s.Bounced, "complained", s.Complained)
	detail := s.detail(reason, p.Reputation)
	if _, err := tx.Exec(ctx, `INSERT INTO workspace_alerts (workspace_id, kind, detail) VALUES ($1, 'sending_paused', $2)`, workspaceID, detail); err != nil {
		return err
	}
	return events.Record(ctx, tx, events.Event{WorkspaceID: workspaceID, Type: "sending_paused", Detail: detail})
}
