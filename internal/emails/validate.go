package emails

import (
	"context"
	"strings"
	"unicode/utf8"

	"forkemail/internal/contacts"
	"forkemail/internal/db"
	"forkemail/internal/domains"
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
	case e.ReplyTo != "" && (e.ReplyTo != contacts.NormalizeEmail(e.ReplyTo) || !contacts.ValidEmail(e.ReplyTo)):
		return "invalid_reply_to"
	case e.Topic != nil && !contacts.ValidTopicKey(*e.Topic):
		return "invalid_topic"
	case strings.TrimSpace(e.HTML) == "" && strings.TrimSpace(e.Text) == "":
		return "missing_body"
	case len(e.HTML) > maxBodyBytes || len(e.Text) > maxBodyBytes:
		return "body_too_large"
	}
	return e.Audience.validate()
}

func checkDomain(ctx context.Context, q db.Querier, ws, fromAddress string) error {
	verified, err := domains.Verified(ctx, q, ws, fromAddress)
	if err != nil {
		return err
	}
	if !verified {
		return invalidError{"from_domain_not_verified"}
	}
	return nil
}
