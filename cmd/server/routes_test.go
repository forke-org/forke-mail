package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forkemail/internal/config"
	"forkemail/internal/ratelimit"
	"forkemail/internal/sns"
)

func testConfig(env string) config.Config {
	return config.Config{
		Env: env, UnsubscribeSecret: strings.Repeat("u", 32), SeedToken: strings.Repeat("s", 32),
		PublicRatePerSec: 100, PublicBurst: 100, AuthFailuresPerMin: 60, AuthFailureBurst: 10,
		PaidRatePerSec: 20, PaidBurst: 40, TrialRatePerSec: 2, TrialBurst: 5,
	}
}

func devStatus(env, path string) int {
	lim := limiters{public: ratelimit.NewStore(), keys: ratelimit.NewStore(), failures: ratelimit.NewStore()}
	router := newRouter(testConfig(env), nil, lim, services{})
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
	req.Header.Set("X-Seed-Token", "wrong")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

func TestDevRoutesAbsentOutsideDevelopment(t *testing.T) {
	for _, path := range []string{"/dev/seed", "/dev/ses-event"} {
		for _, env := range []string{"production", "staging", ""} {
			if code := devStatus(env, path); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
				t.Errorf("APP_ENV=%q: %s returned %d", env, path, code)
			}
		}
		if code := devStatus("development", path); code != http.StatusUnauthorized {
			t.Errorf("development with wrong token on %s returned %d, want 401", path, code)
		}
	}
}

func TestWebhookRejectsForeignTopic(t *testing.T) {
	lim := limiters{public: ratelimit.NewStore(), keys: ratelimit.NewStore(), failures: ratelimit.NewStore()}
	req := httptest.NewRequest(http.MethodPost, "/webhooks/ses", strings.NewReader(`{"Type":"Notification","TopicArn":"arn:aws:sns:us-east-1:123456789012:other"}`))
	rec := httptest.NewRecorder()
	newRouter(testConfig("production"), nil, lim, services{verifier: sns.NewVerifier("us-east-1", "arn:aws:sns:us-east-1:123456789012:events")}).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("forged webhook returned %d, want 403", rec.Code)
	}
}

func TestStaticChunksDoNotConsumePublicEndpointBudget(t *testing.T) {
	cfg := testConfig("production")
	cfg.PublicBurst = 1
	cfg.PublicRatePerSec = 0.00001
	lim := limiters{public: ratelimit.NewStore(), keys: ratelimit.NewStore(), failures: ratelimit.NewStore()}
	router := newRouter(cfg, nil, lim, services{})
	get := func(path string) int {
		out := httptest.NewRecorder()
		router.ServeHTTP(out, httptest.NewRequest("GET", path, nil))
		return out.Code
	}
	for range 40 {
		if code := get("/_next/static/not-a-real-chunk.js"); code != 404 {
			t.Fatalf("static chunk consumed rate limit: %d", code)
		}
	}
	if code := get("/t/o/invalid"); code != 200 {
		t.Fatalf("static assets exhausted public budget: %d", code)
	}
	if code := get("/t/o/invalid"); code != 429 {
		t.Fatalf("public route lost its rate limit: %d", code)
	}
}
