package ratelimit

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"forkemail/internal/httpx"
)

func Middleware(s *Store, keyFn func(*http.Request) (string, Rule)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, rule := keyFn(r)
			ok, remaining, retryAfter := s.Allow(key, rule)
			w.Header().Set("X-RateLimit-Limit", strconv.FormatFloat(rule.PerSecond, 'f', -1, 64))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			if !ok {
				TooMany(w, retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func TooMany(w http.ResponseWriter, retryAfter time.Duration) {
	seconds := max(int(math.Ceil(retryAfter.Seconds())), 1)
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	httpx.JSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited", "retry_after": seconds})
}
