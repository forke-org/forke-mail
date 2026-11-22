// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package events

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PublicStatus struct {
	Pool             *pgxpool.Pool
	TTL              time.Duration
	MaxBounceRate    float64
	MaxComplaintRate float64
	SMTPEnabled      bool

	mu      sync.Mutex
	cached  []byte
	expires time.Time
}

type statusWindow struct {
	Sent               int      `json:"sent"`
	Delivered          int      `json:"delivered"`
	Bounced            int      `json:"bounced"`
	Complained         int      `json:"complained"`
	DeliveryRate       *float64 `json:"delivery_rate"`
	BounceRate         *float64 `json:"bounce_rate"`
	ComplaintRate      *float64 `json:"complaint_rate"`
	DeliverySecondsP50 *float64 `json:"delivery_seconds_p50"`
	DeliverySecondsP95 *float64 `json:"delivery_seconds_p95"`
}

type statusReport struct {
	Status      string                  `json:"status"`
	GeneratedAt time.Time               `json:"generated_at"`
	Windows     map[string]statusWindow `json:"windows"`
	Queue       struct {
		BroadcastPending    int `json:"broadcast_pending"`
		TransactionalQueued int `json:"transactional_queued"`
	} `json:"queue"`
	Limits struct {
		MaxBounceRate    float64 `json:"max_bounce_rate"`
		MaxComplaintRate float64 `json:"max_complaint_rate"`
	} `json:"limits"`
	Infrastructure string      `json:"infrastructure"`
	Components     []Component `json:"components"`
}

const windowQuery = `
	WITH sends AS (
		SELECT sent_at, delivered_at, bounced_at, complained_at FROM email_recipients WHERE sent_at >= now() - $1::interval
		UNION ALL
		SELECT sent_at, delivered_at, bounced_at, complained_at FROM transactional_messages WHERE sent_at >= now() - $1::interval
	)
	SELECT COUNT(*), COUNT(delivered_at), COUNT(bounced_at), COUNT(complained_at),
		percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM delivered_at - sent_at)) FILTER (WHERE delivered_at IS NOT NULL),
		percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM delivered_at - sent_at)) FILTER (WHERE delivered_at IS NOT NULL)
	FROM sends`

func (p *PublicStatus) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=60")
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cached == nil || time.Now().After(p.expires) {
		body, err := p.build(r.Context())
		if err != nil {
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "error": "metrics_unavailable"})
			return
		}
		p.cached, p.expires = body, time.Now().Add(p.ttl())
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(p.cached)
}

func (p *PublicStatus) ttl() time.Duration {
	if p.TTL > 0 {
		return p.TTL
	}
	return time.Minute
}

func (p *PublicStatus) build(ctx context.Context) ([]byte, error) {
	rep := statusReport{Status: "operational", GeneratedAt: time.Now().UTC(), Windows: map[string]statusWindow{}, Infrastructure: "Amazon SES"}
	rep.Limits.MaxBounceRate, rep.Limits.MaxComplaintRate = p.MaxBounceRate, p.MaxComplaintRate
	for name, interval := range map[string]string{"24h": "24 hours", "7d": "7 days", "30d": "30 days"} {
		var win statusWindow
		if err := p.Pool.QueryRow(ctx, windowQuery, interval).Scan(&win.Sent, &win.Delivered, &win.Bounced, &win.Complained, &win.DeliverySecondsP50, &win.DeliverySecondsP95); err != nil {
			return nil, err
		}
		if win.Sent > 0 {
			win.DeliveryRate = ratio(win.Delivered, win.Sent)
			win.BounceRate = ratio(win.Bounced, win.Sent)
			win.ComplaintRate = ratio(win.Complained, win.Sent)
		}
		rep.Windows[name] = win
	}
	if err := p.Pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM email_recipients WHERE status IN ('pending', 'sending')),
			(SELECT COUNT(*) FROM transactional_messages WHERE status IN ('queued', 'sending'))`,
	).Scan(&rep.Queue.BroadcastPending, &rep.Queue.TransactionalQueued); err != nil {
		return nil, err
	}
	list, err := p.components(ctx)
	if err != nil {
		return nil, err
	}
	rep.Components, rep.Status = list, worst(list)
	return json.Marshal(rep)
}

func ratio(part, whole int) *float64 {
	v := float64(part) / float64(whole)
	return &v
}
