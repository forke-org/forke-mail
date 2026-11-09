package lifecycle

import (
	"context"
	"log/slog"
	"time"

	"forkemail/internal/contacts"
	"forkemail/internal/emails"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	runEvery        = time.Hour
	tombstoneAfter  = 30 * 24 * time.Hour
	maxInvitesBatch = 10000
	campaignName    = "Re-engagement"
)

type Runner struct {
	Pool *pgxpool.Pool
}

type Report struct {
	Tombstoned int64
	Expired    int
	Invited    int
}

func (r Runner) Run(ctx context.Context) {
	t := time.NewTicker(runEvery)
	defer t.Stop()
	for {
		rep, err := r.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Error("contact lifecycle failed", "err", err)
		} else if rep != (Report{}) {
			slog.Info("contact lifecycle", "tombstoned", rep.Tombstoned, "expired", rep.Expired, "invited", rep.Invited)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (r Runner) RunOnce(ctx context.Context) (Report, error) {
	var rep Report
	err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtext('forke-mail-contact-lifecycle'))`).Scan(&locked); err != nil || !locked {
			return err
		}
		var err error
		if rep.Tombstoned, err = tombstone(ctx, tx); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT workspace_id, from_name, from_address, reply_to, subject, html, inactive_days, grace_days, min_broadcasts
			FROM reengagement_settings WHERE enabled`)
		if err != nil {
			return err
		}
		type workspaceSettings struct {
			id string
			Settings
		}
		list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (workspaceSettings, error) {
			var s workspaceSettings
			return s, row.Scan(&s.id, &s.FromName, &s.FromAddress, &s.ReplyTo, &s.Subject, &s.HTML, &s.InactiveDays, &s.GraceDays, &s.MinBroadcasts)
		})
		if err != nil {
			return err
		}
		for _, ws := range list {
			n, err := expire(ctx, tx, ws.id, ws.GraceDays)
			if err != nil {
				return err
			}
			rep.Expired += n
			if n, err = invite(ctx, tx, ws.id, ws.Settings); err != nil {
				return err
			}
			rep.Invited += n
		}
		return nil
	})
	return rep, err
}

func tombstone(ctx context.Context, tx pgx.Tx) (int64, error) {
	tag, err := tx.Exec(ctx, `
		DELETE FROM contacts c
		WHERE c.status <> 'active' AND c.status_changed_at < now() - make_interval(secs => $1)
			AND EXISTS (SELECT 1 FROM suppressions s WHERE s.workspace_id = c.workspace_id AND lower(s.email) = lower(c.email))`,
		tombstoneAfter.Seconds())
	return tag.RowsAffected(), err
}

func expire(ctx context.Context, tx pgx.Tx, ws string, graceDays int) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT email FROM contacts
		WHERE workspace_id = $1 AND status = 'active' AND reengage_sent_at < now() - make_interval(days => $2)
			AND (last_engaged_at IS NULL OR last_engaged_at < reengage_sent_at)`, ws, graceDays)
	if err != nil {
		return 0, err
	}
	addresses, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	for _, email := range addresses {
		if _, err := contacts.Suppress(ctx, tx, contacts.Suppression{
			WorkspaceID: ws, Email: email, Reason: "unsubscribed", Source: "dormancy",
			Detail: map[string]any{"grace_days": graceDays},
		}); err != nil {
			return 0, err
		}
	}
	return len(addresses), nil
}

func invite(ctx context.Context, tx pgx.Tx, ws string, s Settings) (int, error) {
	rows, err := tx.Query(ctx, `
		WITH since_engaged AS (
			SELECT r.contact_id, count(*) AS sends, min(r.sent_at) AS first_sent
			FROM email_recipients r
			JOIN emails e ON e.id = r.email_id AND e.track_clicks
			JOIN contacts c ON c.id = r.contact_id
			WHERE r.workspace_id = $1 AND r.status = 'sent' AND r.sent_at > COALESCE(c.last_engaged_at, '-infinity')
			GROUP BY r.contact_id
		)
		SELECT c.id FROM contacts c
		JOIN since_engaged x ON x.contact_id = c.id
		WHERE c.workspace_id = $1 AND c.status = 'active' AND c.reengage_sent_at IS NULL
			AND (c.paused_until IS NULL OR c.paused_until <= now())
			AND NOT EXISTS (SELECT 1 FROM suppressions s WHERE s.workspace_id = c.workspace_id AND lower(s.email) = lower(c.email))
			AND x.sends >= $2 AND x.first_sent < now() - make_interval(days => $3)
		ORDER BY x.first_sent LIMIT $4`, ws, s.MinBroadcasts, s.InactiveDays, maxInvitesBatch)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	var campaign string
	if err := tx.QueryRow(ctx, `
		WITH created AS (
			INSERT INTO campaigns (workspace_id, name) VALUES ($1, $2)
			ON CONFLICT (workspace_id, lower(name)) DO NOTHING RETURNING id
		)
		SELECT id FROM created UNION ALL SELECT id FROM campaigns WHERE workspace_id = $1 AND lower(name) = lower($2) LIMIT 1`,
		ws, campaignName).Scan(&campaign); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `SAVEPOINT reengagement_launch`); err != nil {
		return 0, err
	}
	id, err := emails.InsertDraft(ctx, tx, ws, emails.Draft{
		CampaignID: campaign, Subject: s.Subject, FromName: s.FromName, FromAddress: s.FromAddress,
		ReplyTo: s.ReplyTo, HTML: s.HTML, ContactIDs: ids,
	})
	if err == nil {
		_, err = emails.Launch(ctx, tx, ws, id)
	}
	if err != nil {
		slog.Warn("re-engagement email not launched", "workspace_id", ws, "contacts", len(ids), "err", err)
		_, rollbackErr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT reengagement_launch`)
		return 0, rollbackErr
	}
	_, err = tx.Exec(ctx, `UPDATE contacts SET reengage_sent_at = now() WHERE id = ANY($1::uuid[])`, ids)
	return len(ids), err
}
