package webhooks

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

func Signature(secret, id, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(id + "." + timestamp + "."))
	m.Write(body)
	return "v1=" + hex.EncodeToString(m.Sum(nil))
}

func Verify(secret, id, timestamp, signature string, body []byte, now time.Time) bool {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || ts < now.Unix()-300 || ts > now.Unix()+300 || id == "" {
		return false
	}
	return hmac.Equal([]byte(Signature(secret, id, timestamp, body)), []byte(signature))
}

type Vault struct{ aead cipher.AEAD }

func NewVault(encoded string) (*Vault, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(key) != 32 {
		return nil, errors.New("WEBHOOK_ENCRYPTION_KEY must be base64 of 32 random bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

func (v *Vault) Seal(secret, workspace string) string {
	nonce := make([]byte, v.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return "enc1:" + base64.StdEncoding.EncodeToString(v.aead.Seal(nonce, nonce, []byte(secret), []byte(workspace)))
}

func (v *Vault) Open(encoded, workspace string) (string, error) {
	if !strings.HasPrefix(encoded, "enc1:") {
		return "", errors.New("signing_secret_requires_rotation")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, "enc1:"))
	if err != nil || len(data) < v.aead.NonceSize() {
		return "", errors.New("invalid_encrypted_secret")
	}
	n := v.aead.NonceSize()
	plain, err := v.aead.Open(nil, data[:n], data[n:], []byte(workspace))
	return string(plain), err
}

func newSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return "whsec_" + hex.EncodeToString(b)
}
