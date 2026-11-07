package main

import (
	"net/http"
	"strings"

	"forkemail/frontend"
	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/campaigns"
	"forkemail/internal/config"
	"forkemail/internal/contacts"
	"forkemail/internal/db"
	"forkemail/internal/domains"
	"forkemail/internal/emails"
	"forkemail/internal/events"
	"forkemail/internal/feedback"
	"forkemail/internal/httpx"
	"forkemail/internal/keys"
	"forkemail/internal/ratelimit"
	"forkemail/internal/seed"
	"forkemail/internal/sns"
	"forkemail/internal/unsub"
	"forkemail/internal/workspaces"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type limiters struct {
	public   *ratelimit.Store
	keys     *ratelimit.Store
	failures *ratelimit.Store
}

type services struct {
	identities  domains.Identities
	feedback    *feedback.Processor
	verifier    *sns.Verifier
	sendingMode func() string
	sendRate    func() float64
}

func newRouter(cfg config.Config, pool *pgxpool.Pool, lim limiters, svc services) http.Handler {
	ip := ratelimit.IPResolver{TrustedProxy: cfg.TrustedProxy}
	apiRule := func(plan string) ratelimit.Rule {
		if plan == "paid" {
			return ratelimit.Rule{PerSecond: cfg.PaidRatePerSec, Burst: cfg.PaidBurst}
		}
		return ratelimit.Rule{PerSecond: cfg.TrialRatePerSec, Burst: cfg.TrialBurst}
	}
	publicLimit := ratelimit.Middleware(lim.public, func(r *http.Request) (string, ratelimit.Rule) {
		return "ip:" + ip.ClientIP(r), ratelimit.Rule{PerSecond: cfg.PublicRatePerSec, Burst: cfg.PublicBurst}
	})
	signupLimit := ratelimit.Middleware(lim.public, func(r *http.Request) (string, ratelimit.Rule) {
		return "signup:" + ip.ClientIP(r), ratelimit.Rule{PerSecond: 10.0 / 3600, Burst: 10}
	})
	keyLimit := ratelimit.Middleware(lim.keys, func(r *http.Request) (string, ratelimit.Rule) {
		id := auth.FromContext(r.Context())
		if id.KeyID == "" {
			return "user:" + id.UserID, apiRule("paid")
		}
		return "key:" + id.KeyID, apiRule(id.Plan)
	})
	failures := ratelimit.FailureGuard{
		Store:    lim.failures,
		Rule:     ratelimit.Rule{PerSecond: cfg.AuthFailuresPerMin / 60, Burst: cfg.AuthFailureBurst},
		ClientIP: ip.ClientIP,
	}
	secureCookies := strings.HasPrefix(cfg.PublicBaseURL, "https://")

	authn := &auth.Authenticator{Pool: pool, Failures: failures}
	account := &accounts.Handler{Pool: pool, SecureCookies: secureCookies, SandboxDomains: !cfg.SES(), Failures: failures}
	me := &workspaces.Handler{Pool: pool, APILimit: apiRule, GlobalSendRate: svc.sendRate, TrialSendRate: cfg.TrialSendRatePerSec}
	keyH := &keys.Handler{Pool: pool}
	contactH := &contacts.Handler{Pool: pool}
	campaignH := &campaigns.Handler{Pool: pool}
	emailH := &emails.Handler{Pool: pool}
	eventH := &events.Handler{Pool: pool, SendingMode: svc.sendingMode}
	domainH := &domains.Handler{Pool: pool, Identities: svc.identities}
	unsubH := &unsub.Handler{Pool: pool, Signer: unsub.Signer{Secret: []byte(cfg.UnsubscribeSecret)}}

	r := chi.NewRouter()
	r.Use(httpx.Logger, httpx.Recover, httpx.SecurityHeaders, httpx.LimitBody)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) { httpx.Error(w, http.StatusNotFound, "not_found") })
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, http.StatusMethodNotAllowed, "method_not_allowed")
	})

	r.Group(func(api chi.Router) {
		api.Use(authn.Middleware, keyLimit, auth.Authorize)
		api.Get("/api/me", me.Me)
		api.Post("/api/auth/logout", account.Logout)

		api.Get("/api/keys", keyH.List)
		api.Post("/api/keys", keyH.Create)
		api.Delete("/api/keys/{id}", keyH.Revoke)
		api.Post("/api/keys/{id}/rotate", keyH.Rotate)

		api.Get("/api/dashboard", eventH.Dashboard)
		api.Get("/api/events", eventH.List)

		api.Get("/api/contacts", contactH.List)
		api.Post("/api/contacts/import", contactH.Import)
		api.Post("/api/contacts/tags", contactH.TagContacts)
		api.Delete("/api/contacts/{id}", contactH.Delete)
		api.Get("/api/tags", contactH.ListTags)
		api.Post("/api/tags", contactH.CreateTag)

		api.Get("/api/campaigns", campaignH.List)
		api.Post("/api/campaigns", campaignH.Create)
		api.Get("/api/campaigns/{id}", campaignH.Get)
		api.Patch("/api/campaigns/{id}", campaignH.Update)
		api.Post("/api/campaigns/{id}/emails", emailH.Create)

		api.Get("/api/emails/{id}", emailH.Get)
		api.Patch("/api/emails/{id}", emailH.Update)
		api.Delete("/api/emails/{id}", emailH.Delete)
		api.Post("/api/emails/{id}/send", emailH.Send)
		api.Post("/api/emails/{id}/pause", emailH.Pause)
		api.Post("/api/emails/{id}/resume", emailH.Resume)
		api.Get("/api/emails/{id}/report", emailH.Report)
		api.Get("/api/emails/{id}/messages", emailH.Messages)
		api.Get("/api/emails/{id}/recipients", emailH.Recipients)

		api.Get("/api/domains", domainH.List)
		api.Post("/api/domains", domainH.Create)
		api.Post("/api/domains/{id}/verify", domainH.Verify)
		api.Delete("/api/domains/{id}", domainH.Delete)

		api.Get("/api/suppressions", contactH.ListSuppressions)
		api.Post("/api/suppressions", contactH.CreateSuppression)
		api.Post("/api/suppressions/import", contactH.ImportSuppressions)
		api.Delete("/api/suppressions/{id}", contactH.DeleteSuppression)
	})

	r.Get("/healthz", db.Health(pool))

	if svc.verifier != nil {
		r.Method(http.MethodPost, "/webhooks/ses", &feedback.Webhook{Verifier: svc.verifier, Processor: svc.feedback})
	}

	r.Group(func(public chi.Router) {
		public.Use(publicLimit)
		public.With(signupLimit).Post("/api/auth/signup", account.Signup)
		public.Post("/api/auth/login", account.Login)
		public.Get("/unsubscribe/{token}", unsubH.Confirm)
		public.Post("/unsubscribe/{token}", unsubH.Unsubscribe)
		if cfg.Development() {
			seedH := &seed.Handler{Pool: pool, Token: cfg.SeedToken, Failures: failures, SES: cfg.SES(), SESTestEmail: cfg.SESTestEmail}
			public.Post("/dev/seed", seedH.Seed)
			public.Method(http.MethodPost, "/dev/ses-event", &feedback.DevHandler{Processor: svc.feedback, Token: cfg.SeedToken, Failures: failures})
		}
		public.Handle("/*", frontend.Handler())
	})
	return r
}
