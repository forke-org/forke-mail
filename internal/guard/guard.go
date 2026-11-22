// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package guard

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Guard struct {
	Pool             *pgxpool.Pool
	MaxBounceRate    float64
	MaxComplaintRate float64
	MinSample        int
}

type Window struct {
	Window        string  `json:"window"`
	Sent          int     `json:"sent"`
	Bounced       int     `json:"bounced"`
	Complained    int     `json:"complained"`
	BounceRate    float64 `json:"bounce_rate"`
	ComplaintRate float64 `json:"complaint_rate"`
}

func (g Guard) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		if err := g.Check(ctx); err != nil && ctx.Err() == nil {
			slog.Error("platform guard", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func Windows(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}) ([]Window, error) {
	rows, err := q.Query(ctx, `
		WITH m AS (
			SELECT sent_at, bounced_at, complained_at FROM email_recipients WHERE sent_at > now() - interval '7 days'
			UNION ALL
			SELECT sent_at, bounced_at, complained_at FROM transactional_messages WHERE sent_at > now() - interval '7 days'
		)
		SELECT w, COUNT(*)::int, COUNT(bounced_at)::int, COUNT(complained_at)::int FROM (
			SELECT '24h' AS w, * FROM m WHERE sent_at > now() - interval '24 hours'
			UNION ALL SELECT '7d', * FROM m
		) x GROUP BY w ORDER BY w`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Window, error) {
		var w Window
		err := row.Scan(&w.Window, &w.Sent, &w.Bounced, &w.Complained)
		if w.Sent > 0 {
			w.BounceRate = float64(w.Bounced) / float64(w.Sent)
			w.ComplaintRate = float64(w.Complained) / float64(w.Sent)
		}
		return w, err
	})
}

func (g Guard) reason(ws []Window) (string, *Window) {
	for i := range ws {
		w := &ws[i]
		if w.Sent < g.MinSample {
			continue
		}
		if w.BounceRate > g.MaxBounceRate {
			return "bounce_rate_" + w.Window, w
		}
		if w.ComplaintRate > g.MaxComplaintRate {
			return "complaint_rate_" + w.Window, w
		}
	}
	return "", nil
}

func (g Guard) Check(ctx context.Context) error {
	windows, err := Windows(ctx, g.Pool)
	if err != nil {
		return err
	}
	reason, w := g.reason(windows)
	return pgx.BeginFunc(ctx, g.Pool, func(tx pgx.Tx) error {
		if reason == "" {
			_, err := tx.Exec(ctx, `UPDATE platform_guard SET checked_at = now() WHERE id = 1`)
			return err
		}
		detail := map[string]any{"reason": reason, "window": w.Window, "sent": w.Sent, "bounced": w.Bounced, "complained": w.Complained,
			"bounce_rate": w.BounceRate, "complaint_rate": w.ComplaintRate, "pause_bounce_rate": g.MaxBounceRate, "pause_complaint_rate": g.MaxComplaintRate}
		tag, err := tx.Exec(ctx, `
			UPDATE platform_guard SET active = true, reason = $1, detail = $2, activated_at = now(), checked_at = now()
			WHERE id = 1 AND NOT active`, reason, detail)
		if err != nil || tag.RowsAffected() == 0 {
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE platform_guard SET checked_at = now() WHERE id = 1`)
			}
			return err
		}
		slog.Warn("platform guard on", "reason", reason, "sent", w.Sent, "bounced", w.Bounced, "complained", w.Complained)
		_, err = tx.Exec(ctx, `INSERT INTO workspace_alerts (workspace_id, kind, detail) SELECT id, 'platform_guard', $1 FROM workspaces WHERE platform`, detail)
		return err
	})
}
