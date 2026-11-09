# Diya engine integration: local handoff

Implemented on krishna on 2026-11-09. All delivery evidence here is **local (log driver)** or fake network/provider evidence. No live SES sending, remote push or shared/production/Diya-origin database was used. Existing working-ss directory and zip were preserved.

This records the original integration at dfc782f. The subsequent [multi-instance follow-up](multi-instance-handoff.md) replaces the earlier race-toolchain blocker and single-sender-instance limitation with current evidence, migration 11 and independent verification scripts.

## Outcomes by approved step

| Step | Status | Result and evidence |
|---|---|---|
| 1: merge/migrations | Done | Diya history merged into krishna. Unique versions 1–10; fresh install, real v8 schema with seeded receipts/payload, rollback/reapply tested. Migrations 6–8 unchanged. |
| 2: canonical API | Done | Both routes call Service.Accept over Diya's acceptance transaction. Same real ID, one quota reservation, post-commit notification and polling recovery. Legacy stub keys conflict; no runtime stub. |
| 3: broadcast/sender | Done | Full-access/sending-key broadcast flow, blocked audience, pause/resume, reports, shared trial quota and dispatch checks pass. One global gate counts in-flight slots; bounded urgent bursts yield bulk turns. |
| 4: shared webhooks | Done | Migration 10 extends new bodies only. Real signed tracking routes create open/click events with canonical IDs. SNS verification/deduplication, tenant correlation, signatures, receiver failure, retries and SSRF remain covered. |
| 5: regression | Done except race | Full database-backed suite, build, vet, gofmt, deadcode and frontend export exercised. Race build is blocked by the installed 32-bit compiler; no race safety claim. |
| 6: docs/UI/proof | Done | Quickstart, OpenAPI, README and AGENTS updated; new API and dashboard evidence lives separately from old assets. |

Evidence folder: [diya-integration-20261109](evidence/diya-integration-20261109/). Machine-readable final counts are in summary.json, full test events in tests.jsonl, repeated pipeline tests in pipelines.txt, browser checks in browser-checks.json and redacted real HTTP responses in http-demo.json. PNG names start local-log-. Old assets are not substituted with new captures.

## Baseline and branch safety

| Ref | Before |
|---|---|
| krishna | 678870e718d4c5f577c96059714b36eef8ded658 |
| origin/advik | a7d4f0c626369a3241aa168d241cdb2499e0f08d |
| origin/diya | f704c79db0313f15b81c86986653fb284accaf94 |

Fetch verified the expected remote hashes before mutation. Neither teammate branch was checked out, edited, reset, rebased or pushed. Commits were made only on krishna. Final Git output is supplied alongside the final report, including the final documentation commit. Preservation-proof.json compares SHA-256 before/after for migrations 6–8 and every original reporting asset (20 files total).

## Merge conflicts and resolutions

All conflicts were resolved in the krishna merge. Teammate references were read-only.

| Conflicting paths | Resolution |
|---|---|
| .env.example; AGENTS.md; README.md | Kept Krishna configuration/documentation, then updated current engine documentation. Legacy stub-enabled flag now explicitly errors. |
| cmd/server/main.go; cmd/server/routes.go | Combined Krishna developer services/auth routes with Diya dispatcher/tracking routes. One sender instance, one Worker/global limiter, one native dispatcher; register compatibility once. |
| internal/auth/scopes.go | Kept Krishna permissions and added Diya transactional routes. Sending-only campaign creation remains denied. |
| internal/auth/scopes_test.go | Adopted Diya tests and retained/extended route security coverage through API integration tests. |
| internal/domains/records.go; internal/domains/records_test.go | Kept Krishna DNS diagnostics and default/custom MAIL FROM boundaries. |
| internal/keys/handler.go; internal/keys/rotate.go | Kept Krishna key protections and one-time secret/no-store behavior. |
| internal/transactional/handler.go | Diya native Send/Get/List moved into native_handler.go under the existing Handler type. CompatibilityHandler owns only POST /api/emails; both call Service.Accept. Removed old enqueue/stub implementation. |
| frontend/components/Icon.js; frontend/components/Layout.js; frontend/components/ui.js; frontend/lib/api.js; frontend/pages/activity.js; frontend/public/app.css | Adopted Diya dashboard/navigation/event support. |
| internal/emails/validate.go | Adopted Diya shared sender-identity validation. |
| internal/events/handler.go; internal/events/record.go | Adopted Diya shared broadcast/transactional event model. |
| internal/feedback/processor.go; internal/feedback/reputation.go | Adopted Diya provider correlation/reputation over both message tables; SNS authentication and duplicate protection retained. |
| internal/sender/limits.go; internal/sender/logsender.go; internal/sender/process.go; internal/sender/sender.go; internal/sender/worker.go | Adopted Diya shared sender/priority primitives, then fixed in-flight quota accounting, bounded bulk progress, eligibility recheck and unknown-outcome safety. |
| internal/ses/sender.go | Adopted Diya transactional message tagging through the same Sender implementation; no direct SES HTTP-handler calls. |
| internal/workspaces/quota.go | Adopted Diya transaction-compatible shared quota reservations. |

Diya's incoming 00006_transactional.sql was renamed to 00009_transactional.sql with its Up/Down body preserved. Krishna 00006–00008 were untouched. The renumbering is included in the buildable merge commit; the next commit adds migration reconciliation/upgrade tests. This differs slightly from placing the file rename itself in the second commit, and keeps the merge compilable and testable.

## Single ownership and shared module edits

