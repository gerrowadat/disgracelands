# moderncserver — the C server

This is CircleMUD 3.0 patchlevel 20 plus OasisOLC plus years of local
modification, patched to compile and run on a modern 64-bit Linux box: the
codebase that was played from roughly 2001 to 2008.

It still builds and still works. Nothing is running it, though — nothing
has been running Disgracelands in either language since 2008 — so "the
real server" is a statement about *authority*, not about operation: where
this tree and the Go port disagree about the game, **this tree is what the
game was**. As of 2026-08-23 the Go port at the repository root is
playable, and is the server being developed and played going forward — see
the top-level `README.md`'s "Status" section.

## Why it lives in reference/

`reference/` is the only place C code lives in this repository. The root is
the Go port and nothing else.

This tree has two active jobs, and it will keep both for a long time:

1. **It is the reference implementation for gameplay and compatibility.**
   Where a returning player would notice the difference, or where the two
   servers would read or write data differently, this tree is right by
   definition — it is the one that ran the game. Every parser in the Go
   port was written by reading the corresponding function here.
   (Implementation choices that aren't gameplay- or compatibility-shaped are
   a different question now — `docs/proposals/go-port-plan.md` §0's
   "Fidelity, phase two" — and this tree doesn't settle those.)
2. **It is the parity oracle.** `scripts/world-parity.sh` builds this server,
   has it dump the world it loaded, and diffs that against the Go server's
   dump. That check runs at every release — `.github/workflows/release.yml`,
   not the day-to-day `go.yml`, which is correctness and lint only — and by
   hand with `make parity`. See "The world dump" below.

Nothing here should be deleted. Even now that the Go port is the one being
played, this tree is the answer to every future gameplay or compatibility
question, and deleting it would throw that away for nothing — it costs a
directory.

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
currently agree on every field of all 3,202 records of what ships
(`examples/stock/binary`: 1878 rooms, 569 mobiles, 679 objects, 30 zones, 46
shops). The Disgracelands world itself is 5,248 records and also agreed,
back when `data/` held it, before this repo switched to shipping stock
CircleMUD's `lib/` — `docs/proposals/go-port-plan.md` §10's Phase 1
write-up keeps both counts.

If you change a parser here, that check will tell you whether the Go port
still matches. If you change one there and the check fails, the Go port is
what is wrong.

## Known problems

Inherited, not introduced, and none of them fixed — nor going to be. None
of these have an operational stake: nothing is exposing this tree to
anybody, so they are documented because a reader building it locally
should know what they are running, and because the Go port has to decide
what to do about each one, not because there is a risk here to close.
Where the port does fix one, that is a deviation and gets recorded
(`docs/deviations.md`).

- **The `sprintf`-into-shared-`buf` patterns** throughout `db.c`,
  `improved-edit.c`, `tedit.c`, `zedit.c`, `listrent.c` and `shopconv.c`.
  Several look like genuine buffer-overflow-shaped bugs — old and
  apparently never triggered. Auditing them was an open item until it was
  decided against (issue #143, `TODO.md`'s "Superseded"): a bug reachable
  only by someone who chose to build and run this tree themselves is
  history, not a vulnerability. The `-w` flag is hiding them; that is the
  thing to know if you build it.
- **No 64-bit audit** of anything touching saved binary data. The player
  database is a raw `fwrite()` of a struct whose `long` fields changed width
  when the world moved to 64-bit; see
  `docs/investigations/pfile-conversion.md` and
  `docs/proposals/go-port-plan.md` §4.
- **`crypt(3)` password hashing**, DES, salted with the character's own name
  and truncated to ten stored characters — so only the first eight characters
  of a password ever mattered.
- **Telnet only.** No TLS, no rate limiting, no modern connection hygiene.

None of this has been hardened for 2026, and none of it will be. If you
run this tree at all, run it locally — it exists to be read, dumped and
diffed against the port, not to take connections.

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
