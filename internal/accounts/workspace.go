package accounts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"forkemail/internal/workspaces"

	"github.com/jackc/pgx/v5"
)

var ErrEmailTaken = errors.New("email_taken")

type NewWorkspace struct {
	Name           string
	Plan           string
	DailyLimit     int
	RatePerSec     int
	TrialSendLimit int
	Warmup         bool
}

func CreateWorkspace(ctx context.Context, tx pgx.Tx, ws NewWorkspace) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO workspaces (name, plan, daily_limit, rate_limit_per_sec, trial_send_limit, warmup_until)
		VALUES ($1, $2, $3, $4, $5, CASE WHEN $6 THEN now() + make_interval(days => $7) END) RETURNING id`,
		ws.Name, ws.Plan, ws.DailyLimit, ws.RatePerSec, ws.TrialSendLimit, ws.Warmup, workspaces.WarmupDays).Scan(&id)
	return id, err
}

func AddVerifiedIdentity(ctx context.Context, tx pgx.Tx, workspaceID, kind, name string) error {
	tokens := []string{}
	if kind == "domain" {
		tokens = []string{randomHex(16), randomHex(16), randomHex(16)}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO domains (workspace_id, name, kind, status, dkim_tokens, verified_at) VALUES ($1, $2, $3, 'verified', $4, now())`,
		workspaceID, name, kind, tokens)
	return err
}

func CreateUser(ctx context.Context, tx pgx.Tx, workspaceID, name, email, password string) (string, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (workspace_id, name, email, password_hash) VALUES ($1, $2, lower($3), $4)
		ON CONFLICT (lower(email)) DO NOTHING RETURNING id`, workspaceID, name, email, hash).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", ErrEmailTaken
	}
	return id, err
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
