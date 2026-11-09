package workspaces

import (
	"context"
	"errors"
	"testing"

	"forkemail/internal/db/dbtest"
)

func TestMonthlyLimitStopsReservations(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	dbtest.Exec(t, pool, `UPDATE workspaces SET daily_limit = 1000 WHERE id = $1`, ws)
	used, err := MonthlyUsage(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	SetMonthlyLimit(used + 2)
	t.Cleanup(func() { SetMonthlyLimit(0) })
	for i := 0; i < 2; i++ {
		if _, err := Reserve(context.Background(), pool, ws); err != nil {
			t.Fatalf("reserve %d: %v", i, err)
		}
	}
	if _, err := Reserve(context.Background(), pool, ws); !errors.Is(err, ErrMonthlyLimitReached) {
		t.Fatalf("third reserve: %v", err)
	}
	SetMonthlyLimit(0)
	if _, err := Reserve(context.Background(), pool, ws); err != nil {
		t.Fatalf("unlimited reserve: %v", err)
	}
}
