# reference/

Everything in this repository that is not the Go port. **C code lives here
and nowhere else.**

```
moderncserver/    The C server: the game as it actually is.
tools/            C helper programs written for this revival.
CircleMUD3-src/   Code-only snapshot of the pre-upgrade baseline.
WipeMud-src/      Code-only snapshot of the abandoned 3.1 upgrade attempt.
```

## `moderncserver/` — the working C server

CircleMUD 3.0 patchlevel 20 plus OasisOLC plus years of local modification,
patched to build and run on modern 64-bit Linux. This is the codebase that
was played from 2001 to 2008. Nothing runs it now — nothing has run
Disgracelands in any language since then — so it is authoritative about
the game rather than serving it.

It has two active jobs: it is the reference implementation the
port is written against, and it is the parity oracle
`scripts/world-parity.sh` checks the port against on every CI run.

See `moderncserver/README.md` for how to build and run it, what the `-J`
world dump is for, and the known problems it carries.

## `tools/` — revival-era C helpers

`bin2ascii.c` converts the original binary player database to the
ascii_pfiles format; `pfiledump.c` prints an ascii pfile back. Both were
written for this revival rather than being part of any original tree, which
is why they sit here rather than inside `moderncserver/`.

`bin2ascii` **must** be built 32-bit — see
`../docs/investigations/pfile-conversion.md`. Phase 2 of the port replaces
both with `dlctl` subcommands that need no 32-bit toolchain; they stay until
that is proven against the same 108 records.

## The two snapshots

Code-only snapshots of the other Disgracelands-lineage codebases, kept so
they are available for comparison without needing the full
multi-hundred-megabyte archive dump this repo was seeded from.

- **`CircleMUD3-src/`** — the tree `moderncserver/` is based on. Kept
  unmodified as a clean diff baseline, so it stays possible to see exactly
  how far `moderncserver/` has drifted from what was archived.
- **`WipeMud-src/`** — the abandoned May 2003 upgrade to CircleMUD 3.1. Not
  the baseline the revival is built from (see
  `../docs/investigations/circlemud-archive-report.md` §7 for why), but it
  has real local modifications of its own — notably a race system
  (`races.c`/`races.h`) that never existed in `CircleMUD3` — worth mining.
  See `TODO.md` item 2.

### What is in the snapshots and what is not

Source only: their `src/`, `cnf/`, `configure`, top-level
`README`/`FAQ`/`ChangeLog`, and the plain-text files under their `doc/`.
Binary `.pdf`/`.ps.gz` documentation from WipeMud's `doc/` was dropped —
same content, not in a format worth carrying around.

No game data, no compiled binaries, no logs, no autoconf-generated
`config.*` files.

Neither snapshot has been test-built the way `moderncserver/` has. They are
reference material, not something expected to compile out of the box without
the same treatment — see `moderncserver/README.md` for what that treatment
is.
