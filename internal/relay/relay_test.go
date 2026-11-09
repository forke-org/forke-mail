package relay

import (
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"forkemail/internal/sender"
)

type fakeRelay struct {
	addr     string
	rcptCode int
	mu       sync.Mutex
	from     string
	to       string
	data     string
	auth     string
}

func startRelay(t *testing.T, rcptCode int) *fakeRelay {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	f := &fakeRelay{addr: ln.Addr().String(), rcptCode: rcptCode}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return f
}

func (f *fakeRelay) serve(conn net.Conn) {
	defer conn.Close()
	tp := textproto.NewConn(conn)
	_ = tp.PrintfLine("220 fake ready")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		f.mu.Lock()
		switch verb {
		case "EHLO", "HELO":
			_ = tp.PrintfLine("250-fake")
			_ = tp.PrintfLine("250 AUTH PLAIN")
		case "AUTH":
			f.auth = line
			_ = tp.PrintfLine("235 ok")
		case "MAIL":
			f.from = line
			_ = tp.PrintfLine("250 ok")
		case "RCPT":
			f.to = line
			if f.rcptCode != 250 {
				_ = tp.PrintfLine("%d no thanks", f.rcptCode)
			} else {
				_ = tp.PrintfLine("250 ok")
			}
		case "DATA":
			_ = tp.PrintfLine("354 go")
			f.mu.Unlock()
			b, _ := io.ReadAll(tp.DotReader())
			f.mu.Lock()
			f.data = string(b)
			_ = tp.PrintfLine("250 queued")
		case "QUIT":
			_ = tp.PrintfLine("221 bye")
			f.mu.Unlock()
			return
		default:
			_ = tp.PrintfLine("502 unknown")
		}
		f.mu.Unlock()
	}
}

func relayConfig(t *testing.T, f *fakeRelay) Config {
	host, port, _ := net.SplitHostPort(f.addr)
	p, _ := strconv.Atoi(port)
	return Config{Host: host, Port: p, Username: "user", Password: "pass", TLS: TLSNone, AllowedSenders: []string{"forke.space"}}
}

func message() sender.Message {
	return sender.Message{
		WorkspaceID: "ws-1", MessageID: "msg-1",
		To: "person@example.com", From: (&mail.Address{Name: "Forke", Address: "hello@forke.space"}).String(),
		ReplyTo: "support@forke.space", Subject: "Héllo there", HTML: "<p>Hi</p>", Text: "Hi",
		Headers: map[string]string{"List-Unsubscribe": "<https://mail.forke.space/u/x>", "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"},
	}
}

func TestSendDeliversThroughRelay(t *testing.T) {
	f := startRelay(t, 250)
	id, err := Sender{Config: relayConfig(t, f)}.Send(context.Background(), message())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(id, "@forke.space") {
		t.Fatalf("message id %q", id)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.from != "MAIL FROM:<hello@forke.space>" || f.to != "RCPT TO:<person@example.com>" || f.auth == "" {
		t.Fatalf("envelope %q %q auth %q", f.from, f.to, f.auth)
	}
	msg, err := mail.ReadMessage(strings.NewReader(f.data))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Get("Message-Id") != "<"+id+">" || msg.Header.Get("X-Forke-Message-Id") != "msg-1" || msg.Header.Get("List-Unsubscribe-Post") == "" {
		t.Fatalf("headers %v", msg.Header)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if subject != "Héllo there" {
		t.Fatalf("subject %q", subject)
	}
	if !strings.HasPrefix(msg.Header.Get("Content-Type"), "multipart/alternative") {
		t.Fatalf("content type %q", msg.Header.Get("Content-Type"))
	}
}

func TestSendClassifiesRejections(t *testing.T) {
	cases := map[int]sender.Action{550: sender.Fail, 451: sender.Retry}
	for code, want := range cases {
		f := startRelay(t, code)
		_, err := Sender{Config: relayConfig(t, f)}.Send(context.Background(), message())
		var se *sender.SendError
		if !errors.As(err, &se) || se.Action != want {
			t.Fatalf("code %d: %v", code, err)
		}
	}
}

func TestSendRetriesWhenRelayIsDown(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	host, port, _ := net.SplitHostPort(addr)
	p, _ := strconv.Atoi(port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Sender{Config: Config{Host: host, Port: p, TLS: TLSNone}}.Send(ctx, message())
	var se *sender.SendError
	if !errors.As(err, &se) || se.Action != sender.Retry {
		t.Fatalf("got %v", err)
	}
}

func TestBuildRejectsHeaderInjection(t *testing.T) {
	m := message()
	m.ReplyTo = "a@b.com\r\nBcc: victim@example.com"
	if _, err := Build(m, "id@forke.space", time.Now()); !errors.Is(err, errHeaderInjection) {
		t.Fatalf("got %v", err)
	}
}

func TestClassifyAuthFailurePausesSending(t *testing.T) {
	if got := classify(&textproto.Error{Code: 535, Msg: "bad creds"}); got.Action != sender.PauseSending {
		t.Fatalf("got %v", got.Action)
	}
	if got := unknownOutcome(io.ErrUnexpectedEOF); got.Action != sender.UnknownOutcome {
		t.Fatalf("got %v", got.Action)
	}
}

func TestQuotaLimits(t *testing.T) {
	q := Quota{SendRate: 2, DailyLimit: 100, MonthlyLimit: 3000}
	if l := q.Limits(Usage{Last24Hours: 40, ThisMonth: 500}); l.Max24HourSend != 100 || l.SentLast24Hours != 40 {
		t.Fatalf("daily bound %+v", l)
	}
	if l := q.Limits(Usage{Last24Hours: 40, ThisMonth: 2990}); l.Max24HourSend != 50 {
		t.Fatalf("monthly bound %+v", l)
	}
	if l := q.Limits(Usage{Last24Hours: 0, ThisMonth: 3000}); l.SentLast24Hours < l.Max24HourSend {
		t.Fatalf("exhausted month must block %+v", l)
	}
	if l := (Quota{SendRate: 2}).Limits(Usage{Last24Hours: 999}); l.Max24HourSend != 0 {
		t.Fatalf("unlimited %+v", l)
	}
}

func TestIdentitiesAllowList(t *testing.T) {
	ids := Identities{Allowed: []string{"forke.space", "ops@other.dev"}}
	for name, want := range map[string]bool{"forke.space": true, "Blog@Forke.space": true, "ops@other.dev": true, "other.dev": false, "evil.com": false} {
		got, _ := ids.Get(context.Background(), name)
		if got.VerifiedForSending != want {
			t.Fatalf("%s: %v", name, got.VerifiedForSending)
		}
	}
}
