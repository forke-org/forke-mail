package domains

import (
	"regexp"
	"strings"

	"forkemail/internal/contacts"
)

var labelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

type Record struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

func ValidName(kind, name string) bool {
	switch kind {
	case "email":
		return len(name) <= 253 && contacts.ValidEmail(name)
	case "domain":
		labels := strings.Split(name, ".")
		if len(name) > 253 || len(labels) < 2 || strings.Trim(labels[len(labels)-1], "0123456789") == "" {
			return false
		}
		for _, l := range labels {
			if !labelPattern.MatchString(l) {
				return false
			}
		}
		return true
	}
	return false
}

func DNSRecords(kind, name string, tokens []string) []Record {
	if kind != "domain" {
		return []Record{}
	}
	records := make([]Record, 0, len(tokens)+1)
	for _, t := range tokens {
		records = append(records, Record{Type: "CNAME", Name: t + "._domainkey." + name, Value: t + ".dkim.amazonses.com"})
	}
	return append(records,
		Record{Type: "TXT", Name: "_dmarc." + name, Value: "v=DMARC1; p=none;"},
	)
}
