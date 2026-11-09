# Forke Mail

Forke Mail is a multi-tenant broadcast and transactional email service. Each workspace imports contacts for campaigns or sends individual transactional messages through a shared **Amazon SES** sender. Delivery, bounce and complaint feedback comes back over **Amazon SNS**. The local log driver stores mail and simulates feedback without internet email delivery.

It is a single Go binary that embeds a Next.js dashboard and uses PostgreSQL for all state. It needs no Redis, message broker or ORM.

## Contents

- [Features](#features)
- [How it works](#how-it-works)
- [Requirements](#requirements)
- [Quick start (local, no AWS needed)](#quick-start-local-no-aws-needed)
- [Configuration](#configuration)
- [Using the dashboard](#using-the-dashboard)
- [REST API](#rest-api)
- [Sending real email with Amazon SES](#sending-real-email-with-amazon-ses)
- [Testing SES events without AWS](#testing-ses-events-without-aws)
- [Running the tests](#running-the-tests)
- [Deploying to production](#deploying-to-production)
- [Project layout](#project-layout)
- [Troubleshooting](#troubleshooting)
- [Security model](#security-model)

## Features

- **Workspaces and accounts.** Sign up with email and password, and get a session cookie. Each workspace is fully isolated from the others.
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
  - Stale sending claims fail as unknown_outcome without automatic resend after a crash.
  - Broadcast emails get per-recipient unsubscribe links and List-Unsubscribe / one-click headers.
- **Transactional APIs.** Native POST /api/transactional and compatibility POST /api/emails use one durable acceptance/idempotency service and return the same real ID across keyed retries. Optional signed open/click tracking is transactional only. The dashboard includes list, detail and activity pages.
- **Customer webhooks.** One PostgreSQL outbox for both pipelines, HMAC-signed deliveries, encrypted secrets, SSRF protection, bounded retries and receiver deduplication. New payloads identify transactional message_id separately from broadcast email_id/recipient_id.
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

Open **http://localhost:8080** and sign up. New accounts get a trial workspace (100 sends). With the `log` driver they also get a verified sandbox domain, so you can send immediately; the sends are logged rather than delivered.

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

### Sending

| Variable | Default | Description |
|---|---|---|
| `SENDER_DRIVER` | `log` | `log` (store messages, simulate events) or `ses` (real sending) |
| `SEND_RATE_PER_SEC` | `20` | global send rate for the log driver. With SES it is replaced by the account's `MaxSendRate` |
| `TRIAL_SEND_RATE_PER_SEC` | `1` | send rate for trial workspaces |
| `MAX_BOUNCE_RATE` | `0.05` | reputation guard: 30-day hard bounce rate that pauses a workspace |
| `MAX_COMPLAINT_RATE` | `0.001` | reputation guard: 30-day complaint rate that pauses a workspace |
| `REPUTATION_MIN_SAMPLE` | `100` | the guard only acts once a workspace has sent to at least this many recipients in 30 days |

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
| `PUBLIC_RATE_PER_SEC` / `PUBLIC_BURST` | `10` / `30` | per-IP limit for public routes (login, signup, unsubscribe) |
| `AUTH_FAILURES_PER_MIN` / `AUTH_FAILURE_BURST` | `10` / `10` | per-IP limit on failed logins and invalid API keys |

## Using the dashboard

| Page | What it does |
|---|---|
| **Overview** | get-started checklist, key numbers, recent emails, and whether you are on the log sender, SES sandbox or SES production |
| **Campaigns** | create a campaign, then write emails inside it. The editor has sender, audience, HTML and text content, and a live preview. `{{first_name}}` and `{{email}}` are merge fields; the unsubscribe footer is added automatically |
| **Email report** | live progress and per-recipient status. Delivered, bounced, complained and unsubscribed counts come from real SES events. Pause and resume are here |
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
| dashboard session | everything, including managing API keys |
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
- new signups no longer get a fake sandbox domain;
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
- `PUBLIC_BASE_URL=https://mail.yourcompany.com`, served over TLS by your load balancer or reverse proxy.
- `TRUSTED_PROXY` set to the proxy's IP or CIDR, so per-IP rate limits see real client addresses.
- A managed PostgreSQL with `sslmode=require` in `DATABASE_URL`. Migrations run automatically on startup.
- Strong, unique `UNSUBSCRIBE_SECRET`, and keep it stable.
- Prefer an IAM role (ECS task role, EC2 instance profile) over static keys: leave `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` empty and the default credential chain is used. Attach only the runtime policy.
- Run `sesbootstrap` once against the production `PUBLIC_BASE_URL`, with the bootstrap policy attached temporarily.
- SES production access requested and granted.
- Health check: `GET /healthz` returns `200 {"status":"ok"}`, or `503` when the database is unreachable.
- The server shuts down gracefully on `SIGTERM`, finishing in-flight requests and sends.

### Running more than one instance

Claiming uses `FOR UPDATE SKIP LOCKED`, so several instances never send to the same recipient twice. However, the send rate limiter and the API rate limiters are held in each instance's memory. With N instances, SES could see up to N × `MaxSendRate`, and API limits apply per instance. Run a single instance for sending, or lower the SES-derived rate accordingly.

## Project layout

```
cmd/
  server/          HTTP server, routing, background workers
  sesbootstrap/    idempotent SES/SNS setup
  sescheck/        prints SES account state
internal/
  accounts/        signup, login, logout, workspace and user creation
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
- **Passwords.** Stored with bcrypt (cost 12). Login timing is equalized for unknown emails.
- **Sessions.** `HttpOnly`, `SameSite=Lax`, `Secure` over https, 14-day lifetime, stored hashed. Mutating session requests must be same-origin (`Sec-Fetch-Site` or `Origin`).
- **API keys.** Shown once, stored as SHA-256 hashes with a 12-character display prefix. Creating and rotating require a session and password; revoking requires a session. Sending-only keys cannot manage campaigns, domains, webhooks or keys.
- **Unsubscribe links.** HMAC-SHA256 signed tokens, with one-click `List-Unsubscribe-Post` support.
- **SNS webhook.** Authenticated only by AWS's signature, with a certificate host allowlist, a topic check and dedupe.
- **Strict CSP.** No inline scripts, `frame-ancestors 'none'`, `nosniff`, and `no-store` on API responses.
- **Request limits.** Request body limits, server timeouts, per-IP and per-key rate limits, and lockout on failed authentication.
- **Logging.** Logs carry route pattern, workspace ID and key prefix, never authorization headers, raw keys, passwords or request bodies. Intentional one-time key/webhook-secret responses use no-store.

## Integrated API setup and migration path

See [developer quickstart](docs/developer-quickstart.md), [OpenAPI](docs/openapi.yaml) and [integration handoff](docs/integration-handoff.md) for copyable requests, signatures, scope differences and local verification. Both send APIs use Diya's real engine; the former runtime stub is removed. Leave TRANSACTIONAL_STUB_ENABLED unset or false; true is rejected to prevent silently changing old stub-only workflows into real sends.

Krishna migrations 6–8 remain byte-for-byte unchanged. Transactional schema is migration 9 and new shared customer webhook payloads are migration 10. Startup upgrades empty or Krishna-history databases; legacy receipts and historical queued webhook bytes stay intact. Do not apply this migration history to a Diya-origin database where version 6 means something else. Use a separately reviewed migration path for those databases.

Run one sending instance: global rate/quota scheduling is in-process. Transactional waiting work gets priority with bounded broadcast turns; in-flight calls are not preempted. Both pipelines share quotas, sender identity checks and provider feedback. Transactional messages allow marketing opt-outs but block hard-bounce, complaint and manual suppressions; broadcasts exclude every opt-out. No shared DB, live SES call or push was used for the integration evidence.
