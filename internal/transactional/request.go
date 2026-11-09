package transactional

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"forkemail/internal/contacts"
)

const (
	maxBodyBytes      = 256 << 10
	maxTags           = 10
	maxTagLength      = 64
	maxKeyLength      = 256
	idempotencyHeader = "Idempotency-Key"
)

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type request struct {
	To          string            `json:"to"`
	FromName    string            `json:"from_name"`
	FromAddress string            `json:"from_address"`
	ReplyTo     string            `json:"reply_to"`
	Subject     string            `json:"subject"`
	HTML        string            `json:"html"`
	Text        string            `json:"text"`
	Tags        map[string]string `json:"tags"`
	TrackOpens  bool              `json:"track_opens"`
	TrackClicks bool              `json:"track_clicks"`
}

func (r *request) normalize() {
	r.To = contacts.NormalizeEmail(r.To)
	r.FromAddress = contacts.NormalizeEmail(r.FromAddress)
	r.ReplyTo = contacts.NormalizeEmail(r.ReplyTo)
	r.FromName = strings.TrimSpace(r.FromName)
	r.Subject = strings.TrimSpace(r.Subject)
	if r.Tags == nil {
		r.Tags = map[string]string{}
	}
}

func (r request) validate() string {
	switch {
	case !contacts.ValidEmail(r.To):
		return "invalid_to"
	case !contacts.ValidEmail(r.FromAddress):
		return "invalid_from_address"
	case utf8.RuneCountInString(r.FromName) > 100 || strings.ContainsAny(r.FromName, "\r\n"):
		return "invalid_from_name"
	case r.ReplyTo != "" && !contacts.ValidEmail(r.ReplyTo):
		return "invalid_reply_to"
	case r.Subject == "" || utf8.RuneCountInString(r.Subject) > 300 || strings.ContainsAny(r.Subject, "\r\n"):
		return "invalid_subject"
	case strings.TrimSpace(r.HTML) == "" && strings.TrimSpace(r.Text) == "":
		return "missing_body"
	case len(r.HTML) > maxBodyBytes || len(r.Text) > maxBodyBytes:
		return "body_too_large"
	case r.TrackClicks && strings.TrimSpace(r.HTML) == "", r.TrackOpens && strings.TrimSpace(r.HTML) == "":
		return "tracking_requires_html"
	case !validTags(r.Tags):
		return "invalid_tags"
	}
	return ""
}

func validTags(tags map[string]string) bool {
	if len(tags) > maxTags {
		return false
	}
	for k, v := range tags {
		if len(k) > maxTagLength || len(v) > maxTagLength || !tagPattern.MatchString(k) || !tagPattern.MatchString(v) {
			return false
		}
	}
	return true
}

func validIdempotencyKey(key string) bool {
	if key == "" || len(key) > maxKeyLength {
		return false
	}
	for _, c := range key {
		if c < 0x21 || c > 0x7e {
			return false
		}
	}
	return true
}

func (r request) hash() string {
	raw, _ := json.Marshal(r)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (r request) fingerprint() string {
	h := sha256.New()
	for _, part := range []string{r.To, r.FromName, r.FromAddress, r.ReplyTo, r.Subject, r.HTML, r.Text} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
