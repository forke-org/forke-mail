package relay

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"slices"
	"strings"
	"time"

	"forkemail/internal/sender"
)

var errHeaderInjection = errors.New("header value contains a line break")

func NewMessageID(from string) string {
	domain := "forke.local"
	if addr, err := mail.ParseAddress(from); err == nil {
		if _, d, ok := strings.Cut(addr.Address, "@"); ok {
			domain = strings.ToLower(d)
		}
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b) + "@" + domain
}

func Build(m sender.Message, messageID string, now time.Time) ([]byte, error) {
	headers := [][2]string{
		{"From", m.From},
		{"To", m.To},
		{"Subject", mime.QEncoding.Encode("utf-8", m.Subject)},
		{"Date", now.Format(time.RFC1123Z)},
		{"Message-ID", "<" + messageID + ">"},
		{"MIME-Version", "1.0"},
	}
	if m.ReplyTo != "" {
		headers = append(headers, [2]string{"Reply-To", m.ReplyTo})
	}
	for _, name := range []string{"List-Unsubscribe", "List-Unsubscribe-Post"} {
		if v := m.Headers[name]; v != "" {
			headers = append(headers, [2]string{name, v})
		}
	}
	tags := m.Tags()
	names := make([]string, 0, len(tags))
	for name := range tags {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		headers = append(headers, [2]string{"X-Forke-" + textproto.CanonicalMIMEHeaderKey(strings.ReplaceAll(name, "_", "-")), tags[name]})
	}

	var body bytes.Buffer
	contentType, err := writeBody(&body, m.Text, m.HTML)
	if err != nil {
		return nil, err
	}
	headers = append(headers, [2]string{"Content-Type", contentType})
	if !strings.HasPrefix(contentType, "multipart/") {
		headers = append(headers, [2]string{"Content-Transfer-Encoding", "quoted-printable"})
	}

	var out bytes.Buffer
	for _, h := range headers {
		if strings.ContainsAny(h[1], "\r\n") {
			return nil, fmt.Errorf("%s: %w", h[0], errHeaderInjection)
		}
		out.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	out.WriteString("\r\n")
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func writeBody(w *bytes.Buffer, text, html string) (string, error) {
	switch {
	case html == "":
		return "text/plain; charset=utf-8", writeQP(w, text)
	case text == "":
		return "text/html; charset=utf-8", writeQP(w, html)
	}
	mw := multipart.NewWriter(w)
	for _, part := range [][2]string{{"text/plain; charset=utf-8", text}, {"text/html; charset=utf-8", html}} {
		pw, err := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {part[0]}, "Content-Transfer-Encoding": {"quoted-printable"}})
		if err != nil {
			return "", err
		}
		qp := quotedprintable.NewWriter(pw)
		if _, err := qp.Write([]byte(crlf(part[1]))); err != nil {
			return "", err
		}
		if err := qp.Close(); err != nil {
			return "", err
		}
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	return "multipart/alternative; boundary=" + mw.Boundary(), nil
}

func writeQP(w *bytes.Buffer, s string) error {
	qp := quotedprintable.NewWriter(w)
	if _, err := qp.Write([]byte(crlf(s))); err != nil {
		return err
	}
	return qp.Close()
}

func crlf(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}
