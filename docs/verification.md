# Intern 3 verification and handoff

Historical verification from before Diya integration. Current engine contracts, final tests and migration proof are in [integration-handoff.md](integration-handoff.md). The former missing-adapter boundary described below has been replaced by the real shared acceptance service.

## Branch safety

Work is on krishna. Advik was imported only with fetch and merge origin/advik into krishna. No checkout edit, commit, reset, rebase, deletion or push targeted advik.

Starting origin/advik: a7d4f0c626369a3241aa168d241cdb2499e0f08d

Ending origin/advik: a7d4f0c626369a3241aa168d241cdb2499e0f08d

The local advik reference and a final read-only ls-remote of refs/heads/advik also report that hash. Review the additions with git log --oneline origin/advik..krishna and git diff origin/advik --stat.

## Actual verification

- Installed portable Go 1.26.0 at C:/projects/Forke/tools/go. Docker was absent; used installed PostgreSQL 18 in a new isolated test cluster at 127.0.0.1:55439, database forkemail_test. Existing database clusters were not used.
- Passed full go test ./... with TEST_DATABASE_URL set. Also completed an uncached go test -count=1 ./... run. Database tests actually ran, rather than skipping for a missing database URL.
- Passed go vet ./... and go build ./....
- Passed the repository deadcode check using golang.org/x/tools/cmd/deadcode@v0.43.0 -test ./....
- Compiled the Go receiver example extracted from the quickstart with go test on the extracted source file.
- Parsed OpenAPI YAML and resolved all 27 local references.
- Checked formatting of all 126 Go files after normalizing Windows CRLF for comparison: zero unformatted files. Existing clone files have CRLF; raw Windows gofmt -l reports those line endings. Existing shared files were not bulk-reformatted.
- git diff --check passed.
- Race tests: NOT RUN successfully. The attempted race build failed because installed C:/MinGW/bin/gcc is 32-bit: cc1.exe reports "64-bit mode not compiled in". A 64-bit MinGW-w64 toolchain is required before rerunning go test -race ./internal/webhooks ./cmd/server.
- Real SES delivery and real customer endpoint delivery: NOT RUN. Network/provider integrations use test doubles. No AWS bootstrap or live send was performed.
- Frontend build: NOT RUN; frontend source is unchanged. The existing embedded placeholder allows the Go build; build frontend separately to populate the dashboard.

Initial unconstrained parallel compilation hit the Windows pagefile limit. Successful runs used GOMAXPROCS=1, GOFLAGS=-p=1, GOMEMLIMIT=256MiB and GOGC=20. This is a local resource constraint, not a claim that the failed attempts passed.

## Important tested behaviors

- API key creation/one-time secret response, safe listing, rotation, immediate revocation, workspace isolation and management scope denial.
- DNS expected/observed values, missing and mismatched records, conflicting SPF policies, SPF mechanism ordering, custom versus default MAIL FROM and foreign-domain isolation.
- HMAC raw-body integrity and timestamp window, encryption/workspace binding, private and metadata addresses, mixed DNS answers, connection-time rebinding checks and redirect refusal.
- Durable webhook retries and stable IDs/payloads, recovery with a recreated dispatcher, retry exhaustion, rollback atomicity, duplicate SNS notification fanout and concurrent workers claiming one delivery only once.
- Transactional input/authentication, durable idempotency across handler restart and eight concurrent duplicate calls, payload conflict, enqueue rollback, suppression checks, explicit stub mode and production stub rejection.
- End-to-end developer contract with a fake engine: create key -> fake-provider domain verification -> authenticated send -> stable message ID -> simulated provider event -> verified signed customer notification.
- New migrations Up -> DownTo(5) -> Up in a disposable isolated PostgreSQL schema.

## Complete versus pending

Implemented: API-key tests/cache protection, DNS diagnostic API, outgoing webhook management and signed durable dispatcher, transactional HTTP/idempotency boundary, restricted development stub, quickstart, OpenAPI and integration tests.

Pending shared-engine contract: Advik must supply an EmailEnqueuer adapter that persists the real priority job with the supplied pgx transaction and stable message ID. Quota reservation, priority, final transactional suppression policy and eventual events belong to that shared engine. Production direct-send remains 503 until wired. The development stub creates a receipt only and deliberately sends nothing.

Pending provider operations: production AWS credentials/quotas and real-domain verification are deployment work. Custom MAIL FROM is accurate DNS readiness preview, not automatic SES configuration. Open/Click subscriptions remain unavailable until Intern 1 implements their source events; they are not fabricated.

## Shared files changed

- cmd/server/main.go and routes.go: thin registration/startup of extensions plus DNS diagnostic route.
- internal/auth/scopes.go: register the new authenticated route permissions; existing authentication/key validation algorithms unchanged.
- internal/keys/handler.go and rotate.go: only add Cache-Control: no-store to one-time secret responses.
- internal/domains/records.go and records_test.go: remove misleading root-SPF guidance from default SES setup.
- .env.example, README.md and AGENTS.md: configuration/documentation updates.

Existing campaign sending, sender implementation, SES feedback handler and SNS signature verification code were not modified. Customer event fanout is implemented by a new database trigger in the same transaction as event persistence.

## Rerun locally

Start the isolated test server if stopped:

```powershell
& 'C:/Program Files/PostgreSQL/18/bin/pg_ctl.exe' -D C:/projects/Forke/tools/mail-test-pg -l C:/projects/Forke/tools/mail-test-pg.log -o '-p 55439 -h 127.0.0.1' -w start
$env:PATH = 'C:/projects/Forke/tools/go/bin;' + $env:PATH
$env:TEST_DATABASE_URL = 'postgres://forkemail@127.0.0.1:55439/forkemail_test?sslmode=disable'
$env:GOMAXPROCS = '1'
$env:GOFLAGS = '-p=1'
$env:GOMEMLIMIT = '256MiB'
$env:GOGC = '20'
go test ./...
go vet ./...
go build ./...
```

The test cluster contains only generated fixtures and migration-test data. Stop it with pg_ctl -D C:/projects/Forke/tools/mail-test-pg -m fast -w stop when finished.
