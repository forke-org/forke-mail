package sender

import (
	"context"

	"forkemail/internal/db"

	"github.com/jackc/pgx/v5"
)

func Claim(ctx context.Context, q db.Querier, workspaceID string, n int) ([]Recipient, error) {
	rows, err := q.Query(ctx, `
		UPDATE email_recipients r
		SET status = 'sending', attempts = r.attempts + 1, claimed_at = now()
		WHERE r.status = 'pending' AND r.id IN (
			SELECT cr.id FROM email_recipients cr
			JOIN emails c ON c.id = cr.email_id
			WHERE cr.workspace_id = $1 AND cr.status = 'pending' AND cr.next_attempt_at <= now() AND c.status = 'sending'
			ORDER BY cr.next_attempt_at
			LIMIT $2
			FOR UPDATE OF cr SKIP LOCKED
		)
		RETURNING r.id, r.email_id, r.workspace_id, COALESCE(r.contact_id::text, ''), r.email, r.attempts`, workspaceID, n)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Recipient, error) {
		var r Recipient
		return r, row.Scan(&r.ID, &r.EmailID, &r.WorkspaceID, &r.ContactID, &r.Email, &r.Attempts)
	})
}

func Unclaim(ctx context.Context, q db.Querier, ids []string) error {
	_, err := q.Exec(ctx, `
		UPDATE email_recipients
		SET status = 'pending', attempts = GREATEST(attempts - 1, 0), claimed_at = NULL
		WHERE id = ANY($1::uuid[]) AND status = 'sending'`, ids)
	return err
}
