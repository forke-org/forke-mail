# Local worker safety and independent verification

Follow-up to dfc782f, on krishna only. Evidence is local (log driver/fake provider) against disposable PostgreSQL. No live SES or shared/production database is part of this work. The user subsequently authorized the final push to krishna only; teammate branches remain read-only. The original Diya integration was not redone.

## Worker audit

| Component | Previous behavior | Current ownership/recovery | Proof |
|---|---|---|---|
| Transactional dispatcher | Atomic UPDATE / SKIP LOCKED claim; every stale sending row failed unknown | Random claim token; provider-start boundary committed before external calls; token/status fences for all terminal/retry/quota writes. Expired pre-provider claims requeue; expired started/legacy claims fail unknown and are never resent | TestThreeDispatchersSendOnceAndReserveOnce; TestTransactionalCrashRecoveryRejectsStaleClaim; existing unknown-outcome tests |
| Broadcast workers | Atomic SKIP LOCKED batches; local workspace/global pacing; stale rows always unknown | Atomic tokenized claims, shared DB workspace pacing, quota+provider-start+permit-start transaction. Old owners cannot unclaim/complete a replacement claim. Same safe/ambiguous lease split | TestTwoBulkWorkersDispatchEachRecipientOnce; TestBulkCrashRecoveryFencesOldOwner |
| Customer webhook dispatcher | SKIP LOCKED delivery/endpoint rows, lease tokens and bounded retries | Existing storage contract preserved. Added current-lease check before HTTP and absolute request deadline anchored to claim time; stale owners cannot start a new attempt. Final update remains lease-token fenced | TestTwoWebhookWorkersClaimEachAttemptOnce; TestWebhookExpiredClaimRecoveredAndOldOwnerCannotSend |
| Global send admission | Process-local token bucket, quota counters and priority | PostgreSQL sending_control row serializes token replenishment, quota reservation and priority across all instances. Durable sending_permits retain uncertain usage; leased waiters bound dead-owner priority blocking | TestSharedRateQuotaHoldAndPermitRecovery; shared sender and dispatcher tests |
| Workspace quota | Atomic workspace/usage_daily reservations | Same atomic model. Broadcast reservation moved into the fenced start transaction; transactional reservation remains exactly once at acceptance | Existing quota/idempotency tests and multi-worker quota assertions |
| Recovery / retention | Each process ran its own sweep/maintenance | Transaction-scoped advisory locks cover each batch; mutations are conditional/idempotent. Finished admission history pruned in bounded batches after 25 hours; dead waiters removed | TestMaintenanceAdvisoryLockExcludesOtherInstance; queue/lease tests |
| Identity poller | Every process polled pending identities | Advisory lock around a poll pass; provider reads/status persistence remain idempotent | Advisory-lock exclusion test; domain regression suite |
| Migration startup | Goose Up without an application-level cross-instance guard | Dedicated session advisory lock scoped to current_schema serializes startup migration runs; released on exit/connection loss | Fresh/v8/rollback migration tests and normal multi-worker setup |
| SES account monitor | Per-instance read-only GetAccount snapshots | Read-only polling can run on each node. Snapshot begins at DB time; older snapshots cannot overwrite newer shared control state. Admission conservatively includes local completed/in-flight usage around the observation | Mock SES tests and shared quota tests |
| API/IP limiter janitors | Per-process maps | Remain per process; each janitor owns only its own memory. No singleton lock is appropriate | Existing rate-limit tests |

## Shared accounting and limits

Production workers use only PostgreSQL admission; the in-memory token bucket exists only for pool-less unit fixtures. Broadcast and transactional workers call the same gate. There is no parallel second production limiter. Provider calls run outside SQL transactions, while a short transaction commits ownership and the provider-start boundary first.

All instances for one sending account must share the same database/schema, sender driver and configured account/rate policy. A token bucket allows a burst of max(1, floor(rate)), then replenishes from the database clock. Workspace broadcast pacing is persisted too. Waiting transactional work has priority, with a bulk turn after eight urgent grants when bulk is ready. Priority cannot preempt provider calls already in progress.

The shared SES daily check includes durable active permits and the conservative maximum of the local rolling 24-hour ledger versus the observed SES count plus local completions since observation began. In-flight/ambiguous calls cannot be refunded by a late owner. Snapshot overlap may conservatively reduce available capacity. External senders using the same SES account outside this database are outside admission control; use a dedicated account/policy and retain SES-side enforcement.

