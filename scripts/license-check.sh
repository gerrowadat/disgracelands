#!/bin/sh
#
# Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
# of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
# Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
# (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
# and DikuMUD licenses; see LICENSE. Non-commercial use only.
#
# Check that this tree still complies with the licenses it inherits.
#
# The CircleMUD and DikuMUD licenses are not a formality here: they are the
# terms this whole repository, Go port included, may be used under at all
# (docs/proposals/go-port-plan.md §12). Their requirements are mechanical
# enough to test, and a compliance claim nobody runs is the kind that quietly
# stops being true. So this checks the five things that can be checked from
# the tree:
#
#   1. LICENSE still carries the CircleMUD and DikuMUD licenses verbatim.
#   2. No stock CircleMUD source file has lost or altered the copyright
#      header it shipped with.
#   3. Files written for this project carry their own notice.
#   4. The credit files the license names are present and intact.
#   5. The login sequence names the DikuMUD and CircleMUD creators.
#
# What it cannot check is the in-game half: that `credits` and
# `help circlemud` display those files, and that every transport sends the
# greeting. Those are Phase 3 commands and get their own tests when they
# land.
#
# Usage: scripts/license-check.sh            # all five
#        scripts/license-check.sh --notices  # check 3 alone

set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

BASELINE=reference/CircleMUD3-src/src
CSRC=reference/moderncserver/src
NOTICE='Part of Disgracelands'
fail=0

note() { echo "    $*"; }
bad() { echo "FAIL: $*"; fail=1; }

# Check 3, hoisted into a function so `--notices` can run it without the
# four around it. It is the only one of the five that costs nothing --
# no C tree, no baseline diff, just a grep over files git already lists --
# and the only one a *newly added* file can fail, which is exactly the
# failure mode a release-time-only check is worst at catching. go.yml runs
# this form on every push; release.yml still runs all five.
#
# Rule 5 of the license's credit terms cuts both ways: the notices in
# CircleMUD's files stay, and the files written here carry notices of their
# own, so that a copy of any one file still points at the terms it is under.
check_notices() {
	echo "==> Files written for this project carry a notice"
	missing=0
	# --others so that a file added but not yet committed is checked too: the
	# point is to catch a missing notice before it lands, not after.
	for f in $(git ls-files --cached --others --exclude-standard \
		'*.go' 'reference/tools/*.c' 'scripts/*.sh'); do
		head -40 "$f" | grep -q "$NOTICE" ||
			{ bad "$f has no license notice"; missing=$((missing + 1)); }
	done
	if [ "$missing" -eq 0 ]; then
		note "every tracked .go, reference/tools/*.c and scripts/*.sh"
	fi
}

case "${1-}" in
"")
	;;
--notices)
	check_notices
	if [ "$fail" -eq 0 ]; then
		echo "==> Every file carries its notice"
		exit 0
	fi
	echo "==> NOT compliant - see docs/proposals/go-port-plan.md §12"
	exit 1
	;;
*)
	echo "usage: $0 [--notices]" >&2
	exit 2
	;;
esac

# 1. LICENSE = our notice, then the upstream license byte for byte.
#
# The license requires itself to be distributed "AS IS", so the CircleMUD and
# DikuMUD text in LICENSE is not edited - our own terms are prepended above a
# marker line and the rest is a copy of the file the C tree ships. Compare the
# tail of LICENSE against that file rather than trusting the two to stay in
# step by hand.
echo "==> LICENSE carries the upstream license verbatim"
UPSTREAM=reference/moderncserver/doc/license.doc
lines=$(wc -l < "$UPSTREAM")
if tail -n "$lines" LICENSE | cmp -s - "$UPSTREAM"; then
	note "last $lines lines of LICENSE are doc/license.doc, unmodified"
else
	bad "LICENSE no longer ends with doc/license.doc verbatim"
	note "the CircleMUD license must ship AS IS; add to the preamble instead"
fi

