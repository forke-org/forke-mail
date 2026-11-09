# AGENTS.md

This file gives AI coding agents and new contributors the context they need without reading the whole codebase. `README.md` covers setup and operation for humans; this file covers architecture, invariants and conventions. Keep it accurate when you change behaviour.

## What this is

Forke Mail is a multi-tenant broadcast and transactional email service.

- **Tenancy.** A workspace (tenant) has users, API keys, contacts, tags, suppressions, sending identities, campaigns, and emails inside campaigns.
- **Launch.** Launching an email snapshots its audience into per-recipient rows.
- **Sending.** A background worker sends each recipient through a `Sender`: `log` locally, Amazon SES in production.
- **Feedback.** SES delivery, bounce and complaint events arrive over SNS at `/webhooks/ses` and update recipients, suppressions and workspace reputation.

The whole system is one Go binary: API, embedded Next.js static dashboard, unsubscribe pages, webhook and background workers. PostgreSQL is the only stateful dependency.

## Commands

```sh
docker compose up -d                                   # Postgres 17 on :5433 (db forkemail + forkemail_test)
(cd frontend && npm ci && npm run build)               # static export to frontend/out (embedded by frontend/frontend.go)
set -a && . ./.env && set +a && go run ./cmd/server    # migrations run on startup
cd frontend && npm run dev                             # dashboard dev server :3000, proxies /api to :8080

TEST_DATABASE_URL="postgres://forkemail:forkemail@localhost:5433/forkemail_test?sslmode=disable" go test ./...
gofmt -l cmd internal migrations frontend              # must print nothing
go vet ./...

go run ./cmd/sescheck                                  # SES account/identity/config-set/topic status
go run ./cmd/sesbootstrap                              # idempotent SES config set + SNS topic + webhook subscription

curl -X POST "localhost:8080/dev/seed?reset=true" -H "X-Seed-Token: $SEED_TOKEN"     # dev only, wipes data
curl -X POST localhost:8080/dev/ses-event -H "X-Seed-Token: $SEED_TOKEN" -d '<SES event JSON>'  # dev only
```

DB-backed tests skip when TEST_DATABASE_URL is unset. Integration verification must use disposable local PostgreSQL, not shared, production or Diya-origin databases. Tests create workspaces and isolated migration schemas and clean them up.

## Hard rules (project conventions)

1. **No comments anywhere.** This covers `.go`, `.js`, `.css`, `.html` and `.sql`. The only exceptions are tool directives: `//go:embed` and goose's `-- +goose Up/Down`. Make code self-explanatory with names instead.
2. **Keep it simple.** Do not add an ORM, Redis, message brokers or new frameworks. Use plain SQL through pgx v5, chi for routing, `x/time/rate` for limiters, and goose for migrations.
3. **Never edit existing migrations.** Every schema change goes in a new `migrations/0000N_name.sql` with both Up and Down. Postgres auto-names constraints `<table>_<column>_check`; drop and re-add them by those names.
4. **Fix root causes.** No dead code, no speculative abstractions, small files, one concept per file. `deadcode -test ./...` must stay clean.
5. **Secrets stay private.** Never log credentials, authorization headers, session tokens or message bodies. API keys and customer webhook secrets have intentional one-time create/rotate responses with no-store; list endpoints never expose them. Keep development seed credentials private. The request logger records method, route pattern, status, duration, workspace_id and key_prefix only.
6. **Every query is tenant scoped.** Filter by `workspace_id`, and return 404 (never 403) for IDs belonging to another workspace.
7. **Every new authenticated route needs an entry in `internal/auth/scopes.go`.** Unlisted routes are denied.
8. Match the surrounding style. Errors are JSON `{"error":"snake_case_code"}` via `httpx.Error`, and unexpected errors go through `httpx.Internal`, which logs them and returns `internal_error`.

## Repository map

