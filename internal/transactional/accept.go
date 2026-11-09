package transactional

import (
	"context"
	"errors"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/domains"
	"forkemail/internal/events"
	"forkemail/internal/workspaces"

	"github.com/jackc/pgx/v5"
)

const (
	idempotencyWindow = 24 * time.Hour
	duplicateWindow   = 60 * time.Second
)

const (
	replayedByKey     = "idempotency_key"
	replayedByContent = "duplicate_content"
)

type accepted struct {
	id       string
	replayed string
}

func accept(ctx context.Context, tx pgx.Tx, who auth.Identity, key string, req request) (accepted, error) {
	ws := who.WorkspaceID
	fingerprint := req.fingerprint()
	if key != "" {
		if err := lock(ctx, tx, ws+"|key|"+key); err != nil {
			return accepted{}, err
		}
		var legacy bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM transactional_requests WHERE workspace_id=$1 AND idempotency_key=$2 AND expires_at>now())`, ws, key).Scan(&legacy); err != nil {
			return accepted{}, err
		}
		if legacy {
			return accepted{}, errLegacyKey
		}
		if prior, err := byIdempotencyKey(ctx, tx, ws, key, req.hash()); err != nil || prior.id != "" {
			return prior, err
		}
	} else {
		if err := lock(ctx, tx, ws+"|content|"+fingerprint); err != nil {
			return accepted{}, err
		}
		if prior, err := recentDuplicate(ctx, tx, ws, fingerprint); err != nil || prior.id != "" {
			return prior, err
		}
	}
	if err := checkSendable(ctx, tx, ws, req); err != nil {
		return accepted{}, err
	}
	res, err := workspaces.Reserve(ctx, tx, ws)
	if err != nil {
		return accepted{}, err
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO transactional_messages (workspace_id, api_key_id, idempotency_key, request_hash, fingerprint,
			to_address, from_name, from_address, reply_to, subject, html, text, tags, track_opens, track_clicks, quota_day, quota_trial)
		VALUES ($1, NULLIF($2, '')::uuid, NULLIF($3, ''), $4, $5, $6, $7, $8, NULLIF($9, ''), $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING id`,
		ws, who.KeyID, key, req.hash(), fingerprint, req.To, req.FromName, req.FromAddress, req.ReplyTo, req.Subject,
		req.HTML, req.Text, req.Tags, req.TrackOpens, req.TrackClicks, res.Day, res.Trial).Scan(&id)
	if err != nil {
		return accepted{}, err
	}
	return accepted{id: id}, events.Record(ctx, tx, events.Event{WorkspaceID: ws, MessageID: id, Type: "queued"})
}

func lock(ctx context.Context, tx pgx.Tx, name string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, name)
	return err
}

func byIdempotencyKey(ctx context.Context, tx pgx.Tx, ws, key, hash string) (accepted, error) {
	var id, storedHash string
	var fresh bool
	err := tx.QueryRow(ctx, `
		SELECT id, request_hash, created_at > now() - make_interval(secs => $3)
		FROM transactional_messages WHERE workspace_id = $1 AND idempotency_key = $2`,
		ws, key, idempotencyWindow.Seconds()).Scan(&id, &storedHash, &fresh)
	if errors.Is(err, pgx.ErrNoRows) {
		return accepted{}, nil
	}
	if err != nil {
		return accepted{}, err
	}
	if !fresh {
		_, err = tx.Exec(ctx, `UPDATE transactional_messages SET idempotency_key = NULL WHERE id = $1`, id)
		return accepted{}, err
	}
	if storedHash != hash {
		return accepted{}, errKeyReused
	}
	return accepted{id: id, replayed: replayedByKey}, nil
}

func recentDuplicate(ctx context.Context, tx pgx.Tx, ws, fingerprint string) (accepted, error) {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT id FROM transactional_messages
		WHERE workspace_id = $1 AND fingerprint = $2 AND status <> 'failed'
			AND created_at > now() - make_interval(secs => $3)
		ORDER BY created_at DESC LIMIT 1`, ws, fingerprint, duplicateWindow.Seconds()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return accepted{}, nil
	}
	return accepted{id: id, replayed: replayedByContent}, err
}

func checkSendable(ctx context.Context, tx pgx.Tx, ws string, req request) error {
	var status string
	var paused, suppressed bool
	if err := tx.QueryRow(ctx, `
		SELECT status, sending_paused, EXISTS (
			SELECT 1 FROM suppressions s WHERE s.workspace_id = w.id AND lower(s.email) = lower($2)
				AND s.reason IN ('bounced', 'complained', 'manual'))
		FROM workspaces w WHERE id = $1`, ws, req.To).Scan(&status, &paused, &suppressed); err != nil {
		return err
	}
	switch {
	case status != "active":
		return errWorkspaceInactive
	case paused:
		return errSendingPaused
	case suppressed:
		return errSuppressed
	}
	verified, err := domains.Verified(ctx, tx, ws, req.FromAddress)
	if err == nil && !verified {
		return errDomainUnverified
	}
	return err
}
