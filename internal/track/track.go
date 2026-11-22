// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package track

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/contacts"
	"forkemail/internal/db"
	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxKeys     = 50
	maxValueLen = 500
)

var (
	eventName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,99}$`)
	fieldKey  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

type Handler struct {
	Pool *pgxpool.Pool
}

type request struct {
	Email   string         `json:"email"`
	Event   string         `json:"event"`
	Data    map[string]any `json:"data"`
	Tags    []string       `json:"tags"`
	Contact struct {
		FirstName *string        `json:"first_name"`
		LastName  *string        `json:"last_name"`
		Data      map[string]any `json:"data"`
	} `json:"contact"`
}

func validData(m map[string]any) bool {
	if len(m) > maxKeys {
		return false
	}
	for k, v := range m {
		if !fieldKey.MatchString(k) {
			return false
		}
		switch x := v.(type) {
		case string:
			if utf8.RuneCountInString(x) > maxValueLen {
				return false
			}
		case float64, bool, nil:
		default:
			return false
		}
	}
	return true
}

func (in *request) validate() string {
	in.Email = contacts.NormalizeEmail(in.Email)
	tags, tagErr := contacts.NormalizeTags(in.Tags)
	in.Tags = tags
	switch {
	case tagErr != nil:
		return "invalid_tags"
	case !contacts.ValidEmail(in.Email):
		return "invalid_email"
	case !eventName.MatchString(in.Event):
		return "invalid_event"
	case !validData(in.Data):
		return "invalid_data"
	case !validData(in.Contact.Data):
		return "invalid_contact_data"
	case in.Contact.FirstName != nil && utf8.RuneCountInString(*in.Contact.FirstName) > 100,
		in.Contact.LastName != nil && utf8.RuneCountInString(*in.Contact.LastName) > 100:
		return "invalid_name"
	}
	return ""
}

func Start(ctx context.Context, q db.Querier, ws, contactID, event string, data []byte) (int, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO workflow_runs (workflow_id, workspace_id, contact_id, event_data)
		SELECT w.id, w.workspace_id, $2, $4 FROM workflows w
		WHERE w.workspace_id = $1 AND w.enabled AND w.trigger_event = $3
			AND (w.allow_reentry OR NOT EXISTS (SELECT 1 FROM workflow_runs r WHERE r.workflow_id = w.id AND r.contact_id = $2))`,
		ws, contactID, event, data)
	return int(tag.RowsAffected()), err
}

func (h *Handler) Track(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var in request
	if !httpx.Decode(w, r, &in) {
		return
	}
	if code := in.validate(); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	if in.Data == nil {
		in.Data = map[string]any{}
	}
	if in.Contact.Data == nil {
		in.Contact.Data = map[string]any{}
	}
	eventData, _ := json.Marshal(in.Data)
	contactData, _ := json.Marshal(in.Contact.Data)
	var out struct {
		ContactID string `json:"contact_id"`
		EventID   int64  `json:"event_id"`
		Created   bool   `json:"contact_created"`
		Workflows int    `json:"workflows_started"`
	}
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(r.Context(), `
			INSERT INTO contacts (workspace_id, email, first_name, last_name, data)
			VALUES ($1, $2, COALESCE($3, ''), COALESCE($4, ''), $5)
			ON CONFLICT (workspace_id, lower(email)) DO UPDATE SET
				first_name = COALESCE($3, contacts.first_name), last_name = COALESCE($4, contacts.last_name),
				data = CASE WHEN (SELECT COUNT(*) FROM jsonb_object_keys(contacts.data || $5)) <= 100 THEN contacts.data || $5 END
			RETURNING id, xmax = 0`, ws, in.Email, in.Contact.FirstName, in.Contact.LastName, contactData).Scan(&out.ContactID, &out.Created)
		if err != nil {
			return err
		}
		if err := contacts.AttachTags(r.Context(), tx, ws, out.ContactID, in.Tags); err != nil {
			return err
		}
		if err := tx.QueryRow(r.Context(), `INSERT INTO contact_events (workspace_id, contact_id, name, data) VALUES ($1, $2, $3, $4) RETURNING id`,
			ws, out.ContactID, in.Event, eventData).Scan(&out.EventID); err != nil {
			return err
		}
		out.Workflows, err = Start(r.Context(), tx, ws, out.ContactID, in.Event, eventData)
		return err
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23502" {
			httpx.Error(w, http.StatusUnprocessableEntity, "too_many_contact_fields")
			return
		}
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, out)
}

