package contacts

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	errEmailNotFound = errors.New("email_not_found")
	errEmailNotDraft = errors.New("email_not_draft")
	statuses         = []string{"active", "unsubscribed", "bounced", "complained", "invalid"}
)

type Handler struct {
	Pool *pgxpool.Pool
}

type contactView struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Status    string    `json:"status"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}

func scanContact(row pgx.CollectableRow) (contactView, error) {
	var c contactView
	return c, row.Scan(&c.ID, &c.Email, &c.FirstName, &c.LastName, &c.Status, &c.Tags, &c.CreatedAt)
}

const contactColumns = `c.id, c.email, c.first_name, c.last_name, c.status,
	ARRAY(SELECT t.name FROM contact_tags ct JOIN tags t ON t.id = ct.tag_id WHERE ct.contact_id = c.id ORDER BY lower(t.name)),
	c.created_at`

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	status := r.URL.Query().Get("status")
	if status != "" && !slices.Contains(statuses, status) {
		httpx.Error(w, http.StatusBadRequest, "invalid_status")
		return
	}
	tag := r.URL.Query().Get("tag_id")
	if tag != "" && !httpx.ValidID(tag) {
		httpx.Error(w, http.StatusBadRequest, "invalid_tag_id")
		return
	}
	search := likePattern(r.URL.Query().Get("search"))
	limit, offset := httpx.Page(r)
	const filter = `c.workspace_id = $1 AND ($2 = '' OR c.status = $2)
		AND ($3 = '%%' OR c.email ILIKE $3 OR c.first_name ILIKE $3 OR c.last_name ILIKE $3)
		AND ($4 = '' OR EXISTS (SELECT 1 FROM contact_tags ct WHERE ct.contact_id = c.id AND ct.tag_id = NULLIF($4, '')::uuid))`
	var total int
	if err := h.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM contacts c WHERE `+filter, ws, status, search, tag).Scan(&total); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT `+contactColumns+` FROM contacts c WHERE `+filter+`
		ORDER BY c.created_at DESC, c.id LIMIT $5 OFFSET $6`, ws, status, search, tag, limit, offset)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, scanContact)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"contacts": list, "total": total})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM contacts WHERE id = $1 AND workspace_id = $2`, id, ws)
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

func likePattern(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
	return "%" + s + "%"
}
