// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package oauth

import (
	"net/http"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

type connection struct {
	ID         string     `json:"id"`
	App        string     `json:"app"`
	Project    string     `json:"project"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type approval struct {
	ID          string    `json:"id"`
	Summary     string    `json:"summary"`
	Tool        string    `json:"tool"`
	RequestedBy string    `json:"requested_by"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *Server) Connections(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	rows, err := s.Pool.Query(r.Context(), `
		SELECT g.id, c.name, w.name, g.created_at, g.last_used_at FROM oauth_grants g
		JOIN oauth_clients c ON c.id = g.client_id JOIN workspaces w ON w.id = g.workspace_id
		WHERE g.user_id = $1 AND g.refresh_expires_at > now() ORDER BY g.created_at DESC`, id.UserID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	conns, err := pgx.CollectRows(rows, pgx.RowToStructByPos[connection])
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err = s.Pool.Query(r.Context(), `
		SELECT a.id, a.summary, a.tool, a.requested_by,
			CASE WHEN a.status = 'pending' AND a.expires_at < now() THEN 'expired' ELSE a.status END, a.created_at
		FROM agent_approvals a WHERE a.workspace_id = $1 ORDER BY a.created_at DESC LIMIT 20`, id.WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	approvals, err := pgx.CollectRows(rows, pgx.RowToStructByPos[approval])
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"connections": conns, "approvals": approvals, "mcp_url": s.BaseURL + "/mcp"})
}

func (s *Server) Disconnect(w http.ResponseWriter, r *http.Request) {
	gid, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	tag, err := s.Pool.Exec(r.Context(), `DELETE FROM oauth_grants WHERE id = $1 AND user_id = $2`, gid, auth.FromContext(r.Context()).UserID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
