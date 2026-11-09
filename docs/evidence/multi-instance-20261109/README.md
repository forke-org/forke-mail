# Local follow-up evidence

All runs use local disposable PostgreSQL, the log driver and fake providers/receivers. No live SES or shared/production database is used. The user subsequently authorized pushing the finished krishna branch only.

- toolchain.txt: existing 64-bit MSYS2 GCC selection and Go environment; no installation was needed.
- race-baseline.jsonl: initial integrated baseline, 103 tests passed under race.
- race-critical-five.jsonl: transactional dispatcher/acceptance, sender/priority, workspace quota, customer webhook and SNS packages repeated five times under race; 320 tests passed, no failures/skips.
- startup-race.txt: concurrent startup migration test passed under race.
- powershell/tests.txt and powershell/race.txt: full normal and race runs, 123 passed each. The initial verifier failed its deadcode check due memory exhaustion; that failure is retained in verify-powershell.txt and powershell/deadcode.txt.
- deadcode-token.txt: standalone deadcode success after replacing the verifier's full AST dependency with token-based source scanning. Empty output with successful exit means no dead code.
- verify-powershell-final.txt and powershell-final/: final PowerShell entry point output and raw command logs.
- verify-bash.txt and bash/: Bash entry point output and raw command logs.
- tests.jsonl: earlier 119-test normal run before the final additional verification/startup tests.
- implementation-tests*.txt: intermediate failures retained, including the corrected workspace-readiness comparison and transient machine commit/pagefile exhaustion. Resource usage was reduced and tests repeated; assertions were not weakened.
- race-working-tree-build.jsonl: intermediate verifier build failure from adding a source file during an in-flight test build. The final frozen-tree runs include that file and are authoritative.
- Each final verifier summary.json records individual PASS/FAIL/SKIPPED checks. Missing/skipped required contract tests cannot produce PASS.

The protected 20-file manifest is read from the earlier diya-integration-20261109 preservation-proof.json. Source/worker audit and script usage are in ../../multi-instance-handoff.md. Final Git/push proof is supplied separately after the final commit to avoid a self-referential commit hash.
