package transactional

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	errNotFound          = errors.New("not_found")
	errKeyReused         = errors.New("idempotency_key_reused")
	errSendingPaused     = errors.New("workspace_sending_paused")
	errWorkspaceInactive = errors.New("workspace_inactive")
	errDomainUnverified  = errors.New("from_domain_not_verified")
	errSuppressed        = errors.New("recipient_suppressed")
)

type Handler struct {
	Pool       *pgxpool.Pool
	Dispatcher *Dispatcher
}

type Message struct {
	ID           string            `json:"id"`
	To           string            `json:"to"`
	FromName     string            `json:"from_name"`
	FromAddress  string            `json:"from_address"`
	ReplyTo      *string           `json:"reply_to"`
	Subject      string            `json:"subject"`
	Tags         map[string]string `json:"tags"`
	TrackOpens   bool              `json:"track_opens"`
	TrackClicks  bool              `json:"track_clicks"`
	Status       string            `json:"status"`
	Attempts     int               `json:"attempts"`
	LastError    *string           `json:"last_error"`
	CreatedAt    time.Time         `json:"created_at"`
	SentAt       *time.Time        `json:"sent_at"`
	DispatchMS   *float64          `json:"dispatch_ms"`
	DeliveredAt  *time.Time        `json:"delivered_at"`
	BouncedAt    *time.Time        `json:"bounced_at"`
	BounceType   *string           `json:"bounce_type"`
	ComplainedAt *time.Time        `json:"complained_at"`
	OpenedAt     *time.Time        `json:"opened_at"`
	OpenCount    int               `json:"open_count"`
	ClickedAt    *time.Time        `json:"clicked_at"`
	ClickCount   int               `json:"click_count"`
}

type Detail struct {
	Message
	HTML   string  `json:"html"`
	Text   string  `json:"text"`
	Events []event `json:"events"`
}

type event struct {
	Type      string          `json:"type"`
	Detail    json.RawMessage `json:"detail"`
	CreatedAt time.Time       `json:"created_at"`
}

const selectMessage = `
	SELECT id, to_address, from_name, from_address, reply_to, subject, tags, track_opens, track_clicks,
		status, attempts, last_error, created_at, sent_at,
		EXTRACT(EPOCH FROM (sent_at - created_at)) * 1000,
		delivered_at, bounced_at, bounce_type, complained_at, opened_at, open_count, clicked_at, click_count
	FROM transactional_messages`

func scanTargets(m *Message) []any {
	return []any{&m.ID, &m.To, &m.FromName, &m.FromAddress, &m.ReplyTo, &m.Subject, &m.Tags, &m.TrackOpens, &m.TrackClicks,
		&m.Status, &m.Attempts, &m.LastError, &m.CreatedAt, &m.SentAt, &m.DispatchMS,
		&m.DeliveredAt, &m.BouncedAt, &m.BounceType, &m.ComplainedAt, &m.OpenedAt, &m.OpenCount, &m.ClickedAt, &m.ClickCount}
}