grep -q 'CircleMUD License' LICENSE || bad "LICENSE has no CircleMUD License section"
grep -q 'DikuMud License' LICENSE || bad "LICENSE has no DikuMUD License section"

# The license has to ship with any copy, and a container image is a copy.
grep -q 'LICENSE /LICENSE' build/Dockerfile ||
	bad "build/Dockerfile does not copy LICENSE into the runtime image"

# So is a release archive, which is the copy most people will actually
# take: release.yml attaches scripts/build-dist.sh's output to every
# GitHub release, and that script stages a fixed list of files alongside
# the two binaries.
grep -q '^EXTRAS=.*LICENSE' scripts/build-dist.sh ||
	bad "scripts/build-dist.sh does not put LICENSE in the release archives"

# 2. Stock C files keep the headers they came with.
#
# "You must not remove, change, or modify any notices of copyright, licensing
# or authorship found in any CircleMUD source code files." The C tree has been
# modified extensively to build on modern Linux, all of it marked <DoC>, and
# none of those edits may touch a file's leading comment block. Compare
# against the pre-upgrade baseline import, which is the same code as shipped.
echo "==> Stock CircleMUD headers are unchanged"
# The leading comment block, which is where CircleMUD puts the file's
# copyright and authorship notice.
header() { awk 'NR==1 && $0 !~ /^\/\*/ { exit } { print } /\*\// { exit }' "$1"; }

# Tracked files only: a configured C tree also has a generated conf.h, and what
# this checks should not depend on whether anyone has built lately. The paths
# below src/ (notably src/util/) match between the two trees, so compare by
# relative path rather than by basename.
checked=0
for f in $(git ls-files "$CSRC/*.c" "$CSRC/*.h"); do
	stock="$BASELINE/${f#"$CSRC/"}"
	if [ ! -f "$stock" ]; then
		# Written for this project, not stock: rule 3 applies instead.
		head -40 "$f" | grep -q "$NOTICE" ||
			bad "$f is not stock and carries no Disgracelands notice"
		continue
	fi
	if [ "$(header "$stock")" = "$(header "$f")" ]; then
		checked=$((checked + 1))
	else
		bad "$f: leading comment block differs from $stock"
	fi
done
note "$checked stock source files, headers identical to the baseline import"

# 3. Our own files say who owns them and under what terms. Defined above,
# because go.yml runs this one on its own.
check_notices

# 4. The credit files, which the license requires be preserved and displayed.
echo "==> Credit text is present and intact"
grep -qi 'CircleMUD was developed from Diku' examples/stock/binary/text/credits ||
	bad "examples/stock/binary/text/credits has lost the stock CircleMUD credit"
grep -q 'Type HELP CIRCLEMUD' examples/stock/binary/text/credits ||
	bad "examples/stock/binary/text/credits no longer points at the CIRCLEMUD help entry"
grep -q '^CIRCLE CIRCLEMUD CREDITS' examples/stock/binary/text/help/info.hlp ||
	bad "the CIRCLEMUD help entry is missing from examples/stock/binary/text/help/info.hlp"
for name in Staerfeldt Nyboe Madsen Seifert Hammer; do
	grep -q "$name" examples/stock/binary/text/credits ||
		bad "examples/stock/binary/text/credits does not name $name"
done

# 5. The login sequence, which must name both sets of creators. "Login
# sequence" is defined by the license as everything a player sees between
# connecting and playing; examples/stock/binary/text/greetings is the file that carries it.
echo "==> The login sequence names the creators"
grep -q 'Jeremy Elson' examples/stock/binary/text/greetings ||
	bad "examples/stock/binary/text/greetings does not name the CircleMUD creator"
for name in Staerfeldt Nyboe Madsen Seifert Hammer; do
	grep -q "$name" examples/stock/binary/text/greetings ||
		bad "examples/stock/binary/text/greetings does not name $name"
done

if [ "$fail" -eq 0 ]; then
	echo "==> Compliant"
	exit 0
fi
echo "==> NOT compliant - see docs/proposals/go-port-plan.md §12"
exit 1
