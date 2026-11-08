package sender

import (
	"context"
	"errors"
	"time"
)

const bulkPollInterval = 10 * time.Millisecond

var (
	ErrSendingHeld       = errors.New("ses_sending_paused")
	ErrSESQuotaExhausted = errors.New("ses_daily_quota_exhausted")
)

func (w *Worker) AcquireUrgent(ctx context.Context) error {
	if err := w.urgentBlocked(); err != nil {
		return err
	}
	w.urgentWaiting.Add(1)
	defer w.urgentWaiting.Add(-1)
	return w.global.Wait(ctx)
}

func (w *Worker) waitBulk(ctx context.Context) error {
	for {
		if w.urgentWaiting.Load() == 0 && w.global.Allow() {
			return nil
		}
		t := time.NewTimer(bulkPollInterval)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (w *Worker) urgentBlocked() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if time.Now().Before(w.holdUntil) {
		return ErrSendingHeld
	}
	l := w.limits
	if l.Max24HourSend > 0 && l.SentLast24Hours+float64(w.sentSinceCheck) >= l.Max24HourSend {
		return ErrSESQuotaExhausted
	}
	return nil
}
