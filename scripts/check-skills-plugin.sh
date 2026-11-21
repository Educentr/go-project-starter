#!/usr/bin/env bash
#
# check-skills-plugin.sh — verify that the Claude skills plugin of this
# generator (Educentr/claude, plugins/go-project-starter) is updated for the
# version being released.
#
# The plugin is what AI agents read instead of this repository. Every skill in
# it names the generator version its instructions cover, and the commands it
# hands to users pin that same version. A release that leaves the plugin behind
# does not merely go stale — it keeps teaching the previous version.
#
# Usage:
#   ./scripts/check-skills-plugin.sh [vX.Y.Z]
#
# The version checked against is, in order: the argument, $VERSION, the latest
# tag of this repository.
#
# Overrides:
#   PLUGIN_SRC     read the plugin from this directory instead of fetching it
#                  (offline runs; reviewing a plugin branch checked out locally)
#   PLUGIN_REPO    default Educentr/claude
#   PLUGIN_REF     default main
#   PLUGIN_SUBDIR  default plugins/go-project-starter
#
# Exit codes: 0 plugin covers the version, 1 plugin lags behind, 2 internal error.

set -euo pipefail

PLUGIN_REPO="${PLUGIN_REPO:-Educentr/claude}"
PLUGIN_REF="${PLUGIN_REF:-main}"
PLUGIN_SUBDIR="${PLUGIN_SUBDIR:-plugins/go-project-starter}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MAIN_SKILL="skills/go-project-starter/SKILL.md"

version="${1:-${VERSION:-}}"
if [ -z "$version" ]; then
    version=$(git -C "$ROOT_DIR" describe --tags --abbrev=0 2>/dev/null || true)
    if [ -z "$version" ]; then
        echo "ERROR: no version given and no tag found in $ROOT_DIR" >&2
        echo "       pass it explicitly: $0 vX.Y.Z" >&2
        exit 2
    fi
fi

