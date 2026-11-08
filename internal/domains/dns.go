package domains

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DNSResolver interface {
	LookupTXT(context.Context, string) ([]string, error)
	LookupCNAME(context.Context, string) (string, error)
	LookupMX(context.Context, string) ([]*net.MX, error)
}

type DNSCheck struct {
	Record
	Status   string   `json:"status"`
	Observed []string `json:"observed"`
	Reason   string   `json:"reason,omitempty"`
}

type DNSReport struct {
	Mode                  string     `json:"mail_from_mode"`
	ConfigurationRequired bool       `json:"ses_configuration_required"`
	Checks                []DNSCheck `json:"checks"`
	CheckedAt             time.Time  `json:"checked_at"`
}

type DNSHandler struct {
	Pool     *pgxpool.Pool
	Resolver DNSResolver
	Region   string
}

func (h *DNSHandler) Check(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ws := auth.FromContext(r.Context()).WorkspaceID
	var name, kind string
	var tokens []string
	err := h.Pool.QueryRow(r.Context(), `SELECT name,kind,dkim_tokens FROM domains WHERE id=$1 AND workspace_id=$2`, id, ws).Scan(&name, &kind, &tokens)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, 404, "not_found")
		return
	}
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	if kind != "domain" {
		httpx.Error(w, 422, "domain_identity_required")
		return
	}
	mailFrom := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mail_from")))
	if mailFrom != "" && (!ValidName("domain", mailFrom) || !strings.HasSuffix(mailFrom, "."+name)) {
		httpx.Error(w, 422, "invalid_mail_from_domain")
		return
	}
	region := h.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
	}
	if mailFrom != "" && region == "" {
		httpx.Error(w, 503, "aws_region_required_for_preview")
		return
	}
	resolver := h.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	httpx.JSON(w, 200, CheckDNS(ctx, resolver, name, tokens, mailFrom, region))
}

var dmarcPolicy = regexp.MustCompile(`(^|;)\s*p=(none|quarantine|reject)\s*(;|$)`)

func CheckDNS(ctx context.Context, resolver DNSResolver, name string, tokens []string, mailFrom, region string) DNSReport {
	out := DNSReport{Mode: "ses_default", Checks: []DNSCheck{}, CheckedAt: time.Now().UTC()}
	for _, token := range tokens {
		record := Record{Type: "CNAME", Name: token + "._domainkey." + name, Value: token + ".dkim.amazonses.com"}
		value, err := resolver.LookupCNAME(ctx, record.Name)
		out.Checks = append(out.Checks, checked(record, []string{value}, err, strings.EqualFold(strings.TrimSuffix(value, "."), record.Value)))
	}
	if len(tokens) == 0 {
		out.Checks = append(out.Checks, DNSCheck{Record: Record{Type: "CNAME", Name: "_domainkey." + name}, Status: "pending", Observed: []string{}, Reason: "awaiting_ses_dkim_tokens"})
	}
	if mailFrom != "" {
		out.Mode = "custom_dns_preview"
		out.ConfigurationRequired = true
		record := Record{Type: "TXT", Name: mailFrom, Value: "v=spf1 include:amazonses.com ~all"}
		values, err := resolver.LookupTXT(ctx, mailFrom)
		var policies []string
		for _, v := range values {
			if strings.HasPrefix(strings.ToLower(v), "v=spf1 ") {
				policies = append(policies, v)
			}
		}
		match := len(policies) == 1 && authorizesSES(policies[0])
		out.Checks = append(out.Checks, checked(record, values, err, match))
		if region != "" {
			record = Record{Type: "MX", Name: mailFrom, Value: "10 feedback-smtp." + region + ".amazonses.com"}
			mxs, mxerr := resolver.LookupMX(ctx, mailFrom)
			observed := []string{}
			match = false
			for _, mx := range mxs {
				observed = append(observed, mx.Host)
				if mx.Pref == 10 && strings.EqualFold(strings.TrimSuffix(mx.Host, "."), "feedback-smtp."+region+".amazonses.com") {
					match = true
				}
			}
			out.Checks = append(out.Checks, checked(record, observed, mxerr, match && len(mxs) == 1))
		}
	}
	record := Record{Type: "TXT", Name: "_dmarc." + name, Value: "v=DMARC1; p=none;"}
	values, err := resolver.LookupTXT(ctx, record.Name)
	var policies []string
	for _, v := range values {
		if strings.HasPrefix(v, "v=DMARC1;") {
			policies = append(policies, v)
		}
	}
	out.Checks = append(out.Checks, checked(record, values, err, len(policies) == 1 && dmarcPolicy.MatchString(policies[0])))
	return out
}

func authorizesSES(policy string) bool {
	for _, mechanism := range strings.Fields(strings.ToLower(policy)) {
		switch mechanism {
		case "include:amazonses.com", "+include:amazonses.com":
			return true
		case "all", "+all", "-all", "~all", "?all":
			return false
		}
	}
	return false
}

func checked(r Record, observed []string, err error, matches bool) DNSCheck {
	c := DNSCheck{Record: r, Status: "mismatch", Observed: observed}
	if observed == nil {
		c.Observed = []string{}
	}
	if err != nil || len(observed) == 0 || (len(observed) == 1 && observed[0] == "") {
		c.Status = "pending"
		c.Reason = "dns_missing_or_unavailable"
	} else if matches {
		c.Status = "found"
	}
	return c
}
