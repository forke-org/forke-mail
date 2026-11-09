package contacts

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/db"
	"forkemail/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

var topicKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

type Topic struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

func ValidTopicKey(key string) bool {
	return topicKeyPattern.MatchString(key)
}

func ListTopics(ctx context.Context, q db.Querier, workspaceID string) ([]Topic, error) {
	rows, err := q.Query(ctx, `SELECT key, label FROM topics WHERE workspace_id = $1 ORDER BY created_at, key`, workspaceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Topic, error) {
		var t Topic
		return t, row.Scan(&t.Key, &t.Label)
	})
}

func (h *Handler) Topics(w http.ResponseWriter, r *http.Request) {
	list, err := ListTopics(r.Context(), h.Pool, auth.FromContext(r.Context()).WorkspaceID)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"topics": list})
}

func (h *Handler) PutTopic(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if !ValidTopicKey(key) {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_topic")
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	body.Label = strings.TrimSpace(body.Label)
	if body.Label == "" || utf8.RuneCountInString(body.Label) > 60 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_label")
		return
	}
	if _, err := h.Pool.Exec(r.Context(), `
		INSERT INTO topics (workspace_id, key, label) VALUES ($1, $2, $3)
		ON CONFLICT (workspace_id, key) DO UPDATE SET label = EXCLUDED.label`,
		auth.FromContext(r.Context()).WorkspaceID, key, body.Label); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, Topic{Key: key, Label: body.Label})
}
