package webhooks

import (
	"context"
	"fmt"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/events"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTwoWebhookWorkersClaimEachAttemptOnce(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	v := testVault(t)
	secret := "local-multi-secret"
	for i := range 8 {
		dbtest.Exec(t, pool, `INSERT INTO webhook_endpoints(workspace_id,url,secret,events,enabled) VALUES($1,$2,$3,ARRAY['email.delivered'],true)`, ws, fmt.Sprintf("https://hooks%d.example.com", i), v.Seal(secret, ws))
	}
	if err := events.Record(context.Background(), pool, events.Event{WorkspaceID: ws, Type: "delivered"}); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	seen := map[string]int{}
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if !Verify(secret, r.Header.Get("Webhook-Id"), r.Header.Get("Webhook-Timestamp"), r.Header.Get("Webhook-Signature"), body, time.Now()) {
			t.Error("signature invalid")
		}
		mu.Lock()
		seen[r.Header.Get("Webhook-Id")]++
		mu.Unlock()
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	var wg sync.WaitGroup
	for range 2 {
		d := &Dispatcher{Pool: pool, Vault: v, Client: client}
		wg.Go(func() {
			for {
				ok, err := d.Once(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				if !ok {
					return
				}
			}
		})
	}
	wg.Wait()
	if len(seen) != 8 {
		t.Fatal("missing deliveries", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatal("duplicate attempt", id, n)
		}
	}
	if dbtest.QueryString(t, pool, `SELECT count(*)::text FROM webhook_deliveries WHERE workspace_id=$1 AND status='delivered' AND attempts=1`, ws) != "8" {
		t.Fatal("attempt accounting wrong")
	}
}

func TestWebhookExpiredClaimRecoveredAndOldOwnerCannotSend(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	v := testVault(t)
	dbtest.Exec(t, pool, `INSERT INTO webhook_endpoints(workspace_id,url,secret,events,enabled) VALUES($1,'https://hooks.example.com',$2,ARRAY['email.delivered'],true)`, ws, v.Seal("local-secret", ws))
	if err := events.Record(context.Background(), pool, events.Event{WorkspaceID: ws, Type: "delivered"}); err != nil {
		t.Fatal(err)
	}
	count := 0
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		count++
		return &http.Response{StatusCode: 204, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	old := &Dispatcher{Pool: pool, Vault: v, Client: client}
	j, err := old.claim(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dbtest.Exec(t, pool, `UPDATE webhook_deliveries SET claimed_at=clock_timestamp()-interval '1 minute' WHERE id=$1`, j.id)
	dbtest.Exec(t, pool, `UPDATE webhook_endpoints SET next_dispatch_at=clock_timestamp() WHERE workspace_id=$1`, ws)
	fresh := &Dispatcher{Pool: pool, Vault: v, Client: client}
	if ok, err := fresh.Once(context.Background()); !ok || err != nil {
		t.Fatal("recovery failed", err)
	}
	if _, err := old.deliver(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("expired owner delivered again")
	}
	if got := dbtest.QueryString(t, pool, `SELECT status||':'||attempts::text FROM webhook_deliveries WHERE id=$1`, j.id); got != "delivered:2" {
		t.Fatal("recovery state", got)
	}
}
