// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package mcp

import (
	"context"
	"strings"
	"sync"
	"testing"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/db/dbtest"
	"forkemail/internal/transactional"

	"github.com/jackc/pgx/v5"
)

type fakeMail struct {
	mu   sync.Mutex
	sent []transactional.Submission
}

func (f *fakeMail) Accept(_ context.Context, _ auth.Identity, _ string, s transactional.Submission) (transactional.Acceptance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, s)
	return transactional.Acceptance{}, nil
}

func TestApprovalEmailGoesToOwners(t *testing.T) {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 100)
	err := pgx.BeginFunc(t.Context(), pool, func(tx pgx.Tx) error {
		_, err := accounts.CreateUser(t.Context(), tx, ws, "Owner", ws+"@owner.test", "test-password")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	mail := &fakeMail{}
	h := &Handler{Pool: pool, BaseURL: "https://mail.test", Mail: mail, MailWorkspace: ws, MailFrom: "alerts@example.com"}
	id := dbtest.QueryString(t, pool, `INSERT INTO agent_approvals (workspace_id, requested_by, tool, args, summary, expires_at)
		VALUES ($1, 'An AI app', 'send_broadcast', '{}', 'Send the broadcast "Launch" to 120 people.', now() + interval '30 minutes') RETURNING id::text`, ws)
	h.notifyApproval(ws, id, "An AI app", `Send the broadcast "Launch" to 120 people.`)
	if len(mail.sent) != 1 || mail.sent[0].To != ws+"@owner.test" || !strings.Contains(mail.sent[0].Text, "https://mail.test/approve/"+id) {
		t.Fatalf("approval email %+v", mail.sent)
	}
	for range approvalEmailsPerHour + 1 {
		dbtest.Exec(t, pool, `INSERT INTO agent_approvals (workspace_id, requested_by, tool, args, summary, expires_at) VALUES ($1, 'x', 'x', '{}', 'x', now())`, ws)
	}
	h.notifyApproval(ws, id, "An AI app", "again")
	if len(mail.sent) != 1 {
		t.Fatal("approval emails are not capped")
	}
}
