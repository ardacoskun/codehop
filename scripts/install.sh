#!/bin/sh
# Install the latest codehop release on macOS or Linux.
#   curl -fsSL https://raw.githubusercontent.com/ardacoskun/codehop/main/scripts/install.sh | sh
# Environment: CODEHOP_VERSION (e.g. v0.1.0, default latest),
#              CODEHOP_INSTALL_DIR (default ~/.local/bin).
set -eu

repo="ardacoskun/codehop"
bin_dir="${CODEHOP_INSTALL_DIR:-$HOME/.local/bin}"

fail() { echo "codehop install: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required"; }
# HTTPS only, TLS 1.2 or newer, including after redirects.
fetch() { curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL "$@"; }
need curl
need tar

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) fail "unsupported OS $(uname -s); on Windows use scripts/install.ps1" ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported architecture $(uname -m)" ;;
esac

version="${CODEHOP_VERSION:-}"
if [ -z "$version" ]; then
  latest_url="$(fetch -I -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest")"
  version="${latest_url##*/}"
fi
printf '%s' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$' ||
  fail "invalid version '$version' (expected e.g. v0.1.0)"

archive="codehop_${version#v}_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$version"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading codehop $version ($os/$arch)..."
fetch -o "$tmp/$archive" "$base/$archive"
fetch -o "$tmp/checksums.txt" "$base/checksums.txt"

expected="$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
[ -n "$expected" ] || fail "no checksum for $archive"
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$archive" | cut -d' ' -f1)"
else
  actual="$(shasum -a 256 "$tmp/$archive" | cut -d' ' -f1)"
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" codehop
mkdir -p "$bin_dir"
install -m 755 "$tmp/codehop" "$bin_dir/codehop"
echo "Installed codehop $version to $bin_dir/codehop"
echo "Verify provenance (optional): gh attestation verify $bin_dir/codehop --repo $repo"

case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo "Note: $bin_dir is not on your PATH; add it in your shell profile." ;;
esac
echo 'Next: add the shell integration, e.g. eval "$(codehop init zsh)" in ~/.zshrc'
