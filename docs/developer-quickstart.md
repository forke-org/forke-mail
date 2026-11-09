# Forke Mail developer quickstart

Forke Mail uses an authenticated developer API, verified sending identities and signed customer webhooks. Examples below use Bash/curl (Git Bash on Windows). Replace placeholders locally; never commit credentials or cookie jars.

## Delivery

Transactional sends go through `POST /api/transactional` (or the Resend-compatible `POST /api/resend/emails`) and the priority dispatcher. Broadcasts go through campaigns. Both deliver with the configured `SENDER_DRIVER` (`log`, `smtp` or `ses`).

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

Supported events: `email.queued`, `email.opened`, `email.clicked`, `email.sent`, `email.delivered`, `email.bounced`, `email.soft_bounced`, `email.complained`, `email.unsubscribed`, `email.failed`, `email.rejected`, `email.delayed`, `workspace.sending_paused`. `email.opened` and `email.clicked` are emitted for transactional messages sent with `track_opens`/`track_clicks`. `email.sent` means provider acceptance, while `email.delivered` means recipient-server acceptance; neither proves inbox placement or human engagement.

## 4. Send a transactional email

```bash
curl -i "$BASE/api/transactional" \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -H 'Idempotency-Key: signup-user-123-v1' \
  -H 'Content-Type: application/json' \
  -d '{"from_name":"Forke","from_address":"welcome@your-owned.example","to":"person@example.com","subject":"Welcome","text":"Hello from Forke Mail"}'
```

The response is `202` with the message (`id`, `status: "queued"`, …). The `Idempotency-Key` header is optional. With it, the same key and payload within 24 hours returns the original message with `Idempotent-Replayed: true`, and a different payload returns `409 idempotency_key_reused`. Without it, an identical message within 60 seconds is returned as a `duplicate_content` replay.

The sender must belong to a verified identity in the same workspace. Recipients suppressed for bounces, complaints or manual blocks are refused with `422 recipient_suppressed`; unsubscribing from broadcasts does not block transactional mail. Recipients are not added to marketing contacts by this endpoint.

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
| 409 | `idempotency_key_reused`, `workspace_sending_paused`, `webhook_endpoint_limit`, `webhook_secret_rotation_required` |
| 413 | `body_too_large` (JSON limit 1 MiB) |
| 422 | `invalid_to`, `invalid_from_address`, `from_domain_not_verified`, `recipient_suppressed`, `invalid_webhook_url`, `invalid_webhook_events`, `invalid_mail_from_domain` |
| 429 | API rate limit |
| 500 | `internal_error` |
| 502 | `ses_request_failed` |
| 503 | `webhooks_not_configured`, `aws_region_required_for_preview` |

## Reproducible demo

Run tests with `TEST_DATABASE_URL` pointing to disposable PostgreSQL. These use a real database and fake network/provider adapters:

```bash
go test ./cmd/server -run TestDeveloperContractEndToEnd -v
go test ./internal/webhooks -run 'TestDurableWebhookFlow|TestSNSDeduplicationQueuesOneDelivery' -v
```

The end-to-end test creates a key, verifies a domain with the local identity provider, sends a transactional message, processes simulated provider feedback and validates the signed outgoing customer webhook, whose payload carries the `message_id`.

OpenAPI for these endpoints is in `docs/openapi.yaml`. Custom MAIL FROM provider configuration remains a follow-up; the DNS preview does not claim it exists.
