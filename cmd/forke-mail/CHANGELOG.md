# Forke Mail CLI changelog

## 1.1.0

## Install the Forke Mail CLI on any machine

The CLI now ships for **macOS, Linux, Windows and FreeBSD**, as plain archives and as native Linux packages, so you can install it the way you install everything else:

```sh
# macOS and Linux: picks the right file and checks it before installing
curl -fsSL https://mail.forke.space/install.sh | sh

# Debian and Ubuntu
sudo dpkg -i forke-mail_1.1.0_linux_amd64.deb

# Fedora, RHEL and openSUSE
sudo rpm -i forke-mail_1.1.0_linux_amd64.rpm

# Alpine
sudo apk add --allow-untrusted forke-mail_1.1.0_linux_amd64.apk
```

| Platform | Download |
| --- | --- |
| macOS | `macOS_universal.zip` (any Mac), `macOS_arm64.zip` (Apple Silicon), `macOS_amd64.zip` (Intel) |
| Linux | `linux_amd64`, `linux_arm64`, `linux_armv7`, `linux_386` as `.tar.gz`, `.deb`, `.rpm` or `.apk` |
| Windows | `windows_amd64.zip`, `windows_arm64.zip`, `windows_386.zip`; unzip and add `forke-mail.exe` to your PATH |
| FreeBSD | `freebsd_amd64.tar.gz`, `freebsd_arm64.tar.gz` |

## Check what you downloaded

Every release has a checksums file and a signed build attestation, so you can prove a file came from this repository's release workflow:

```sh
sha256sum -c forke-mail_1.1.0_checksums.txt --ignore-missing
gh attestation verify forke-mail_1.1.0_linux_amd64.tar.gz --repo forke-org/forke-mail
```

> [!NOTE]
> Upgrading from 1.0.x? Run the installer again. You stay signed in, and `forke-mail version` now shows the exact release you're on. New to the CLI? Start with `forke-mail login`, then `forke-mail init` in your project. Docs: https://www.forke.space/docs/forke-mail-cli

## What's Changed

### ✨ Features
- Builds for macOS (Apple Silicon, Intel and universal), Linux (`amd64`, `arm64`, `armv7`, `386`), Windows (`amd64`, `arm64`, `386`) and FreeBSD (`amd64`, `arm64`)
- `.deb`, `.rpm` and `.apk` packages for every Linux CPU, installing `forke-mail` to `/usr/bin`
- A `checksums.txt` file and a signed build attestation on every release
- `forke-mail version` reports the release it was built from instead of a fixed number

### 🐛 Fixes
- The installer checks the SHA-256 checksum and installs nothing if the file doesn't match
- The installer finds the latest release itself, so it keeps working as new versions ship

### 📚 Docs & Chores
- Releases are built and published by GoReleaser on GitHub Actions from `v*` tags
- Release notes come from the CLI changelog, with the commit list and a compare link added automatically

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
