#!/bin/sh
#
# Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
# of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
# Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
# (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
# and DikuMUD licenses; see LICENSE. Non-commercial use only.
#
# Play a script against both servers and diff what they say.
#
# This is the by-hand route, for writing a new script: `make session-parity`
# (test/parity) is the suite, and it is what plays testdata/parity/ properly
# — with a triage table for the differences that are already decided, which
# this script has no notion of and will simply print. Write a script here,
# then add it to test/parity's own table.
#
# The world-parity harness compares what the two servers *loaded*; this
# compares what they *say*. Plan §11 calls it the missing piece, and it is:
# everything else in the suite compares the Go against a reading of the C or
# against an oracle written by hand, and a reading is what has been wrong
# repeatedly.
#
# A freshly booted pair of servers per script, on their own throwaway copies
# of examples/stock/binary/ with an empty roster — so the first character a
# script creates is an implementor on each side, and nothing one script does
# is visible to the next. Sharing one pair is what test/parity used to do and
# stopped doing: an object one script failed to pick up is still lying in the
# temple when the next one walks through it, and every room description from
# there on differs for a reason that is not its own.
#
# Usage: scripts/session-parity.sh [script.session ...]

set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

OUT=$(mktemp -d)
CPID=""
GPID=""
cleanup() {
	[ -n "$CPID" ] && kill "$CPID" 2>/dev/null || true
	[ -n "$GPID" ] && kill "$GPID" 2>/dev/null || true
	rm -rf "$OUT"
}
trap cleanup EXIT

CSERVER=reference/moderncserver
MAKEFILES="$CSERVER/src/Makefile $CSERVER/src/util/Makefile"

if [ ! -x "$CSERVER/bin/circle" ]; then
	echo "==> Building the C server"
	for mk in $MAKEFILES; do
		[ -f "$mk" ] && cp "$mk" "$OUT/$(echo "$mk" | tr / _)"
	done
	( cd "$CSERVER" && \
		CFLAGS="-std=gnu89 -fcommon -Wno-implicit-function-declaration -w" \
		CC=gcc ./configure >/dev/null )
	for mk in $MAKEFILES; do
		saved="$OUT/$(echo "$mk" | tr / _)"
		[ -f "$saved" ] && cp "$saved" "$mk"
	done
	make -C "$CSERVER/src" >/dev/null
fi

# A scratch data directory each, so neither server's saves reach the other and
# both start with an empty roster.
#
# The preparation itself lives in Go (internal/parity/stage.go, run here as
# `dlctl parity stage`) because test/parity needs exactly the same directory
# and a second copy of it in shell was a standing invitation for the two
# harnesses to end up comparing different games. What it does, and why, is
# commented there: the roster is emptied so that the first character created
# is an implementor on both sides, and two board objects the patched C server
# dies without are added to the copy.
prepare_lib() {
	go run ./cmd/dlctl parity stage --from-dir=examples/stock/binary --to-dir="$1"
}

SEED=20080101

# A free port, asked of the kernel rather than guessed.
free_port() {
	python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()'
}

SCRIPTS=${*:-testdata/parity/*.session}
STATUS=0
RUN=0

for script in $SCRIPTS; do
	name=$(basename "$script" .session)
	echo "==> $name"

	RUN=$((RUN + 1))
	CLIB="$OUT/$RUN/clib"
	GLIB="$OUT/$RUN/glib"
	prepare_lib "$CLIB"
	prepare_lib "$GLIB"

	CPORT=$(free_port)
	GPORT=$(free_port)

	# -S fixes the RNG seed and -M holds the mobiles still, both <DoC>
	# additions for exactly this harness. -q skips the rent scan; -d must be
	# absolute because the C server chdir()s.
	"$ROOT/$CSERVER/bin/circle" -q -M -S "$SEED" -d "$CLIB" "$CPORT" >"$OUT/$RUN/c.log" 2>&1 &
	CPID=$!
	go run ./cmd/dlmud \
		--lib-dir="$GLIB" \
		--listen-telnets= --listen-telnet="127.0.0.1:$GPORT" \
		--rng=circle --rng-seed="$SEED" --freeze-mobiles \
		--log-level=error >"$OUT/$RUN/g.log" 2>&1 &
	GPID=$!

	# Wait for both to accept. A server that never comes up is a failure
	# worth reporting as itself rather than as an empty transcript.
	for port in "$CPORT" "$GPORT"; do
		waited=0
		until python3 -c "import socket,sys;s=socket.socket();sys.exit(0 if s.connect_ex(('127.0.0.1',$port))==0 else 1)" 2>/dev/null; do
			waited=$((waited + 1))
			if [ "$waited" -gt 120 ]; then
				echo "!!! nothing listening on $port after 60s" >&2
				echo "--- C log ---"; tail -20 "$OUT/$RUN/c.log" >&2 || true
				echo "--- Go log ---"; tail -20 "$OUT/$RUN/g.log" >&2 || true
				exit 1
			fi
			sleep 0.5
		done
	done

	go run ./cmd/dlctl parity session \
		--script="$script" --c-addr="127.0.0.1:$CPORT" --go-addr="127.0.0.1:$GPORT" \
		--out-dir="$OUT/$RUN/$name" --ignore-colour || STATUS=1

	kill "$CPID" 2>/dev/null || true
	kill "$GPID" 2>/dev/null || true
	CPID=""
	GPID=""
done

if [ "$STATUS" -ne 0 ]; then
	echo
	echo "The C server is the reference implementation: where the two differ,"
	echo "the Go server is what is wrong -- but check docs/deviations.md's"
	echo "\"What the session-parity suite found\" before assuming a difference is"
	echo "new. This script prints every difference; test/parity is where the"
	echo "decided ones are already written down."
fi
exit "$STATUS"
