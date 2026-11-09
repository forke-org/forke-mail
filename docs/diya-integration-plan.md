# Integration plan: Diya engine + Krishna developer APIs

Status: approved and implemented locally on krishna on 2026-11-09. This document preserves the proposed plan; docs/integration-handoff.md records actual outcomes, tests, limitations and proof. No push or live SES send was performed. Existing working-ss assets are preserved.

## Inspected versions

- krishna: 678870e718d4c5f577c96059714b36eef8ded658
- origin/advik: a7d4f0c626369a3241aa168d241cdb2499e0f08d
- origin/diya: f704c79db0313f15b81c86986653fb284accaf94

Important discovery: the latest Diya commit is named "transactional email engine on top of advik's forke mail base". It contains the real transactional queue/dispatcher, priority integration, idempotency, tracking, APIs and dashboard. The Advik base contains the broadcast engine. Integrate based on actual code rather than inferring module ownership from branch names.

The branches share initial commit 61abec1, but Advik's implementation commit is not an ancestor of Diya's branch: Diya imported its contents into a separate commit. Expect add/add conflicts, not a clean incremental merge.

## Intended final flow

```text
Application or dashboard
    -> shared key/session authentication and route permissions
    -> verified sending identity
    -> broadcast API -> campaign audience snapshot -> paced bulk worker
    -> transactional API -> canonical accept/idempotency -> priority dispatcher
    -> same Sender implementation and global SES rate/quota accounting
    -> SES/log provider feedback and supported tracking
    -> shared email_events with correct broadcast or transactional ID
    -> durable signed customer webhooks
```

Keys, domain verification and customer-webhook management remain shared platform features. No duplicated sender, direct provider call from an HTTP handler, separate global limiter or second transactional idempotency store will be introduced.

## Phase 1: protect the baseline and merge history

After review approval, confirm krishna and capture clean tracked-file status plus all three remote hashes. Fetch current refs again, inspect any new changes and preserve reporting assets. Work and commits remain exclusively on krishna. Neither teammate branch will be edited, reset, rebased or pushed.

Merge origin/diya into krishna with conflicts resolved locally. Preserve our key/cache protection, domain diagnostics, customer webhook implementation, docs and existing migrations. Preserve Diya's transactional engine, priority/shared-sender changes, tracking and frontend pages. Resolve duplicated Handler/Send definitions deliberately; never retain two implementations that both register the same route or enqueue the same request.

Likely conflict points: AGENTS.md, README.md, cmd/server/main.go, cmd/server/routes.go, internal/auth/scopes.go, internal/transactional/handler.go and the transactional package's shared type names. Review semantic integration even in automatically merged files.

Acceptance: both remote hashes remain unchanged by our actions, original broadcast features remain present and all duplicate symbols/routes have a single owner. Commit a buildable/tested merge, not unresolved source conflicts.

## Phase 2: reconcile schema versions before starting the merged server

Both branches have migration 00006 with different meanings:

- Krishna: 00006_customer_webhooks.sql, followed by 00007_transactional_api.sql and 00008_webhook_retention.sql.
- Diya: 00006_transactional.sql, creating transactional_messages and extending events/logs.

Preserve Krishna's already-published 00006-00008 unchanged. Adopt the incoming transactional schema as the next available new migration (currently 00009) on krishna, preserving its Up/Down behavior. The merged migration directory must not contain two files with version 6. This only changes the integrated copy/history on krishna; origin/diya stays intact.

Add a subsequent new migration (currently 00010) for customer webhook payload changes. Confirm numbers again before implementation in case teammates add migrations.

Test fresh database installation and upgrade from the actual Krishna 00008 schema with existing data. Also inspect the schema/migration ledger before using any database created directly from Diya: its version 6 means something different. Do not assume it can run the Krishna upgrade path, reset it, or silently mark migrations as applied. A Diya-origin database requires a separately reviewed reconciliation path if one must be supported.

Acceptance: unique migration versions; fresh install and Krishna upgrade preserve data; rollback/reapply works in disposable schemas. No tests or startup use shared/production data.

## Phase 3: use Diya's real transaction service behind both API entry points

Diya's canonical transactional acceptance is currently package-private accept(ctx, tx, who, key, req). It already validates sendability, handles key/content dedupe, reserves quota, creates transactional_messages and records queued. Its real dispatcher uses the shared Sender and sender.Worker priority gate.

Expose a small service facade around this acceptance path. It returns the actual engine-generated message ID and replay outcome and supports enqueue within one pgx transaction. Wake the dispatcher only after commit; polling remains the recovery path. Do not force the old stub's preassigned receipt UUID onto a second independent engine record.

Retain Diya's existing routes and UI contract:

- POST /api/transactional
- GET /api/transactional
- GET /api/transactional/stats
- GET /api/transactional/{id}

Keep POST /api/emails as a compatibility adapter for our previously documented API:

| Contract difference | Compatibility behavior |
|---|---|
| Our from vs Diya from_address | Map explicitly at the HTTP boundary |
| Our required key <=128 vs Diya optional key <=256 | Keep existing /api/emails validation; retain native behavior on /api/transactional |
| Our queued response vs Diya detailed message response | Preserve each endpoint's documented response while returning the same real message ID |
| Our Idempotency-Replayed vs Diya Idempotent-Replayed | Preserve the existing header for each route; document both |
| Different conflict error codes and replay HTTP status | Normalize only in the compatibility adapter, not through a second store |

Both routes use transactional_messages as the single authoritative idempotency store. No double quota reservation. Update the old EmailEnqueuer contract where required to return the canonical acceptance ID/outcome, or replace the stub-only boundary with the shared facade; do not preserve an incompatible abstraction just for appearance.

