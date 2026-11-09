package transactional

import (
	"context"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/sender"
	"strings"
	"testing"
	"time"
)

type changingGate struct {
	openGate
	change func()
}

func (g *changingGate) AcquireUrgent(context.Context) (sender.Permit, error) {
	g.change()
	return sender.Permit{}, nil
}

func TestEligibilityRecheckedAfterWaitingForSharedGate(t *testing.T) {
	for _, kind := range []string{"identity", "suppression"} {
		t.Run(kind, func(t *testing.T) {
			e := setup(t, "trial", 10)
			res, err := e.accept(t, "", e.req("user@example.com", "body"))
			if err != nil {
				t.Fatal(err)
			}
			s := &fakeSender{}
			gate := &changingGate{change: func() {
				if kind == "identity" {
					dbtest.Exec(t, e.pool, `UPDATE domains SET status='pending' WHERE workspace_id=$1`, e.ws)
				} else {
					dbtest.Exec(t, e.pool, `INSERT INTO suppressions(workspace_id,email,reason) VALUES($1,'user@example.com','manual')`, e.ws)
				}
			}}
			d := NewDispatcher(e.pool, s, gate, NewTracker([]byte(strings.Repeat("k", 32)), "http://localhost"))
			j, ok, err := claimOne(context.Background(), e.pool, res.id)
			if !ok || err != nil {
				t.Fatal("claim failed", err)
			}
			d.process(context.Background(), j)
			if s.calls != 0 || e.value(t, `SELECT status FROM transactional_messages WHERE id=$1`, res.id) != "failed" {
				t.Fatal("ineligible queued message sent")
			}
			if e.value(t, `SELECT trial_sent_count::text FROM workspaces WHERE id=$1`, e.ws) != "0" {
				t.Fatal("definitive rejection retained quota")
			}
		})
	}
}

func TestPollingRecoversDurableQueueWithoutNotification(t *testing.T) {
	e := setup(t, "paid", 0)
	res, err := e.accept(t, "", e.req("user@example.com", "after restart"))
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSender{}
	d := NewDispatcher(e.pool, s, &openGate{}, NewTracker([]byte(strings.Repeat("k", 32)), "http://localhost"))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { d.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if e.value(t, `SELECT status FROM transactional_messages WHERE id=$1`, res.id) == "sent" {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.calls != 1 {
				t.Fatal("duplicate dispatch")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("queue did not recover after process restart")
}

func TestTransactionalMarketingOptOutDoesNotCreateContactsOrFooter(t *testing.T) {
	e := setup(t, "paid", 0)
	dbtest.Exec(t, e.pool, `INSERT INTO suppressions(workspace_id,email,reason) VALUES($1,'user@example.com','unsubscribed')`, e.ws)
	res, err := e.accept(t, "", e.req("user@example.com", "reset code"))
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSender{}
	e.dispatch(t, s, res.id)
	if s.calls != 1 || strings.Contains(s.sent[0].HTML, "/unsubscribe/") || s.sent[0].Headers["List-Unsubscribe"] != "" {
		t.Fatal("transactional mail has newsletter semantics")
	}
	if e.value(t, `SELECT count(*)::text FROM contacts WHERE workspace_id=$1`, e.ws) != "0" {
		t.Fatal("transactional created marketing contacts")
	}
	if _, ok, err := claimOne(context.Background(), e.pool, res.id); ok || err != nil {
		t.Fatal("sent message reclaimed")
	}
}
