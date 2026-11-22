// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package feedback

import (
	"context"
	"fmt"
	"testing"
)

func TestEvaluate(t *testing.T) {
	th := Thresholds{MaxBounceRate: 0.05, MaxComplaintRate: 0.001, MinSample: 100}
	cases := []struct {
		s    Stats
		want string
	}{
		{Stats{Sent: 99, Bounced: 50}, ""},
		{Stats{Sent: 100, Bounced: 5}, ""},
		{Stats{Sent: 100, Bounced: 6}, "bounce_rate_exceeded"},
		{Stats{Sent: 1000, Complained: 1}, ""},
		{Stats{Sent: 1000, Complained: 2}, "complaint_rate_exceeded"},
		{Stats{}, ""},
	}
	for _, c := range cases {
		if got := th.Evaluate(c.s); got != c.want {
			t.Errorf("%+v = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestReputationGuardPausesWorkspace(t *testing.T) {
	f := setup(t)
	f.p.Reputation = Thresholds{MaxBounceRate: 0.05, MaxComplaintRate: 0.001, MinSample: 20}
	sending := f.email
	exec := func(sql string, args ...any) {
		if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE emails SET status = 'sending', started_at = now() WHERE id = $1`, sending)
	var ids, mids []string
	for i := 0; i < 19; i++ {
		rid, mid := f.recipient(t, fmt.Sprintf("r%d@x.io", i))
		ids, mids = append(ids, rid), append(mids, mid)
	}
	for i := 0; i < 3; i++ {
		f.handle(t, "", f.event("Bounce", mids[i], ids[i], bounce("Permanent", "General")))
	}
	if got := f.count(t, `SELECT sending_paused::text FROM workspaces WHERE id = $1`, f.ws); got != "false" {
		t.Fatal("paused below the minimum sample")
	}
	rid, mid := f.recipient(t, "last@x.io")
	f.handle(t, "", f.event("Bounce", mid, rid, bounce("Permanent", "General")))
	if got := f.count(t, `SELECT sending_paused::text || ':' || pause_reason FROM workspaces WHERE id = $1`, f.ws); got != "true:bounce_rate_exceeded" {
		t.Fatalf("workspace = %s", got)
	}
	if got := f.count(t, `SELECT status || ':' || pause_reason FROM emails WHERE id = $1`, sending); got != "paused:reputation" {
		t.Fatalf("email = %s", got)
	}
	if got := f.count(t, `SELECT COUNT(*)::text FROM email_events WHERE workspace_id = $1 AND type = 'sending_paused'`, f.ws); got != "1" {
		t.Fatalf("sending_paused events = %s", got)
	}
}

func TestWarn(t *testing.T) {
	th := Thresholds{MaxBounceRate: 0.05, WarnBounceRate: 0.02, MaxComplaintRate: 0.001, WarnComplaintRate: 0.0005, MinSample: 100}
	cases := []struct {
		s    Stats
		want string
	}{
		{Stats{Sent: 50, Bounced: 5}, ""},
		{Stats{Sent: 100, Bounced: 2}, ""},
		{Stats{Sent: 100, Bounced: 3}, "bounce_rate_high"},
		{Stats{Sent: 2000, Complained: 1}, ""},
		{Stats{Sent: 2000, Complained: 2}, "complaint_rate_high"},
	}
	for _, c := range cases {
		if got := th.Warn(c.s); got != c.want {
			t.Errorf("%+v = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestReputationWarningBeforePause(t *testing.T) {
	f := setup(t)
	f.p.Reputation = Thresholds{MaxBounceRate: 0.5, WarnBounceRate: 0.05, MaxComplaintRate: 0.5, MinSample: 20}
	var ids, mids []string
	for i := 0; i < 20; i++ {
		rid, mid := f.recipient(t, fmt.Sprintf("w%d@x.io", i))
		ids, mids = append(ids, rid), append(mids, mid)
	}
	for i := 0; i < 3; i++ {
		f.handle(t, "", f.event("Bounce", mids[i], ids[i], bounce("Permanent", "General")))
	}
	if got := f.count(t, `SELECT sending_paused::text || ':' || (reputation_warned_at IS NOT NULL)::text FROM workspaces WHERE id = $1`, f.ws); got != "false:true" {
		t.Fatalf("workspace = %s", got)
	}
	if got := f.count(t, `SELECT COUNT(*)::text || ':' || MIN(kind) || ':' || MIN(detail->>'reason') FROM workspace_alerts WHERE workspace_id = $1`, f.ws); got != "1:reputation_warning:bounce_rate_high" {
		t.Fatalf("alerts = %s", got)
	}
	if got := f.count(t, `SELECT COUNT(*)::text FROM email_events WHERE workspace_id = $1 AND type = 'reputation_warning'`, f.ws); got != "1" {
		t.Fatalf("warning events = %s", got)
	}
	f.handle(t, "", f.event("Bounce", mids[5], ids[5], bounce("Permanent", "General")))
	if got := f.count(t, `SELECT COUNT(*)::text FROM workspace_alerts WHERE workspace_id = $1`, f.ws); got != "1" {
		t.Fatalf("warned twice in a day: %s", got)
	}
}
