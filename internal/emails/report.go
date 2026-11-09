package emails

import (
	"errors"
	"math"
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

type report struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	PauseReason *string `json:"pause_reason"`
	Total       int     `json:"total"`
	Counts      counts  `json:"counts"`
	Progress    float64 `json:"progress"`
}

type counts struct {
	Pending      int `json:"pending"`
	Sent         int `json:"sent"`
	Delivered    int `json:"delivered"`
	Bounced      int `json:"bounced"`
	Complained   int `json:"complained"`
	Unsubscribed int `json:"unsubscribed"`
	Failed       int `json:"failed"`
	Skipped      int `json:"skipped"`
	Opened       int `json:"opened"`
	Clicked      int `json:"clicked"`
}

func (h *Handler) Report(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	rep := report{ID: id}
	c := &rep.Counts
	err := h.Pool.QueryRow(r.Context(), `
		SELECT status, pause_reason FROM emails WHERE id = $1 AND workspace_id = $2`, id, ws).Scan(&rep.Status, &rep.PauseReason)
	if errors.Is(err, pgx.ErrNoRows) {
		err = errNotFound
	}
	if err == nil {
		err = h.Pool.QueryRow(r.Context(), `
			SELECT COUNT(*),
				COUNT(*) FILTER (WHERE status IN ('pending', 'sending')),
				COUNT(*) FILTER (WHERE status = 'sent'),
				COUNT(*) FILTER (WHERE status = 'failed'),
				COUNT(*) FILTER (WHERE status = 'skipped'),
				COUNT(delivered_at), COUNT(bounced_at), COUNT(complained_at), COUNT(unsubscribed_at),
				COUNT(opened_at), COUNT(clicked_at)
			FROM email_recipients WHERE email_id = $1 AND workspace_id = $2`, id, ws,
		).Scan(&rep.Total, &c.Pending, &c.Sent, &c.Failed, &c.Skipped, &c.Delivered, &c.Bounced, &c.Complained, &c.Unsubscribed, &c.Opened, &c.Clicked)
	}
	if writeError(w, r, err) {
		return
	}
	if rep.Total > 0 {
		rep.Progress = math.Round(float64(rep.Total-c.Pending)/float64(rep.Total)*1000) / 10
	}
	httpx.JSON(w, http.StatusOK, rep)
}
