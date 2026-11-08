package keys

import (
	"context"
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/db"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	errNotFound       = errors.New("not_found")
	errLastFullAccess = errors.New("last_full_access_key")
)

type Handler struct {
	Pool *pgxpool.Pool
}

type keyView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scope      string     `json:"scope"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

type createdKey struct {
	keyView
	Key string `json:"key"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.FromContext(r.Context())
	var body struct {
		Name     string `json:"name"`
		Scope    string `json:"scope"`
		Password string `json:"password"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	if !h.confirmPassword(w, r, body.Password) {
		return
	}
	if body.Name == "" || utf8.RuneCountInString(body.Name) > 100 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_name")
		return
	}
	if body.Scope != auth.FullAccess && body.Scope != auth.SendingAccess {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_scope")
		return
	}
	if id.Plan == "trial" && body.Scope == auth.FullAccess {
		httpx.Error(w, http.StatusForbidden, "trial_cannot_create_full_access_key")
		return
	}
	created, err := insertKey(r.Context(), h.Pool, id.WorkspaceID, body.Name, body.Scope)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusCreated, created)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, name, scope, prefix, created_at, last_used_at, revoked_at
		FROM api_keys WHERE workspace_id = $1
		ORDER BY revoked_at IS NOT NULL, created_at DESC`, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (keyView, error) {
		var k keyView
		return k, row.Scan(&k.ID, &k.Name, &k.Scope, &k.Prefix, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"keys": list})
}

func (h *Handler) confirmPassword(w http.ResponseWriter, r *http.Request, password string) bool {
	ok, err := accounts.VerifyPassword(r.Context(), h.Pool, auth.FromContext(r.Context()).UserID, password)
	if err != nil {
		httpx.Internal(w, r, err)
		return false
	}
	if !ok {
		httpx.Error(w, http.StatusForbidden, "incorrect_password")
	}
	return ok
}

func insertKey(ctx context.Context, q db.Querier, workspaceID, name, scope string) (createdKey, error) {
	key := auth.GenerateKey()
	k := createdKey{keyView: keyView{Name: name, Scope: scope, Prefix: key.Prefix}, Key: key.Plain}
	err := q.QueryRow(ctx, `
		INSERT INTO api_keys (workspace_id, name, scope, prefix, key_hash)
		VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at`,
		workspaceID, name, scope, key.Prefix, key.Hash).Scan(&k.ID, &k.CreatedAt)
	return k, err
}
