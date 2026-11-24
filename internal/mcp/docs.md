# Forke Mail

> Forke Mail is an email API for developers and AI agents: transactional email, templates, contacts, events, workflows and broadcasts, delivered through Amazon SES. Connect Claude, ChatGPT, Cursor or any MCP app in one step, or use the CLI and HTTP API.

## Overview

- **MCP server:** `https://mail.forke.space/mcp`. Any AI app that supports the Model Context Protocol can add domains, write templates, send email, track events and read analytics for you.
- **CLI:** `forke-mail`, one binary for macOS, Linux and Windows. It signs in through your browser, sets up projects and runs the MCP server locally.
- **HTTP API:** plain JSON over HTTPS at `https://mail.forke.space/api`. Every tool the agents use is an API call you can make yourself.
- **Safe by default:** agents do everyday work on their own, and ask you in the chat before anything risky, saying clearly when something can't be undone.
- **Free to build with:** test keys (`fm_test_…`) run every call for real but never deliver email and never use credits.

## Quickstart

Pick the path that fits how you work.

### With an AI app

- Add `https://mail.forke.space/mcp` as a connector in your AI app (steps for each app are below).
- Sign in to Forke Mail in the browser window that opens, pick a project and press **Allow**.
- Ask it things like "set up Forke Mail for my Next.js app", "add my domain acme.com", "write a welcome email template" or "why did the email to ada@example.com bounce?".

### With the CLI

```sh
brew install forke-org/tap/forke-mail   # or: curl -fsSL https://mail.forke.space/install.sh | sh
forke-mail login
cd my-app
forke-mail init
forke-mail send --to you@example.com --from "Acme <hi@acme.com>" --subject "Hello" --text "It works"
```

`forke-mail init` creates a test key, saves it to `.env` as `FORKE_MAIL_API_KEY`, adds `.env` to `.gitignore` and writes Forke Mail notes into `AGENTS.md` so your coding agent knows how to send email in this project.

### With the HTTP API

```sh
curl https://mail.forke.space/api/transactional \
  -H "Authorization: Bearer $FORKE_MAIL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"to":"ada@example.com","from_name":"Acme","from_address":"hi@acme.com","subject":"Welcome","html":"<p>Hi Ada!</p>"}'
```

## Connect your AI app

All apps use the same address: `https://mail.forke.space/mcp`. Sign-in uses OAuth, so you never paste a key into the chat.

### Claude (web and desktop)

- Open **Settings → Connectors → Add custom connector**.
- Name it Forke Mail and paste `https://mail.forke.space/mcp`.
- Press **Connect**, sign in, choose a project and press **Allow**.

### ChatGPT

- Open **Settings → Connectors → Create** (developer mode may need to be on for custom connectors).
- Paste `https://mail.forke.space/mcp` as the MCP server URL and choose **OAuth** for authentication.
- Sign in, choose a project and press **Allow**.

### Claude Code

```sh
claude mcp add --transport http forke-mail https://mail.forke.space/mcp
```

Then run `/mcp` inside Claude Code and choose Forke Mail to sign in.

### Cursor, Windsurf, VS Code, Antigravity and other apps

Add this to the app's MCP settings file (for Cursor: `~/.cursor/mcp.json` or `.cursor/mcp.json` in your project):

```json
{
  "mcpServers": {
    "forke-mail": { "url": "https://mail.forke.space/mcp" }
  }
}
```

The app opens your browser to sign in the first time it connects.

### Apps that only run local servers

Install the CLI, sign in once with `forke-mail login`, then use the CLI as a local (stdio) server:

```json
{
  "mcpServers": {
    "forke-mail": { "command": "forke-mail", "args": ["mcp"] }
  }
}
```

### Servers, CI and agent frameworks

Skip OAuth and send an API key as a bearer token to the MCP endpoint, or set `FORKE_MAIL_API_KEY` for the CLI. Use a test key while developing.

