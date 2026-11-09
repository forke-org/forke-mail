package transactional

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/contacts"
	"forkemail/internal/httpx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CompatibilityHandler struct {
	Pool        *pgxpool.Pool
	Enqueuer    EmailEnqueuer
	Development bool
}
type Receipt struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Mode   string `json:"mode"`
}

var errConflict = errors.New("idempotency_conflict")
var errSender = errors.New("sender_not_verified")
var errBlocked = errors.New("recipient_suppressed")
var errPaused = errors.New("workspace_sending_paused")
var errCapacity = errors.New("transactional_queue_full")

func (h *CompatibilityHandler) Send(w http.ResponseWriter, r *http.Request) {
	if h.Enqueuer == nil {
		httpx.Error(w, 503, "transactional_engine_unavailable")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) == 0 || len(key) > 128 || strings.TrimSpace(key) != key || strings.IndexFunc(key, func(c rune) bool { return c < 33 || c > 126 }) >= 0 {
		httpx.Error(w, 400, "invalid_idempotency_key")
		return
	}
	var body Request
	if !httpx.Decode(w, r, &body) {
		return
	}
	body.From = contacts.NormalizeEmail(body.From)
	body.To = contacts.NormalizeEmail(body.To)
	if !contacts.ValidEmail(body.From) || !contacts.ValidEmail(body.To) || utf8.RuneCountInString(body.Subject) == 0 || utf8.RuneCountInString(body.Subject) > 256 || strings.ContainsAny(body.Subject, "\r\n") || (strings.TrimSpace(body.HTML) == "" && strings.TrimSpace(body.Text) == "") {
		httpx.Error(w, 422, "invalid_email_request")
		return
	}
	data, _ := json.Marshal(body)
	hash := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(hash[:])
	ws := auth.FromContext(r.Context()).WorkspaceID
	mode := "engine"
	if h.Development {
		mode = "development_stub"
	}
	out := Receipt{Status: "queued", Mode: mode}
	replayed := false
	err := pgx.BeginFunc(r.Context(), h.Pool, func(tx pgx.Tx) error {
		var paused bool
		if err := tx.QueryRow(r.Context(), `SELECT sending_paused FROM workspaces WHERE id=$1 FOR UPDATE`, ws).Scan(&paused); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM transactional_requests WHERE workspace_id=$1 AND expires_at<now()`, ws); err != nil {
			return err
		}
		var previousHash, previousMode string
		err := tx.QueryRow(r.Context(), `SELECT id,request_hash,mode FROM transactional_requests WHERE workspace_id=$1 AND idempotency_key=$2`, ws, key).Scan(&out.ID, &previousHash, &previousMode)
		if err == nil {
			if previousHash != fingerprint {
				return errConflict
			}
			out.Mode = previousMode
			replayed = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if paused {
			return errPaused
		}
		var verified, blocked bool
		domain := strings.Split(body.From, "@")[1]
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM domains WHERE workspace_id=$1 AND status='verified' AND ((kind='domain' AND lower(name)=$2) OR (kind='email' AND lower(name)=$3)))`, ws, domain, body.From).Scan(&verified); err != nil {
			return err
		}
		if !verified {
			return errSender
		}
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM suppressions WHERE workspace_id=$1 AND lower(email)=$2) OR EXISTS(SELECT 1 FROM contacts WHERE workspace_id=$1 AND lower(email)=$2 AND status<>'active')`, ws, body.To).Scan(&blocked); err != nil {
			return err
		}
		if blocked {
			return errBlocked
		}
		var count int
		if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM transactional_requests WHERE workspace_id=$1`, ws).Scan(&count); err != nil {
			return err
		}
		if count >= 1000 {
			return errCapacity
		}
		if err := tx.QueryRow(r.Context(), `INSERT INTO transactional_requests(workspace_id,idempotency_key,request_hash,mode) VALUES($1,$2,$3,$4) RETURNING id`, ws, key, fingerprint, mode).Scan(&out.ID); err != nil {
			return err
		}
		return h.Enqueuer.Enqueue(r.Context(), tx, Job{ID: out.ID, WorkspaceID: ws, IdempotencyKey: key, Message: body})
	})
	switch {
	case errors.Is(err, errConflict):
		httpx.Error(w, 409, "idempotency_conflict")
	case errors.Is(err, errSender):
		httpx.Error(w, 422, "sender_not_verified")
	case errors.Is(err, errBlocked):
		httpx.Error(w, 422, "recipient_suppressed")
	case errors.Is(err, errPaused):
		httpx.Error(w, 409, "workspace_sending_paused")
	case errors.Is(err, errCapacity):
		httpx.Error(w, 429, "transactional_queue_full")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		w.Header().Set("Cache-Control", "no-store")
		if replayed {
			w.Header().Set("Idempotency-Replayed", "true")
		}
		httpx.JSON(w, 202, out)
	}
}
