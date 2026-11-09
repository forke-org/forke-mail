# Local integration evidence

All screenshots and HTTP data are from **local (log driver)** execution on disposable PostgreSQL. Delivery feedback is simulated. No live SES delivery is claimed. API keys, passwords and session cookies are excluded/redacted.

- local-log-01-transactional-list.png: real transactional list/statistics.
- local-log-02-transactional-detail.png: real queued/sent/delivered timeline and opt-in tracking metadata.
- local-log-03-activity.png: both message types in one event stream.
- local-log-04-broadcast-report.png: one eligible recipient delivered; manually blocked address excluded.
- local-log-05-api-evidence.png: local-log-api-evidence.html rendered from saved actual HTTP responses. It is a verification report, not a production application screen.
- http-demo.json: redacted responses from account/key/domain creation, both send APIs, cross-route replay, contact import/campaign launch and final outcomes. One 400 is strict rejection of an unsupported suppression reason field; the corrected email-only request succeeds. No failing response is hidden.
- tests.jsonl / summary.json: final full-suite results, 103 passed / 0 failed / 0 skipped tests; 12 packages have no test files.
- sustained-load.txt: actual concurrent urgent/bulk scheduling and shared-rate test passed five repeated runs.
- pipelines.txt: six key integration/router tests repeated three times, all pass.
- tests-initial.jsonl: initial 101-test green run before the additional static-assets regression.
- pipelines-before-fix.txt: retained targeted failures from concurrent demo outbox consumption and an early asynchronous feedback assertion. Final runs stop the demo server and await actual delivery outcomes.
- browser-before-fix.json: static JavaScript 429 evidence; browser-checks.json records the clean final captures and repeated navigation after the route fix.
- build.txt, vet.txt, gofmt.txt, deadcode.txt: actual checks; formatting/deadcode success means empty output. Frontend npm ci/webpack static export also passed, with zero audit vulnerabilities.
- race.txt: actual 32-bit GCC blocker. Race tests did not run.
- baseline-files.txt / preservation-proof.json: SHA-256 comparison for the unchanged three published migrations and all original working-ss assets/zip, 20 files total.
- git-before-docs.txt: source commit list and refs before this documentation/evidence commit. Final Git proof is supplied after the final commit.
- diff-vs-advik.txt / diff-vs-diya.txt: tracked branch deltas before the final documentation commit. These do not indicate any remote modification.

Full decisions, conflicts, migration listing and operational limits are in [integration-handoff.md](../../integration-handoff.md). The demo server was stopped after capture; PostgreSQL test data remains disposable. Keep normal test runs isolated from separately running application workers.
