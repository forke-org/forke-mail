package sender

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"forkemail/internal/unsub"
	"forkemail/internal/workspaces"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"
)

type Worker struct {
	pool       *pgxpool.Pool
	sender     Sender
	signer     unsub.Signer
	baseURL    string
	globalRate float64
	trialRate  float64
	global     *rate.Limiter

	mu             sync.Mutex
	lanes          map[string]bool
	limits         Limits
	sentSinceCheck int
	holdUntil      time.Time
	quotaLogged    time.Time
	wg             sync.WaitGroup
}

type lane struct {
	workspaceID string
	perSecond   float64
}

func NewWorker(pool *pgxpool.Pool, s Sender, signer unsub.Signer, baseURL string, globalRate, trialRate float64) *Worker {
	return &Worker{
		pool:       pool,
		sender:     s,
		signer:     signer,
		baseURL:    baseURL,
		globalRate: globalRate,
		trialRate:  trialRate,
		global:     rate.NewLimiter(rate.Limit(globalRate), max(1, int(globalRate))),
		lanes:      make(map[string]bool),
	}
}

func (w *Worker) Run(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		w.tick(ctx)
		select {
		case <-ctx.Done():
			w.wg.Wait()
			return
		case <-t.C:
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	if w.idle() {
		return
	}
	if _, err := w.pool.Exec(ctx, `
		UPDATE emails e SET status = 'done', finished_at = now()
		WHERE e.status = 'sending' AND NOT EXISTS (
			SELECT 1 FROM email_recipients r
			WHERE r.email_id = e.id AND r.status IN ('pending', 'sending'))`); err != nil && ctx.Err() == nil {
		slog.Error("finish emails failed", "err", err)
		return
	}
	rows, err := w.pool.Query(ctx, `
		SELECT DISTINCT ws.id, ws.plan, ws.rate_limit_per_sec
		FROM emails c JOIN workspaces ws ON ws.id = c.workspace_id
		WHERE c.status = 'sending' AND ws.status = 'active' AND NOT ws.sending_paused`)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("list sending workspaces failed", "err", err)
		}
		return
	}
	w.mu.Lock()
	globalRate := w.globalRate
	w.mu.Unlock()
	var lanes []lane
	for rows.Next() {
		var id, plan string
		var configured int
		if err := rows.Scan(&id, &plan, &configured); err != nil {
			rows.Close()
			slog.Error("scan workspace failed", "err", err)
			return
		}
		lanes = append(lanes, lane{workspaceID: id, perSecond: workspaces.SendRate(plan, configured, globalRate, w.trialRate)})
	}
	rows.Close()
	for _, l := range lanes {
		w.startLane(ctx, l)
	}
}

func (w *Worker) startLane(ctx context.Context, l lane) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lanes[l.workspaceID] {
		return
	}
	w.lanes[l.workspaceID] = true
	w.wg.Add(1)
	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.lanes, l.workspaceID)
			w.mu.Unlock()
			w.wg.Done()
		}()
		w.runLane(ctx, l)
	}()
}

func (w *Worker) runLane(ctx context.Context, l lane) {
	limiter := rate.NewLimiter(rate.Limit(l.perSecond), 1)
	for ctx.Err() == nil && !w.idle() {
		batch, err := Claim(ctx, w.pool, l.workspaceID, max(1, int(l.perSecond)))
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("claim failed", "workspace_id", l.workspaceID, "err", err)
			}
			return
		}
		if len(batch) == 0 {
			return
		}
		for i, rec := range batch {
			if !w.process(ctx, limiter, rec) {
				w.unclaim(ctx, batch[i:])
				return
			}
		}
	}
}

func (w *Worker) unclaim(ctx context.Context, batch []Recipient) {
	ids := make([]string, len(batch))
	for i, r := range batch {
		ids[i] = r.ID
	}
	unclaimCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := Unclaim(unclaimCtx, w.pool, ids); err != nil {
		slog.Error("unclaim failed", "err", err)
	}
}
