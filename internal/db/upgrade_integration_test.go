package db_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"forkemail/internal/db/dbtest"
	"forkemail/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestMigrationVersionsUnique(t *testing.T) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		p, _, _ := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(p)
		if err != nil {
			t.Fatal(err)
		}
		if prior, ok := seen[v]; ok {
			t.Fatalf("duplicate migration %d: %s and %s", v, prior, e.Name())
		}
		seen[v] = e.Name()
	}
}

func TestUpgradeFromKrishnaV8PreservesData(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	schema := fmt.Sprintf("krishna_v8_upgrade_%d", time.Now().UnixNano())
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
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.UpTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	var ws, endpoint, receipt, payload string
	if err = db.QueryRowContext(ctx, `INSERT INTO workspaces(name,plan) VALUES('upgrade fixture','paid') RETURNING id`).Scan(&ws); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `INSERT INTO webhook_endpoints(workspace_id,url,secret,events,enabled) VALUES($1,'https://example.com',repeat('x',40),ARRAY['email.queued'],true) RETURNING id`, ws).Scan(&endpoint); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `INSERT INTO transactional_requests(workspace_id,idempotency_key,request_hash,mode) VALUES($1,'legacy-key',repeat('0',64),'development_stub') RETURNING id`, ws).Scan(&receipt); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO email_events(workspace_id,type) VALUES($1,'queued')`, ws); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT payload FROM webhook_deliveries WHERE endpoint_id=$1`, endpoint).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var after string
	if err = db.QueryRowContext(ctx, `SELECT payload FROM webhook_deliveries WHERE endpoint_id=$1`, endpoint).Scan(&after); err != nil || after != payload {
		t.Fatalf("historical payload changed: %v", err)
	}
	if err = db.QueryRowContext(ctx, `SELECT id::text FROM transactional_requests WHERE workspace_id=$1 AND idempotency_key='legacy-key'`, ws).Scan(&after); err != nil || after != receipt {
		t.Fatalf("legacy receipt changed: %v", err)
	}
	if _, err = p.DownTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(ctx, `SELECT payload FROM webhook_deliveries WHERE endpoint_id=$1`, endpoint).Scan(&after); err != nil || after != payload {
		t.Fatalf("reapply changed payload: %v", err)
	}
}
