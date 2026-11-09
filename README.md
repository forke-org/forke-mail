# Forke Mail

Forke Mail is Forke's own email service and our replacement for Resend. It imports contacts, groups broadcast emails into campaigns, sends one-to-one transactional email (sign-in codes, password resets, receipts) over the API, and delivers through an **SMTP relay** (OCI Email Delivery on the Oracle Always Free tier) or **Amazon SES**.

It speaks a **Resend-compatible API** under `/api/resend`, so the Forke apps switch over by changing a base URL and an API key. Forke admins sign in with their existing Forke accounts: `super_admin` and `admin` roles come straight from Forke's `admins` table.

It is a single Go binary that embeds a Next.js dashboard and uses PostgreSQL for all state. It needs no Redis, message broker or ORM.

## Contents

- [Features](#features)
- [How it works](#how-it-works)
- [Requirements](#requirements)
- [Quick start (local, no AWS needed)](#quick-start-local-no-aws-needed)
- [Configuration](#configuration)
- [Sign-in and roles](#sign-in-and-roles)
- [Switching Forke from Resend](#switching-forke-from-resend)
- [Sending through OCI Email Delivery](#sending-through-oci-email-delivery)
- [Using the dashboard](#using-the-dashboard)
- [REST API](#rest-api)
- [Transactional email](#transactional-email)
- [Sending real email with Amazon SES](#sending-real-email-with-amazon-ses)
- [Testing SES events without AWS](#testing-ses-events-without-aws)
- [Running the tests](#running-the-tests)
- [Deploying to production](#deploying-to-production)
- [Project layout](#project-layout)
- [Troubleshooting](#troubleshooting)
- [Security model](#security-model)
- [Credits](#credits)

## Features

- **Forke sign-in.** No public sign-up. Forke admins log in with their Forke email (or username) and password, checked against Forke's `admins` table. `super_admin` and `admin` roles map to permissions, and a role change or a disabled admin in Forke applies on the very next request.
- **Workspaces.** Each workspace is fully isolated from the others. Forke admins share one workspace (`FORKE_WORKSPACE_NAME`).
- **Plans.** Trial workspaces get a fixed send quota, enforced atomically even under concurrency. When the quota runs out, the dashboard shows an upgrade prompt. Paid workspaces get a daily limit.
- **API keys.** `fm_live_…` keys are stored only as SHA-256 hashes. They come in two scopes, `full_access` and `sending_access`, and are created, rotated or revoked in the dashboard after re-entering your password.
- **Contacts.**
  - CSV or pasted import of up to 50,000 rows, with tags applied at import time.
  - Search, filter and bulk tagging.
  - Addresses that are suppressed, bounced or unsubscribed are never reactivated by an import.
- **Campaigns and emails.** A campaign groups emails, for example "Product updates". Each email targets one of:
  - all contacts;
  - contacts with chosen tags;
  - hand-picked contacts;
  - contacts imported straight into the email.
- **Sending pipeline.**
  - A Postgres-backed worker claims recipients with `FOR UPDATE SKIP LOCKED`.
  - Each workspace gets its own rate limit, under one global SES rate limit.
  - Failed sends are retried with backoff.
  - A sweeper recovers recipients stuck mid-send after a crash.
  - Every email gets a per-recipient unsubscribe link and the `List-Unsubscribe` / one-click headers.
- **SMTP relay.** `SENDER_DRIVER=smtp` delivers through any authenticated SMTP relay over STARTTLS or TLS, built for OCI Email Delivery. Daily and monthly caps keep you inside a free allowance, and a plain-text part is generated from HTML when you only send HTML.
- **Resend-compatible API.** `POST /emails`, audiences, contacts and broadcasts under `/api/resend`, including the `{{{RESEND_UNSUBSCRIBE_URL}}}` placeholder.
- **Amazon SES integration.**
  - Sends through SESv2 using a configuration set and message tags.
  - Classifies SES errors without SDK retries; ambiguous outcomes are never retried, so no one gets a duplicate.
  - Follows the account's SES quota automatically.
- **Domain verification.** Add a domain and the dashboard shows the exact DKIM, SPF and DMARC records to publish, or add a single email address for sandbox testing. Pending identities are rechecked every minute.
- **SNS feedback webhook.**
  - Every message's signature is verified, with signature versions 1 and 2 supported.
  - Each notification is processed exactly once.
  - Delivery, bounce, complaint, reject and delay events update the per-email report.
- **Suppressions.**
  - Sources: the unsubscribe link, SES hard bounces, SES complaints, manual adds and imports.
  - Checked at launch, again right before every send, and on every contact import.
- **Reputation guard.** A workspace whose 30-day bounce or complaint rate crosses a threshold is paused automatically, protecting the shared SES account for every other tenant.
- **Dashboard.** Built with Next.js and TanStack Table, in light and dark themes, served from the same origin under a strict CSP.

## How it works

```
                 ┌─────────────────────────── Go binary ────────────────────────────┐
 Browser ──────▶ │ chi router ─ auth (session cookie / API key) ─ scopes ─ handlers │
 API clients ──▶ │      │                                                           │
                 │      ├── embedded Next.js dashboard (static export)              │
                 │      ├── /unsubscribe/{token}   (HMAC-signed, public)            │
                 │      └── /webhooks/ses          (SNS signature is the auth)      │
                 │                                                                  │
                 │ background: send worker · stuck-row sweeper · SES quota monitor  │
                 │             · identity verification poller                       │
                 └──────────────┬───────────────────────────────┬───────────────────┘
                                │                               │
                           PostgreSQL                 Amazon SES ──▶ SNS topic
                      (all state, migrations                 ▲              │
                       run on startup)                       └── events ────┘
```

1. **Launching** an email snapshots its audience into `email_recipients`, leaving out suppressed and inactive contacts.
2. **The worker** claims pending recipients for each workspace, then for each one:
   - checks suppressions again;
   - reserves quota atomically;
   - renders the message and sends it through the configured sender.
3. **The sender** is selected by `SENDER_DRIVER`:
   - `log` stores the rendered message in `message_logs` and feeds synthetic SES events through the event processor.
   - `smtp` delivers through an SMTP relay such as OCI Email Delivery.
   - `ses` calls Amazon SES.
4. **SES events** reach `/webhooks/ses` via SNS. They are verified, deduplicated and applied: a delivery stamps the recipient, and a hard bounce or complaint suppresses the address and re-evaluates the workspace's reputation.

The `log` driver runs the exact same event processor as production, so the whole flow works locally without an AWS account.

## Requirements

| Tool | Version | Used for |
|---|---|---|
| Go | 1.26+ | server and CLI tools |
| Node.js | 20.9+ (22 recommended) | building the dashboard |
| Docker | any recent | local PostgreSQL 17 |
| ngrok | 3.28+ | only for receiving real SNS events on your machine |
| AWS account | – | only for real sending |

## Quick start (local, no AWS needed)

```sh
git clone <your-repo-url> forke-mail && cd forke-mail

cp .env.example .env
# Edit .env: set UNSUBSCRIBE_SECRET and SEED_TOKEN to random values of 32+ characters, e.g.
#   openssl rand -hex 32

docker compose up -d                              # PostgreSQL on localhost:5433 (also creates forkemail_test)
(cd frontend && npm ci && npm run build)          # builds the dashboard into frontend/out (embedded in the binary)

set -a && . ./.env && set +a
go run ./cmd/server                               # migrations run automatically on startup
```

Locally you can leave `FORKE_DATABASE_URL` empty and sign in with a demo account from `/dev/seed` (below). To sign in with real Forke accounts, point `FORKE_DATABASE_URL` at a copy of Forke's database (only the `admins` table is read). With the `log` driver, domains you add are verified instantly and sends are logged rather than delivered.

### Demo data

In development (`APP_ENV=development`) you can create three demo workspaces in one call. Each gets an owner login and an API key, which are shown only once:

```sh
curl -X POST "http://localhost:8080/dev/seed?reset=true" -H "X-Seed-Token: $SEED_TOKEN"
```

| Workspace | Plan | API key scope |
|---|---|---|
| Forke | paid | `full_access` |
| Acme Trial | trial, 100 sends | `sending_access` |
| Beta Trial | trial, 5 sends (handy for testing the quota prompt) | `sending_access` |

`reset=true` **wipes all workspaces** first. Then import `samples/contacts.csv` from the Contacts page. It contains normal addresses, the SES mailbox simulator addresses, a duplicate and two invalid rows.

### Working on the dashboard

```sh
cd frontend && npm run dev        # http://localhost:3000 with hot reload; proxies /api to the Go server on :8080
```

Keep the Go server running alongside it. Run `npm run build` before building the Go binary so it embeds your changes.

## Configuration

All configuration comes from environment variables, and `.env.example` lists every one. The server validates them at startup and **refuses to start** if a value is missing or invalid.

### Core

| Variable | Default | Description |
|---|---|---|
| `APP_ENV` | `production` | `development` enables `/dev/seed` and `/dev/ses-event` and allows an `http://` base URL with the log driver |
| `HTTP_ADDR` | `:8080` | listen address |
| `DATABASE_URL` | – (required) | PostgreSQL connection string |
| `PUBLIC_BASE_URL` | – (required) | public origin, no trailing slash. Must be `https://` except in development with the log driver. Used for unsubscribe links, `List-Unsubscribe` headers and the SNS endpoint; session cookies are `Secure` when it is https |
| `UNSUBSCRIBE_SECRET` | – (required, 32+ chars) | HMAC key for unsubscribe tokens. Changing it invalidates links in emails already sent |
| `SEED_TOKEN` | – (32+ chars, development only) | protects `/dev/seed` and `/dev/ses-event` |
| `TRUSTED_PROXY` | empty | IP or CIDR of your reverse proxy. Only then is `X-Forwarded-For` trusted for client IPs, which rate limits depend on |
| `FORKE_DATABASE_URL` | – (required outside development) | Forke's PostgreSQL. Only `admins` (`id`, `email`, `username`, `name`, `password_hash`, `role`, `is_disabled`) is read. A read-only database user is enough |
| `FORKE_WORKSPACE_NAME` | `Forke` | the workspace Forke admins land in; created on first sign-in |

### Sending

| Variable | Default | Description |
|---|---|---|
| `SENDER_DRIVER` | `log` | `log` (store messages, simulate events), `smtp` (relay such as OCI Email Delivery) or `ses` |
| `SEND_RATE_PER_SEC` | `20` | global send rate for the log and smtp drivers. With SES it is replaced by the account's `MaxSendRate` |
| `TRIAL_SEND_RATE_PER_SEC` | `1` | send rate for trial workspaces |
| `MAX_BOUNCE_RATE` | `0.05` | reputation guard: 30-day hard bounce rate that pauses a workspace |
| `MAX_COMPLAINT_RATE` | `0.001` | reputation guard: 30-day complaint rate that pauses a workspace |
| `REPUTATION_MIN_SAMPLE` | `100` | the guard only acts once a workspace has sent to at least this many recipients in 30 days |

### SMTP relay (required when `SENDER_DRIVER=smtp`)

| Variable | Default | Description |
|---|---|---|
| `SMTP_HOST` | – | relay host, e.g. `smtp.email.ap-mumbai-1.oci.oraclecloud.com` |
| `SMTP_PORT` | `587` | relay port |
| `SMTP_TLS` | `starttls` | `starttls`, `tls` (implicit, port 465) or `none` (development only) |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | empty | SMTP credentials; set both or neither |
| `SMTP_HELO_NAME` | empty | name sent in `EHLO`, e.g. `mail.forke.space` |
| `SMTP_ALLOWED_SENDERS` | – | comma-separated domains or addresses approved at the relay. Only these verify as sending identities |
| `SMTP_DAILY_LIMIT` | `0` (none) | rolling 24-hour cap across all workspaces. Broadcasts idle at 90 %, transactional mail continues to 100 % |
| `SMTP_MONTHLY_LIMIT` | `0` (none) | calendar-month cap; sending stops when it is reached |

### Amazon SES (required when `SENDER_DRIVER=ses`)

| Variable | Default | Description |
|---|---|---|
| `AWS_REGION` | – | SES and SNS region, e.g. `us-east-1` |
| `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` | empty | optional. Leave both empty to use the default AWS credential chain (profile, SSO, instance or task role) |
| `SES_CONFIGURATION_SET` | – | configuration set attached to every send (`sesbootstrap` creates it) |
| `SNS_TOPIC_NAME` | `forkemail-ses-events` | topic name used by `sesbootstrap` |
| `SNS_TOPIC_ARN` | – | printed by `sesbootstrap`. The webhook rejects messages from any other topic |
| `SES_TEST_EMAIL` | empty | development seed only: an address already verified in SES, added to the Forke demo workspace |

### Rate limits

| Variable | Default | Description |
|---|---|---|
| `API_PAID_RATE_PER_SEC` / `API_PAID_BURST` | `20` / `40` | per-key limit for paid workspaces and dashboard sessions |
| `API_TRIAL_RATE_PER_SEC` / `API_TRIAL_BURST` | `2` / `5` | per-key limit for trial workspaces |
| `PUBLIC_RATE_PER_SEC` / `PUBLIC_BURST` | `10` / `30` | per-IP limit for public routes (login, unsubscribe, tracking) |
| `AUTH_FAILURES_PER_MIN` / `AUTH_FAILURE_BURST` | `10` / `10` | per-IP limit on failed logins and invalid API keys |

## Using the dashboard

| Page | What it does |
|---|---|
| **Overview** | get-started checklist, key numbers, recent emails, and whether you are on the log sender, SES sandbox or SES production |
| **Campaigns** | create a campaign, then write emails inside it. The editor has sender, audience, HTML and text content, and a live preview. `{{first_name}}` and `{{email}}` are merge fields; the unsubscribe footer is added automatically |
| **Email report** | live progress and per-recipient status. Delivered, bounced, complained and unsubscribed counts come from real SES events. Pause and resume are here |
| **Transactional** | one-to-one API messages: dispatch latency (p50/p95/p99), delivery and bounce counts, opt-in open and click rates, a per-message event timeline, and a **Send test** button with a password-reset template |
| **Contacts** | import CSV or pasted rows with tags, search, filter by status or tag, bulk tag, delete |
| **Suppressions** | every address that is never emailed, with reason and source. Add one, paste a list or upload a CSV. Removing a hard bounce or complaint shows a warning first |
| **Activity** | the workspace event log (queued, sent, delivered, bounced, complained, rejected, …) |
| **Domains** | add a domain (shows DNS records with copy buttons) or a single address, verify, and remove |
| **API Keys** | create, rotate and revoke keys. Your password is required, and each key is shown once |
| **Settings** | plan and usage, verified sending identities, account details |

Every email's from address must use one of the workspace's **verified** identities. For a domain, that is any address on it; for a single email identity, exactly that address. This is enforced when an email is created, when it is updated and when it is sent.

## REST API

Authenticate with `Authorization: Bearer fm_live_…`. Errors are JSON of the form `{"error":"code"}`.

| Scope | Can do |
|---|---|
| `super_admin` session | everything, including managing API keys |
| `admin` session | everything except API keys, adding or removing domains, managing webhooks and removing suppressions |
| `full_access` key | everything except managing API keys |
| `sending_access` key | import contacts, list campaigns, create emails in a campaign, send, read email status and reports |

Any route not in the scope table returns 403 or 404. The full table is in `internal/auth/scopes.go`.

```sh
K="Authorization: Bearer fm_live_..."
B=http://localhost:8080

curl -H "$K" -F file=@samples/contacts.csv -F tags=launch $B/api/contacts/import
curl -H "$K" -H 'Content-Type: application/json' -d '{"name":"Product updates"}' $B/api/campaigns
curl -H "$K" -H 'Content-Type: application/json' $B/api/campaigns/<campaign_id>/emails -d '{
  "subject": "Hi {{first_name}}",
  "from_name": "Forke",
  "from_address": "news@your-verified-domain.com",
  "html": "<p>Hello {{first_name}}</p>",
  "text": "Hello {{first_name}}",
  "audience": {"all": false, "tag_ids": ["<tag_id>"], "contact_ids": []}
}'
curl -H "$K" -X POST $B/api/emails/<email_id>/send
curl -H "$K" $B/api/emails/<email_id>/report
```

| Status | Meaning |
|---|---|
| `402` | trial quota too small for this audience: `{"error":"trial_quota_exceeded","remaining":…,"required":…}` |
| `409 workspace_sending_paused` | the reputation guard has paused the workspace |
| `422 from_domain_not_verified` | the from address is not on a verified identity |
| `429` | rate limited; honour `Retry-After` |

Other routes:

| Route | Purpose |
|---|---|
| `GET /api/me` | identity, plan, quota, rate limits, verified identities and sending pause state |
| `GET /api/emails/{id}/recipients` | per-recipient status, SES message ID and outcome timestamps |
| `GET /api/emails/{id}/messages` | rendered messages (log driver) |
| `GET /api/events` | workspace event log |
| `/api/domains`, `/api/suppressions` | dashboard and `full_access` keys only |
| `GET /healthz` | database health for load balancers |

## Sign-in and roles

`POST /api/auth/login` with `{"email": "...", "password": "..."}`. The `email` field also accepts a Forke username.

1. The admin is looked up in Forke's `admins` table and the password is checked with bcrypt (`$2a$`/`$2b$` hashes from bcryptjs work as they are).
2. Only admins with `role` `super_admin` or `admin`, `is_disabled = false` and a password set (accepted invitation) can sign in.
3. On first sign-in a local user linked by `forke_admin_id` is created in the Forke workspace; later sign-ins refresh name, email and role.
4. Forke Mail issues its own random session token (stored hashed, `HttpOnly`, 14 days). It never trusts Forke's `admin_token` cookie.
5. Every request with a session re-reads the admin's role from Forke. Demoting an admin takes effect on the next request, and disabling one ends all their sessions.

Creating, rotating or revoking API keys asks for the Forke password again.

## Switching Forke from Resend

Forke's apps (`forke-admin`, `forke-dashboard`, `forke-marketing`) call Resend's REST API with `fetch`. Forke Mail serves the same calls under `/api/resend`:

| Resend call used by Forke | Forke Mail behaviour |
|---|---|
| `POST /emails` | transactional send: one `to` address, optional `reply_to`, `tags`, `Idempotency-Key` header. Returns `{"id"}` |
| `GET /emails/{id}` | message with `last_event` (`queued`, `sent`, `delivered`, `bounced`, …) |
| `GET /audiences`, `POST /audiences` | audiences are contact tags. Creating an existing name returns the existing audience |
| `POST /audiences/{id}/contacts` | upserts the contact and tags it. `unsubscribed: true` suppresses it. Unsubscribed, bounced or complained contacts are never reactivated |
| `POST /broadcasts` | creates a draft email for the audience inside a campaign named after the broadcast `name` |
| `POST /broadcasts/{id}/send` | launches it; the worker sends to every active, unsuppressed contact in the audience |

`{{{RESEND_UNSUBSCRIBE_URL}}}` (and `{{unsubscribe_url}}`) in a broadcast's HTML or text becomes the recipient's signed unsubscribe link, and the one-click `List-Unsubscribe` headers are added. Errors keep Resend's shape: `{"statusCode": 422, "name": "validation_error", "message": "..."}`, and `429` keeps `Retry-After`.

Not supported yet, and rejected with a clear 422 instead of being silently dropped: more than one `to`, `cc`/`bcc`, attachments, custom headers and `scheduled_at`.

### Steps

1. Deploy Forke Mail at `https://mail.forke.space` (see [Deploying to production](#deploying-to-production)).
2. Sign in as a `super_admin`, add the sending domain (`forke.space`) and create a `full_access` API key.
3. In each Forke app, point the two Resend URLs at Forke Mail and swap the key:

   ```ts
   const RESEND_API = process.env.FORKE_MAIL_API_URL || 'https://api.resend.com'
   // and in the single-send helper:
   const res = await fetch(`${RESEND_API}/emails`, { ... })
   ```

   ```sh
   FORKE_MAIL_API_URL=https://mail.forke.space/api/resend
   RESEND_API_KEY=fm_live_...
   ```

4. Remove `RESEND_AUDIENCE_ID`, or set it to the Forke Mail audience id. With it unset, the apps find or create "Forke Subscribers" by name, as they already do.

The apps' existing retry logic (`429` with `Retry-After`, `5xx`) works unchanged.

## Sending through OCI Email Delivery

Outbound port 25 is blocked on Oracle Cloud, but OCI Email Delivery accepts authenticated SMTP on port 587, and the Always Free tier includes a monthly allowance (check the current number in the OCI console; it has been 3,000 a month).

1. **OCI console → Developer Services → Email Delivery.** Create an *email domain* for `forke.space`, add its DKIM record, and add the SPF include OCI shows to Cloudflare DNS.
2. Add **approved senders** for every from address you use (`hello@`, `blog@`, `updates@`…).
3. Create **SMTP credentials** for an IAM user (Identity → Users → SMTP credentials).
4. Configure Forke Mail:

   ```sh
   SENDER_DRIVER=smtp
   SMTP_HOST=smtp.email.ap-mumbai-1.oci.oraclecloud.com
   SMTP_PORT=587
   SMTP_TLS=starttls
   SMTP_USERNAME=<ocid smtp username>
   SMTP_PASSWORD=<smtp password>
   SMTP_ALLOWED_SENDERS=forke.space
   SMTP_DAILY_LIMIT=100
   SMTP_MONTHLY_LIMIT=3000
   ```

5. Set an OCI budget alert. On a pay-as-you-go tenancy, mail over the free allowance is billed, and the limits above are what keep Forke Mail under it.

What the SMTP driver does and doesn't know:

- **Status.** A message is `sent` once the relay accepts it. Delivery, bounce and complaint events need provider feedback, and OCI Email Delivery doesn't push events the way SES/SNS does, so those counters stay empty with this driver.
- **Bounces.** OCI keeps its own suppression list for hard bounces and complaints, so repeat sends to bad addresses are dropped at the relay.
- **Unsubscribes** are fully handled by Forke Mail.
- **Errors.** `4xx` replies are retried with backoff, `5xx` fails the message, and an authentication failure pauses sending for 10 minutes. A connection dropped after the message was handed over is marked `unknown_outcome` and never retried, so nobody gets a duplicate.
- **Message IDs.** Every message carries `X-Forke-*` headers (workspace, email or message and recipient ids), and its `Message-ID` is stored, ready for bounce processing.

## Transactional email

Transactional messages are one-to-one emails triggered by something a user did: sign-in codes, password resets, receipts and security alerts. They use their own table, queue and dispatcher. That keeps them separate from campaigns, but they share the SES account, the feedback webhook, suppressions and the reputation guard.

### Sending one

```sh
curl -X POST $B/api/transactional \
  -H "Authorization: Bearer fm_live_..." \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: password-reset-user-42-1730000000" \
  -d '{
    "to": "user@example.com",
    "from_name": "Forke",
    "from_address": "no-reply@your-verified-domain.com",
    "reply_to": "support@your-verified-domain.com",
    "subject": "Your password reset code",
    "html": "<p>Your code is <b>482913</b></p>",
    "text": "Your code is 482913",
    "tags": {"category": "password_reset"},
    "track_opens": false,
    "track_clicks": false
  }'
```

| Field | Notes |
|---|---|
| `to` | exactly one recipient |
| `from_address` | must be on a verified identity, as for campaigns |
| `from_name`, `reply_to` | optional |
| `subject` | required, up to 300 characters |
| `html`, `text` | at least one is required, each up to 256 KB. Nothing is added to them: no unsubscribe footer and no `List-Unsubscribe` header |
| `tags` | optional, up to 10 `key: value` pairs of letters, digits, `-` and `_` |
| `track_opens`, `track_clicks` | off by default. Both need an HTML body |

Responses:

| Status | Meaning |
|---|---|
| `202` | accepted and queued; the body is the message, with `status: "queued"` |
| `200` + `Idempotent-Replayed: true` | a repeat of an earlier request. The body is the **original** message, and `replayed` is `idempotency_key` or `duplicate_content` |
| `409 idempotency_key_reused` | that key was already used in the last 24 hours with a different request body |
| `422 recipient_suppressed` | the address hard bounced, complained or was suppressed manually |
| `422 from_domain_not_verified`, `409 workspace_sending_paused`, `402 trial_quota_exceeded`, `429 daily_limit_reached` | as for campaigns |

Read it back with `GET /api/transactional/{id}`. That works with any key scope and includes the event timeline. `GET /api/transactional` (filter with `?status=`) and `GET /api/transactional/stats` need a dashboard session or a `full_access` key.

### Fast dispatch and priority

- **Instant send.** Once the request's transaction commits, the handler passes the message ID straight to an in-process dispatcher with 8 workers, so it is normally handed to SES within milliseconds. A 1-second poller picks up anything the in-process hand-off missed: retries, a full hand-off buffer, or rows left by a restart.
- **Priority over broadcasts.** Both paths share one global SES rate limiter. Transactional sends reserve a token and wait for it. Broadcast lanes only take a token when one is free *and* no transactional send is waiting, so a busy campaign never delays a password reset by more than one token interval.
- **Quota headroom.** Broadcasts stop at 90% of the SES 24-hour quota. Transactional sends may use the last 10%, so a big campaign cannot leave users unable to sign in.
- **Latency measurement.** `dispatch_ms` is the time from acceptance to SES returning a message ID. Each `sent` event also records `send_ms`, the SES API call alone. `/api/transactional/stats` reports p50, p95, p99 and max over the last 24 hours, and the dashboard shows them.

### Idempotency and deduplication

- **`Idempotency-Key` header (recommended).** A repeat within 24 hours with the same body returns the original message and sends nothing. A different body returns `409`. The key is per workspace, up to 256 printable ASCII characters. Concurrent requests with the same key are serialized with a Postgres advisory lock, so exactly one message is created.
- **Content dedupe (no key).** Without a key, an identical message (same recipient, sender, reply-to, subject and bodies) accepted in the last 60 seconds and not failed is returned instead of being sent again. This catches double-clicked "resend code" buttons and client retries. To send identical content twice on purpose, give each request its own key.
- **Never a duplicate send.** As with campaigns, an SES call with no answer (a timeout or network error) is marked `failed` with `unknown_outcome` and never retried. A message stuck in `sending` for 5 minutes, for example after a crash, is also failed rather than resent.
- **Expiry.** A message still waiting after an hour (for example while SES sending is paused) fails with `expired` instead of delivering a stale code.

### Suppressions and feedback

Hard bounces, complaints and manual suppressions block transactional sends. **Unsubscribes do not**: unsubscribing from marketing must not stop password resets. SES events reach transactional messages through the same `/webhooks/ses` endpoint (they are matched by SES message ID and checked against the `workspace_id`/`message_id` tags). They set `delivered_at`, `bounced_at` and `complained_at`, suppress bounced and complaining addresses, and count towards the 30-day reputation guard.

### Open and click tracking, and its limits

Tracking is opt-in per message. Leave it off for codes and security alerts.

- **Opens.** A 1×1 image is added before `</body>` at send time. The stored HTML is never modified. Treat open counts as estimates:
  - Apple Mail Privacy Protection preloads images, so it produces opens nobody made.
  - Many clients block images, so real opens go missing.
  - Known image proxies, link scanners and requests with no user agent are ignored and not counted.
- **Clicks.** Every `http(s)` link in the HTML is rewritten to `/t/c/{token}`, which records the click and redirects with `302`. The token is HMAC-signed over the message ID and the exact URL, so it cannot be used as an open redirect.
- **Privacy.** No IP addresses or user agents are stored, only counts, first-seen times and the clicked URL.
- **Secret.** Tracking tokens use a key derived from `UNSUBSCRIBE_SECRET`. Changing that secret breaks tracking links in mail already sent.

## Sending real email with Amazon SES

This section takes you from a fresh AWS account to real email with live feedback events on your machine. In production, replace the ngrok URL with your real domain.

### 1. Create an IAM user and policies

In the AWS console, go to **IAM → Users → Create user** (for example `forkemail-app`, programmatic access only), then create an access key for it. Attach these two inline policies, replacing `REGION` and `ACCOUNT_ID`.

**Runtime policy.** Keep this one permanently:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "Send",
      "Effect": "Allow",
      "Action": "ses:SendEmail",
      "Resource": [
        "arn:aws:ses:REGION:ACCOUNT_ID:identity/*",
        "arn:aws:ses:REGION:ACCOUNT_ID:configuration-set/forkemail"
      ]
    },
    {
      "Sid": "Identities",
      "Effect": "Allow",
      "Action": ["ses:CreateEmailIdentity", "ses:GetEmailIdentity", "ses:DeleteEmailIdentity"],
      "Resource": "arn:aws:ses:REGION:ACCOUNT_ID:identity/*"
    },
    {
      "Sid": "Account",
      "Effect": "Allow",
      "Action": "ses:GetAccount",
      "Resource": "*"
    }
  ]
}
```

**Bootstrap policy.** This one is also used by `sescheck`. Attach it only while running `sesbootstrap`, then remove it:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ConfigurationSet",
      "Effect": "Allow",
      "Action": [
        "ses:GetConfigurationSet",
        "ses:CreateConfigurationSet",
        "ses:CreateConfigurationSetEventDestination",
        "ses:UpdateConfigurationSetEventDestination"
      ],
      "Resource": "arn:aws:ses:REGION:ACCOUNT_ID:configuration-set/forkemail"
    },
    {
      "Sid": "Account",
      "Effect": "Allow",
      "Action": ["ses:PutAccountSuppressionAttributes", "ses:GetAccount", "ses:ListEmailIdentities"],
      "Resource": "*"
    },
    {
      "Sid": "Topic",
      "Effect": "Allow",
      "Action": [
        "sns:CreateTopic",
        "sns:GetTopicAttributes",
        "sns:SetTopicAttributes",
        "sns:ListSubscriptionsByTopic",
        "sns:Subscribe",
        "sns:Unsubscribe"
      ],
      "Resource": "arn:aws:sns:REGION:ACCOUNT_ID:forkemail-ses-events"
    }
  ]
}
```

If you changed `SES_CONFIGURATION_SET` or `SNS_TOPIC_NAME`, change the names in these ARNs to match.

### 2. Check your credentials

Add the AWS settings to `.env`:

```sh
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=AKIA...
AWS_SECRET_ACCESS_KEY=...
SES_CONFIGURATION_SET=forkemail
```

Then run:

```sh
set -a && . ./.env && set +a && go run ./cmd/sescheck
```

It prints:
- the region and account;
- whether you are in sandbox or production;
- the sending rate and 24-hour quota;
- your verified identities;
- whether the configuration set and topic exist.

### 3. Expose your machine with ngrok

SNS can only deliver to a public https URL:

```sh
ngrok http 8080
```

Copy the `https://….ngrok-free.app` URL. On the free plan it changes every time ngrok restarts; a free static domain (`ngrok http --url=<name>.ngrok-free.app 8080`) keeps it stable. If ngrok fails with `ERR_NGROK_121`, run `ngrok update`.

Set `PUBLIC_BASE_URL` in `.env` to that URL, with no trailing slash.

### 4. Bootstrap the SES and SNS resources

```sh
set -a && . ./.env && set +a && go run ./cmd/sesbootstrap
```

`sesbootstrap` is idempotent. On each run it:
1. creates the SES configuration set if it is missing;
2. creates the SNS topic if it is missing;
3. sets the topic policy so only `ses.amazonaws.com` can publish to it, and only for this account's configuration set;
4. creates or updates the configuration set's event destination for `Send`, `Delivery`, `Bounce`, `Complaint`, `Reject` and `DeliveryDelay`;
5. enables account-level suppression for `BOUNCE` and `COMPLAINT`;
6. removes https subscriptions that point at old URLs (stale ngrok tunnels);
7. subscribes `PUBLIC_BASE_URL/webhooks/ses` and waits for confirmation.

On the first run it prints `SNS_TOPIC_ARN=…`. Put that line in `.env`.

### 5. Switch the server to SES and subscribe the webhook

In `.env` set `SENDER_DRIVER=ses`, then restart the server and run bootstrap once more:

```sh
set -a && . ./.env && set +a && go run ./cmd/server          # terminal 1
set -a && . ./.env && set +a && go run ./cmd/sesbootstrap    # terminal 2
```

The second bootstrap subscribes the webhook. SNS sends a `SubscriptionConfirmation`, the server verifies its signature and confirms it automatically, and bootstrap prints `subscription confirmed`. Now remove the bootstrap policy from the IAM user.

With the SES driver:
- domains are no longer verified instantly the way the log driver does it;
- `/dev/seed` creates no fake domains, except that `SES_TEST_EMAIL`, if set, is added to the Forke workspace;
- browse the app through your `https://` URL, because session cookies are now marked `Secure`.

### 6. Verify a sending identity

On the **Domains** page, choose one of:

- **Domain** (recommended). Enter `yourdomain.com`.
  - Publish the returned DKIM `CNAME`s and review DMARC guidance. Default SES MAIL FROM already uses SES SPF. A custom MAIL FROM subdomain needs separate provider configuration plus its own regional MX and SPF records; `/api/domains/{id}/dns?mail_from=...` is a DNS-readiness preview only.
  - The server asks SES for the status every 60 seconds; you can also click **Verify**. SES runs its own DNS checks, usually within minutes but sometimes hours, and up to 72 hours.
  - Once verified, you can send from any address on the domain.
- **Single email address** (fastest for testing). SES emails the address a link; click it, then press **Verify**. You can then send from exactly that address.

DNS gotchas:
- A domain may have **only one SPF record**. If one already exists (for example from your mailbox provider), merge them into a single record: `v=spf1 include:_spf.yourmailhost.com include:amazonses.com ~all`.
- Likewise, keep only one `_dmarc` record.
- Some DNS providers append the domain to record names automatically. With those, enter `xxxx._domainkey`, not `xxxx._domainkey.yourdomain.com`.

### 7. The SES sandbox

New SES accounts start in the **sandbox**:
- you can send only to verified identities and the [mailbox simulator](https://docs.aws.amazon.com/ses/latest/dg/send-an-email-from-console.html) addresses (`success@`, `bounce@`, `complaint@` and `suppressionlist@simulator.amazonses.com`);
- the limits are about 200 emails per day at 1 per second.

Sends to unverified addresses fail with `message_rejected: Email address is not verified`. That is expected, and it never suppresses the address. Request production access in the SES console under **Account dashboard → Request production access**. The Overview page shows which mode you are in.

A good first test:
1. Verify your own address.
2. Add it as a contact.
3. Send to it plus the three simulator contacts.
4. Within seconds the report should show your address and `success@` delivered, `bounce@` bounced and `complaint@` complained, and Suppressions should list the bounce and the complaint.

### How the server handles SES

| SES response | What happens |
|---|---|
| success | message ID stored, recipient marked `sent`, a `sent` event recorded |
| throttling or `LimitExceeded`, or a 5xx with a response | recipient returns to pending, with backoff; failed after 5 attempts |
| `SendingPaused` / `AccountSuspended` | email paused (`ses_sending_paused`); the worker stops claiming for 10 minutes |
| `MailFromDomainNotVerified` / identity `NotFound` | recipient failed; email paused (`identity_not_verified`) |
| `MessageRejected` (including sandbox "not verified") | recipient failed with the reason; **no suppression** |
| network error or timeout (outcome unknown) | recipient failed `unknown_outcome`, **never retried**, to avoid sending twice |

Other behaviour:
- SDK retries are disabled for `SendEmail`.
- `GetAccount` runs at startup and every 10 minutes. `MaxSendRate` becomes the global rate, each workspace gets at most 70% of it so transactional mail keeps headroom, and the worker idles once `SentLast24Hours` reaches 90% of `Max24HourSend`.

| SES event | Effect |
|---|---|
| `Delivery` | `delivered_at` set, `delivered` event recorded |
| `Bounce` with type `Permanent` (including `OnAccountSuppressionList`) | `bounced_at` set, contact marked `bounced`, address suppressed (`ses_bounce`), reputation re-evaluated |
| `Bounce` with type `Transient` or `Undetermined` | `soft_bounced` event only |
| `Complaint` | `complained_at` set, contact marked `complained`, address suppressed (`ses_complaint`), reputation re-evaluated |
| `Reject` | recipient failed with the reason |
| `DeliveryDelay` | `delayed` event only |
| `Send` or other types | ignored |

Every webhook message is checked before it is trusted:
- the signing certificate must come from `https://sns.<AWS_REGION>.amazonaws.com/….pem`;
- the signature must verify;
- `TopicArn` must equal `SNS_TOPIC_ARN`.

Anything else gets a bare `403`. Notifications are deduplicated by SNS message ID in the same transaction as their processing. Events for unknown message IDs, or whose `workspace_id`/`email_id`/`recipient_id` tags don't match the recipient, return `200` and are ignored, so SNS does not retry them forever.

### Reputation guard

After every hard bounce or complaint, the workspace's 30-day rates are recomputed over the recipients it sent to. If the sample is at least `REPUTATION_MIN_SAMPLE` and a rate exceeds its threshold:
- the workspace is marked `sending_paused`;
- its sending emails are paused with reason `reputation`;
- a `sending_paused` event is recorded.

Launching and the worker both refuse paused workspaces, while reads keep working. The dashboard shows a banner with the reason. The rule applies to every workspace. An operator clears the pause after cleaning up the contact list:

```sql
UPDATE workspaces SET sending_paused = false, pause_reason = NULL WHERE id = '<workspace_id>';
```

## Testing SES events without AWS

With `APP_ENV=development`, `POST /dev/ses-event` runs a raw SES event JSON through the **same processor** as the webhook, skipping only the signature check. Get a `ses_message_id` from `GET /api/emails/<id>/recipients`; log-driver IDs start with `log-`.

```sh
E() { curl -s -X POST localhost:8080/dev/ses-event -H "X-Seed-Token: $SEED_TOKEN" -d "$1"; echo; }
M=log-...

E '{"eventType":"Delivery","mail":{"messageId":"'$M'","timestamp":"2026-11-07T10:00:00Z"},"delivery":{"timestamp":"2026-11-07T10:00:01Z","recipients":["ada@example.com"],"smtpResponse":"250 2.0.0 OK"}}'
E '{"eventType":"Bounce","mail":{"messageId":"'$M'","timestamp":"2026-11-07T10:00:00Z"},"bounce":{"bounceType":"Permanent","bounceSubType":"General","bouncedRecipients":[{"emailAddress":"ada@example.com","diagnosticCode":"smtp; 550 5.1.1 user unknown"}],"timestamp":"2026-11-07T10:00:01Z"}}'
E '{"eventType":"Bounce","mail":{"messageId":"'$M'","timestamp":"2026-11-07T10:00:00Z"},"bounce":{"bounceType":"Transient","bounceSubType":"MailboxFull","bouncedRecipients":[{"emailAddress":"ada@example.com"}],"timestamp":"2026-11-07T10:00:01Z"}}'
E '{"eventType":"Complaint","mail":{"messageId":"'$M'","timestamp":"2026-11-07T10:00:00Z"},"complaint":{"complainedRecipients":[{"emailAddress":"ada@example.com"}],"complaintFeedbackType":"abuse","timestamp":"2026-11-07T10:00:01Z"}}'
E '{"eventType":"Reject","mail":{"messageId":"'$M'","timestamp":"2026-11-07T10:00:00Z"},"reject":{"reason":"Bad content"}}'
```

The response is `processed`, `ignored`, `unknown_message` or `tag_mismatch`. With the log driver, sending to `bounce@simulator.amazonses.com`, `complaint@simulator.amazonses.com` or `suppressionlist@simulator.amazonses.com` produces those events automatically.

## Running the tests

```sh
docker compose up -d     # the forkemail_test database is created on first start
TEST_DATABASE_URL="postgres://forkemail:forkemail@localhost:5433/forkemail_test?sslmode=disable" go test ./...
```

Without `TEST_DATABASE_URL`, the database-backed tests are skipped. If your Postgres volume predates `docker/initdb.sql`, create the database once with `docker compose exec postgres createdb -U forkemail forkemail_test`.

The suite covers:
- SNS signature verification, with a locally generated certificate and both signature versions;
- subscription URL checks;
- SES error classification, and the rule that no API error ever suppresses an address;
- event processing and deduplication;
- the reputation guard;
- domain rules and DNS record generation;
- suppression checks at launch, before send and on import;
- concurrent claiming and the trial quota under concurrency;
- the unsubscribe token and scopes;
- configuration validation.

CI (`.github/workflows/ci.yml`) runs `gofmt`, `go vet`, the dashboard build and the full test suite against PostgreSQL on every push and pull request.

## Deploying to production

### Docker image

```sh
docker build -t forkemail .
docker run -p 8080:8080 --env-file .env.production forkemail
```

The multi-stage image builds the dashboard, compiles static binaries and ships them on `distroless/static:nonroot`. The CLI tools are in the image too:

```sh
docker run --rm --env-file .env.production --entrypoint /app/sescheck forkemail
docker run --rm --env-file .env.production --entrypoint /app/sesbootstrap forkemail
```

### Checklist

- `APP_ENV=production`, which disables the `/dev/*` routes.
- `PUBLIC_BASE_URL=https://mail.forke.space`, served over TLS by nginx (see `deploy/`).
- `FORKE_DATABASE_URL` pointing at Forke's database with a read-only user that can `SELECT` from `admins`.
- `TRUSTED_PROXY` set to the proxy's IP or CIDR, so per-IP rate limits see real client addresses.
- A managed PostgreSQL with `sslmode=require` in `DATABASE_URL`. Migrations run automatically on startup.
- Strong, unique `UNSUBSCRIBE_SECRET`, and keep it stable.
- With `SENDER_DRIVER=smtp`: relay credentials, `SMTP_ALLOWED_SENDERS` and the daily and monthly caps set (see [Sending through OCI Email Delivery](#sending-through-oci-email-delivery)). The SES items below apply only to `SENDER_DRIVER=ses`.
- Prefer an IAM role (ECS task role, EC2 instance profile) over static keys: leave `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` empty and the default credential chain is used. Attach only the runtime policy.
- Run `sesbootstrap` once against the production `PUBLIC_BASE_URL`, with the bootstrap policy attached temporarily.
- SES production access requested and granted.
- Health check: `GET /healthz` returns `200 {"status":"ok"}`, or `503` when the database is unreachable.
- The server shuts down gracefully on `SIGTERM`, finishing in-flight requests and sends.

### Running more than one instance

Claiming uses `FOR UPDATE SKIP LOCKED`, so several instances never send to the same recipient or transactional message twice. Transactional priority over broadcasts holds only within one instance. However, the send rate limiter and the API rate limiters are held in each instance's memory. With N instances, SES could see up to N × `MaxSendRate`, and API limits apply per instance. Run a single instance for sending, or lower the SES-derived rate accordingly.

## Project layout

```
cmd/
  server/          HTTP server, routing, background workers
  sesbootstrap/    idempotent SES/SNS setup
  sescheck/        prints SES account state
internal/
  accounts/        Forke admin login, local dev login, logout, workspace and user creation
  forke/           read-only access to Forke's admins table (password check, live role)
  relay/           SMTP relay sender, MIME builder, quota guard, allowed-sender identities
  resendapi/       Resend-compatible endpoints translated onto the native handlers
  auth/            API keys, sessions, authentication middleware, route scopes
  campaigns/       campaign CRUD
  config/          environment parsing and validation
  contacts/        contacts, CSV import, tags, suppressions
  db/              pool, migrations, health check, test helpers (dbtest)
  domains/         sending identities, DNS records, verification poller
  emails/          email CRUD, audiences, launch, pause/resume, reports
  events/          event log, dashboard
  feedback/        SES event processor, SNS webhook, dev endpoint, reputation guard
  httpx/           JSON helpers, security headers, logging, request limits
  keys/            API key management
  ratelimit/       token-bucket stores, per-IP and per-key middleware, failure guard
  seed/            development seed
  sender/          worker, claiming, rendering, log sender, limits, sweeper
  ses/             SESv2 sender, error classification, account monitor, identities
  sns/             SNS message signature verification and subscription confirmation
  transactional/   one-to-one API sends: idempotency, dedupe, priority dispatcher, open/click tracking
  unsub/           signed unsubscribe tokens and pages
  workspaces/      /api/me, quota reservation, send rates
migrations/        goose SQL migrations (embedded)
frontend/          Next.js dashboard (pages router, static export, embedded via frontend.go)
samples/           contacts.csv for trying imports
docker/            PostgreSQL init script
```

`AGENTS.md` describes the architecture, invariants and conventions in depth, for contributors and AI coding agents.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| Server exits with a list of config errors | fix the named variables; nothing starts until the configuration is valid |
| `ses get account` error at startup | wrong credentials, wrong region, or the runtime policy is missing `ses:GetAccount` |
| Domain stays **Pending** | check the records with `dig +short CNAME <token>._domainkey.<domain>`. If they resolve, SES has not rechecked yet: wait (up to 72 hours) or use a single email identity meanwhile |
| `Amazon SES rejected the request` when adding a domain | the IAM user is missing `ses:CreateEmailIdentity` |
| Recipients fail with `Email address is not verified` | SES sandbox: verify the recipient or request production access |
| `sesbootstrap` says the subscription is still pending | the server isn't running with this `SNS_TOPIC_ARN`, or isn't reachable at `PUBLIC_BASE_URL/webhooks/ses` |
| Logged out immediately on `http://localhost` with the SES driver | session cookies are `Secure` when `PUBLIC_BASE_URL` is https; browse through the https URL |
| ngrok `ERR_NGROK_121` | the agent is too old; run `ngrok update` |
| ngrok URL changed | update `PUBLIC_BASE_URL`, restart the server, run `sesbootstrap` again |
| `workspace_sending_paused` | the reputation guard tripped; see [Reputation guard](#reputation-guard) |

## Security model

- **Tenant isolation.** Every query is scoped by `workspace_id`; cross-tenant IDs return 404.
- **Passwords.** Forke Mail stores no passwords for Forke admins; they are checked against Forke's bcrypt hashes. Login timing is equalized for unknown accounts.
- **Sessions.** `HttpOnly`, `SameSite=Lax`, `Secure` over https, 14-day lifetime, stored hashed. Mutating session requests must be same-origin (`Sec-Fetch-Site` or `Origin`).
- **API keys.** Shown once, stored as SHA-256 hashes with a 12-character display prefix. Creating, rotating and revoking keys requires a `super_admin` session and the account password.
- **Unsubscribe links.** HMAC-SHA256 signed tokens, with one-click `List-Unsubscribe-Post` support.
- **SNS webhook.** Authenticated only by AWS's signature, with a certificate host allowlist, a topic check and dedupe.
- **Strict CSP.** No inline scripts, `frame-ancestors 'none'`, `nosniff`, and `no-store` on API responses.
- **Request limits.** Request body limits, server timeouts, per-IP and per-key rate limits, and lockout on failed authentication.
- **Logging.** Secrets are never logged or returned. Logs carry the route pattern, workspace ID and key prefix only, never authorization headers, keys, passwords or request bodies.

## Credits

Forke Mail was built by the Forke backend interns:

- **Advik Gupta**: the platform: workspaces, API keys, contacts, campaigns, the sending worker, SES and SNS integration, suppressions, the reputation guard and the dashboard.
- **Diya Gupta**: the transactional email engine: priority dispatch, idempotency, open and click tracking and the transactional dashboard.
- **Krishna Agarwal**: the developer platform: signed customer webhooks with SSRF protection, DNS diagnostics, API key lifecycle tests, the OpenAPI contract and CI.

Their original branches are preserved in this repository's history.
