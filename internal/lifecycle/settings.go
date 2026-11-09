package lifecycle

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/contacts"
	"forkemail/internal/domains"
	"forkemail/internal/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var trackableLink = regexp.MustCompile(`(?i)href\s*=\s*["']https?://`)

type Settings struct {
	Enabled       bool   `json:"enabled"`
	FromName      string `json:"from_name"`
	FromAddress   string `json:"from_address"`
	ReplyTo       string `json:"reply_to"`
	Subject       string `json:"subject"`
	HTML          string `json:"html"`
	InactiveDays  int    `json:"inactive_days"`
	GraceDays     int    `json:"grace_days"`
	MinBroadcasts int    `json:"min_broadcasts"`
}

type Handler struct {
	Pool *pgxpool.Pool
}

func (s *Settings) normalize() {
	s.FromName = strings.TrimSpace(s.FromName)
	s.FromAddress = contacts.NormalizeEmail(s.FromAddress)
	s.ReplyTo = contacts.NormalizeEmail(s.ReplyTo)
	s.Subject = strings.TrimSpace(s.Subject)
	if s.InactiveDays == 0 {
		s.InactiveDays = 180
	}
	if s.GraceDays == 0 {
		s.GraceDays = 30
	}
	if s.MinBroadcasts == 0 {
		s.MinBroadcasts = 3
	}
}

func (s Settings) validate() string {
	switch {
	case !contacts.ValidEmail(s.FromAddress):
		return "invalid_from_address"
	case s.ReplyTo != "" && !contacts.ValidEmail(s.ReplyTo):
		return "invalid_reply_to"
	case s.Subject == "" || utf8.RuneCountInString(s.Subject) > 300 || strings.ContainsAny(s.Subject, "\r\n"):
		return "invalid_subject"
	case !trackableLink.MatchString(s.HTML):
		return "html_needs_a_link"
	case s.InactiveDays < 30 || s.InactiveDays > 730:
		return "invalid_inactive_days"
	case s.GraceDays < 7 || s.GraceDays > 90:
		return "invalid_grace_days"
	case s.MinBroadcasts < 1 || s.MinBroadcasts > 50:
		return "invalid_min_broadcasts"
	}
	return ""
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	var s Settings
	err := h.Pool.QueryRow(r.Context(), `
		SELECT enabled, from_name, from_address, reply_to, subject, html, inactive_days, grace_days, min_broadcasts
		FROM reengagement_settings WHERE workspace_id = $1`, auth.FromContext(r.Context()).WorkspaceID,
	).Scan(&s.Enabled, &s.FromName, &s.FromAddress, &s.ReplyTo, &s.Subject, &s.HTML, &s.InactiveDays, &s.GraceDays, &s.MinBroadcasts)
	if errors.Is(err, pgx.ErrNoRows) {
		s = Settings{InactiveDays: 180, GraceDays: 30, MinBroadcasts: 3}
	} else if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}

func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	ws := auth.FromContext(r.Context()).WorkspaceID
	var s Settings
	if !httpx.Decode(w, r, &s) {
		return
	}
	s.normalize()
	if code := s.validate(); code != "" {
		httpx.Error(w, http.StatusUnprocessableEntity, code)
		return
	}
	verified, err := domains.Verified(r.Context(), h.Pool, ws, s.FromAddress)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if !verified {
		httpx.Error(w, http.StatusUnprocessableEntity, "from_domain_not_verified")
		return
	}
	if _, err := h.Pool.Exec(r.Context(), `
		INSERT INTO reengagement_settings (workspace_id, enabled, from_name, from_address, reply_to, subject, html, inactive_days, grace_days, min_broadcasts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (workspace_id) DO UPDATE SET enabled = EXCLUDED.enabled, from_name = EXCLUDED.from_name,
			from_address = EXCLUDED.from_address, reply_to = EXCLUDED.reply_to, subject = EXCLUDED.subject, html = EXCLUDED.html,
			inactive_days = EXCLUDED.inactive_days, grace_days = EXCLUDED.grace_days, min_broadcasts = EXCLUDED.min_broadcasts,
			updated_at = now()`,
		ws, s.Enabled, s.FromName, s.FromAddress, s.ReplyTo, s.Subject, s.HTML, s.InactiveDays, s.GraceDays, s.MinBroadcasts); err != nil {
		httpx.Internal(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}
