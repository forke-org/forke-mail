package sender

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const stuckAfter = 5 * time.Minute

func sweep(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE email_recipients SET status = 'pending', claimed_at = NULL
		WHERE status = 'sending' AND claimed_at < now() - make_interval(secs => $1)`, stuckAfter.Seconds())
	return tag.RowsAffected(), err
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
