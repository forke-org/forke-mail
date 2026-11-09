package transactional

import (
	"context"
	"fmt"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/sender"
	"forkemail/internal/tracking"
	"forkemail/internal/unsub"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestThreeDispatchersSendOnceAndReserveOnce(t *testing.T) {
	e := setup(t, "trial", 24)
	ids := map[string]bool{}
	for i := range 24 {
		res, err := e.accept(t, fmt.Sprint(i), e.req(fmt.Sprintf("user%d@example.com", i), "body"))
		if err != nil {
			t.Fatal(err)
		}
		ids[res.id] = true
	}
	fake := &fakeSender{}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for range 3 {
		gate := sender.NewWorker(e.pool, fake, unsub.Signer{}, "", 50, 10)
		d := NewDispatcher(e.pool, fake, gate, tracking.NewTracker([]byte(strings.Repeat("k", 32)), "http://localhost"))
		wg.Go(func() { d.Run(ctx) })
	}
	defer func() { cancel(); wg.Wait() }()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if e.value(t, `SELECT count(*)::text FROM transactional_messages WHERE workspace_id=$1 AND status='sent'`, e.ws) == "24" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	wg.Wait()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.calls != 24 || len(fake.sent) != 24 {
		t.Fatalf("provider calls %d sent %d", fake.calls, len(fake.sent))
	}
	seen := map[string]int{}
	for _, m := range fake.sent {
		if !ids[m.MessageID] {
			t.Fatal("message ID changed")
		}
		seen[m.MessageID]++
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatal("duplicate", id, n)
		}
	}
	if n := e.value(t, `SELECT trial_sent_count::text FROM workspaces WHERE id=$1`, e.ws); n != "24" {
		t.Fatal("duplicate quota", n)
	}
}

func TestTransactionalCrashRecoveryRejectsStaleClaim(t *testing.T) {
	e := setup(t, "trial", 2)
	res, err := e.accept(t, "crash", e.req("user@example.com", "body"))
	if err != nil {
		t.Fatal(err)
	}
	old, ok, err := claimOne(context.Background(), e.pool, res.id)
	if !ok || err != nil {
		t.Fatal(err)
	}
	dbtest.Exec(t, e.pool, `UPDATE transactional_messages SET claimed_at=clock_timestamp()-interval '6 minutes' WHERE id=$1`, res.id)
	if _, err := sweepStuck(context.Background(), e.pool); err != nil {
		t.Fatal(err)
	}
	fresh, ok, err := claimOne(context.Background(), e.pool, res.id)
	if !ok || err != nil || fresh.claimToken == old.claimToken {
		t.Fatal("lease not recovered", err)
	}
	fake := &fakeSender{}
	d := NewDispatcher(e.pool, fake, &openGate{}, tracking.NewTracker([]byte(strings.Repeat("k", 32)), "http://localhost"))
	d.process(context.Background(), old)
	if fake.calls != 0 {
		t.Fatal("stale worker sent")
	}
	if got := e.value(t, `SELECT claim_token::text FROM transactional_messages WHERE id=$1`, res.id); got != fresh.claimToken {
		t.Fatal("stale worker changed fresh claim")
	}
	d.process(context.Background(), fresh)
	if fake.calls != 1 || e.value(t, `SELECT trial_sent_count::text FROM workspaces WHERE id=$1`, e.ws) != "1" {
		t.Fatal("recovered message/quota duplicated")
	}
}
