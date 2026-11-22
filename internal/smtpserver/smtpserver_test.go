// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package smtpserver

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"forkemail/internal/auth"
	"forkemail/internal/transactional"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

func TestParse(t *testing.T) {
	raw := "From: =?utf-8?q?Ada_L=C3=B6velace?= <ada@example.com>\r\nReply-To: help@example.com\r\nSubject: =?utf-8?b?SGVsbG8g8J+Riw==?=\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nHi =E2=80=94 there\r\n--b\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\nPHA+SGk8L3A+\r\n--b--\r\n"
	m, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if m.FromName != "Ada Lövelace" || m.FromAddress != "ada@example.com" || m.ReplyTo != "help@example.com" || m.Subject != "Hello 👋" || m.Text != "Hi — there" || m.HTML != "<p>Hi</p>" {
		t.Fatalf("%+v", m)
	}
	attachment := "From: a@example.com\r\nContent-Type: multipart/mixed; boundary=x\r\n\r\n--x\r\nContent-Type: text/plain\r\n\r\nhi\r\n--x\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=a.pdf\r\n\r\nPDF\r\n--x--\r\n"
	if _, err := Parse([]byte(attachment)); !errors.Is(err, errAttachments) {
		t.Fatalf("attachment err = %v", err)
	}
	if _, err := Parse([]byte("From: a@example.com\r\n\r\n   \r\n")); !errors.Is(err, errNoBody) {
		t.Fatalf("empty err = %v", err)
	}
}

type fakeKeys struct{}

func (fakeKeys) IdentifyKey(_ context.Context, plain string) (auth.Identity, error) {
	if plain == "fm_live_good" {
		return auth.Identity{WorkspaceID: "ws", Scope: auth.SendingAccess}, nil
	}
	return auth.Identity{}, errors.New("unauthorized")
}

type fakeMail struct{ got []transactional.Submission }

func (f *fakeMail) Accept(_ context.Context, who auth.Identity, _ string, req transactional.Submission) (transactional.Acceptance, error) {
	if who.WorkspaceID != "ws" {
		return transactional.Acceptance{}, errors.New("wrong workspace")
	}
	if strings.HasSuffix(req.FromAddress, "@unverified.test") {
		return transactional.Acceptance{}, errors.New("from_domain_not_verified")
	}
	f.got = append(f.got, req)
	return transactional.Acceptance{ID: "x"}, nil
}

func selfSigned(t *testing.T) tls.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func TestSubmission(t *testing.T) {
	mail := &fakeMail{}
	s := smtp.NewServer(&Backend{Keys: fakeKeys{}, Mail: mail})
	s.Domain = "localhost"
	s.TLSConfig = &tls.Config{Certificates: []tls.Certificate{selfSigned(t)}}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve(l)
	t.Cleanup(func() { s.Close() })

	plain, err := smtp.Dial(l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := plain.Auth(sasl.NewPlainClient("", Username, "fm_live_good")); err == nil {
		t.Fatal("login allowed without TLS")
	}
	plain.Close()

	dial := func() *smtp.Client {
		c, err := smtp.DialStartTLS(l.Addr().String(), &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	bad := dial()
	if err := bad.Auth(sasl.NewPlainClient("", Username, "fm_live_wrong")); err == nil {
		t.Fatal("bad key accepted")
	}
	bad.Close()

	c := dial()
	defer c.Close()
	if err := c.Auth(sasl.NewPlainClient("", Username, "fm_live_good")); err != nil {
		t.Fatal(err)
	}
	msg := "From: App <hi@verified.test>\r\nSubject: Code\r\nContent-Type: text/plain\r\n\r\nYour code is 123\r\n"
	if err := c.SendMail("hi@verified.test", []string{"a@example.com", "b@example.com"}, strings.NewReader(msg)); err != nil {
		t.Fatal(err)
	}
	if len(mail.got) != 2 || mail.got[0].Subject != "Code" || mail.got[1].To != "b@example.com" || mail.got[0].FromName != "App" {
		t.Fatalf("%+v", mail.got)
	}
	err = c.SendMail("x@unverified.test", []string{"a@example.com"}, strings.NewReader("From: x@unverified.test\r\nSubject: S\r\n\r\nhi\r\n"))
	var smtpErr *smtp.SMTPError
	if !errors.As(err, &smtpErr) || smtpErr.Code != 550 {
		t.Fatalf("unverified sender err = %v", err)
	}
}
