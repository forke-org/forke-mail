// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package accounts

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func RemoveMember(ctx context.Context, tx pgx.Tx, workspaceID, userID string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`, workspaceID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND workspace_id = $2`, userID, workspaceID); err != nil {
		return err
	}
	return rehome(ctx, tx, workspaceID, userID)
}

func DeleteWorkspace(ctx context.Context, tx pgx.Tx, workspaceID string) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT user_id FROM workspace_members WHERE workspace_id = $1`, workspaceID)
	if err != nil {
		return false, err
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1`, workspaceID); err != nil {
		return false, err
	}
	for _, u := range users {
		if err := rehome(ctx, tx, workspaceID, u); err != nil {
			return false, err
		}
	}
	tag, err := tx.Exec(ctx, `DELETE FROM workspaces WHERE id = $1 AND NOT platform`, workspaceID)
	return tag.RowsAffected() > 0, err
}

func rehome(ctx context.Context, tx pgx.Tx, workspaceID, userID string) error {
	var home string
	if err := tx.QueryRow(ctx, `SELECT workspace_id FROM users WHERE id = $1`, userID).Scan(&home); err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	if home != workspaceID {
		return nil
	}
	var next string
	err := tx.QueryRow(ctx, `SELECT workspace_id FROM workspace_members WHERE user_id = $1 ORDER BY created_at LIMIT 1`, userID).Scan(&next)
	if err == pgx.ErrNoRows {
		_, err = tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		return err
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE users SET workspace_id = $2, role = (SELECT role FROM workspace_members WHERE user_id = $1 AND workspace_id = $2) WHERE id = $1`, userID, next)
	return err
}