```json
{
  "mcpServers": {
    "forke-mail": {
      "url": "https://mail.forke.space/mcp",
      "headers": { "Authorization": "Bearer fm_test_..." }
    }
  }
}
```

## Signing in and projects

- When an app connects, Forke Mail shows a sign-in page and then a permission screen with the app's name. Pick the project it should use and press **Allow**. **Cancel** sends the app away with nothing.
- If you are not signed in yet, you sign in first and land back on the permission screen.
- An app works inside one project at a time. Ask it to "switch to my other project" and it uses `switch_project`. API keys always belong to a single project.
- Access tokens last an hour and renew themselves for 60 days of use. See and disconnect every app under **Connectors** in the dashboard; disconnecting takes effect immediately.
- Apps act with your role in the project. Anything your role can't do, the app can't do either.

## Safety model

Every action is sorted into one of four levels. The server enforces this, not the AI app, so a confused or misbehaving agent can't skip it.

| Level | What happens | Examples |
| --- | --- | --- |
| Runs directly | The tool just runs. | Reading anything, adding a domain, saving templates and segments, drafting broadcasts, tracking events, sending with a test key |
| Asks you in the chat | The tool replies `CONFIRMATION NEEDED` with a summary and a one-time `confirm_token`. The agent must show you the summary and only continue after you say yes. | Sending a real email, creating API keys, importing contacts, creating or changing workflows and webhooks, changing settings, deleting templates, contacts or segments |
| Asks you in the chat, marked can't be undone | The reply starts `CONFIRMATION NEEDED (can't be undone)`, and the agent must tell you so before you answer. | Launching a broadcast, deleting a domain, resetting or deleting a project, revoking or rotating keys, changing roles or removing members, moving credits, removing suppressions |
| Never through agents | The tool refuses and points you to the dashboard. | Passwords, signing in or out, platform administration |

Details that keep this safe:

- A `confirm_token` only works for the exact same tool and arguments, for 10 minutes. Changing anything (a different recipient, a new subject) needs a new confirmation.
- The check happens on Forke Mail's server, so an agent cannot skip it by calling the API differently.

Example conversation:

```text
You:    Send the welcome email to ada@example.com
Agent:  Forke Mail wants confirmation: send a real email to ada@example.com
        from hi@acme.com with subject "Welcome to Acme". Should I send it?
