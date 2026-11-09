package tracking

import (
	"context"
	"errors"

	"forkemail/internal/events"

	"github.com/jackc/pgx/v5"
)

func (h *Handler) recordRecipientOpen(ctx context.Context, id string) error {
	return pgx.BeginFunc(ctx, h.Pool, func(tx pgx.Tx) error {
		var ws, emailID string
		var first bool
		err := tx.QueryRow(ctx, `
			UPDATE email_recipients r SET opened_at = COALESCE(r.opened_at, now()), open_count = r.open_count + 1
			FROM emails e
			WHERE r.id = $1 AND e.id = r.email_id AND e.track_opens AND r.status = 'sent'
			RETURNING r.workspace_id, r.email_id, r.open_count = 1`, id).Scan(&ws, &emailID, &first)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !first) {
			return nil
		}
		if err != nil {
			return err
		}
		return events.Record(ctx, tx, events.Event{WorkspaceID: ws, EmailID: emailID, RecipientID: id, Type: "opened"})
	})
}

func (h *Handler) recordRecipientClick(ctx context.Context, id, target string) error {
	return pgx.BeginFunc(ctx, h.Pool, func(tx pgx.Tx) error {
		var ws, emailID string
		var contactID *string
		err := tx.QueryRow(ctx, `
			UPDATE email_recipients r SET clicked_at = COALESCE(r.clicked_at, now()), click_count = r.click_count + 1
			FROM emails e
			WHERE r.id = $1 AND e.id = r.email_id AND e.track_clicks AND r.status = 'sent'
			RETURNING r.workspace_id, r.email_id, r.contact_id`, id).Scan(&ws, &emailID, &contactID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if contactID != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE contacts SET last_engaged_at = now(), reengage_sent_at = NULL
				WHERE id = $1 AND workspace_id = $2`, *contactID, ws); err != nil {
				return err
			}
		}
		return events.Record(ctx, tx, events.Event{WorkspaceID: ws, EmailID: emailID, RecipientID: id, Type: "clicked", Detail: map[string]any{"url": target}})
	})
}
