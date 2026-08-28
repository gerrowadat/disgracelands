#!/usr/bin/env bash
#
# Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
# of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
# Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
# (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
# and DikuMUD licenses; see LICENSE. Non-commercial use only.
#
# act-clean.sh -- drop the containers, volumes and cache act reuses between
# runs, without taking anybody else's run down with them.
#
# `make ci-clean` used to be `docker rm -f $(docker ps -aq --filter
# name=act-)`. Two things are wrong with that, and both bite in this repo
# specifically because it is developed from several worktrees at once.
#
# **It is not scoped to this repository.** act names a container after the
# workflow and job only -- `act-<workflow>-<job>-<hash>` -- so the filter
# matches any act container on the machine, including ones belonging to
# projects that have nothing to do with this one. Their owner gets a cold
# run, or a broken one if they were mid-flight.
#
# **It takes no lock.** `make ci` runs under flock on ACT_LOCK precisely so
# that two checkouts cannot drive the same container at once; ci-clean
# reached past that and removed the container a live run was executing in.
# The symptoms are `exitcode '137'` on whatever step was running and
# `Error response from daemon: RWLayer of container ... is unexpectedly
# nil`, reported to the *other* session, which has no idea why. That
# happened on 2026-08-28.
#
# Note what is *not* a usable signal: whether the container is running.
# .actrc passes --reuse, so a container stays Up between runs by design --
# that is where the caches live and why a second run takes seconds. "Up"
# means "act has been here", not "act is here now". The lock is the only
# thing that answers the question, which is why this script is invoked
# under the same one `make ci` uses rather than trying to be clever.
#
# Usage: act-clean.sh <workspace-dir> <cache-dir>
set -euo pipefail

workspace=${1:?usage: act-clean.sh <workspace-dir> <cache-dir>}
cache=${2:?usage: act-clean.sh <workspace-dir> <cache-dir>}

if ! command -v docker >/dev/null 2>&1; then
	echo "==> docker is not installed; nothing to clean"
else
	# Every checkout of this repository shares one .git directory: the
	# primary one's. That is what identifies "a container of ours" rather
	# than "somebody else's project", and it is the same test
	# act-guard.sh applies.
	common_dir=$(git rev-parse --git-common-dir 2>/dev/null || echo "")
	if [ -z "$common_dir" ]; then
		echo "==> not in a git checkout; refusing to guess which containers are ours" >&2
		exit 1
	fi
	repo_root=$(cd "$(dirname "$common_dir")" && pwd -P)

	ours=""
	skipped=""
	for name in $(docker ps -a --filter "name=act-" --format "{{.Names}}" 2>/dev/null); do
		# The workspace is the mount whose source volume is the
		# container's own named volume; act mounts it at the absolute
		# path of the directory it was created from. A container with no
		# such mount has never run a job and tells us nothing about who
		# owns it, so leave it be.
		dest=$(docker inspect "$name" \
			--format '{{range .Mounts}}{{if eq .Name "'"$name"'"}}{{.Destination}}{{end}}{{end}}' \
			2>/dev/null || true)
		if [ -z "$dest" ]; then
			skipped="$skipped $name"
			continue
		fi

		case "$dest" in
		"$repo_root" | "$repo_root"/*) ours="$ours $name" ;;
		*) skipped="$skipped $name" ;;
		esac
	done

	if [ -n "$ours" ]; then
		echo "==> removing this repository's act containers and their volumes:"
		for name in $ours; do
			echo "      $name"
		done
		# shellcheck disable=SC2086 # deliberate word splitting of the name list
		docker rm -f $ours >/dev/null
		for name in $ours; do
			# The workspace volume is the one that matters -- act
			# populates it with `docker cp`, which overwrites files
			# but never removes them, so a file deleted on your branch
			# stays in it and keeps getting compiled. The -env volume
			# goes with it.
			docker volume rm -f "$name" "$name-env" >/dev/null 2>&1 || true
		done
	else
		echo "==> no act containers belong to this repository"
	fi

	if [ -n "$skipped" ]; then
		echo "==> left alone (not this repository's):"
		for name in $skipped; do
			echo "      $name"
		done
	fi

	# act-toolcache is deliberately not touched. It is act's shared
	# tool-cache volume, is not bound to any checkout, and holds
	# downloaded toolchains rather than anything that can go stale
	# against a branch -- removing it costs every project on the machine
	# a re-download and fixes nothing this script exists to fix.
fi

# This checkout's own cache store. golangci-lint's cache is keyed by package
# *content* and records absolute paths, so an entry saved by one checkout is
# a legitimate hit for another with the same content -- and replays that
# checkout's filenames. Clearing the containers without clearing this leaves
# exactly the symptom that sends people looking at volumes in the first
# place. ACT_CACHE is already per-checkout, so this needs no scoping.
if [ -e "$cache" ]; then
	rm -rf "$cache"
	echo "==> removed $cache"
else
	echo "==> no cache store at $cache"
fi
