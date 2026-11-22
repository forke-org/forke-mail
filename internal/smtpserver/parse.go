package smtpserver

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
)

var (
	errAttachments = errors.New("attachments_not_supported")
	errNoBody      = errors.New("missing_body")
)

type Message struct {
	FromName    string
	FromAddress string
	ReplyTo     string
	Subject     string
	HTML        string
	Text        string
	MessageID   string
}

var words = &mime.WordDecoder{}

func decodeHeader(s string) string {
	out, err := words.DecodeHeader(s)
	if err != nil {
		return s
	}
	return strings.TrimSpace(out)
}

func decodeBody(r io.Reader, encoding string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		return io.ReadAll(base64.NewDecoder(base64.StdEncoding, &stripNewlines{r: r}))
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(r))
	}
	return io.ReadAll(r)
}

type stripNewlines struct{ r io.Reader }

func (s *stripNewlines) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	out := p[:0]
	for _, b := range p[:n] {
		if b != '\r' && b != '\n' {
			out = append(out, b)
		}
	}
	return len(out), err
}

func (m *Message) walk(contentType, encoding, disposition string, body io.Reader, depth int) error {
	if depth > 5 {
		return errAttachments
	}
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		media = "text/plain"
	}
	if d, _, _ := mime.ParseMediaType(disposition); d == "attachment" {
		return errAttachments
	}
	if strings.HasPrefix(media, "multipart/") {
		mr := multipart.NewReader(body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			if err := m.walk(part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Disposition"), part, depth+1); err != nil {
				return err
			}
		}
	}
	data, err := decodeBody(body, encoding)
	if err != nil {
		return err
	}
	switch media {
	case "text/html":
		if m.HTML == "" {
			m.HTML = string(data)
		}
	case "text/plain":
		if m.Text == "" {
			m.Text = string(data)
		}
	default:
		return errAttachments
	}
	return nil
}

func Parse(raw []byte) (Message, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Message{}, err
	}
	m := Message{Subject: decodeHeader(msg.Header.Get("Subject")), MessageID: strings.Trim(msg.Header.Get("Message-Id"), "<> ")}
	if from, err := mail.ParseAddress(msg.Header.Get("From")); err == nil {
		m.FromName, m.FromAddress = decodeHeader(from.Name), from.Address
	}
	if rt, err := mail.ParseAddress(msg.Header.Get("Reply-To")); err == nil {
		m.ReplyTo = rt.Address
	}
	contentType := msg.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "text/plain"
	}
	if err := m.walk(contentType, msg.Header.Get("Content-Transfer-Encoding"), msg.Header.Get("Content-Disposition"), msg.Body, 0); err != nil {
		return Message{}, err
	}
	if strings.TrimSpace(m.HTML) == "" && strings.TrimSpace(m.Text) == "" {
		return Message{}, errNoBody
	}
	return m, nil
}
