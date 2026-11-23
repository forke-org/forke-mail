// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package events

import (
	"fmt"
	"testing"
	"time"

	"forkemail/internal/db/dbtest"
)

func TestComponentJudgement(t *testing.T) {
	th := threshold{time.Minute, 10 * time.Minute}
	for lag, want := range map[float64]string{5: "operational", 120: "degraded", 900: "down"} {
		if got := th.judge(&lag); got != want {
			t.Fatalf("lag %v: %s, want %s", lag, got, want)
		}
	}
	if th.judge(nil) != "operational" {
		t.Fatal("idle component is not operational")
	}
	if got := worst([]Component{{Status: "degraded"}, {Status: "not_enabled"}, {Status: "operational"}}); got != "degraded" {
		t.Fatal(got)
	}
}

func TestIdleBroadcastsAreOperational(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	email := dbtest.DraftEmail(t, pool, ws)
	dbtest.SentRecipient(t, pool, ws, email, "idle@example.com", "idle-"+ws)
	dbtest.Exec(t, pool, `UPDATE email_recipients SET sent_at = now() - interval '3 hours' WHERE email_id = $1`, email)
	if dbtest.QueryString(t, pool, `SELECT COUNT(*)::text FROM email_recipients WHERE status = 'pending'`) != "0" {
		t.Skip("another test has broadcasts pending")
	}
	list, err := (&PublicStatus{Pool: pool}).components(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if list[1].ID != "broadcasts" || list[1].Status != "operational" || list[1].LagSeconds != nil {
		t.Fatalf("idle broadcasts judged %+v", list[1])
	}
}

func TestQuietEventsAreOperationalWhenEverythingIsReported(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	email := dbtest.DraftEmail(t, pool, ws)
	for i := range 6 {
		dbtest.SentRecipient(t, pool, ws, email, fmt.Sprintf("r%d@example.com", i), fmt.Sprintf("quiet-%s-%d", ws, i))
	}
	dbtest.Exec(t, pool, `UPDATE email_recipients SET sent_at = now() - interval '30 minutes', delivered_at = now() - interval '29 minutes' WHERE email_id = $1`, email)
	if dbtest.QueryString(t, pool, `SELECT COUNT(*)::text FROM email_recipients WHERE sent_at BETWEEN now() - interval '1 hour' AND now() - interval '10 minutes' AND delivered_at IS NULL AND bounced_at IS NULL AND complained_at IS NULL`) != "0" {
		t.Skip("another test has unreported sends")
	}
	list, err := (&PublicStatus{Pool: pool}).components(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.ID == "email_events" && c.Status != "operational" {
			t.Fatalf("all sends reported but email events judged %s", c.Status)
		}
	}
}
