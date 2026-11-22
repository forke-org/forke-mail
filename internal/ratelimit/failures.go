// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package ratelimit

import "net/http"

type FailureGuard struct {
	Store    *Store
	Rule     Rule
	ClientIP func(*http.Request) string
}

func (g FailureGuard) Reject(w http.ResponseWriter, r *http.Request) bool {
	key := "failures:" + g.ClientIP(r)
	if !g.Store.Blocked(key, g.Rule) {
		return false
	}
	_, _, retryAfter := g.Store.Allow(key, g.Rule)
	TooMany(w, retryAfter)
	return true
}

func (g FailureGuard) Fail(r *http.Request) {
	g.Store.Allow("failures:"+g.ClientIP(r), g.Rule)
}
