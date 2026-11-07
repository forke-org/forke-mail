package domains

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pollInterval = 60 * time.Second

type pending struct {
	id, name string
}

func RunPoller(ctx context.Context, pool *pgxpool.Pool, identities Identities) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			poll(ctx, pool, identities)
		}
	}
}

func poll(ctx context.Context, pool *pgxpool.Pool, identities Identities) {
	rows, err := pool.Query(ctx, `
		SELECT id, name FROM domains WHERE status = 'pending' ORDER BY last_checked_at NULLS FIRST LIMIT 100`)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("list pending identities failed", "err", err)
		}
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pending, error) {
		var p pending
		return p, row.Scan(&p.id, &p.name)
	})
	if err != nil {
		slog.Error("scan pending identities failed", "err", err)
		return
	}
	for _, p := range list {
		if err := Refresh(ctx, pool, identities, p.id, p.name); err != nil && ctx.Err() == nil {
			slog.Error("refresh identity failed", "domain_id", p.id, "err", err)
		}
	}
}
