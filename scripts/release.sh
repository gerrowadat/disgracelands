#!/bin/sh
#
# Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
# of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
# Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
# (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
# and DikuMUD licenses; see LICENSE. Non-commercial use only.
#
# Cut a release: work out the next semver version, regenerate the example
# yaml worlds from their binary source (catching any drift between the two
# before it ships rather than after), run the local checks -- including the
# play regression suite and the cross-compile of the published platforms,
# both release-only -- then hand the version to
# .github/workflows/release.yml and wait for it.
#
# Nothing here tags anything. release.yml runs the full regression suite
# and only then, in its publish and image jobs, creates the tag, the
# GitHub release and the container image. A failed suite therefore leaves
# no trace at all: no tag, no release, no notes, no package, and the same
# version number free for the next attempt. This script used to tag and
# push first and let the tag push trigger the workflow, which meant every
# failed release left a real vX.Y.Z tag on a commit that had just been
# proved not to release -- fetchable by anyone, and awkward to retract
# once it had been.
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

# 1. Preconditions. gh does the dispatching and the waiting, so find out
# it is missing now rather than after `make check` and `make play` have
# spent twenty minutes earning the right to use it.
if ! command -v gh >/dev/null 2>&1; then
	echo "release.sh: gh (the GitHub CLI) is required to dispatch the release workflow" >&2
	exit 1
fi
if ! gh auth status >/dev/null 2>&1; then
	echo "release.sh: gh is not authenticated; run 'gh auth login'" >&2
	exit 1
fi

# main, clean, and level with origin. A release built from anything else
# is a release nobody can reproduce from git history alone.
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

if git rev-parse -q --verify "refs/tags/$tag" >/dev/null 2>&1; then
	echo "release.sh: $tag already exists locally" >&2
	exit 1
fi

# The tag is created on the runner now, not here, so origin's copy is the
# one that matters -- a tag this clone has never fetched would otherwise
# get all the way to the workflow before failing.
if [ -n "$(git ls-remote --tags origin "refs/tags/$tag" 2>/dev/null)" ]; then
	echo "release.sh: $tag already exists on origin" >&2
	exit 1
fi

echo "==> Current version: v$current"
echo "==> Releasing:       $tag"

# 3. Regenerate both example yaml worlds from their binary source, and
# check whether anything actually changed. This is the same check
# release.yml makes authoritatively; doing it here first means a real
# drift gets a commit and a description instead of a failed release run.
echo "==> Checking example yaml worlds against a fresh dlctl import"

# Before regenerating anything: refuse on a leftover etc/time. It is the mud
# clock's epoch, written into whatever --lib-dir a server ran on -- and
# examples/stock/binary is the *default* --lib-dir, so `make run`, a compose
# stack or a bare `go run ./cmd/dlmud` all leave one behind. import reads
# it, so the regeneration below would quietly rewrite state/clock.yaml to
# whenever you last stopped a server and, worse, commit that. It is not
# gitignored precisely so it stays visible; this is the second line of that
# defence, because "visible in git status" and "noticed" are different things.
for pair in stock mini; do
	if [ -e "examples/$pair/binary/etc/time" ]; then
		echo "release.sh: examples/$pair/binary/etc/time exists." >&2
		echo "  It is runtime state left by a server that ran on this directory," >&2
		echo "  dlctl import reads it, and regenerating with it in place would" >&2
		echo "  commit a state/clock.yaml with your clock in it rather than the" >&2
		echo "  epoch this example ships. Delete it and re-run:" >&2
		echo "    rm examples/$pair/binary/etc/time" >&2
		exit 1
	fi
done

regenerated=0
for pair in stock mini; do
	work=$(mktemp -d)
	go run ./cmd/dlctl import \
		--from-dir="examples/$pair/binary" --to-dir="$work" >/dev/null
	# -x .dlversion: the stamp records which release of dlctl wrote the
	# directory (docs/design/data-format-versioning.md), so it is expected
	# to differ between a checked-in copy and a fresh import and says
	# nothing about whether the world data drifted. An unreleased `go run`
	# dlctl, which is what this is, writes none at all.
	if ! diff -rq -x .dlversion "examples/$pair/yaml" "$work" >/dev/null 2>&1; then
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

