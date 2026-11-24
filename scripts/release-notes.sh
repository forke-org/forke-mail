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
  printf '\n**Full Changelog**: https://github.com/%s/compare/%s...%s\n' "$repo" "$prev" "$tag"
fi
