package main

import (
	"context"
	"forkemail/internal/domains"
	"forkemail/internal/feedback"
	"forkemail/internal/ratelimit"
	"forkemail/internal/sender"
	"forkemail/internal/transactional"
	"forkemail/internal/unsub"
	"forkemail/internal/webhooks"
	"sync"
	"testing"
	"time"
)

func newIntegratedFixture(t *testing.T) (apiFixture, *webhooks.Vault) {
	t.Helper()
	f := newAPIFixture(t)
	v, err := webhooks.NewVault("a2tra2tra2tra2tra2tra2tra2tra2tra2tra2tra2s=")
	if err != nil {
		t.Fatal(err)
	}
	p := &feedback.Processor{Pool: f.pool}
	mailer := sender.LogSender{Pool: f.pool, Feedback: p}
	signer := unsub.Signer{Secret: []byte("local-test-signing-secret-32-characters")}
	w := sender.NewWorker(f.pool, mailer, signer, "http://localhost:8080", 100, 10)
	f.worker = w
	d := transactional.NewDispatcher(f.pool, mailer, w, transactional.NewTracker([]byte("local-test-signing-secret-32-characters"), "http://localhost:8080"))
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Go(func() { w.Run(ctx) })
	wg.Go(func() { d.Run(ctx) })
	t.Cleanup(func() { cancel(); wg.Wait() })
	lim := limiters{public: ratelimit.NewStore(), keys: ratelimit.NewStore(), failures: ratelimit.NewStore()}
	cfg := testConfig("development")
	cfg.PublicBaseURL = "http://localhost:8080"
	f.router = newRouter(cfg, f.pool, lim, services{dispatcher: d, developer: &developerServices{hooks: &webhooks.Handler{Pool: f.pool, Vault: v, Network: webhooks.Network{Resolver: publicDNS{}}}}, feedback: p, identities: domains.Local{}, sendRate: w.GlobalRate})
	return f, v
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not reached before deadline")
}
