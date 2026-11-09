package emails

import (
	"context"
	"testing"

	"forkemail/internal/db/dbtest"
)

func TestSnapshotRespectsPauseAndTopics(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	ws := dbtest.Workspace(t, pool, "paid", 0)
	dbtest.Exec(t, pool, `INSERT INTO contacts (workspace_id, email, paused_until, topics_opted_out) VALUES
		($1, 'reader@x.io', NULL, '{}'),
		($1, 'paused@x.io', now() + interval '10 days', '{}'),
		($1, 'pause-over@x.io', now() - interval '1 day', '{}'),
		($1, 'no-blog@x.io', NULL, '{blog}')`, ws)
	blog := dbtest.DraftEmail(t, pool, ws)
	dbtest.Exec(t, pool, `UPDATE emails SET topic = 'blog' WHERE id = $1`, blog)
	if err := saveAudience(ctx, pool, ws, blog, Audience{All: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := Snapshot(ctx, pool, ws, blog); err != nil {
		t.Fatal(err)
	}
	if got := dbtest.QueryString(t, pool, `SELECT string_agg(email, ',' ORDER BY email) FROM email_recipients WHERE email_id = $1`, blog); got != "pause-over@x.io,reader@x.io" {
		t.Fatalf("blog recipients = %s", got)
	}
	changelog := dbtest.DraftEmail(t, pool, ws)
	dbtest.Exec(t, pool, `UPDATE emails SET topic = 'changelog' WHERE id = $1`, changelog)
	if err := saveAudience(ctx, pool, ws, changelog, Audience{All: true}); err != nil {
		t.Fatal(err)
	}
	if n, _ := Snapshot(ctx, pool, ws, changelog); n != 3 {
		t.Fatalf("changelog snapshot = %d, want 3 (everyone but the paused contact)", n)
	}
}
