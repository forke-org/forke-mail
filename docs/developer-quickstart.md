# Forke Mail developer quickstart

Forke Mail uses an authenticated developer API, verified sending identities and signed customer webhooks. Examples below use Bash/curl (Git Bash on Windows). Replace placeholders locally; never commit credentials or cookie jars.

## Delivery engine

Broadcast and transactional messages share the configured Sender, global rate/quota gate, workspace quotas and email_events outbox. Both HTTP send routes call transactional.Service.Accept, commit one PostgreSQL enqueue/idempotency transaction, then notify the dispatcher. Polling recovers missed notifications. No HTTP handler calls SES.

The normal-runtime stub is removed; TRANSACTIONAL_STUB_ENABLED=true is rejected at startup. Local (log driver) stores rendered mail and simulates provider feedback without sending internet mail. Live SES delivery was not exercised in this integration.

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

For a populated dashboard, run `npm ci && npm run build` inside `frontend` before starting Go. The dashboard includes transactional list, detail and activity pages. On Windows without touch, run `npx next build --webpack` and create frontend/out/.keep afterwards.

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

Supported events: `email.queued`, `email.sent`, `email.delivered`, `email.bounced`, `email.soft_bounced`, `email.complained`, `email.unsubscribed`, `email.failed`, `email.rejected`, `email.delayed`, `email.opened`, `email.clicked`, `workspace.sending_paused`. Transactional open/click tracking requires opt-in on the native request. HMAC-signed pixels and links generate these events; invalid tokens generate none. Known automated agents are excluded, but tracking remains an estimate. Broadcast open/click tracking is not implemented. Sent means provider acceptance and delivered means recipient-server acceptance; neither proves inbox placement.

## 4. Submit a transactional request

```bash
curl -i "$BASE/api/emails" \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -H 'Idempotency-Key: signup-user-123-v1' \
  -H 'Content-Type: application/json' \
  -d '{"from":"welcome@your-owned.example","to":"person@example.com","subject":"Welcome","text":"Hello from Forke Mail"}'
```

Compatibility response (new or replayed request, HTTP 202):

```json
{"id":"message-uuid","status":"queued","mode":"engine"}
```

The compatibility key is required, 1–128 printable non-space ASCII characters. Native keys are optional, up to 256 characters. Both use the same workspace-scoped 24-hour record in transactional_messages: retries through either route return the same real ID and reserve quota once. Compatibility replays return 202 and `Idempotency-Replayed: true`; native replays return 200 and `Idempotent-Replayed: true`. Changed payloads return 409 (`idempotency_conflict` on compatibility, `idempotency_key_reused` on native). Native requests without keys deduplicate matching content for 60 seconds. Expired keys can represent another send. Unexpired legacy stub receipt keys return `legacy_stub_key_requires_new_key`; use a fresh key. There is no separate 1,000-receipt stub limit.

Compatibility accepts one to, plain from, subject ≤256 characters and HTML and/or text. Native uses from_address, subject ≤300, and supports from_name, reply_to, tags, track_opens and track_clicks. Up to 10 tags are accepted (alphanumeric/underscore/hyphen keys and values, ≤64 bytes). Each HTML/text body is ≤256 KiB; JSON is ≤1 MiB. Tracking requires HTML. Unknown fields, invalid addresses and header CR/LF are rejected.

```bash
curl -i "$BASE/api/transactional" \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -H 'Idempotency-Key: native-reset-user-123' \
  -H 'Content-Type: application/json' \
  -d '{"from_address":"welcome@your-owned.example","to":"person@example.com","subject":"Reset","html":"<a href=\"https://your-owned.example/reset\">Reset</a>","track_opens":true,"track_clicks":true}'
curl -sS "$BASE/api/transactional/MESSAGE_ID" -H 'Authorization: Bearer YOUR_API_KEY'
curl -sS -b "$COOKIE_JAR" "$BASE/api/transactional?limit=20&offset=0"
curl -sS -b "$COOKIE_JAR" "$BASE/api/transactional/stats"
```

Native first responses are 202 message objects with id, status, attempts, timestamps and tracking metadata. The worker can advance status before the response is loaded. Compatibility always reports its acceptance receipt as queued with mode=engine; inspect the ID for current state. Queued does not guarantee delivery.

Sending keys can submit through both APIs and read a message by ID. List/stats require session or full_access. Sending keys cannot create/manage campaigns or manage keys/domains/webhooks. Transactional quota is reserved atomically at acceptance; broadcast quota is reserved before provider dispatch. Trial exhaustion returns 402, daily exhaustion 429.

