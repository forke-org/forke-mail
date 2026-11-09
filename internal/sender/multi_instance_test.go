package sender

import (
	"context"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/unsub"
	"github.com/jackc/pgx/v5"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingSender struct {
	mu   sync.Mutex
	seen map[string]int
}

func (s *recordingSender) Send(_ context.Context, m Message) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[m.RecipientID]++
	return "multi-" + m.RecipientID, nil
}

func TestTwoBulkWorkersDispatchEachRecipientOnce(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "trial", 20)
	email := dbtest.DraftEmail(t, pool, ws)
	domain := ws + ".multi.test"
	dbtest.Exec(t, pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified')`, ws, domain)
	dbtest.Exec(t, pool, `INSERT INTO contacts(workspace_id,email) SELECT $1,'bulk'||g||'@example.com' FROM generate_series(1,20)g`, ws)
	dbtest.Exec(t, pool, `INSERT INTO email_recipients(workspace_id,email_id,contact_id,email) SELECT workspace_id,$2,id,email FROM contacts WHERE workspace_id=$1`, ws, email)
	dbtest.Exec(t, pool, `UPDATE emails SET status='sending',from_address=$2 WHERE id=$1`, email, "hello@"+domain)
	fake := &recordingSender{seen: map[string]int{}}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for range 2 {
		w := NewWorker(pool, fake, unsub.Signer{Secret: []byte(strings.Repeat("k", 32))}, "http://localhost", 50, 20)
		wg.Go(func() { w.Run(ctx) })
	}
	defer func() { cancel(); wg.Wait() }()
	until := time.Now().Add(12 * time.Second)
	for time.Now().Before(until) {
		if dbtest.QueryString(t, pool, `SELECT count(*)::text FROM email_recipients WHERE workspace_id=$1 AND status='sent'`, ws) == "20" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	wg.Wait()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.seen) != 20 {
		t.Fatalf("sent %d unique recipients", len(fake.seen))
	}
	for id, n := range fake.seen {
		if n != 1 {
			t.Fatalf("%s sent %d times", id, n)
		}
	}
	if got := dbtest.QueryString(t, pool, `SELECT trial_sent_count::text FROM workspaces WHERE id=$1`, ws); got != "20" {
		t.Fatal("quota", got)
	}
}

