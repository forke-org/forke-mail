package team

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/domains"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	errPlatform    = errors.New("team_managed_by_forke")
	errOnlyProject = errors.New("only_project")
	errLastOwner   = errors.New("last_owner")
	errConfirm     = errors.New("confirmation_mismatch")
)

type Manage struct {
	Pool       *pgxpool.Pool
	Identities domains.Identities
}

var resetTables = []string{"email_events", "message_logs", "campaigns", "transactional_messages", "contacts", "tags", "topics", "templates", "workspace_alerts"}

func (m *Manage) project(ctx context.Context, q pgx.Tx, ws string) (string, bool, error) {
	var name string
	var platform bool
	err := q.QueryRow(ctx, `SELECT name, platform FROM workspaces WHERE id = $1 FOR UPDATE`, ws).Scan(&name, &platform)
	return name, platform, err
}

func (m *Manage) Rename(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || utf8.RuneCountInString(body.Name) > 100 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_workspace_name")
		return
	}
	tag, err := m.Pool.Exec(r.Context(), `UPDATE workspaces SET name = $2 WHERE id = $1 AND NOT platform`, auth.FromContext(r.Context()).WorkspaceID, body.Name)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusConflict, "team_managed_by_forke")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"name": body.Name})
}

func (m *Manage) Reset(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var body struct {
		Confirm string `json:"confirm"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	err := pgx.BeginFunc(r.Context(), m.Pool, func(tx pgx.Tx) error {
		name, platform, err := m.project(r.Context(), tx, ws)
		if err != nil {
			return err
		}
		if platform {
			return errPlatform
		}
		if strings.TrimSpace(body.Confirm) != name {
			return errConfirm
		}
		for _, table := range resetTables {
			if _, err := tx.Exec(r.Context(), `DELETE FROM `+table+` WHERE workspace_id = $1`, ws); err != nil {
				return err
			}
		}
		return nil
	})
	m.respond(w, r, err, http.StatusNoContent)
}

func (m *Manage) Delete(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	var body struct {
		Confirm string `json:"confirm"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	var names []string
	err := pgx.BeginFunc(r.Context(), m.Pool, func(tx pgx.Tx) error {
		name, platform, err := m.project(r.Context(), tx, id.WorkspaceID)
		if err != nil {
			return err
		}
		if platform {
			return errPlatform
		}
		if strings.TrimSpace(body.Confirm) != name {
			return errConfirm
		}
		var others int
		if err := tx.QueryRow(r.Context(), `SELECT COUNT(*) FROM workspace_members WHERE user_id = $1 AND workspace_id <> $2`, id.UserID, id.WorkspaceID).Scan(&others); err != nil {
			return err
		}
		if others == 0 {
			return errOnlyProject
		}
		rows, err := tx.Query(r.Context(), `SELECT name FROM domains WHERE workspace_id = $1`, id.WorkspaceID)
		if err != nil {
			return err
		}
		if names, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE sessions SET workspace_id = NULL WHERE workspace_id = $1`, id.WorkspaceID); err != nil {
			return err
		}
		_, err = accounts.DeleteWorkspace(r.Context(), tx, id.WorkspaceID)
		return err
	})
	if err == nil {
		m.release(r, names)
	}
	m.respond(w, r, err, http.StatusNoContent)
}

func (m *Manage) Leave(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	err := pgx.BeginFunc(r.Context(), m.Pool, func(tx pgx.Tx) error {
		_, platform, err := m.project(r.Context(), tx, id.WorkspaceID)
		if err != nil {
			return err
		}
		if platform {
			return errPlatform
		}
		var others, owners int
		if err := tx.QueryRow(r.Context(), `
			SELECT (SELECT COUNT(*) FROM workspace_members WHERE user_id = $1 AND workspace_id <> $2),
				(SELECT COUNT(*) FROM workspace_members WHERE workspace_id = $2 AND role = 'super_admin' AND user_id <> $1)`,
			id.UserID, id.WorkspaceID).Scan(&others, &owners); err != nil {
			return err
		}
		if others == 0 {
			return errOnlyProject
		}
		if id.Role == auth.SuperAdmin && owners == 0 {
			return errLastOwner
		}
		if _, err := tx.Exec(r.Context(), `UPDATE sessions SET workspace_id = NULL WHERE user_id = $1 AND workspace_id = $2`, id.UserID, id.WorkspaceID); err != nil {
			return err
		}
		return accounts.RemoveMember(r.Context(), tx, id.WorkspaceID, id.UserID)
	})
	m.respond(w, r, err, http.StatusNoContent)
}

func (m *Manage) SetRole(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	id := auth.FromContext(r.Context())
	var body struct {
		Role string `json:"role"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	if body.Role != auth.SuperAdmin && body.Role != auth.Admin {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_role")
		return
	}
	err := pgx.BeginFunc(r.Context(), m.Pool, func(tx pgx.Tx) error {
		_, platform, err := m.project(r.Context(), tx, id.WorkspaceID)
		if err != nil {
			return err
		}
		if platform {
			return errPlatform
		}
		if body.Role == auth.Admin {
			var owners int
			if err := tx.QueryRow(r.Context(), `SELECT COUNT(*) FROM workspace_members WHERE workspace_id = $1 AND role = 'super_admin' AND user_id <> $2`, id.WorkspaceID, userID).Scan(&owners); err != nil {
				return err
			}
			if owners == 0 {
				return errLastOwner
			}
		}
		tag, err := tx.Exec(r.Context(), `UPDATE workspace_members SET role = $3 WHERE workspace_id = $1 AND user_id = $2`, id.WorkspaceID, userID, body.Role)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		_, err = tx.Exec(r.Context(), `UPDATE users SET role = $3 WHERE id = $2 AND workspace_id = $1`, id.WorkspaceID, userID, body.Role)
		return err
	})
	m.respond(w, r, err, http.StatusNoContent)
}

func (m *Manage) release(r *http.Request, names []string) {
	if m.Identities == nil {
		return
	}
	for _, n := range names {
		if err := m.Identities.Delete(r.Context(), n); err != nil {
			slog.Warn("release identity", "name", n, "err", err)
		}
	}
}

func (m *Manage) respond(w http.ResponseWriter, r *http.Request, err error, ok int) {
	switch {
	case err == nil:
		w.WriteHeader(ok)
	case errors.Is(err, pgx.ErrNoRows):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.Is(err, errPlatform):
		httpx.Error(w, http.StatusConflict, "team_managed_by_forke")
	case errors.Is(err, errOnlyProject):
		httpx.Error(w, http.StatusConflict, "only_project")
	case errors.Is(err, errLastOwner):
		httpx.Error(w, http.StatusConflict, "last_owner")
	case errors.Is(err, errConfirm):
		httpx.Error(w, http.StatusUnprocessableEntity, "confirmation_mismatch")
	default:
		httpx.Internal(w, r, err)
	}
}
