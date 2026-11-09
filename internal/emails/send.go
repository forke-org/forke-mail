package emails

import (
	"context"
	"fmt"
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"
	"forkemail/internal/workspaces"

	"github.com/jackc/pgx/v5"
)

type quotaError struct {
	remaining int
	required  int64
}

func (e *quotaError) Error() string {
	return fmt.Sprintf("trial quota exceeded: %d remaining, %d required", e.remaining, e.required)
}

type monthlyError struct {
	remaining int64
	required  int64
}

func (e *monthlyError) Error() string {
	return fmt.Sprintf("monthly limit: %d remaining, %d required", e.remaining, e.required)
}

func checkMonthlyRoom(ctx context.Context, tx pgx.Tx, required int64) error {
	limit := workspaces.MonthlyLimit()
	if limit <= 0 {
		return nil
	}
	used, err := workspaces.MonthlyUsage(ctx, tx)
	if err != nil {
		return err
	}
	if remaining := max(limit-used, 0); required > remaining {
		return &monthlyError{remaining: remaining, required: required}
	}
	return nil
}

func (h *Handler) Send(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var recipients int64
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var err error
		recipients, err = send(r.Context(), tx, ws, id)
		return err
	})
	if writeError(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "status": "sending", "recipients": recipients})
}

func send(ctx context.Context, tx pgx.Tx, ws, id string) (int64, error) {
	e, err := load(ctx, tx, ws, id, true)
	if err != nil {
		return 0, err
	}
	if e.Status != "draft" && e.Status != "scheduled" {
		return 0, errNotDraft
	}
	if err := checkDomain(ctx, tx, ws, e.FromAddress); err != nil {
		return 0, err
	}
	n, err := Snapshot(ctx, tx, ws, id)
	if err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, invalidError{"no_eligible_recipients"}
	}
	var plan string
	var remaining int
	var paused bool
	if err := tx.QueryRow(ctx, `
		SELECT plan, trial_send_limit - trial_sent_count, sending_paused FROM workspaces WHERE id = $1`, ws).Scan(&plan, &remaining, &paused); err != nil {
		return 0, err
	}
	if paused {
		return 0, errSendingPaused
	}
	if plan == "trial" && n > int64(remaining) {
		return 0, &quotaError{remaining: remaining, required: n}
	}
	if err := checkMonthlyRoom(ctx, tx, n); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_events (workspace_id, email_id, recipient_id, type)
		SELECT workspace_id, email_id, id, 'queued' FROM email_recipients WHERE email_id = $1`, id); err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE emails SET status = 'sending', started_at = now(), pause_reason = NULL
		WHERE id = $1 AND workspace_id = $2`, id, ws)
	return n, err
}
