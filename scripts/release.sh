#!/bin/sh
#
# Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
# of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
# Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
# (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
# and DikuMUD licenses; see LICENSE. Non-commercial use only.
#
# Cut a release: bump the semver tag, regenerate the example yaml worlds
# from their binary source (catching any drift between the two before it
# ships rather than after), run the local checks -- including the play
# regression suite, which is release-only -- then tag and push.
#
# .github/workflows/release.yml is what actually runs the full regression
# suite and creates the GitHub release — this script's job is everything
# that has to happen *before* a tag exists for that workflow to trigger
# on: computing the next version, and making sure what gets tagged is
# what the tag claims it is.
#
# Usage: scripts/release.sh patch|minor|major
#        scripts/release.sh v1.2.3        # an explicit version, skipping the bump math

set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

usage() {
	echo "Usage: $0 patch|minor|major|vX.Y.Z" >&2
	exit 2
}

[ $# -eq 1 ] || usage
ARG=$1

# 1. Preconditions. A release built from anything else is a release nobody
# can reproduce from git history alone.
branch=$(git rev-parse --abbrev-ref HEAD)
if [ "$branch" != "main" ]; then
	echo "release.sh: on branch '$branch', not main" >&2
	exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
	echo "release.sh: working tree is not clean" >&2
	git status --short >&2
	exit 1
fi

git fetch origin main >/dev/null 2>&1 || true
local_sha=$(git rev-parse HEAD)
remote_sha=$(git rev-parse origin/main 2>/dev/null || echo "")
if [ -n "$remote_sha" ] && [ "$local_sha" != "$remote_sha" ]; then
	echo "release.sh: local main ($local_sha) does not match origin/main ($remote_sha)" >&2
	echo "pull or push first" >&2
	exit 1
fi

# 2. The current version, from the latest vX.Y.Z tag reachable from HEAD.
# No matching tag means this is the first release.
current=$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null || echo "v0.0.0")
current=${current#v}
major=$(echo "$current" | cut -d. -f1)
minor=$(echo "$current" | cut -d. -f2)
patch=$(echo "$current" | cut -d. -f3)

case "$ARG" in
v[0-9]*.[0-9]*.[0-9]*)
	next=${ARG#v}
	;;
patch)
	next="$major.$minor.$((patch + 1))"
	;;
minor)
	next="$major.$((minor + 1)).0"
	;;
major)
	next="$((major + 1)).0.0"
	;;
*)
	usage
	;;
esac
tag="v$next"

if git rev-parse "$tag" >/dev/null 2>&1; then
	echo "release.sh: $tag already exists" >&2
	exit 1
fi

echo "==> Current version: v$current"
echo "==> Releasing:       $tag"

# 3. Regenerate both example yaml worlds from their binary source, and
# check whether anything actually changed. This is the same check
# release.yml makes authoritatively; doing it here first means a real
# drift gets a commit and a description instead of a failed release run.
echo "==> Checking example yaml worlds against a fresh dlctl lib import"
regenerated=0
for pair in stock mini; do
	work=$(mktemp -d)
	go run ./cmd/dlctl lib import \
		--from-dir="examples/$pair/binary" --to-dir="$work" >/dev/null
	if ! diff -rq "examples/$pair/yaml" "$work" >/dev/null 2>&1; then
		echo "    examples/$pair/yaml is stale against examples/$pair/binary -- regenerating"
		rm -rf "examples/$pair/yaml"
		cp -r "$work" "examples/$pair/yaml"
		regenerated=1
	fi
	rm -rf "$work"
done

if [ "$regenerated" -eq 1 ]; then
	git add examples/stock/yaml examples/mini/yaml
	git commit -m "Regenerate example yaml worlds for $tag

dlctl lib import's output no longer matched what was checked in --
either the yaml format's own version (internal/persist/dataversion)
moved, or the binary source changed without a matching regeneration.
Caught here rather than at release.yml's own drift check, which now has
nothing to fail on.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
	echo "==> Committed regenerated example worlds"
else
	echo "    both example yaml worlds already match; nothing to regenerate"
fi

# 4. The fast local checks. release.yml's full-suite job is the
# authoritative gate -- this is a chance to fail fast, locally, before
# pushing a tag that triggers it, not a replacement for it.
echo "==> make check"
make check

# The play regression suite. Slower than everything above put together
# -- it builds a server binary and then starts one process per test --
# and worth it here for the same reason release.yml runs it: it is the
# only thing that exercises the boot sequence, the zone resets, the
# specials and the shutdown saves at all, and a release is exactly when
# "the world still loads and you can still play it" should be checked
# rather than assumed.
echo "==> make play"
make play

# world-parity.sh builds the C reference server natively -- no 32-bit
# toolchain needed for this one, unlike the ilp32 checks `make check`
# above already ran (or skipped, same as any 64-bit machine; release.yml
# is where those run for real). Only gcc/cc itself is required.
if command -v gcc >/dev/null 2>&1 || command -v cc >/dev/null 2>&1; then
	echo "==> make parity"
	make parity
else
	echo "==> skipping make parity locally (no C compiler here); release.yml runs it for real"
fi

# 5. Tag and push. The tag push is what triggers release.yml.
git tag -a "$tag" -m "Release $tag"
git push origin main
git push origin "$tag"

echo
echo "==> Pushed $tag. Watch the release workflow:"
echo "    gh run watch -R $(git remote get-url origin | sed -E 's#.*/([^/]+/[^/.]+)(\.git)?$#\1#')"
