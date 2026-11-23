// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"
)

type tool struct {
	Name        string
	Title       string
	Description string
	Props       map[string]any
	Required    []string
	ReadOnly    bool
	Destructive bool
	Run         func(c *call, a map[string]any) (any, error)
}

type obj = map[string]any

func str(desc string) obj { return obj{"type": "string", "description": desc} }
func num(desc string) obj { return obj{"type": "integer", "description": desc} }
func strs(desc string) obj {
	return obj{"type": "array", "items": obj{"type": "string"}, "description": desc}
}
func dict(desc string) obj {
	return obj{"type": "object", "description": desc, "additionalProperties": true}
}

func s(a map[string]any, k string) string {
	v, _ := a[k].(string)
	return strings.TrimSpace(v)
}

func id(a map[string]any, k string) (string, error) {
	v := s(a, k)
	if !httpx.ValidID(v) {
		return "", fmt.Errorf("%s must be an ID from a list tool", k)
	}
	return v, nil
}

func q(pairs ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			v.Set(pairs[i], pairs[i+1])
		}
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

func n(a map[string]any, k string) string {
	switch v := a[k].(type) {
	case float64:
		return fmt.Sprint(int(v))
	case string:
		return v
	}
	return ""
}

func get(path string) func(c *call, a map[string]any) (any, error) {
	return func(c *call, a map[string]any) (any, error) { return c.do("GET", path, nil, "") }
}