# Normalize to the "vX.Y.Z" form the plugin uses. Anything else (a `git
# describe` string of an untagged commit, a branch name) is rejected rather
# than compared: the plugin can only ever name a released version.
version="v${version#v}"
if ! [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "ERROR: '$version' is not a release version (expected vX.Y.Z)" >&2
    exit 2
fi

tmp_dir=""
cleanup() {
    if [ -n "$tmp_dir" ]; then
        rm -rf "$tmp_dir"
    fi
}
trap cleanup EXIT

if [ -n "${PLUGIN_SRC:-}" ]; then
    if [ ! -d "$PLUGIN_SRC" ]; then
        echo "ERROR: PLUGIN_SRC=$PLUGIN_SRC is not a directory" >&2
        exit 2
    fi
    plugin_dir="$PLUGIN_SRC"
    source_desc="$PLUGIN_SRC"
else
    tmp_dir=$(mktemp -d)
    source_desc="$PLUGIN_REPO@$PLUGIN_REF:$PLUGIN_SUBDIR"
    # One request for the whole repository (a few hundred KB of markdown), so
    # the scan below sees every file of the plugin at that ref. No token is
    # sent: the plugin repository is public, and a wrong token in the
    # environment would turn the read into a 401. For a checkout that needs
    # credentials, point PLUGIN_SRC at it instead.
    # Explicit timeouts and one retry: a stalled connection must fail the check
    # in bounded time, not hang a release.
    if ! curl -fsSL --connect-timeout 10 --max-time 180 --retry 2 \
        -o "$tmp_dir/plugin.tar.gz" \
        "https://api.github.com/repos/$PLUGIN_REPO/tarball/$PLUGIN_REF"; then
        echo "ERROR: cannot download $PLUGIN_REPO@$PLUGIN_REF" >&2
        echo "       check the plugin out and re-run with PLUGIN_SRC=<path>/$PLUGIN_SUBDIR" >&2
        exit 2
    fi
    mkdir -p "$tmp_dir/src"
    tar -xzf "$tmp_dir/plugin.tar.gz" -C "$tmp_dir/src" --strip-components=1
    plugin_dir="$tmp_dir/src/$PLUGIN_SUBDIR"
    if [ ! -d "$plugin_dir" ]; then
        echo "ERROR: $PLUGIN_SUBDIR not found in $PLUGIN_REPO@$PLUGIN_REF" >&2
        exit 2
    fi
fi

if [ ! -f "$plugin_dir/$MAIN_SKILL" ]; then
    echo "ERROR: $source_desc has no $MAIN_SKILL" >&2
    exit 2
fi

# The version statement of the main skill: "These instructions cover
# **go-project-starter v0.31.0**". It is the version the plugin as a whole
# claims to document, so everything else is compared against it.
declared=$(grep -oE 'go-project-starter v[0-9]+\.[0-9]+\.[0-9]+' "$plugin_dir/$MAIN_SKILL" |
    head -n1 | awk '{print $2}' || true)
if [ -z "$declared" ]; then
    echo "ERROR: $MAIN_SKILL declares no generator version" >&2
    echo "       expected a line like: These instructions cover **go-project-starter vX.Y.Z**" >&2
    exit 2
fi

# Files that are instructions. The eval workspace is excluded: it holds
# recorded runs, whose point is that they are historical.
plugin_files=()
while IFS= read -r f; do
    plugin_files+=("$f")
done < <(
    find "$plugin_dir" -type f \( -name '*.md' -o -name '*.json' \) \
        -not -path '*/go-project-starter-workspace/*' | sort
)

# A reference page named for older releases ("references/releases-v0.30-v0.31.md")
# is history by its own title: the commands in it belong to the releases it
# describes, so it is listed rather than checked. The page named for the current
# line is checked like any other.
declared_minor=$(echo "$declared" | awk -F. '{print $1"."$2}')
scan_files=()
historical_files=()
for f in "${plugin_files[@]}"; do
    name_ver=$(grep -oE 'v[0-9]+\.[0-9]+' <<<"$(basename "$f")" | sort -V | tail -n1 || true)
    if [ -n "$name_ver" ] && [ "$name_ver" != "$declared_minor" ] &&
        [ "$(printf '%s\n%s\n' "$name_ver" "$declared_minor" | sort -V | tail -n1)" = "$declared_minor" ]; then
        historical_files+=("${f#"$plugin_dir"/}")
    else
        scan_files+=("$f")
    fi
done

# Version-bearing text that must move with the release:
#   - "go install …/go-project-starter@vX.Y.Z", "go run …@vX.Y.Z"   — what users install
#   - "make update-starter STARTER_VERSION=vX.Y.Z", "GPSVERSION = vX.Y.Z" — the project pin
#   - "…/go-project-starter/blob/vX.Y.Z/docs/…"                     — docs of the covered version
# Deliberately not matched: a bare "since v0.24.0", a "releases/tag/vX.Y.Z"
# link, a version of another tool — those are history, and history stays.
version_refs='go-project-starter@v[0-9]+\.[0-9]+\.[0-9]+|STARTER_VERSION=v[0-9]+\.[0-9]+\.[0-9]+|GPSVERSION[ ?:=]+v[0-9]+\.[0-9]+\.[0-9]+|go-project-starter/blob/v[0-9]+\.[0-9]+\.[0-9]+'

echo "Release version:        $version"
echo "Plugin instructions:    $declared   ($source_desc)"
echo "Instruction files:      ${#scan_files[@]}"
if [ ${#historical_files[@]} -ne 0 ]; then
    echo "Historical pages:       ${historical_files[*]}   (not checked)"
fi

problems=0

if [ "$declared" != "$version" ]; then
    if [ "$(printf '%s\n%s\n' "$declared" "$version" | sort -V | tail -n1)" = "$declared" ]; then
        echo ""
        echo "NOTE: the plugin already documents $declared, ahead of $version — nothing to update."
    else
        echo ""
        echo "ERROR: the skills plugin documents $declared, but $version is being released."
        echo "       ($MAIN_SKILL)"
        problems=1
    fi
fi

# Every other skill carries its own version statement, and each one is read on
# its own — a skill left behind is wrong even when the main one is current.
stale_skills=$(grep -rlE "go-project-starter v[0-9]+\.[0-9]+\.[0-9]+" \
    --include='SKILL.md' "$plugin_dir/skills" 2>/dev/null |
    while read -r f; do
        stated=$(grep -oE 'go-project-starter v[0-9]+\.[0-9]+\.[0-9]+' "$f" | head -n1 | awk '{print $2}')
        if [ "$stated" != "$declared" ]; then
            echo "  ${f#"$plugin_dir"/}: states $stated"
        fi
    done || true)
if [ -n "$stale_skills" ]; then
    echo ""
    echo "ERROR: skills state a generator version other than $declared:"
    echo "$stale_skills"
    problems=1
fi

# An empty file list would make grep read stdin; it cannot happen while the main
# skill is present (checked above), but the guard keeps that coupling explicit.
stale_refs=""
if [ ${#scan_files[@]} -ne 0 ]; then
    stale_refs=$(grep -rhoE "$version_refs" "${scan_files[@]}" 2>/dev/null |
        sed -E 's/.*(v[0-9]+\.[0-9]+\.[0-9]+).*/\1/' | sort -u |
        grep -vFx "$declared" || true)
fi

if [ -n "$stale_refs" ]; then
    echo ""
    echo "ERROR: commands and tagged links still point at $(echo "$stale_refs" | tr '\n' ' ')instead of $declared:"
    for stale in $stale_refs; do
        grep -rnE "$version_refs" "${scan_files[@]}" |
            grep -F "$stale" |
            sed -E "s|^$plugin_dir/|  |"
    done
    problems=1
fi

if [ "$problems" -ne 0 ]; then
    echo ""
    echo "Update $PLUGIN_REPO ($PLUGIN_SUBDIR) before announcing the release:"
    echo "  - the version every skill states, and every command pin and tagged link in them"
    echo "  - the references/ pages touched by what this release changed"
    echo "  - version in .claude-plugin/plugin.json"
    echo "See docs/development/skills-plugin.md for what maps to what."
    exit 1
fi

echo "OK: the skills plugin covers $version."
