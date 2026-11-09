package db_test

import (
	"context"
	"fmt"
	"forkemail/internal/db"
	"forkemail/internal/db/dbtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync"
	"testing"
	"time"
)

func TestMaintenanceAdvisoryLockExcludesOtherInstance(t *testing.T) {
	pool := dbtest.Pool(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- db.WithLock(context.Background(), pool, "test-singleton", func(pgx.Tx) error { close(entered); <-release; return nil })
	}()
	<-entered
	called := false
	err := db.WithLock(context.Background(), pool, "test-singleton", func(pgx.Tx) error { called = true; return nil })
	close(release)
	first := <-done
	if err != nil || first != nil || called {
		t.Fatal("singleton lock allowed overlap", err, first)
	}
}

func TestConcurrentStartupMigrationsShareOneHistory(t *testing.T) {
	root := dbtest.Pool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	name := fmt.Sprintf("startup_%d", time.Now().UnixNano())
	if _, err := root.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := root.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{name}.Sanitize()+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = name + ",public"
	cfg.MaxConns = 6
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			if err := db.Migrate(ctx, pool); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := dbtest.QueryString(t, pool, `SELECT count(*)::text FROM goose_db_version WHERE version_id=11 AND is_applied`); got != "1" {
		t.Fatal("migration history duplicated", got)
	}
}
