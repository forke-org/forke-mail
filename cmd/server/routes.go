package main

import (
	"forkemail/internal/access"
	"forkemail/internal/alerts"
	"forkemail/internal/deliveries"
	"forkemail/internal/segments"
	"forkemail/internal/team"
	"forkemail/internal/templates"
	"forkemail/internal/track"
	"forkemail/internal/workflows"
	"net/http"
	"strings"

	"forkemail/frontend"
	"forkemail/internal/accounts"
	"forkemail/internal/auth"
	"forkemail/internal/campaigns"
	"forkemail/internal/compatapi"
	"forkemail/internal/config"
	"forkemail/internal/contacts"
	"forkemail/internal/db"
	"forkemail/internal/domains"
	"forkemail/internal/emails"
	"forkemail/internal/events"
	"forkemail/internal/feedback"
	"forkemail/internal/forke"
	"forkemail/internal/httpx"
	"forkemail/internal/keys"
	"forkemail/internal/lifecycle"
	"forkemail/internal/ratelimit"
	"forkemail/internal/seed"
	"forkemail/internal/sns"
	"forkemail/internal/tracking"
	"forkemail/internal/transactional"
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
	dispatcher  *transactional.Dispatcher
	developer   *developerServices
	identities  domains.Identities
	feedback    *feedback.Processor
	verifier    *sns.Verifier
	sendingMode func() string
	sendRate    func() float64
	admins      *forke.Directory
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
	if svc.admins != nil {
		authn.Admins = svc.admins
	}
	account := &accounts.Handler{Pool: pool, Admins: svc.admins, WorkspaceName: cfg.ForkeWorkspace, SecureCookies: secureCookies, Failures: failures}
	me := &workspaces.Handler{Pool: pool, APILimit: apiRule, GlobalSendRate: svc.sendRate, TrialSendRate: cfg.TrialSendRatePerSec}
	keyH := &keys.Handler{Pool: pool, Passwords: accounts.Passwords{Pool: pool, Admins: svc.admins}}
	contactH := &contacts.Handler{Pool: pool}
	campaignH := &campaigns.Handler{Pool: pool}
	emailH := &emails.Handler{Pool: pool}
	eventH := &events.Handler{Pool: pool, SendingMode: svc.sendingMode}
	domainH := &domains.Handler{Pool: pool, Identities: svc.identities}
	dnsH := &domains.DNSHandler{Pool: pool, Region: cfg.AWS.Region}
	transactionalH := &transactional.Handler{Pool: pool, Dispatcher: svc.dispatcher}
	trackH := &tracking.Handler{Pool: pool, Tracker: tracking.NewTracker([]byte(cfg.UnsubscribeSecret), cfg.PublicBaseURL)}
	compatH := &compatapi.Handler{Pool: pool, Transactional: transactionalH, Emails: emailH, Tags: contactH.CreateTag}
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
		lifecycleH := &lifecycle.Handler{Pool: pool}
		api.Get("/api/settings/reengagement", lifecycleH.Get)
		api.Put("/api/settings/reengagement", lifecycleH.Put)
		alertsH := &alerts.Handler{Pool: pool, WarnBounceRate: cfg.WarnBounceRate, WarnComplaintRate: cfg.WarnComplaintRate, MaxBounceRate: cfg.MaxBounceRate, MaxComplaintRate: cfg.MaxComplaintRate}
		api.Get("/api/settings/alerts", alertsH.Get)
		api.Get("/api/deliveries", (&deliveries.Handler{Pool: pool}).List)
		teamH := &team.Handler{Pool: pool, WorkspaceID: cfg.AlertWorkspaceID, From: cfg.InviteFrom, AppURL: cfg.PublicBaseURL}
		if svc.dispatcher != nil {
			teamH.Mail = transactional.Service{Pool: pool, Dispatcher: svc.dispatcher}
		}
		projectsH := &team.Projects{Pool: pool, MaxProjects: cfg.MaxProjects, Defaults: accounts.NewWorkspace{Plan: "trial", DailyLimit: cfg.BetaDailyLimit, RatePerSec: 5, TrialSendLimit: cfg.ProjectSendLimit}}
		api.Get("/api/projects", projectsH.List)
		api.Post("/api/projects", projectsH.Create)
		api.Post("/api/projects/switch", projectsH.Switch)
		manageH := &team.Manage{Pool: pool, Identities: svc.identities}
		api.Patch("/api/projects/current", manageH.Rename)
		api.Post("/api/projects/current/reset", manageH.Reset)
		api.Post("/api/projects/current/delete", manageH.Delete)
		api.Post("/api/projects/current/leave", manageH.Leave)
		api.Patch("/api/team/members/{id}", manageH.SetRole)
		accountH := &team.Account{Manage: team.Manage{Pool: pool, Identities: svc.identities}, SecureCookies: secureCookies}
		api.Get("/api/account", accountH.Get)
		api.Patch("/api/account", accountH.Update)
		api.Post("/api/account/password", accountH.Password)
		api.Post("/api/account/delete", accountH.Delete)
		templatesH := &templates.Handler{Pool: pool}
		api.Get("/api/templates", templatesH.List)
		api.Post("/api/templates", templatesH.Create)
		api.Get("/api/templates/{id}", templatesH.Get)
		api.Put("/api/templates/{id}", templatesH.Update)
		api.Delete("/api/templates/{id}", templatesH.Delete)
		segmentsH := &segments.Handler{Pool: pool}
		api.Get("/api/segments", segmentsH.List)
		api.Post("/api/segments", segmentsH.Create)
		api.Post("/api/segments/preview", segmentsH.Preview)
		api.Get("/api/segments/{id}", segmentsH.Get)
		api.Put("/api/segments/{id}", segmentsH.Update)
		api.Delete("/api/segments/{id}", segmentsH.Delete)
		api.Get("/api/segments/{id}/contacts", segmentsH.Contacts)
		trackH := &track.Handler{Pool: pool}
		api.Post("/api/events", trackH.Track)
		api.Get("/api/contact-events", trackH.List)
		api.Get("/api/data", trackH.Data)
		api.Delete("/api/data/fields/{name}", trackH.DeleteField)
		api.Delete("/api/data/events/{name}", trackH.DeleteEvent)
		workflowsH := &workflows.Handler{Pool: pool}
		api.Get("/api/workflows", workflowsH.List)
		api.Post("/api/workflows", workflowsH.Create)
		api.Get("/api/workflows/{id}", workflowsH.Get)
		api.Put("/api/workflows/{id}", workflowsH.Update)
		api.Delete("/api/workflows/{id}", workflowsH.Delete)
		api.Get("/api/workflows/{id}/runs", workflowsH.Runs)
		api.Get("/api/team", teamH.List)
		api.Post("/api/team/invites", teamH.InviteMember)
		api.Delete("/api/team/invites/{id}", teamH.RevokeInvite)
		api.Delete("/api/team/members/{id}", teamH.RemoveMember)
		api.Put("/api/settings/alerts", alertsH.Put)
		api.Get("/api/topics", contactH.Topics)
		api.Put("/api/topics/{key}", contactH.PutTopic)

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
		api.Get("/api/domains/{id}/dns", dnsH.Check)
		api.Delete("/api/domains/{id}", domainH.Delete)

		api.Get("/api/suppressions", contactH.ListSuppressions)
		api.Post("/api/suppressions", contactH.CreateSuppression)
		api.Post("/api/suppressions/import", contactH.ImportSuppressions)
		api.Delete("/api/suppressions/{id}", contactH.DeleteSuppression)
		registerDeveloperRoutes(api, pool, svc.developer, svc.dispatcher)
		api.Post("/api/transactional", transactionalH.Send)
		api.Get("/api/transactional", transactionalH.List)
		api.Get("/api/transactional/stats", transactionalH.Stats)
		api.Get("/api/transactional/{id}", transactionalH.Get)

		api.Post("/api/forke-mail/emails", compatH.SendEmail)
		api.Get("/api/forke-mail/emails/{id}", compatH.GetEmail)
		api.Get("/api/forke-mail/audiences", compatH.ListAudiences)
		api.Post("/api/forke-mail/audiences", compatH.CreateAudience)
		api.Post("/api/forke-mail/audiences/{id}/contacts", compatH.AddContact)
		api.Delete("/api/forke-mail/audiences/{id}/contacts/{contact}", compatH.RemoveContact)
		api.Post("/api/forke-mail/broadcasts", compatH.CreateBroadcast)
		api.Post("/api/forke-mail/broadcasts/{id}/send", compatH.SendBroadcast)
	})

	r.Get("/healthz", db.Health(pool))
	r.Get("/_next/static/*", frontend.Handler().ServeHTTP)
	r.Head("/_next/static/*", frontend.Handler().ServeHTTP)

	if svc.verifier != nil {
		r.Method(http.MethodPost, "/webhooks/ses", &feedback.Webhook{Verifier: svc.verifier, Processor: svc.feedback})
	}

	r.Group(func(public chi.Router) {
		public.Use(publicLimit)
		public.Post("/api/auth/login", account.Login)
		accessH := &access.Public{Pool: pool, SecureCookies: secureCookies, ReservedName: cfg.ForkeWorkspace, Workspace: access.WorkspaceDefaults{SendLimit: cfg.BetaSendLimit, DailyLimit: cfg.BetaDailyLimit, RatePerSec: 5}}
		public.Post("/api/access/request", accessH.Request)
		public.Get("/api/access/invite", accessH.Invite)
		public.Post("/api/access/invite", accessH.Accept)
		joinH := &team.Join{Pool: pool, SecureCookies: secureCookies}
		public.Get("/api/team/join", joinH.Lookup)
		public.Post("/api/team/join", joinH.Accept)
		if cfg.PlatformToken != "" {
			platform := &access.Platform{Pool: pool, Token: cfg.PlatformToken, WorkspaceID: cfg.AlertWorkspaceID, From: cfg.InviteFrom, AppURL: cfg.PublicBaseURL, Identities: svc.identities}
			if svc.dispatcher != nil {
				platform.Mail = transactional.Service{Pool: pool, Dispatcher: svc.dispatcher}
			}
			platform.Routes(public)
		}
		public.Get("/unsubscribe/{token}", unsubH.Confirm)
		public.Post("/unsubscribe/{token}", unsubH.Unsubscribe)
		public.Post("/preferences/{token}", unsubH.Update)
		public.Get("/t/o/{token}", trackH.Open)
		public.Get("/t/c/{token}", trackH.Click)
		public.Method(http.MethodGet, "/api/public/status", &events.PublicStatus{Pool: pool, MaxBounceRate: cfg.MaxBounceRate, MaxComplaintRate: cfg.MaxComplaintRate})
		public.Get("/status", redirectTo(cfg.StatusPageURL+"/#mail"))
		public.Get("/status/", redirectTo(cfg.StatusPageURL+"/#mail"))
		if cfg.Development() {
			seedH := &seed.Handler{Pool: pool, Token: cfg.SeedToken, Failures: failures, SES: cfg.SES(), SESTestEmail: cfg.SESTestEmail}
			public.Post("/dev/seed", seedH.Seed)
			public.Method(http.MethodPost, "/dev/ses-event", &feedback.DevHandler{Processor: svc.feedback, Token: cfg.SeedToken, Failures: failures})
		}
		public.Handle("/*", frontend.Handler())
	})
	return r
}

func redirectTo(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusFound)
	}
}
