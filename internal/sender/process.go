package sender

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"forkemail/internal/domains"
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
	permit, err := w.waitBulk(ctx, rec.WorkspaceID, float64(limiter.Limit()))
	if err != nil {
		return false
	}
	consumed := false
	defer func() { permit.Finish(ctx, consumed) }()
	if w.idle() {
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
	verified, verifyErr := domains.Verified(ctx, w.pool, rec.WorkspaceID, st.content.FromAddress)
	if verifyErr != nil {
		return false
	}
	if !verified {
		w.complete(writeCtx, rec, "failed", "", "identity_not_verified")
		w.pause(writeCtx, rec, "identity_not_verified")
		return false
	}
	var res workspaces.Reservation
	err = pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE email_recipients SET provider_started_at=clock_timestamp() WHERE id=$1 AND workspace_id=$2 AND claim_token=$3 AND status='sending' AND provider_started_at IS NULL AND claimed_at>clock_timestamp()-interval '5 minutes'`, rec.ID, rec.WorkspaceID, rec.ClaimToken)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrPermitExpired
		}
		res, err = workspaces.Reserve(ctx, tx, rec.WorkspaceID)
		if err != nil {
			return err
		}
		return permit.Start(ctx, tx)
	})
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
		consumed = true
		w.complete(writeCtx, rec, "sent", messageID, "")
		if sim, ok := w.sender.(Simulator); ok {
			sim.Simulate(writeCtx, msg, messageID)
		}
		return true
	}
	failure := Classify(sendErr)
	consumed = failure.Action == UnknownOutcome
	return w.handleFailure(writeCtx, rec, failure, res)
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
		SELECT ws.status, ws.sending_paused, c.status, c.subject, c.from_name, c.from_address, c.reply_to, c.html, c.text,
			COALESCE(ct.status, ''), COALESCE(ct.first_name, ''),
			EXISTS (SELECT 1 FROM suppressions s WHERE s.workspace_id = r.workspace_id AND lower(s.email) = lower(r.email))
		FROM email_recipients r
		JOIN emails c ON c.id = r.email_id AND c.workspace_id = r.workspace_id
		JOIN workspaces ws ON ws.id = r.workspace_id
		LEFT JOIN contacts ct ON ct.id = r.contact_id AND ct.workspace_id = r.workspace_id
		WHERE r.id = $1`, rec.ID,
	).Scan(&st.workspaceStatus, &st.sendingPaused, &st.emailStatus, &c.Subject, &c.FromName, &c.FromAddress, &c.ReplyTo, &c.HTML, &c.Text,
		&st.contactStatus, &rec.FirstName, &st.suppressed)
	return st, err
}

func (w *Worker) complete(ctx context.Context, rec Recipient, status, messageID, reason string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE email_recipients
			SET status = $2, ses_message_id = NULLIF($3, ''), last_error = NULLIF($4, ''),
				sent_at = CASE WHEN $2 = 'sent' THEN now() END, claimed_at = NULL
			WHERE id = $1 AND workspace_id=$5 AND claim_token=$6 AND status='sending'`, rec.ID, status, messageID, reason, rec.WorkspaceID, rec.ClaimToken)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return nil
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

func (w *Worker) handleFailure(ctx context.Context, rec Recipient, f *SendError, res workspaces.Reservation) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	message := f.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	slog.Warn("send failed", "recipient_id", rec.ID, "email_id", rec.EmailID, "reason", f.Reason, "attempts", rec.Attempts)
	err := pgx.BeginFunc(ctx, w.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT true FROM email_recipients WHERE id=$1 AND workspace_id=$2 AND claim_token=$3 AND status='sending' FOR UPDATE`, rec.ID, rec.WorkspaceID, rec.ClaimToken).Scan(&exists); errors.Is(err, pgx.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		if f.Action != UnknownOutcome {
			if err := workspaces.Release(ctx, tx, res); err != nil {
				return err
			}
		}
		status := "failed"
		reason := f.Reason
		delay := 0.0
		if f.Action == PauseSending || (f.Action == Retry && rec.Attempts < maxAttempts) {
			status = "pending"
			reason = message
			delay = (time.Duration(1<<rec.Attempts) * 5 * time.Second).Seconds()
		}
		if _, err := tx.Exec(ctx, `UPDATE email_recipients SET status=$2,last_error=$3,claimed_at=NULL,claim_token=NULL,provider_started_at=NULL,next_attempt_at=clock_timestamp()+make_interval(secs=>$4),attempts=attempts-CASE WHEN $5 THEN 1 ELSE 0 END WHERE id=$1`, rec.ID, status, reason, delay, f.Action == PauseSending); err != nil {
			return err
		}
		if f.Action == PauseSending || f.Action == PauseIdentity {
			pause := "identity_not_verified"
			if f.Action == PauseSending {
				pause = "ses_sending_paused"
			}
			if _, err := tx.Exec(ctx, `UPDATE emails SET status='paused',pause_reason=$3 WHERE id=$1 AND workspace_id=$2 AND status='sending'`, rec.EmailID, rec.WorkspaceID, pause); err != nil {
				return err
			}
		}
		if status == "failed" {
			return events.Record(ctx, tx, events.Event{WorkspaceID: rec.WorkspaceID, EmailID: rec.EmailID, RecipientID: rec.ID, Type: "failed", Detail: map[string]any{"reason": f.Reason, "error": message, "attempts": rec.Attempts}})
		}
		return nil
	})
	if f.Action == PauseSending {
		w.HoldSending(f.Reason)
	}
	if err != nil {
		slog.Error("record send failure failed", "recipient_id", rec.ID, "err", err)
	}
	return f.Action != PauseSending && f.Action != PauseIdentity
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
