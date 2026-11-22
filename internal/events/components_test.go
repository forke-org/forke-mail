package events

import (
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
