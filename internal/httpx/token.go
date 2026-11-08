package httpx

import (
	"crypto/sha256"
	"crypto/subtle"
)

func TokenMatches(got, want string) bool {
	a := sha256.Sum256([]byte(got))
	b := sha256.Sum256([]byte(want))
	return want != "" && subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
