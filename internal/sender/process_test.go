package sender

import (
	"context"
	"errors"
	"strings"
	"testing"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/feedback"
	"forkemail/internal/unsub"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"
)

type fakeSender struct {
	calls int
	err   error
}

func (f *fakeSender) Send(ctx context.Context, m Message) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return "fake-" + m.RecipientID, nil
}

func sendOne(t *testing.T, pool *pgxpool.Pool, s Sender, ws, address string) (string, string, bool) {
	t.Helper()
	email := dbtest.DraftEmail(t, pool, ws)
	domain := ws + ".sender.test"
	dbtest.Exec(t, pool, `INSERT INTO domains(workspace_id,name,status) VALUES($1,$2,'verified') ON CONFLICT DO NOTHING`, ws, domain)
	dbtest.Exec(t, pool, `UPDATE emails SET from_address=$2 WHERE id=$1`, email, "sender@"+domain)
	dbtest.Exec(t, pool, `UPDATE emails SET status = 'sending', started_at = now() WHERE id = $1`, email)
	contact := dbtest.QueryString(t, pool, `INSERT INTO contacts (workspace_id, email) VALUES ($1, $2) RETURNING id::text`, ws, address)
	dbtest.Exec(t, pool, `INSERT INTO email_recipients (email_id, workspace_id, contact_id, email) VALUES ($1, $2, $3, $4)`, email, ws, contact, address)
	w := NewWorker(pool, s, unsub.Signer{Secret: []byte(strings.Repeat("s", 32))}, "https://mail.test", 1000, 1000)
	batch, err := Claim(context.Background(), pool, ws, 10)
	if err != nil || len(batch) != 1 {
		t.Fatalf("claim %d %v", len(batch), err)
	}
	cont := w.process(context.Background(), rate.NewLimiter(rate.Inf, 1), batch[0])
	return email, batch[0].ID, cont
}

func TestSendErrorsNeverSuppressAndUnknownIsNeverRetried(t *testing.T) {
	pool := dbtest.Pool(t)
	cases := []struct {
		action      Action
		reason      string
		status      string
		emailStatus string
		trialUsed   string
	}{
		{Retry, "throttled", "pending", "sending", "0"},
		{Fail, "message_rejected: Email address is not verified.", "failed", "sending", "0"},
		{PauseSending, "ses_sending_paused", "pending", "paused:ses_sending_paused", "0"},
		{PauseIdentity, "identity_not_verified", "failed", "paused:identity_not_verified", "0"},
		{UnknownOutcome, "unknown_outcome", "failed", "sending", "1"},
	}
	for _, c := range cases {
		ws := dbtest.Workspace(t, pool, "trial", 10)
		fake := &fakeSender{err: &SendError{Action: c.action, Reason: c.reason, Err: errors.New("aws said no")}}
		email, rid, _ := sendOne(t, pool, fake, ws, "person@x.io")
		if got := dbtest.QueryString(t, pool, `SELECT COUNT(*)::text FROM suppressions WHERE workspace_id = $1`, ws); got != "0" {
			t.Errorf("%s: api error created a suppression", c.reason)
		}
		if got := dbtest.QueryString(t, pool, `SELECT status FROM email_recipients WHERE id = $1`, rid); got != c.status {
			t.Errorf("%s: recipient status %s, want %s", c.reason, got, c.status)
		}
		if got := dbtest.QueryString(t, pool, `SELECT status || COALESCE(':' || pause_reason, '') FROM emails WHERE id = $1`, email); got != c.emailStatus {
			t.Errorf("%s: email %s, want %s", c.reason, got, c.emailStatus)
		}
		if got := dbtest.QueryString(t, pool, `SELECT trial_sent_count::text FROM workspaces WHERE id = $1`, ws); got != c.trialUsed {
			t.Errorf("%s: trial used %s, want %s", c.reason, got, c.trialUsed)
		}
		if c.action == UnknownOutcome {
			if got := dbtest.QueryString(t, pool, `SELECT last_error FROM email_recipients WHERE id = $1`, rid); got != "unknown_outcome" {
				t.Errorf("unknown outcome last_error %s", got)
			}
			if batch, _ := Claim(context.Background(), pool, ws, 10); len(batch) != 0 {
				t.Error("unknown outcome recipient was claimable again")
			}
		}
	}
}

func TestRetryGivesUpAfterMaxAttempts(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	_, rid, _ := sendOne(t, pool, &fakeSender{err: &SendError{Action: Retry, Reason: "throttled", Err: errors.New("x")}}, ws, "a@x.io")
	dbtest.Exec(t, pool, `UPDATE email_recipients SET attempts = $2, next_attempt_at = now() WHERE id = $1`, rid, maxAttempts-1)
	w := NewWorker(pool, &fakeSender{err: &SendError{Action: Retry, Reason: "throttled", Err: errors.New("x")}}, unsub.Signer{Secret: []byte(strings.Repeat("s", 32))}, "https://mail.test", 1000, 1000)
	batch, _ := Claim(context.Background(), pool, ws, 10)
	w.process(context.Background(), rate.NewLimiter(rate.Inf, 1), batch[0])
	if got := dbtest.QueryString(t, pool, `SELECT status FROM email_recipients WHERE id = $1`, rid); got != "failed" {
		t.Fatalf("status after max attempts %s", got)
	}
}

func TestSuppressionRecheckedBeforeSend(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	dbtest.Exec(t, pool, `INSERT INTO suppressions (workspace_id, email, reason, source) VALUES ($1, 'late@x.io', 'manual', 'manual')`, ws)
	fake := &fakeSender{}
	_, rid, _ := sendOne(t, pool, fake, ws, "LATE@x.io")
	if fake.calls != 0 {
		t.Fatal("sent to a suppressed address")
	}
	if got := dbtest.QueryString(t, pool, `SELECT status || ':' || last_error FROM email_recipients WHERE id = $1`, rid); got != "skipped:recipient_blocked" {
		t.Fatalf("recipient %s", got)
	}
}

func TestLogSenderEmitsSESEventsThroughProcessor(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	log := LogSender{Pool: pool, Feedback: &feedback.Processor{Pool: pool, Reputation: feedback.Thresholds{MaxBounceRate: 1, MaxComplaintRate: 1, MinSample: 1000}}}
	for address, want := range map[string]string{
		"success@simulator.amazonses.com":         "delivered:f:f",
		"bounce@simulator.amazonses.com":          "-:t:f",
		"suppressionlist@simulator.amazonses.com": "-:t:f",
		"complaint@simulator.amazonses.com":       "delivered:f:t",
		"someone@x.io":                            "delivered:f:f",
	} {
		_, rid, _ := sendOne(t, pool, log, ws, address)
		got := dbtest.QueryString(t, pool, `SELECT CASE WHEN delivered_at IS NULL THEN '-' ELSE 'delivered' END || ':' ||
			CASE WHEN bounced_at IS NULL THEN 'f' ELSE 't' END || ':' || CASE WHEN complained_at IS NULL THEN 'f' ELSE 't' END
			FROM email_recipients WHERE id = $1 AND status = 'sent' AND ses_message_id LIKE 'log-%'`, rid)
		if got != want {
			t.Errorf("%s: %s, want %s", address, got, want)
		}
	}
	if got := dbtest.QueryString(t, pool, `SELECT string_agg(source, ',' ORDER BY email) FROM suppressions WHERE workspace_id = $1`, ws); got != "ses_bounce,ses_complaint,ses_bounce" {
		t.Errorf("suppressions %s", got)
	}
}
