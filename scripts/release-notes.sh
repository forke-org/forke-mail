#!/bin/sh
# Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
# Source-Available License (Non-Commercial / Fair Source).
# Open for inspection, learning, and non-commercial development.
# Commercial use, hosting, or resale without authorization is strictly prohibited.
set -eu

tag=$1
repo=${GITHUB_REPOSITORY:-forke-org/forke-mail}
version=${tag#v}
section=$(awk -v v="## $version" '$0 == v { found = 1; next } found && /^## [0-9]/ { exit } found' cmd/forke-mail/CHANGELOG.md)
[ -n "$section" ] || { echo "No changelog section for $version" >&2; exit 1; }
prev=$(git describe --tags --abbrev=0 --match 'v[0-9]*' --match 'cli-v[0-9]*' "$tag^" 2>/dev/null || true)

printf '%s\n' "$section"
if [ -n "$prev" ]; then
  printf '\n### 🧾 Commits\n\n'
  git log --reverse --format="- %s by @AUTHOR in [%h](https://github.com/$repo/commit/%H)%x09%ae" "$prev..$tag" -- cmd/forke-mail .goreleaser.yaml internal/mcp/install.sh scripts/release-notes.sh .github/workflows/cli-release.yml .github/workflows/cli-release-notes.yml |
    while IFS="$(printf '\t')" read -r line email; do
      handle=$(printf '%s' "$email" | sed -n 's/^[0-9]*+\([^@]*\)@users\.noreply\.github\.com$/\1/p')
      [ -n "$handle" ] || handle=$(printf '%s' "$email" | cut -d@ -f1)
      printf '%s\n' "$line" | sed "s/@AUTHOR/@$handle/"
    done
  printf '\n**Full Changelog**: https://github.com/%s/compare/%s...%s\n' "$repo" "$prev" "$tag"
fi
