package sender

import (
	"context"
	"forkemail/internal/db"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const stuckAfter = 5 * time.Minute

func sweep(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var n int64
	err := db.WithLock(ctx, pool, "bulk-recovery", func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `WITH failed AS (
	UPDATE email_recipients SET status='failed',last_error='unknown_outcome',claimed_at=NULL
	WHERE status='sending' AND claimed_at<now()-make_interval(secs=>$1) AND (provider_started_at IS NOT NULL OR claim_token IS NULL)
	RETURNING workspace_id,email_id,id), recorded AS (
	INSERT INTO email_events(workspace_id,email_id,recipient_id,type,detail)
	SELECT workspace_id,email_id,id,'failed','{"reason":"unknown_outcome"}'::jsonb FROM failed)
	SELECT count(*) FROM failed`, stuckAfter.Seconds()).Scan(&n)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE email_recipients SET status='pending',claimed_at=NULL,claim_token=NULL,attempts=greatest(attempts-1,0) WHERE status='sending' AND provider_started_at IS NULL AND claim_token IS NOT NULL AND claimed_at<clock_timestamp()-interval '5 minutes'`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE sending_permits SET state=CASE WHEN state='sending' THEN 'used' ELSE 'released' END,finished_at=expires_at WHERE state IN ('reserved','sending') AND expires_at<clock_timestamp()`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM sending_waiters WHERE expires_at<clock_timestamp()`); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM sending_permits WHERE id IN(SELECT id FROM sending_permits WHERE state IN ('used','released') AND finished_at<clock_timestamp()-interval '25 hours' LIMIT 1000)`)
		return err
	})
	return n, err
}

func RunSweeper(ctx context.Context, pool *pgxpool.Pool) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := sweep(ctx, pool)
			if err != nil {
				slog.Error("sweeper failed", "err", err)
			} else if n > 0 {
				slog.Warn("sweeper reset stuck recipients", "count", n)
			}
		}
	}
}
