package feedback

import (
	"context"
	"log/slog"

	"forkemail/internal/events"

	"github.com/jackc/pgx/v5"
)

type Thresholds struct {
	MaxBounceRate    float64
	MaxComplaintRate float64
	MinSample        int
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
		return nil
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
	return events.Record(ctx, tx, events.Event{
		WorkspaceID: workspaceID, Type: "sending_paused",
		Detail: map[string]any{"reason": reason, "sent": s.Sent, "bounced": s.Bounced, "complained": s.Complained},
	})
}
