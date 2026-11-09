package sender

import (
	"context"
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
	ObservedAt      time.Time
}

func (w *Worker) SetLimits(l Limits) error {
	w.mu.Lock()
	w.limits = l
	w.sentSinceCheck = 0
	if l.MaxSendRate > 0 {
		w.globalRate = l.MaxSendRate
		if w.global != nil {
			w.global.SetLimit(rate.Limit(l.MaxSendRate))
			w.global.SetBurst(max(1, int(l.MaxSendRate)))
		}
	}
	r := w.globalRate
	w.mu.Unlock()
	if w.pool == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := w.ensureControl(ctx); err != nil {
		return err
	}
	var observed any
	if !l.ObservedAt.IsZero() {
		observed = l.ObservedAt
	}
	_, err := w.pool.Exec(ctx, `UPDATE sending_control SET tokens=least($2::double precision,least(burst,tokens+greatest(0,extract(epoch from(clock_timestamp()-refilled_at)))*rate)),refilled_at=clock_timestamp(),rate=$1,burst=$2,max_24h=$3,snapshot_sent=$4,snapshot_at=coalesce($5::timestamptz,clock_timestamp()) WHERE id=1 AND ($5::timestamptz IS NULL OR snapshot_at<=$5)`, r, max(1, int(r)), l.Max24HourSend, l.SentLast24Hours, observed)
	return err
}

func (w *Worker) HoldSending(reason string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.holdUntil = time.Now().Add(sesPauseHold)
	slog.Warn("worker holding sends", "reason", reason, "until", w.holdUntil)
	if w.pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := w.ensureControlLocked(ctx); err != nil {
			slog.Error("persist sending hold failed", "err", err)
			return
		}
		if _, err := w.pool.Exec(ctx, `UPDATE sending_control SET hold_until=greatest(hold_until,clock_timestamp()+interval '10 minutes') WHERE id=1`); err != nil {
			slog.Error("persist sending hold failed", "err", err)
		}
	}
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