dlctl import's output no longer matched what was checked in -- the
binary source changed without a matching regeneration, or an importer's
own output did. (.dlversion is excluded from the comparison: it records
which release of dlctl wrote the directory, not anything about the
world.) Caught here rather than at release.yml's own drift check, which
now has nothing to fail on.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>"
	echo "==> Committed regenerated example worlds"
else
	echo "    both example yaml worlds already match; nothing to regenerate"
fi

# 4. The local checks. release.yml's full-suite job is the authoritative
# gate -- these are a chance to fail locally, before a runner spends half
# an hour reaching the same answer, not a replacement for it.
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

# The cross-compile. Cheap next to everything above -- three targets,
# no cgo, so it is the host's own Go toolchain three times -- and it is
# the only check anywhere that builds for a platform other than this one:
# neither `make check` nor `make play` can notice that dlmud has stopped
# compiling for Windows. release.yml runs the same script and would catch
# it, twenty minutes further in.
if command -v zip >/dev/null 2>&1; then
	echo "==> make dist"
	make dist
else
	echo "==> skipping make dist locally (no zip here); release.yml runs it for real"
fi

# 5. Push main, then hand the version to release.yml and wait. The
# workflow tags, releases and pushes the image only if its suite passes;
# if it does not, this exits non-zero having changed nothing on origin
# except main, which was already pushed and already green on go.yml.
git push origin main
head=$(git rev-parse HEAD)
repo=$(git remote get-url origin | sed -E 's#.*/([^/]+/[^/.]+)(\.git)?$#\1#')

# Run IDs increase monotonically per repository, so the newest one that
# exists *before* dispatching is the watermark for finding the run we are
# about to create. `gh workflow run` prints nothing identifying -- that is
# the documented gap in it, and every other way of matching (by branch, by
# sha, by timestamp) misidentifies a concurrent run sooner or later.
before=$(gh run list --workflow=release.yml --limit 1 \
	--json databaseId --jq '.[0].databaseId // 0' 2>/dev/null || echo 0)
[ -n "$before" ] || before=0

echo "==> Dispatching release.yml for $tag at $head"
gh workflow run release.yml --ref main \
	-f version="$tag" -f commit="$head" -f publish=true

run_id=""
waited=0
while [ "$waited" -lt 120 ]; do
	run_id=$(gh run list --workflow=release.yml --event=workflow_dispatch --limit 20 \
		--json databaseId \
		--jq "[.[] | select(.databaseId > $before)] | sort_by(.databaseId) | .[0].databaseId // empty")
	[ -n "$run_id" ] && break
	sleep 3
	waited=$((waited + 3))
done

if [ -z "$run_id" ]; then
	echo "release.sh: dispatched, but no new release.yml run appeared within ${waited}s" >&2
	echo "nothing has been tagged or published. Find the run and watch it by hand:" >&2
	echo "    gh run list --workflow=release.yml -R $repo" >&2
	exit 1
fi

echo "==> Watching run $run_id"
if ! gh run watch "$run_id" --exit-status; then
	echo >&2
	echo "release.sh: the release suite failed. $tag was NOT tagged, released or pushed." >&2
	echo "Fix it, merge the fix, and run this again for the same version:" >&2
	echo "    make release BUMP=$tag" >&2
	echo "    gh run view $run_id --log-failed -R $repo" >&2
	exit 1
fi

git fetch origin --tags >/dev/null 2>&1 || true

echo
echo "==> Released $tag"
gh release view "$tag" --json url --jq '"    " + .url' 2>/dev/null || true
echo "    ghcr.io/$repo:$next  (also :${next%.*} and :latest)"
