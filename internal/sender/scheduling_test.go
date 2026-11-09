package sender

import (
	"context"
	"errors"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/unsub"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrentUrgentCannotOversubscribeQuota(t *testing.T) {
	w := NewWorker(nil, nil, unsub.Signer{}, "", 1000, 10)
	w.SetLimits(Limits{Max24HourSend: 1})
	var n atomic.Int32
	permits := make(chan Permit, 16)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			if p, err := w.AcquireUrgent(context.Background()); err == nil {
				n.Add(1)
				permits <- p
			}
		})
	}
	wg.Wait()
	if n.Load() != 1 {
		t.Fatalf("quota granted %d times", n.Load())
	}
	p := <-permits
	p.Finish(context.Background(), true)
	if _, err := w.AcquireUrgent(context.Background()); !errors.Is(err, ErrSESQuotaExhausted) {
		t.Fatal("used quota granted again")
	}
}

func TestBulkProgressUnderContinuousUrgentLoad(t *testing.T) {
	w := NewWorker(nil, nil, unsub.Signer{}, "", 500, 10)
	w.urgentWaiting.Store(100)
	w.urgentStreak = 8
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p, err := w.waitBulk(ctx, "", 0)
	if err != nil {
		t.Fatal("bulk starved", err)
	}
	p.Finish(ctx, false)
	if w.urgentStreak != 0 {
		t.Fatal("fairness budget not reset")
	}
	ctx, cancel2 := context.WithCancel(context.Background())
	cancel2()
	for w.global.Allow() {
	}
	if _, err := w.AcquireUrgent(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
}

func TestStaleBroadcastOutcomeNeverRequeued(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	email := dbtest.DraftEmail(t, pool, ws)
	rid := dbtest.SentRecipient(t, pool, ws, email, "a@example.com", "stale-"+ws)
	dbtest.Exec(t, pool, `UPDATE email_recipients SET status='sending',claimed_at=now()-interval '10 minutes' WHERE id=$1`, rid)
	if _, err := sweep(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	if value := dbtest.QueryString(t, pool, `SELECT status||':'||last_error FROM email_recipients WHERE id=$1`, rid); !strings.HasPrefix(value, "failed:unknown_outcome") {
		t.Fatal(value)
	}
	if rows, err := Claim(context.Background(), pool, ws, 1); err != nil || len(rows) != 0 {
		t.Fatal("ambiguous outcome requeued", err)
	}
}

func TestSustainedConcurrentUrgentLoadAllowsBulkWithinSharedRate(t *testing.T) {
	w := NewWorker(nil, nil, unsub.Signer{}, "", 40, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	var urgent, bulk atomic.Int32
	var wg sync.WaitGroup
	start := time.Now()
	for range 8 {
		wg.Go(func() {
			for ctx.Err() == nil {
				p, err := w.AcquireUrgent(ctx)
				if err != nil {
					return
				}
				urgent.Add(1)
				p.Finish(ctx, true)
			}
		})
	}
	deadline := time.Now().Add(time.Second)
	for urgent.Load() < 8 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	for range 4 {
		p, err := w.waitBulk(ctx, "", 0)
		if err != nil {
			cancel()
			wg.Wait()
			t.Fatal("broadcast starved under actual urgent load", err)
		}
		bulk.Add(1)
		p.Finish(ctx, true)
	}
	cancel()
	wg.Wait()
	if urgent.Load() < 8 || bulk.Load() != 4 {
		t.Fatal("both work classes did not progress")
	}
	if total := urgent.Load() + bulk.Load(); float64(total) > 41+40*time.Since(start).Seconds() {
		t.Fatalf("shared token bucket exceeded: %d", total)
	}
}
