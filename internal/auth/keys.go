package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

const (
	keyPrefix           = "fm_live_"
	visiblePrefixLength = 12
)

var keyPattern = regexp.MustCompile(`^fm_live_[0-9a-f]{64}$`)

type NewKey struct {
	Plain  string
	Prefix string
	Hash   string
}

func GenerateKey() NewKey {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	plain := keyPrefix + hex.EncodeToString(b)
	return NewKey{Plain: plain, Prefix: plain[:visiblePrefixLength], Hash: HashKey(plain)}
}

func HashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func validKeyFormat(plain string) bool {
	return keyPattern.MatchString(plain)
}
