package sender

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const stuckAfter = 5 * time.Minute

func sweep(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var n int64
	err := pool.QueryRow(ctx, `WITH failed AS (
	UPDATE email_recipients SET status='failed',last_error='unknown_outcome',claimed_at=NULL
	WHERE status='sending' AND claimed_at<now()-make_interval(secs=>$1)
	RETURNING workspace_id,email_id,id), recorded AS (
	INSERT INTO email_events(workspace_id,email_id,recipient_id,type,detail)
	SELECT workspace_id,email_id,id,'failed','{"reason":"unknown_outcome"}'::jsonb FROM failed)
	SELECT count(*) FROM failed`, stuckAfter.Seconds()).Scan(&n)
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
