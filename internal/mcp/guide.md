# Forke Mail

> Email API for developers and AI agents: transactional emails, templates, contacts, events, workflows and broadcasts, delivered through Amazon SES. Base URL: https://mail.forke.space

## Connect an AI agent (MCP)

- Remote MCP server: `https://mail.forke.space/mcp` (Streamable HTTP). Sign in with OAuth in the browser, or send `Authorization: Bearer <API key>`.
- Claude (claude.ai, Desktop): Settings → Connectors → Add custom connector → `https://mail.forke.space/mcp`.
- ChatGPT: Settings → Connectors → Create → MCP server URL `https://mail.forke.space/mcp`, authentication OAuth.
- Claude Code: `claude mcp add --transport http forke-mail https://mail.forke.space/mcp`
- Cursor, Windsurf, VS Code, Antigravity and other clients: add `{"mcpServers": {"forke-mail": {"url": "https://mail.forke.space/mcp"}}}`, or run the CLI as a local server: `{"command": "forke-mail", "args": ["mcp"]}`.

## Safety model

- Reading, drafting and test sends run directly.
- Sending real email, creating API keys, importing contacts and deleting things answer `CONFIRMATION NEEDED`. The agent must ask the user, then repeat the call with `confirm_token`.
- Launching broadcasts, deleting domains or projects, revoking keys, changing roles and anything about credits or payments answer `APPROVAL NEEDED` with a link. The user approves in the browser; the agent repeats the call with `approval_id`.
- Agents never pay, buy or add credits. Signing in, passwords and account deletion stay with the user.

## API keys

- `fm_test_…` keys: every call works and is validated, emails are stored with status `test` and never delivered, no credits used. Use these while building.
- `fm_live_…` keys: real sending. Keep keys in an environment variable such as `FORKE_MAIL_API_KEY`, never in code.
- Send `Authorization: Bearer <key>`. Errors look like `{"error":"snake_case_code"}`.

## Send an email

```sh
curl https://mail.forke.space/api/transactional \
  -H "Authorization: Bearer $FORKE_MAIL_API_KEY" -H "Content-Type: application/json" \
  -H "Idempotency-Key: order-123-receipt" \
  -d '{"to":"ada@example.com","from_name":"Acme","from_address":"hello@acme.com","subject":"Your receipt","html":"<p>Thanks!</p>"}'
```

```ts
await fetch("https://mail.forke.space/api/transactional", {
  method: "POST",
  headers: { Authorization: `Bearer ${process.env.FORKE_MAIL_API_KEY}`, "Content-Type": "application/json" },
  body: JSON.stringify({ to, from_address: "hello@acme.com", from_name: "Acme", template: "welcome", data: { name } }),
});
```

The from address must be on a verified domain (test keys accept any). Use `template` (name or ID) with `data` instead of `subject`/`html`; `{{var}}` is escaped, `{{{var}}}` is raw HTML.

## Main endpoints

- `GET /api/me` project, plan, credits, limits and verified domains
- `POST /api/transactional`, `GET /api/transactional/{id}`, `GET /api/deliveries?q=&status=`
- `GET|POST /api/domains`, `POST /api/domains/{id}/verify`, `GET /api/domains/{id}/dns`
- `GET|POST /api/templates`, `GET|PUT|DELETE /api/templates/{id}`
- `GET /api/contacts?search=`, `POST /api/contacts/import`, `DELETE /api/contacts/{id}`
- `POST /api/events` `{"email","event","data","tags","contact":{"first_name","last_name","data"}}` upserts the contact and starts workflows
- `GET|POST /api/segments`, `POST /api/segments/preview`
- `GET|POST /api/workflows`, `GET|PUT|DELETE /api/workflows/{id}`
- `POST /api/forke-mail/broadcasts` (draft), `POST /api/forke-mail/broadcasts/{id}/send`
- `GET /api/analytics?days=7&stream=all`
- `GET|POST /api/suppressions`, `GET /api/topics`, `GET /api/team`, webhooks under `/api/webhooks`

## Links

- Dashboard: https://mail.forke.space
- Agent docs: https://mail.forke.space/docs/agents
- Status: https://status.forke.space
- Support: support@forke.space
