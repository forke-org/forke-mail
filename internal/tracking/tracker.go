package tracking

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"html"
	"regexp"
	"strings"
)

const (
	openPrefix           = "o\n"
	clickPrefix          = "c\n"
	recipientOpenPrefix  = "ro\n"
	recipientClickPrefix = "rc\n"
)

var (
	errInvalidToken = errors.New("invalid_token")
	b64             = base64.RawURLEncoding.Strict()
	hrefPattern     = regexp.MustCompile(`(?i)(<a\b[^>]*?\bhref\s*=\s*)("([^"]*)"|'([^']*)')`)
	mailboxProxies  = []string{"googleimageproxy", "yahoomailproxy"}
	automatedAgents = []string{"bot", "crawler", "spider", "preview", "scanner",
		"barracuda", "proofpoint", "mimecast", "safelinks", "python-requests", "curl/", "go-http-client"}
)

type Tracker struct {
	key     []byte
	baseURL string
}

func NewTracker(secret []byte, baseURL string) Tracker {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte("forke-mail-transactional-tracking"))
	return Tracker{key: m.Sum(nil), baseURL: baseURL}
}

func (t Tracker) Instrument(body, messageID string, opens, clicks bool) string {
	return t.instrument(body, openPrefix+messageID, clickPrefix+messageID, opens, clicks)
}

func (t Tracker) InstrumentRecipient(body, recipientID string, opens, clicks bool) string {
	return t.instrument(body, recipientOpenPrefix+recipientID, recipientClickPrefix+recipientID, opens, clicks)
}

func (t Tracker) instrument(body, openPayload, clickPayload string, opens, clicks bool) string {
	if clicks {
		body = hrefPattern.ReplaceAllStringFunc(body, func(tag string) string {
			parts := hrefPattern.FindStringSubmatch(tag)
			target := html.UnescapeString(parts[3] + parts[4])
			if !trackable(target) || t.own(target) {
				return tag
			}
			return parts[1] + `"` + html.EscapeString(t.baseURL+"/t/c/"+t.sign(clickPayload+"\n"+target)) + `"`
		})
	}
	if opens {
		pixel := `<img src="` + html.EscapeString(t.baseURL+"/t/o/"+t.sign(openPayload)) +
			`" width="1" height="1" alt="" style="display:block;width:1px;height:1px;border:0">`
		if i := strings.LastIndex(strings.ToLower(body), "</body>"); i >= 0 {
			return body[:i] + pixel + body[i:]
		}
		body += pixel
	}
	return body
}

func (t Tracker) own(target string) bool {
	for _, path := range []string{"/unsubscribe/", "/preferences/"} {
		if strings.HasPrefix(target, t.baseURL+path) {
			return true
		}
	}
	return false
}

func trackable(target string) bool {
	lower := strings.ToLower(strings.TrimSpace(target))
	return (strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")) && !strings.ContainsAny(target, " \r\n\t")
}

func (t Tracker) sign(payload string) string {
	return b64.EncodeToString([]byte(payload)) + "." + b64.EncodeToString(t.mac([]byte(payload)))
}

func (t Tracker) verify(token, prefix string) (string, error) {
	encPayload, encSig, ok := strings.Cut(token, ".")
	if !ok {
		return "", errInvalidToken
	}
	payload, err := b64.DecodeString(encPayload)
	if err != nil {
		return "", errInvalidToken
	}
	sig, err := b64.DecodeString(encSig)
	if err != nil || !hmac.Equal(sig, t.mac(payload)) || !strings.HasPrefix(string(payload), prefix) {
		return "", errInvalidToken
	}
	return strings.TrimPrefix(string(payload), prefix), nil
}

func (t Tracker) mac(payload []byte) []byte {
	m := hmac.New(sha256.New, t.key)
	m.Write(payload)
	return m.Sum(nil)
}

func (t Tracker) verifyOpen(token string) (id string, recipient bool, err error) {
	if id, err = t.verify(token, openPrefix); err == nil {
		return id, false, nil
	}
	id, err = t.verify(token, recipientOpenPrefix)
	return id, true, err
}

func (t Tracker) verifyClick(token string) (id, target string, recipient bool, err error) {
	rest, err := t.verify(token, clickPrefix)
	if err != nil {
		if rest, err = t.verify(token, recipientClickPrefix); err != nil {
			return "", "", false, err
		}
		recipient = true
	}
	id, target, err = splitClick(rest)
	return id, target, recipient, err
}

func splitClick(rest string) (messageID, target string, err error) {
	messageID, target, ok := strings.Cut(rest, "\n")
	if !ok || !trackable(target) {
		return "", "", errInvalidToken
	}
	return messageID, target, nil
}

func automated(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	return ua == "" || containsAny(ua, mailboxProxies) || containsAny(ua, automatedAgents)
}

func countableOpen(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	if containsAny(ua, mailboxProxies) {
		return true
	}
	return ua != "" && !containsAny(ua, automatedAgents)
}

func containsAny(s string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}