Provider requests have a 30-second timeout. Queue claims expire after five minutes: pre-provider ownership can recover without consuming quota twice; provider-started ownership becomes unknown_outcome and is never blindly resent. Admission reservations expire after 30 seconds before start; started permits remain conservatively consumed after their five-minute deadline. Recovery loops run once per minute. Dead priority waiters expire after 30 seconds.

Webhook requests have a ten-second deadline anchored to their claim and a 30-second recovery lease. Customer webhooks remain at-least-once across ambiguous receiver/crash outcomes; receivers must deduplicate Webhook-Id. Two workers cannot claim the same current attempt. No implementation can guarantee exactly-once external receiver side effects after a lost acknowledgement without receiver cooperation.

Migration 11 adds ownership/start columns and shared gate tables. Versions 1–10 are unchanged, including protected 6–8. Existing in-flight rows are backfilled as potentially provider-started. Stop older binaries before enabling the new multi-instance deployment: an older binary does not implement the new fencing/admission contract. Revert migration 11 only with all workers stopped and only when accepting loss of its admission history; rollback tests use disposable schemas.

API/IP token buckets remain per-process. Use coordinated ingress limits if a cluster-wide HTTP request limit is required. The previous requirement to run one sending server is replaced by the shared database/account/configuration requirement above; public API shapes and key scopes are unchanged.

## Race toolchain

Existing C:/msys64/ucrt64/bin/gcc.exe is MSYS2 GCC 15.2.0, target x86_64-w64-mingw32. Only process CC/PATH/CGO variables were changed; no compiler installation or system-wide settings were needed. The initial full race run passed the existing 103 tests. Final full/critical-package results are recorded in docs/evidence/multi-instance-20261109 and the verification-run logs.

Commands use CC=C:/msys64/ucrt64/bin/gcc.exe and CGO_ENABLED=1. The concurrency-critical repeat is go test -race -count=5 ./internal/transactional ./internal/sender ./internal/workspaces ./internal/webhooks ./internal/feedback. Build/test parallelism and heap limits were reduced to fit the machine's commit/pagefile limit; earlier unsuccessful runs are retained, including an initial workspace-readiness calculation fixed before successful runs and transient memory exhaustion. No test assertion was deleted or relaxed.

Database fixtures use a separate schema per test name/process. Multiple fixture calls within the same test still share that schema, preserving cross-workspace isolation tests. This isolates account-wide admission state between tests. Six test-pool connections bound local PostgreSQL resource usage; production keeps its existing connection configuration.

## Run the independent verifier

Use Go 1.26+, Git, local PostgreSQL and a role with CREATEDB rights. The scripts refuse non-local hosts, fallback hosts or database names without verify_. Use a throwaway administration database, not postgres or a production database.

PowerShell example for the existing local test cluster:

```powershell
& 'C:\Program Files\PostgreSQL\18\bin\createdb.exe' -h 127.0.0.1 -p 55439 -U forkemail verify_admin
$env:VERIFY_DATABASE_URL='postgres://forkemail@127.0.0.1:55439/verify_admin?sslmode=disable'
$env:VERIFY_CC='C:/msys64/ucrt64/bin/gcc.exe'
./scripts/verify-integration.ps1
```

Bash/Git Bash:

```bash
export VERIFY_DATABASE_URL='postgres://forkemail@127.0.0.1:55439/verify_admin?sslmode=disable'
export VERIFY_CC='C:/msys64/ucrt64/bin/gcc.exe'
bash scripts/verify-integration.sh
```

Each invocation creates its own verify_timestamp_random database, runs checks, and drops only that owned database. The verify_admin connection is never migrated or used for test rows; it only creates/drops the generated database. Concurrent standalone servers must not point to a verifier-owned database. Evidence defaults to docs/evidence/verify_timestamp_random; VERIFY_EVIDENCE_DIR can choose another output directory. VERIFY_EXPECTED_ADVIK, VERIFY_EXPECTED_DIYA and GO_BIN are configurable. The recorded protected assets must be copied along with this workspace when verifying elsewhere; missing or changed assets fail verification.

The real HTTP check starts httptest TCP servers over the actual router/shared log sender and a local fake receiver. It creates a key and domain through HTTP, retries the same request across both send routes, checks one canonical row/reservation, verifies the signed message_id webhook, and checks sending-key campaign creation is forbidden. Only the test transport maps the public-looking webhook URL to the local fake receiver; production SSRF rules remain intact.

Every check prints PASS/FAIL, with SKIPPED only for an unavailable race toolchain. A failed required check or cleanup exits nonzero. Raw command output is saved separately. The two entry scripts share the Go verifier implementation so they enforce the same database guards, hash checks, migrations, HTTP flow and tooling checks.
