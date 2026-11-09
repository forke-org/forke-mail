package sender

import (
	"log/slog"
	"time"

	"golang.org/x/time/rate"
)

const (
	dailyQuotaHeadroom = 0.9
	sesPauseHold       = 10 * time.Minute
)

type Limits struct {
	MaxSendRate     float64
	Max24HourSend   float64
	SentLast24Hours float64
}

func (w *Worker) SetLimits(l Limits) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.limits = l
	w.sentSinceCheck = 0
	if l.MaxSendRate > 0 {
		w.globalRate = l.MaxSendRate
		w.global.SetLimit(rate.Limit(l.MaxSendRate))
		w.global.SetBurst(max(1, int(l.MaxSendRate)))
	}
}

func (w *Worker) HoldSending(reason string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.holdUntil = time.Now().Add(sesPauseHold)
	slog.Warn("worker holding sends", "reason", reason, "until", w.holdUntil)
}

func (w *Worker) CountSent() {
	w.mu.Lock()
	w.sentSinceCheck++
	w.mu.Unlock()
}

func (w *Worker) idle() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if time.Now().Before(w.holdUntil) {
		return true
	}
	l := w.limits
	if l.Max24HourSend <= 0 {
		return false
	}
	sent := l.SentLast24Hours + float64(w.sentSinceCheck)
	if sent < l.Max24HourSend*dailyQuotaHeadroom {
		return false
	}
	if time.Since(w.quotaLogged) > time.Minute {
		w.quotaLogged = time.Now()
		slog.Warn("24 hour send quota nearly used, worker idle", "sent_last_24h", sent, "max_24h", l.Max24HourSend)
	}
	return true
}

func (w *Worker) GlobalRate() float64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.globalRate
}
