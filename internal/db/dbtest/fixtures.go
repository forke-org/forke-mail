package dbtest

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func DraftEmail(t *testing.T, pool *pgxpool.Pool, workspaceID string) string {
	t.Helper()
	campaign := QueryString(t, pool, `INSERT INTO campaigns (workspace_id, name) VALUES ($1, $2 || gen_random_uuid()) RETURNING id::text`, workspaceID, t.Name())
	return QueryString(t, pool, `INSERT INTO emails (workspace_id, campaign_id, subject, from_address, text)
		VALUES ($1, $2, 's', 'a@forke.space', 'hi') RETURNING id::text`, workspaceID, campaign)
}

func SentRecipient(t *testing.T, pool *pgxpool.Pool, workspaceID, emailID, address, messageID string) string {
	t.Helper()
	contact := QueryString(t, pool, `INSERT INTO contacts (workspace_id, email) VALUES ($1, $2)
		ON CONFLICT (workspace_id, lower(email)) DO UPDATE SET email = EXCLUDED.email RETURNING id::text`, workspaceID, address)
	return QueryString(t, pool, `INSERT INTO email_recipients (email_id, workspace_id, contact_id, email, status, ses_message_id, sent_at)
		VALUES ($1, $2, $3, $4, 'sent', $5, now()) RETURNING id::text`, emailID, workspaceID, contact, address, messageID)
}
