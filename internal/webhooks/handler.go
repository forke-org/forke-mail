package webhooks

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var EventTypes = []string{"email.queued", "email.sent", "email.delivered", "email.bounced", "email.soft_bounced", "email.complained", "email.unsubscribed", "email.failed", "email.rejected", "email.delayed", "email.opened", "email.clicked", "workspace.sending_paused"}

type Handler struct {
	Pool    *pgxpool.Pool
	Vault   *Vault
	Network Network
}
type Endpoint struct {
	ID            string    `json:"id"`
	URL           string    `json:"url"`
	Events        []string  `json:"events"`
	Enabled       bool      `json:"enabled"`
	SecretVersion int       `json:"secret_version"`
	CreatedAt     time.Time `json:"created_at"`
}

const selectEndpoint = `SELECT id,url,events,enabled,secret_version,created_at FROM webhook_endpoints`

func scanEndpoint(row pgx.Row) (Endpoint, error) {
	var e Endpoint
	err := row.Scan(&e.ID, &e.URL, &e.Events, &e.Enabled, &e.SecretVersion, &e.CreatedAt)
	return e, err
}
func validEvents(events []string) bool {
	if len(events) == 0 || len(events) > len(EventTypes) {
		return false
	}
	seen := map[string]bool{}
	for _, e := range events {
		if !slices.Contains(EventTypes, e) || seen[e] {
			return false
		}
		seen[e] = true
	}
	return true
}
func (h *Handler) ready(w http.ResponseWriter) bool {
	if h.Vault == nil {
		httpx.Error(w, 503, "webhooks_not_configured")
		return false
	}
	return true
}
func (h *Handler) validURL(r *http.Request, raw string) bool {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	return h.Network.Validate(ctx, raw) == nil
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	var b struct {
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if !httpx.Decode(w, r, &b) {
		return
	}
	if !validEvents(b.Events) {
		httpx.Error(w, 422, "invalid_webhook_events")
		return
	}
	if !h.validURL(r, b.URL) {
		httpx.Error(w, 422, "invalid_webhook_url")
		return
	}
	ws := auth.FromContext(r.Context()).WorkspaceID
	secret := newSecret()
	var out Endpoint
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `SELECT id FROM workspaces WHERE id=$1 FOR UPDATE`, ws); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM webhook_endpoints WHERE workspace_id=$1`, ws).Scan(&count); err != nil {
			return err
		}
		if count >= 10 {
			return errors.New("endpoint_limit")
		}
		row := tx.QueryRow(r.Context(), `INSERT INTO webhook_endpoints(workspace_id,url,secret,events,enabled) VALUES($1,$2,$3,$4,true) RETURNING id,url,events,enabled,secret_version,created_at`, ws, b.URL, h.Vault.Seal(secret, ws), b.Events)
		var err error
		out, err = scanEndpoint(row)
		return err
	})
	if err != nil {
		if err.Error() == "endpoint_limit" {
			httpx.Error(w, 409, "webhook_endpoint_limit")
		} else {
			httpx.Internal(w, r, err)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, 201, struct {
		Endpoint
		Secret string `json:"secret"`
	}{out, secret})
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), selectEndpoint+` WHERE workspace_id=$1 ORDER BY created_at DESC`, auth.FromContext(r.Context()).WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Endpoint, error) { return scanEndpoint(row) })
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if items == nil {
		items = []Endpoint{}
	}
	httpx.JSON(w, 200, map[string]any{"webhooks": items})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ws := auth.FromContext(r.Context()).WorkspaceID
	var b struct {
		URL     *string   `json:"url"`
		Events  *[]string `json:"events"`
		Enabled *bool     `json:"enabled"`
	}
	if !httpx.Decode(w, r, &b) {
		return
	}
	var exists bool
	if err := h.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM webhook_endpoints WHERE id=$1 AND workspace_id=$2)`, id, ws).Scan(&exists); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if !exists {
		httpx.Error(w, 404, "not_found")
		return
	}
	if b.URL == nil && b.Events == nil && b.Enabled == nil {
		httpx.Error(w, 422, "empty_update")
		return
	}
	if b.URL != nil && !h.validURL(r, *b.URL) {
		httpx.Error(w, 422, "invalid_webhook_url")
		return
	}
	var events []string
	if b.Events != nil {
		events = *b.Events
		if !validEvents(events) {
			httpx.Error(w, 422, "invalid_webhook_events")
			return
		}
	}
	if b.Enabled != nil && *b.Enabled && !h.ready(w) {
		return
	}
	var out Endpoint
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		if b.Enabled != nil && *b.Enabled {
			var encrypted string
			if err := tx.QueryRow(r.Context(), `SELECT secret FROM webhook_endpoints WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, id, ws).Scan(&encrypted); err != nil {
				return err
			}
			if _, err := h.Vault.Open(encrypted, ws); err != nil {
				return errors.New("rotation_required")
			}
		}
		var err error
		out, err = scanEndpoint(tx.QueryRow(r.Context(), `UPDATE webhook_endpoints SET url=COALESCE($3,url),events=COALESCE($4::text[],events),enabled=COALESCE($5,enabled) WHERE id=$1 AND workspace_id=$2 RETURNING id,url,events,enabled,secret_version,created_at`, id, ws, b.URL, events, b.Enabled))
		if err != nil {
			return err
		}
		if !out.Enabled {
			_, err = tx.Exec(r.Context(), `UPDATE webhook_deliveries SET status='cancelled',finished_at=now(),lease_token=NULL WHERE endpoint_id=$1 AND workspace_id=$2 AND status IN ('pending','processing')`, id, ws)
		}
		return err
	})
	if err != nil && err.Error() == "rotation_required" {
		httpx.Error(w, 409, "webhook_secret_rotation_required")
		return
	}
	if h.writeError(w, r, err) {
		return
	}
	httpx.JSON(w, 200, out)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM webhook_endpoints WHERE id=$1 AND workspace_id=$2`, id, auth.FromContext(r.Context()).WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, 404, "not_found")
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) Rotate(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ws := auth.FromContext(r.Context()).WorkspaceID
	secret := newSecret()
	var version int
	err := h.Pool.QueryRow(r.Context(), `UPDATE webhook_endpoints SET secret=$3,secret_version=secret_version+1 WHERE id=$1 AND workspace_id=$2 RETURNING secret_version`, id, ws, h.Vault.Seal(secret, ws)).Scan(&version)
	if h.writeError(w, r, err) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, 201, map[string]any{"id": id, "secret": secret, "secret_version": version})
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "not_found")
	} else {
		httpx.Internal(w, r, err)
	}
	return true
}
