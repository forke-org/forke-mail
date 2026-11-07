package unsub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

var errInvalidToken = errors.New("invalid_token")

var b64 = base64.RawURLEncoding.Strict()

type Signer struct {
	Secret []byte
}

type Target struct {
	WorkspaceID string
	ContactID   string
	EmailID     string
}

func (s Signer) Sign(t Target) string {
	payload := []byte(t.WorkspaceID + ":" + t.ContactID + ":" + t.EmailID)
	return b64.EncodeToString(payload) + "." + b64.EncodeToString(s.mac(payload))
}

func (s Signer) Verify(token string) (Target, error) {
	encPayload, encSig, ok := strings.Cut(token, ".")
	if !ok {
		return Target{}, errInvalidToken
	}
	payload, err := b64.DecodeString(encPayload)
	if err != nil {
		return Target{}, errInvalidToken
	}
	sig, err := b64.DecodeString(encSig)
	if err != nil || !hmac.Equal(sig, s.mac(payload)) {
		return Target{}, errInvalidToken
	}
	parts := strings.Split(string(payload), ":")
	if len(parts) != 3 {
		return Target{}, errInvalidToken
	}
	return Target{WorkspaceID: parts[0], ContactID: parts[1], EmailID: parts[2]}, nil
}

func (s Signer) mac(payload []byte) []byte {
	m := hmac.New(sha256.New, s.Secret)
	m.Write(payload)
	return m.Sum(nil)
}
