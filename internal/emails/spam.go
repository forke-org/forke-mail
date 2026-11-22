package emails

import (
	"context"
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"
	"forkemail/internal/spamcheck"
)

func (h *Handler) spamCheck(ctx context.Context, ws, id string) (spamcheck.Result, error) {
	e, err := load(ctx, h.Pool, ws, id, false)
	if err != nil {
		return spamcheck.Result{}, err
	}
	var platform bool
	if err := h.Pool.QueryRow(ctx, `SELECT platform FROM workspaces WHERE id = $1`, ws).Scan(&platform); err != nil {
		return spamcheck.Result{}, err
	}
	res := h.Spam.Check(ctx, spamcheck.Email{FromName: e.FromName, FromAddress: e.FromAddress, Subject: e.Subject, HTML: e.HTML, Text: e.Text})
	if platform {
		res.Spam = false
	}
	return res, nil
}

func (h *Handler) SpamCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	res, err := h.spamCheck(r.Context(), auth.FromContext(r.Context()).WorkspaceID, id)
	if writeError(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
