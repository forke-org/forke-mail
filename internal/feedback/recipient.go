package feedback

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

const (
	broadcastTable     = "email_recipients"
	transactionalTable = "transactional_messages"
)

type recipient struct {
	id, workspaceID, emailID, email, table string
}

func (r recipient) transactional() bool {
	return r.table == transactionalTable
}

func (r recipient) expectedTags() map[string]string {
	if r.transactional() {
		return map[string]string{"workspace_id": r.workspaceID, "message_id": r.id}
	}
	return map[string]string{"workspace_id": r.workspaceID, "email_id": r.emailID, "recipient_id": r.id}
}

func findRecipient(ctx context.Context, tx pgx.Tx, sesMessageID string) (recipient, error) {
	r := recipient{table: broadcastTable}
	err := tx.QueryRow(ctx, `
		SELECT id, workspace_id, email_id, email FROM email_recipients
		WHERE ses_message_id = $1 FOR UPDATE`, sesMessageID).Scan(&r.id, &r.workspaceID, &r.emailID, &r.email)
	if !errors.Is(err, pgx.ErrNoRows) {
		return r, err
	}
	r = recipient{table: transactionalTable}
	err = tx.QueryRow(ctx, `
		SELECT id, workspace_id, to_address FROM transactional_messages
		WHERE ses_message_id = $1 FOR UPDATE`, sesMessageID).Scan(&r.id, &r.workspaceID, &r.email)
	return r, err
}
