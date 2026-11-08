package db_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"forkemail/internal/db/dbtest"
	"forkemail/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestDeveloperMigrationsRollbackReapply(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	schema := fmt.Sprintf("intern3_migration_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`) })
	cfg, err := pgx.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*cfg)
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.DownTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=$1 AND table_name='webhook_deliveries')`, schema).Scan(&exists); err != nil || !exists {
		t.Fatalf("reapply failed %v", err)
	}
}
