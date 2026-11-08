package contacts

import (
	"context"

	"forkemail/internal/db"
)

type Suppression struct {
	WorkspaceID string
	Email       string
	Reason      string
	Source      string
	Detail      map[string]any
}

func Suppress(ctx context.Context, q db.Querier, s Suppression) (bool, error) {
	if s.Detail == nil {
		s.Detail = map[string]any{}
	}
	inserted, err := q.Exec(ctx, `
		INSERT INTO suppressions (workspace_id, email, reason, source, detail) VALUES ($1, lower($2), $3, $4, $5)
		ON CONFLICT (workspace_id, lower(email)) DO NOTHING`, s.WorkspaceID, s.Email, s.Reason, s.Source, s.Detail)
	if err != nil || s.Reason == "manual" {
		return inserted.RowsAffected() > 0, err
	}
	updated, err := q.Exec(ctx, `
		UPDATE contacts SET status = $3
		WHERE workspace_id = $1 AND lower(email) = lower($2) AND status = 'active'`, s.WorkspaceID, s.Email, s.Reason)
	return inserted.RowsAffected()+updated.RowsAffected() > 0, err
}
