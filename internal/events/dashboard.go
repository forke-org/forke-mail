package events

import (
	"net/http"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
)

type dashboard struct {
	SendingMode     string         `json:"sending_mode"`
	Contacts        int            `json:"contacts"`
	ActiveContacts  int            `json:"active_contacts"`
	Campaigns       int            `json:"campaigns"`
	EmailsSent      int            `json:"emails_sent"`
	SentToday       int            `json:"sent_today"`
	SentLast7Days   int            `json:"sent_last_7_days"`
	EventsLast7Days map[string]int `json:"events_last_7_days"`
	RecentEmails    []recentEmail  `json:"recent_emails"`
}

type recentEmail struct {
	ID           string    `json:"id"`
	Subject      string    `json:"subject"`
	CampaignName string    `json:"campaign_name"`
	Status       string    `json:"status"`
	Recipients   int       `json:"recipients"`
	CreatedAt    time.Time `json:"created_at"`
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	d := dashboard{SendingMode: h.SendingMode(), EventsLast7Days: map[string]int{}}
	err := h.Pool.QueryRow(r.Context(), `
		SELECT
			(SELECT COUNT(*) FROM contacts WHERE workspace_id = $1),
			(SELECT COUNT(*) FROM contacts WHERE workspace_id = $1 AND status = 'active'),
			(SELECT COUNT(*) FROM campaigns WHERE workspace_id = $1),
			(SELECT COUNT(*) FROM emails WHERE workspace_id = $1 AND status <> 'draft'),
			(SELECT COALESCE(SUM(sent_count), 0) FROM usage_daily WHERE workspace_id = $1 AND date = current_date),
			(SELECT COALESCE(SUM(sent_count), 0) FROM usage_daily WHERE workspace_id = $1 AND date > current_date - 7)`, ws,
	).Scan(&d.Contacts, &d.ActiveContacts, &d.Campaigns, &d.EmailsSent, &d.SentToday, &d.SentLast7Days)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	for _, t := range types {
		d.EventsLast7Days[t] = 0
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT type, COUNT(*) FROM email_events
		WHERE workspace_id = $1 AND created_at > now() - interval '7 days'
		GROUP BY type`, ws)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			rows.Close()
			httpx.Internal(w, r, err)
			return
		}
		d.EventsLast7Days[t] = n
	}
	if err := rows.Err(); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	rows, err = h.Pool.Query(r.Context(), `
		SELECT e.id, e.subject, c.name, e.status, (SELECT COUNT(*) FROM email_recipients r WHERE r.email_id = e.id), e.created_at
		FROM emails e JOIN campaigns c ON c.id = e.campaign_id
		WHERE e.workspace_id = $1 ORDER BY e.created_at DESC LIMIT 5`, ws)
	if err == nil {
		d.RecentEmails, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (recentEmail, error) {
			var e recentEmail
			return e, row.Scan(&e.ID, &e.Subject, &e.CampaignName, &e.Status, &e.Recipients, &e.CreatedAt)
		})
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}
