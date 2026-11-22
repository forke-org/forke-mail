// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package smtpserver

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/contacts"
	"forkemail/internal/transactional"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"golang.org/x/time/rate"
)

const (
	Username       = "forke"
	maxMessage     = 1 << 20
	maxRecipients  = 50
	sessionTimeout = 2 * time.Minute
)

type Keys interface {
	IdentifyKey(ctx context.Context, plain string) (auth.Identity, error)
}

type Mailer interface {
	Accept(ctx context.Context, who auth.Identity, key string, req transactional.Submission) (transactional.Acceptance, error)
}

type Config struct {
	Hostname string
	Addr     string
	TLSAddr  string
	CertFile string
	KeyFile  string
}

func (c Config) Enabled() bool {
	return (c.Addr != "" || c.TLSAddr != "") && c.CertFile != "" && c.KeyFile != ""
}

type Backend struct {
	Keys     Keys
	Mail     Mailer
	failures sync.Map
}

func (b *Backend) limiter(ip string) *rate.Limiter {
	v, _ := b.failures.LoadOrStore(ip, rate.NewLimiter(rate.Every(6*time.Second), 10))
	return v.(*rate.Limiter)
}

func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	ip, _, _ := net.SplitHostPort(c.Conn().RemoteAddr().String())
	return &session{backend: b, ip: ip, tls: func() bool { _, ok := c.TLSConnectionState(); return ok }}, nil
}

type session struct {
	backend *Backend
	ip      string
	tls     func() bool
	who     *auth.Identity
	from    string
	to      []string
}

func smtpError(code int, enhanced smtp.EnhancedCode, msg string) *smtp.SMTPError {
	return &smtp.SMTPError{Code: code, EnhancedCode: enhanced, Message: msg}
}

var (
	errAuthRequired = smtpError(530, smtp.EnhancedCode{5, 7, 0}, "Authentication required: username forke, password an API key")
	errAuthFailed   = smtpError(535, smtp.EnhancedCode{5, 7, 8}, "Invalid username or API key")
	errTooMany      = smtpError(421, smtp.EnhancedCode{4, 7, 0}, "Too many failed logins, try again later")
	errNeedTLS      = smtpError(538, smtp.EnhancedCode{5, 7, 11}, "Use STARTTLS or port 465 before logging in")
)

func (s *session) AuthMechanisms() []string { return []string{sasl.Plain, sasl.Login} }

func (s *session) check(username, password string) error {
	if !s.tls() {
		return errNeedTLS
	}
	lim := s.backend.limiter(s.ip)
	if lim.Tokens() < 1 {
		return errTooMany
	}
	if username != Username {
		lim.Allow()
		return errAuthFailed
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := s.backend.Keys.IdentifyKey(ctx, strings.TrimSpace(password))
	if err != nil || !auth.Allowed(id.Scope, "POST /api/transactional") {
		lim.Allow()
		return errAuthFailed
	}
	s.who = &id
	return nil
}

func (s *session) Auth(mech string) (sasl.Server, error) {
	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(_, username, password string) error { return s.check(username, password) }), nil
	case sasl.Login:
		return newLoginServer(s.check), nil
	}
	return nil, smtp.ErrAuthUnknownMechanism
}

func (s *session) Mail(from string, _ *smtp.MailOptions) error {
	if s.who == nil {
		return errAuthRequired
	}
	s.from = contacts.NormalizeEmail(from)
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	if s.who == nil {
		return errAuthRequired
	}
	addr := contacts.NormalizeEmail(to)
	if !contacts.ValidEmail(addr) {
		return smtpError(553, smtp.EnhancedCode{5, 1, 3}, "Invalid recipient address")
	}
	if len(s.to) >= maxRecipients {
		return smtpError(452, smtp.EnhancedCode{4, 5, 3}, "Too many recipients")
	}
	s.to = append(s.to, addr)
	return nil
}

