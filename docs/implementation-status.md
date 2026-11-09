# Intern 3 implementation record

Current status (2026-11-09): Diya's engine is integrated locally on krishna. Both send APIs use real canonical acceptance, shared dispatch/quota and shared signed customer events. See [integration-handoff.md](integration-handoff.md) for completed phases, actual test results, migration safety and remaining operator tasks. No push/live SES/shared database was used.

Base: origin/advik at a7d4f0c626369a3241aa168d241cdb2499e0f08d, merged into krishna without changing advik.

Build commands: go build ./..., go vet ./..., go test ./.... Database tests require TEST_DATABASE_URL. Frontend changes additionally require npm ci and npm run build in frontend.

Local tooling: portable Go in C:/projects/Forke/tools/go; isolated PostgreSQL 18 test cluster on 127.0.0.1:55439. Docker is unavailable; the installed PostgreSQL binaries provide real integration tests without Docker. No production database or AWS account is used.
