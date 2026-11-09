package transactional

import (
	"context"
	"errors"
	"time"

	"forkemail/internal/db"
	"forkemail/internal/events"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const stuckAfter = 5 * time.Minute

type job struct {
	id, workspaceID                     string
	to, fromName, fromAddress, replyTo  string
	subject, html, text                 string
	trackOpens, trackClicks, quotaTrial bool
	attempts                            int
	createdAt                           time.Time
	quotaDay                            *time.Time
	claimToken                          string
}

const claimReturning = `
	RETURNING id, workspace_id, to_address, from_name, from_address, COALESCE(reply_to, ''), subject, html, text,
		track_opens, track_clicks, quota_trial, attempts, created_at, quota_day,claim_token`

func scanJob(row pgx.Row) (job, error) {
	var j job
	err := row.Scan(&j.id, &j.workspaceID, &j.to, &j.fromName, &j.fromAddress, &j.replyTo, &j.subject, &j.html, &j.text,
		&j.trackOpens, &j.trackClicks, &j.quotaTrial, &j.attempts, &j.createdAt, &j.quotaDay, &j.claimToken)
	return j, err
}

func claimOne(ctx context.Context, q db.Querier, id string) (job, bool, error) {
	j, err := scanJob(q.QueryRow(ctx, `
		UPDATE transactional_messages SET status = 'sending', attempts = attempts + 1, claimed_at = clock_timestamp(),claim_token=gen_random_uuid(),provider_started_at=NULL
		WHERE id = $1 AND status = 'queued' AND next_attempt_at <= now()`+claimReturning, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return j, false, nil
	}
	return j, err == nil, err
}

func claimDue(ctx context.Context, q db.Querier, n int) ([]job, error) {
	rows, err := q.Query(ctx, `
		UPDATE transactional_messages SET status = 'sending', attempts = attempts + 1, claimed_at = clock_timestamp(),claim_token=gen_random_uuid(),provider_started_at=NULL
		WHERE id IN (
			SELECT id FROM transactional_messages
			WHERE status = 'queued' AND next_attempt_at <= now()
			ORDER BY next_attempt_at LIMIT $1
			FOR UPDATE SKIP LOCKED)`+claimReturning, n)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (job, error) { return scanJob(row) })
}

func sweepStuck(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var n int
	err := db.WithLock(ctx, pool, "transactional-recovery", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE transactional_messages SET status = 'failed', claimed_at = NULL, last_error = 'unknown_outcome'
			WHERE status = 'sending' AND claimed_at < now() - make_interval(secs => $1) AND (provider_started_at IS NOT NULL OR claim_token IS NULL)
			RETURNING id, workspace_id, attempts`, stuckAfter.Seconds())
		if err != nil {
			return err
		}
		type stuck struct {
			id, ws   string
			attempts int
		}
		list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (stuck, error) {
			var s stuck
			return s, row.Scan(&s.id, &s.ws, &s.attempts)
		})
		if err != nil {
			return err
		}
		for _, s := range list {
			if err := events.Record(ctx, tx, events.Event{
				WorkspaceID: s.ws, MessageID: s.id, Type: "failed",
				Detail: map[string]any{"reason": "unknown_outcome", "attempts": s.attempts},
			}); err != nil {
				return err
			}
		}
		n = len(list)
		_, err = tx.Exec(ctx, `UPDATE transactional_messages SET status='queued',claimed_at=NULL,claim_token=NULL,attempts=greatest(attempts-1,0) WHERE status='sending' AND provider_started_at IS NULL AND claim_token IS NOT NULL AND claimed_at<clock_timestamp()-interval '5 minutes'`)
		return err
	})
	return n, err
}
