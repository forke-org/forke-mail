// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package workspaces

import (
	"context"
	"time"

	"forkemail/internal/db"
)

const WarmupDays = 14

var warmupCaps = [WarmupDays]int{200, 200, 500, 500, 1000, 1000, 1000, 2000, 2000, 2000, 5000, 5000, 5000, 5000}

type Warmup struct {
	Active    bool       `json:"active"`
	Day       int        `json:"day"`
	Cap       int        `json:"cap"`
	UsedToday int        `json:"used_today"`
	EndsAt    *time.Time `json:"ends_at"`
}

func (w Warmup) Remaining() int {
	return max(0, w.Cap-w.UsedToday)
}

func LoadWarmup(ctx context.Context, q db.Querier, ws string) (Warmup, error) {
	var w Warmup
	var until *time.Time
	if err := q.QueryRow(ctx, `
		SELECT warmup_until, COALESCE((SELECT SUM((SELECT COUNT(*) FROM email_recipients r WHERE r.email_id = e.id)) FROM emails e
			WHERE e.workspace_id = $1 AND e.started_at >= date_trunc('day', now())), 0)::int
		FROM workspaces WHERE id = $1`, ws).Scan(&until, &w.UsedToday); err != nil {
		return w, err
	}
	if until == nil || time.Now().After(*until) {
		return w, nil
	}
	day := int(time.Since(until.Add(-WarmupDays*24*time.Hour)).Hours() / 24)
	day = min(max(day, 0), WarmupDays-1)
	w.Active, w.Day, w.Cap, w.EndsAt = true, day+1, warmupCaps[day], until
	return w, nil
}
