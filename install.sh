#!/bin/sh
# Skein installer for Linux and macOS.
#
# One-line install:
#   curl -fsSL https://raw.githubusercontent.com/kaladinstorm84/skein/main/install.sh | sh
#
# Options (environment variables):
#   SKEIN_VERSION      Release tag to install, e.g. v0.7.0 (default: latest)
#   SKEIN_INSTALL_DIR  Target directory (default: /usr/local/bin if writable, else ~/.local/bin)
#
# Downloads the release binary for this OS/architecture, verifies it against
# the release SHA256SUMS.txt, and installs it as "skein".

set -eu

REPO="kaladinstorm84/skein"

fail() {
    echo "install.sh: error: $*" >&2
    exit 1
}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
    linux | darwin) ;;
    *) fail "unsupported OS '$os' (on Windows use install.ps1)" ;;
esac

arch=$(uname -m)
case "$arch" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) fail "unsupported architecture '$arch' (releases cover amd64 and arm64)" ;;
esac

asset="skein-${os}-${arch}"

version="${SKEIN_VERSION:-latest}"
if [ "$version" = "latest" ]; then
    base="https://github.com/${REPO}/releases/latest/download"
else
    base="https://github.com/${REPO}/releases/download/${version}"
fi

if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -q -O "$2" "$1"; }
else
    fail "curl or wget is required"
fi

install_dir="${SKEIN_INSTALL_DIR:-}"
if [ -z "$install_dir" ]; then
    if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
        install_dir=/usr/local/bin
    else
        install_dir="${HOME}/.local/bin"
    fi
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${asset} (${version}) from github.com/${REPO} ..."
fetch "${base}/${asset}" "${tmp}/${asset}" || fail "download failed: ${base}/${asset}"
fetch "${base}/SHA256SUMS.txt" "${tmp}/SHA256SUMS.txt" || fail "download failed: ${base}/SHA256SUMS.txt"

# tr strips CR: the published SHA256SUMS.txt has CRLF line endings.
expected=$(tr -d '\r' <"${tmp}/SHA256SUMS.txt" | awk -v f="$asset" '$2 == f { print $1 }')
[ -n "$expected" ] || fail "no entry for ${asset} in SHA256SUMS.txt"

if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "${tmp}/${asset}" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "${tmp}/${asset}" | awk '{ print $1 }')
else
    fail "sha256sum or shasum is required to verify the download"
fi
[ "$actual" = "$expected" ] || fail "checksum mismatch for ${asset} (expected ${expected}, got ${actual})"
echo "Checksum verified: ${actual}"

mkdir -p "$install_dir"
install -m 0755 "${tmp}/${asset}" "${install_dir}/skein"
echo "Installed: ${install_dir}/skein"

case ":${PATH}:" in
    *":${install_dir}:"*) ;;
    *)
        echo "NOTE: ${install_dir} is not on your PATH. Add it, for example:"
        echo "  export PATH=\"${install_dir}:\$PATH\""
        ;;
esac
