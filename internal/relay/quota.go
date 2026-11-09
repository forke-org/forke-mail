package relay

import (
	"context"
	"log/slog"
	"time"

	"forkemail/internal/sender"

	"github.com/jackc/pgx/v5/pgxpool"
)

const quotaRefresh = time.Minute

type Usage struct {
	Last24Hours int
	ThisMonth   int
}

type Quota struct {
	Pool         *pgxpool.Pool
	SendRate     float64
	DailyLimit   int
	MonthlyLimit int
	Apply        func(sender.Limits) error
}

func (q Quota) Limits(u Usage) sender.Limits {
	l := sender.Limits{MaxSendRate: q.SendRate}
	if q.DailyLimit == 0 && q.MonthlyLimit == 0 {
		return l
	}
	remaining := -1
	if q.DailyLimit > 0 {
		remaining = q.DailyLimit - u.Last24Hours
	}
	if q.MonthlyLimit > 0 {
		monthly := q.MonthlyLimit - u.ThisMonth
		if remaining < 0 || monthly < remaining {
			remaining = monthly
		}
	}
	if remaining <= 0 {
		return sender.Limits{MaxSendRate: q.SendRate, Max24HourSend: 1, SentLast24Hours: 1}
	}
	l.SentLast24Hours = float64(u.Last24Hours)
	l.Max24HourSend = float64(u.Last24Hours + remaining)
	return l
}

func (q Quota) Refresh(ctx context.Context) (Usage, error) {
	var u Usage
	err := q.Pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM email_recipients WHERE sent_at > now() - interval '24 hours') +
			(SELECT count(*) FROM transactional_messages WHERE sent_at > now() - interval '24 hours'),
			(SELECT count(*) FROM email_recipients WHERE sent_at >= date_trunc('month', now())) +
			(SELECT count(*) FROM transactional_messages WHERE sent_at >= date_trunc('month', now()))`).Scan(&u.Last24Hours, &u.ThisMonth)
	if err != nil {
		return Usage{}, err
	}
	return u, q.Apply(q.Limits(u))
}

func (q Quota) Run(ctx context.Context) {
	t := time.NewTicker(quotaRefresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			u, err := q.Refresh(ctx)
			if err != nil {
				if ctx.Err() == nil {
					slog.Error("smtp quota refresh failed", "err", err)
				}
				continue
			}
			if q.MonthlyLimit > 0 && u.ThisMonth >= q.MonthlyLimit {
				slog.Warn("smtp monthly limit reached, sending idle", "sent_this_month", u.ThisMonth, "monthly_limit", q.MonthlyLimit)
			}
		}
	}
}
