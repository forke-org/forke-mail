# Forke Mail developer quickstart

Forke Mail uses an authenticated developer API, verified sending identities and signed customer webhooks. Examples below use Bash/curl (Git Bash on Windows). Replace placeholders locally; never commit credentials or cookie jars.

## Current delivery boundary

The customer webhook pipeline is implemented. The new direct-send API accepts an `EmailEnqueuer` adapter, but Advik's branch currently exposes campaign sending rather than a direct transactional enqueue service. Until that shared engine is connected, production `POST /api/emails` returns `503 transactional_engine_unavailable`.

For API development only, set `APP_ENV=development` and `TRANSACTIONAL_STUB_ENABLED=true`. Responses explicitly contain `"mode":"development_stub"`. The stub records an idempotent receipt in PostgreSQL, does not retain message contents, does not dispatch email and does not generate synthetic delivery notifications. Startup rejects this flag outside development. A queued stub receipt is not evidence that any email will be delivered.

## Local setup

Use Go 1.26+, PostgreSQL 17+ and the setup in README. Set `DATABASE_URL`, `PUBLIC_BASE_URL`, `UNSUBSCRIBE_SECRET` and, in development, `SEED_TOKEN`. The existing `log` sender avoids AWS calls; its domain identity provider reports verified without real DNS verification.

Generate a persistent encryption key once:

```bash
openssl rand -base64 32
```

Put that value in `WEBHOOK_ENCRYPTION_KEY` in your uncommitted environment. Keep it stable across restarts and back it up securely. Signing secrets are AES-256-GCM encrypted with workspace-bound associated data. Losing/changing the encryption key prevents existing secrets from decrypting; rotate/reconfigure endpoints with a planned recovery procedure, not a random replacement on each deploy.

Without this key, webhook creation/rotation returns `503 webhooks_not_configured` and no webhook dispatcher starts. Migrated legacy endpoints are disabled and must have their secrets rotated before enabling. After configuration, run:

```bash
go run ./cmd/server
```

For a populated dashboard, run `npm ci && npm run build` inside `frontend` before starting Go. No dashboard UI changes are required for the new HTTP endpoints.

## 1. Create an account and API key

```bash
BASE=http://localhost:8080
COOKIE_JAR=$(mktemp)
curl -sS -c "$COOKIE_JAR" "$BASE/api/auth/signup" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Developer","email":"developer@example.com","password":"replace-with-a-strong-password","workspace":"My app"}'

curl -sS -b "$COOKIE_JAR" "$BASE/api/keys" \
  -H 'Content-Type: application/json' \
  -d '{"name":"My backend","scope":"sending_access","password":"replace-with-a-strong-password"}'
```

Save the returned `key` in your backend's secret store. It is shown only at creation or rotation. The database stores its SHA-256 hash, not the raw API key; list responses contain metadata only. Secret responses have `Cache-Control: no-store`.

Key management requires a dashboard session. Create/rotate require password confirmation; revoke currently requires the session but not an extra password. Trial workspaces cannot create full-access keys. Sending-only keys cannot manage keys, domains or webhooks. Full-access keys can manage domains/webhooks but cannot manage API keys.

```bash
curl -sS -b "$COOKIE_JAR" "$BASE/api/keys"
curl -sS -b "$COOKIE_JAR" -X POST "$BASE/api/keys/KEY_ID/rotate" \
  -H 'Content-Type: application/json' \
  -d '{"password":"replace-with-a-strong-password"}'
curl -i -b "$COOKIE_JAR" -X DELETE "$BASE/api/keys/KEY_ID"
```

Rotation immediately revokes the old API key and returns a new key once. Existing last-full-access-key protection remains in place for revocation. In-flight requests already authenticated can complete; subsequent requests with the revoked key receive 401.

## 2. Register and verify a domain

```bash
curl -sS -b "$COOKIE_JAR" "$BASE/api/domains" \
  -H 'Content-Type: application/json' -d '{"name":"your-owned.example","kind":"domain"}'
curl -sS -b "$COOKIE_JAR" -X POST "$BASE/api/domains/DOMAIN_ID/verify"
curl -sS -b "$COOKIE_JAR" "$BASE/api/domains/DOMAIN_ID/dns"
```

In SES mode, publish the DKIM records returned by SES. The verify endpoint and existing poller use SES identity state; DNS diagnostics do not override it. The DNS endpoint returns expected record type/name/value, observed values, `found`, `pending` or `mismatch`, and `checked_at`. DNS missing/timeouts are pending, wrong/conflicting values are mismatch. An existing valid DMARC policy is accepted; `p=none` is starter guidance, not a demand to replace a stricter policy. Missing DNS in log mode is expected even if the local fake identity says verified.

Default SES MAIL FROM uses an `amazonses.com` domain. Do not add a root SPF record and assume custom MAIL FROM is configured. To preview DNS for a custom MAIL FROM subdomain:

```bash
curl -sS -b "$COOKIE_JAR" "$BASE/api/domains/DOMAIN_ID/dns?mail_from=bounce.your-owned.example"
```