You:    Yes
Agent:  Sent. Message ID 5f1c…, status queued.
```

## Credits, limits and payments

- Live sends use credits and count towards your daily limit. New projects warm up over 14 days with a growing daily broadcast cap to protect deliverability.
- Test keys never use credits.
- When a project is out of credits or over a limit, the API returns an error (often `402`) and agents tell you what happened and where to fix it.
- Agents never buy, pay, add credits or enter payment details. Moving credits between projects is marked can't be undone and needs your yes in the chat, as will anything involving money.
- `get_account` (or `forke-mail whoami`) shows the plan, credits, limits and warm-up status.

## Test mode and live mode

- **Test keys** (`fm_test_…`): requests are validated exactly like live ones, emails are stored with status `test` and appear in **Deliveries**, but nothing is delivered and no credits are used. Any from address works, so you can build before your domain is verified. Test keys can't launch broadcasts.
- **Live keys** (`fm_live_…`): real delivery from verified domains.
- Create either kind under **API keys** in the dashboard, with `forke-mail init`, or ask your agent to `create_api_key`.
- Keep keys in an environment variable such as `FORKE_MAIL_API_KEY`. Never put them in code or commit them.

## Tools reference

These are the tools the MCP server and CLI offer right now. Every tool that changes something also accepts `confirm_token`, which agents fill in after you confirm in the chat.

{{tools}}

## Prompts to try

Ready-made requests that work well with the Forke Mail tools. In Claude and other apps that support MCP prompts they appear in the prompt or slash-command menu; anywhere else, copy one into the chat.

{{prompts}}

## CLI reference

### Install

With [Homebrew](https://brew.sh) on macOS or Linux:

```sh
brew install forke-org/tap/forke-mail
```

Or with the install script, which checks the release's SHA-256 checksum:

```sh
curl -fsSL https://mail.forke.space/install.sh | sh
```

On Windows, download `forke-mail_<version>_windows_amd64.zip` from the [latest release](https://github.com/forke-org/forke-mail/releases/latest), unzip it and put `forke-mail.exe` on your PATH. Debian, Fedora and Alpine packages are on the same page. Update with `brew upgrade forke-mail` or by running the install script again. To build from source: `go build -o forke-mail ./cmd/forke-mail` in the forke-mail repository.

### Commands

| Command | What it does |
| --- | --- |
| `forke-mail login` | Sign in through your browser. Credentials are stored in your user config folder, readable only by you. |
| `forke-mail logout` | Revoke this device and remove the stored credentials. |
| `forke-mail whoami` | Project, plan, credits, limits and whether you're using a test key. |
| `forke-mail init` | Set up the current folder: test key in `.env`, `.env` in `.gitignore`, notes in `AGENTS.md`. |
| `forke-mail send` | Send an email: `--to`, `--from`, `--subject`, `--text`, `--html`, `--html-file`, `--template`, `--data key=value`. |
| `forke-mail domains` | List sending domains and their status. |
| `forke-mail tools` | List every tool with its description. |
| `forke-mail call <tool>` | Run any tool with JSON (`'{"name":"x"}'`) or `key=value` arguments. |
| `forke-mail mcp` | Run as a local MCP server over stdio. |

Options: `--yes` answers confirmation prompts with yes, `--json` prints raw output, `--no-browser` prints links instead of opening them, `--url` points at another server. Environment: `FORKE_MAIL_API_KEY` uses a key instead of signing in, `FORKE_MAIL_URL` changes the server.

When a command needs confirmation the CLI asks in the terminal, and says when the action can't be undone.

## HTTP API

### Authentication and errors

- Send `Authorization: Bearer <API key>` on every request.
- Errors are JSON: `{"error":"snake_case_code"}` with a matching status code. `401` means a missing or revoked key, `403` that the key's permission or your role doesn't allow it, `404` that nothing with that ID exists in this project, `409` a conflict, `422` invalid input, `402` out of credits or over a limit, `429` too many requests.
- Send an `Idempotency-Key` header when sending email so retries never send twice.

### Send email

`POST /api/transactional`

```json
{
  "to": "ada@example.com",
  "from_name": "Acme",
  "from_address": "hi@acme.com",
  "reply_to": "support@acme.com",
  "subject": "Your receipt",
  "html": "<p>Thanks for your order.</p>",
  "text": "Thanks for your order.",
  "template": "receipt",
  "data": { "name": "Ada", "total": "$42" },
  "track_opens": true,
  "track_clicks": true
}
```

Use either `html`/`text` or `template` with `data`. `GET /api/transactional/{id}` returns the status: `queued`, `sent`, `failed` or `test`, plus delivered, bounced, complained, opened and clicked times.

```ts
await fetch("https://mail.forke.space/api/transactional", {
  method: "POST",
  headers: {
    Authorization: `Bearer ${process.env.FORKE_MAIL_API_KEY}`,
    "Content-Type": "application/json",
    "Idempotency-Key": `welcome-${user.id}`,
  },
  body: JSON.stringify({ to: user.email, from_address: "hi@acme.com", from_name: "Acme", template: "welcome", data: { name: user.name } }),
});
```

### Templates

- `GET /api/templates`, `POST /api/templates`, `GET|PUT|DELETE /api/templates/{id}` (the template ID; the MCP tools also accept names).
- Body: `name`, `kind` (`transactional` reaches everyone; `marketing` skips unsubscribed contacts), `subject`, `html`, `text`.
- `{{name}}` inserts an escaped value, `{{{html}}}` inserts raw HTML, `{{date}}` is today's date.

### Contacts and events

- `POST /api/events` with `email`, `event`, optional `data`, `tags` and `contact` (`first_name`, `last_name`, `data`) creates or updates the contact, records the event and starts matching workflows.
- `GET /api/contacts?search=&status=`, `POST /api/contacts/import` (CSV), `DELETE /api/contacts/{id}`, `GET /api/tags`.

### Segments

- `GET|POST /api/segments`, `POST /api/segments/preview` to see the size first.
- Rules: `{"field","op","value"}` with fields such as `email`, `status`, `tag`, `created_at`, `last_engaged_at`, `event` and `data.<field>`, combined with `match: all|any` and one level of `{"field":"group","op":"any|all","rules":[...]}`.

### Broadcasts

- `POST /api/forke-mail/broadcasts` creates a draft: `from`, `subject`, `html`/`text` or `template` with `data`, and `segment` (name) or `audience_id` (tag), optional `topic`. Templates for broadcasts must keep the `{{unsubscribe_url}}` link.
- `POST /api/forke-mail/broadcasts/{id}/send` launches it. Through agents this always asks you first, marked can't be undone.

### Workflows

- `GET|POST /api/workflows`, `GET|PUT|DELETE /api/workflows/{id}`, `GET /api/workflows/{id}/runs`.
- A workflow has a trigger event, steps (`wait` and `send` with a template) and an optional exit event that stops it.

### Domains

- `POST /api/domains` with `name` returns the DNS records to add: three DKIM CNAMEs, SPF and DMARC.
- `POST /api/domains/{id}/verify` re-checks, `GET /api/domains/{id}/dns` shows exactly which record is missing.

### Webhooks, analytics and more

- Webhooks: `GET|POST /api/webhooks`, `PATCH|DELETE /api/webhooks/{id}`, `POST /api/webhooks/{id}/rotate`, signed with HMAC.
- Analytics: `GET /api/analytics?days=1|7|30|90&stream=all|transactional|broadcast`.
- Deliveries: `GET /api/deliveries?q=&status=&kind=`.
- Suppressions `GET|POST /api/suppressions`, topics `GET /api/topics`, team `GET /api/team`, account `GET /api/me`.

## Building your own MCP client

- Discovery: `GET /.well-known/oauth-protected-resource` and `GET /.well-known/oauth-authorization-server`. An unauthenticated call to `/mcp` returns `401` with a `WWW-Authenticate` header pointing at the first.
- Register with `POST /oauth/register` (dynamic client registration, public clients). Redirect URIs may be `https`, `http://127.0.0.1` or `http://localhost` on any port, or an app scheme such as `cursor://`.
- Authorize with `GET /oauth/authorize` using PKCE (`S256`), then exchange the code at `POST /oauth/token`. Refresh tokens rotate on every use. Revoke with `POST /oauth/revoke`.
- The MCP endpoint speaks JSON-RPC over Streamable HTTP (protocol versions 2025-06-18, 2025-03-26 and 2024-11-05) and answers with JSON.
- Server card: `GET /.well-known/mcp.json`.

## Troubleshooting

- **sender_not_verified:** the from address isn't on a verified domain. Add the domain, add its DNS records and run `check_domain`. Test keys accept any from address.
- **402 or a limit error:** the project is out of credits or over its daily limit. Check `get_account`.
- **CONFIRMATION NEEDED keeps coming back:** the arguments changed between calls. Confirm the final version.
- **The app can't connect:** remove the connector and add it again to sign in fresh, and check status.forke.space.

## Links

- Dashboard: https://mail.forke.space
- Connectors in the dashboard: https://mail.forke.space/connectors/
- Full docs: https://www.forke.space/docs/forke-mail
- Machine-readable docs: https://mail.forke.space/llms.txt
- Status: https://status.forke.space
- Support: support@forke.space
