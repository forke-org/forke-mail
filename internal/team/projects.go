// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package team

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Projects struct {
	Pool        *pgxpool.Pool
	MaxProjects int
	Defaults    accounts.NewWorkspace
}

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Plan      string    `json:"plan"`
	Members   int       `json:"members"`
	Current   bool      `json:"current"`
	CreatedAt time.Time `json:"created_at"`
}

var errTooMany = errors.New("too_many_projects")

func (p *Projects) List(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	rows, err := p.Pool.Query(r.Context(), `
		SELECT w.id, w.name, m.role, w.plan, (SELECT COUNT(*) FROM workspace_members x WHERE x.workspace_id = w.id), w.id = $2, m.created_at
		FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = $1 AND w.status = 'active' ORDER BY w.platform DESC, m.created_at`, id.UserID, id.WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Project])
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"projects": list, "max_projects": p.MaxProjects})
}

func (p *Projects) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
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
	var project string
	err := pgx.BeginFunc(r.Context(), p.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtext('forke-mail-projects:' || $1))`, id.UserID); err != nil {
			return err
		}
		var owned int
		if err := tx.QueryRow(r.Context(), `SELECT COUNT(*) FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id WHERE m.user_id = $1 AND m.role = 'super_admin' AND NOT w.platform`, id.UserID).Scan(&owned); err != nil {
			return err
		}
		if owned >= p.MaxProjects {
			return errTooMany
		}
		ws := p.Defaults
		ws.Name = body.Name
		var err error
		if project, err = accounts.CreateWorkspace(r.Context(), tx, ws); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, 'super_admin')`, project, id.UserID); err != nil {
			return err
		}
		return switchSession(r, tx, id.UserID, project)
	})
	if errors.Is(err, errTooMany) {
		httpx.Error(w, http.StatusConflict, "too_many_projects")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": project})
}

func (p *Projects) Switch(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	var body struct {
		ID string `json:"id"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	if !httpx.ValidID(body.ID) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	var ok bool
	if err := p.Pool.QueryRow(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id WHERE m.user_id = $1 AND m.workspace_id = $2 AND w.status = 'active')`,
		id.UserID, body.ID).Scan(&ok); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if !ok {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err := pgx.BeginFunc(r.Context(), p.Pool, func(tx pgx.Tx) error { return switchSession(r, tx, id.UserID, body.ID) }); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func switchSession(r *http.Request, tx pgx.Tx, userID, workspaceID string) error {
	c, err := r.Cookie(auth.SessionCookie)
	if err != nil {
		return nil
	}
	_, err = tx.Exec(r.Context(), `UPDATE sessions SET workspace_id = $3 WHERE token_hash = $1 AND user_id = $2`, auth.HashKey(c.Value), userID, workspaceID)
	return err
}

var errNotEnough = errors.New("not_enough_credits")

func (p *Projects) MoveCredits(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	var body struct {
		To     string `json:"to"`
		Amount int    `json:"amount"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	if !httpx.ValidID(body.To) || body.To == id.WorkspaceID {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if body.Amount < 1 || body.Amount > 10_000_000 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_amount")
		return
	}
	err := pgx.BeginFunc(r.Context(), p.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `
			SELECT w.id, w.trial_send_limit - w.trial_sent_count FROM workspaces w
			JOIN workspace_members m ON m.workspace_id = w.id AND m.user_id = $1 AND m.role = 'super_admin'
			WHERE w.id = ANY($2::uuid[]) AND w.plan = 'trial' AND NOT w.platform AND w.status = 'active'
			ORDER BY w.id FOR UPDATE OF w`, id.UserID, []string{id.WorkspaceID, body.To})
		if err != nil {
			return err
		}
		left := map[string]int{}
		for rows.Next() {
			var ws string
			var n int
			if err := rows.Scan(&ws, &n); err != nil {
				return err
			}
			left[ws] = n
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(left) != 2 {
			return pgx.ErrNoRows
		}
		if left[id.WorkspaceID] < body.Amount {
			return errNotEnough
		}
		if _, err := tx.Exec(r.Context(), `UPDATE workspaces SET trial_send_limit = trial_send_limit - $2 WHERE id = $1`, id.WorkspaceID, body.Amount); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `UPDATE workspaces SET trial_send_limit = trial_send_limit + $2 WHERE id = $1`, body.To, body.Amount)
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.Is(err, errNotEnough):
		httpx.Error(w, http.StatusUnprocessableEntity, "not_enough_credits")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
