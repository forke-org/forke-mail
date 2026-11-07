package emails

import (
	"context"
	"strings"
	"unicode/utf8"

	"forkemail/internal/contacts"
	"forkemail/internal/db"
)

const maxBodyBytes = 256 << 10

func validate(e Email) string {
	switch {
	case e.Subject == "" || utf8.RuneCountInString(e.Subject) > 300 || strings.ContainsAny(e.Subject, "\r\n"):
		return "invalid_subject"
	case utf8.RuneCountInString(e.FromName) > 100 || strings.ContainsAny(e.FromName, "\r\n"):
		return "invalid_from_name"
	case e.FromAddress != contacts.NormalizeEmail(e.FromAddress) || !contacts.ValidEmail(e.FromAddress):
		return "invalid_from_address"
	case strings.TrimSpace(e.HTML) == "" && strings.TrimSpace(e.Text) == "":
		return "missing_body"
	case len(e.HTML) > maxBodyBytes || len(e.Text) > maxBodyBytes:
		return "body_too_large"
	}
	return e.Audience.validate()
}

func checkDomain(ctx context.Context, q db.Querier, ws, fromAddress string) error {
	domain := fromAddress[strings.LastIndex(fromAddress, "@")+1:]
	var verified bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM domains WHERE workspace_id = $1 AND status = 'verified'
			AND ((kind = 'domain' AND lower(name) = lower($2)) OR (kind = 'email' AND lower(name) = lower($3))))`,
		ws, domain, fromAddress).Scan(&verified); err != nil {
		return err
	}
	if !verified {
		return invalidError{"from_domain_not_verified"}
	}
	return nil
}