var tools = []*tool{
	{Name: "get_account", Title: "Get account", ReadOnly: true,
		Description: "Current project, plan, credits and limits, warm-up status, verified sending domains and whether this connection uses a test key. Call this first.",
		Run: func(c *call, a map[string]any) (any, error) {
			raw, err := c.do("GET", "/api/me", nil, "")
			if err != nil {
				return nil, err
			}
			mode := "live"
			if c.id.Test {
				mode = "test (emails are stored, never delivered, no credits used)"
			}
			return obj{"key_mode": mode, "account": raw}, nil
		}},
	{Name: "list_projects", Title: "List projects", ReadOnly: true,
		Description: "Projects (workspaces) the signed-in user belongs to.", Run: get("/api/projects")},
	{Name: "switch_project", Title: "Switch project",
		Description: "Point this AI connection at another of the user's projects. Only for OAuth connections; API keys always belong to one project.",
		Props:       obj{"project_id": str("Project ID from list_projects")}, Required: []string{"project_id"},
		Run: func(c *call, a map[string]any) (any, error) {
			if !c.id.Delegated {
				return nil, errors.New("API keys belong to a single project. Use a key from the other project instead")
			}
			pid, err := id(a, "project_id")
			if err != nil {
				return nil, err
			}
			tag, err := c.h.Pool.Exec(c.ctx, `
				UPDATE oauth_grants SET workspace_id = $1 WHERE access_hash = $2 AND EXISTS (
					SELECT 1 FROM workspace_members m JOIN workspaces w ON w.id = m.workspace_id
					WHERE m.user_id = oauth_grants.user_id AND m.workspace_id = $1 AND w.status = 'active')`, pid, auth.HashKey(c.token))
			if err != nil {
				return nil, err
			}
			if tag.RowsAffected() == 0 {
				return nil, errors.New("you are not a member of that project")
			}
			return obj{"switched_to": pid}, nil
		}},
	{Name: "send_email", Title: "Send an email",
		Description: "Send one transactional email (sign-in link, receipt, notification). Use html and/or text, or a saved template with data. With a live key this delivers a real email, so it asks for confirmation. With a test key nothing is delivered.",
		Props: obj{"to": str("Recipient email address"), "from": str(`Sender, e.g. "Acme <hello@acme.com>". Must be on a verified domain unless using a test key`),
			"subject": str("Subject. Optional when the template has one"), "html": str("HTML body"), "text": str("Plain-text body"),
			"template": str("Template name or ID instead of html/text"), "data": dict("Template variables, string values"),
			"reply_to": str("Reply-to address"), "idempotency_key": str("Optional key so retries never send twice")},
		Required: []string{"to", "from"},
		Run: func(c *call, a map[string]any) (any, error) {
			from, err := mail.ParseAddress(s(a, "from"))
			if err != nil {
				return nil, errors.New(`from must be an email address, optionally with a name: "Acme <hello@acme.com>"`)
			}
			body := obj{"to": s(a, "to"), "from_name": from.Name, "from_address": from.Address, "subject": s(a, "subject"),
				"html": s(a, "html"), "text": s(a, "text"), "template": s(a, "template"), "reply_to": s(a, "reply_to")}
			if d, ok := a["data"].(map[string]any); ok {
				data := map[string]string{}
				for k, v := range d {
					data[k] = fmt.Sprint(v)
				}
				body["data"] = data
			}
			headers := map[string]string{}
			if k := s(a, "idempotency_key"); k != "" {
				headers["Idempotency-Key"] = k
			}
			subject := s(a, "subject")
			if subject == "" {
				subject = "template " + s(a, "template")
			}
			return c.doWith("POST", "/api/transactional", body, fmt.Sprintf("Send a real email to %s from %s with subject %q.", s(a, "to"), from.Address, subject), headers)
		}},
	{Name: "get_email", Title: "Get a sent email", ReadOnly: true,
		Description: "Status and content of one transactional email: queued, sent, delivered, bounced, complained, opened, clicked, or test.",
		Props:       obj{"id": str("Email ID returned by send_email")}, Required: []string{"id"},
		Run: func(c *call, a map[string]any) (any, error) {
			eid, err := id(a, "id")
			if err != nil {
				return nil, err
			}
			return c.do("GET", "/api/transactional/"+eid, nil, "")
		}},
	{Name: "list_deliveries", Title: "Search deliveries", ReadOnly: true,
		Description: "Recent emails across transactional and broadcasts with their delivery status. Use it to answer why an email bounced or did not arrive.",
		Props: obj{"search": str("Recipient address or subject text"), "status": str("delivered, sent, pending, bounced, complained, failed, problems or test"),
			"kind": str("transactional or broadcast"), "limit": num("1-200, default 50")},
		Run: func(c *call, a map[string]any) (any, error) {
			return c.do("GET", "/api/deliveries"+q("q", s(a, "search"), "status", s(a, "status"), "kind", s(a, "kind"), "limit", n(a, "limit")), nil, "")
		}},
	{Name: "list_domains", Title: "List domains", ReadOnly: true, Description: "Sending domains and their verification status.", Run: get("/api/domains")},
	{Name: "add_domain", Title: "Add a sending domain",
		Description: "Add a domain to send from. Returns the DNS records (DKIM CNAMEs, SPF, DMARC) the user must add at their DNS provider. Then use check_domain.",
		Props:       obj{"name": str("Domain such as acme.com or mail.acme.com")}, Required: []string{"name"},
		Run: func(c *call, a map[string]any) (any, error) {
			return c.do("POST", "/api/domains", obj{"name": strings.ToLower(s(a, "name")), "kind": "domain"}, "")
		}},
	{Name: "check_domain", Title: "Check a domain",
		Description: "Re-check a domain's verification and DNS records, with exactly which record is missing.",
		Props:       obj{"id": str("Domain ID from list_domains")}, Required: []string{"id"},
		Run: func(c *call, a map[string]any) (any, error) {
			did, err := id(a, "id")
			if err != nil {
				return nil, err
			}
			verify, err := c.do("POST", "/api/domains/"+did+"/verify", nil, "")
			if err != nil {
				return nil, err
			}
			dns, err := c.do("GET", "/api/domains/"+did+"/dns", nil, "")
			if err != nil {
				return nil, err
			}
			return obj{"domain": verify, "dns": dns}, nil
		}},
	{Name: "list_templates", Title: "List templates", ReadOnly: true, Description: "Saved email templates with their variables.", Run: get("/api/templates")},
	{Name: "get_template", Title: "Get a template", ReadOnly: true,
		Description: "One template with its HTML, text and variables.", Props: obj{"id": str("Template ID or name")}, Required: []string{"id"},
		Run: func(c *call, a map[string]any) (any, error) {
			return c.do("GET", "/api/templates/"+url.PathEscape(s(a, "id")), nil, "")
		}},
	{Name: "save_template", Title: "Create or update a template",
		Description: "Create a template, or update the one with the same name. Use {{variable}} for escaped values and {{{variable}}} for raw HTML. kind is transactional (always delivered) or marketing (skipped for unsubscribed contacts).",
		Props: obj{"name": str("Unique template name"), "kind": str("transactional or marketing"), "subject": str("Subject, may use variables"),
			"html": str("HTML body"), "text": str("Plain-text body")},
		Required: []string{"name", "subject"},
		Run: func(c *call, a map[string]any) (any, error) {
			kind := s(a, "kind")
			if kind == "" {
				kind = "transactional"
			}
			body := obj{"name": s(a, "name"), "kind": kind, "subject": s(a, "subject"), "html": s(a, "html"), "text": s(a, "text")}
			raw, err := c.do("GET", "/api/templates", nil, "")
			if err != nil {
				return nil, err
			}
			var list struct {
				Templates []struct{ ID, Name string } `json:"templates"`
			}
			_ = json.Unmarshal(raw, &list)
			for _, t := range list.Templates {
				if strings.EqualFold(t.Name, s(a, "name")) {
					return c.do("PUT", "/api/templates/"+t.ID, body, "")
				}
			}
			return c.do("POST", "/api/templates", body, "")
		}},
	{Name: "delete_template", Title: "Delete a template", Destructive: true,
		Description: "Delete a template. Emails or workflows using it will fail, so this asks for confirmation.",
		Props:       obj{"id": str("Template ID")}, Required: []string{"id"},
		Run: func(c *call, a map[string]any) (any, error) {
			tid, err := id(a, "id")
			if err != nil {
				return nil, err
			}
			return c.do("DELETE", "/api/templates/"+tid, nil, "Delete template "+tid+". Emails and workflows that use it will stop working.")
		}},
	{Name: "list_contacts", Title: "List contacts", ReadOnly: true,
		Description: "Contacts in the project.", Props: obj{"search": str("Email or name text"), "status": str("active, unsubscribed, bounced, complained"), "limit": num("1-200"), "offset": num("Skip this many")},
		Run: func(c *call, a map[string]any) (any, error) {
			return c.do("GET", "/api/contacts"+q("search", s(a, "search"), "status", s(a, "status"), "limit", n(a, "limit"), "offset", n(a, "offset")), nil, "")
		}},
	{Name: "upsert_contact", Title: "Add or update a contact",
		Description: "Create a contact or update their name, custom data and tags. Records a contact.saved event.",
		Props: obj{"email": str("Email address"), "first_name": str("First name"), "last_name": str("Last name"),
			"data": dict("Custom fields such as plan or company (string, number or boolean values)"), "tags": strs("Tags to add, created if missing")},
		Required: []string{"email"},
		Run: func(c *call, a map[string]any) (any, error) {
			contact := obj{}
			for _, k := range []string{"first_name", "last_name"} {
				if v := s(a, k); v != "" {
					contact[k] = v
				}
			}
			if d, ok := a["data"].(map[string]any); ok {
				contact["data"] = d
			}
			return c.do("POST", "/api/events", obj{"email": s(a, "email"), "event": "contact.saved", "contact": contact, "tags": a["tags"]}, "")
		}},
	{Name: "track_event", Title: "Track an event",
		Description: "Record that a contact did something (user.signed_up, order.paid). Creates the contact if needed and starts any workflow triggered by this event, which may send emails.",
		Props: obj{"email": str("Contact email"), "event": str("Event name, letters, numbers and . _ : -"), "data": dict("Event properties"),
			"tags": strs("Tags to add to the contact"), "contact": dict("first_name, last_name and data to merge into the contact")},
		Required: []string{"email", "event"},
		Run: func(c *call, a map[string]any) (any, error) {
			body := obj{"email": s(a, "email"), "event": s(a, "event"), "data": a["data"], "tags": a["tags"]}
			if ct, ok := a["contact"].(map[string]any); ok {
				body["contact"] = ct
			}
			return c.do("POST", "/api/events", body, "")
		}},
	{Name: "list_segments", Title: "List segments", ReadOnly: true, Description: "Saved audiences built from rules, with their size.", Run: get("/api/segments")},
	{Name: "create_segment", Title: "Create a segment",
		Description: `Save an audience from rules. Each rule is {"field","op","value"}; fields include email, first_name, last_name, status, tag, created_at, last_engaged_at, event, data.<field>. Use {"field":"group","op":"any"|"all","rules":[...]} for one nested group. Call call_api with POST /api/segments/preview to check the size first.`,
		Props:       obj{"name": str("Segment name"), "match": str("all or any"), "rules": obj{"type": "array", "items": obj{"type": "object"}, "description": "Rules"}},
		Required:    []string{"name", "rules"},
		Run: func(c *call, a map[string]any) (any, error) {
			match := s(a, "match")
			if match == "" {
				match = "all"
			}
			return c.do("POST", "/api/segments", obj{"name": s(a, "name"), "match": match, "rules": a["rules"]}, "")
		}},
	{Name: "create_broadcast", Title: "Create a broadcast draft",
		Description: "Create a newsletter or announcement as a draft. Nothing is sent until send_broadcast is approved by the user.",
		Props: obj{"name": str("Internal name"), "from": str(`"Acme <news@acme.com>"`), "subject": str("Subject"), "html": str("HTML body"), "text": str("Plain-text body"),
			"segment": str("Segment name for the audience"), "audience_id": str("Tag ID for the audience (alternative to segment)"), "topic": str("Optional topic key so people can opt out of just this topic")},
		Required: []string{"from", "subject"},
		Run: func(c *call, a map[string]any) (any, error) {
			body := obj{}
			for _, k := range []string{"name", "from", "subject", "html", "text", "segment", "audience_id", "topic"} {
				if v := s(a, k); v != "" {
					body[k] = v
				}
			}
			return c.do("POST", "/api/forke-mail/broadcasts", body, "")
		}},
	{Name: "send_broadcast", Title: "Send a broadcast", Destructive: true,
		Description: "Launch a broadcast draft to its whole audience. Always needs the project owner's approval in the browser.",
		Props:       obj{"broadcast_id": str("ID from create_broadcast")}, Required: []string{"broadcast_id"},
		Run: func(c *call, a map[string]any) (any, error) {
			bid, err := id(a, "broadcast_id")
			if err != nil {
				return nil, err
			}
			summary := "Send broadcast " + bid + " to its whole audience."
			if raw, err := c.do("GET", "/api/emails/"+bid, nil, ""); err == nil {
				var e struct {
					Subject     string
					FromAddress string `json:"from_address"`
					Audience    struct{ Size int }
				}
				if json.Unmarshal(raw, &e) == nil {
					summary = fmt.Sprintf("Send the broadcast %q from %s to %d people.", e.Subject, e.FromAddress, e.Audience.Size)
				}
			}
			return c.do("POST", "/api/forke-mail/broadcasts/"+bid+"/send", nil, summary)
		}},
	{Name: "get_analytics", Title: "Get analytics", ReadOnly: true,
		Description: "Sent, delivered, opened, clicked, bounced and complaint numbers with rates, compared with the previous period.",
		Props:       obj{"days": num("1, 7, 30 or 90"), "stream": str("all, transactional or broadcast")},
		Run: func(c *call, a map[string]any) (any, error) {
			days := n(a, "days")
			if days == "" {
				days = "7"
			}
			return c.do("GET", "/api/analytics"+q("days", days, "stream", s(a, "stream")), nil, "")
		}},
	{Name: "create_api_key", Title: "Create an API key",
		Description: "Create an API key for the user's app. mode test (default) stores emails without delivering them; mode live sends for real. The key is shown once: tell the user to put it in an environment variable (FORKE_MAIL_API_KEY), never in code.",
		Props:       obj{"name": str("What the key is for"), "mode": str("test or live"), "scope": str("sending_access (default) or full_access")},
		Required:    []string{"name"},
		Run: func(c *call, a map[string]any) (any, error) {
			mode, scope := s(a, "mode"), s(a, "scope")
			if mode == "" {
				mode = "test"
			}
			if scope == "" {
				scope = auth.SendingAccess
			}
			return c.do("POST", "/api/keys", obj{"name": s(a, "name"), "mode": mode, "scope": scope}, fmt.Sprintf("Create a %s API key named %q with %s.", mode, s(a, "name"), scope))
		}},
	{Name: "call_api", Title: "Call any Forke Mail API",
		Description: "Call any /api endpoint for things the other tools do not cover (workflows, topics, suppressions, team, webhooks, settings, campaigns, reports). Reading is free; risky changes ask for confirmation or approval. See the resource forke-mail://guide for the endpoint list.",
		Props:       obj{"method": str("GET, POST, PUT, PATCH or DELETE"), "path": str("Path starting with /api/, with query string if needed"), "body": dict("JSON body")},
		Required:    []string{"method", "path"},
		Run: func(c *call, a map[string]any) (any, error) {
			method := strings.ToUpper(s(a, "method"))
			switch method {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
			default:
				return nil, errors.New("method must be GET, POST, PUT, PATCH or DELETE")
			}
			var body any
			if b, ok := a["body"]; ok && method != "GET" {
				body = b
			}
			return c.do(method, s(a, "path"), body, fmt.Sprintf("%s %s", method, s(a, "path")))
		}},
}

var toolByName = func() map[string]*tool {
	m := map[string]*tool{}
	for _, t := range tools {
		m[t.Name] = t
	}
	return m
}()

func toolList() []obj {
	out := make([]obj, 0, len(tools))
	for _, t := range tools {
		props := obj{}
		for k, v := range t.Props {
			props[k] = v
		}
		if !t.ReadOnly {
			props["confirm_token"] = str("Only after the user confirmed: the token from a CONFIRMATION NEEDED reply")
			props["approval_id"] = str("Only after the user approved in the browser: the ID from an APPROVAL NEEDED reply")
		}
		required := t.Required
		if required == nil {
			required = []string{}
		}
		out = append(out, obj{
			"name": t.Name, "title": t.Title, "description": t.Description,
			"inputSchema": obj{"type": "object", "properties": props, "required": required},
			"annotations": obj{"title": t.Title, "readOnlyHint": t.ReadOnly, "destructiveHint": t.Destructive, "openWorldHint": t.Name == "send_email" || t.Name == "send_broadcast"},
		})
	}
	return out
}
