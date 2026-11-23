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

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerAgentRoutes(r chi.Router, cfg config.Config, pool *pgxpool.Pool, authn *auth.Authenticator, api http.Handler, keyLimit, publicLimit func(http.Handler) http.Handler) {
	oa := &oauth.Server{Pool: pool, Auth: authn, BaseURL: cfg.PublicBaseURL}
	m := &mcp.Handler{Pool: pool, Auth: authn, API: api, Limit: keyLimit, BaseURL: cfg.PublicBaseURL, Secret: []byte(cfg.UnsubscribeSecret)}
	r.Group(func(pub chi.Router) {
		pub.Use(publicLimit)
		for _, p := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
			pub.Get(p, oa.ResourceMetadata)
			pub.Options(p, oa.Preflight)
		}
		pub.Get("/.well-known/oauth-authorization-server", oa.ServerMetadata)
		pub.Options("/.well-known/oauth-authorization-server", oa.Preflight)
		pub.Post("/oauth/register", oa.Register)
		pub.Options("/oauth/register", oa.Preflight)
		pub.Get("/oauth/authorize", oa.Authorize)
		pub.Post("/oauth/authorize", oa.Decide)
		pub.Post("/oauth/token", oa.Token)
		pub.Options("/oauth/token", oa.Preflight)
		pub.Post("/oauth/revoke", oa.Revoke)
		pub.Options("/oauth/revoke", oa.Preflight)
		pub.Get("/llms.txt", m.LLMs)
		pub.Get("/install.sh", m.Install)
		pub.Get("/robots.txt", m.Robots)
		pub.Get("/sitemap.xml", m.Sitemap)
		pub.Get("/.well-known/mcp.json", m.ServerCard)
		pub.Get("/docs/agents", m.AgentsDocs)
		pub.Get("/docs/agents/", m.AgentsDocs)
		pub.Get("/approve/{id}", m.ShowApproval)
		pub.Post("/approve/{id}", m.DecideApproval)
	})
	r.Handle("/mcp", m)
}
