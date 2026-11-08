package webhooks

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/events"
	"forkemail/internal/feedback"
	"github.com/jackc/pgx/v5"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDurableWebhookFlow(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	v := testVault(t)
	secret := "test-signing-secret"
	endpoint := dbtest.QueryString(t, pool, `INSERT INTO webhook_endpoints(workspace_id,url,secret,events,enabled) VALUES($1,'https://hooks.example.com/event',$2,ARRAY['email.delivered'],true) RETURNING id`, ws, v.Seal(secret, ws))
	email := dbtest.DraftEmail(t, pool, ws)
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		return events.Record(ctx, tx, events.Event{WorkspaceID: ws, EmailID: email, Type: "delivered"})
	})
	if err != nil {
		t.Fatal(err)
	}
	delivery := dbtest.QueryString(t, pool, `SELECT id FROM webhook_deliveries WHERE endpoint_id=$1 AND workspace_id=$2`, endpoint, ws)
	var seenID, seenBody string
	count := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		id := r.Header.Get("Webhook-Id")
		if !Verify(secret, id, r.Header.Get("Webhook-Timestamp"), r.Header.Get("Webhook-Signature"), body, time.Now()) {
			t.Error("invalid outgoing signature")
		}
		if count > 0 && (seenID != id || seenBody != string(body)) {
			t.Error("retry changed delivery identity or body")
		}
		seenID = id
		seenBody = string(body)
		count++
		code := 503
		if count > 1 {
			code = 204
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	d := &Dispatcher{Pool: pool, Vault: v, Client: client}
	if ok, err := d.Once(ctx); !ok || err != nil {
		t.Fatalf("first dispatch %v %v", ok, err)
	}
	if s := dbtest.QueryString(t, pool, `SELECT status FROM webhook_deliveries WHERE id=$1 AND workspace_id=$2`, delivery, ws); s != "pending" {
		t.Fatalf("retry state %s", s)
	}
	var next time.Time
	if err := pool.QueryRow(ctx, `SELECT next_attempt_at FROM webhook_deliveries WHERE id=$1 AND workspace_id=$2`, delivery, ws).Scan(&next); err != nil {
		t.Fatal(err)
	}
	if time.Until(next) < 3*time.Second {
		t.Fatal("backoff missing")
	}
	dbtest.Exec(t, pool, `UPDATE webhook_deliveries SET next_attempt_at=now() WHERE id=$1 AND workspace_id=$2`, delivery, ws)
	dbtest.Exec(t, pool, `UPDATE webhook_endpoints SET next_dispatch_at=now() WHERE id=$1 AND workspace_id=$2`, endpoint, ws)
	d = &Dispatcher{Pool: pool, Vault: v, Client: client}
	if ok, err := d.Once(ctx); !ok || err != nil {
		t.Fatalf("restart dispatch %v %v", ok, err)
	}
	if s := dbtest.QueryString(t, pool, `SELECT status FROM webhook_deliveries WHERE id=$1 AND workspace_id=$2`, delivery, ws); s != "delivered" {
		t.Fatalf("status %s", s)
	}
	if count != 2 || seenID != delivery {
		t.Fatal("wrong dispatch count")
	}
	if _, err := d.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("delivered job resent")
	}
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := events.Record(ctx, tx, events.Event{WorkspaceID: ws, EmailID: email, Type: "delivered"}); err != nil {
			return err
		}
		return context.Canceled
	})
	if err != context.Canceled {
		t.Fatal(err)
	}
	n := dbtest.QueryString(t, pool, `SELECT count(*)::text FROM webhook_deliveries WHERE endpoint_id=$1 AND workspace_id=$2`, endpoint, ws)
	if n != "1" {
		t.Fatal("rollback left a delivery")
	}
}

func TestConcurrentDispatchDoesNotDoubleClaim(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	v := testVault(t)
	dbtest.Exec(t, pool, `INSERT INTO webhook_endpoints(workspace_id,url,secret,events,enabled) VALUES($1,'https://hooks.example.com',$2,ARRAY['email.delivered'],true)`, ws, v.Seal("secret", ws))
	if err := events.Record(ctx, pool, events.Event{WorkspaceID: ws, Type: "delivered"}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	d := &Dispatcher{Pool: pool, Vault: v, Client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := d.Once(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("dispatch count %d", calls.Load())
	}
}

func TestSNSDeduplicationQueuesOneDelivery(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	v := testVault(t)
	endpoint := dbtest.QueryString(t, pool, `INSERT INTO webhook_endpoints(workspace_id,url,secret,events,enabled) VALUES($1,'https://hooks.example.com',$2,ARRAY['email.delivered'],true) RETURNING id`, ws, v.Seal("secret", ws))
	email := dbtest.DraftEmail(t, pool, ws)
	messageID := "message-" + ws
	dbtest.SentRecipient(t, pool, ws, email, "person@example.com", messageID)
	p := feedback.Processor{Pool: pool}
	raw := []byte(`{"eventType":"Delivery","mail":{"messageId":"` + messageID + `"}}`)
	for range 2 {
		if _, err := p.Handle(ctx, "sns-"+ws, raw); err != nil {
			t.Fatal(err)
		}
	}
	if n := dbtest.QueryString(t, pool, `SELECT count(*)::text FROM webhook_deliveries WHERE endpoint_id=$1 AND workspace_id=$2`, endpoint, ws); n != "1" {
		t.Fatalf("duplicate fanout %s", n)
	}
}

func TestRetryBoundAndLeaseRecovery(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	v := testVault(t)
	endpoint := dbtest.QueryString(t, pool, `INSERT INTO webhook_endpoints(workspace_id,url,secret,events,enabled) VALUES($1,'https://hooks.example.com',$2,ARRAY['email.failed'],true) RETURNING id`, ws, v.Seal("secret", ws))
	if err := events.Record(ctx, pool, events.Event{WorkspaceID: ws, Type: "failed"}); err != nil {
		t.Fatal(err)
	}
	dbtest.Exec(t, pool, `UPDATE webhook_deliveries SET status='processing',attempts=7,claimed_at=now()-interval '1 minute',lease_token=gen_random_uuid() WHERE endpoint_id=$1 AND workspace_id=$2`, endpoint, ws)
	d := &Dispatcher{Pool: pool, Vault: v, Client: &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}}
	if ok, err := d.Once(ctx); !ok || err != nil {
		t.Fatalf("recover %v %v", ok, err)
	}
	if status := dbtest.QueryString(t, pool, `SELECT status FROM webhook_deliveries WHERE endpoint_id=$1 AND workspace_id=$2`, endpoint, ws); status != "failed" {
		t.Fatal(status)
	}
	for i := 1; i <= 8; i++ {
		if retryDelay(i) < 5*time.Second || retryDelay(i) > time.Hour+time.Second {
			t.Fatal("bad retry " + strconv.Itoa(i))
		}
	}
}
