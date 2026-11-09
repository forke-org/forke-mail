package db

import (
	"context"
	"time"

	"forkemail/migrations"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	lock, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer lock.Release()
	if _, err := lock.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended('forkemail:migrate:'||current_schema(),0))`); err != nil {
		return err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := lock.Exec(c, `SELECT pg_advisory_unlock(hashtextextended('forkemail:migrate:'||current_schema(),0))`); e != nil {
			lock.Conn().Close(c)
		}
	}()
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		return err
	}
	_, err = provider.Up(ctx)
	return err
}
