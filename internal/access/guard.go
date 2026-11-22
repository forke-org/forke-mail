// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package access

import (
	"encoding/json"
	"net/http"
	"time"

	"forkemail/internal/guard"
	"forkemail/internal/httpx"
)

func (p *Platform) GuardStatus(w http.ResponseWriter, r *http.Request) {
	var out struct {
		Active      bool            `json:"active"`
		Reason      string          `json:"reason"`
		Detail      json.RawMessage `json:"detail"`
		ActivatedAt *time.Time      `json:"activated_at"`
		ClearedAt   *time.Time      `json:"cleared_at"`
		CheckedAt   *time.Time      `json:"checked_at"`
		Held        int             `json:"held_projects"`
		Windows     []guard.Window  `json:"windows"`
	}
	err := p.Pool.QueryRow(r.Context(), `
		SELECT active, reason, detail, activated_at, cleared_at, checked_at,
			CASE WHEN active THEN (SELECT COUNT(*) FROM workspaces w WHERE workspace_held(w.id)) ELSE 0 END
		FROM platform_guard WHERE id = 1`).Scan(&out.Active, &out.Reason, &out.Detail, &out.ActivatedAt, &out.ClearedAt, &out.CheckedAt, &out.Held)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if out.Windows, err = guard.Windows(r.Context(), p.Pool); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (p *Platform) GuardClear(w http.ResponseWriter, r *http.Request) {
	if _, err := p.Pool.Exec(r.Context(), `UPDATE platform_guard SET active = false, cleared_at = now() WHERE id = 1`); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
