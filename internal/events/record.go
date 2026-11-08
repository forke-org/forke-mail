package events

import (
	"context"

	"forkemail/internal/db"
)

type Event struct {
	WorkspaceID  string
	EmailID      string
	RecipientID  string
	MessageID    string
	SESMessageID string
	Type         string
	Detail       map[string]any
}

func Record(ctx context.Context, q db.Querier, e Event) error {
	if e.Detail == nil {
		e.Detail = map[string]any{}
	}
	_, err := q.Exec(ctx, `
		INSERT INTO email_events (workspace_id, email_id, recipient_id, message_id, ses_message_id, type, detail)
		VALUES ($1, NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, NULLIF($5, ''), $6, $7)`,
		e.WorkspaceID, e.EmailID, e.RecipientID, e.MessageID, e.SESMessageID, e.Type, e.Detail)
	return err
}
