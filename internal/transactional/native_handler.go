package transactional

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"forkemail/internal/auth"
	"forkemail/internal/db"
	"forkemail/internal/httpx"
	"forkemail/internal/workspaces"

	"github.com/jackc/pgx/v5"
)

var statuses = []string{"queued", "sending", "sent", "failed"}

type sendResponse struct {
	Message
	Replayed string `json:"replayed,omitempty"`
}

func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	who := auth.FromContext(r.Context())
	key := r.Header.Get(idempotencyHeader)
	if key != "" && !validIdempotencyKey(key) {
		httpx.Error(w, http.StatusBadRequest, "invalid_idempotency_key")
		return
	}
	var req request
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.normalize()
	if code := req.validate(); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	var res accepted
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var err error
		res, err = accept(r.Context(), tx, who, key, req)
		return err
	})
	if writeError(w, r, err) {
		return
	}
	status := http.StatusOK
	if res.replayed == "" {
		h.Dispatcher.Notify(res.id)
		status = http.StatusAccepted
	} else {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	m, err := load(r.Context(), h.Pool, who.WorkspaceID, res.id)
	if writeError(w, r, err) {
		return
	}
	httpx.JSON(w, status, sendResponse{Message: m, Replayed: res.replayed})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	d := Detail{}
	var err error
	d.Message, err = load(r.Context(), h.Pool, ws, id)
	if writeError(w, r, err) {
		return
	}
	if err := h.Pool.QueryRow(r.Context(), `
		SELECT html, text FROM transactional_messages WHERE id = $1 AND workspace_id = $2`, id, ws).Scan(&d.HTML, &d.Text); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT type, detail, created_at FROM email_events
		WHERE workspace_id = $1 AND message_id = $2 ORDER BY id`, ws, id)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	d.Events, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (event, error) {
		var e event
		return e, row.Scan(&e.Type, &e.Detail, &e.CreatedAt)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	status := r.URL.Query().Get("status")
	if status != "" && !slices.Contains(statuses, status) {
		httpx.Error(w, http.StatusBadRequest, "invalid_status")
		return
	}
	limit, offset := httpx.Page(r)
	rows, err := h.Pool.Query(r.Context(), selectMessage+`
		WHERE workspace_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC LIMIT $3 OFFSET $4`, ws, status, limit, offset)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Message, error) {
		var m Message
		return m, row.Scan(scanTargets(&m)...)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": list})
}

func load(ctx context.Context, q db.Querier, ws, id string) (Message, error) {
	var m Message
	err := q.QueryRow(ctx, selectMessage+` WHERE id = $1 AND workspace_id = $2`, id, ws).Scan(scanTargets(&m)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, errNotFound
	}
	return m, err
}

func writeError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.Is(err, errKeyReused):
		httpx.Error(w, http.StatusConflict, "idempotency_key_reused")
	case errors.Is(err, errSendingPaused), errors.Is(err, errWorkspaceInactive):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, errDomainUnverified), errors.Is(err, errSuppressed):
		httpx.Error(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, workspaces.ErrTrialQuotaExhausted):
		httpx.JSON(w, http.StatusPaymentRequired, map[string]any{"error": "trial_quota_exceeded", "remaining": 0, "required": 1})
	case errors.Is(err, workspaces.ErrDailyLimitReached):
		httpx.Error(w, http.StatusTooManyRequests, "daily_limit_reached")
	default:
		httpx.Internal(w, r, err)
	}
	return true
}
