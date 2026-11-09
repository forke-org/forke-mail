package lifecycle

import (
	"context"
	"testing"

	"forkemail/internal/db/dbtest"

	"github.com/jackc/pgx/v5/pgxpool"
)

func contactID(t *testing.T, pool *pgxpool.Pool, ws, email string) string {
	return dbtest.QueryString(t, pool, `SELECT COALESCE((SELECT id::text FROM contacts WHERE workspace_id = $1 AND email = $2), 'gone')`, ws, email)
}

func TestTombstoneKeepsSuppressionAfterThirtyDays(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	dbtest.Exec(t, pool, `INSERT INTO contacts (workspace_id, email, status) VALUES
		($1, 'old@x.io', 'unsubscribed'), ($1, 'recent@x.io', 'bounced'), ($1, 'invalid@x.io', 'invalid'), ($1, 'active@x.io', 'active')`, ws)
	dbtest.Exec(t, pool, `UPDATE contacts SET status_changed_at = now() - interval '31 days' WHERE workspace_id = $1 AND email IN ('old@x.io', 'invalid@x.io', 'active@x.io')`, ws)
	dbtest.Exec(t, pool, `INSERT INTO suppressions (workspace_id, email, reason, source) VALUES ($1, 'old@x.io', 'unsubscribed', 'unsubscribe_link'), ($1, 'recent@x.io', 'bounced', 'ses_bounce')`, ws)
	if _, err := (Runner{Pool: pool}).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	for email, want := range map[string]bool{"old@x.io": true, "recent@x.io": false, "invalid@x.io": false, "active@x.io": false} {
		if gone := contactID(t, pool, ws, email) == "gone"; gone != want {
			t.Errorf("%s deleted=%v, want %v", email, gone, want)
		}
	}
	if n := dbtest.QueryString(t, pool, `SELECT count(*)::text FROM suppressions WHERE workspace_id = $1 AND email = 'old@x.io'`, ws); n != "1" {
		t.Fatal("suppression removed with the contact")
	}
}

func TestDormantContactsGetOneInviteThenUnsubscribe(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	ws := dbtest.Workspace(t, pool, "paid", 0)
	domain := ws[:8] + ".lifecycle.test"
	dbtest.Exec(t, pool, `UPDATE workspaces SET daily_limit = 1000 WHERE id = $1`, ws)
	dbtest.Exec(t, pool, `INSERT INTO domains (workspace_id, name, kind, status) VALUES ($1, $2, 'domain', 'verified')`, ws, domain)
	dbtest.Exec(t, pool, `INSERT INTO reengagement_settings (workspace_id, enabled, from_address, subject, html) VALUES ($1, true, $2, 'Still want Forke updates?', '<a href="https://forke.space/stay">Yes</a>')`, ws, "hello@"+domain)
	dbtest.Exec(t, pool, `INSERT INTO contacts (workspace_id, email, last_engaged_at) VALUES
		($1, 'dormant@x.io', NULL), ($1, 'clicker@x.io', now() - interval '5 days'), ($1, 'newbie@x.io', NULL)`, ws)
	for i := 0; i < 3; i++ {
		email := dbtest.DraftEmail(t, pool, ws)
		dbtest.Exec(t, pool, `INSERT INTO email_recipients (email_id, workspace_id, contact_id, email, status, sent_at)
			SELECT $1, $2, id, email, 'sent', CASE WHEN email = 'newbie@x.io' THEN now() - interval '10 days' ELSE now() - interval '200 days' END
			FROM contacts WHERE workspace_id = $2`, email, ws)
	}
	r := Runner{Pool: pool}
	rep, err := r.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Invited < 1 {
		t.Fatalf("report %+v", rep)
	}
	invited := dbtest.QueryString(t, pool, `SELECT string_agg(email, ',' ORDER BY email) FROM contacts WHERE workspace_id = $1 AND reengage_sent_at IS NOT NULL`, ws)
	if invited != "dormant@x.io" {
		t.Fatalf("invited %q", invited)
	}
	if got := dbtest.QueryString(t, pool, `SELECT e.status || ':' || count(r.*) FROM emails e JOIN campaigns c ON c.id = e.campaign_id JOIN email_recipients r ON r.email_id = e.id WHERE e.workspace_id = $1 AND c.name = 'Re-engagement' GROUP BY e.status`, ws); got != "sending:1" {
		t.Fatalf("re-engagement email %q", got)
	}
	if _, err := r.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if n := dbtest.QueryString(t, pool, `SELECT count(*)::text FROM emails e JOIN campaigns c ON c.id = e.campaign_id WHERE e.workspace_id = $1 AND c.name = 'Re-engagement'`, ws); n != "1" {
		t.Fatalf("invited twice: %s emails", n)
	}
	dbtest.Exec(t, pool, `UPDATE contacts SET reengage_sent_at = now() - interval '31 days' WHERE workspace_id = $1 AND email = 'dormant@x.io'`, ws)
	if _, err := r.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := dbtest.QueryString(t, pool, `SELECT c.status || '/' || s.source FROM contacts c JOIN suppressions s ON s.workspace_id = c.workspace_id AND s.email = c.email WHERE c.workspace_id = $1 AND c.email = 'dormant@x.io'`, ws); got != "unsubscribed/dormancy" {
		t.Fatalf("dormant contact ended as %q", got)
	}
	if got := dbtest.QueryString(t, pool, `SELECT status FROM contacts WHERE workspace_id = $1 AND email = 'clicker@x.io'`, ws); got != "active" {
		t.Fatalf("engaged contact changed to %s", got)
	}
}

func TestContactChangesQueueWebhooks(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	dbtest.Exec(t, pool, `INSERT INTO webhook_endpoints (workspace_id, url, secret, events, enabled) VALUES ($1, 'https://hooks.example.com', repeat('s', 40), '{contact.updated,contact.deleted}', true)`, ws)
	contact := dbtest.QueryString(t, pool, `INSERT INTO contacts (workspace_id, email) VALUES ($1, 'sync@x.io') RETURNING id::text`, ws)
	dbtest.Exec(t, pool, `UPDATE contacts SET first_name = 'No event' WHERE id = $1`, contact)
	dbtest.Exec(t, pool, `UPDATE contacts SET paused_until = now() + interval '30 days' WHERE id = $1`, contact)
	dbtest.Exec(t, pool, `UPDATE contacts SET status = 'unsubscribed' WHERE id = $1`, contact)
	dbtest.Exec(t, pool, `DELETE FROM contacts WHERE id = $1`, contact)
	got := dbtest.QueryString(t, pool, `SELECT string_agg((payload::jsonb->>'type') || ':' || (payload::jsonb->'data'->>'status') || ':' || (payload::jsonb->'data'->>'email'), ',' ORDER BY event_id) FROM webhook_deliveries WHERE workspace_id = $1`, ws)
	if got != "contact.updated:active:sync@x.io,contact.updated:unsubscribed:sync@x.io,contact.deleted:unsubscribed:sync@x.io" {
		t.Fatalf("deliveries %q", got)
	}
}
