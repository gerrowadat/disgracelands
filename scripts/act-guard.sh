#!/usr/bin/env bash
#
# Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
# of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
# Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
# (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
# and DikuMUD licenses; see LICENSE. Non-commercial use only.
#
# act-guard.sh -- stop `act --reuse` running one checkout's CI against
# another checkout's files.
#
# act names a job's container and volume after the workflow and the job and
# nothing else: `createContainerName("act", "<workflow name>/<job name>")`,
# hashed (pkg/runner/run_context.go:92, v0.2.89). The working directory is
# not part of it. With --reuse in .actrc the container survives between runs,
# and a reused container keeps the workspace mount it was *created* with.
#
# This repo is routinely developed from git worktrees, so that combination is
# not a corner case here, it is the normal state: every worktree on the
# machine, plus the primary checkout, share one container per job. Whichever
# ran `make ci` first owns it, and every other one afterwards gets a run whose
# workspace is a directory belonging to somebody else. act still copies the
# current tree in on top, so the container accumulates several checkouts of
# the same repo side by side -- which is how a lint run comes back with
# 53 findings in files that are not in your tree at all, and how `go vet`
# fails on an identifier that exists in nobody's branch but the one next door.
#
# It fails the other way too, and that is the dangerous one: a *green* run
# that never looked at your code.
#
# So: before act starts, remove any act container whose workspace belongs to a
# different checkout of this same repository. Containers for unrelated
# projects are left alone -- the test is that the workspace path is a sibling
# under this repo's own worktree root, not merely that the name starts with
# "act-".
#
# Usage: act-guard.sh <workspace-dir>
set -euo pipefail

workspace=${1:?usage: act-guard.sh <workspace-dir>}
workspace=$(cd "$workspace" && pwd -P)

if ! command -v docker >/dev/null 2>&1; then
	exit 0
fi

# Every checkout of this repository shares one .git directory: the primary
# one's. That is what identifies "a different checkout of the same repo"
# rather than "somebody else's project".
common_dir=$(git rev-parse --git-common-dir 2>/dev/null || echo "")
if [ -z "$common_dir" ]; then
	exit 0
fi
repo_root=$(cd "$(dirname "$common_dir")" && pwd -P)

stale=""
for name in $(docker ps -a --filter "name=act-" --format "{{.Names}}" 2>/dev/null); do
	# The workspace is the mount whose source volume is the container's own
	# named volume; act mounts it at the absolute path of the directory it
	# was created from.
	dest=$(docker inspect "$name" \
		--format '{{range .Mounts}}{{if eq .Name "'"$name"'"}}{{.Destination}}{{end}}{{end}}' \
		2>/dev/null || true)
	[ -n "$dest" ] || continue
	[ "$dest" != "$workspace" ] || continue

	# Belongs to this repo (the primary checkout, or a worktree under it)?
	case "$dest" in
	"$repo_root" | "$repo_root"/*) stale="$stale $name" ;;
	esac
done

if [ -n "$stale" ]; then
	echo "==> act: dropping container(s) built for a different checkout of this repo:"
	for name in $stale; do
		echo "      $name"
	done
	# The volume matters more than the container -- see the ci-clean comment
	# in the Makefile. Remove both, and the -env volume that goes with each.
	# shellcheck disable=SC2086 # deliberate word splitting of the name list
	docker rm -f $stale >/dev/null
	for name in $stale; do
		docker volume rm -f "$name" "$name-env" >/dev/null 2>&1 || true
	done
fi
