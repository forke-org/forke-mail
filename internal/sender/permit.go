package sender

import (
	"context"
	"errors"
	"forkemail/internal/db"
	"log/slog"
	"sync"
	"time"
)

var ErrPermitExpired = errors.New("send_permit_expired")

type Permit struct {
	start  func(context.Context, db.Querier) error
	finish func(context.Context, bool)
}

func (p Permit) Start(ctx context.Context, q db.Querier) error {
	if p.start != nil {
		return p.start(ctx, q)
	}
	return nil
}
func (p Permit) Finish(ctx context.Context, consumed bool) {
	if p.finish != nil {
		p.finish(ctx, consumed)
	}
}

func (w *Worker) sharedPermit(id string) Permit {
	var once sync.Once
	return Permit{start: func(ctx context.Context, q db.Querier) error {
		tag, err := q.Exec(ctx, `UPDATE sending_permits SET state='sending',expires_at=clock_timestamp()+interval '5 minutes' WHERE id=$1 AND state='reserved' AND expires_at>clock_timestamp()`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrPermitExpired
		}
		return nil
	}, finish: func(ctx context.Context, consumed bool) {
		once.Do(func() {
			write, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			state := "released"
			if consumed {
				state = "used"
			}
			if _, err := w.pool.Exec(write, `UPDATE sending_permits SET state=$2,finished_at=clock_timestamp() WHERE id=$1 AND state IN ('reserved','sending')`, id, state); err != nil {
				slog.Error("finish sending permit failed", "err", err)
			}
			if consumed {
				w.mu.Lock()
				w.sentSinceCheck++
				w.mu.Unlock()
			}
		})
	}}
}

func (w *Worker) localPermit() Permit {
	var once sync.Once
	return Permit{finish: func(_ context.Context, consumed bool) {
		once.Do(func() {
			w.mu.Lock()
			defer w.mu.Unlock()
			if consumed {
				w.sentSinceCheck++
			}
			w.inFlight--
		})
	}}
}
