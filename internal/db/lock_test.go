package db_test

import (
	"context"
	"forkemail/internal/db"
	"forkemail/internal/db/dbtest"
	"github.com/jackc/pgx/v5"
	"testing"
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
