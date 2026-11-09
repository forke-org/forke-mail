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

func (w *Worker) AcquireUrgent(ctx context.Context) (Permit, error) {
	if w.pool != nil {
		return w.acquireShared(ctx, true, "", 0)
	}
	if err := w.urgentBlocked(); err != nil {
		return Permit{}, err
	}
	w.urgentWaiting.Add(1)
	defer w.urgentWaiting.Add(-1)
	for {
		w.mu.Lock()
		yield := w.bulkWaiting.Load() > 0 && w.urgentStreak >= 8
		w.mu.Unlock()
		if !yield {
			break
		}
		if err := waitTurn(ctx); err != nil {
			return Permit{}, err
		}
	}
	if err := w.global.Wait(ctx); err != nil {
		return Permit{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.claimSlotLocked(); err != nil {
		return Permit{}, err
	}
	w.urgentStreak = min(8, w.urgentStreak+1)
	return w.localPermit(), nil
}

func (w *Worker) waitBulk(ctx context.Context, workspace string, perSecond float64) (Permit, error) {
	if w.pool != nil {
		return w.acquireShared(ctx, false, workspace, perSecond)
	}
	w.bulkWaiting.Add(1)
	defer w.bulkWaiting.Add(-1)
	for {
		w.mu.Lock()
		if (w.urgentWaiting.Load() == 0 || w.urgentStreak >= 8) && w.global.Allow() {
			err := w.claimSlotLocked()
			if err == nil {
				w.urgentStreak = 0
			}
			w.mu.Unlock()
			if err != nil {
				return Permit{}, err
			}
			return w.localPermit(), nil
		}
		w.mu.Unlock()
		if err := waitTurn(ctx); err != nil {
			return Permit{}, err
		}
	}
}

func waitTurn(ctx context.Context) error {
	t := time.NewTimer(bulkPollInterval)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (w *Worker) claimSlotLocked() error {
	if time.Now().Before(w.holdUntil) {
		return ErrSendingHeld
	}
	if w.limits.Max24HourSend > 0 && w.limits.SentLast24Hours+float64(w.sentSinceCheck+w.inFlight) >= w.limits.Max24HourSend {
		return ErrSESQuotaExhausted
	}
	w.inFlight++
	return nil
}

func (w *Worker) urgentBlocked() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if time.Now().Before(w.holdUntil) {
		return ErrSendingHeld
	}
	l := w.limits
	if l.Max24HourSend > 0 && l.SentLast24Hours+float64(w.sentSinceCheck+w.inFlight) >= l.Max24HourSend {
		return ErrSESQuotaExhausted
	}
	return nil
}
