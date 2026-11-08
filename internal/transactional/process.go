package transactional

import (
	"context"
	"log/slog"
	"mime"
	"net/mail"
	"time"

	"forkemail/internal/events"
	"forkemail/internal/sender"
	"forkemail/internal/workspaces"

	"github.com/jackc/pgx/v5"
)

const (
	maxAttempts  = 5
	sendTimeout  = 30 * time.Second
	maxQueueAge  = time.Hour
	gateBackoff  = 30 * time.Second
	retryBackoff = time.Second
)

func (d *Dispatcher) process(ctx context.Context, j job) {
	writeCtx := context.WithoutCancel(ctx)
	if reason, err := d.blocked(ctx, j); err != nil || reason != "" {
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("load transactional state failed", "message_id", j.id, "err", err)
			}
			d.requeue(writeCtx, j, gateBackoff, "")
			return
		}
		d.fail(writeCtx, j, reason, true)
		return
	}
	if err := d.gate.AcquireUrgent(ctx); err != nil {
		reason := err.Error()
		if ctx.Err() != nil {
			reason = ""
		}
		d.requeue(writeCtx, j, gateBackoff, reason)
		return
	}
	msg := d.render(j)
	start := time.Now()
	sendCtx, cancel := context.WithTimeout(writeCtx, sendTimeout)
	messageID, sendErr := d.sender.Send(sendCtx, msg)
	cancel()
	sendMS := float64(time.Since(start).Microseconds()) / 1000
	if sendErr == nil {
		d.gate.CountSent()
		d.complete(writeCtx, j, messageID, sendMS)
		if sim, ok := d.sender.(sender.Simulator); ok {
			sim.Simulate(writeCtx, msg, messageID)
		}
		return
	}
	f := sender.Classify(sendErr)
	slog.Warn("transactional send failed", "message_id", j.id, "reason", f.Reason, "attempts", j.attempts)
	switch {
	case f.Action == sender.PauseSending:
		d.gate.HoldSending(f.Reason)
		d.requeue(writeCtx, j, gateBackoff, f.Reason)
	case f.Action == sender.Retry && j.attempts < maxAttempts:
		d.retry(writeCtx, j, retryBackoff*time.Duration(1<<j.attempts), truncate(f.Error()))
	default:
		d.fail(writeCtx, j, f.Reason, f.Action != sender.UnknownOutcome)
	}
}

func (d *Dispatcher) blocked(ctx context.Context, j job) (string, error) {
	if time.Since(j.createdAt) > maxQueueAge {
		return "expired", nil
	}
	var status string
	var paused, suppressed bool
	err := d.pool.QueryRow(ctx, `
		SELECT status, sending_paused, EXISTS (
			SELECT 1 FROM suppressions s WHERE s.workspace_id = w.id AND lower(s.email) = lower($2)
				AND s.reason IN ('bounced', 'complained', 'manual'))
		FROM workspaces w WHERE id = $1`, j.workspaceID, j.to).Scan(&status, &paused, &suppressed)
	switch {
	case err != nil:
		return "", err
	case status != "active":
		return errWorkspaceInactive.Error(), nil
	case paused:
		return errSendingPaused.Error(), nil
	case suppressed:
		return errSuppressed.Error(), nil
	}
	return "", nil
}

func (d *Dispatcher) render(j job) sender.Message {
	from := (&mail.Address{Name: j.fromName, Address: j.fromAddress}).String()
	body := j.html
	if j.trackOpens || j.trackClicks {
		body = d.tracker.Instrument(body, j.id, j.trackOpens, j.trackClicks)
	}
	headers := map[string]string{
		"From":         from,
		"To":           j.to,
		"Subject":      mime.QEncoding.Encode("utf-8", j.subject),
		"MIME-Version": "1.0",
	}
	if j.replyTo != "" {
		headers["Reply-To"] = j.replyTo
	}
	return sender.Message{
		WorkspaceID: j.workspaceID, MessageID: j.id, To: j.to, From: from, ReplyTo: j.replyTo,
		Subject: j.subject, HTML: body, Text: j.text, Headers: headers,
	}
}

func (d *Dispatcher) complete(ctx context.Context, j job, messageID string, sendMS float64) {
	err := pgx.BeginFunc(ctx, d.pool, func(tx pgx.Tx) error {
		var dispatchMS float64
		if err := tx.QueryRow(ctx, `
			UPDATE transactional_messages
			SET status = 'sent', ses_message_id = $2, sent_at = now(), claimed_at = NULL, last_error = NULL
			WHERE id = $1
			RETURNING EXTRACT(EPOCH FROM (sent_at - created_at)) * 1000`, j.id, messageID).Scan(&dispatchMS); err != nil {
			return err
		}
		return events.Record(ctx, tx, events.Event{
			WorkspaceID: j.workspaceID, MessageID: j.id, SESMessageID: messageID, Type: "sent",
			Detail: map[string]any{"message_id": messageID, "dispatch_ms": dispatchMS, "send_ms": sendMS, "attempts": j.attempts},
		})
	})
	if err != nil {
		slog.Error("complete transactional message failed", "message_id", j.id, "err", err)
	}
}

func (d *Dispatcher) retry(ctx context.Context, j job, backoff time.Duration, lastError string) {
	if _, err := d.pool.Exec(ctx, `
		UPDATE transactional_messages
		SET status = 'queued', claimed_at = NULL, last_error = $2, next_attempt_at = now() + make_interval(secs => $3)
		WHERE id = $1 AND status = 'sending'`, j.id, lastError, backoff.Seconds()); err != nil {
		slog.Error("retry transactional message failed", "message_id", j.id, "err", err)
	}
}

func (d *Dispatcher) requeue(ctx context.Context, j job, backoff time.Duration, lastError string) {
	if _, err := d.pool.Exec(ctx, `
		UPDATE transactional_messages
		SET status = 'queued', attempts = GREATEST(attempts - 1, 0), claimed_at = NULL,
			last_error = COALESCE(NULLIF($2, ''), last_error), next_attempt_at = now() + make_interval(secs => $3)
		WHERE id = $1 AND status = 'sending'`, j.id, lastError, backoff.Seconds()); err != nil {
		slog.Error("requeue transactional message failed", "message_id", j.id, "err", err)
	}
}

func (d *Dispatcher) fail(ctx context.Context, j job, reason string, releaseQuota bool) {
	err := pgx.BeginFunc(ctx, d.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE transactional_messages SET status = 'failed', claimed_at = NULL, last_error = $2
			WHERE id = $1`, j.id, reason); err != nil {
			return err
		}
		if releaseQuota && j.quotaDay != nil {
			res := workspaces.Reservation{WorkspaceID: j.workspaceID, Trial: j.quotaTrial, Day: *j.quotaDay}
			if err := workspaces.Release(ctx, tx, res); err != nil {
				return err
			}
		}
		return events.Record(ctx, tx, events.Event{
			WorkspaceID: j.workspaceID, MessageID: j.id, Type: "failed",
			Detail: map[string]any{"reason": reason, "attempts": j.attempts},
		})
	})
	if err != nil {
		slog.Error("fail transactional message failed", "message_id", j.id, "err", err)
	}
}

func truncate(s string) string {
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
