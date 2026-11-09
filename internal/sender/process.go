package sender

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"forkemail/internal/events"
	"forkemail/internal/unsub"
	"forkemail/internal/workspaces"

	"github.com/jackc/pgx/v5"
	"golang.org/x/time/rate"
)

const (
	maxAttempts = 5
	sendTimeout = 30 * time.Second
)

type state struct {
	workspaceStatus string
	sendingPaused   bool
	emailStatus     string
	contactStatus   string
	suppressed      bool
	content         Content
}

func (w *Worker) process(ctx context.Context, limiter *rate.Limiter, rec Recipient) bool {
	if limiter.Wait(ctx) != nil || w.waitBulk(ctx) != nil || w.idle() {
		return false
	}
	st, err := w.loadState(ctx, &rec)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("load recipient state failed", "recipient_id", rec.ID, "err", err)
		}
		return false
	}
	if st.workspaceStatus != "active" || st.sendingPaused || st.emailStatus != "sending" {
		return false
	}
	writeCtx := context.WithoutCancel(ctx)
	if st.contactStatus != "active" || st.suppressed {
		w.complete(writeCtx, rec, "skipped", "", "recipient_blocked")
		return true
	}
	res, err := workspaces.Reserve(ctx, w.pool, rec.WorkspaceID)
	if errors.Is(err, workspaces.ErrTrialQuotaExhausted) || errors.Is(err, workspaces.ErrDailyLimitReached) {
		w.pause(writeCtx, rec, err.Error())
		return false
	}
	if err != nil {
		slog.Error("reserve quota failed", "workspace_id", rec.WorkspaceID, "err", err)
		return false
	}
	url := w.baseURL + "/unsubscribe/" + w.signer.Sign(unsub.Target{
		WorkspaceID: rec.WorkspaceID, ContactID: rec.ContactID, EmailID: rec.EmailID,
	})
	msg := Render(st.content, rec, url)
	sendCtx, cancel := context.WithTimeout(writeCtx, sendTimeout)
	messageID, sendErr := w.sender.Send(sendCtx, msg)
	cancel()
	if sendErr == nil {
		w.CountSent()
		w.complete(writeCtx, rec, "sent", messageID, "")
		if sim, ok := w.sender.(Simulator); ok {
			sim.Simulate(writeCtx, msg, messageID)
		}
		return true
	}
	failure := Classify(sendErr)
	if failure.Action != UnknownOutcome {
		if err := workspaces.Release(writeCtx, w.pool, res); err != nil {
			slog.Error("release quota failed", "workspace_id", rec.WorkspaceID, "err", err)
		}
	}
	return w.handleFailure(writeCtx, rec, failure)
}

func Classify(err error) *SendError {
	var se *SendError
	if errors.As(err, &se) {
		return se
	}
	return &SendError{Action: UnknownOutcome, Reason: "unknown_outcome", Err: err}
}

func (w *Worker) loadState(ctx context.Context, rec *Recipient) (state, error) {
	var st state
	c := &st.content
	err := w.pool.QueryRow(ctx, `
		SELECT ws.status, ws.sending_paused, c.status, c.subject, c.from_name, c.from_address, c.html, c.text,
			COALESCE(ct.status, ''), COALESCE(ct.first_name, ''),
			EXISTS (SELECT 1 FROM suppressions s WHERE s.workspace_id = r.workspace_id AND lower(s.email) = lower(r.email))
		FROM email_recipients r
		JOIN emails c ON c.id = r.email_id AND c.workspace_id = r.workspace_id
		JOIN workspaces ws ON ws.id = r.workspace_id
		LEFT JOIN contacts ct ON ct.id = r.contact_id AND ct.workspace_id = r.workspace_id
		WHERE r.id = $1`, rec.ID,
	).Scan(&st.workspaceStatus, &st.sendingPaused, &st.emailStatus, &c.Subject, &c.FromName, &c.FromAddress, &c.HTML, &c.Text,
		&st.contactStatus, &rec.FirstName, &st.suppressed)
	return st, err
}

func (w *Worker) complete(ctx context.Context, rec Recipient, status, messageID, reason string) {
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE email_recipients
			SET status = $2, ses_message_id = NULLIF($3, ''), last_error = NULLIF($4, ''),
				sent_at = CASE WHEN $2 = 'sent' THEN now() END, claimed_at = NULL
			WHERE id = $1`, rec.ID, status, messageID, reason); err != nil {
			return err
		}
		if status != "sent" {
			return nil
		}
		return events.Record(ctx, tx, events.Event{
			WorkspaceID: rec.WorkspaceID, EmailID: rec.EmailID, RecipientID: rec.ID, SESMessageID: messageID,
			Type: "sent", Detail: map[string]any{"message_id": messageID},
		})
	})
	if err != nil {
		slog.Error("complete recipient failed", "recipient_id", rec.ID, "err", err)
	}
}

func (w *Worker) handleFailure(ctx context.Context, rec Recipient, f *SendError) bool {
	message := f.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	slog.Warn("send failed", "recipient_id", rec.ID, "email_id", rec.EmailID, "reason", f.Reason, "attempts", rec.Attempts)
	var err error
	switch {
	case f.Action == PauseSending:
		w.HoldSending(f.Reason)
		if err = Unclaim(ctx, w.pool, []string{rec.ID}); err == nil {
			err = w.pause(ctx, rec, "ses_sending_paused")
		}
	case f.Action == Retry && rec.Attempts < maxAttempts:
		backoff := time.Duration(1<<rec.Attempts) * 5 * time.Second
		_, err = w.pool.Exec(ctx, `
			UPDATE email_recipients
			SET status = 'pending', claimed_at = NULL, last_error = $2, next_attempt_at = now() + make_interval(secs => $3)
			WHERE id = $1`, rec.ID, message, backoff.Seconds())
	default:
		err = w.fail(ctx, rec, f.Reason, message)
		if err == nil && f.Action == PauseIdentity {
			err = w.pause(ctx, rec, "identity_not_verified")
		}
	}
	if err != nil {
		slog.Error("record send failure failed", "recipient_id", rec.ID, "err", err)
	}
	return f.Action != PauseSending && f.Action != PauseIdentity
}

func (w *Worker) fail(ctx context.Context, rec Recipient, reason, message string) error {
	return pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE email_recipients SET status = 'failed', claimed_at = NULL, last_error = $2
			WHERE id = $1`, rec.ID, reason); err != nil {
			return err
		}
		return events.Record(ctx, tx, events.Event{
			WorkspaceID: rec.WorkspaceID, EmailID: rec.EmailID, RecipientID: rec.ID,
			Type: "failed", Detail: map[string]any{"reason": reason, "error": message, "attempts": rec.Attempts},
		})
	})
}

func (w *Worker) pause(ctx context.Context, rec Recipient, reason string) error {
	_, err := w.pool.Exec(ctx, `
		UPDATE emails SET status = 'paused', pause_reason = $3
		WHERE id = $1 AND workspace_id = $2 AND status = 'sending'`, rec.EmailID, rec.WorkspaceID, reason)
	if err != nil {
		slog.Error("pause email failed", "email_id", rec.EmailID, "err", err)
	}
	return err
}
