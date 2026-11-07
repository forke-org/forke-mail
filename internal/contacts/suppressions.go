package contacts

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

type suppressionView struct {
	ID        string          `json:"id"`
	Email     string          `json:"email"`
	Reason    string          `json:"reason"`
	Source    string          `json:"source"`
	Detail    json.RawMessage `json:"detail"`
	CreatedAt time.Time       `json:"created_at"`
}

const selectSuppression = `SELECT id, email, reason, source, detail, created_at FROM suppressions`

func scanSuppression(row pgx.CollectableRow) (suppressionView, error) {
	var s suppressionView
	return s, row.Scan(&s.ID, &s.Email, &s.Reason, &s.Source, &s.Detail, &s.CreatedAt)
}

func (h *Handler) ListSuppressions(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	search := likePattern(r.URL.Query().Get("search"))
	limit, offset := httpx.Page(r)
	var total int
	if err := h.Pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM suppressions WHERE workspace_id = $1 AND email ILIKE $2`, ws, search).Scan(&total); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err := h.Pool.Query(r.Context(), selectSuppression+`
		WHERE workspace_id = $1 AND email ILIKE $2
		ORDER BY created_at DESC, id LIMIT $3 OFFSET $4`, ws, search, limit, offset)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, scanSuppression)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"suppressions": list, "total": total})
}

func (h *Handler) CreateSuppression(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var body struct {
		Email string `json:"email"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	email := NormalizeEmail(body.Email)
	if !ValidEmail(email) {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_email")
		return
	}
	created, err := Suppress(r.Context(), h.Pool, Suppression{WorkspaceID: ws, Email: email, Reason: "manual", Source: "manual"})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, map[string]any{"email": email, "created": created})
}

func (h *Handler) ImportSuppressions(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	parsed, ok := parseUpload(w, r)
	if !ok {
		return
	}
	emails := make([]string, len(parsed.Rows))
	for i, row := range parsed.Rows {
		emails[i] = row.Email
	}
	tag, err := h.Pool.Exec(r.Context(), `
		INSERT INTO suppressions (workspace_id, email, reason, source)
		SELECT $1, e, 'manual', 'import' FROM unnest($2::text[]) e
		ON CONFLICT (workspace_id, lower(email)) DO NOTHING`, ws, emails)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	added := int(tag.RowsAffected())
	httpx.JSON(w, http.StatusOK, map[string]int{
		"added": added, "skipped": len(emails) - added + parsed.Duplicates, "invalid": parsed.Invalid,
	})
}

func (h *Handler) DeleteSuppression(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var email string
		err := tx.QueryRow(r.Context(), `
			DELETE FROM suppressions WHERE id = $1 AND workspace_id = $2 RETURNING email`, id, ws).Scan(&email)
		if err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `
			UPDATE contacts SET status = 'active'
			WHERE workspace_id = $1 AND lower(email) = lower($2) AND status IN ('unsubscribed', 'bounced', 'complained')`, ws, email)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
