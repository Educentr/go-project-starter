#!/usr/bin/env bash
#
# check-go-mod-version.sh — verify that `go <X>` in go.mod matches policy:
#   1) declared >= max(go required by direct deps)
#   2) declared minor == max-dep minor (i.e. no unjustified bump above what deps need)
#
# Override #2 with a file .go-version-rationale at the repo root: any non-empty
# content is treated as a justification (typically a one-liner with the specific
# Go feature or dependency requirement that forced the bump).
#
# Exit codes: 0 ok, 1 policy violation, 2 internal error.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_MOD="${GO_MOD:-$ROOT_DIR/go.mod}"
RATIONALE_FILE="${RATIONALE_FILE:-$ROOT_DIR/.go-version-rationale}"

if [ ! -f "$GO_MOD" ]; then
    echo "ERROR: $GO_MOD not found" >&2
    exit 2
fi

declared=$(awk '/^go [0-9]+\.[0-9]+/ {print $2; exit}' "$GO_MOD")
if [ -z "$declared" ]; then
    echo "ERROR: no 'go <version>' line in $GO_MOD" >&2
    exit 2
fi

cd "$ROOT_DIR"

direct_deps=$(go list -mod=mod -m -f '{{if and (not .Indirect) (not .Main)}}{{.Path}}@{{.Version}}{{end}}' all 2>/dev/null || true)
if [ -z "$direct_deps" ]; then
    echo "ERROR: 'go list -m all' failed (check go.mod / module cache)" >&2
    exit 2
fi

GOMODCACHE="$(go env GOMODCACHE)"
max_go=""
max_dep=""
for dep_at_ver in $direct_deps; do
    dep="${dep_at_ver%@*}"
    ver="${dep_at_ver#*@}"
    modfile="$GOMODCACHE/cache/download/$dep/@v/$ver.mod"
    if [ ! -f "$modfile" ]; then
        go mod download "$dep_at_ver" >/dev/null 2>&1 || continue
    fi
    [ -f "$modfile" ] || continue
    gov=$(awk '/^go [0-9]+\.[0-9]+/ {print $2; exit}' "$modfile")
    [ -n "$gov" ] || continue
    if [ -z "$max_go" ] || [ "$(printf '%s\n%s\n' "$max_go" "$gov" | sort -V | tail -1)" = "$gov" ] && [ "$max_go" != "$gov" ]; then
        max_go="$gov"
        max_dep="$dep_at_ver"
    fi
done

if [ -z "$max_go" ]; then
    echo "WARN: could not determine max Go version required by direct deps — skipping policy check"
    exit 0
fi

echo "go.mod declares:        go $declared"
echo "Max required by deps:   go $max_go   (from $max_dep)"

if [ "$(printf '%s\n%s\n' "$declared" "$max_go" | sort -V | tail -1)" != "$declared" ]; then
    echo ""
    echo "ERROR: go.mod declares go $declared but direct deps require >= go $max_go"
    echo "       Raise the 'go' line in go.mod to at least $max_go."
    exit 1
fi

declared_minor="$(echo "$declared" | awk -F. '{print $1"."$2}')"
max_minor="$(echo "$max_go" | awk -F. '{print $1"."$2}')"

if [ "$declared_minor" != "$max_minor" ]; then
    if [ -s "$RATIONALE_FILE" ]; then
        echo ""
        echo "NOTE: go.mod minor ($declared_minor) exceeds deps minor ($max_minor),"
        echo "      but rationale file is present at $RATIONALE_FILE:"
        sed 's/^/      | /' "$RATIONALE_FILE"
        echo "OK: bump justified."
        exit 0
    fi
    echo ""
    echo "ERROR: go.mod minor ($declared_minor) exceeds deps minor ($max_minor)."
    echo "       Cosmetic bumps of the 'go' line widen the surface of the auto-toolchain"
    echo "       problem (see docs/GO-VERSION-POLICY.md). If this bump is intentional,"
    echo "       commit a file '.go-version-rationale' at the repo root describing why"
    echo "       (specific Go feature used, dependency requirement, etc.)."
    exit 1
fi

echo "OK: go.mod version aligns with deps policy."