| Path | Responsibility |
|---|---|
| `cmd/server/main.go` | loads config, connects, migrates, picks the sender driver, starts background tasks (worker, sweeper, limiter janitors, and with SES the account monitor and identity poller), graceful shutdown |
| `cmd/server/routes.go` | router: middleware chain, authenticated API group, public group, `/healthz`, `/webhooks/ses` (only when SES is configured), `/dev/*` (development only) |
| `cmd/sesbootstrap`, `cmd/sescheck` | operator CLIs (see README) |
| `internal/config` | env parsing and validation (`Load`, `LoadAWS`, `ValidateBaseURL`). Startup fails on invalid config |
| `internal/accounts` | signup (creates a trial workspace, plus a sandbox domain only with the log driver), login (bcrypt, timing-equalized), logout, `CreateWorkspace`, `AddVerifiedIdentity`, `CreateUser` |
| `internal/auth` | `GenerateKey`/`HashKey` (`fm_live_` + 64 hex, SHA-256, 12-char prefix), sessions (`fm_session` cookie, hashed in `sessions`), `Authenticator.Middleware` (Bearer key first, then cookie; session mutations must pass `httpx.SameOrigin`), `Authorize` + `routeScopes` |
| `internal/httpx` | JSON encode/decode (strict, 1 MB), `PathID`/`ValidID`/`Page`, security headers + CSP, recover, logger, body limit, `SameOrigin`, `TokenMatches` (constant time) |
| `internal/ratelimit` | token-bucket `Store` with a janitor, per-key/per-IP middleware, `FailureGuard` for auth failures, `IPResolver` (`X-Forwarded-For` only from `TRUSTED_PROXY`) |
| `internal/contacts` | contact list/delete, CSV parse (`ParseCSV`, 50k-row cap), `Import` (never reactivates blocked or suppressed contacts; can attach tags and add contacts to a draft email's audience), tags, `Suppress`, suppression handlers |
| `internal/campaigns` | campaign CRUD (a campaign is a named group of emails) |
| `internal/emails` | email CRUD, `Audience` (all / tags / contacts), `audienceMatch` SQL shared by size and `Snapshot`, `checkDomain`, `send` (launch), pause/resume, report, recipients, messages |
| `internal/sender` | `Worker` (per-workspace lanes + global limiter), `Claim`/`Unclaim` (SKIP LOCKED), `process` (per-recipient send path), `Render` (merge fields, unsubscribe footer and headers), `LogSender` (+ `Simulate` synthetic SES events), `Limits`/idle logic, `RunSweeper` |
| `internal/ses` | `Sender` (SESv2 `SendEmail`, SDK retries off), `Classify` (SES error → `sender.Action`), `Account` (GetAccount every 10 min → `Worker.SetLimits`, `Mode()`), `Identities` (create/get/delete email identity) |
| `internal/sns` | `Message`, `StringToSign`, `Verifier.Verify` (cert URL allowlist `https://sns.<region>.amazonaws.com/…pem`, cert cache, SignatureVersion 1 = SHA1 and 2 = SHA256, topic check), `Verifier.Confirm`. `snstest` has the test signer |
| `internal/feedback` | `Event` (SES event JSON model), `Processor.Handle` (dedupe + apply in one transaction), reputation guard, `Webhook` handler, `DevHandler` |
| `internal/domains` | identity handlers, `ValidName`, `DNSRecords`, `Refresh`/`save` (SES status → local status), `RunPoller` (pending identities every 60 s), `Identities` interface + `Local` (log driver: instantly verified) |
| `internal/events` | `Record` (insert into `email_events`), activity list, dashboard (includes `sending_mode`) |
| `internal/workspaces` | `/api/me`, `Reserve`/`Release` (atomic trial and daily quota), `SendRate` |
| `internal/keys` | API key list/create/rotate/revoke (password required; trial workspaces cannot create `full_access`) |
| `internal/unsub` | `Signer` (HMAC token `base64url(ws:contact:email).base64url(mac)`, strict base64), confirm page + POST (one-click) |
| `internal/seed` | dev seed: Forke (paid, `full_access`), Acme Trial (100), Beta Trial (5) |
| `internal/db` | `Connect`, `Migrate` (goose provider over the embedded FS), `Health`, `Querier`; `dbtest` holds test helpers and fixtures |
| `migrations/` | 1 workspaces/users/sessions/keys/domains · 2 contacts/tags/suppressions · 3 campaigns/emails/audience/recipients · 4 logs/events/usage/endpoints · 5 SES/feedback · 6 customer webhook outbox · 7 legacy receipts · 8 retention · 9 transactional engine · 10 shared webhook payload · 11 shared admission and dispatch leases |
| `internal/transactional` | Service.Accept, native Handler, CompatibilityHandler, durable dispatcher, signed tracking and statistics. Same Sender/global gate as broadcasts |
| `frontend/` | Next.js 16 pages router, static export, React 19, TanStack Table v8. `lib/api.js` (fetch wrapper + error messages), `lib/session.js` (auth state, upgrade modal), `components/` (DataTable, Modal/Confirm, Layout, …), `pages/` (one page per route). Theme tokens live in `public/app.css`; `public/theme.js` sets the theme before paint |

## Data model (key columns only)

- **`workspaces`**
  - `plan` (`paid`|`trial`), `status`, `daily_limit`, `rate_limit_per_sec`, `trial_send_limit`, `trial_sent_count`
  - `sending_paused` + `pause_reason` (`bounce_rate_exceeded`|`complaint_rate_exceeded`). A check constraint keeps them consistent.
- **`users`** (unique `lower(email)`), **`sessions`** (`token_hash` PK), **`api_keys`** (`scope`, `prefix`, `key_hash`, `revoked_at`).
- **`domains`**: the sending identities.
  - `kind` (`domain`|`email`), `status` (`pending`|`verified`|`failed`), `ses_status`, `dkim_tokens[]`, `verified_at`, `last_checked_at`.
  - `name` is **globally unique** on `lower(name)`, because an SES identity is account-wide.
- **`contacts`**: unique `(workspace_id, lower(email))`. `status` is `active`|`unsubscribed`|`bounced`|`complained`|`invalid`.
- **`tags`**, **`contact_tags`**.
- **`suppressions`**: unique `(workspace_id, lower(email))`.
  - `reason`: `unsubscribed`|`bounced`|`complained`|`manual`.
  - `source`: `unsubscribe_link`|`ses_bounce`|`ses_complaint`|`manual`|`import`.
  - `detail` is jsonb.
- **`campaigns`**: unique `(workspace_id, lower(name))`.
- **`emails`**
  - `campaign_id`, `subject`, `from_name`, `from_address`, `html`, `text`, `audience_all`.
  - `status`: `draft`|`scheduled`|`sending`|`paused`|`done`. `scheduled` is accepted by launch, but nothing sets it yet.
  - `pause_reason`: `manual`|`trial_quota_exhausted`|`daily_limit_reached`|`ses_sending_paused`|`identity_not_verified`|`reputation`. Constraint: `(status = 'paused') = (pause_reason IS NOT NULL)`.
- **`email_tags`**, **`email_contacts`**: the saved audience of an email.
- **`email_recipients`**: the per-recipient send state.
  - `status`: `pending`|`sending`|`sent`|`failed`|`skipped`. Also `attempts`, `next_attempt_at`, `claimed_at`, `last_error`.
  - `ses_message_id` is unique when not null.
  - `sent_at`, `delivered_at`, `bounced_at`, `bounce_type`, `complained_at`, `unsubscribed_at`.
  - Unique `(email_id, contact_id)`.
- **`email_events`**: an append-only log.
  - `type`: `queued`, `sent`, `delivered`, `bounced`, `soft_bounced`, `complained`, `unsubscribed`, `failed`, `rejected`, `delayed`, `sending_paused`.
  - Also `detail` jsonb, `ses_message_id`.
- **`message_logs`**: rendered messages written by `LogSender`.
- **`usage_daily`**: `(workspace_id, date, sent_count)` for daily limits.
- **`processed_sns_messages`**: `sns_message_id` PK, the webhook dedupe.
- **`webhook_endpoints`**: customer HTTPS endpoints; the secret column contains workspace-bound AES-GCM ciphertext for newly managed endpoints. Legacy endpoints migrate disabled and require signing-secret rotation. A trigger on email_events atomically creates webhook_deliveries, including events inserted directly by campaign launch.
- **`webhook_deliveries`**: PostgreSQL outbox with HMAC signing, leases, bounded retries and seven-day finished-history retention. internal/webhooks owns endpoint management, URL/network protection, encryption and dispatch. WEBHOOK_ENCRYPTION_KEY enables dispatch; missing configuration disables creation/rotation.
- **transactional_requests**: read-only historical stub receipts. Unexpired keys conflict explicitly; never replay them as real sends or write new receipts.
- **transactional_messages**: sole live idempotency store and durable priority queue. Native POST /api/transactional and compatibility POST /api/emails share Service.Accept over Diya's accept transaction; only notify after commit. The compatibility handler maps from to from_address and adapts replay headers/status/errors. Main starts one Dispatcher sharing the same Sender and sender.Worker gate as broadcasts. The runtime stub is removed; its old enabled flag fails startup.
- **Migration order**: 1–5 base, 6 customer webhooks, 7 legacy receipts, 8 webhook retention, 9 transactional engine, 10 new shared webhook payload shape. Never edit 6–8. Diya-origin version-6 databases are incompatible with this history and need a separate planned migration path. Never mark versions manually.
- **Customer event correlation**: new payloads contain message_id and message_type (transactional/broadcast/workspace), preserving broadcast email_id/recipient_id. Migration 10 replaces only the trigger function; historical queued bytes stay unchanged. Signed opt-in transactional tracking routes generate email.opened/email.clicked; no broadcast tracking is fabricated.

Intern 3 extensions are registered in cmd/server/developer.go. Domain diagnostics live in internal/domains/dns.go; GET /api/domains/{id}/dns is independent of SES identity verification. Custom MAIL FROM is a DNS preview, not provider configuration. Full API examples and signing contracts are in docs/developer-quickstart.md and docs/openapi.yaml.

**Report counts** come from `email_recipients`: status counts plus `COUNT(delivered_at)`, `COUNT(bounced_at)`, `COUNT(complained_at)` and `COUNT(unsubscribed_at)`. They do not come from `email_events`.

## Core flows

### Request pipeline

`httpx.Logger → Recover → SecurityHeaders → LimitBody`. Then one of three paths:

- **API group:** `Authenticator.Middleware → per-key rate limit → auth.Authorize`.
  - Scope principals are `session`, `full_access` and `sending_access`.
  - `auth.FromContext(ctx)` gives `Identity{UserID, KeyID, KeyPrefix, Scope, WorkspaceID, Plan}`.
- **Public group:** per-IP limit, covering signup (with an extra hourly limit), login, unsubscribe, the dev routes and the static frontend.
- **`/webhooks/ses` and `/healthz`:** attached directly to the router, with no rate limiting.

### Launch (`emails.send`, `POST /api/emails/{id}/send`)

Runs in one transaction:
1. Lock the email; it must be `draft` or `scheduled`.
2. `checkDomain`.
3. `Snapshot` inserts the recipients matching `audienceMatch`: active, not suppressed, and in the email's audience. If there are none: `422 no_eligible_recipients`.
4. Refuse if the workspace is `sending_paused` (`409`).
5. Refuse trial workspaces whose remaining quota is smaller than the recipient count (`402`).
6. Record `queued` events and set the email to `sending`.

### Worker (`sender.Worker`)

Every second:
- Mark emails `done` once they have no pending or sending recipients.
- Start a lane for each active, non-paused workspace that has `sending` emails.

Each lane runs a limiter at `workspaces.SendRate`: `min(configured or trial rate, 70% of the global rate)`. The global rate is `SEND_RATE_PER_SEC` with the log driver, or SES `MaxSendRate` with SES.

`process` handles one claimed recipient:
1. Wait on the limiters.
2. `loadState`: re-check the workspace, email and contact status and **the suppression list**. A blocked recipient is marked `skipped` with `recipient_blocked`.
3. `workspaces.Reserve`. If the trial quota or daily limit is exhausted, the email is paused.
4. `Render` the message and `Sender.Send` it with a 30 s timeout.
5. On success, `complete` marks the recipient sent and records a `sent` event. If the sender implements `Simulator` (the log driver), `Simulate` then emits synthetic SES events.
6. On failure, `classify` takes the `*SendError` (anything else counts as `UnknownOutcome`) and `handleFailure` acts on it. Quota is released for every action except `UnknownOutcome`.

| Action | Effect |
|---|---|
| `Retry` | back to `pending`, with backoff of `5s·2^attempts`; `failed` after 5 attempts |
| `Fail` | recipient `failed` with the reason in `last_error` |
| `PauseSending` | unclaim the recipient, pause the email `ses_sending_paused`, and `holdSending` the whole worker for 10 minutes |
| `PauseIdentity` | recipient `failed`, email paused `identity_not_verified` |
| `UnknownOutcome` | recipient `failed` with `unknown_outcome`, never retried |

**API errors never create suppressions.** Only feedback events and unsubscribes do.

The broadcast worker idles during holds or at 90% of SES daily quota. Transactional work can use remaining headroom up to the hard limit. One PostgreSQL-backed gate serializes account token/quota/priority decisions across instances and persists workspace pacing. The in-memory gate exists only for pool-less unit tests. Gate acquisition is bounded to one minute; bulk lanes claim one recipient at a time. Permit.Start is committed atomically with the message ownership/provider-start boundary; Permit.Finish releases definitive non-sends or records consumed/uncertain usage. Every path must finish its permit. Never introduce a second production rate limiter.

Migration 11 adds claim_token and provider_started_at to both queues and the sending_control/sending_permits/sending_waiters/workspace_send_gates tables. All message completion/retry/quota mutations are owner-token fenced. Five-minute expired pre-provider claims requeue; started/legacy claims fail unknown_outcome without resend. Reserved permits expire after 30 seconds; started permits become conservatively used after five minutes. Recovery, retention and identity polling batches use advisory locks; startup migrations use a session advisory lock. Account snapshots are ordered by DB observation time. API/IP buckets and their janitors remain per process.

Transactional acceptance reserves workspace quota atomically; definitive failures release it, ambiguous outcomes retain it. Retries and replays keep the canonical ID and reserve once. Suppression blocks bounced/complained/manual but permits marketing unsubscribes. Eligibility is checked before and after waiting for the gate. Transactional mail has no marketing-contact insertion or unsubscribe footer. The dispatcher polls for missed notifications/restarts. Signature/DNS/lease/retry customer delivery logic remains in internal/webhooks and never blocks sender/provider acknowledgement.

### SES classification (`ses.Classify`)

| SES error | Action |
|---|---|
| `TooManyRequestsException`, `LimitExceededException`, `Throttling*` | `Retry` |
| `*SendingPausedException`, `AccountSendingPaused`, `ConfigurationSetSendingPaused`, `AccountSuspendedException` | `PauseSending` |
| `MailFromDomainNotVerifiedException`, `NotFoundException` | `PauseIdentity` |
| `MessageRejected` | `Fail` (`message_rejected: <msg>`) |
| any other response with HTTP ≥ 500 | `Retry` |
| any other response | `Fail` (`ses_error: <code>`) |
| no response (network error, timeout) | `UnknownOutcome` |

### Feedback (`feedback.Processor.Handle(ctx, snsMessageID, raw)`)

Runs in one transaction:
1. If an SNS message ID is present, insert it into `processed_sns_messages`. On conflict, return `duplicate`.
2. Parse the event (`eventType`, falling back to `notificationType`). Invalid JSON or an unhandled type returns `ignored`.
3. Look up the broadcast recipient, then transactional message, with ses_message_id = mail.messageId FOR UPDATE. If neither exists: unknown_message.
4. Cross-check workspace_id and broadcast email_id/recipient_id or transactional message_id tags when present. On mismatch: tag_mismatch.
5. Apply the event:

| Event | Effect |
|---|---|
| `Delivery` | `delivered_at` |
| `Bounce` with `bounceType` `Permanent` | `bounced_at` + `bounce_type`, `Suppress` (`bounced`/`ses_bounce`, which also sets the contact status), event, `checkReputation` |
| other bounce types | `soft_bounced` event only |
| `Complaint` | `complained_at`, `Suppress` (`complained`/`ses_complaint`), event, `checkReputation` |
| `Reject` | recipient `failed` |
| `DeliveryDelay` | `delayed` event |

The webhook returns 200 for every verified message, including duplicates, unknown message IDs and tag mismatches. Processing errors return 500 so SNS retries, verification failures return 403 with no detail, and malformed JSON returns 400. `SubscriptionConfirmation` is confirmed by GETting `SubscribeURL`, which must pass the same host allowlist; redirects are not followed.

`checkReputation` computes the 30-day `COUNT(*)`, `COUNT(bounced_at)` and `COUNT(complained_at)` over recipients with `sent_at`. If `Thresholds.Evaluate` returns a reason:
- set `workspaces.sending_paused`, only if it isn't already set;
- pause the workspace's `sending` emails with `reputation`;
- record a workspace-level `sending_paused` event.

Clearing the pause is manual (SQL).

### Suppression checkpoints

Suppressions are enforced at three points:
1. `audienceMatch`, at snapshot and audience size;
2. `loadState`, before each send;
3. `contacts.Import`, which skips suppressed addresses and never updates non-active contacts.

`DELETE /api/suppressions/{id}` also reactivates a contact whose status is `unsubscribed`, `bounced` or `complained`.

### Identities

`POST /api/domains`:
1. Inserts the row inside a transaction. The global unique index rejects duplicates with `409 identity_taken`.
2. Calls `Identities.Create`. On `AlreadyExists`, it falls back to `Get`.
3. Saves the tokens and status, then commits.

Local status is `verified` iff SES reports `VerifiedForSendingStatus`, `failed` for `FAILED`/`NOT_FOUND`, and `pending` otherwise. `checkDomain` accepts a from address when the workspace has a verified identity where either:
- `kind = 'domain'` and the name equals the address's domain, or
- `kind = 'email'` and the name equals the full address.

### Unsubscribe

`GET /unsubscribe/{token}` shows a confirmation page. `POST` (the button, or a mail client's one-click `List-Unsubscribe-Post`) does the following:
- `Suppress` with `unsubscribe_link`;
- set `email_recipients.unsubscribed_at`;
- record an `unsubscribed` event.

Links use `PUBLIC_BASE_URL`.

## Scopes

`internal/auth/scopes.go` maps `"METHOD /pattern"` to the allowed principals.

| Principal set | Allowed |
|---|---|
| `sessionOnly` | logout and API key management |
| `workspace` (session + `full_access`) | dashboard, events, contacts read and write, tags, campaign create/read/update, email update/delete/pause/resume/messages/recipients, domains, suppressions |
| `anyone` (+ `sending_access`) | GET /api/me, contact import, GET /api/campaigns, create an email in a campaign, GET an email, send, report, both transactional send routes and transactional detail by ID |

Transactional list/stats and webhook management require session or full_access. Sending-only keys cannot create or manage campaigns. Key management remains session-only.

## Frontend notes

- **Build.** The production build is `next build --webpack` with `output: 'export'`. Webpack, not Turbopack, because the strict CSP forbids inline scripts. `trailingSlash: true`, so page links look like `/emails/edit/?id=…`.
- **Auth state.** All data fetching goes through `lib/api.js` (`api(method, path, body)`). A 401 signs the user out, a 429 shows a toast, and a 402 opens the upgrade modal. Add a human message for each new error code to `messages` in `lib/api.js`.
- **Session.** `lib/session.js` exposes `me` (from `/api/me`), `trial`, `exhausted`, `refreshMe` and `showUpgrade`. `me.domains` is the list of **verified** identity names; a name containing `@` is a single email identity, which the email editor's From field handles.
- **Tables.** Every table uses `components/DataTable.js` (TanStack Table): client-side or server-side pagination via the `server` prop, sorting and row selection.
- **Styling.** Plain CSS with tokens on `:root` and `:root[data-theme="dark"]`. Do not add a CSS framework.
- **Request logging.** `pages/emails/edit.js` logs each broadcast request payload with `console.log('[forke] request', …)`. This is intentional; keep it.
- **Embedding.** Run `npm run build` after frontend changes, because the Go binary embeds `frontend/out`.

## Recipes

- **New authenticated endpoint.** Write the handler in the right `internal/<domain>` package, register it in `cmd/server/routes.go`, add it to `routeScopes`, and extend `internal/auth/scopes_test.go`.
- **New column or enum value.** Add a new migration. For an enum value, drop and re-add the check constraint. Update the Go structs and scans, and the frontend badges in `components/ui.js` if the value is shown.
- **New event type.** Add it to:
  - the `email_events_type_check` constraint (new migration);
  - `events.types` in `internal/events/handler.go`;
  - the filter list in `frontend/pages/activity.js`;
  - badge tones in `components/ui.js`.
- **New email pause reason.** Add it to:
  - the `emails_pause_reason_check` constraint (new migration);
  - the place that sets it;
  - `pauseReasons`, and `noResume` if resuming makes no sense, in `frontend/pages/emails/view.js`.
- **Another sender.** Implement `sender.Sender` and return a `*sender.SendError` with the right `Action` for every failure, then select it in `cmd/server/main.go`.
- **Testing SES behaviour without AWS.** Fake the narrow client interfaces in `internal/ses` (`sendAPI`, `accountAPI`, `identityAPI`), use `snstest.NewSigner` for signed SNS messages, and use `/dev/ses-event` or `Processor.Handle` for events.

## Gotchas

- `.env` holds real credentials and is gitignored. Never commit it or print its values.
- Run `sesbootstrap` twice on a fresh setup: once to create the topic and get `SNS_TOPIC_ARN` into `.env`, and again after restarting the server so it can confirm the subscription.
- When `PUBLIC_BASE_URL` is https, session cookies are `Secure`. Over plain `http://localhost`, Safari drops them.
- The free ngrok URL changes on every restart. When it does, update `PUBLIC_BASE_URL`, restart the server and rerun `sesbootstrap`.
- Sending instances must share one database/schema, sender driver/account and rate policy. Sending admission is DB-backed; API/IP request limits remain per process. Stop older binaries before enabling migration-11 multi-instance operation. Independent verification and local race evidence are documented in docs/multi-instance-handoff.md.
- Under the SES driver, signup and seed create no fake domains. Under the log driver they do, and those fake names (e.g. `forke.space`) occupy the global identity namespace in that database.
- `go:embed all:out` needs `frontend/out` to exist. `frontend/out/.keep` is committed so a fresh clone compiles before the dashboard is built; it then serves an empty UI.
