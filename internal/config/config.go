package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"

	"forkemail/internal/relay"
)

const minSecretLength = 32

type Config struct {
	Env                 string
	Addr                string
	DatabaseURL         string
	PublicBaseURL       string
	UnsubscribeSecret   string
	SeedToken           string
	TrustedProxy        netip.Prefix
	SenderDriver        string
	AWS                 AWS
	Relay               relay.Config
	SESTestEmail        string
	MaxBounceRate       float64
	MaxComplaintRate    float64
	ReputationMinSample int
	SendRatePerSec      float64
	TrialSendRatePerSec float64
	PaidRatePerSec      float64
	PaidBurst           int
	TrialRatePerSec     float64
	TrialBurst          int
	PublicRatePerSec    float64
	PublicBurst         int
	AuthFailuresPerMin  float64
	AuthFailureBurst    int
}

func (c Config) Development() bool {
	return c.Env == "development"
}

func (c Config) SES() bool {
	return c.SenderDriver == "ses"
}

func (c Config) SMTP() bool {
	return c.SenderDriver == "smtp"
}

func Load() (Config, error) {
	var errs []error
	c := Config{
		Env:               get("APP_ENV", "production"),
		Addr:              get("HTTP_ADDR", ":8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		PublicBaseURL:     os.Getenv("PUBLIC_BASE_URL"),
		UnsubscribeSecret: os.Getenv("UNSUBSCRIBE_SECRET"),
		SeedToken:         os.Getenv("SEED_TOKEN"),
		SenderDriver:      get("SENDER_DRIVER", "log"),
		SESTestEmail:      strings.ToLower(strings.TrimSpace(os.Getenv("SES_TEST_EMAIL"))),
	}
	c.SendRatePerSec = number(&errs, "SEND_RATE_PER_SEC", 20)
	c.TrialSendRatePerSec = number(&errs, "TRIAL_SEND_RATE_PER_SEC", 1)
	c.PaidRatePerSec = number(&errs, "API_PAID_RATE_PER_SEC", 20)
	c.PaidBurst = int(number(&errs, "API_PAID_BURST", 40))
	c.TrialRatePerSec = number(&errs, "API_TRIAL_RATE_PER_SEC", 2)
	c.TrialBurst = int(number(&errs, "API_TRIAL_BURST", 5))
	c.PublicRatePerSec = number(&errs, "PUBLIC_RATE_PER_SEC", 10)
	c.PublicBurst = int(number(&errs, "PUBLIC_BURST", 30))
	c.AuthFailuresPerMin = number(&errs, "AUTH_FAILURES_PER_MIN", 10)
	c.AuthFailureBurst = int(number(&errs, "AUTH_FAILURE_BURST", 10))
	c.MaxBounceRate = fraction(&errs, "MAX_BOUNCE_RATE", 0.05)
	c.MaxComplaintRate = fraction(&errs, "MAX_COMPLAINT_RATE", 0.001)
	c.ReputationMinSample = int(number(&errs, "REPUTATION_MIN_SAMPLE", 100))

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(c.UnsubscribeSecret) < minSecretLength {
		errs = append(errs, fmt.Errorf("UNSUBSCRIBE_SECRET must be at least %d characters", minSecretLength))
	}
	if c.Development() && len(c.SeedToken) < minSecretLength {
		errs = append(errs, fmt.Errorf("SEED_TOKEN must be at least %d characters", minSecretLength))
	}
	if err := ValidateBaseURL(c.PublicBaseURL, c.Development() && !c.SES() && !c.SMTP()); err != nil {
		errs = append(errs, err)
	}
	switch c.SenderDriver {
	case "log":
	case "ses":
		aws, err := LoadAWS()
		c.AWS = aws
		if err != nil {
			errs = append(errs, err)
		}
		if aws.TopicARN == "" {
			errs = append(errs, errors.New("SNS_TOPIC_ARN is required when SENDER_DRIVER=ses (run sesbootstrap to create it)"))
		}
	case "smtp":
		c.Relay = loadSMTP(&errs)
		if err := c.Relay.Validate(c.Development()); err != nil {
			errs = append(errs, err)
		}
	default:
		errs = append(errs, errors.New("SENDER_DRIVER must be log, ses or smtp"))
	}
	if c.SESTestEmail != "" && !strings.Contains(c.SESTestEmail, "@") {
		errs = append(errs, errors.New("SES_TEST_EMAIL must be an email address"))
	}
	if raw := os.Getenv("TRUSTED_PROXY"); raw != "" {
		p, err := parsePrefix(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("TRUSTED_PROXY: %w", err))
		}
		c.TrustedProxy = p
	}
	return c, errors.Join(errs...)
}

func ValidateBaseURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(raw)
	switch {
	case raw == "":
		return errors.New("PUBLIC_BASE_URL is required")
	case err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "":
		return errors.New("PUBLIC_BASE_URL must be an absolute URL such as https://mail.example.com")
	case strings.HasSuffix(raw, "/"):
		return errors.New("PUBLIC_BASE_URL must not end with a slash")
	case u.Scheme == "https", allowHTTP && u.Scheme == "http":
		return nil
	}
	return errors.New("PUBLIC_BASE_URL must use https (http is only allowed in development with SENDER_DRIVER=log)")
}

func loadSMTP(errs *[]error) relay.Config {
	c := relay.Config{
		Host:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
		Port:     587,
		Username: os.Getenv("SMTP_USERNAME"),
		Password: os.Getenv("SMTP_PASSWORD"),
		TLS:      get("SMTP_TLS", relay.TLSStartTLS),
		HeloName: strings.TrimSpace(os.Getenv("SMTP_HELO_NAME")),
	}
	if raw := os.Getenv("SMTP_PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			*errs = append(*errs, errors.New("SMTP_PORT must be a number"))
		}
		c.Port = port
	}
	c.DailyLimit = count(errs, "SMTP_DAILY_LIMIT")
	c.MonthlyLimit = count(errs, "SMTP_MONTHLY_LIMIT")
	for _, s := range strings.Split(os.Getenv("SMTP_ALLOWED_SENDERS"), ",") {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			c.AllowedSenders = append(c.AllowedSenders, s)
		}
	}
	return c
}

func count(errs *[]error, key string) int {
	raw := os.Getenv(key)
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		*errs = append(*errs, fmt.Errorf("%s must be zero or a positive whole number", key))
		return 0
	}
	return v
}

func get(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func number(errs *[]error, key string, fallback float64) float64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 {
		*errs = append(*errs, fmt.Errorf("%s must be a positive number", key))
		return fallback
	}
	return v
}

func fraction(errs *[]error, key string, fallback float64) float64 {
	v := number(errs, key, fallback)
	if v > 1 {
		*errs = append(*errs, fmt.Errorf("%s must be between 0 and 1", key))
	}
	return v
}

func parsePrefix(raw string) (netip.Prefix, error) {
	if strings.Contains(raw, "/") {
		return netip.ParsePrefix(raw)
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}
