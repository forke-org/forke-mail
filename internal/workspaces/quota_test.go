package workspaces

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"forkemail/internal/db/dbtest"
)

func TestTrialQuotaNeverExceededUnderConcurrency(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "trial", 25)
	var granted, exhausted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Reserve(context.Background(), pool, ws)
			switch {
			case err == nil:
				granted.Add(1)
			case errors.Is(err, ErrTrialQuotaExhausted):
				exhausted.Add(1)
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if granted.Load() != 25 || exhausted.Load() != 75 {
		t.Fatalf("granted %d, exhausted %d", granted.Load(), exhausted.Load())
	}
	if got := dbtest.QueryString(t, pool, `SELECT trial_sent_count::text FROM workspaces WHERE id = $1`, ws); got != "25" {
		t.Fatalf("trial_sent_count = %s", got)
	}
}

func TestReleaseReturnsQuota(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "trial", 1)
	res, err := Reserve(context.Background(), pool, ws)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Reserve(context.Background(), pool, ws); !errors.Is(err, ErrTrialQuotaExhausted) {
		t.Fatalf("got %v, want exhausted", err)
	}
	if err := Release(context.Background(), pool, res); err != nil {
		t.Fatal(err)
	}
	if _, err := Reserve(context.Background(), pool, ws); err != nil {
		t.Fatalf("reserve after release: %v", err)
	}
}

func TestPaidDailyLimit(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	dbtest.Exec(t, pool, `UPDATE workspaces SET daily_limit = 2 WHERE id = $1`, ws)
	for i := 0; i < 2; i++ {
		if _, err := Reserve(context.Background(), pool, ws); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Reserve(context.Background(), pool, ws); !errors.Is(err, ErrDailyLimitReached) {
		t.Fatalf("got %v, want daily limit", err)
	}
}
