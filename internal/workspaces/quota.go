package workspaces

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrTrialQuotaExhausted = errors.New("trial_quota_exhausted")
	ErrDailyLimitReached   = errors.New("daily_limit_reached")
)

type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type Reservation struct {
	WorkspaceID string
	Trial       bool
	Day         time.Time
}

func Reserve(ctx context.Context, pool beginner, workspaceID string) (Reservation, error) {
	res := Reservation{WorkspaceID: workspaceID}
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := checkMonthly(ctx, tx); err != nil {
			return err
		}
		var plan string
		var dailyLimit int
		err := tx.QueryRow(ctx, `
			UPDATE workspaces
			SET trial_sent_count = trial_sent_count + CASE WHEN plan = 'trial' THEN 1 ELSE 0 END
			WHERE id = $1 AND (plan = 'paid' OR trial_sent_count < trial_send_limit)
			RETURNING plan, daily_limit`, workspaceID).Scan(&plan, &dailyLimit)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTrialQuotaExhausted
		}
		if err != nil {
			return err
		}
		res.Trial = plan == "trial"
		err = tx.QueryRow(ctx, `
			INSERT INTO usage_daily (workspace_id, date, sent_count)
			SELECT $1, current_date, 1 WHERE $2 OR $3 > 0
			ON CONFLICT (workspace_id, date) DO UPDATE SET sent_count = usage_daily.sent_count + 1
			WHERE $2 OR usage_daily.sent_count < $3
			RETURNING date`, workspaceID, res.Trial, dailyLimit).Scan(&res.Day)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDailyLimitReached
		}
		return err
	})
	return res, err
}

func Release(ctx context.Context, pool beginner, res Reservation) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if res.Trial {
			if _, err := tx.Exec(ctx, `
				UPDATE workspaces SET trial_sent_count = trial_sent_count - 1
				WHERE id = $1 AND trial_sent_count > 0`, res.WorkspaceID); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `
			UPDATE usage_daily SET sent_count = sent_count - 1
			WHERE workspace_id = $1 AND date = $2 AND sent_count > 0`, res.WorkspaceID, res.Day)
		return err
	})
}

func SendRate(plan string, configured int, globalRate, trialRate float64) float64 {
	if plan == "trial" {
		return min(trialRate, globalRate*0.7)
	}
	return min(float64(configured), globalRate*0.7)
}
