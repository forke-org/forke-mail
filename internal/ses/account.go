package ses

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"forkemail/internal/sender"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
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
	Apply  func(sender.Limits)

	mu   sync.Mutex
	last Snapshot
}

func (a *Account) Refresh(ctx context.Context) (Snapshot, error) {
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
		},
	}
	a.mu.Lock()
	a.last = s
	a.mu.Unlock()
	if a.Apply != nil {
		a.Apply(s.Limits)
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