func TestBulkCrashRecoveryFencesOldOwner(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	email := dbtest.DraftEmail(t, pool, ws)
	dbtest.Exec(t, pool, `UPDATE emails SET status='sending' WHERE id=$1`, email)
	dbtest.Exec(t, pool, `INSERT INTO email_recipients(workspace_id,email_id,email) VALUES($1,$2,'a@example.com')`, ws, email)
	old, err := Claim(context.Background(), pool, ws, 1)
	if err != nil || len(old) != 1 {
		t.Fatal(err)
	}
	dbtest.Exec(t, pool, `UPDATE email_recipients SET claimed_at=clock_timestamp()-interval '6 minutes' WHERE id=$1`, old[0].ID)
	if _, err := sweep(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	newer, err := Claim(context.Background(), pool, ws, 1)
	if err != nil || len(newer) != 1 || newer[0].ClaimToken == old[0].ClaimToken {
		t.Fatal("lease not replaced")
	}
	if err := Unclaim(context.Background(), pool, old); err != nil {
		t.Fatal(err)
	}
	if got := dbtest.QueryString(t, pool, `SELECT claim_token::text FROM email_recipients WHERE id=$1`, old[0].ID); got != newer[0].ClaimToken {
		t.Fatal("old owner changed new claim")
	}
	w := NewWorker(pool, nil, unsub.Signer{}, "", 20, 1)
	w.complete(context.Background(), old[0], "sent", "late", "")
	if got := dbtest.QueryString(t, pool, `SELECT status FROM email_recipients WHERE id=$1`, old[0].ID); got != "sending" {
		t.Fatal("stale completion accepted")
	}
	dbtest.Exec(t, pool, `UPDATE email_recipients SET provider_started_at=clock_timestamp(),claimed_at=clock_timestamp()-interval '6 minutes' WHERE id=$1`, newer[0].ID)
	if _, err := sweep(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	if rows, err := Claim(context.Background(), pool, ws, 1); err != nil || len(rows) != 0 {
		t.Fatal("ambiguous provider claim retried")
	}
}

func TestSharedRateQuotaHoldAndPermitRecovery(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	a := NewWorker(pool, nil, unsub.Signer{}, "", 8, 1)
	b := NewWorker(pool, nil, unsub.Signer{}, "", 8, 1)
	if err := a.ensureControl(ctx); err != nil {
		t.Fatal(err)
	}
	dbtest.Exec(t, pool, `UPDATE sending_control SET tokens=0,refilled_at=clock_timestamp()`)
	start := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var times []time.Duration
	for _, w := range []*Worker{a, b} {
		wg.Go(func() {
			for range 8 {
				p, err := w.AcquireUrgent(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				times = append(times, time.Since(start))
				mu.Unlock()
				p.Finish(ctx, false)
			}
		})
	}
	wg.Wait()
	if len(times) != 16 || time.Since(start) < 1900*time.Millisecond {
		t.Fatal("combined rate exceeded", len(times), time.Since(start))
	}
	for i, elapsed := range times {
		if float64(i+1) > elapsed.Seconds()*8+1.1 {
			t.Fatalf("aggregate budget exceeded at %d: %s", i, elapsed)
		}
	}
	if err := a.SetLimits(Limits{MaxSendRate: 1000, Max24HourSend: 1}); err != nil {
		t.Fatal(err)
	}
	first, err := a.AcquireUrgent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AcquireUrgent(ctx); err != ErrSESQuotaExhausted {
		t.Fatalf("other instance bypassed quota: %v", err)
	}
	dbtest.Exec(t, pool, `UPDATE sending_permits SET expires_at=clock_timestamp()-interval '1 second' WHERE state='reserved'`)
	second, err := b.AcquireUrgent(ctx)
	if err != nil {
		t.Fatal("unused permit not recovered", err)
	}
	first.Finish(ctx, false)
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return second.Start(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
	dbtest.Exec(t, pool, `UPDATE sending_permits SET expires_at=clock_timestamp()-interval '1 second' WHERE state='sending'`)
	if _, err := a.AcquireUrgent(ctx); err != ErrSESQuotaExhausted {
		t.Fatal("ambiguous permit refunded", err)
	}
	second.Finish(ctx, false)
	if got := dbtest.QueryString(t, pool, `SELECT count(*)::text FROM sending_permits WHERE state='used'`); got != "1" {
		t.Fatal("late refund changed uncertain quota", got)
	}
	if err := a.SetLimits(Limits{MaxSendRate: 1000}); err != nil {
		t.Fatal(err)
	}
	a.HoldSending("test")
	if _, err := b.AcquireUrgent(ctx); err != ErrSendingHeld {
		t.Fatal("hold not shared", err)
	}
}

func TestSharedQuotaSurvivesSnapshotRefreshAndRestart(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	a := NewWorker(pool, nil, unsub.Signer{}, "", 1000, 10)
	if err := a.SetLimits(Limits{Max24HourSend: 2}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		p, err := a.AcquireUrgent(ctx)
		if err != nil {
			t.Fatal(err)
		}
		p.Finish(ctx, true)
	}
	var observed time.Time
	if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&observed); err != nil {
		t.Fatal(err)
	}
	b := NewWorker(pool, nil, unsub.Signer{}, "", 1000, 10)
	if err := b.SetLimits(Limits{Max24HourSend: 2, SentLast24Hours: 1, ObservedAt: observed}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetLimits(Limits{Max24HourSend: 100, ObservedAt: observed.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AcquireUrgent(ctx); err != ErrSESQuotaExhausted {
		t.Fatal("refresh or stale snapshot refunded usage", err)
	}
	restarted := NewWorker(pool, nil, unsub.Signer{}, "", 1000, 10)
	if _, err := restarted.AcquireUrgent(ctx); err != ErrSESQuotaExhausted {
		t.Fatal("restart reset account quota", err)
	}
}

func TestSharedPriorityAllowsBulkAcrossInstances(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	urgent := NewWorker(pool, nil, unsub.Signer{}, "", 40, 10)
	bulk := NewWorker(pool, nil, unsub.Signer{}, "", 40, 10)
	var wg sync.WaitGroup
	started := make(chan struct{}, 8)
	for range 8 {
		wg.Go(func() {
			started <- struct{}{}
			for ctx.Err() == nil {
				p, err := urgent.AcquireUrgent(ctx)
				if err != nil {
					return
				}
				p.Finish(ctx, false)
			}
		})
	}
	for range 8 {
		<-started
	}
	for range 3 {
		p, err := bulk.waitBulk(ctx, "", 0)
		if err != nil {
			cancel()
			wg.Wait()
			t.Fatal("bulk starved across instances", err)
		}
		p.Finish(ctx, false)
	}
	cancel()
	wg.Wait()
}
