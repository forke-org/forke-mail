package emails

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	errNotFound      = errors.New("not_found")
	errNotDraft      = errors.New("email_not_draft")
	errInvalidState  = errors.New("invalid_email_state")
	errSendingPaused = errors.New("workspace_sending_paused")
)

type invalidError struct{ code string }

func (e invalidError) Error() string { return e.code }

type Handler struct {
	Pool *pgxpool.Pool
}

type Email struct {
	ID           string     `json:"id"`
	CampaignID   string     `json:"campaign_id"`
	CampaignName string     `json:"campaign_name"`
	Subject      string     `json:"subject"`
	FromName     string     `json:"from_name"`
	FromAddress  string     `json:"from_address"`
	ReplyTo      string     `json:"reply_to"`
	HTML         string     `json:"html"`
	Text         string     `json:"text"`
	Status       string     `json:"status"`
	PauseReason  *string    `json:"pause_reason"`
	ScheduledAt  *time.Time `json:"scheduled_at"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	CreatedAt    time.Time  `json:"created_at"`
	Recipients   int        `json:"recipients"`
	Processed    int        `json:"processed"`
	Audience     Audience   `json:"audience"`
}

type fields struct {
	Subject     *string   `json:"subject"`
	FromName    *string   `json:"from_name"`
	FromAddress *string   `json:"from_address"`
	ReplyTo     *string   `json:"reply_to"`
	HTML        *string   `json:"html"`
	Text        *string   `json:"text"`
	Audience    *Audience `json:"audience"`
}

func (f fields) applyTo(e *Email) {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	set(&e.Subject, f.Subject)
	set(&e.FromName, f.FromName)
	set(&e.FromAddress, f.FromAddress)
	set(&e.ReplyTo, f.ReplyTo)
	set(&e.HTML, f.HTML)
	set(&e.Text, f.Text)
	if f.Audience != nil {
		e.Audience = *f.Audience
	}
}

const selectEmail = `
	SELECT e.id, e.campaign_id, c.name, e.subject, e.from_name, e.from_address, e.reply_to, e.html, e.text, e.status, e.pause_reason,
		e.scheduled_at, e.started_at, e.finished_at, e.created_at, s.total, s.processed
	FROM emails e
	JOIN campaigns c ON c.id = e.campaign_id
	CROSS JOIN LATERAL (
		SELECT COUNT(*) AS total, COUNT(*) FILTER (WHERE r.status IN ('sent', 'failed', 'skipped')) AS processed
		FROM email_recipients r WHERE r.email_id = e.id
	) s`

func scanTargets(e *Email) []any {
	return []any{&e.ID, &e.CampaignID, &e.CampaignName, &e.Subject, &e.FromName, &e.FromAddress, &e.ReplyTo, &e.HTML, &e.Text,
		&e.Status, &e.PauseReason, &e.ScheduledAt, &e.StartedAt, &e.FinishedAt, &e.CreatedAt, &e.Recipients, &e.Processed}
}
