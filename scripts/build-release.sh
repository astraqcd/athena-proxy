#!/usr/bin/env bash
# Build every release target from a single machine and write archives plus a
# SHA256SUMS manifest into dist/.
#
#   ./scripts/build-release.sh v1.0.0

set -euo pipefail

VERSION="${1:-dev}"
BINARY="athena-proxy"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="${ROOT}/dist"

TARGETS=(
	"linux/amd64"
	"linux/arm64"
	"darwin/amd64"
	"darwin/arm64"
	"windows/amd64"
	"windows/arm64"
)

rm -rf "$DIST"
mkdir -p "$DIST"

for target in "${TARGETS[@]}"; do
	goos="${target%%/*}"
	goarch="${target##*/}"

	ext=""
	[ "$goos" = "windows" ] && ext=".exe"

	stage="$(mktemp -d)"
	archive="${BINARY}_${VERSION}_${goos}_${goarch}"

	echo "building ${archive}"
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
		-trimpath \
		-ldflags "-s -w -X main.version=${VERSION}" \
		-o "${stage}/${BINARY}${ext}" \
		"$ROOT"

	cp "${ROOT}/LICENSE" "${ROOT}/README.md" "$stage/"

	if [ "$goos" = "windows" ]; then
		(cd "$stage" && zip -q -X -r "${DIST}/${archive}.zip" .)
	else
		tar -czf "${DIST}/${archive}.tar.gz" -C "$stage" .
	fi

	rm -rf "$stage"
done

(cd "$DIST" && sha256sum -- * >SHA256SUMS)

echo
echo "dist/:"
ls -1 "$DIST"
