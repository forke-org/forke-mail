package dbtest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"forkemail/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync"
	"testing"
	"time"
)

var schemas = struct {
	sync.Mutex
	names map[string]string
}{names: map[string]string{}}

func isolatedPool(t *testing.T, raw string) *pgxpool.Pool {
	t.Helper()
	schemas.Lock()
	defer schemas.Unlock()
	ctx := context.Background()
	schema := schemas.names[t.Name()]
	if schema == "" {
		admin, err := db.Connect(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := pgx.BeginFunc(ctx, admin, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('forkemail:test-extension',0))`); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public`)
			return err
		}); err != nil {
			admin.Close()
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(t.Name()))
		schema = fmt.Sprintf("test_%d_%x_%d", os.Getpid(), hash[:5], time.Now().UnixNano())
		if _, err := admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
			admin.Close()
			t.Fatal(err)
		}
		schemas.names[t.Name()] = schema
		t.Cleanup(func() {
			schemas.Lock()
			delete(schemas.names, t.Name())
			schemas.Unlock()
			defer admin.Close()
			if _, err := admin.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`); err != nil {
				t.Error(err)
			}
		})
	}
	cfg, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.MaxConns = 6
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	return pool
}
