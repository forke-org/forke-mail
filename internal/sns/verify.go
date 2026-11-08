package sns

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid sns message")

const maxCertBytes = 64 << 10

type Verifier struct {
	Region   string
	TopicARN string
	Client   *http.Client

	mu    sync.Mutex
	certs map[string]*x509.Certificate
}

func NewVerifier(region, topicARN string) *Verifier {
	return &Verifier{
		Region:   region,
		TopicARN: topicARN,
		Client: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (v *Verifier) Verify(ctx context.Context, m Message) error {
	if m.TopicArn == "" || m.TopicArn != v.TopicARN {
		return fmt.Errorf("%w: topic", ErrInvalid)
	}
	certURL, ok := v.TrustedURL(m.SigningCertURL)
	if !ok || !strings.HasSuffix(certURL.Path, ".pem") {
		return fmt.Errorf("%w: signing cert url", ErrInvalid)
	}
	payload, ok := StringToSign(m)
	if !ok {
		return fmt.Errorf("%w: type", ErrInvalid)
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return fmt.Errorf("%w: signature encoding", ErrInvalid)
	}
	cert, err := v.cert(ctx, certURL.String())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("%w: key type", ErrInvalid)
	}
	switch m.SignatureVersion {
	case "1":
		sum := sha1.Sum([]byte(payload))
		err = rsa.VerifyPKCS1v15(pub, crypto.SHA1, sum[:], sig)
	case "2":
		sum := sha256.Sum256([]byte(payload))
		err = rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
	default:
		err = errors.New("signature version")
	}
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return nil
}

func (v *Verifier) TrustedURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host != "sns."+v.Region+".amazonaws.com" {
		return nil, false
	}
	return u, true
}

func (v *Verifier) cert(ctx context.Context, certURL string) (*x509.Certificate, error) {
	v.mu.Lock()
	cached := v.certs[certURL]
	v.mu.Unlock()
	if cached != nil && time.Now().Before(cached.NotAfter) {
		return cached, nil
	}
	body, err := v.get(ctx, certURL, maxCertBytes)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(body)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("certificate pem")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	if now := time.Now(); now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return nil, errors.New("certificate expired")
	}
	v.mu.Lock()
	if v.certs == nil {
		v.certs = make(map[string]*x509.Certificate)
	}
	v.certs[certURL] = cert
	v.mu.Unlock()
	return cert, nil
}

func (v *Verifier) get(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := v.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}
