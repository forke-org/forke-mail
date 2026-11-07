package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPerKeyLimiterReturns429(t *testing.T) {
	store := NewStore()
	h := Middleware(store, func(r *http.Request) (string, Rule) {
		return r.Header.Get("X-Key"), Rule{PerSecond: 1, Burst: 3}
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))

	do := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Key", key)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	for i := 0; i < 3; i++ {
		if rec := do("a"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d", i, rec.Code)
		}
	}
	rec := do("a")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	for _, h := range []string{"Retry-After", "X-RateLimit-Limit", "X-RateLimit-Remaining"} {
		if rec.Header().Get(h) == "" {
			t.Errorf("missing %s header", h)
		}
	}
	if rec := do("b"); rec.Code != http.StatusOK {
		t.Fatalf("other key limited: %d", rec.Code)
	}
}
