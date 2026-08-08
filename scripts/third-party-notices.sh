#!/usr/bin/env bash
# Write a THIRD-PARTY-NOTICES file covering every module linked into the binary.
#
#   ./scripts/third-party-notices.sh dist/THIRD-PARTY-NOTICES

set -euo pipefail

GO_LICENSES_VERSION="v1.6.0"

OUT="${1:-THIRD-PARTY-NOTICES}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE="$(cd "$ROOT" && go list -m)"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

printf '{{range .}}{{.Name}}\t{{.Version}}\t{{.LicenseName}}\t{{.LicensePath}}\n{{end}}' \
	>"${WORK}/report.tpl"

GOBIN="$WORK" go install "github.com/google/go-licenses@${GO_LICENSES_VERSION}"

(cd "$ROOT" && GOOS=windows "${WORK}/go-licenses" report ./... \
	--template "${WORK}/report.tpl" \
	--ignore "$MODULE") >"${WORK}/modules.tsv"

[ -s "${WORK}/modules.tsv" ] || {
	echo "no third-party modules reported" >&2
	exit 1
}

rule="$(printf '=%.0s' $(seq 78))"

{
	cat <<-'EOF'
		athena-proxy — third-party notices

		This binary links the Go modules listed below. Each section names the module
		and the version linked, and reproduces that module's licence in full.
	EOF

	while IFS=$'\t' read -r name version license path; do
		[ -n "$name" ] || continue
		printf '\n%s\n%s %s — %s\n%s\n\n' "$rule" "$name" "$version" "$license" "$rule"
		cat "$path"
	done <"${WORK}/modules.tsv"
} >"$OUT"

echo "wrote ${OUT} ($(wc -l <"${WORK}/modules.tsv") modules)"
