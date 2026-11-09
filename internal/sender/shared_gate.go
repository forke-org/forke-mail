package sender

import (
	"context"
	"github.com/jackc/pgx/v5"
	"math"
	"time"
)

func (w *Worker) ensureControl(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ensureControlLocked(ctx)
}

func (w *Worker) ensureControlLocked(ctx context.Context) error {
	r := w.globalRate
	_, err := w.pool.Exec(ctx, `INSERT INTO sending_control(id,rate,burst,tokens) VALUES(1,$1,$2::integer,$2::integer::double precision) ON CONFLICT(id) DO NOTHING`, r, max(1, int(r)))
	return err
}

func (w *Worker) acquireShared(ctx context.Context, urgent bool, workspace string, perSecond float64) (Permit, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := w.ensureControl(ctx); err != nil {
		return Permit{}, err
	}
	var waiter string
	if err := w.pool.QueryRow(ctx, `INSERT INTO sending_waiters(urgent) VALUES($1) RETURNING id`, urgent).Scan(&waiter); err != nil {
		return Permit{}, err
	}
	defer func() {
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		w.pool.Exec(c, `DELETE FROM sending_waiters WHERE id=$1`, waiter)
	}()
	for {
		id, delay, err := w.sharedTurn(ctx, waiter, urgent, workspace, perSecond)
		if err != nil {
			return Permit{}, err
		}
		if id != "" {
			return w.sharedPermit(id), nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Permit{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (w *Worker) sharedTurn(ctx context.Context, waiter string, urgent bool, workspace string, perSecond float64) (string, time.Duration, error) {
	var permit string
	delay := 10 * time.Millisecond
	var denied error
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		var now, refilled, hold, snapshot time.Time
		var rate, tokens, max24, observed float64
		var burst, streak int
		if err := tx.QueryRow(ctx, `SELECT rate,burst,tokens,refilled_at,greatest(hold_until,'1970-01-01'::timestamptz),urgent_streak,max_24h,snapshot_sent,greatest(snapshot_at,'1970-01-01'::timestamptz) FROM sending_control WHERE id=1 FOR UPDATE`).Scan(&rate, &burst, &tokens, &refilled, &hold, &streak, &max24, &observed, &snapshot); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		if now.Before(hold) {
			return ErrSendingHeld
		}
		if _, err := tx.Exec(ctx, `UPDATE sending_permits SET state=CASE WHEN state='sending' THEN 'used' ELSE 'released' END,finished_at=expires_at WHERE state IN ('reserved','sending') AND expires_at<clock_timestamp()`); err != nil {
			return err
		}
		var used, since, active int
		if err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='used' AND finished_at>clock_timestamp()-interval '24 hours'),count(*) FILTER(WHERE state='used' AND finished_at>=greatest($1,clock_timestamp()-interval '24 hours')),count(*) FILTER(WHERE state IN ('reserved','sending')) FROM sending_permits WHERE state<>'released'`, snapshot).Scan(&used, &since, &active); err != nil {
			return err
		}
		quota := math.Max(float64(used), observed+float64(since)) + float64(active)
		threshold := max24
		if !urgent {
			threshold *= dailyQuotaHeadroom
		}
		if max24 > 0 && quota >= threshold {
			denied = ErrSESQuotaExhausted
			return nil
		}
		ready := now
		if workspace != "" {
			perSecond = min(perSecond, rate*.7)
			if _, err := tx.Exec(ctx, `INSERT INTO workspace_send_gates(workspace_id) VALUES($1) ON CONFLICT DO NOTHING`, workspace); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT greatest(next_at,'1970-01-01'::timestamptz) FROM workspace_send_gates WHERE workspace_id=$1 FOR UPDATE`, workspace).Scan(&ready); err != nil {
				return err
			}
			if ready.Before(now) {
				ready = now
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sending_waiters(id,urgent,ready_at,expires_at) VALUES($1,$3,$2,clock_timestamp()+interval '30 seconds') ON CONFLICT(id) DO UPDATE SET ready_at=excluded.ready_at,expires_at=excluded.expires_at`, waiter, ready, urgent); err != nil {
			return err
		}
		var urgentReady, bulkReady bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sending_waiters WHERE urgent AND expires_at>clock_timestamp() AND ready_at<=clock_timestamp()),EXISTS(SELECT 1 FROM sending_waiters WHERE NOT urgent AND expires_at>clock_timestamp() AND ready_at<=clock_timestamp())`).Scan(&urgentReady, &bulkReady); err != nil {
			return err
		}
		tokens = min(float64(burst), tokens+max(0, now.Sub(refilled).Seconds())*rate)
		if ready.After(now) {
			delay = min(100*time.Millisecond, max(time.Millisecond, ready.Sub(now)))
			return nil
		}
		if (urgent && bulkReady && streak >= 8) || (!urgent && urgentReady && streak < 8) {
			return nil
		}
		if tokens < 1 {
			delay = min(100*time.Millisecond, max(time.Millisecond, time.Duration((1-tokens)/rate*float64(time.Second))))
			return nil
		}
		nextStreak := 0
		if urgent {
			nextStreak = min(8, streak+1)
		}
		if _, err := tx.Exec(ctx, `UPDATE sending_control SET tokens=$1,refilled_at=$2,urgent_streak=$3 WHERE id=1`, tokens-1, now, nextStreak); err != nil {
			return err
		}
		if workspace != "" {
			if _, err := tx.Exec(ctx, `UPDATE workspace_send_gates SET next_at=clock_timestamp()+make_interval(secs=>$2) WHERE workspace_id=$1`, workspace, 1/perSecond); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx, `INSERT INTO sending_permits DEFAULT VALUES RETURNING id`).Scan(&permit)
	})
	if err == nil {
		err = denied
	}
	return permit, delay, err
}
