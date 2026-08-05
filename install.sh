#!/usr/bin/env sh
# Install or upgrade athena-proxy from the latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/astraqcd/athena-proxy/main/install.sh | sh
#
# Optional env:
#   ATHENA_PROXY_VERSION      Pin a release tag (e.g. v1.0.0). Default: latest.
#   ATHENA_PROXY_INSTALL_DIR  Install directory. Default: ~/.local/bin

set -eu

REPO="astraqcd/athena-proxy"
BINARY="athena-proxy"
INSTALL_DIR="${ATHENA_PROXY_INSTALL_DIR:-${HOME}/.local/bin}"
VERSION="${ATHENA_PROXY_VERSION:-}"

info() { printf '%s\n' "$*"; }
die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "need '$1' on PATH"
}

need_cmd uname
need_cmd mktemp
need_cmd tar

if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
	die "need 'curl' or 'wget' on PATH"
fi

download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	else
		wget -qO "$2" "$1"
	fi
}

fetch_body() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1"
	else
		wget -qO- "$1"
	fi
}

resolve_latest() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSLI -o /dev/null -w '%{url_effective}' \
			"https://github.com/${REPO}/releases/latest" | sed 's#.*/tag/##'
		return
	fi
	wget -S --spider "https://github.com/${REPO}/releases/latest" 2>&1 |
		tr -d '\r' |
		awk 'tolower($1)=="location:" {print $2}' |
		tail -n1 |
		sed 's#.*/tag/##'
}

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
linux | darwin) ;;
mingw* | msys* | cygwin*)
	die "use install.ps1 on Windows: irm https://raw.githubusercontent.com/${REPO}/main/install.ps1 | iex"
	;;
*)
	die "unsupported OS: $OS"
	;;
esac

case "$ARCH" in
x86_64 | amd64) ARCH="amd64" ;;
aarch64 | arm64) ARCH="arm64" ;;
*) die "unsupported architecture: $ARCH" ;;
esac

if [ -z "$VERSION" ]; then
	info "Resolving latest release…"
	VERSION="$(resolve_latest | tr -d '[:space:]')" || true
	if [ -z "$VERSION" ]; then
		json="$(fetch_body "https://api.github.com/repos/${REPO}/releases/latest")" ||
			die "could not resolve latest release; set ATHENA_PROXY_VERSION"
		VERSION="$(printf '%s' "$json" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
	fi
	[ -n "$VERSION" ] || die "could not resolve latest release; set ATHENA_PROXY_VERSION"
fi

case "$VERSION" in
v*) ;;
*) VERSION="v${VERSION}" ;;
esac

ASSET="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"
DEST="${INSTALL_DIR}/${BINARY}"

info "Installing ${BINARY} ${VERSION} (${OS}/${ARCH}) → ${DEST}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT HUP TERM

download "$URL" "${TMPDIR}/${ASSET}" || die "download failed: ${URL}"
tar -xzf "${TMPDIR}/${ASSET}" -C "$TMPDIR" || die "could not unpack ${ASSET}"
[ -f "${TMPDIR}/${BINARY}" ] || die "${ASSET} holds no ${BINARY} binary"

chmod 755 "${TMPDIR}/${BINARY}"
mkdir -p "$INSTALL_DIR"
mv -f "${TMPDIR}/${BINARY}" "$DEST"

info "Installed $("$DEST" --version 2>/dev/null || printf '%s' "$DEST")"

case ":${PATH}:" in
*":${INSTALL_DIR}:"*) ;;
*)
	info "Note: ${INSTALL_DIR} is not on PATH. Add it, e.g.:"
	info "  export PATH=\"${INSTALL_DIR}:\$PATH\""
	;;
esac
