package campaigns

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Pool *pgxpool.Pool
}

type campaign struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	Emails      int        `json:"emails"`
	Sent        int        `json:"sent"`
	LastEmailAt *time.Time `json:"last_email_at"`
}

type emailSummary struct {
	ID         string     `json:"id"`
	Subject    string     `json:"subject"`
	Status     string     `json:"status"`
	Recipients int        `json:"recipients"`
	Processed  int        `json:"processed"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type fields struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

const selectCampaign = `
	SELECT c.id, c.name, c.description, c.created_at,
		(SELECT COUNT(*) FROM emails e WHERE e.campaign_id = c.id),
		(SELECT COUNT(*) FROM email_recipients r JOIN emails e ON e.id = r.email_id WHERE e.campaign_id = c.id AND r.status = 'sent'),
		(SELECT max(e.created_at) FROM emails e WHERE e.campaign_id = c.id)
	FROM campaigns c`

func scanCampaign(row pgx.Row) (campaign, error) {
	var c campaign
	return c, row.Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt, &c.Emails, &c.Sent, &c.LastEmailAt)
}

func (f fields) apply(c *campaign) string {
	if f.Name != nil {
		c.Name = strings.TrimSpace(*f.Name)
	}
	if f.Description != nil {
		c.Description = strings.TrimSpace(*f.Description)
	}
	switch {
	case c.Name == "" || utf8.RuneCountInString(c.Name) > 200:
		return "invalid_name"
	case utf8.RuneCountInString(c.Description) > 500:
		return "invalid_description"
	}
	return ""
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	rows, err := h.Pool.Query(r.Context(), selectCampaign+` WHERE c.workspace_id = $1 ORDER BY c.created_at DESC`, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (campaign, error) { return scanCampaign(row) })
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"campaigns": list})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var f fields
	if !httpx.Decode(w, r, &f) {
		return
	}
	var c campaign
	if code := f.apply(&c); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO campaigns (workspace_id, name, description) VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id, lower(name)) DO NOTHING RETURNING id`, ws, c.Name, c.Description).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusConflict, "campaign_exists")
		return
	}
	if err == nil {
		c, err = scanCampaign(h.Pool.QueryRow(r.Context(), selectCampaign+` WHERE c.id = $1`, id))
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, c)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	c, err := scanCampaign(h.Pool.QueryRow(r.Context(), selectCampaign+` WHERE c.id = $1 AND c.workspace_id = $2`, id, ws))
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT e.id, e.subject, e.status, s.total, s.processed, e.created_at, e.started_at, e.finished_at
		FROM emails e
		CROSS JOIN LATERAL (
			SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE r.status IN ('sent', 'failed', 'skipped')) AS processed
			FROM email_recipients r WHERE r.email_id = e.id
		) s
		WHERE e.campaign_id = $1 AND e.workspace_id = $2
		ORDER BY e.created_at DESC`, id, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	emails, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (emailSummary, error) {
		var e emailSummary
		return e, row.Scan(&e.ID, &e.Subject, &e.Status, &e.Recipients, &e.Processed, &e.CreatedAt, &e.StartedAt, &e.FinishedAt)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"campaign": c, "emails": emails})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var f fields
	if !httpx.Decode(w, r, &f) {
		return
	}
	c, err := scanCampaign(h.Pool.QueryRow(r.Context(), selectCampaign+` WHERE c.id = $1 AND c.workspace_id = $2`, id, ws))
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if code := f.apply(&c); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `
		UPDATE campaigns SET name = $3, description = $4
		WHERE id = $1 AND workspace_id = $2
			AND NOT EXISTS (SELECT 1 FROM campaigns o WHERE o.workspace_id = $2 AND lower(o.name) = lower($3) AND o.id <> $1)`,
		id, ws, c.Name, c.Description)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusConflict, "campaign_exists")
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}
