# moderncserver — the C server

This is the Disgracelands game as it actually is: CircleMUD 3.0 patchlevel 20
plus OasisOLC plus years of local modification, patched to compile and run on
a modern 64-bit Linux box.

It is the codebase that was played from roughly 2001 to 2008. It works. Until
the Go port at the repository root can do everything it does, **this is the
real server** and the Go tree is the project.

## Why it lives in reference/

`reference/` is the only place C code lives in this repository. The root is
the Go port and nothing else.

That is a statement about where the project is going, not a demotion. This
tree has two active jobs, and it will keep both for a long time:

1. **It is the reference implementation.** Where the Go port and this tree
   disagree about anything, this tree is right by definition — it is the one
   that has been running the game. Every parser in the Go port was written by
   reading the corresponding function here.
2. **It is the parity oracle.** `scripts/world-parity.sh` builds this server,
   has it dump the world it loaded, and diffs that against the Go server's
   dump. That check runs in CI on every change. See "The world dump" below.

Nothing here should be deleted until the Go port has been running the real
game for a while.

## What is in here

```
src/            The game itself. Start here for code.
                src/util/  Original CircleMUD-era utilities (autowiz,
                           mudpasswd, listrent, ...), built by `make utils`.
cnf/            autoconf input (configure.in, acconfig.h)
configure       Generated from cnf/ - run this before `make`
doc/            The original stock CircleMUD documentation - building,
                coding, running, wizhelp. Upstream material, kept as-is.
                Distinct from the repository's own docs/ at the root.
bin/            Build output. Not committed.
log/            Runtime logs. Not committed.
lib             Symlink to ../../data, the shared game data directory. The
                server's compiled-in default is "lib" (config.c's DFLT_DIR),
                so this keeps it working unchanged from its new home.
autorun*, automaint, macrun.pl, vms_autorun.com
                Original operational scripts for keeping the server running
                across restarts, on various platforms. Not audited.
FAQ, ChangeLog  Upstream CircleMUD project documents, kept as-is.
```

Game data is **not** here. It lives in `examples/stock/binary/` at the
repository root, because both servers read it and it is not C code. The
`lib` symlink above points at it.

## Building it

This is pre-C99 code from ~2002, written against a far more permissive
compiler than anything from the last decade. GCC 14+ turns several things it
relies on into hard errors by default, so both `configure` and `make` need
non-default flags:

```sh
cd reference/moderncserver
export CFLAGS="-std=gnu89 -fcommon -Wno-implicit-function-declaration -w"
export CC=gcc
./configure
make
```

Three flags and why:

- `-std=gnu89` — the code predates C99 declarations-after-statements and
  implicit `int`.
- `-fcommon` — it relies on tentative definitions being merged, which GCC 10
  stopped doing by default.
- `-w` — suppresses roughly 2002-era warning noise. Several of the suppressed
  warnings are real bugs; see "Known problems" below.

`make utils` (part of `make all`) also builds `src/util/*`.

Running it:

```sh
bin/circle -q 4000
```

## The world dump

`src/worlddump.c` and the `-J <file>` option are the one addition made for
the Go port. They are marked `<DoC>` like every other local change.

```sh
bin/circle -J /tmp/world.json -d ../../data
```

This loads the world exactly as a real boot does — including `renum_world()`
and `renum_zone_table()`, whose effects are the interesting part — then
writes it as canonical JSON and exits without opening a socket or touching
player data. It is read-only with respect to the game and unreachable from
normal operation.

The Go server produces the same format with `dlctl world dump --parity`, and
`scripts/world-parity.sh` at the repository root diffs the two. They
currently agree on every field of all 5,248 records.

If you change a parser here, that check will tell you whether the Go port
still matches. If you change one there and the check fails, the Go port is
what is wrong.

## Known problems

Inherited, not introduced, and none of them fixed:

- **The `sprintf`-into-shared-`buf` patterns** throughout `db.c`,
  `improved-edit.c`, `tedit.c`, `zedit.c`, `listrent.c` and `shopconv.c`.
  Several look like genuine buffer-overflow-shaped bugs — old, apparently
  never triggered, but "apparently never triggered" is not "safe". The `-w`
  flag is hiding them.
- **No 64-bit audit** of anything touching saved binary data. The player
  database is a raw `fwrite()` of a struct whose `long` fields changed width
  when the world moved to 64-bit; see
  `docs/investigations/pfile-conversion.md` and
  `docs/proposals/go-port-plan.md` §4.
- **`crypt(3)` password hashing**, DES, salted with the character's own name
  and truncated to ten stored characters — so only the first eight characters
  of a password ever mattered.
- **Telnet only.** No TLS, no rate limiting, no modern connection hygiene.

None of this has been hardened for 2026. Local, LAN, or VPN-only remains the
sane posture for this tree.

## Licence

The CircleMUD and DikuMUD licences apply to everything here and to the Go
port derived from it. `LICENSE` at the repository root reproduces
`doc/license.doc` verbatim, below a preamble stating this project's own
copyright and that it does not loosen those terms; `doc/license.doc` itself
stays pristine, and `scripts/license-check.sh` checks that the two still
agree byte for byte.

`src/.accepted` records that the licence was accepted and is deliberately
committed — `make all` depends on it, and without it the build runs
`licheck`, which waits for a keypress.

See `docs/proposals/go-port-plan.md` §12 for what the licence requires in
practice.
