// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

const (
	visiblePrefixLength = 12
	ModeLive            = "live"
	ModeTest            = "test"
)

var keyPattern = regexp.MustCompile(`^fm_(live|test)_[0-9a-f]{64}$`)

type NewKey struct {
	Plain  string
	Prefix string
	Hash   string
}

func GenerateKey(mode string) NewKey {
	if mode != ModeTest {
		mode = ModeLive
	}
	plain := "fm_" + mode + "_" + RandomHex(32)
	return NewKey{Plain: plain, Prefix: plain[:visiblePrefixLength], Hash: HashKey(plain)}
}

func RandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func HashKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func validKeyFormat(plain string) bool {
	return keyPattern.MatchString(plain)
}
