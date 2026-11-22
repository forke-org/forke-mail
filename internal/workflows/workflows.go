package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/contacts"
	"forkemail/internal/db"
	"forkemail/internal/domains"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxSteps = 20

var eventName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,99}$`)

type Step struct {
	Type        string `json:"type"`
	Minutes     int    `json:"minutes,omitempty"`
	TemplateID  string `json:"template_id,omitempty"`
	FromAddress string `json:"from_address,omitempty"`
	FromName    string `json:"from_name,omitempty"`
	ReplyTo     string `json:"reply_to,omitempty"`
}

type Workflow struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	TriggerEvent string    `json:"trigger_event"`
	ExitEvent    string    `json:"exit_event"`
	Steps        []Step    `json:"steps"`
	Enabled      bool      `json:"enabled"`
	AllowReentry bool      `json:"allow_reentry"`
	Running      int       `json:"running"`
	Completed    int       `json:"completed"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Handler struct {
	Pool *pgxpool.Pool
}

func (in *Workflow) validate(ctx context.Context, q db.Querier, ws string) (string, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.TriggerEvent = strings.TrimSpace(in.TriggerEvent)
	in.ExitEvent = strings.TrimSpace(in.ExitEvent)
	switch {
	case in.Name == "" || utf8.RuneCountInString(in.Name) > 100:
		return "invalid_name", nil
	case !eventName.MatchString(in.TriggerEvent):
		return "invalid_event", nil
	case in.ExitEvent != "" && (!eventName.MatchString(in.ExitEvent) || in.ExitEvent == in.TriggerEvent):
		return "invalid_exit_event", nil
	case len(in.Steps) > maxSteps:
		return "too_many_steps", nil
	case in.Enabled && len(in.Steps) == 0:
		return "workflow_needs_steps", nil
	}
	if in.Steps == nil {
		in.Steps = []Step{}
	}
	for i := range in.Steps {
		s := &in.Steps[i]
		switch s.Type {
		case "wait":
			if s.Minutes < 1 || s.Minutes > 60*24*90 {
				return "invalid_wait", nil
			}
			*s = Step{Type: "wait", Minutes: s.Minutes}
		case "send":
			s.FromAddress = contacts.NormalizeEmail(s.FromAddress)
			s.ReplyTo = contacts.NormalizeEmail(s.ReplyTo)
			s.FromName = strings.TrimSpace(s.FromName)
			s.Minutes = 0
			if !httpx.ValidID(s.TemplateID) {
				return "invalid_template", nil
			}
			var ok bool
			if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM templates WHERE id = $1 AND workspace_id = $2)`, s.TemplateID, ws).Scan(&ok); err != nil {
				return "", err
			}
			if !ok {
				return "invalid_template", nil
			}
			if !contacts.ValidEmail(s.FromAddress) || (s.ReplyTo != "" && !contacts.ValidEmail(s.ReplyTo)) || utf8.RuneCountInString(s.FromName) > 100 {
				return "invalid_from_address", nil
			}
			verified, err := domains.Verified(ctx, q, ws, s.FromAddress)
			if err != nil {
				return "", err
			}
			if !verified {
				return "from_domain_not_verified", nil
			}
		default:
			return "invalid_step", nil
		}
	}
	return "", nil
}

const columns = `w.id, w.name, w.trigger_event, w.exit_event, w.steps, w.enabled, w.allow_reentry,
	(SELECT COUNT(*) FROM workflow_runs r WHERE r.workflow_id = w.id AND r.status = 'running')::int,
	(SELECT COUNT(*) FROM workflow_runs r WHERE r.workflow_id = w.id AND r.status = 'done')::int,
	w.created_at, w.updated_at`

func scan(row pgx.Row) (Workflow, error) {
	var w Workflow
	var raw []byte
	if err := row.Scan(&w.ID, &w.Name, &w.TriggerEvent, &w.ExitEvent, &raw, &w.Enabled, &w.AllowReentry, &w.Running, &w.Completed, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return w, err
	}
	return w, json.Unmarshal(raw, &w.Steps)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), `SELECT `+columns+` FROM workflows w WHERE w.workspace_id = $1 ORDER BY w.updated_at DESC`, auth.FromContext(r.Context()).WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Workflow, error) { return scan(row) })
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"workflows": list})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	wf, err := scan(h.Pool.QueryRow(r.Context(), `SELECT `+columns+` FROM workflows w WHERE w.id = $1 AND w.workspace_id = $2`, id, auth.FromContext(r.Context()).WorkspaceID))
	respond(w, r, wf, err, http.StatusOK)
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request, id string) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var in Workflow
	if !httpx.Decode(w, r, &in) {
		return
	}
	code, err := in.validate(r.Context(), h.Pool, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	steps, _ := json.Marshal(in.Steps)
	if id == "" {
		err = h.Pool.QueryRow(r.Context(), `INSERT INTO workflows (workspace_id, name, trigger_event, exit_event, steps, enabled, allow_reentry) VALUES ($1, $2, $3, $7, $4, $5, $6) RETURNING id`,
			ws, in.Name, in.TriggerEvent, steps, in.Enabled, in.AllowReentry, in.ExitEvent).Scan(&id)
	} else {
		err = h.Pool.QueryRow(r.Context(), `UPDATE workflows SET name = $3, trigger_event = $4, steps = $5, enabled = $6, allow_reentry = $7, exit_event = $8, updated_at = now() WHERE id = $1 AND workspace_id = $2 RETURNING id`,
			id, ws, in.Name, in.TriggerEvent, steps, in.Enabled, in.AllowReentry, in.ExitEvent).Scan(&id)
	}
	if err != nil {
		respond(w, r, Workflow{}, err, 0)
		return
	}
	wf, err := scan(h.Pool.QueryRow(r.Context(), `SELECT `+columns+` FROM workflows w WHERE w.id = $1`, id))
	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusCreated
	}
	respond(w, r, wf, err, status)
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
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM workflows WHERE id = $1 AND workspace_id = $2`, id, auth.FromContext(r.Context()).WorkspaceID)
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

type Run struct {
	ID         string     `json:"id"`
	Email      string     `json:"email"`
	Step       int        `json:"step"`
	Status     string     `json:"status"`
	NextRunAt  time.Time  `json:"next_run_at"`
	LastError  *string    `json:"last_error"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

func (h *Handler) Runs(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT r.id, c.email, r.step, r.status, r.next_run_at, r.last_error, r.created_at, r.finished_at
		FROM workflow_runs r JOIN contacts c ON c.id = r.contact_id
		WHERE r.workflow_id = $1 AND r.workspace_id = $2 ORDER BY r.created_at DESC LIMIT 100`, id, auth.FromContext(r.Context()).WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Run])
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"runs": list})
}

func respond(w http.ResponseWriter, r *http.Request, wf Workflow, err error, status int) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		httpx.JSON(w, status, wf)
	}
}
