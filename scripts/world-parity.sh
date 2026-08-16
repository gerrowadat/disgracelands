#!/bin/sh
#
# Check that the Go loader and the C loader hold the same world.
#
# Builds both servers, has each dump the world it loaded as canonical JSON,
# and diffs the two. Exits non-zero on any difference.
#
# This is Phase 1's acceptance criterion from docs/proposals/go-port-plan.md,
# made runnable: without it, "the Go loader reproduces the C loader's view
# exactly" is an argument rather than a test.
#
# Usage: scripts/world-parity.sh [world-dir]

set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORLD_DIR=${1:-data/world}
OUT=$(mktemp -d)
trap 'rm -rf "$OUT"' EXIT

cd "$ROOT"

# The C server lives in reference/, which is the only place C code lives; see
# reference/moderncserver/README.md. It needs pre-C99 flags to build on a
# modern compiler, for the reasons that file explains.
#
# ./configure regenerates src/Makefile and reference/moderncserver/src/util/Makefile, which are both
# committed, so they are saved and put back afterwards. Running a test should
# not leave the working tree dirty.
CSERVER=reference/moderncserver

if [ ! -x "$CSERVER/bin/circle" ]; then
	echo "==> Building the C server"
	for mk in "$CSERVER/src/Makefile" "$CSERVER/reference/moderncserver/src/util/Makefile"; do
		[ -f "$mk" ] && cp "$mk" "$OUT/$(echo "$mk" | tr / _)"
	done

	( cd "$CSERVER" && \
		CFLAGS="-std=gnu89 -fcommon -Wno-implicit-function-declaration -w" \
		CC=gcc ./configure >/dev/null )

	for mk in "$CSERVER/src/Makefile" "$CSERVER/reference/moderncserver/src/util/Makefile"; do
		saved="$OUT/$(echo "$mk" | tr / _)"
		[ -f "$saved" ] && cp "$saved" "$mk"
	done

	make -C "$CSERVER/src" >/dev/null
fi

echo "==> Dumping the world from the C server"
# -J loads the world exactly as a real boot does, including the renumbering
# passes, then writes JSON and exits without opening a socket. -d is given as
# an absolute path because the C server chdir()s into it.
"$CSERVER/bin/circle" -J "$OUT/c.json" -d "$ROOT/$(dirname "$WORLD_DIR")" >/dev/null 2>&1

echo "==> Dumping the world from the Go server"
# --parity omits the two mob fields the C server does not retain after
# loading; see internal/persist/world/dump.go.
go run ./cmd/dlctl world dump --world-dir="$WORLD_DIR" --parity --out="$OUT/go.json"

echo "==> Comparing"
# Both dumps are the same canonical format, but one is pretty-printed and the
# other is not, so compare the parsed structures rather than the bytes.
python3 - "$OUT/c.json" "$OUT/go.json" <<'PY'
import json, sys, collections

c = json.load(open(sys.argv[1]))
g = json.load(open(sys.argv[2]))

diffs = collections.Counter()
examples = {}


def record(path, a, b):
    # Group by field rather than by record, so "every mob's act_flags differ"
    # reads as one problem instead of nine hundred.
    key = '.'.join(p for p in path if not p.isdigit())
    diffs[key] += 1
    if key not in examples:
        examples[key] = ('.'.join(path), a, b)


def walk(path, a, b):
    if type(a) is not type(b) and not (isinstance(a, (int, float)) and isinstance(b, (int, float))):
        record(path, a, b)
        return
    if isinstance(a, dict):
        for k in set(a) | set(b):
            walk(path + [k], a.get(k), b.get(k))
    elif isinstance(a, list):
        if len(a) != len(b):
            record(path + ['(length)'], len(a), len(b))
            return
        for i, (x, y) in enumerate(zip(a, b)):
            walk(path + [str(i)], x, y)
    elif a != b:
        record(path, a, b)


for section in ('counts', 'zones', 'rooms', 'mobiles', 'objects', 'shops'):
    walk([section], c.get(section), g.get(section))

counts = c['counts']
print("    %d rooms, %d mobiles, %d objects, %d zones, %d shops" % (
    counts['rooms'], counts['mobiles'], counts['objects'],
    counts['zones'], counts['shops']))

if not diffs:
    print("    identical")
    sys.exit(0)

print("\n    %d differing field(s) in %d kind(s):\n" % (sum(diffs.values()), len(diffs)))
for key, n in diffs.most_common(20):
    path, a, b = examples[key]
    print("    %6d  %s" % (n, key))
    print("            first at %s" % path)
    print("            C : %s" % repr(a)[:100])
    print("            Go: %s" % repr(b)[:100])
sys.exit(1)
PY