This returns `mail_from_mode=custom_dns_preview` and `ses_configuration_required=true`. It checks SPF on that subdomain and the regional MX target (AWS region must be configured). It does not call SES to configure MAIL FROM. An operator must configure the matching SES identity separately, following [AWS's custom MAIL FROM guide](https://docs.aws.amazon.com/ses/latest/dg/mail-from.html). Only subdomains of the registered domain are accepted. Never publish multiple SPF policies at the same name; merge with an existing policy carefully. DMARC can align through DKIM without custom MAIL FROM.

## 3. Register a customer webhook

```bash
curl -sS -b "$COOKIE_JAR" "$BASE/api/webhooks" \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://app.your-owned.example/forke-events","events":["email.delivered","email.bounced","email.complained"]}'
curl -sS -b "$COOKIE_JAR" "$BASE/api/webhooks"
curl -sS -b "$COOKIE_JAR" -X PATCH "$BASE/api/webhooks/WEBHOOK_ID" \
  -H 'Content-Type: application/json' -d '{"enabled":false}'
curl -sS -b "$COOKIE_JAR" -X POST "$BASE/api/webhooks/WEBHOOK_ID/rotate"
curl -sS -b "$COOKIE_JAR" "$BASE/api/webhooks/WEBHOOK_ID/deliveries?limit=20&offset=0"
curl -i -b "$COOKIE_JAR" -X DELETE "$BASE/api/webhooks/WEBHOOK_ID"
```

Keep the one-time returned `secret` separately from your API key. At most ten endpoints per workspace. URLs must resolve to public HTTPS destinations on port 443, with no credentials or fragment. Private, loopback, link-local, metadata, reserved and mixed public/private DNS destinations are blocked. DNS is checked at the actual connection and connections use vetted IPs with normal TLS hostname verification. Redirects and environment HTTP proxies are disabled. Localhost receivers require test doubles in automated tests or a public HTTPS endpoint for manual development.

Supported events: `email.queued`, `email.sent`, `email.delivered`, `email.bounced`, `email.soft_bounced`, `email.complained`, `email.unsubscribed`, `email.failed`, `email.rejected`, `email.delayed`, `workspace.sending_paused`. Opens/clicks are not implemented in the upstream engine and are rejected as subscriptions. `email.sent` means provider acceptance, while `email.delivered` means recipient-server acceptance; neither proves inbox placement or human engagement.

## 4. Submit a transactional request

```bash
curl -i "$BASE/api/emails" \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -H 'Idempotency-Key: signup-user-123-v1' \
  -H 'Content-Type: application/json' \
  -d '{"from":"welcome@your-owned.example","to":"person@example.com","subject":"Welcome","text":"Hello from Forke Mail"}'
```

Development-stub example:

```json
{"id":"message-uuid","status":"queued","mode":"development_stub"}
```

The idempotency key is required, 1-128 printable non-space ASCII characters, workspace-scoped and retained for 24 hours. Same normalized payload/key returns the same ID with `Idempotency-Replayed: true`; changed payload returns 409. The PostgreSQL record survives handler/server restart. Expired keys can be reused and may represent another send. At most 1,000 unexpired direct-send receipts per workspace. This is separate from the existing sender quota; actual dispatch quota reservation belongs in the shared enqueue/worker engine.

The request currently supports one `to` address, a plain sender address, a subject up to 256 characters and HTML and/or text. Unknown JSON fields, invalid addresses and subject CR/LF are rejected. The sender must belong to a verified identity in the same workspace. Existing suppressions and inactive contacts block new requests conservatively; marketing versus transactional opt-out policy must be agreed with Intern 1 before production wiring. Recipients are not imported into marketing contacts by this endpoint.

## 5. Verify customer webhook signatures and reject replays

Headers:

```text
Webhook-Id: stable-delivery-uuid
Webhook-Timestamp: unix-seconds
Webhook-Key-Version: 1
Webhook-Signature: v1=hex-hmac-sha256
```

Signed bytes are `Webhook-Id + "." + Webhook-Timestamp + "." + exact_raw_request_body`. Use the entire `whsec_...` string as the HMAC key. Never parse/re-encode JSON before verification. Retries retain the delivery/event IDs and exact body, but get a fresh timestamp/signature. Body IDs are strings because internal event IDs are bigint.

Here is a complete Go receiver example for an application already using pgx. The application owns this table and database:

```sql
CREATE TABLE received_forke_webhooks (
    delivery_id text PRIMARY KEY,
    payload jsonb NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);
```

```go
package receiver

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "io"
    "net/http"
    "strconv"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

func Handler(pool *pgxpool.Pool, secret string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
        if err != nil { http.Error(w, "body too large", 413); return }
        id := r.Header.Get("Webhook-Id")
        timestamp := r.Header.Get("Webhook-Timestamp")
        ts, err := strconv.ParseInt(timestamp, 10, 64)
        now := time.Now().Unix()
        if err != nil || id == "" || ts < now-300 || ts > now+300 {
            http.Error(w, "stale or invalid timestamp", 401); return
        }
        mac := hmac.New(sha256.New, []byte(secret))
        mac.Write([]byte(id + "." + timestamp + "."))
        mac.Write(raw)
        expected := "v1=" + hex.EncodeToString(mac.Sum(nil))
        if !hmac.Equal([]byte(expected), []byte(r.Header.Get("Webhook-Signature"))) {
            http.Error(w, "invalid signature", 401); return
        }
        if !json.Valid(raw) { http.Error(w, "invalid JSON", 400); return }
        _, err = pool.Exec(r.Context(), `
            INSERT INTO received_forke_webhooks(delivery_id,payload)
            VALUES($1,$2::jsonb) ON CONFLICT(delivery_id) DO NOTHING`, id, string(raw))
        if err != nil { http.Error(w, "try later", 503); return }
        w.WriteHeader(http.StatusNoContent)
    }
}
```

This example durably stores each delivery once. Process those stored rows separately, or atomically apply your business update and insert the delivery ID in one transaction. Do not acknowledge before durable acceptance, or run a non-idempotent business side effect after a duplicate insert. A timestamp window alone does not block a replay within that window; the unique delivery ID does. Keep replay IDs at least 24 hours. The sender provides at-least-once delivery, not exactly-once processing.

On signing-secret rotation, queued/retried deliveries use the current secret/version on their next claim. An already claimed attempt can use the previous secret for up to its 10-second request timeout; receivers should allow a brief bounded overlap, then discard the old secret. Rotation is not a full replay of past events.

## Delivery limits and failure policy

- Two webhook workers, one attempted dispatch per endpoint per second and a 10-second request timeout.
- At most eight attempts and a 24-hour delivery lifetime, with exponential backoff plus jitter.
- 2xx acknowledges; network failures, 429 and 5xx retry. Other statuses, including redirects, fail permanently.
- Worker leases expire after 30 seconds and recover after restart/periodic maintenance. Disabling an endpoint cancels queued/claimed rows; an already in-flight HTTP request can complete. Deleting an endpoint deletes its delivery history.
- Finished delivery history is retained for seven days and pruned in bounded batches. Event retention is separate. Monitor pending queue age, disk usage and failure counts; PostgreSQL capacity is finite.
- Only metadata/status is stored for attempts; customer response bodies and signing/API secrets are not logged.
- Existing API token buckets apply: paid keys default to 20 requests/sec with burst 40; trial keys 2/sec with burst 5; dashboard sessions use the paid API rule. Public endpoints default to 10/sec with burst 30. Limits are per process, so run a single instance unless coordinated limiting is added.

## Error contract

Errors are JSON `{"error":"snake_case_code"}`.

| Status | Examples |
|---|---|
| 400 | `invalid_json`, `invalid_idempotency_key` |
| 401 | `unauthorized` for missing/invalid/revoked credentials |
| 403 | `insufficient_scope`, `incorrect_password` |
| 404 | `not_found`, including IDs owned by another workspace |
| 409 | `idempotency_conflict`, `workspace_sending_paused`, `webhook_endpoint_limit`, `webhook_secret_rotation_required` |
| 413 | `body_too_large` (JSON limit 1 MiB) |
| 422 | `invalid_email_request`, `sender_not_verified`, `recipient_suppressed`, `invalid_webhook_url`, `invalid_webhook_events`, `invalid_mail_from_domain` |
| 429 | API rate limit or `transactional_queue_full` |
| 500 | `internal_error` |
| 502 | `ses_request_failed` |
| 503 | `transactional_engine_unavailable`, `webhooks_not_configured`, `aws_region_required_for_preview` |

## Reproducible demo and engine handoff

Run tests with `TEST_DATABASE_URL` pointing to disposable PostgreSQL. These use a real database and fake network/provider adapters:

```bash
go test ./cmd/server -run TestDeveloperContractEndToEndWithFakeEngine -v
go test ./internal/webhooks -run 'TestDurableWebhookFlow|TestSNSDeduplicationQueuesOneDelivery' -v
```

The end-to-end test creates a key, verifies a domain with the local fake identity provider, submits an authenticated request through a fixture enqueue adapter, receives its stable message ID, processes simulated provider feedback and validates the signed outgoing customer webhook. It does not prove AWS delivery or provide the missing production transactional engine.

Advik's adapter must implement `transactional.EmailEnqueuer.Enqueue(ctx, tx, job)`, persist its priority job using the supplied transaction and provided stable message ID, and return only after durable enqueue. Errors must roll back that transaction. The worker must enforce quota/suppression checks and emit normal persisted events correlated to that ID. Then assign the adapter to `developerServices.transaction.Enqueuer` with `Development=false`. Do not make direct SES calls in the HTTP handler. Existing campaign endpoints remain usable through the existing broadcast engine; this adapter must not silently route OTPs through marketing audiences or add newsletter footers.

OpenAPI for the Intern 3 surface is in `docs/openapi.yaml`. SES Open/Click plumbing and custom MAIL FROM provider configuration remain explicit follow-up integration items; current endpoints do not claim they exist.
