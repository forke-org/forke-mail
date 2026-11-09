package accounts

import (
	"context"
	"errors"

	"forkemail/internal/forke"

	"github.com/jackc/pgx/v5"
)

var errAdminEmailTaken = errors.New("a local user already owns this admin's email")

func ForkeWorkspace(ctx context.Context, tx pgx.Tx, name string) (string, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('forke-mail-workspace'))`); err != nil {
		return "", err
	}
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM workspaces WHERE name = $1 ORDER BY created_at LIMIT 1`, name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreateWorkspace(ctx, tx, NewWorkspace{Name: name, Plan: "paid", DailyLimit: 100000, RatePerSec: 14})
	}
	return id, err
}

func UpsertAdmin(ctx context.Context, tx pgx.Tx, workspaceID string, a forke.Admin) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		INSERT INTO users (workspace_id, name, email, forke_admin_id, role)
		VALUES ($1, left($2, 100), lower($3), $4, $5)
		ON CONFLICT (forke_admin_id) WHERE forke_admin_id IS NOT NULL
		DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email, role = EXCLUDED.role
		RETURNING id`, workspaceID, a.Name, a.Email, a.ID, a.Role).Scan(&id)
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
		return "", errAdminEmailTaken
	}
	return id, err
}
