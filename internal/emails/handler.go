package emails

import (
	"context"
	"errors"
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/db"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

func load(ctx context.Context, q db.Querier, ws, id string, forUpdate bool) (Email, error) {
	query := selectEmail + ` WHERE e.id = $1 AND e.workspace_id = $2`
	if forUpdate {
		query += ` FOR UPDATE OF e`
	}
	var e Email
	err := q.QueryRow(ctx, query, id, ws).Scan(scanTargets(&e)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, errNotFound
	}
	if err != nil {
		return e, err
	}
	e.Audience, err = loadAudience(ctx, q, ws, id)
	return e, err
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	campaignID, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var f fields
	if !httpx.Decode(w, r, &f) {
		return
	}
	var e Email
	f.applyTo(&e)
	if code := validate(e); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM campaigns WHERE id = $1 AND workspace_id = $2)`, campaignID, ws).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errNotFound
		}
		if err := checkDomain(r.Context(), tx, ws, e.FromAddress); err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO emails (workspace_id, campaign_id, subject, from_name, from_address, reply_to, topic, html, text)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
			ws, campaignID, e.Subject, e.FromName, e.FromAddress, e.ReplyTo, e.Topic, e.HTML, e.Text).Scan(&id); err != nil {
			return err
		}
		if err := saveAudience(r.Context(), tx, ws, id, e.Audience); err != nil {
			return err
		}
		var err error
		e, err = load(r.Context(), tx, ws, id, false)
		return err
	})
	if writeError(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, e)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	e, err := load(r.Context(), h.Pool, ws, id, false)
	if writeError(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, e)
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
	var e Email
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var err error
		if e, err = load(r.Context(), tx, ws, id, true); err != nil {
			return err
		}
		if e.Status != "draft" {
			return errNotDraft
		}
		f.applyTo(&e)
		if code := validate(e); code != "" {
			return invalidError{code}
		}
		if err := checkDomain(r.Context(), tx, ws, e.FromAddress); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `
			UPDATE emails SET subject = $3, from_name = $4, from_address = $5, reply_to = $6, topic = $7, html = $8, text = $9
			WHERE id = $1 AND workspace_id = $2`, id, ws, e.Subject, e.FromName, e.FromAddress, e.ReplyTo, e.Topic, e.HTML, e.Text); err != nil {
			return err
		}
		if f.Audience != nil {
			if err := saveAudience(r.Context(), tx, ws, id, e.Audience); err != nil {
				return err
			}
		}
		e, err = load(r.Context(), tx, ws, id, false)
		return err
	})
	if writeError(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM emails WHERE id = $1 AND workspace_id = $2 AND status = 'draft'`, id, ws)
	if err == nil && tag.RowsAffected() == 0 {
		err = h.missingOr(r, id, ws, errNotDraft)
	}
	if writeError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) missingOr(r *http.Request, id, ws string, otherwise error) error {
	var exists bool
	if err := h.Pool.QueryRow(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM emails WHERE id = $1 AND workspace_id = $2)`, id, ws).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errNotFound
	}
	return otherwise
}

func writeError(w http.ResponseWriter, r *http.Request, err error) bool {
	var invalid invalidError
	var quota *quotaError
	var monthly *monthlyError
	switch {
	case err == nil:
		return false
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.Is(err, errNotDraft):
		httpx.Error(w, http.StatusConflict, "email_not_draft")
	case errors.Is(err, errInvalidState):
		httpx.Error(w, http.StatusConflict, "invalid_email_state")
	case errors.Is(err, errSendingPaused):
		httpx.Error(w, http.StatusConflict, "workspace_sending_paused")
	case errors.As(err, &invalid):
		httpx.Error(w, http.StatusUnprocessableEntity, invalid.code)
	case errors.As(err, &monthly):
		httpx.JSON(w, http.StatusConflict, map[string]any{
			"error": "monthly_limit_insufficient", "remaining": monthly.remaining, "required": monthly.required,
		})
	case errors.As(err, &quota):
		httpx.JSON(w, http.StatusPaymentRequired, map[string]any{
			"error": "trial_quota_exceeded", "remaining": quota.remaining, "required": quota.required,
		})
	default:
		httpx.Internal(w, r, err)
	}
	return true
}
