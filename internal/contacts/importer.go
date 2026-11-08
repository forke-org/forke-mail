package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ImportResult struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Invalid int `json:"invalid"`
}

type ImportTarget struct {
	Tags    []string
	EmailID string
}

func Import(ctx context.Context, pool *pgxpool.Pool, workspaceID string, parsed Parsed, target ImportTarget) (ImportResult, error) {
	res := ImportResult{Invalid: parsed.Invalid, Skipped: parsed.Duplicates}
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := lockDraftEmail(ctx, tx, workspaceID, target.EmailID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE import_rows (email text, first_name text, last_name text) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"import_rows"}, []string{"email", "first_name", "last_name"},
			pgx.CopyFromSlice(len(parsed.Rows), func(i int) ([]any, error) {
				r := parsed.Rows[i]
				return []any{r.Email, r.FirstName, r.LastName}, nil
			})); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			INSERT INTO contacts (workspace_id, email, first_name, last_name)
			SELECT $1, i.email, i.first_name, i.last_name FROM import_rows i
			WHERE NOT EXISTS (SELECT 1 FROM suppressions s WHERE s.workspace_id = $1 AND lower(s.email) = i.email)
			ON CONFLICT (workspace_id, lower(email)) DO UPDATE SET
				first_name = CASE WHEN EXCLUDED.first_name <> '' THEN EXCLUDED.first_name ELSE contacts.first_name END,
				last_name = CASE WHEN EXCLUDED.last_name <> '' THEN EXCLUDED.last_name ELSE contacts.last_name END
			WHERE contacts.status = 'active'
			RETURNING id, xmax = 0`, workspaceID)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			var inserted bool
			if err := rows.Scan(&id, &inserted); err != nil {
				return err
			}
			ids = append(ids, id)
			if inserted {
				res.Added++
			} else {
				res.Updated++
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		res.Skipped += len(parsed.Rows) - len(ids)
		if target.EmailID != "" {
			if _, err := tx.Exec(ctx, `
				INSERT INTO email_contacts (email_id, contact_id)
				SELECT $1, unnest($2::uuid[]) ON CONFLICT DO NOTHING`, target.EmailID, ids); err != nil {
				return err
			}
		}
		return attachTags(ctx, tx, workspaceID, ids, target.Tags)
	})
	return res, err
}

func lockDraftEmail(ctx context.Context, tx pgx.Tx, workspaceID, emailID string) error {
	if emailID == "" {
		return nil
	}
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM emails WHERE id = $1 AND workspace_id = $2 FOR UPDATE`, emailID, workspaceID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errEmailNotFound
	}
	if err == nil && status != "draft" {
		return errEmailNotDraft
	}
	return err
}
