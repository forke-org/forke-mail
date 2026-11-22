// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package segments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/db"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Segment struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Match       string     `json:"match"`
	Rules       []Rule     `json:"rules"`
	Size        int        `json:"size"`
	RefreshedAt *time.Time `json:"refreshed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func Refresh(ctx context.Context, q db.Querier, ws, id string) (int, error) {
	var match string
	var raw []byte
	err := q.QueryRow(ctx, `SELECT match, rules FROM segments WHERE id = $1 AND workspace_id = $2`, id, ws).Scan(&match, &raw)
	if err != nil {
		return 0, err
	}
	var rules []Rule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return 0, err
	}
	cond, args, err := Compile(match, rules, 2)
	if err != nil {
		return 0, err
	}
	if _, err := q.Exec(ctx, `DELETE FROM segment_contacts WHERE segment_id = $1`, id); err != nil {
		return 0, err
	}
	tag, err := q.Exec(ctx, `INSERT INTO segment_contacts (segment_id, contact_id) SELECT $1, c.id FROM contacts c WHERE c.workspace_id = $2 AND `+cond,
		append([]any{id, ws}, args...)...)
	if err != nil {
		return 0, err
	}
	size := int(tag.RowsAffected())
	_, err = q.Exec(ctx, `UPDATE segments SET size = $2, refreshed_at = now() WHERE id = $1`, id, size)
	return size, err
}

func RefreshForEmail(ctx context.Context, q db.Querier, ws, emailID string) error {
	rows, err := q.Query(ctx, `SELECT segment_id FROM email_segments es JOIN segments s ON s.id = es.segment_id WHERE es.email_id = $1 AND s.workspace_id = $2`, emailID, ws)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := Refresh(ctx, q, ws, id); err != nil {
			return err
		}
	}
	return nil
}

type Handler struct {
	Pool *pgxpool.Pool
}

type input struct {
	Name  string `json:"name"`
	Match string `json:"match"`
	Rules []Rule `json:"rules"`
}

func (in *input) validate() string {
	in.Name = strings.TrimSpace(in.Name)
	if in.Match == "" {
		in.Match = "all"
	}
	if in.Rules == nil {
		in.Rules = []Rule{}
	}
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 100 {
		return "invalid_name"
	}
	if _, _, err := Compile(in.Match, in.Rules, 0); err != nil {
		return err.Error()
	}
	return ""
}

const columns = `id, name, match, rules, size, refreshed_at, created_at, updated_at`

func scan(row pgx.Row) (Segment, error) {
	var s Segment
	var raw []byte
	if err := row.Scan(&s.ID, &s.Name, &s.Match, &raw, &s.Size, &s.RefreshedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return s, err
	}
	return s, json.Unmarshal(raw, &s.Rules)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), `SELECT `+columns+` FROM segments WHERE workspace_id = $1 ORDER BY name`, auth.FromContext(r.Context()).WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Segment, error) { return scan(row) })
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"segments": list})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	s, err := scan(h.Pool.QueryRow(r.Context(), `SELECT `+columns+` FROM segments WHERE id = $1 AND workspace_id = $2`, id, auth.FromContext(r.Context()).WorkspaceID))
	h.respond(w, r, s, err, http.StatusOK)
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request, id string) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var in input
	if !httpx.Decode(w, r, &in) {
		return
	}
	if code := in.validate(); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	rules, _ := json.Marshal(in.Rules)
	var s Segment
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var err error
		if id == "" {
			err = tx.QueryRow(r.Context(), `INSERT INTO segments (workspace_id, name, match, rules) VALUES ($1, $2, $3, $4) RETURNING id`, ws, in.Name, in.Match, rules).Scan(&id)
		} else {
			err = tx.QueryRow(r.Context(), `UPDATE segments SET name = $3, match = $4, rules = $5, updated_at = now() WHERE id = $1 AND workspace_id = $2 RETURNING id`, id, ws, in.Name, in.Match, rules).Scan(&id)
		}
		if err != nil {
			return err
		}
		if _, err := Refresh(r.Context(), tx, ws, id); err != nil {
			return err
		}
		s, err = scan(tx.QueryRow(r.Context(), `SELECT `+columns+` FROM segments WHERE id = $1`, id))
		return err
	})
	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusCreated
	}
	h.respond(w, r, s, err, status)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) { h.save(w, r, "") }

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	h.save(w, r, id)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM segments WHERE id = $1 AND workspace_id = $2`, id, auth.FromContext(r.Context()).WorkspaceID)
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

type member struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var in input
	if !httpx.Decode(w, r, &in) {
		return
	}
	in.Name = "preview"
	if code := in.validate(); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	cond, args, _ := Compile(in.Match, in.Rules, 1)
	all := append([]any{ws}, args...)
	var size int
	if err := h.Pool.QueryRow(r.Context(), `SELECT COUNT(*) FROM contacts c WHERE c.workspace_id = $1 AND `+cond, all...).Scan(&size); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err := h.Pool.Query(r.Context(), `SELECT c.id, c.email, c.first_name, c.last_name, c.status, c.created_at FROM contacts c WHERE c.workspace_id = $1 AND `+cond+` ORDER BY c.created_at DESC LIMIT 10`, all...)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	sample, err := pgx.CollectRows(rows, pgx.RowToStructByPos[member])
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"size": size, "sample": sample})
}

func (h *Handler) Contacts(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ws := auth.FromContext(r.Context()).WorkspaceID
	limit, offset := httpx.Page(r)
	rows, err := h.Pool.Query(r.Context(), `
		SELECT c.id, c.email, c.first_name, c.last_name, c.status, c.created_at
		FROM segment_contacts sc JOIN contacts c ON c.id = sc.contact_id JOIN segments s ON s.id = sc.segment_id
		WHERE sc.segment_id = $1 AND s.workspace_id = $2 ORDER BY c.created_at DESC LIMIT $3 OFFSET $4`, id, ws, limit, offset)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[member])
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"contacts": list})
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, s Segment, err error, status int) {
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		httpx.Error(w, http.StatusConflict, "segment_name_taken")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		httpx.JSON(w, status, s)
	}
}
