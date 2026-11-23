# Forke Mail CLI changelog

## 1.1.0

### New
- Builds for every platform: macOS (Apple Silicon, Intel and a universal binary), Linux (x86, x86-64, ARM64 and ARMv7) as `.tar.gz`, `.deb`, `.rpm` and `.apk`, Windows (x86, x86-64 and ARM64) and FreeBSD.
- Every release ships with a checksums file and a signed build attestation you can check with `gh attestation verify`.
- `forke-mail version` reports the exact release it was built from.

### Changed
- The installer picks the right archive for your system and verifies its checksum before installing.

## 1.0.2

### Changed
- Risky actions are confirmed in the terminal. Ones that can't be undone, such as launching a broadcast or revoking a key, say so before asking. Browser approvals are gone.

### Fixed
- Brief server errors (502, 503, 504) and dropped connections are retried instead of failing the command.

## 1.0.1

### Changed
- The browser page you see after `forke-mail login` now looks like the rest of Forke Mail and says clearly whether sign-in worked.

## 1.0.0

### New
- `forke-mail login` signs in through your browser; `logout` revokes the device.
- `forke-mail init` sets up a project: a test key in `.env`, `.env` in `.gitignore` and notes in `AGENTS.md`.
- `forke-mail send`, `domains`, `whoami`, `tools` and `call <tool>` for every Forke Mail tool.
- `forke-mail mcp` runs a local MCP server for AI apps.
