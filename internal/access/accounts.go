package access

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"log/slog"
	"net/http"
	"time"

	"forkemail/internal/accounts"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

type Member struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	Username  *string    `json:"username"`
	Role      string     `json:"role"`
	Forke     bool       `json:"forke"`
	Projects  int        `json:"projects"`
	JoinedAt  time.Time  `json:"joined_at"`
	LastLogin *time.Time `json:"last_login"`
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
	RatePerSec    int        `json:"rate_limit_per_sec"`
	WarmupUntil   *time.Time `json:"warmup_until"`
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
		SELECT w.id, w.name, w.plan, w.status, w.platform, w.trial_send_limit, w.trial_sent_count, w.daily_limit, w.rate_limit_per_sec, CASE WHEN w.warmup_until > now() THEN w.warmup_until END, w.sending_paused, w.created_at,
			COALESCE(s.sent_30d, 0), COALESCE(s.sent_total, 0), s.last_sent_at,
			(SELECT COUNT(*) FROM contacts c WHERE c.workspace_id = w.id),
			COALESCE((SELECT array_agg(d.name ORDER BY d.name) FROM domains d WHERE d.workspace_id = w.id AND d.status = 'verified'), '{}'),
			COALESCE((SELECT jsonb_agg(jsonb_build_object('id', u.id, 'name', u.name, 'email', u.email, 'username', u.username, 'role', m.role,
					'forke', u.forke_admin_id IS NOT NULL, 'joined_at', m.created_at,
					'projects', (SELECT COUNT(*) FROM workspace_members x WHERE x.user_id = u.id),
					'last_login', (SELECT MAX(s.expires_at) - interval '14 days' FROM sessions s WHERE s.user_id = u.id)) ORDER BY m.created_at)
				FROM workspace_members m JOIN users u ON u.id = m.user_id WHERE m.workspace_id = w.id), '[]')
		FROM workspaces w
		LEFT JOIN LATERAL (
			SELECT COUNT(*) FILTER (WHERE sent_at > now() - interval '30 days') AS sent_30d, COUNT(*) AS sent_total, MAX(sent_at) AS last_sent_at FROM (
				SELECT sent_at FROM email_recipients WHERE workspace_id = w.id AND sent_at IS NOT NULL
				UNION ALL SELECT sent_at FROM transactional_messages WHERE workspace_id = w.id AND sent_at IS NOT NULL
			) x
		) s ON true
		ORDER BY w.platform DESC, w.created_at DESC`)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Account, error) {
		var a Account
		return a, row.Scan(&a.ID, &a.Name, &a.Plan, &a.Status, &a.Forke, &a.Credits, &a.CreditsUsed, &a.DailyLimit, &a.RatePerSec, &a.WarmupUntil, &a.SendingPaused, &a.CreatedAt,
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
	tag, err := p.Pool.Exec(r.Context(), `UPDATE workspaces SET status = $2 WHERE id = $1 AND NOT platform`, id, body.Status)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	if body.Status == "paused" {
		if _, err := p.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE workspace_id = $1 OR user_id IN (SELECT id FROM users WHERE workspace_id = $1)`, id); err != nil {
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
		deleted, err := accounts.DeleteWorkspace(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if !deleted {
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

func (p *Platform) SetLimits(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Plan       string `json:"plan"`
		DailyLimit int    `json:"daily_limit"`
		RatePerSec int    `json:"rate_limit_per_sec"`
		EndWarmup  bool   `json:"end_warmup"`
	}
	if !httpx.Decode(w, r, &body) {
		return
	}
	switch {
	case body.Plan != "trial" && body.Plan != "paid":
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_plan")
		return
	case body.DailyLimit < 0 || body.DailyLimit > 10_000_000:
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_daily_limit")
		return
	case body.RatePerSec < 1 || body.RatePerSec > 1000:
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_rate")
		return
	}
	tag, err := p.Pool.Exec(r.Context(), `
		UPDATE workspaces SET plan = $2, daily_limit = $3, rate_limit_per_sec = $4, warmup_until = CASE WHEN $5 THEN NULL ELSE warmup_until END
		WHERE id = $1`, id, body.Plan, body.DailyLimit, body.RatePerSec, body.EndWarmup)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"plan": body.Plan, "daily_limit": body.DailyLimit, "rate_limit_per_sec": body.RatePerSec})
}

func (p *Platform) RemoveMember(w http.ResponseWriter, r *http.Request) {
	ws, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	user := chi.URLParam(r, "user")
	if !httpx.ValidID(user) {
		httpx.Error(w, http.StatusNotFound, "not_found")
		return
	}
	err := pgx.BeginFunc(r.Context(), p.Pool, func(tx pgx.Tx) error {
		var forke bool
		if err := tx.QueryRow(r.Context(), `
			SELECT u.forke_admin_id IS NOT NULL AND w.platform FROM workspace_members m JOIN users u ON u.id = m.user_id JOIN workspaces w ON w.id = m.workspace_id
			WHERE m.workspace_id = $1 AND m.user_id = $2`, ws, user).Scan(&forke); err != nil {
			return err
		}
		if forke {
			return errManagedByForke
		}
		return accounts.RemoveMember(r.Context(), tx, ws, user)
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.Is(err, errManagedByForke):
		httpx.Error(w, http.StatusConflict, "team_managed_by_forke")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (p *Platform) DeleteUser(w http.ResponseWriter, r *http.Request) {
	user, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	var names []string
	err := pgx.BeginFunc(r.Context(), p.Pool, func(tx pgx.Tx) error {
		var forke bool
		var email string
		if err := tx.QueryRow(r.Context(), `SELECT forke_admin_id IS NOT NULL, email FROM users WHERE id = $1`, user).Scan(&forke, &email); err != nil {
			return err
		}
		if forke {
			return errManagedByForke
		}
		rows, err := tx.Query(r.Context(), `
			SELECT m.workspace_id FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id
			WHERE m.user_id = $1 AND NOT w.platform AND NOT EXISTS (SELECT 1 FROM workspace_members o WHERE o.workspace_id = m.workspace_id AND o.user_id <> $1)`, user)
		if err != nil {
			return err
		}
		solo, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, ws := range solo {
			rows, err := tx.Query(r.Context(), `SELECT name FROM domains WHERE workspace_id = $1`, ws)
			if err != nil {
				return err
			}
			d, err := pgx.CollectRows(rows, pgx.RowTo[string])
			if err != nil {
				return err
			}
			names = append(names, d...)
			if _, err := accounts.DeleteWorkspace(r.Context(), tx, ws); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM access_requests WHERE lower(email) = lower($1)`, email); err != nil {
			return err
		}
		_, err = tx.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, user)
		return err
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.Error(w, http.StatusNotFound, "not_found")
	case errors.Is(err, errManagedByForke):
		httpx.Error(w, http.StatusConflict, "team_managed_by_forke")
	case err != nil:
		httpx.Internal(w, r, err)
	default:
		p.releaseIdentities(r, names)
		w.WriteHeader(http.StatusNoContent)
	}
}

var errManagedByForke = errors.New("team_managed_by_forke")
