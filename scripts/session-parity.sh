#!/bin/sh
#
# Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
# of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
# Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
# (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
# and DikuMUD licenses; see LICENSE. Non-commercial use only.
#
# Play the same script against both servers and diff what they say.
#
# The world-parity harness compares what the two servers *loaded*; this
# compares what they *say*. Plan §11 calls it the missing piece, and it is:
# everything else in the suite compares the Go against a reading of the C or
# against an oracle written by hand, and a reading is what has been wrong
# repeatedly.
#
# Both servers run on a throwaway copy of examples/stock/binary/ with an
# empty roster, so the first character created is an implementor on each —
# the same character, with the same powers, on both sides.
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
prepare_lib() {
	target=$1
	mkdir -p "$target"
	# -L so the copy is of the data rather than of any symlinks in it.
	cp -RL examples/stock/binary/. "$target/"
	# The player-bearing directories: emptied, not removed, because both
	# servers expect them to exist.
	for d in pfiles plrobjs plralias house; do
		rm -rf "${target:?}/$d"
		mkdir -p "$target/$d"
	done
	for d in plrobjs/A plrobjs/B plrobjs/C plrobjs/N plrobjs/O plrobjs/P plrobjs/Z; do
		mkdir -p "$target/$d"
	done
	rm -f "$target/etc/players" "$target/etc/plrmail" "$target/etc/hcontrol"
	: > "$target/etc/players"

	# Two bulletin boards the C server needs and the shipped world does not
	# have.
	#
	# `boards.c` in the patched tree declares six boards (boards.c:67-72) and
	# two of them — 3094 "suggestion" and 3095 "pkill" — are Disgracelands
	# additions whose objects only ever existed in the archived world.
	# `examples/stock/binary/` here is stock CircleMUD 3.0 bpl20, which has
	# 3096-3099 and no more, so the C server hits "SYSERR: Fatal board error:
	# board vnum 3095 does not exist!" and dies the moment an immortal looks
	# at the board room.
	#
	# So the scratch copy gets them, modelled on 3096 and identical for both
	# servers. Synthetic data for a test, in a directory that is deleted
	# afterwards — examples/stock/binary/ itself is untouched. See
	# docs/deviations.md.
	obj="$target/world/obj/30.obj"
	if [ -f "$obj" ] && ! grep -q "^#3094$" "$obj"; then
		# In ascending vnum order, before #3096. That is not tidiness:
		# `real_object` binary-searches obj_index, which is built in file
		# order, so a record out of order is a record the server cannot find —
		# appending these at the end left them in the file and invisible to
		# the lookup, with the same fatal error as before.
		awk '
			/^#3096$/ && !done {
				for (v = 3094; v <= 3095; v++) {
					print "#" v
					print "board bulletin~"
					print "a bulletin board~"
					print "A bulletin board is mounted on a wall here.~"
					print "~"
					print "13 0 0"
					print "0 0 0 0"
					print "0 0 0"
					print "E"
					print "board~"
					print "If you can read this, the board is not working."
					print "~"
				}
				done = 1
			}
			{ print }
		' "$obj" > "$obj.new"
		mv "$obj.new" "$obj"
	fi
}

# A free port, asked of the kernel rather than guessed.
free_port() {
	python3 -c 'import socket;s=socket.socket();s.bind(("127.0.0.1",0));print(s.getsockname()[1]);s.close()'
}

CLIB="$OUT/clib"
GLIB="$OUT/glib"
prepare_lib "$CLIB"
prepare_lib "$GLIB"

CPORT=$(free_port)
GPORT=$(free_port)
SEED=20080101

echo "==> Booting the C server on $CPORT"
# -S fixes the RNG seed, which is a <DoC> addition for exactly this harness.
# -q skips the rent scan; -d must be absolute because the C server chdir()s.
"$ROOT/$CSERVER/bin/circle" -q -S "$SEED" -d "$CLIB" "$CPORT" >"$OUT/c.log" 2>&1 &
CPID=$!

echo "==> Booting the Go server on $GPORT"
go run ./cmd/dlmud \
	--lib-dir="$GLIB" \
	--listen-telnets= --listen-telnet="127.0.0.1:$GPORT" \
	--rng=circle --rng-seed="$SEED" \
	--log-level=error >"$OUT/g.log" 2>&1 &
GPID=$!

# Wait for both to accept. A server that never comes up is a failure worth
# reporting as itself rather than as an empty transcript.
for port in "$CPORT" "$GPORT"; do
	waited=0
	until python3 -c "import socket,sys;s=socket.socket();sys.exit(0 if s.connect_ex(('127.0.0.1',$port))==0 else 1)" 2>/dev/null; do
		waited=$((waited + 1))
		if [ "$waited" -gt 120 ]; then
			echo "!!! nothing listening on $port after 60s" >&2
			echo "--- C log ---"; tail -20 "$OUT/c.log" >&2 || true
			echo "--- Go log ---"; tail -20 "$OUT/g.log" >&2 || true
			exit 1
		fi
		sleep 0.5
	done
done

SCRIPTS=${*:-testdata/parity/*.session}
STATUS=0

for script in $SCRIPTS; do
	name=$(basename "$script" .session)
	echo "==> $name"
	go run ./cmd/dlctl parity session \
		--script="$script" --c-addr="127.0.0.1:$CPORT" --go-addr="127.0.0.1:$GPORT" \
		--out-dir="$OUT/$name" || STATUS=1
done

if [ "$STATUS" -ne 0 ]; then
	echo
	echo "The C server is the reference implementation: where the two differ,"
	echo "the Go server is what is wrong. See docs/proposals/go-port-plan.md §11."
fi
exit "$STATUS"