Retire normal-runtime use of the development stub. Keep fakes only where explicitly needed by tests. Existing transactional_requests records are old receipts, not unsent email jobs: never replay or convert them into real sends automatically. Define an explicit conflict for unexpired legacy stub keys or require a fresh key. Preserve/archive the old table through forward-only schema changes; do not rewrite migration 00007.

Acceptance: real log-driver transactional dispatch replaces queued-only stub responses; IDs agree across API, storage, events, detail endpoint and webhook. Repeated requests through either route cannot generate duplicate messages.

## Phase 4: broadcast API compatibility and sender behavior

The broadcast endpoints already exist and should be verified with API keys end to end:

- contacts import, campaign create/read, draft email create/update;
- POST /api/emails/{id}/send;
- pause/resume, report and recipient status.

Keep the intended scope table. A sending-only key is not currently allowed to create/manage campaigns; developers provision campaigns through a session/full-access key, then may use sending access for the existing permitted send operations. Do not silently broaden permissions merely to simplify a demo.

Use Diya's shared priority gate and quota accounting for urgent versus bulk sends. Verify that priority applies to waiting work; do not claim already-dispatched provider requests are preempted. Check retry/cancellation/quota boundaries, trial fairness and broadcast progress under sustained transactional traffic.

Suppression behavior intentionally differs: broadcasts exclude unsubscribed contacts, while Diya's transactional path blocks bounced/complained/manual suppressions but permits marketing opt-outs. Remove the old stub wrapper's blanket inactive-contact check so it does not accidentally change the engine's policy. Password resets must not create marketing contacts or receive newsletter unsubscribe footers. Recheck sender eligibility and suppression around dispatch and preserve safe handling of ambiguous send outcomes.

Acceptance: both send paths work through one shared sender/quota system, with independent appropriate recipient policies and no broadcast regression.

## Phase 5: forward both streams to customer webhooks

Diya adds email_events.message_id for transactional messages; broadcast events still use email_id and recipient_id. Our current queue_customer_webhooks trigger copies only the latter two. Without an update, transactional events can be delivered without the identifier needed to correlate them.

Update the trigger in a new migration to add message_id and an explicit message type to newly generated payloads. Keep broadcast email_id/recipient_id intact, stable event/delivery IDs, signatures, retry behavior and existing stored payload bytes. Do not rewrite historical pending deliveries or reinterpret email_id as a transactional ID without a documented compatibility rule.

Diya implements opened/clicked through signed tracking links/pixels, not newly enabled SES Open/Click ingestion. Add email.opened and email.clicked to allowed customer subscriptions, docs and OpenAPI only after confirming this source path end to end. Opens/clicks are best-effort client signals, not proof of human engagement; invalid tokens must not create events. Do not fabricate tracking outcomes for broadcasts, which have no equivalent tracking implementation in this branch.

Preserve authenticated incoming SNS handling, tenant-scoped provider correlation and duplicate-notification protection for both tables. Customer HTTP failure must not block provider acknowledgement or email dispatch. Preserve encrypted endpoint secrets, URL/network protections, bounded retries and replay-verification documentation.

Acceptance: broadcast and real-engine transactional delivery/bounce events arrive at a fake customer's receiver with the correct IDs and valid signatures; tracking events arrive only when supported and enabled; receiver outages do not stop mail processing.

## Phase 6: regression tests and working demos

Use real disposable PostgreSQL plus the shared log sender/fake provider clients. These are future checks, not tests already run during this planning turn.

1. All existing Krishna and Diya unit/integration tests; build, vet, formatting and deadcode checks.
2. Fresh migrations, version-8 upgrade with rows, rollback/reapply and no duplicate versions.
3. Key lifecycle and scopes across both send APIs, cross-tenant message/report/stat/webhook isolation.
4. Broadcast: import -> campaign -> audience -> send -> outcome -> signed webhook; blocked recipients never sent.
5. Transactional: key + verified domain -> real acceptance -> priority/log sender dispatch -> sent/provider ID -> feedback -> detail/status -> signed webhook. No production path depends on fixtureEngine or DevelopmentStub.
6. Same-key concurrency and cross-route retries create one job, return the same message ID and reserve quota once. Changed payload conflicts. Legacy stub keys cannot silently suppress a real send.
7. Duplicate SNS notifications, failed customer receiver, queue restart recovery, SSRF and signature tests remain valid.
8. Priority and rate/quota limits with both worker types active, ambiguous outcomes not resent blindly, and no unintended marketing footer/contact creation.
9. Signed tracking token validation, supported open/click webhook IDs and scanner/privacy caveats.
10. Frontend build and transactional list/detail/activity pages after integration.

Attempt race tests if a working 64-bit C compiler is available; otherwise record the exact blocker. Live SES testing is separate and requires configured access; do not equate a log-driver demo with inbox delivery.

Acceptance: both pipelines demonstrated against the merged code using actual engine components, with new API screenshots and test evidence clearly labelled local versus live.

## Phase 7: documentation, commits and branch proof

Update quickstart, OpenAPI, README, AGENTS and handoff notes for the shared API contracts, real engine wiring, migration path, suppression differences and supported tracking. Replace previous "production engine unavailable" statements only when the real adapter actually works. Keep optional AWS/custom MAIL FROM operation boundaries accurate.

Use reviewable commits for merge/schema reconciliation, canonical service + adapter, shared events/webhooks, regression tests and docs/UI verification. Before any later push, check branch/status and git log origin/advik..krishna, then push only origin krishna. Record origin/advik and origin/diya hashes before and after; remote branches remain read-only.

## Review decision

Recommended direction: preserve Diya's native transactional API/UI and engine as canonical, add our /api/emails compatibility adapter, and connect both event streams to the existing developer-platform features. No implementation begins until the user reviews this plan.
