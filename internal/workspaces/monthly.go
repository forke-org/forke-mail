package workspaces

import (
	"context"
	"errors"
	"sync/atomic"

	"forkemail/internal/db"
)

var ErrMonthlyLimitReached = errors.New("monthly_limit_reached")

var monthlyLimit atomic.Int64

func SetMonthlyLimit(n int64) {
	monthlyLimit.Store(n)
}

func MonthlyLimit() int64 {
	return monthlyLimit.Load()
}

func MonthlyUsage(ctx context.Context, q db.Querier) (int64, error) {
	var used int64
	err := q.QueryRow(ctx, `
		SELECT COALESCE(sum(sent_count), 0) FROM usage_daily
		WHERE date >= date_trunc('month', current_date)::date`).Scan(&used)
	return used, err
}

func checkMonthly(ctx context.Context, q db.Querier) error {
	limit := monthlyLimit.Load()
	if limit <= 0 {
		return nil
	}
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('forke-mail-monthly-limit'))`); err != nil {
		return err
	}
	used, err := MonthlyUsage(ctx, q)
	if err != nil {
		return err
	}
	if used >= limit {
		return ErrMonthlyLimitReached
	}
	return nil
}
