package tracking

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"forkemail/internal/events"
	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var transparentGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
}

type Handler struct {
	Pool    *pgxpool.Pool
	Tracker Tracker
}

func (h *Handler) Open(w http.ResponseWriter, r *http.Request) {
	if id, recipient, err := h.Tracker.verifyOpen(chi.URLParam(r, "token")); err == nil && httpx.ValidID(id) && !automated(r.UserAgent()) {
		record := h.recordOpen
		if recipient {
			record = h.recordRecipientOpen
		}
		if err := record(r.Context(), id); err != nil {
			slog.Error("record open failed", "message_id", id, "err", err)
		}
	}
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	_, _ = w.Write(transparentGIF)
}

func (h *Handler) Click(w http.ResponseWriter, r *http.Request) {
	id, target, recipient, err := h.Tracker.verifyClick(chi.URLParam(r, "token"))
	if err != nil || !httpx.ValidID(id) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if !automated(r.UserAgent()) {
		record := h.recordClick
		if recipient {
			record = h.recordRecipientClick
		}
		if err := record(r.Context(), id, target); err != nil {
			slog.Error("record click failed", "message_id", id, "err", err)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handler) recordOpen(ctx context.Context, id string) error {
	return pgx.BeginFunc(ctx, h.Pool, func(tx pgx.Tx) error {
		var ws string
		var first bool
		err := tx.QueryRow(ctx, `
			UPDATE transactional_messages SET opened_at = COALESCE(opened_at, now()), open_count = open_count + 1
			WHERE id = $1 AND track_opens AND status = 'sent'
			RETURNING workspace_id, open_count = 1`, id).Scan(&ws, &first)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !first) {
			return nil
		}
		if err != nil {
			return err
		}
		return events.Record(ctx, tx, events.Event{WorkspaceID: ws, MessageID: id, Type: "opened"})
	})
}

func (h *Handler) recordClick(ctx context.Context, id, target string) error {
	return pgx.BeginFunc(ctx, h.Pool, func(tx pgx.Tx) error {
		var ws string
		err := tx.QueryRow(ctx, `
			UPDATE transactional_messages SET clicked_at = COALESCE(clicked_at, now()), click_count = click_count + 1
			WHERE id = $1 AND track_clicks AND status = 'sent'
			RETURNING workspace_id`, id).Scan(&ws)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return events.Record(ctx, tx, events.Event{WorkspaceID: ws, MessageID: id, Type: "clicked", Detail: map[string]any{"url": target}})
	})
}
