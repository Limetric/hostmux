#!/bin/sh
# hostmux installer.
#
#   curl -fsSL https://raw.githubusercontent.com/Limetric/hostmux/main/install.sh | sh
#
# Downloads the latest release binary for your OS/arch, verifies its SHA-256
# checksum, and installs it. Override the destination with HOSTMUX_INSTALL_DIR
# (default: /usr/local/bin). Re-run to upgrade.
set -eu

REPO="Limetric/hostmux"
BINARY="hostmux"
INSTALL_DIR="${HOSTMUX_INSTALL_DIR:-/usr/local/bin}"

fail() {
	echo "install: $1" >&2
	exit 1
}

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
linux | darwin) ;;
*) fail "unsupported OS '$os'. Windows users: download from https://github.com/${REPO}/releases/latest" ;;
esac

arch="$(uname -m)"
case "$arch" in
x86_64 | amd64) arch="amd64" ;;
aarch64 | arm64) arch="arm64" ;;
*) fail "unsupported architecture '$arch'" ;;
esac

asset="${BINARY}-${os}-${arch}"
base="https://github.com/${REPO}/releases/latest/download"

command -v curl >/dev/null 2>&1 || fail "curl is required"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${asset}..."
curl -fSL "${base}/${asset}" -o "${tmp}/${BINARY}" || fail "download failed"

# Verify the checksum when the .sha256 sidecar is present. Skip silently if
# absent so releases made before checksums were published still install.
if curl -fsSL "${base}/${asset}.sha256" -o "${tmp}/${BINARY}.sha256" 2>/dev/null; then
	echo "Verifying checksum..."
	expected="$(awk '{print $1}' "${tmp}/${BINARY}.sha256")"
	if command -v sha256sum >/dev/null 2>&1; then
		actual="$(sha256sum "${tmp}/${BINARY}" | awk '{print $1}')"
	elif command -v shasum >/dev/null 2>&1; then
		actual="$(shasum -a 256 "${tmp}/${BINARY}" | awk '{print $1}')"
	else
		fail "no sha256sum or shasum available to verify the download"
	fi
	[ "$expected" = "$actual" ] || fail "checksum mismatch (expected ${expected}, got ${actual})"
fi

chmod +x "${tmp}/${BINARY}"

if [ -w "$INSTALL_DIR" ]; then
	mv "${tmp}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
	echo "Installing to ${INSTALL_DIR} (requires sudo)..."
	sudo mv "${tmp}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

echo "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
"${INSTALL_DIR}/${BINARY}" version 2>/dev/null || true
