// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package mcp

import (
	_ "embed"
	"encoding/json"
	"html"
	"html/template"
	"net/http"
	"regexp"
	"strings"
	"time"

	"forkemail/internal/httpx"
)

//go:embed docs.html
var docsHTML string

var docsPage = template.Must(template.New("docs").Parse(docsHTML))

var (
	inlineCode = regexp.MustCompile("`([^`]+)`")
	inlineLink = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)\s]+)\)`)
	bareLink   = regexp.MustCompile(`(^|[\s(])(https://[^\s<)]+[^\s<).,])`)
	bold       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	slugChars  = regexp.MustCompile(`[^a-z0-9]+`)
)

func inline(s string) string {
	s = html.EscapeString(s)
	var codes []string
	s = inlineCode.ReplaceAllStringFunc(s, func(m string) string {
		codes = append(codes, "<code>"+m[1:len(m)-1]+"</code>")
		return "\x00" + string(rune('A'+len(codes)-1)) + "\x00"
	})
	s = inlineLink.ReplaceAllString(s, `<a href="$2">$1</a>`)
	if !strings.Contains(s, "<a ") {
		s = bareLink.ReplaceAllString(s, `$1<a href="$2">$2</a>`)
	}
	s = bold.ReplaceAllString(s, "<strong>$1</strong>")
	for i, c := range codes {
		s = strings.Replace(s, "\x00"+string(rune('A'+i))+"\x00", c, 1)
	}
	return s
}

type heading struct {
	ID, Text string
	Sub      bool
}

func slug(text string) string {
	return strings.Trim(slugChars.ReplaceAllString(strings.ToLower(text), "-"), "-")
}

func tableRow(ln string) []string {
	cells := strings.Split(strings.Trim(strings.TrimSpace(ln), "|"), "|")
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
	}
	return cells
}

func renderMarkdown(md string) (template.HTML, []heading, string, string) {
	var b strings.Builder
	var toc []heading
	var title, lead string
	inList, inCode, inTable := false, false, false
	closeBlocks := func() {
		if inList {
			b.WriteString("</ul>\n")
			inList = false
		}
		if inTable {
			b.WriteString("</tbody></table></div>\n")
			inTable = false
		}
	}
	for _, ln := range strings.Split(md, "\n") {
		switch {
		case strings.HasPrefix(ln, "```"):
			closeBlocks()
			if inCode {
				b.WriteString("</code></pre>\n")
			} else {
				b.WriteString(`<pre data-lang="` + html.EscapeString(strings.TrimPrefix(ln, "```")) + `"><code>`)
			}
			inCode = !inCode
		case inCode:
			b.WriteString(html.EscapeString(ln) + "\n")
		case strings.HasPrefix(ln, "# "):
			title = strings.TrimPrefix(ln, "# ")
		case strings.HasPrefix(ln, "## "), strings.HasPrefix(ln, "### "):
			closeBlocks()
			sub := strings.HasPrefix(ln, "### ")
			text := strings.TrimLeft(ln, "# ")
			id := slug(text)
			tag := "h2"
			if sub {
				tag = "h3"
			}
			toc = append(toc, heading{ID: id, Text: text, Sub: sub})
			b.WriteString("<" + tag + ` id="` + id + `"><a class="anchor" href="#` + id + `">` + html.EscapeString(text) + "</a></" + tag + ">\n")
		case strings.HasPrefix(ln, "> "):
			lead = strings.TrimPrefix(ln, "> ")
		case strings.HasPrefix(ln, "|"):
			cells := tableRow(ln)
			if strings.HasPrefix(cells[0], "---") {
				continue
			}
			if !inTable {
				closeBlocks()
				b.WriteString(`<div class="table"><table><thead><tr>`)
				for _, c := range cells {
					b.WriteString("<th>" + inline(c) + "</th>")
				}
				b.WriteString("</tr></thead><tbody>\n")
				inTable = true
				continue
			}
			b.WriteString("<tr>")
			for _, c := range cells {
				b.WriteString("<td>" + inline(c) + "</td>")
			}
			b.WriteString("</tr>\n")
		case strings.HasPrefix(ln, "- "):
			if inTable {
				closeBlocks()
			}
			if !inList {
				b.WriteString("<ul>\n")
				inList = true
			}
			b.WriteString("<li>" + inline(strings.TrimPrefix(ln, "- ")) + "</li>\n")
		case strings.TrimSpace(ln) == "":
			closeBlocks()
		default:
			closeBlocks()
			b.WriteString("<p>" + inline(ln) + "</p>\n")
		}
	}
	closeBlocks()
	return template.HTML(b.String()), toc, title, lead
}

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
Allow: /docs/
Allow: /llms.txt
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
  <url><loc>` + h.BaseURL + `/docs/agents/</loc><lastmod>` + day + `</lastmod><changefreq>weekly</changefreq><priority>1.0</priority></url>
  <url><loc>` + h.BaseURL + `/login/</loc><changefreq>monthly</changefreq><priority>0.5</priority></url>
</urlset>
`))
}

func (h *Handler) ServerCard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"name": "space.forke/mail", "title": "Forke Mail", "version": serverVersion,
		"description": "Send transactional email and broadcasts, manage domains, templates, contacts and workflows. Risky actions ask the user first.",
		"websiteUrl":  h.BaseURL + "/docs/agents/", "documentation": h.BaseURL + "/llms.txt",
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

type docsView struct {
	Title, Lead, URL string
	LD               template.JS
	Body             template.HTML
	TOC              []heading
}

func (h *Handler) AgentsDocs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/docs/agents/" {
		http.Redirect(w, r, "/docs/agents/", http.StatusMovedPermanently)
		return
	}
	body, toc, title, lead := renderMarkdown(Guide)
	ld, _ := json.Marshal(map[string]any{
		"@context": "https://schema.org", "@type": "SoftwareApplication", "name": "Forke Mail MCP server",
		"applicationCategory": "DeveloperApplication", "operatingSystem": "Any", "url": h.BaseURL + "/docs/agents/",
		"description": lead, "offers": map[string]any{"@type": "Offer", "price": "0", "priceCurrency": "USD"},
		"publisher": map[string]any{"@type": "Organization", "name": "Forke", "url": "https://www.forke.space"},
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_ = docsPage.Execute(w, docsView{Title: title + " docs: MCP server, CLI and API for AI agents", Lead: lead, URL: h.BaseURL + "/docs/agents/", LD: template.JS(ld), Body: body, TOC: toc})
}
