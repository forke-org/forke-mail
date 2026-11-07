package emails

import (
	"context"
	"errors"
	"testing"

	"forkemail/internal/db/dbtest"
)

func TestCheckDomainRequiresOwnVerifiedIdentity(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	other := dbtest.Workspace(t, pool, "paid", 0)
	dbtest.Exec(t, pool, `INSERT INTO domains (workspace_id, name, kind, status) VALUES
		($1, 'mine-verified.test', 'domain', 'verified'), ($1, 'mine-pending.test', 'domain', 'pending'),
		($1, 'solo@gmail-test.test', 'email', 'verified')`, ws)
	dbtest.Exec(t, pool, `INSERT INTO domains (workspace_id, name, kind, status) VALUES ($1, 'theirs.test', 'domain', 'verified')`, other)
	cases := map[string]bool{
		"hi@mine-verified.test":     true,
		"HI@Mine-Verified.test":     true,
		"hi@mine-pending.test":      false,
		"hi@theirs.test":            false,
		"solo@gmail-test.test":      true,
		"another@gmail-test.test":   false,
		"hi@sub.mine-verified.test": false,
	}
	for from, ok := range cases {
		err := checkDomain(context.Background(), pool, ws, from)
		var invalid invalidError
		switch {
		case ok && err != nil:
			t.Errorf("%s rejected: %v", from, err)
		case !ok && !(errors.As(err, &invalid) && invalid.code == "from_domain_not_verified"):
			t.Errorf("%s: got %v, want from_domain_not_verified", from, err)
		}
	}
}

func TestLaunchRefusedWhileWorkspacePaused(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	dbtest.Exec(t, pool, `INSERT INTO domains (workspace_id, name, status) VALUES ($1, $2, 'verified')`, ws, ws+".test")
	email := dbtest.DraftEmail(t, pool, ws)
	dbtest.Exec(t, pool, `UPDATE emails SET from_address = $2, audience_all = true WHERE id = $1`, email, "a@"+ws+".test")
	dbtest.Exec(t, pool, `INSERT INTO contacts (workspace_id, email) VALUES ($1, 'c@x.io')`, ws)
	dbtest.Exec(t, pool, `UPDATE workspaces SET sending_paused = true, pause_reason = 'bounce_rate_exceeded' WHERE id = $1`, ws)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := send(context.Background(), tx, ws, email); !errors.Is(err, errSendingPaused) {
		t.Fatalf("got %v, want errSendingPaused", err)
	}
}