Both flows require a workspace-owned verified sender. Broadcasts exclude marketing opt-outs and inactive contacts. Transactional sends block bounced, complained and manually suppressed addresses, but allow marketing unsubscribes. They create no marketing contacts and add no newsletter footer. Sender identity and suppression are rechecked before dispatch, including after waiting on the shared gate.

The shared PostgreSQL-backed gate prioritizes waiting transactional work, yielding a broadcast turn after eight urgent grants when bulk is ready. All sending instances share token/quota accounting and workspace pacing. In-flight calls are not preempted. Broadcasts retain 90% SES daily headroom; transactional work can use the remainder up to the hard limit. Expired pre-provider ownership safely requeues; provider-started ambiguous outcomes become unknown_outcome without automatic retry, retaining quota conservatively. Definitive failures release transactional quota; retries keep the same ID/reservation. Transactional queue age is limited to one hour. See docs/multi-instance-handoff.md for lease/deployment boundaries.

New webhook bodies after migration 10 include:

```json
{"id":"event-id","version":"1","type":"email.delivered","created_at":"timestamp","data":{"message_id":"transactional-message-uuid","message_type":"transactional","email_id":null,"recipient_id":null}}
```

Broadcast bodies have message_type=broadcast, message_id=null, and the original email_id/recipient_id. Workspace events have message_type=workspace. Webhook-Id is the delivery UUID; body id is the event ID. Historical pending bodies are unchanged and may omit these new fields. Never reinterpret email_id as a transactional ID.

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
- Existing API token buckets apply: paid keys default to 20 requests/sec with burst 40; trial keys 2/sec with burst 5; dashboard sessions use the paid API rule. Public endpoints default to 10/sec with burst 30. HTTP limits remain per process; coordinate them at ingress if a cluster-wide request budget is required. Sending admission is separately PostgreSQL-backed across instances.

## Error contract

Errors are JSON `{"error":"snake_case_code"}`.

| Status | Examples |
|---|---|
| 400 | `invalid_json`, `invalid_idempotency_key` |
| 401 | `unauthorized` for missing/invalid/revoked credentials |
| 402 | `trial_quota_exceeded` |
| 403 | `insufficient_scope`, `incorrect_password` |
| 404 | `not_found`, including IDs owned by another workspace |
| 409 | `idempotency_conflict`, `idempotency_key_reused`, `legacy_stub_key_requires_new_key`, `workspace_sending_paused`, `webhook_endpoint_limit`, `webhook_secret_rotation_required` |
| 413 | `body_too_large` (JSON limit 1 MiB) |
| 422 | `invalid_email_request`, `sender_not_verified`, `recipient_suppressed`, `invalid_webhook_url`, `invalid_webhook_events`, `invalid_mail_from_domain` |
| 429 | API rate limit or `daily_limit_reached` |
| 500 | `internal_error` |
| 502 | `ses_request_failed` |
| 503 | `webhooks_not_configured`, `aws_region_required_for_preview` |

## Reproducible demo and engine handoff

Run tests with `TEST_DATABASE_URL` pointing to disposable PostgreSQL. These use a real database and fake network/provider adapters:

```bash
go test ./cmd/server -run 'TestDeveloperContractEndToEndWithRealLogEngine|TestBroadcastAPIToSignedWebhook|TestTrackingEventsReachSignedCustomerWebhook' -v
go test ./internal/webhooks -run 'TestDurableWebhookFlow|TestSNSDeduplicationQueuesOneDelivery' -v
```

The end-to-end tests create a key, verify a domain with the local identity provider, submit an authenticated request to the real queue, dispatch through the shared log sender, process simulated feedback and validate signed customer webhooks. Broadcast coverage includes import, campaign/draft changes, launch/pause/resume and reports with blocked recipients excluded. Tracking tests hit the actual signed pixel/link routes. These prove local (log driver) behavior, not AWS delivery or DNS ownership.

Migrations 1–10 run at startup. Krishna versions 6–8 remain unchanged; Diya's transactional schema is 9 and the webhook payload extension is 10. Krishna v8 upgrades preserve receipts and historical payload bytes. Diya-origin databases whose version 6 means transactional schema require a separate operator-designed migration path; do not use this chain or mark versions manually. Rollback tests use disposable fixtures; reverting migration 9 removes transactional data.

The real adapter is wired; no engine integration contract remains pending. Production AWS/DNS/IAM configuration, custom MAIL FROM provider setup and a live receiver smoke test remain operator tasks. OpenAPI is in docs/openapi.yaml; evidence and implementation decisions are in docs/integration-handoff.md. Manual dashboard routes are /transactional/, /transactional/view/?id=MESSAGE_ID and /activity/.