- POST /api/transactional: transactional.Handler.Send in native_handler.go.
- GET native list/detail/stats: the same Handler, plus stats.go.
- POST /api/emails: transactional.CompatibilityHandler.Send; maps from to from_address and preserves compatibility headers/status/errors.
- Durable transactional acceptance: transactional.Service.Accept → accept transaction → transactional_messages. The legacy table is read only to reject conflicting stub receipts.
- Dispatch: one transactional.Dispatcher and broadcast Worker use the same Sender and global gate constructed in main.go. No new SES client/limiter/idempotency store was introduced.
- Shared sender edits after merge: priority.go, limits.go, worker.go, process.go and sweeper.go. The later multi-instance follow-up replaces ReleaseSend with owner-bound Permit.Start/Finish and durable shared admission. SES and feedback modules adopt Diya's incoming integration rather than a competing sender.
- Customer webhook ownership: internal/webhooks and queue_customer_webhooks trigger; migration 10 replaces the function for future events only. SNS handling remains separate from customer HTTP dispatch.
- Key lifecycle remains owned by internal/keys and auth. Management scope was not broadened.
- UI verification found static chunks consuming the public endpoint bucket. Dedicated GET/HEAD /_next/static/* serving avoids dashboard hydration failures; API/public action rate limits remain intact, with a regression test.

## Final migration directory

```text
00001_workspaces.sql
00002_contacts.sql
00003_campaigns.sql
00004_events.sql
00005_ses.sql
00006_customer_webhooks.sql
00007_transactional_api.sql
00008_webhook_retention.sql
00009_transactional.sql
00010_shared_webhook_payload.sql
```

Fresh and Krishna-history databases are supported. Do not apply this chain to a database initialized from Diya's differently numbered version 6. A separate data migration must be designed for that case. Rollback/reapply evidence uses isolated disposable schemas, not production data; reverting version 9 drops transactional data.

## Actual validation and limitations

Tooling: portable Go 1.26, PostgreSQL 18 on 127.0.0.1:55439, Node 24 and Next.js production webpack export. Docker was absent, so native PostgreSQL provided the disposable test environment. Database name: integration_krishna_20261109. Tests and the demo server were run serially; never run an independent server's background outbox worker against the test database during a test run.

```powershell
$env:PATH='C:\projects\Forke\tools\go\bin;'+$env:PATH
$env:GOMAXPROCS='1'
$env:GOFLAGS='-p=1'
$env:GOMEMLIMIT='512MiB'
$env:CGO_ENABLED='0'
$env:TEST_DATABASE_URL='postgres://forkemail@127.0.0.1:55439/integration_krishna_20261109?sslmode=disable'
go build ./...
go test -json -count=1 ./...
go vet ./...
gofmt -l cmd internal migrations frontend
go run golang.org/x/tools/cmd/deadcode@v0.43.0 -test ./...
```

Final suite: 103 tests passed, 0 failed, 0 skipped; 16 tested packages passed and 12 packages had no test files. Six key pipeline/router tests passed on each of three repeated runs. An additional real concurrent urgent/bulk load test passed five repeated runs, verifying bulk progress within the shared token bucket. Build, vet and deadcode passed; gofmt output is empty after normalizing checkout CRLF in Go files (no additional Git source delta). Four dashboard captures and four repeated navigation checks passed without page/console/network errors.

Frontend: npm ci, npx next build --webpack, then create out/.keep on Windows. All 17 static pages exported; npm audit reported zero vulnerabilities. OpenAPI YAML parsing and route/schema checks also ran.

Historical race attempt at dfc782f used the 32-bit MinGW compiler and failed to build runtime/cgo. The follow-up found existing C:/msys64/ucrt64/bin/gcc.exe, target x86_64-w64-mingw32. Full normal/race runs now pass 123 tests each, and concurrency-critical packages pass 320 tests across five repeats. See multi-instance-handoff.md and its raw evidence for the current results.

The first browser repeat exposed static-chunk 429s, subsequently fixed and retested. A repeat shared-quota test originally waited for message logging rather than feedback completion; it now waits for both delivered outcomes before asserting. A concurrent standalone server also consumed test webhook jobs, so the server was stopped and test runs repeated in isolation. The pre-fix targeted failure log is retained as pipelines-before-fix.txt; final results are in pipelines.txt/tests.jsonl. This does not weaken receiver-isolation assertions.

## Remaining operator work and risks

No engine adapter/wiring contract remains pending. Live SES delivery, real SNS infrastructure, DNS ownership/DKIM, custom MAIL FROM provider configuration and a real customer HTTPS receiver smoke test were not exercised. Default SES MAIL FROM remains distinct from custom DNS preview, consistent with the [AWS guide](https://docs.aws.amazon.com/ses/latest/dg/mail-from.html).

The follow-up now provides PostgreSQL-backed sending admission across instances sharing the same database/schema/account/configuration. Priority affects waiting work only; existing in-flight calls are not preempted. Bounded bulk turns are not an exact latency SLA. API request buckets remain per process. Conservative unknown_outcome handling can drop uncertain messages; operators must reconcile rather than blindly send again. See the follow-up handoff for current tests and deployment limits.

Keep WEBHOOK_ENCRYPTION_KEY stable in a secret store. Losing it prevents secret decryption. New event payload fields do not backfill old pending deliveries. Receivers must accept both old/new shapes and deduplicate delivery IDs. PostgreSQL capacity is finite; monitor queue age, quota holds, storage and failed deliveries. Signed tracking is an estimate and only implemented for transactional messages.
