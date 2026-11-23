#!/bin/sh
# Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
# Source-Available License (Non-Commercial / Fair Source).
# Open for inspection, learning, and non-commercial development.
# Commercial use, hosting, or resale without authorization is strictly prohibited.
set -eu

repo="https://github.com/forke-org/forke-mail"
os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  armv7*|armv6*) arch=armv7 ;;
  i386|i686) arch=386 ;;
  *) echo "Unsupported CPU: $arch" >&2; exit 1 ;;
esac
case "$os" in
  darwin) label=macOS; ext=zip; arch=universal ;;
  linux|freebsd) label=$os; ext=tar.gz ;;
  *) echo "On Windows, download the zip for your CPU from $repo/releases/latest" >&2; exit 1 ;;
esac

tag=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$repo/releases/latest" | sed 's#.*/tag/##')
version=${tag#v}
case "$version" in
  [0-9]*) ;;
  *) echo "Could not find the latest release." >&2; exit 1 ;;
esac
name="forke-mail_${version}_${label}_${arch}.${ext}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading forke-mail $version for $label/$arch..."
curl -fsSL "$repo/releases/download/$tag/$name" -o "$tmp/$name"
curl -fsSL "$repo/releases/download/$tag/forke-mail_${version}_checksums.txt" -o "$tmp/checksums.txt"
want=$(grep " $name\$" "$tmp/checksums.txt" | cut -d' ' -f1)
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$tmp/$name" | cut -d' ' -f1); else got=$(shasum -a 256 "$tmp/$name" | cut -d' ' -f1); fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then
  echo "Checksum mismatch for $name. Nothing was installed." >&2
  exit 1
fi

if [ "$ext" = zip ]; then (cd "$tmp" && unzip -q "$name"); else tar -xzf "$tmp/$name" -C "$tmp"; fi
dir="${FORKE_MAIL_INSTALL_DIR:-/usr/local/bin}"
if [ ! -w "$dir" ]; then
  dir="$HOME/.local/bin"
  mkdir -p "$dir"
fi
mv "$tmp/forke-mail" "$dir/forke-mail"
chmod +x "$dir/forke-mail"
echo "Installed $dir/forke-mail ($version)"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "Add $dir to your PATH." ;;
esac
echo "Next: forke-mail login"
