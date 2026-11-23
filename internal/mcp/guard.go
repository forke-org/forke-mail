// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package mcp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Risk int

const (
	Safe Risk = iota
	Confirm
	Approve
	Blocked
)

var riskName = map[Risk]string{Safe: "safe", Confirm: "confirm", Approve: "approve", Blocked: "blocked"}

type rule struct {
	method string
	path   *regexp.Regexp
	risk   Risk
}

func r(method, pattern string, risk Risk) rule {
	return rule{method, regexp.MustCompile("^" + pattern + "$"), risk}
}

const idPart = `[^/]+`

var rules = []rule{
	r("*", `/api/auth/.*`, Blocked),
	r("*", `/api/platform/.*`, Blocked),
	r("POST", `/api/account/password`, Blocked),
	r("POST", `/api/emails/`+idPart+`/send`, Approve),
	r("POST", `/api/forke-mail/broadcasts/`+idPart+`/send`, Approve),
	r("POST", `/api/emails/`+idPart+`/resume`, Approve),
	r("DELETE", `/api/domains/`+idPart, Approve),
	r("POST", `/api/projects/current/(delete|reset|leave)`, Approve),
	r("POST", `/api/projects/credits`, Approve),
	r("POST", `/api/account/delete`, Approve),
	r("DELETE", `/api/keys/`+idPart, Approve),
	r("POST", `/api/keys/`+idPart+`/rotate`, Approve),
	r("DELETE", `/api/team/members/`+idPart, Approve),
	r("PATCH", `/api/team/members/`+idPart, Approve),
	r("DELETE", `/api/suppressions/`+idPart, Approve),
	r("PUT", `/api/settings/.*`, Confirm),
	r("PATCH", `/api/account`, Confirm),
	r("PATCH", `/api/projects/current`, Confirm),
	r("POST", `/api/transactional`, Confirm),
	r("POST", `/api/forke-mail/emails`, Confirm),
	r("POST", `/api/emails`, Confirm),
	r("POST", `/api/webhooks`, Confirm),
	r("PATCH", `/api/webhooks/`+idPart, Confirm),
	r("POST", `/api/webhooks/`+idPart+`/rotate`, Confirm),
	r("POST", `/api/keys`, Confirm),
	r("POST", `/api/contacts/import`, Confirm),
	r("POST", `/api/suppressions/import`, Confirm),
	r("POST", `/api/team/invites`, Confirm),
	r("POST", `/api/workflows`, Confirm),
	r("PUT", `/api/workflows/`+idPart, Confirm),
	r("DELETE", `/api/.*`, Confirm),
}

func classify(method, path string) Risk {
	path, _, _ = strings.Cut(path, "?")
	if !strings.HasPrefix(path, "/api/") || strings.Contains(path, "..") {
		return Blocked
	}
	for _, rl := range rules {
		if (rl.method == "*" || rl.method == method) && rl.path.MatchString(path) {
			return rl.risk
		}
	}
	return Safe
}

func canonical(tool string, args map[string]any) string {
	clean := map[string]any{}
	for k, v := range args {
		if k != "confirm_token" && k != "approval_id" {
			clean[k] = v
		}
	}
	b, _ := json.Marshal(clean)
	return tool + "\n" + string(b)
}

type confirmer struct{ secret []byte }

func (c confirmer) mac(principal, action string, exp int64) string {
	m := hmac.New(sha256.New, c.secret)
	m.Write([]byte("forke-mail-mcp-confirm\n" + principal + "\n" + action + "\n" + strconv.FormatInt(exp, 10)))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (c confirmer) token(principal, action string, now time.Time) string {
	exp := now.Add(10 * time.Minute).Unix()
	return strconv.FormatInt(exp, 36) + "." + c.mac(principal, action, exp)
}

func (c confirmer) valid(principal, action, token string, now time.Time) bool {
	expPart, sig, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expPart, 36, 64)
	if err != nil || now.Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(c.mac(principal, action, exp)))
}
