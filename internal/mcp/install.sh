#!/bin/sh
# Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
# Source-Available License (Non-Commercial / Fair Source).
# Open for inspection, learning, and non-commercial development.
# Commercial use, hosting, or resale without authorization is strictly prohibited.
set -eu

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "Unsupported CPU: $arch" >&2; exit 1 ;;
esac
case "$os" in
  darwin|linux) ;;
  *) echo "On Windows, download forke-mail-windows-amd64.exe from https://github.com/forke-org/forke-mail/releases/latest" >&2; exit 1 ;;
esac

url="https://github.com/forke-org/forke-mail/releases/latest/download/forke-mail-$os-$arch"
dir="${FORKE_MAIL_INSTALL_DIR:-/usr/local/bin}"
if [ ! -w "$dir" ]; then
  dir="$HOME/.local/bin"
  mkdir -p "$dir"
fi
tmp=$(mktemp)
echo "Downloading forke-mail for $os/$arch..."
curl -fsSL "$url" -o "$tmp"
chmod +x "$tmp"
mv "$tmp" "$dir/forke-mail"
echo "Installed $dir/forke-mail"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "Add $dir to your PATH, then run: forke-mail login" ;;
esac
echo "Next: forke-mail login"
