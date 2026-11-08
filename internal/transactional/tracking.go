package transactional

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
	openPrefix  = "o\n"
	clickPrefix = "c\n"
)

var (
	errInvalidToken = errors.New("invalid_token")
	b64             = base64.RawURLEncoding.Strict()
	hrefPattern     = regexp.MustCompile(`(?i)(<a\b[^>]*?\bhref\s*=\s*)("([^"]*)"|'([^']*)')`)
	automatedAgents = []string{"googleimageproxy", "yahoomailproxy", "bot", "crawler", "spider", "preview", "scanner",
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
	if clicks {
		body = hrefPattern.ReplaceAllStringFunc(body, func(tag string) string {
			parts := hrefPattern.FindStringSubmatch(tag)
			target := html.UnescapeString(parts[3] + parts[4])
			if !trackable(target) {
				return tag
			}
			return parts[1] + `"` + html.EscapeString(t.baseURL+"/t/c/"+t.sign(clickPrefix+messageID+"\n"+target)) + `"`
		})
	}
	if opens {
		pixel := `<img src="` + html.EscapeString(t.baseURL+"/t/o/"+t.sign(openPrefix+messageID)) +
			`" width="1" height="1" alt="" style="display:block;width:1px;height:1px;border:0">`
		if i := strings.LastIndex(strings.ToLower(body), "</body>"); i >= 0 {
			return body[:i] + pixel + body[i:]
		}
		body += pixel
	}
	return body
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

func (t Tracker) verifyOpen(token string) (string, error) {
	return t.verify(token, openPrefix)
}

func (t Tracker) verifyClick(token string) (messageID, target string, err error) {
	rest, err := t.verify(token, clickPrefix)
	if err != nil {
		return "", "", err
	}
	messageID, target, ok := strings.Cut(rest, "\n")
	if !ok || !trackable(target) {
		return "", "", errInvalidToken
	}
	return messageID, target, nil
}

func automated(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	if ua == "" {
		return true
	}
	for _, marker := range automatedAgents {
		if strings.Contains(ua, marker) {
			return true
		}
	}
	return false
}
