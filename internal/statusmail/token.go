// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package statusmail

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	b64             = base64.RawURLEncoding
	errInvalidToken = errors.New("invalid_token")
)

const (
	confirmKind     = "confirm"
	unsubscribeKind = "unsubscribe"
	confirmLifetime = 48 * time.Hour
)

type signer []byte

func (s signer) mac(payload string) []byte {
	key := hmac.New(sha256.New, s)
	key.Write([]byte("forke-status-subscriptions"))
	m := hmac.New(sha256.New, key.Sum(nil))
	m.Write([]byte(payload))
	return m.Sum(nil)
}

func (s signer) sign(kind, email string, expires time.Time) string {
	exp := int64(0)
	if !expires.IsZero() {
		exp = expires.Unix()
	}
	payload := kind + "\n" + email + "\n" + strconv.FormatInt(exp, 10)
	return b64.EncodeToString([]byte(payload)) + "." + b64.EncodeToString(s.mac(payload))
}

func (s signer) verify(token, kind string, now time.Time) (string, error) {
	encPayload, encSig, ok := strings.Cut(token, ".")
	if !ok {
		return "", errInvalidToken
	}
	payload, err := b64.DecodeString(encPayload)
	if err != nil {
		return "", errInvalidToken
	}
	sig, err := b64.DecodeString(encSig)
	if err != nil || !hmac.Equal(sig, s.mac(string(payload))) {
		return "", errInvalidToken
	}
	parts := strings.Split(string(payload), "\n")
	if len(parts) != 3 || parts[0] != kind {
		return "", errInvalidToken
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || (exp != 0 && now.Unix() > exp) {
		return "", errInvalidToken
	}
	return parts[1], nil
}
