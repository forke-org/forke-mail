package contacts

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/db"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

const (
	maxTagsPerRequest = 20
	maxBulkContacts   = 10000
)

var errInvalidTags = errors.New("invalid_tags")

func normalizeTags(raw []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, t := range raw {
		t = normalizeName(t)
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > 50 {
			return nil, errInvalidTags
		}
		if !seen[strings.ToLower(t)] {
			seen[strings.ToLower(t)] = true
			out = append(out, t)
		}
	}
	if len(out) > maxTagsPerRequest {
		return nil, errInvalidTags
	}
	return out, nil
}

func attachTags(ctx context.Context, q db.Querier, workspaceID string, contactIDs, names []string) error {
	if len(names) == 0 || len(contactIDs) == 0 {
		return nil
	}
	if _, err := q.Exec(ctx, `
		INSERT INTO tags (workspace_id, name) SELECT $1, unnest($2::text[])
		ON CONFLICT (workspace_id, lower(name)) DO NOTHING`, workspaceID, names); err != nil {
		return err
	}
	lower := make([]string, len(names))
	for i, n := range names {
		lower[i] = strings.ToLower(n)
	}
	_, err := q.Exec(ctx, `
		INSERT INTO contact_tags (contact_id, tag_id)
		SELECT c.id, t.id
		FROM contacts c CROSS JOIN tags t
		WHERE c.workspace_id = $1 AND c.id = ANY($2::uuid[])
			AND t.workspace_id = $1 AND lower(t.name) = ANY($3::text[])
		ON CONFLICT DO NOTHING`, workspaceID, contactIDs, lower)
	return err
}

type tagView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Contacts int    `json:"contacts"`
}

func (h *Handler) ListTags(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	rows, err := h.Pool.Query(r.Context(), `
		SELECT t.id, t.name, (SELECT COUNT(*) FROM contact_tags ct WHERE ct.tag_id = t.id)
		FROM tags t WHERE t.workspace_id = $1 ORDER BY lower(t.name)`, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (tagView, error) {
		var t tagView
		return t, row.Scan(&t.ID, &t.Name, &t.Contacts)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tags": list})
}

func (h *Handler) CreateTag(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var body struct {
		Name string `json:"name"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	names, err := normalizeTags([]string{body.Name})
	if err != nil || len(names) != 1 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_name")
		return
	}
	t := tagView{Name: names[0]}
	err = h.Pool.QueryRow(r.Context(), `
		INSERT INTO tags (workspace_id, name) VALUES ($1, $2)
		ON CONFLICT (workspace_id, lower(name)) DO NOTHING RETURNING id`, ws, t.Name).Scan(&t.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusConflict, "tag_exists")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

func (h *Handler) TagContacts(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var body struct {
		ContactIDs []string `json:"contact_ids"`
		Tags       []string `json:"tags"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	names, err := normalizeTags(body.Tags)
	if err != nil || len(names) == 0 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_tags")
		return
	}
	if len(body.ContactIDs) == 0 || len(body.ContactIDs) > maxBulkContacts || !httpx.ValidIDs(body.ContactIDs) {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_contact_ids")
		return
	}
	if err := attachTags(r.Context(), h.Pool, ws, body.ContactIDs, names); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"tags": names})
}
