package db

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func WithLock(ctx context.Context, pool *pgxpool.Pool, name string, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var acquired bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended(current_schema()||':'||$1,0))`, "forkemail:"+name).Scan(&acquired); err != nil {
			return err
		}
		if !acquired {
			return nil
		}
		return fn(tx)
	})
}
