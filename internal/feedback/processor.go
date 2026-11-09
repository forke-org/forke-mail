package feedback

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"forkemail/internal/contacts"
	"forkemail/internal/events"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	Processed      = "processed"
	Duplicate      = "duplicate"
	Ignored        = "ignored"
	UnknownMessage = "unknown_message"
	TagMismatch    = "tag_mismatch"
)

type Processor struct {
	Pool       *pgxpool.Pool
	Reputation Thresholds
}

func (p *Processor) Handle(ctx context.Context, snsMessageID string, raw []byte) (string, error) {
	var result string
	err := pgx.BeginFunc(ctx, p.Pool, func(tx pgx.Tx) error {
		if snsMessageID != "" {
			tag, err := tx.Exec(ctx, `
				INSERT INTO processed_sns_messages (sns_message_id) VALUES ($1) ON CONFLICT DO NOTHING`, snsMessageID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				result = Duplicate
				return nil
			}
		}
		var ev Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			slog.Warn("ses event is not valid json", "sns_message_id", snsMessageID)
			result = Ignored
			return nil
		}
		var err error
		result, err = p.apply(ctx, tx, ev)
		return err
	})
	return result, err
}

func (p *Processor) apply(ctx context.Context, tx pgx.Tx, ev Event) (string, error) {
	kind := ev.Kind()
	switch kind {
	case "Delivery", "Bounce", "Complaint", "Reject", "DeliveryDelay":
	default:
		return Ignored, nil
	}
	if ev.Mail.MessageID == "" {
		return Ignored, nil
	}
	r, err := findRecipient(ctx, tx, ev.Mail.MessageID)
	if errors.Is(err, pgx.ErrNoRows) {
		slog.Info("ses event for unknown message", "event", kind)
		return UnknownMessage, nil
	}
	if err != nil {
		return "", err
	}
	if !tagsMatch(ev.Mail.Tags, r.expectedTags()) {
		slog.Warn("ses event tags do not match recipient", "event", kind, "recipient_id", r.id)
		return TagMismatch, nil
	}
	record := func(eventType string, detail map[string]any) error {
		e := events.Event{WorkspaceID: r.workspaceID, SESMessageID: ev.Mail.MessageID, Type: eventType, Detail: detail}
		if r.transactional() {
			e.MessageID = r.id
		} else {
			e.EmailID, e.RecipientID = r.emailID, r.id
		}
		return events.Record(ctx, tx, e)
	}
	switch kind {
	case "Delivery":
		err = p.delivery(ctx, tx, r, ev, record)
	case "Bounce":
		err = p.bounce(ctx, tx, r, ev, record)
	case "Complaint":
		err = p.complaint(ctx, tx, r, ev, record)
	case "Reject":
		err = p.reject(ctx, tx, r, ev, record)
	case "DeliveryDelay":
		detail := map[string]any{}
		if ev.DeliveryDelay != nil {
			detail["delay_type"] = ev.DeliveryDelay.DelayType
			detail["expiration_time"] = ev.DeliveryDelay.ExpirationTime
		}
		err = record("delayed", detail)
	}
	if err != nil {
		return "", err
	}
	return Processed, nil
}

func tagsMatch(tags map[string][]string, expected map[string]string) bool {
	for _, name := range []string{"workspace_id", "email_id", "recipient_id", "message_id"} {
		values, ok := tags[name]
		if !ok {
			continue
		}
		want, known := expected[name]
		if !known || len(values) != 1 || values[0] != want {
			return false
		}
	}
	return true
}

type recordFunc func(eventType string, detail map[string]any) error

func (p *Processor) delivery(ctx context.Context, tx pgx.Tx, r recipient, ev Event, record recordFunc) error {
	detail := map[string]any{}
	ts := at(ev.Mail.Timestamp)
	if ev.Delivery != nil {
		ts = at(ev.Delivery.Timestamp)
		detail["smtp_response"] = ev.Delivery.SMTPResponse
	}
	if _, err := tx.Exec(ctx, `
		UPDATE `+r.table+` SET delivered_at = COALESCE(delivered_at, $2) WHERE id = $1`, r.id, ts); err != nil {
		return err
	}
	return record("delivered", detail)
}

func (p *Processor) bounce(ctx context.Context, tx pgx.Tx, r recipient, ev Event, record recordFunc) error {
	if ev.Bounce == nil {
		return nil
	}
	b := ev.Bounce
	detail := map[string]any{"bounce_type": b.BounceType, "bounce_sub_type": b.BounceSubType, "feedback_id": b.FeedbackID}
	for _, br := range b.BouncedRecipients {
		if br.DiagnosticCode != "" {
			detail["diagnostic_code"] = br.DiagnosticCode
		}
	}
	if b.BounceType != "Permanent" {
		return record("soft_bounced", detail)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE `+r.table+` SET bounced_at = COALESCE(bounced_at, $2), bounce_type = $3 WHERE id = $1`,
		r.id, at(b.Timestamp), b.BounceType+"/"+b.BounceSubType); err != nil {
		return err
	}
	if _, err := contacts.Suppress(ctx, tx, contacts.Suppression{
		WorkspaceID: r.workspaceID, Email: r.email, Reason: "bounced", Source: "ses_bounce", Detail: detail,
	}); err != nil {
		return err
	}
	if err := record("bounced", detail); err != nil {
		return err
	}
	return p.checkReputation(ctx, tx, r.workspaceID)
}

func (p *Processor) complaint(ctx context.Context, tx pgx.Tx, r recipient, ev Event, record recordFunc) error {
	detail := map[string]any{}
	ts := at(ev.Mail.Timestamp)
	if ev.Complaint != nil {
		ts = at(ev.Complaint.Timestamp)
		detail["feedback_type"] = ev.Complaint.ComplaintFeedbackType
		detail["feedback_id"] = ev.Complaint.FeedbackID
	}
	if _, err := tx.Exec(ctx, `
		UPDATE `+r.table+` SET complained_at = COALESCE(complained_at, $2) WHERE id = $1`, r.id, ts); err != nil {
		return err
	}
	if _, err := contacts.Suppress(ctx, tx, contacts.Suppression{
		WorkspaceID: r.workspaceID, Email: r.email, Reason: "complained", Source: "ses_complaint", Detail: detail,
	}); err != nil {
		return err
	}
	if err := record("complained", detail); err != nil {
		return err
	}
	return p.checkReputation(ctx, tx, r.workspaceID)
}

func (p *Processor) reject(ctx context.Context, tx pgx.Tx, r recipient, ev Event, record recordFunc) error {
	reason := "rejected"
	if ev.Reject != nil && ev.Reject.Reason != "" {
		reason = "rejected: " + ev.Reject.Reason
	}
	if _, err := tx.Exec(ctx, `
		UPDATE `+r.table+` SET status = 'failed', last_error = $2 WHERE id = $1`, r.id, reason); err != nil {
		return err
	}
	return record("rejected", map[string]any{"reason": reason})
}
