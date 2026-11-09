package relay

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/mail"
	"net/smtp"
	"time"

	"forkemail/internal/sender"
)

const dialTimeout = 15 * time.Second

type Sender struct {
	Config Config
}

func (s Sender) Send(ctx context.Context, m sender.Message) (string, error) {
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return "", &sender.SendError{Action: sender.Fail, Reason: "invalid_from", Err: err}
	}
	id := NewMessageID(m.From)
	raw, err := Build(m, id, time.Now())
	if err != nil {
		return "", &sender.SendError{Action: sender.Fail, Reason: "invalid_message", Err: err}
	}
	if err := s.deliver(ctx, from.Address, m.To, raw); err != nil {
		return "", err
	}
	return id, nil
}

func (s Sender) deliver(ctx context.Context, from, to string, raw []byte) error {
	conn, err := s.dial(ctx)
	if err != nil {
		return &sender.SendError{Action: sender.Retry, Reason: "smtp_connect_failed", Err: err}
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c, err := smtp.NewClient(conn, s.Config.Host)
	if err != nil {
		conn.Close()
		return classify(err)
	}
	defer c.Close()
	if err := s.handshake(c); err != nil {
		return err
	}
	if err := c.Mail(from); err != nil {
		return classify(err)
	}
	if err := c.Rcpt(to); err != nil {
		return classify(err)
	}
	wc, err := c.Data()
	if err != nil {
		return classify(err)
	}
	if _, err := wc.Write(raw); err != nil {
		return classify(err)
	}
	if err := wc.Close(); err != nil {
		return unknownOutcome(err)
	}
	_ = c.Quit()
	return nil
}

func (s Sender) handshake(c *smtp.Client) error {
	if s.Config.HeloName != "" {
		if err := c.Hello(s.Config.HeloName); err != nil {
			return classify(err)
		}
	}
	if s.Config.TLS == TLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return &sender.SendError{Action: sender.PauseSending, Reason: "smtp_starttls_unavailable", Err: errors.New("relay does not offer STARTTLS")}
		}
		if err := c.StartTLS(s.tlsConfig()); err != nil {
			return classify(err)
		}
	}
	if s.Config.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Config.Username, s.Config.Password, s.Config.Host)); err != nil {
			return classify(err)
		}
	}
	return nil
}

func (s Sender) dial(ctx context.Context) (net.Conn, error) {
	d := &net.Dialer{Timeout: dialTimeout}
	if s.Config.TLS == TLSImplicit {
		return (&tls.Dialer{NetDialer: d, Config: s.tlsConfig()}).DialContext(ctx, "tcp", s.Config.Addr())
	}
	return d.DialContext(ctx, "tcp", s.Config.Addr())
}

func (s Sender) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: s.Config.Host, MinVersion: tls.VersionTLS12}
}
