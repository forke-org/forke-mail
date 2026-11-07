package emails

import (
	"context"
	"testing"

	"forkemail/internal/db/dbtest"
)

func TestSnapshotTargetsAudienceAndExcludesBlocked(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	ws := dbtest.Workspace(t, pool, "paid", 0)
	other := dbtest.Workspace(t, pool, "paid", 0)
	email := dbtest.DraftEmail(t, pool, ws)
	dbtest.Exec(t, pool, `INSERT INTO contacts (workspace_id, email, status) VALUES
		($1, 'tagged@x.io', 'active'), ($1, 'picked@x.io', 'active'), ($1, 'sup@x.io', 'active'),
		($1, 'unsub@x.io', 'unsubscribed'), ($1, 'bounce@x.io', 'bounced'), ($1, 'outside@x.io', 'active')`, ws)
	dbtest.Exec(t, pool, `INSERT INTO contacts (workspace_id, email) VALUES ($1, 'foreign@x.io')`, other)
	tag := dbtest.QueryString(t, pool, `INSERT INTO tags (workspace_id, name) VALUES ($1, 'vip') RETURNING id::text`, ws)
	dbtest.Exec(t, pool, `INSERT INTO contact_tags (contact_id, tag_id)
		SELECT id, $2 FROM contacts WHERE workspace_id = $1 AND email IN ('tagged@x.io', 'sup@x.io', 'unsub@x.io')`, ws, tag)
	picked := dbtest.QueryString(t, pool, `SELECT id::text FROM contacts WHERE workspace_id = $1 AND email = 'picked@x.io'`, ws)
	bounced := dbtest.QueryString(t, pool, `SELECT id::text FROM contacts WHERE workspace_id = $1 AND email = 'bounce@x.io'`, ws)
	foreign := dbtest.QueryString(t, pool, `SELECT id::text FROM contacts WHERE workspace_id = $1`, other)
	dbtest.Exec(t, pool, `INSERT INTO suppressions (workspace_id, email, reason) VALUES ($1, 'SUP@x.io', 'manual')`, ws)

	if err := saveAudience(ctx, pool, ws, email, Audience{ContactIDs: []string{foreign}}); err == nil {
		t.Fatal("accepted a contact from another workspace")
	}
	if err := saveAudience(ctx, pool, ws, email, Audience{TagIDs: []string{tag}, ContactIDs: []string{picked, bounced, picked}}); err != nil {
		t.Fatal(err)
	}
	a, err := loadAudience(ctx, pool, ws, email)
	if err != nil || a.Size != 2 {
		t.Fatalf("audience size = %d, %v", a.Size, err)
	}
	n, err := Snapshot(ctx, pool, ws, email)
	if err != nil || n != 2 {
		t.Fatalf("snapshot = %d, %v", n, err)
	}
	if got := dbtest.QueryString(t, pool, `SELECT string_agg(email, ',' ORDER BY email) FROM email_recipients WHERE email_id = $1`, email); got != "picked@x.io,tagged@x.io" {
		t.Fatalf("recipients = %s", got)
	}
	if n, _ := Snapshot(ctx, pool, ws, email); n != 0 {
		t.Fatalf("second snapshot inserted %d duplicates", n)
	}

	everyone := dbtest.DraftEmail(t, pool, ws)
	if err := saveAudience(ctx, pool, ws, everyone, Audience{All: true}); err != nil {
		t.Fatal(err)
	}
	if n, _ := Snapshot(ctx, pool, ws, everyone); n != 3 {
		t.Fatalf("all-contacts snapshot = %d, want 3", n)
	}
}
