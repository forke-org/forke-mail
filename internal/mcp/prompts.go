// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package mcp

import (
	"strings"
)

type promptArg struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"-"`
}

type Prompt struct {
	Name        string      `json:"name"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Arguments   []promptArg `json:"arguments"`
	Text        string      `json:"text"`
}

var Prompts = []Prompt{
	{Name: "setup-app", Title: "Set up Forke Mail in my app",
		Description: "Add email to the project you're working on: key, helper, first test send.",
		Arguments:   []promptArg{{Name: "framework", Description: "Your stack, such as Next.js, Express, Django or Laravel", Default: "web"}},
		Text: `Set up Forke Mail in this {{framework}} project.
1. Call get_account to see my project, verified domains and limits.
2. Create a test API key with create_api_key (mode "test") and add it to .env as FORKE_MAIL_API_KEY. Make sure .env is in .gitignore.
3. Add a small sendEmail helper that calls POST https://mail.forke.space/api/transactional with the key from the environment and an Idempotency-Key header.
4. Send one test email through the helper and show me the result.
5. Tell me what I need to switch to live sending (verified domain and a live key).`},
	{Name: "welcome-email", Title: "Create a welcome email",
		Description: "Design a welcome email template that matches your product.",
		Arguments:   []promptArg{{Name: "product", Description: "Product name", Required: true, Default: "my product"}, {Name: "tone", Description: "Friendly, formal, playful…", Default: "friendly"}},
		Text: `Create a welcome email template for {{product}} with a {{tone}} tone.
Use save_template named "welcome" (kind transactional) with {{name}} for the person's name and a clear call to action button. Keep the HTML simple and mobile friendly, and include a plain-text version. Then send me a preview with a test key.`},
	{Name: "auth-emails", Title: "Add sign-in and password reset emails",
		Description: "Templates and code for magic links, one-time codes and password resets.",
		Arguments:   []promptArg{{Name: "framework", Description: "Your stack", Default: "web"}},
		Text: `Add sign-in and password reset emails to this {{framework}} app with Forke Mail.
Create templates "sign-in-code" ({{code}}, expires in 10 minutes) and "password-reset" ({{reset_link}}) with save_template, then wire them into the existing auth flow using the template name and data. Use an Idempotency-Key per request so retries never send twice.`},
	{Name: "welcome-series", Title: "Build a welcome email series",
		Description: "A workflow that sends a short series after someone signs up.",
		Arguments:   []promptArg{{Name: "event", Description: "The event that starts it, such as user.signed_up", Default: "user.signed_up"}},
		Text: `Build a welcome series that starts when the {{event}} event is tracked.
Draft three short emails (day 0, day 2, day 5) as templates, then create a workflow with call_api POST /api/workflows using wait and send steps, exiting if the person does the main action. Show me the plan before you create the workflow, and track one test event to check it.`},
	{Name: "newsletter", Title: "Write and send a newsletter",
		Description: "Draft a broadcast for a segment and send it when you're happy.",
		Arguments:   []promptArg{{Name: "topic", Description: "What it's about", Required: true, Default: "our latest update"}, {Name: "segment", Description: "Segment name to send to", Default: "main"}},
		Text: `Write a newsletter about {{topic}} for the {{segment}} segment.
Check the segment size with list_segments, draft it with create_broadcast (keep the {{unsubscribe_url}} link), show me the subject and content, and only call send_broadcast after I say yes.`},
	{Name: "verify-domain", Title: "Verify my sending domain",
		Description: "Add a domain and walk through the DNS records step by step.",
		Arguments:   []promptArg{{Name: "domain", Description: "Your domain, such as acme.com", Required: true, Default: "my domain"}},
		Text: `Help me send email from {{domain}}.
Add it with add_domain, then show me each DNS record as a small table (type, name, value) for my DNS provider. After I add them, run check_domain and tell me exactly which record is still missing until it's verified.`},
	{Name: "debug-delivery", Title: "Why didn't my email arrive?",
		Description: "Find a missing email and explain what happened.",
		Arguments:   []promptArg{{Name: "email", Description: "Recipient address", Required: true, Default: "the recipient"}},
		Text:        `An email to {{email}} didn't arrive. Use list_deliveries with that address, then get_email for the latest one, and explain in plain words what happened (sent, delivered, bounced, suppressed or failed) and what I should do next.`},
	{Name: "migrate", Title: "Move to Forke Mail from another provider",
		Description: "Replace Resend, SendGrid, Postmark or SES calls in your code.",
		Arguments:   []promptArg{{Name: "provider", Description: "The current provider", Required: true, Default: "my current provider"}},
		Text: `Move this codebase from {{provider}} to Forke Mail.
Find every place that sends email, list them for me first, then replace them with calls to POST https://mail.forke.space/api/transactional (or templates where the same email repeats). Keep the same from addresses, add idempotency keys, and use a test key until I say to go live.`},
	{Name: "weekly-report", Title: "How are my emails doing?",
		Description: "A short report on delivery, opens, clicks and problems.",
		Text:        `Give me a short report on my email over the last 7 days using get_analytics and list_deliveries with status "problems": what was sent, delivery and open rates compared with the week before, and anything that bounced or was reported as spam, with one suggestion to improve.`},
}

func (p Prompt) render(args map[string]string) string {
	out := p.Text
	for _, a := range p.Arguments {
		v := strings.TrimSpace(args[a.Name])
		if v == "" {
			v = a.Default
		}
		out = strings.ReplaceAll(out, "{{"+a.Name+"}}", v)
	}
	return out
}

func promptByName(name string) (Prompt, bool) {
	for _, p := range Prompts {
		if p.Name == name {
			return p, true
		}
	}
	return Prompt{}, false
}

func promptsMarkdown() string {
	var b strings.Builder
	for _, p := range Prompts {
		b.WriteString("### " + p.Title + "\n\n" + p.Description + "\n\n```text\n" + p.render(nil) + "\n```\n\n")
	}
	return strings.TrimSpace(b.String())
}

func PromptCards() []map[string]string {
	out := make([]map[string]string, 0, len(Prompts))
	for _, p := range Prompts {
		out = append(out, map[string]string{"name": p.Name, "title": p.Title, "description": p.Description, "text": p.render(nil)})
	}
	return out
}
