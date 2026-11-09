package ses

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"forkemail/internal/sender"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/jackc/pgx/v5/pgxpool"
)

const accountRefresh = 10 * time.Minute

type accountAPI interface {
	GetAccount(ctx context.Context, in *sesv2.GetAccountInput, optFns ...func(*sesv2.Options)) (*sesv2.GetAccountOutput, error)
}

type Snapshot struct {
	ProductionAccess bool
	SendingEnabled   bool
	Limits           sender.Limits
	CheckedAt        time.Time
}

type Account struct {
	Client accountAPI
	Apply  func(sender.Limits) error
	Pool   *pgxpool.Pool

	mu   sync.Mutex
	last Snapshot
}

func (a *Account) Refresh(ctx context.Context) (Snapshot, error) {
	observed := time.Now()
	if a.Pool != nil {
		if err := a.Pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&observed); err != nil {
			return Snapshot{}, err
		}
	}
	out, err := a.Client.GetAccount(ctx, &sesv2.GetAccountInput{})
	if err != nil {
		return Snapshot{}, err
	}
	if out.SendQuota == nil {
		return Snapshot{}, errors.New("ses account has no send quota")
	}
	s := Snapshot{
		ProductionAccess: out.ProductionAccessEnabled,
		SendingEnabled:   out.SendingEnabled,
		CheckedAt:        time.Now(),
		Limits: sender.Limits{
			MaxSendRate:     out.SendQuota.MaxSendRate,
			Max24HourSend:   out.SendQuota.Max24HourSend,
			SentLast24Hours: out.SendQuota.SentLast24Hours,
			ObservedAt:      observed,
		},
	}
	a.mu.Lock()
	a.last = s
	a.mu.Unlock()
	if a.Apply != nil {
		if err := a.Apply(s.Limits); err != nil {
			return Snapshot{}, err
		}
	}
	return s, nil
}

func (a *Account) Run(ctx context.Context) {
	t := time.NewTicker(accountRefresh)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s, err := a.Refresh(ctx)
			if err != nil {
				if ctx.Err() == nil {
					slog.Error("ses get account failed", "err", err)
				}
				continue
			}
			slog.Info("ses account refreshed", "max_send_rate", s.Limits.MaxSendRate,
				"max_24h", s.Limits.Max24HourSend, "sent_24h", s.Limits.SentLast24Hours, "production", s.ProductionAccess)
		}
	}
}

func (a *Account) Mode() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last.ProductionAccess {
		return "production"
	}
	return "sandbox"
}