func reject(err error) error {
	code := err.Error()
	switch code {
	case "attachments_not_supported":
		return smtpError(554, smtp.EnhancedCode{5, 6, 0}, "Attachments are not supported yet")
	case "missing_body":
		return smtpError(554, smtp.EnhancedCode{5, 6, 0}, "The message has no text or HTML body")
	case "from_domain_not_verified", "invalid_from_address":
		return smtpError(550, smtp.EnhancedCode{5, 7, 1}, "Sender domain is not verified in this Forke Mail project")
	case "recipient_suppressed":
		return smtpError(550, smtp.EnhancedCode{5, 1, 1}, "Recipient is on the suppression list")
	case "workspace_sending_paused", "workspace_inactive":
		return smtpError(554, smtp.EnhancedCode{5, 7, 1}, "Sending is paused for this project")
	case "trial_quota_exceeded", "daily_limit_reached", "monthly_limit_reached":
		return smtpError(452, smtp.EnhancedCode{4, 2, 2}, "Sending limit reached")
	}
	if strings.HasPrefix(code, "invalid_") || code == "body_too_large" {
		return smtpError(554, smtp.EnhancedCode{5, 6, 0}, "Message rejected: "+code)
	}
	slog.Error("smtp accept", "err", err)
	return smtpError(451, smtp.EnhancedCode{4, 3, 0}, "Temporary failure, try again")
}

func (s *session) Data(r io.Reader) error {
	if s.who == nil {
		return errAuthRequired
	}
	if len(s.to) == 0 {
		return smtpError(554, smtp.EnhancedCode{5, 5, 1}, "No valid recipients")
	}
	raw, err := io.ReadAll(io.LimitReader(r, maxMessage+1))
	if err != nil {
		return err
	}
	if len(raw) > maxMessage {
		return smtpError(552, smtp.EnhancedCode{5, 3, 4}, "Message too big")
	}
	m, err := Parse(raw)
	if err != nil {
		return reject(err)
	}
	from := m.FromAddress
	if from == "" {
		from = s.from
	}
	digest := sha256.Sum256(raw)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, to := range s.to {
		key := "smtp-" + hex.EncodeToString(digest[:12]) + "-" + to
		if len(key) > 200 {
			key = key[:200]
		}
		_, err := s.backend.Mail.Accept(ctx, *s.who, key, transactional.Submission{
			To: to, FromAddress: from, FromName: m.FromName, ReplyTo: m.ReplyTo, Subject: m.Subject, HTML: m.HTML, Text: m.Text,
			Tags: map[string]string{"source": "smtp"},
		})
		if err != nil {
			return reject(err)
		}
	}
	return nil
}

func (s *session) Reset() {
	s.from = ""
	s.to = nil
}

func (s *session) Logout() error { return nil }

type certificate struct {
	certFile, keyFile string
	mu                sync.Mutex
	loaded            time.Time
	cert              *tls.Certificate
}

func (c *certificate) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info, err := os.Stat(c.certFile)
	if err != nil {
		return nil, err
	}
	if c.cert == nil || info.ModTime().After(c.loaded) {
		cert, err := tls.LoadX509KeyPair(c.certFile, c.keyFile)
		if err != nil {
			return nil, err
		}
		c.cert, c.loaded = &cert, info.ModTime()
	}
	return c.cert, nil
}

func Run(ctx context.Context, cfg Config, b *Backend) {
	cert := &certificate{certFile: cfg.CertFile, keyFile: cfg.KeyFile}
	if _, err := cert.get(nil); err != nil {
		slog.Error("smtp server certificate", "err", err)
		return
	}
	tlsConfig := &tls.Config{GetCertificate: cert.get, MinVersion: tls.VersionTLS12}
	var servers []*smtp.Server
	start := func(addr string, implicit bool) {
		s := smtp.NewServer(b)
		s.Addr, s.Domain, s.TLSConfig = addr, cfg.Hostname, tlsConfig
		s.MaxMessageBytes, s.MaxRecipients = maxMessage, maxRecipients
		s.ReadTimeout, s.WriteTimeout = sessionTimeout, sessionTimeout
		servers = append(servers, s)
		go func() {
			var err error
			if implicit {
				err = s.ListenAndServeTLS()
			} else {
				err = s.ListenAndServe()
			}
			if err != nil && !errors.Is(err, smtp.ErrServerClosed) {
				slog.Error("smtp server", "addr", addr, "err", err)
			}
		}()
		slog.Info("smtp server", "addr", addr, "implicit_tls", implicit)
	}
	if cfg.Addr != "" {
		start(cfg.Addr, false)
	}
	if cfg.TLSAddr != "" {
		start(cfg.TLSAddr, true)
	}
	<-ctx.Done()
	for _, s := range servers {
		_ = s.Close()
	}
}
