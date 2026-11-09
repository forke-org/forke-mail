package transactional

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"forkemail/internal/auth"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/feedback"
	"forkemail/internal/sender"
	"forkemail/internal/workspaces"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeSender struct {
	mu    sync.Mutex
	sent  []sender.Message
	err   error
	calls int
}

func (f *fakeSender) Send(ctx context.Context, m sender.Message) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	f.sent = append(f.sent, m)
	return "ses-" + m.MessageID, nil
}

type openGate struct{ held []string }

func (g *openGate) AcquireUrgent(ctx context.Context) (sender.Permit, error) {
	return sender.Permit{}, nil
}
func (g *openGate) HoldSending(reason string) { g.held = append(g.held, reason) }

type env struct {
	pool *pgxpool.Pool
	ws   string
	from string
}

func setup(t *testing.T, plan string, trialLimit int) env {
	t.Helper()
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, plan, trialLimit)
	domain := "tx-" + ws[:8] + ".test"
	dbtest.Exec(t, pool, `INSERT INTO domains (workspace_id, name, kind, status) VALUES ($1, $2, 'domain', 'verified')`, ws, domain)
	return env{pool: pool, ws: ws, from: "no-reply@" + domain}
}

func (e env) req(to, body string) request {
	r := request{To: to, FromName: "Forke", FromAddress: e.from, Subject: "Reset your password", HTML: "<p>" + body + "</p>", Text: body}
	r.normalize()
	return r
}

func (e env) accept(t *testing.T, key string, r request) (accepted, error) {
	t.Helper()
	var res accepted
	err := pgx.BeginFunc(context.Background(), e.pool, func(tx pgx.Tx) error {
		var err error
		res, err = accept(context.Background(), tx, auth.Identity{WorkspaceID: e.ws}, key, r)
		return err
	})
	return res, err
}

func (e env) dispatch(t *testing.T, s sender.Sender, id string) *Dispatcher {
	t.Helper()
	d := NewDispatcher(e.pool, s, &openGate{}, NewTracker([]byte(strings.Repeat("k", 32)), "https://mail.test"))
	j, ok, err := claimOne(context.Background(), e.pool, id)
	if err != nil || !ok {
		t.Fatalf("claim %v %v", ok, err)
	}
	d.process(context.Background(), j)
	return d
}

func (e env) value(t *testing.T, sql string, args ...any) string {
	t.Helper()
	return dbtest.QueryString(t, e.pool, sql, args...)
}

func TestAcceptAndDispatchRecordsLatencyAndEvents(t *testing.T) {
	e := setup(t, "paid", 0)
	res, err := e.accept(t, "", e.req("user@x.io", "code 123456"))
	if err != nil || res.replayed != "" {
		t.Fatalf("accept %+v %v", res, err)
	}
	fake := &fakeSender{}
	e.dispatch(t, fake, res.id)
	if len(fake.sent) != 1 || fake.sent[0].To != "user@x.io" || fake.sent[0].Tags()["message_id"] != res.id {
		t.Fatalf("sent %+v", fake.sent)
	}
	if got := e.value(t, `SELECT status || ':' || ses_message_id || ':' || (sent_at >= created_at)::text FROM transactional_messages WHERE id = $1`, res.id); got != "sent:ses-"+res.id+":true" {
		t.Fatalf("message %s", got)
	}
	if got := e.value(t, `SELECT string_agg(type, ',' ORDER BY id) FROM email_events WHERE message_id = $1`, res.id); got != "queued,sent" {
		t.Fatalf("events %s", got)
	}
	if got := e.value(t, `SELECT (detail ? 'dispatch_ms')::text FROM email_events WHERE message_id = $1 AND type = 'sent'`, res.id); got != "true" {
		t.Fatal("sent event lacks dispatch_ms")
	}
}

