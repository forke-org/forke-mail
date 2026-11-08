package contacts

import (
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxNameLength = 100

func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func ValidEmail(s string) bool {
	if len(s) < 3 || len(s) > 254 || strings.ContainsAny(s, "[]\"\\ ") {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Name != "" || addr.Address != s {
		return false
	}
	at := strings.LastIndex(s, "@")
	local, domain := s[:at], s[at+1:]
	return local != "" && len(local) <= 64 &&
		strings.Contains(domain, ".") &&
		!strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".") &&
		!strings.Contains(domain, "..")
}

func normalizeName(s string) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
	for utf8.RuneCountInString(s) > maxNameLength {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}
