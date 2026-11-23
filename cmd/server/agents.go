// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package main

import (
	"net/http"

	"forkemail/internal/auth"
	"forkemail/internal/config"
	"forkemail/internal/mcp"
	"forkemail/internal/oauth"
	"forkemail/internal/transactional"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerAgentRoutes(r chi.Router, cfg config.Config, pool *pgxpool.Pool, authn *auth.Authenticator, dispatcher *transactional.Dispatcher, api http.Handler, keyLimit, agentLimit, publicLimit func(http.Handler) http.Handler) {
	oa := &oauth.Server{Pool: pool, Auth: authn, BaseURL: cfg.PublicBaseURL}
	m := &mcp.Handler{Pool: pool, Auth: authn, API: api, Limit: keyLimit, BaseURL: cfg.PublicBaseURL, Secret: []byte(cfg.UnsubscribeSecret),
		MailWorkspace: cfg.AlertWorkspaceID, MailFrom: cfg.AlertFrom}
	if dispatcher != nil {
		m.Mail = transactional.Service{Pool: pool, Dispatcher: dispatcher}
	}
	r.Group(func(machine chi.Router) {
		machine.Use(agentLimit)
		for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-authorization-server"} {
			machine.Options(p, oa.Preflight)
		}
		machine.Get("/.well-known/oauth-protected-resource", oa.ResourceMetadata)
		machine.Get("/.well-known/oauth-protected-resource/mcp", oa.ResourceMetadata)
		machine.Get("/.well-known/oauth-authorization-server", oa.ServerMetadata)
		machine.Get("/.well-known/mcp.json", m.ServerCard)
		for _, p := range []string{"/oauth/register", "/oauth/token", "/oauth/revoke"} {
			machine.Options(p, oa.Preflight)
		}
		machine.Post("/oauth/register", oa.Register)
		machine.Post("/oauth/token", oa.Token)
		machine.Post("/oauth/revoke", oa.Revoke)
	})
	r.Group(func(pub chi.Router) {
		pub.Use(publicLimit)
		pub.Get("/llms.txt", m.LLMs)
		pub.Get("/install.sh", m.Install)
		pub.Get("/robots.txt", m.Robots)
		pub.Get("/sitemap.xml", m.Sitemap)
		pub.Get("/docs/agents", m.AgentsDocs)
		pub.Get("/docs/agents/", m.AgentsDocs)
		pub.Get("/oauth/authorize", oa.Authorize)
		pub.Post("/oauth/authorize", oa.Decide)
		pub.Get("/approve/{id}", m.ShowApproval)
		pub.Post("/approve/{id}", m.DecideApproval)
	})
	r.Handle("/mcp", m)
}
