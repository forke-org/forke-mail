package snstest

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"sync"
	"testing"
	"time"

	"forkemail/internal/sns"
)

const (
	Region   = "us-east-1"
	TopicARN = "arn:aws:sns:us-east-1:123456789012:forkemail-ses-events"
	CertURL  = "https://sns.us-east-1.amazonaws.com/SimpleNotificationService-test.pem"
)

type Signer struct {
	key *rsa.PrivateKey
	pem []byte

	mu       sync.Mutex
	Requests []string
}

func NewSigner(t *testing.T) *Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sns.amazonaws.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &Signer{key: key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func (s *Signer) Verifier() *sns.Verifier {
	v := sns.NewVerifier(Region, TopicARN)
	v.Client = &http.Client{Transport: s}
	return v
}

func (s *Signer) RoundTrip(r *http.Request) (*http.Response, error) {
	s.mu.Lock()
	s.Requests = append(s.Requests, r.URL.String())
	s.mu.Unlock()
	body := []byte("confirmed")
	if r.URL.String() == CertURL {
		body = s.pem
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func (s *Signer) Sign(t *testing.T, m sns.Message) sns.Message {
	t.Helper()
	if m.TopicArn == "" {
		m.TopicArn = TopicARN
	}
	if m.SigningCertURL == "" {
		m.SigningCertURL = CertURL
	}
	if m.SignatureVersion == "" {
		m.SignatureVersion = "2"
	}
	if m.Timestamp == "" {
		m.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	payload, ok := sns.StringToSign(m)
	if !ok {
		t.Fatalf("cannot sign type %q", m.Type)
	}
	var sig []byte
	var err error
	if m.SignatureVersion == "1" {
		sum := sha1.Sum([]byte(payload))
		sig, err = rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA1, sum[:])
	} else {
		sum := sha256.Sum256([]byte(payload))
		sig, err = rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, sum[:])
	}
	if err != nil {
		t.Fatal(err)
	}
	m.Signature = base64.StdEncoding.EncodeToString(sig)
	return m
}
