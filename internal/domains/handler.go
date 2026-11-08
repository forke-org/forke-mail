package domains

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/db"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	errNotFound = errors.New("not_found")
	errProvider = errors.New("ses_request_failed")
)

func providerError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", errProvider, err)
}

type Handler struct {
	Pool       *pgxpool.Pool
	Identities Identities
}

type Domain struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	SESStatus     *string    `json:"ses_status"`
	CreatedAt     time.Time  `json:"created_at"`
	VerifiedAt    *time.Time `json:"verified_at"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	Records       []Record   `json:"records"`
}

const selectDomain = `SELECT id, name, kind, status, ses_status, created_at, verified_at, last_checked_at, dkim_tokens FROM domains`

func scanDomain(row pgx.Row) (Domain, error) {
	var d Domain
	var tokens []string
	err := row.Scan(&d.ID, &d.Name, &d.Kind, &d.Status, &d.SESStatus, &d.CreatedAt, &d.VerifiedAt, &d.LastCheckedAt, &tokens)
	d.Records = DNSRecords(d.Kind, d.Name, tokens)
	return d, err
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	rows, err := h.Pool.Query(r.Context(), selectDomain+` WHERE workspace_id = $1 ORDER BY created_at DESC`, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Domain, error) { return scanDomain(row) })
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"domains": list})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var body struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(body.Name))
	if body.Kind == "" {
		body.Kind = "domain"
	}
	if !ValidName(body.Kind, name) {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_identity")
		return
	}
	var d Domain
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO domains (workspace_id, name, kind) VALUES ($1, $2, $3) RETURNING id`, ws, name, body.Kind).Scan(&id); err != nil {
			return err
		}
		identity, err := h.Identities.Create(r.Context(), name)
		if err != nil {
			return providerError(err)
		}
		if err := save(r.Context(), tx, id, identity); err != nil {
			return err
		}
		d, err = scanDomain(tx.QueryRow(r.Context(), selectDomain+` WHERE id = $1`, id))
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		httpx.Error(w, http.StatusConflict, "identity_taken")
		return
	}
	if h.writeError(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, d)
}

func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	d, err := h.load(r.Context(), ws, id)
	if err == nil {
		err = Refresh(r.Context(), h.Pool, h.Identities, d.ID, d.Name)
	}
	if err == nil {
		d, err = h.load(r.Context(), ws, id)
	}
	if h.writeError(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	d, err := h.load(r.Context(), ws, id)
	if err == nil {
		err = providerError(h.Identities.Delete(r.Context(), d.Name))
	}
	if err == nil {
		_, err = h.Pool.Exec(r.Context(), `DELETE FROM domains WHERE id = $1 AND workspace_id = $2`, id, ws)
	}
	if h.writeError(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) load(ctx context.Context, ws, id string) (Domain, error) {
	d, err := scanDomain(h.Pool.QueryRow(ctx, selectDomain+` WHERE id = $1 AND workspace_id = $2`, id, ws))
	if errors.Is(err, pgx.ErrNoRows) {
		return d, errNotFound
	}
	return d, err
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, errNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.Is(err, errProvider):
		slog.Error("ses identity request failed", "path", httpx.RoutePath(r), "err", err)
		httpx.Error(w, http.StatusBadGateway, "ses_request_failed")
	default:
		httpx.Internal(w, r, err)
	}
	return true
}

func Refresh(ctx context.Context, q db.Querier, identities Identities, id, name string) error {
	identity, err := identities.Get(ctx, name)
	if errors.Is(err, ErrIdentityNotFound) {
		identity, err = Identity{Status: "NOT_FOUND"}, nil
	}
	if err != nil {
		return providerError(err)
	}
	return save(ctx, q, id, identity)
}

func save(ctx context.Context, q db.Querier, id string, identity Identity) error {
	status := "pending"
	switch {
	case identity.VerifiedForSending:
		status = "verified"
	case identity.Status == "FAILED" || identity.Status == "NOT_FOUND":
		status = "failed"
	}
	_, err := q.Exec(ctx, `
		UPDATE domains SET status = $2, ses_status = $3, last_checked_at = now(),
			dkim_tokens = CASE WHEN cardinality($4::text[]) > 0 THEN $4 ELSE dkim_tokens END,
			verified_at = CASE WHEN $2 = 'verified' THEN COALESCE(verified_at, now()) END
		WHERE id = $1`, id, status, identity.Status, identity.DkimTokens)
	return err
}
