#!/usr/bin/env bash
# Install the mcpgate release binary for this OS/arch, verifying its SHA-256
# against the release's checksums.txt.
#
#   bash install.sh                      # latest release → ~/.local/bin
#   MCPGATE_VERSION=0.1.0 bash install.sh
#   MCPGATE_INSTALL_DIR=/usr/local/bin bash install.sh
set -euo pipefail

REPO="bouncei/mcpgate"
INSTALL_DIR="${MCPGATE_INSTALL_DIR:-$HOME/.local/bin}"

fallback() {
  cat >&2 <<EOF
mcpgate: $1
Install another way:
  go install github.com/${REPO}/cmd/mcpgate@latest      # Go 1.25+
  docker build -t mcpgate https://github.com/${REPO}.git
EOF
  exit 1
}

command -v curl >/dev/null || fallback "curl is required"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  MINGW*|MSYS*|CYGWIN*) os=windows ;;
  *) fallback "unsupported OS $(uname -s)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fallback "unsupported architecture $(uname -m)" ;;
esac

version="${MCPGATE_VERSION:-}"
if [ -z "$version" ]; then
  # /releases/latest redirects to /releases/tag/vX.Y.Z; avoids the rate-limited API.
  latest=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest") \
    || fallback "could not reach github.com"
  version="${latest##*/v}"
  [ "$version" != "$latest" ] || fallback "no published release found"
fi
version="${version#v}"

asset="mcpgate_${version}_${os}_${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/v${version}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${asset}..."
curl -fsSL -o "$tmp/$asset" "$base/$asset" || fallback "download failed: $base/$asset"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fallback "checksums.txt download failed"

expected=$(awk -v f="$asset" '$2 == f {print $1}' "$tmp/checksums.txt")
[ -n "$expected" ] || fallback "$asset not listed in checksums.txt"
if command -v sha256sum >/dev/null; then
  actual=$(sha256sum "$tmp/$asset" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
fi
[ "$expected" = "$actual" ] || fallback "checksum mismatch for $asset (expected $expected, got $actual)"

tar -xzf "$tmp/$asset" -C "$tmp"
bin=mcpgate
[ "$os" = windows ] && bin=mcpgate.exe
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/$bin" "$INSTALL_DIR/$bin"

echo "Installed mcpgate $("$INSTALL_DIR/$bin" version 2>&1) to $INSTALL_DIR/$bin"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Note: $INSTALL_DIR is not on PATH. Add it, e.g.: export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
