package compatapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"forkemail/internal/auth"
	"forkemail/internal/contacts"
	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type audience struct {
	Object string `json:"object"`
	ID     string `json:"id"`
	Name   string `json:"name"`
}

func (h *Handler) ListAudiences(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	rows, err := h.Pool.Query(r.Context(), `SELECT id, name FROM tags WHERE workspace_id = $1 ORDER BY lower(name)`, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (audience, error) {
		a := audience{Object: "audience"}
		return a, row.Scan(&a.ID, &a.Name)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"object": "list", "data": list})
}

func (h *Handler) CreateAudience(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	c := delegate(h.Tags, r, body, "")
	a := audience{Object: "audience"}
	switch {
	case c.ok():
		_ = json.Unmarshal(c.body.Bytes(), &a)
	case c.code() == "tag_exists":
		err := h.Pool.QueryRow(r.Context(), `SELECT id, name FROM tags WHERE workspace_id = $1 AND lower(name) = lower($2)`,
			auth.FromContext(r.Context()).WorkspaceID, body.Name).Scan(&a.ID, &a.Name)
		if err != nil {
			httpx.Internal(w, r, err)
			return
		}
	default:
		relay(w, c)
		return
	}
	a.Object = "audience"
	httpx.JSON(w, http.StatusCreated, a)
}

func (h *Handler) AddContact(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var body struct {
		Email        string `json:"email"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		Unsubscribed bool   `json:"unsubscribed"`
	}
	if !decode(w, r, &body) {
		return
	}
	email := contacts.NormalizeEmail(body.Email)
	if !contacts.ValidEmail(email) {
		fail(w, http.StatusUnprocessableEntity, "invalid_email")
		return
	}
	audienceID := chi.URLParam(r, "id")
	var tagName string
	err := pgx.ErrNoRows
	if httpx.ValidID(audienceID) {
		err = h.Pool.QueryRow(r.Context(), `SELECT name FROM tags WHERE id = $1 AND workspace_id = $2`, audienceID, ws).Scan(&tagName)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "audience_not_found")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	parsed := contacts.Parsed{Rows: []contacts.Row{{Email: email, FirstName: body.FirstName, LastName: body.LastName}}}
	if _, err := contacts.Import(r.Context(), h.Pool, ws, parsed, contacts.ImportTarget{Tags: []string{tagName}}); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if body.Unsubscribed {
		if _, err := contacts.Suppress(r.Context(), h.Pool, contacts.Suppression{WorkspaceID: ws, Email: email, Reason: "unsubscribed", Source: "import"}); err != nil {
			httpx.Internal(w, r, err)
			return
		}
	}
	var id string
	err = h.Pool.QueryRow(r.Context(), `SELECT id FROM contacts WHERE workspace_id = $1 AND lower(email) = $2`, ws, email).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"object": "contact", "id": id})
}

func (h *Handler) RemoveContact(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	audienceID := chi.URLParam(r, "id")
	if !httpx.ValidID(audienceID) {
		fail(w, http.StatusNotFound, "audience_not_found")
		return
	}
	var exists bool
	if err := h.Pool.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM tags WHERE id = $1 AND workspace_id = $2)`, audienceID, ws).Scan(&exists); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if !exists {
		fail(w, http.StatusNotFound, "audience_not_found")
		return
	}
	ref, err := url.PathUnescape(chi.URLParam(r, "contact"))
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "invalid_email")
		return
	}
	var id string
	if httpx.ValidID(ref) {
		err = h.Pool.QueryRow(r.Context(), `DELETE FROM contacts WHERE id = $1 AND workspace_id = $2 RETURNING id`, ref, ws).Scan(&id)
	} else {
		email := contacts.NormalizeEmail(ref)
		if !contacts.ValidEmail(email) {
			fail(w, http.StatusUnprocessableEntity, "invalid_email")
			return
		}
		err = h.Pool.QueryRow(r.Context(), `DELETE FROM contacts WHERE workspace_id = $1 AND lower(email) = $2 RETURNING id`, ws, email).Scan(&id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, http.StatusNotFound, "contact_not_found")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"object": "contact", "contact": id, "deleted": true})
}
