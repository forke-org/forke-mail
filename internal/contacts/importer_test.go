package contacts

import (
	"context"
	"strings"
	"testing"

	"forkemail/internal/db/dbtest"
)

func TestImportNeverReactivatesBlockedContacts(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	ctx := context.Background()
	dbtest.Exec(t, pool, `INSERT INTO contacts (workspace_id, email, status) VALUES
		($1, 'unsub@x.io', 'unsubscribed'), ($1, 'bounce@x.io', 'bounced'), ($1, 'complaint@x.io', 'complained'), ($1, 'keep@x.io', 'active')`, ws)
	dbtest.Exec(t, pool, `INSERT INTO suppressions (workspace_id, email, reason) VALUES ($1, 'manual@x.io', 'manual')`, ws)

	parsed, err := ParseCSV(strings.NewReader("UNSUB@x.io,U\nbounce@x.io\ncomplaint@x.io\nmanual@x.io\nkeep@x.io,Kept\nnew@x.io\nnew@x.io\nbad\n"))
	if err != nil {
		t.Fatal(err)
	}
	email := dbtest.DraftEmail(t, pool, ws)
	res, err := Import(ctx, pool, ws, parsed, ImportTarget{EmailID: email, Tags: []string{"t1"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 1 || res.Updated != 1 || res.Skipped != 5 || res.Invalid != 1 {
		t.Fatalf("unexpected result %+v", res)
	}
	for email, want := range map[string]string{"unsub@x.io": "unsubscribed", "bounce@x.io": "bounced", "complaint@x.io": "complained", "keep@x.io": "active", "new@x.io": "active"} {
		if got := dbtest.QueryString(t, pool, `SELECT status FROM contacts WHERE workspace_id = $1 AND email = $2`, ws, email); got != want {
			t.Errorf("%s status = %s, want %s", email, got, want)
		}
	}
	if got := dbtest.QueryString(t, pool, `SELECT COUNT(*)::text FROM contacts WHERE workspace_id = $1 AND email = 'manual@x.io'`, ws); got != "0" {
		t.Error("suppressed address was inserted as a contact")
	}
	if got := dbtest.QueryString(t, pool, `SELECT COUNT(*)::text FROM email_contacts WHERE email_id = $1`, email); got != "2" {
		t.Errorf("email audience contacts = %s, want 2", got)
	}
	if got := dbtest.QueryString(t, pool, `SELECT COUNT(*)::text FROM contact_tags ct JOIN tags t ON t.id = ct.tag_id WHERE t.workspace_id = $1 AND t.name = 't1'`, ws); got != "2" {
		t.Errorf("tagged contacts = %s, want 2", got)
	}
	if got := dbtest.QueryString(t, pool, `SELECT first_name FROM contacts WHERE workspace_id = $1 AND email = 'keep@x.io'`, ws); got != "Kept" {
		t.Errorf("active contact not updated: %q", got)
	}
}
