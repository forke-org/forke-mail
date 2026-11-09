package relay

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	TLSStartTLS = "starttls"
	TLSImplicit = "tls"
	TLSNone     = "none"
)

type Config struct {
	Host           string
	Port           int
	Username       string
	Password       string
	TLS            string
	HeloName       string
	DailyLimit     int
	MonthlyLimit   int
	AllowedSenders []string
}

func (c Config) Addr() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
}

func (c Config) Validate(development bool) error {
	var errs []error
	if c.Host == "" {
		errs = append(errs, errors.New("SMTP_HOST is required when SENDER_DRIVER=smtp"))
	}
	if c.Port <= 0 || c.Port > 65535 {
		errs = append(errs, errors.New("SMTP_PORT must be a valid port"))
	}
	switch c.TLS {
	case TLSStartTLS, TLSImplicit:
	case TLSNone:
		if !development {
			errs = append(errs, errors.New("SMTP_TLS=none is only allowed in development"))
		}
	default:
		errs = append(errs, fmt.Errorf("SMTP_TLS must be %s, %s or %s", TLSStartTLS, TLSImplicit, TLSNone))
	}
	if (c.Username == "") != (c.Password == "") {
		errs = append(errs, errors.New("SMTP_USERNAME and SMTP_PASSWORD must be set together"))
	}
	if len(c.AllowedSenders) == 0 {
		errs = append(errs, errors.New("SMTP_ALLOWED_SENDERS must list the domains or addresses approved at the relay"))
	}
	for _, s := range c.AllowedSenders {
		if s == "" || strings.ContainsAny(s, " /") {
			errs = append(errs, fmt.Errorf("SMTP_ALLOWED_SENDERS has an invalid entry %q", s))
		}
	}
	if c.DailyLimit < 0 || c.MonthlyLimit < 0 {
		errs = append(errs, errors.New("SMTP_DAILY_LIMIT and SMTP_MONTHLY_LIMIT must not be negative"))
	}
	return errors.Join(errs...)
}