func TestIdempotencyKeyReplaysAndRejectsDifferentBody(t *testing.T) {
	e := setup(t, "trial", 10)
	first, err := e.accept(t, "reset-42", e.req("user@x.io", "a"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := e.accept(t, "reset-42", e.req("user@x.io", "a"))
	if err != nil || again.id != first.id || again.replayed != replayedByKey {
		t.Fatalf("replay %+v %v", again, err)
	}
	if _, err := e.accept(t, "reset-42", e.req("user@x.io", "b")); !errors.Is(err, errKeyReused) {
		t.Fatalf("different body with same key: %v", err)
	}
	if got := e.value(t, `SELECT trial_sent_count::text FROM workspaces WHERE id = $1`, e.ws); got != "1" {
		t.Fatalf("quota charged %s times", got)
	}
	dbtest.Exec(t, e.pool, `UPDATE transactional_messages SET created_at = now() - interval '25 hours' WHERE id = $1`, first.id)
	fresh, err := e.accept(t, "reset-42", e.req("user@x.io", "b"))
	if err != nil || fresh.id == first.id || fresh.replayed != "" {
		t.Fatalf("expired key not reusable: %+v %v", fresh, err)
	}
}

func TestDuplicateContentWithoutKeyIsDeduplicated(t *testing.T) {
	e := setup(t, "paid", 0)
	first, _ := e.accept(t, "", e.req("user@x.io", "receipt 1"))
	dup, err := e.accept(t, "", e.req("USER@x.io", "receipt 1"))
	if err != nil || dup.id != first.id || dup.replayed != replayedByContent {
		t.Fatalf("duplicate %+v %v", dup, err)
	}
	other, _ := e.accept(t, "", e.req("user@x.io", "receipt 2"))
	keyed, _ := e.accept(t, "explicit-key", e.req("user@x.io", "receipt 1"))
	if other.id == first.id || keyed.id == first.id {
		t.Fatal("distinct content or explicit key was deduplicated")
	}
	dbtest.Exec(t, e.pool, `UPDATE transactional_messages SET status = 'failed' WHERE id = $1`, first.id)
	retry, _ := e.accept(t, "", e.req("user@x.io", "receipt 1"))
	if retry.id == first.id {
		t.Fatal("a failed message blocked a resend")
	}
}

func TestConcurrentDuplicatesCreateOneMessage(t *testing.T) {
	e := setup(t, "paid", 0)
	var wg sync.WaitGroup
	ids := make(chan string, 20)
	for i := range 20 {
		key, body := "", "otp 111"
		if i%2 == 0 {
			key, body = "same-key", "otp 222"
		}
		wg.Go(func() {
			res, err := e.accept(t, key, e.req("racer@x.io", body))
			if err != nil {
				t.Error(err)
				return
			}
			ids <- res.id
		})
	}
	wg.Wait()
	close(ids)
	if got := e.value(t, `SELECT COUNT(*)::text FROM transactional_messages WHERE workspace_id = $1`, e.ws); got != "2" {
		t.Fatalf("%s messages created, want 2 (one per key mode)", got)
	}
}

func TestSuppressionsAndIdentityChecks(t *testing.T) {
	e := setup(t, "paid", 0)
	dbtest.Exec(t, e.pool, `INSERT INTO suppressions (workspace_id, email, reason, source) VALUES
		($1, 'bounced@x.io', 'bounced', 'ses_bounce'), ($1, 'unsub@x.io', 'unsubscribed', 'unsubscribe_link')`, e.ws)
	if _, err := e.accept(t, "", e.req("Bounced@x.io", "hi")); !errors.Is(err, errSuppressed) {
		t.Errorf("bounced address: %v", err)
	}
	if _, err := e.accept(t, "", e.req("unsub@x.io", "hi")); err != nil {
		t.Errorf("unsubscribed address must still get transactional mail: %v", err)
	}
	r := e.req("ok@x.io", "hi")
	r.FromAddress = "no-reply@unverified.test"
	if _, err := e.accept(t, "", r); !errors.Is(err, errDomainUnverified) {
		t.Errorf("unverified domain: %v", err)
	}
	dbtest.Exec(t, e.pool, `UPDATE workspaces SET sending_paused = true, pause_reason = 'bounce_rate_exceeded' WHERE id = $1`, e.ws)
	if _, err := e.accept(t, "", e.req("ok@x.io", "hi")); !errors.Is(err, errSendingPaused) {
		t.Errorf("paused workspace: %v", err)
	}
}

func TestTrialQuotaEnforcedAtAccept(t *testing.T) {
	e := setup(t, "trial", 1)
	if _, err := e.accept(t, "", e.req("a@x.io", "1")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.accept(t, "", e.req("b@x.io", "2")); !errors.Is(err, workspaces.ErrTrialQuotaExhausted) {
		t.Fatalf("second send: %v", err)
	}
	if got := e.value(t, `SELECT COUNT(*)::text FROM transactional_messages WHERE workspace_id = $1`, e.ws); got != "1" {
		t.Fatalf("rejected send was stored: %s", got)
	}
}

func TestSendFailureHandling(t *testing.T) {
	cases := []struct {
		action    sender.Action
		status    string
		trialUsed string
	}{
		{sender.Retry, "queued", "1"},
		{sender.Fail, "failed", "0"},
		{sender.PauseIdentity, "failed", "0"},
		{sender.PauseSending, "queued", "1"},
		{sender.UnknownOutcome, "failed", "1"},
	}
	for _, c := range cases {
		e := setup(t, "trial", 10)
		res, err := e.accept(t, "", e.req("user@x.io", "x"))
		if err != nil {
			t.Fatal(err)
		}
		e.dispatch(t, &fakeSender{err: &sender.SendError{Action: c.action, Reason: "boom", Err: errors.New("x")}}, res.id)
		if got := e.value(t, `SELECT status FROM transactional_messages WHERE id = $1`, res.id); got != c.status {
			t.Errorf("action %d: status %s, want %s", c.action, got, c.status)
		}
		if got := e.value(t, `SELECT trial_sent_count::text FROM workspaces WHERE id = $1`, e.ws); got != c.trialUsed {
			t.Errorf("action %d: trial used %s, want %s", c.action, got, c.trialUsed)
		}
		if got := e.value(t, `SELECT COUNT(*)::text FROM suppressions WHERE workspace_id = $1`, e.ws); got != "0" {
			t.Errorf("action %d: send error created a suppression", c.action)
		}
	}
}

func TestRetriesGiveUpAndExpiredMessagesFail(t *testing.T) {
	e := setup(t, "paid", 0)
	res, _ := e.accept(t, "", e.req("user@x.io", "x"))
	dbtest.Exec(t, e.pool, `UPDATE transactional_messages SET attempts = $2 WHERE id = $1`, res.id, maxAttempts-1)
	e.dispatch(t, &fakeSender{err: &sender.SendError{Action: sender.Retry, Reason: "throttled", Err: errors.New("x")}}, res.id)
	if got := e.value(t, `SELECT status || ':' || last_error FROM transactional_messages WHERE id = $1`, res.id); got != "failed:throttled" {
		t.Errorf("after max attempts %s", got)
	}
	old, _ := e.accept(t, "", e.req("user@x.io", "y"))
	dbtest.Exec(t, e.pool, `UPDATE transactional_messages SET created_at = now() - interval '2 hours' WHERE id = $1`, old.id)
	fake := &fakeSender{}
	e.dispatch(t, fake, old.id)
	if fake.calls != 0 || e.value(t, `SELECT last_error FROM transactional_messages WHERE id = $1`, old.id) != "expired" {
		t.Error("expired message was sent")
	}
}

func TestStuckSendingIsFailedNotResent(t *testing.T) {
	e := setup(t, "paid", 0)
	res, _ := e.accept(t, "", e.req("user@x.io", "x"))
	dbtest.Exec(t, e.pool, `UPDATE transactional_messages SET status = 'sending', claimed_at = now() - interval '10 minutes' WHERE id = $1`, res.id)
	if _, err := sweepStuck(context.Background(), e.pool); err != nil {
		t.Fatal(err)
	}
	if got := e.value(t, `SELECT status || ':' || last_error FROM transactional_messages WHERE id = $1`, res.id); got != "failed:unknown_outcome" {
		t.Fatalf("stuck message %s", got)
	}
}

func TestLogSenderFeedbackReachesTransactionalMessages(t *testing.T) {
	e := setup(t, "paid", 0)
	log := sender.LogSender{Pool: e.pool, Feedback: &feedback.Processor{Pool: e.pool, Reputation: feedback.Thresholds{MaxBounceRate: 1, MaxComplaintRate: 1, MinSample: 1000}}}
	ok, _ := e.accept(t, "", e.req("someone@x.io", "x"))
	bounce, _ := e.accept(t, "", e.req("bounce@simulator.amazonses.com", "x"))
	e.dispatch(t, log, ok.id)
	e.dispatch(t, log, bounce.id)
	if got := e.value(t, `SELECT (delivered_at IS NOT NULL)::text FROM transactional_messages WHERE id = $1`, ok.id); got != "true" {
		t.Error("delivery not recorded")
	}
	if got := e.value(t, `SELECT (bounced_at IS NOT NULL)::text || ':' || (SELECT source FROM suppressions WHERE workspace_id = $2) FROM transactional_messages WHERE id = $1`, bounce.id, e.ws); got != "true:ses_bounce" {
		t.Errorf("bounce %s", got)
	}
	if got := e.value(t, `SELECT string_agg(type, ',' ORDER BY id) FROM email_events WHERE message_id = $1`, bounce.id); got != "queued,sent,bounced" {
		t.Errorf("events %s", got)
	}
	if got := e.value(t, `SELECT COUNT(*)::text FROM message_logs WHERE message_id = $1`, ok.id); got != "1" {
		t.Errorf("message log rows %s", got)
	}
}

func TestOpenAndClickTracking(t *testing.T) {
	e := setup(t, "paid", 0)
	r := e.req("user@x.io", "x")
	r.HTML = `<a href="https://forke.space/r">go</a>`
	r.TrackOpens, r.TrackClicks = true, true
	res, _ := e.accept(t, "", r)
	fake := &fakeSender{}
	d := e.dispatch(t, fake, res.id)
	h := &TrackHandler{Pool: e.pool, Tracker: d.tracker}
	ctx := context.Background()
	for range 3 {
		if err := h.recordOpen(ctx, res.id); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.recordClick(ctx, res.id, "https://forke.space/r"); err != nil {
		t.Fatal(err)
	}
	if got := e.value(t, `SELECT open_count || ':' || click_count FROM transactional_messages WHERE id = $1`, res.id); got != "3:1" {
		t.Errorf("counts %s", got)
	}
	if got := e.value(t, `SELECT string_agg(type, ',' ORDER BY id) FROM email_events WHERE message_id = $1`, res.id); got != "queued,sent,opened,clicked" {
		t.Errorf("events %s", got)
	}
	if !strings.Contains(fake.sent[0].HTML, "https://mail.test/t/c/") || !strings.Contains(fake.sent[0].HTML, "https://mail.test/t/o/") {
		t.Error("sent html not instrumented")
	}
	if got := e.value(t, `SELECT html FROM transactional_messages WHERE id = $1`, res.id); got != r.HTML {
		t.Error("stored html was modified")
	}
}
