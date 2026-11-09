package transactional

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"forkemail/internal/sender"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	dispatchConcurrency = 8
	wakeBuffer          = 4096
	pollInterval        = time.Second
	sweepInterval       = time.Minute
)

type Gate interface {
	AcquireUrgent(ctx context.Context) (sender.Permit, error)
	HoldSending(reason string)
}

type Dispatcher struct {
	pool    *pgxpool.Pool
	sender  sender.Sender
	gate    Gate
	tracker Tracker
	wake    chan string
	claimed chan job
}

func NewDispatcher(pool *pgxpool.Pool, s sender.Sender, gate Gate, tracker Tracker) *Dispatcher {
	return &Dispatcher{
		pool:    pool,
		sender:  s,
		gate:    gate,
		tracker: tracker,
		wake:    make(chan string, wakeBuffer),
		claimed: make(chan job),
	}
}

func (d *Dispatcher) Notify(id string) {
	if d == nil {
		return
	}
	select {
	case d.wake <- id:
	default:
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for range dispatchConcurrency {
		wg.Go(func() { d.work(ctx) })
	}
	poll := time.NewTicker(pollInterval)
	sweepTicker := time.NewTicker(sweepInterval)
	defer poll.Stop()
	defer sweepTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-poll.C:
			d.pollDue(ctx)
		case <-sweepTicker.C:
			d.sweep(ctx)
		}
	}
}

func (d *Dispatcher) work(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-d.wake:
			j, ok, err := claimOne(ctx, d.pool, id)
			if err != nil && ctx.Err() == nil {
				slog.Error("claim transactional message failed", "message_id", id, "err", err)
			}
			if ok {
				d.process(ctx, j)
			}
		case j := <-d.claimed:
			d.process(ctx, j)
		}
	}
}

func (d *Dispatcher) pollDue(ctx context.Context) {
	batch, err := claimDue(ctx, d.pool, dispatchConcurrency)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("claim due transactional messages failed", "err", err)
		}
		return
	}
	for i, j := range batch {
		select {
		case d.claimed <- j:
		case <-ctx.Done():
			for _, rest := range batch[i:] {
				d.requeue(context.WithoutCancel(ctx), rest, 0, "")
			}
			return
		}
	}
}

func (d *Dispatcher) sweep(ctx context.Context) {
	n, err := sweepStuck(ctx, d.pool)
	if err != nil {
		slog.Error("transactional sweeper failed", "err", err)
	} else if n > 0 {
		slog.Warn("transactional sweeper failed stuck messages", "count", n)
	}
}
