package templates

import (
	"context"
	"errors"
	"html"
	"net/http"
	"regexp"
	"sort"
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

const maxBody = 256 << 10

var placeholder = regexp.MustCompile(`\{\{\{\s*([A-Za-z_][A-Za-z0-9_.]{0,63})\s*\}\}\}|\{\{\s*([A-Za-z_][A-Za-z0-9_.]{0,63})\s*\}\}`)

func key(m []string) (string, bool) {
	if m[1] != "" {
		return m[1], true
	}
	return m[2], false
}

var ErrNotFound = errors.New("template_not_found")

type Template struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Subject   string    `json:"subject"`
	HTML      string    `json:"html"`
	Text      string    `json:"text"`
	Variables []string  `json:"variables"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func Variables(parts ...string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range parts {
		for _, m := range placeholder.FindAllStringSubmatch(p, -1) {
			k, _ := key(m)
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func fill(s string, data map[string]string, escape bool) string {
	return placeholder.ReplaceAllStringFunc(s, func(match string) string {
		k, raw := key(placeholder.FindStringSubmatch(match))
		v, ok := data[k]
		if !ok {
			if raw {
				return ""
			}
			return match
		}
		if escape && !raw {
			return html.EscapeString(v)
		}
		return v
	})
}

func Render(ctx context.Context, q db.Querier, workspaceID, ref string, data map[string]string) (subject, body, text string, err error) {
	if httpx.ValidID(ref) {
		err = q.QueryRow(ctx, `SELECT subject, html, text FROM templates WHERE id = $1 AND workspace_id = $2`, ref, workspaceID).Scan(&subject, &body, &text)
	} else {
		err = q.QueryRow(ctx, `SELECT subject, html, text FROM templates WHERE lower(name) = lower($1) AND workspace_id = $2`, ref, workspaceID).Scan(&subject, &body, &text)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", err
	}
	if _, ok := data["date"]; !ok {
		withDate := make(map[string]string, len(data)+1)
		for k, v := range data {
			withDate[k] = v
		}
		withDate["date"] = time.Now().UTC().Format("2 Jan 2006")
		data = withDate
	}
	subject = strings.Join(strings.Fields(fill(subject, data, false)), " ")
	return subject, fill(body, data, true), fill(text, data, false), nil
}

type Handler struct {
	Pool *pgxpool.Pool
}

type input struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
}

func (in *input) validate() string {
	in.Name = strings.TrimSpace(in.Name)
	in.Subject = strings.TrimSpace(in.Subject)
	if in.Kind == "" {
		in.Kind = "transactional"
	}
	switch {
	case in.Name == "" || utf8.RuneCountInString(in.Name) > 100:
		return "invalid_name"
	case in.Kind != "transactional" && in.Kind != "marketing":
		return "invalid_kind"
	case in.Subject == "" || utf8.RuneCountInString(in.Subject) > 300 || strings.ContainsAny(in.Subject, "\r\n"):
		return "invalid_subject"
	case strings.TrimSpace(in.HTML) == "" && strings.TrimSpace(in.Text) == "":
		return "missing_body"
	case len(in.HTML) > maxBody || len(in.Text) > maxBody:
		return "body_too_large"
	}
	return ""
}

const columns = `id, name, kind, subject, html, text, created_at, updated_at`

func scan(row pgx.Row) (Template, error) {
	var t Template
	err := row.Scan(&t.ID, &t.Name, &t.Kind, &t.Subject, &t.HTML, &t.Text, &t.CreatedAt, &t.UpdatedAt)
	t.Variables = Variables(t.Subject, t.HTML, t.Text)
	return t, err
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), `SELECT `+columns+` FROM templates WHERE workspace_id = $1 ORDER BY updated_at DESC`, auth.FromContext(r.Context()).WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Template, error) { return scan(row) })
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"templates": list})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	t, err := scan(h.Pool.QueryRow(r.Context(), `SELECT `+columns+` FROM templates WHERE id = $1 AND workspace_id = $2`, id, auth.FromContext(r.Context()).WorkspaceID))
	h.respond(w, r, t, err, http.StatusOK)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in input
	if !httpx.Decode(w, r, &in) {
		return
	}
	if code := in.validate(); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	t, err := scan(h.Pool.QueryRow(r.Context(), `
		INSERT INTO templates (workspace_id, name, kind, subject, html, text) VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+columns,
		auth.FromContext(r.Context()).WorkspaceID, in.Name, in.Kind, in.Subject, in.HTML, in.Text))
	h.respond(w, r, t, err, http.StatusCreated)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var in input
	if !httpx.Decode(w, r, &in) {
		return
	}
	if code := in.validate(); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	t, err := scan(h.Pool.QueryRow(r.Context(), `
		UPDATE templates SET name = $3, kind = $4, subject = $5, html = $6, text = $7, updated_at = now()
		WHERE id = $1 AND workspace_id = $2 RETURNING `+columns,
		id, auth.FromContext(r.Context()).WorkspaceID, in.Name, in.Kind, in.Subject, in.HTML, in.Text))
	h.respond(w, r, t, err, http.StatusOK)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM templates WHERE id = $1 AND workspace_id = $2`, id, auth.FromContext(r.Context()).WorkspaceID)
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

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, t Template, err error, status int) {
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		httpx.Error(w, http.StatusConflict, "template_name_taken")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		httpx.JSON(w, status, t)
	}
}
