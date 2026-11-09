package sender

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"forkemail/internal/feedback"

	"github.com/jackc/pgx/v5/pgxpool"
)

type LogSender struct {
	Pool     *pgxpool.Pool
	Feedback *feedback.Processor
}

func (s LogSender) Send(ctx context.Context, m Message) (string, error) {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	messageID := "log-" + hex.EncodeToString(b)
	if _, err := s.Pool.Exec(ctx, `
		INSERT INTO message_logs (workspace_id, email_id, recipient_id, message_id, payload)
		VALUES ($1, NULLIF($2, '')::uuid, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, $5)`, m.WorkspaceID, m.EmailID, m.RecipientID, m.MessageID,
		map[string]any{"message_id": messageID, "message": m}); err != nil {
		return "", &SendError{Action: Retry, Reason: "log_write_failed", Err: err}
	}
	return messageID, nil
}

func (s LogSender) Simulate(ctx context.Context, m Message, messageID string) {
	for _, ev := range simulatedEvents(m, messageID, time.Now().UTC()) {
		raw, err := json.Marshal(ev)
		if err == nil {
			_, err = s.Feedback.Handle(ctx, "", raw)
		}
		if err != nil {
			slog.Error("simulated ses event failed", "recipient_id", m.RecipientID, "message_id", m.MessageID, "err", err)
		}
	}
}

func simulatedEvents(m Message, messageID string, now time.Time) []feedback.Event {
	tags := map[string][]string{}
	for name, value := range m.Tags() {
		tags[name] = []string{value}
	}
	mail := feedback.Mail{Timestamp: now, MessageID: messageID, Source: m.From, Destination: []string{m.To}, Tags: tags}
	delivery := feedback.Event{EventType: "Delivery", Mail: mail, Delivery: &feedback.Delivery{
		Timestamp: now, Recipients: []string{m.To}, SMTPResponse: "250 2.6.0 Message received",
	}}
	local, domain, _ := strings.Cut(strings.ToLower(m.To), "@")
	if domain != "simulator.amazonses.com" {
		return []feedback.Event{delivery}
	}
	permanent := func(subType string) feedback.Event {
		return feedback.Event{EventType: "Bounce", Mail: mail, Bounce: &feedback.Bounce{
			BounceType: "Permanent", BounceSubType: subType, Timestamp: now,
			BouncedRecipients: []feedback.Recipient{{EmailAddress: m.To, Action: "failed", Status: "5.1.1", DiagnosticCode: "smtp; 550 5.1.1 user unknown"}},
		}}
	}
	switch local {
	case "bounce":
		return []feedback.Event{permanent("General")}
	case "suppressionlist":
		return []feedback.Event{permanent("OnAccountSuppressionList")}
	case "complaint":
		return []feedback.Event{delivery, {EventType: "Complaint", Mail: mail, Complaint: &feedback.Complaint{
			ComplainedRecipients: []feedback.Recipient{{EmailAddress: m.To}}, ComplaintFeedbackType: "abuse", Timestamp: now,
		}}}
	}
	return []feedback.Event{delivery}
}