type Event struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Email     string          `json:"email"`
	ContactID string          `json:"contact_id"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	name := r.URL.Query().Get("name")
	contact := r.URL.Query().Get("contact_id")
	if contact != "" && !httpx.ValidID(contact) {
		httpx.Error(w, http.StatusBadRequest, "invalid_contact_id")
		return
	}
	limit, offset := httpx.Page(r)
	rows, err := h.Pool.Query(r.Context(), `
		SELECT e.id, e.name, c.email, c.id, e.data, e.created_at FROM contact_events e JOIN contacts c ON c.id = e.contact_id
		WHERE e.workspace_id = $1 AND ($2 = '' OR e.name = $2) AND ($3 = '' OR e.contact_id = NULLIF($3, '')::uuid)
		ORDER BY e.id DESC LIMIT $4 OFFSET $5`, ws, name, contact, limit, offset)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Event])
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"events": list})
}

type usage struct {
	Name     string     `json:"name"`
	Count    int        `json:"count"`
	LastSeen *time.Time `json:"last_seen,omitempty"`
	UsedBy   []string   `json:"used_by"`
}

func (h *Handler) usedBy(ctx context.Context, ws, kind, name string) ([]string, error) {
	rows, err := h.Pool.Query(ctx, `
		SELECT 'segment: ' || s.name FROM segments s WHERE s.workspace_id = $1 AND CASE WHEN $2 = 'event'
			THEN jsonb_path_exists(s.rules, '$.** ? (@.field == "event" && @.value == $v)', jsonb_build_object('v', $3::text))
			ELSE jsonb_path_exists(s.rules, '$.** ? (@.field == $f)', jsonb_build_object('f', 'data.' || $3)) END
		UNION ALL
		SELECT 'workflow: ' || w.name FROM workflows w WHERE w.workspace_id = $1 AND $2 = 'event' AND w.trigger_event = $3`, ws, kind, name)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (h *Handler) Data(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	rows, err := h.Pool.Query(r.Context(), `
		SELECT k, COUNT(*)::int FROM contacts c, jsonb_object_keys(c.data) k WHERE c.workspace_id = $1 GROUP BY k ORDER BY k`, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	fields, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (usage, error) {
		var u usage
		return u, row.Scan(&u.Name, &u.Count)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err = h.Pool.Query(r.Context(), `
		SELECT name, COUNT(*)::int, MAX(created_at) FROM contact_events WHERE workspace_id = $1 GROUP BY name ORDER BY MAX(created_at) DESC`, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (usage, error) {
		var u usage
		return u, row.Scan(&u.Name, &u.Count, &u.LastSeen)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	for i := range fields {
		if fields[i].UsedBy, err = h.usedBy(r.Context(), ws, "field", fields[i].Name); err != nil {
			httpx.Internal(w, r, err)
			return
		}
	}
	for i := range events {
		if events[i].UsedBy, err = h.usedBy(r.Context(), ws, "event", events[i].Name); err != nil {
			httpx.Internal(w, r, err)
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"fields": fields, "events": events})
}

func (h *Handler) DeleteField(w http.ResponseWriter, r *http.Request) {
	h.remove(w, r, "field", chi.URLParam(r, "name"), fieldKey, `UPDATE contacts SET data = data - $2 WHERE workspace_id = $1 AND data ? $2`)
}

func (h *Handler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	h.remove(w, r, "event", chi.URLParam(r, "name"), eventName, `DELETE FROM contact_events WHERE workspace_id = $1 AND name = $2`)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request, kind, name string, pattern *regexp.Regexp, query string) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	if !pattern.MatchString(name) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	used, err := h.usedBy(r.Context(), ws, kind, name)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if len(used) > 0 {
		httpx.JSON(w, http.StatusConflict, map[string]any{"error": "in_use", "used_by": used})
		return
	}
	if _, err := h.Pool.Exec(r.Context(), query, ws, name); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
