package sender

import (
	"context"
	"strings"
	"testing"
	"time"

	"forkemail/internal/unsub"
)

func TestUrgentSendsJumpAheadOfBulk(t *testing.T) {
	w := NewWorker(nil, nil, unsub.Signer{Secret: []byte(strings.Repeat("s", 32))}, "https://mail.test", 20, 20)
	ctx := context.Background()
	for w.global.Allow() {
	}
	order := make(chan string, 2)
	go func() {
		if p, err := w.waitBulk(ctx, "", 0); err == nil {
			p.Finish(ctx, false)
			order <- "bulk"
		}
	}()
	time.Sleep(5 * time.Millisecond)
	go func() {
		if p, err := w.AcquireUrgent(ctx); err == nil {
			p.Finish(ctx, false)
			order <- "urgent"
		}
	}()
	if first := <-order; first != "urgent" {
		t.Fatalf("%s went first", first)
	}
	if second := <-order; second != "bulk" {
		t.Fatalf("second %s", second)
	}
}

func TestUrgentRespectsHoldAndFullQuotaButNotHeadroom(t *testing.T) {
	w := NewWorker(nil, nil, unsub.Signer{Secret: []byte(strings.Repeat("s", 32))}, "https://mail.test", 1000, 1000)
	w.SetLimits(Limits{Max24HourSend: 100, SentLast24Hours: 95})
	if !w.idle() {
		t.Fatal("bulk should idle inside the 10% headroom")
	}
	if p, err := w.AcquireUrgent(context.Background()); err != nil {
		t.Fatalf("urgent blocked inside headroom: %v", err)
	} else {
		p.Finish(context.Background(), false)
	}
	w.SetLimits(Limits{Max24HourSend: 100, SentLast24Hours: 100})
	if _, err := w.AcquireUrgent(context.Background()); err != ErrSESQuotaExhausted {
		t.Fatalf("full quota: %v", err)
	}
	w.SetLimits(Limits{})
	w.HoldSending("test")
	if _, err := w.AcquireUrgent(context.Background()); err != ErrSendingHeld {
		t.Fatalf("hold: %v", err)
	}
}
