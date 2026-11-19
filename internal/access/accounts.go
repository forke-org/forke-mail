package access

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

type Member struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Username *string `json:"username"`
	Role     string  `json:"role"`
	Forke    bool    `json:"forke"`
}

type Account struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Plan          string     `json:"plan"`
	Status        string     `json:"status"`
	Forke         bool       `json:"forke"`
	Credits       int        `json:"credits"`
	CreditsUsed   int        `json:"credits_used"`
	DailyLimit    int        `json:"daily_limit"`
	Sent30d       int        `json:"sent_30d"`
	SentTotal     int        `json:"sent_total"`
	Contacts      int        `json:"contacts"`
	Domains       []string   `json:"domains"`
	SendingPaused bool       `json:"sending_paused"`
	CreatedAt     time.Time  `json:"created_at"`
	LastSentAt    *time.Time `json:"last_sent_at"`
	Members       []Member   `json:"members"`
}

func (p *Platform) Accounts(w http.ResponseWriter, r *http.Request) {
	rows, err := p.Pool.Query(r.Context(), `
		SELECT w.id, w.name, w.plan, w.status, w.name = $1, w.trial_send_limit, w.trial_sent_count, w.daily_limit, w.sending_paused, w.created_at,
			COALESCE(s.sent_30d, 0), COALESCE(s.sent_total, 0), s.last_sent_at,
			(SELECT COUNT(*) FROM contacts c WHERE c.workspace_id = w.id),
			COALESCE((SELECT array_agg(d.name ORDER BY d.name) FROM domains d WHERE d.workspace_id = w.id AND d.status = 'verified'), '{}'),
			COALESCE((SELECT jsonb_agg(jsonb_build_object('name', u.name, 'email', u.email, 'username', u.username, 'role', u.role, 'forke', u.forke_admin_id IS NOT NULL) ORDER BY u.created_at)
				FROM users u WHERE u.workspace_id = w.id), '[]')
		FROM workspaces w
		LEFT JOIN LATERAL (
			SELECT COUNT(*) FILTER (WHERE sent_at > now() - interval '30 days') AS sent_30d, COUNT(*) AS sent_total, MAX(sent_at) AS last_sent_at FROM (
				SELECT sent_at FROM email_recipients WHERE workspace_id = w.id AND sent_at IS NOT NULL
				UNION ALL SELECT sent_at FROM transactional_messages WHERE workspace_id = w.id AND sent_at IS NOT NULL
			) x
		) s ON true
		ORDER BY w.name = $1 DESC, w.created_at DESC`, p.ForkeWorkspace)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Account, error) {
		var a Account
		return a, row.Scan(&a.ID, &a.Name, &a.Plan, &a.Status, &a.Forke, &a.Credits, &a.CreditsUsed, &a.DailyLimit, &a.SendingPaused, &a.CreatedAt,
			&a.Sent30d, &a.SentTotal, &a.LastSentAt, &a.Contacts, &a.Domains, &a.Members)
	})
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"accounts": list})
}

func (p *Platform) AddCredits(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Amount int `json:"amount"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	if body.Amount == 0 || body.Amount > 10_000_000 || body.Amount < -10_000_000 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_amount")
		return
	}
	var credits int
	err := p.Pool.QueryRow(r.Context(), `
		UPDATE workspaces SET trial_send_limit = GREATEST(trial_sent_count, trial_send_limit + $2)
		WHERE id = $1 AND plan = 'trial' RETURNING trial_send_limit`, id, body.Amount).Scan(&credits)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]int{"credits": credits})
}

func (p *Platform) SetStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	if body.Status != "active" && body.Status != "paused" {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_status")
		return
	}
	tag, err := p.Pool.Exec(r.Context(), `UPDATE workspaces SET status = $2 WHERE id = $1 AND name <> $3`, id, body.Status, p.ForkeWorkspace)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if body.Status == "paused" {
		if _, err := p.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE workspace_id = $1)`, id); err != nil {
			httpx.Internal(w, r, err)
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": body.Status})
}

func (p *Platform) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var names []string
	err := pgx.BeginFunc(r.Context(), p.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT name FROM domains WHERE workspace_id = $1`, id)
		if err != nil {
			return err
		}
		if names, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM access_requests WHERE workspace_id = $1`, id); err != nil {
			return err
		}
		tag, err := tx.Exec(r.Context(), `DELETE FROM workspaces WHERE id = $1 AND name <> $2`, id, p.ForkeWorkspace)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	p.releaseIdentities(r, names)
	w.WriteHeader(http.StatusNoContent)
}

func (p *Platform) releaseIdentities(r *http.Request, names []string) {
	if p.Identities == nil {
		return
	}
	for _, name := range names {
		if err := p.Identities.Delete(r.Context(), name); err != nil {
			slog.Warn("release identity", "name", name, "err", err)
		}
	}
}

func (p *Platform) DeleteRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	tag, err := p.Pool.Exec(r.Context(), `DELETE FROM access_requests WHERE id = $1 AND status <> 'joined'`, id)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
