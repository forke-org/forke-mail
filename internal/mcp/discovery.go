// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package mcp

import (
	_ "embed"
	"net/http"
	"strings"
	"time"

	"forkemail/internal/httpx"
)

//go:embed install.sh
var installScript string

var toolLevel = map[string]string{
	"send_email":      "Asks you first with a live key; runs directly with a test key",
	"delete_template": "Asks you first",
	"create_api_key":  "Asks you first",
	"send_broadcast":  "Needs your approval in the browser",
	"call_api":        "Depends on the endpoint (see Safety model)",
}

func toolsMarkdown() string {
	var b strings.Builder
	b.WriteString("| Tool | What it does | Level |\n| --- | --- | --- |\n")
	for _, t := range tools {
		level := toolLevel[t.Name]
		if level == "" {
			level = "Runs directly"
		}
		b.WriteString("| `" + t.Name + "` | " + strings.ReplaceAll(t.Description, "|", "/") + " | " + level + " |\n")
	}
	return b.String()
}

func (h *Handler) LLMs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(Guide))
}

func (h *Handler) Robots(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(`User-agent: *
Allow: /$
Allow: /login/
Allow: /llms.txt
Allow: /install.sh
Allow: /.well-known/mcp.json
Disallow: /

Sitemap: ` + h.BaseURL + `/sitemap.xml
`))
}

func (h *Handler) Sitemap(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	day := time.Now().UTC().Format("2006-01-02")
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + h.BaseURL + `/login/</loc><lastmod>` + day + `</lastmod><changefreq>monthly</changefreq><priority>0.5</priority></url>
</urlset>
`))
}

func (h *Handler) ServerCard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"name": "space.forke/mail", "title": "Forke Mail", "version": serverVersion,
		"description": "Send transactional email and broadcasts, manage domains, templates, contacts and workflows. Risky actions ask the user first.",
		"websiteUrl":  DocsURL, "documentation": h.BaseURL + "/llms.txt",
		"remotes":        []map[string]any{{"type": "streamable-http", "url": h.BaseURL + "/mcp"}},
		"authentication": map[string]any{"type": "oauth2", "metadata": h.BaseURL + "/.well-known/oauth-protected-resource"},
		"tools":          toolNames(),
	})
}

func toolNames() []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}

const DocsURL = "https://www.forke.space/docs/forke-mail"

func (h *Handler) AgentsDocs(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, DocsURL, http.StatusMovedPermanently)
}

func (h *Handler) Install(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(installScript))
}
