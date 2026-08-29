#!/bin/sh
#
# Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
# of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
# Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
# (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
# and DikuMUD licenses; see LICENSE. Non-commercial use only.
#
# Run every fuzz target in the tree for a real budget, one at a time.
#
# `go test -fuzz` takes exactly one target per invocation -- the regexp has
# to match one function in one package, because a fuzzing run owns the
# process -- so "run the fuzzers" is a loop rather than a flag, and this is
# that loop. It is the half of the fuzz budget docs/proposals/yaml-only.md
# §10 proposed and nobody built: the seed corpora already replay on every
# push, because the targets are ordinary Go tests and `go test ./...` runs
# their seeds, but until this nothing had ever generated an input the seeds
# did not already contain. Twelve checked-in corpus entries looked like
# coverage and were regression tests.
#
# The targets are *discovered*, not listed here. A list in this file is a
# list that goes stale the first time somebody adds a fifth target and does
# not know to come here -- the same shape of failure as a flag whose only
# implementation stopped being linked in (issue #274). "go test -list" is
# asked instead, so a new FuzzXxx anywhere in the tree is picked up by every
# existing caller with no change to this script or to release.yml.
#
# Usage: scripts/fuzz.sh [fuzztime]
#
#   scripts/fuzz.sh          # 1m per target
#   scripts/fuzz.sh 30s      # a quicker sanity pass
#   scripts/fuzz.sh 10m      # a real hunt
#
# On a finding, Go writes the failing input to that target's own
# testdata/fuzz/<Target>/ directory and the run fails. That file is the
# regression test -- commit it. This script prints it on the way out,
# because the most common place to run this is CI, where the working tree
# is thrown away afterwards and an uncommitted crasher is a finding nobody
# can reproduce.

set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
FUZZTIME=${1:-1m}
GO=${GO:-go}

cd "$ROOT"

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# Discover every target, as "package<TAB>Func" pairs.
#
# `go test -list` prints a package's matching function names and then that
# package's own result line, so the names arrive *before* the package they
# belong to. Buffer the names and flush them when the result line names
# their package. A package with no match prints only its result line, which
# flushes nothing.
$GO test -list '^Fuzz' ./... 2>/dev/null | awk '
	/^Fuzz/           { pending[++n] = $0; next }
	/^(ok|FAIL|\?)/   { for (i = 1; i <= n; i++) print $2 "\t" pending[i]; n = 0 }
' > "$WORK/targets"

count=$(wc -l < "$WORK/targets" | tr -d ' ')
if [ "$count" -eq 0 ]; then
	echo 'no fuzz targets found: has "go test -list" changed its output?' >&2
	exit 1
fi

echo "==> $count fuzz targets, $FUZZTIME each"

while IFS="$(printf '\t')" read -r pkg fn; do
	echo
	echo "==> $fn ($pkg)"

	# -run '^$' keeps the package's ordinary tests from re-running once per
	# target. Nothing is skipped by it: the fuzzing engine replays the seed
	# corpus itself before it starts generating.
	if $GO test -run '^$' -fuzz "^${fn}\$" -fuzztime "$FUZZTIME" "$pkg"; then
		continue
	fi

	echo
	echo "==> $fn failed. The input it failed on:"
	dir="$($GO list -f '{{.Dir}}' "$pkg")/testdata/fuzz/$fn"
	# Newest first, so this is the crasher this run just wrote rather than
	# a seed that has been committed since forever.
	newest=$(ls -t "$dir" 2>/dev/null | head -1)
	if [ -n "$newest" ]; then
		# Capped: a world-file crasher is routinely tens of kilobytes,
		# and a CI log that has to be scrolled past is a log nobody
		# reads. The head of it plus the path is enough to know what
		# was hit; the file itself is what gets committed.
		echo "--- $dir/$newest (first 2kB of $(wc -c < "$dir/$newest") bytes)"
		head -c 2048 "$dir/$newest"
		echo
		echo
		echo "==> Commit that file: it is this bug's regression test."
		echo "    Minimise it first if it is large -- the input that"
		echo "    happened to fail is rarely the smallest one that does."
	else
		# A seed corpus entry failed rather than a generated input, so
		# there is nothing new to commit -- the reproducer is already in
		# the tree, either as an f.Add in the target or as a committed
		# corpus file. That is a plain test failure wearing fuzzing
		# clothes, and `go test ./...` would have caught it too.
		echo "(nothing new in $dir: a seed failed, not a generated input,"
		echo " so the reproducer is already committed. See the output above.)"
	fi
	exit 1
done < "$WORK/targets"

echo
echo "==> All $count targets survived $FUZZTIME each"
