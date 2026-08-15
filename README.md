# Disgracelands — Reborn

Reviving Disgracelands, a CircleMUD-based MUD played (mostly via
[Redbrick](https://www.redbrick.dcu.ie/), DCU's student-run Internet
society) from roughly 2001 to 2008. This repo is the revived codebase,
seeded from the archive of the original server.

It is **not** a stock CircleMUD checkout: this is CircleMUD 3.0 patchlevel
20 plus the OasisOLC and DG Scripts add-ons plus years of Disgracelands'
own local modifications (marked `<DoC>`/`</DoC>` in the source), pulled
out of the tree that was actually live the longest — see
`docs/investigations/circlemud-archive-report.md` for how that was
determined, and why it isn't the more "final"-looking but short-lived
CircleMUD 3.1 upgrade attempt the original project also has lying around.

## Status / where to start

It compiles and boots on modern Linux and serves the real Disgracelands
login banner with the original world loaded. It does **not** yet have any
player data, and the live game still reads/writes an old binary save
format that hasn't been wired up to anything portable — see `TODO.md` for
what's actually left.

- **Building it** (both trees): `BUILDING.md`
- **What's left to do**: `TODO.md`
- **All documentation, with a map of what's where**: `docs/README.md`

The short version of that map:

- **Running the Go server**: `docs/configuration.md` and
  `docs/operations.md`
- **The Go port's design and phasing**: `docs/proposals/go-port-plan.md`
- **How Disgracelands got here, and what's custom about it**:
  `docs/investigations/` — the archive investigation, the timeline, the
  non-stock feature inventory, and the player-file format work

## Repo structure

```
src/            The game itself (C source + Makefiles). Start here for code.
cnf/            autoconf input (configure.in, acconfig.h) - see BUILDING.md
configure       Generated from cnf/ - run this before `make`
cmd/            Go port: the dlmud server and dlctl tooling binaries.
internal/       Go port: everything else. In progress, no game in it yet -
                see docs/proposals/go-port-plan.md. The C tree above stays the
                working game and the reference implementation throughout.
build/          Dockerfile and compose file for the Go server.
lib/            Runtime game data: world files, help text, boards, etc.
                No player data ships here - see "Player data" below.
bin/            Build output (compiled binaries). Not committed - gitignored.
log/            Runtime logs. Not committed - gitignored, kept as empty dir.
doc/            Original stock CircleMUD documentation (building, coding,
                running, wizhelp, etc.) - upstream reference material,
                distinct from this project's own docs/ below.
docs/           This project's own documentation. The root is operator
                docs for the Go server; docs/proposals/ is work not yet
                done; docs/investigations/ is archaeology on the original
                codebase. See docs/README.md.
tools/          Modern tooling written for this revival (e.g. the binary
                player-database-to-ascii_pfiles converter) - distinct
                from src/util/ (the original CircleMUD-era utilities) and
                reference/ below. See tools/README.md.
reference/      Code-only snapshots of the two other Disgracelands-lineage
                codebases (the pre-upgrade CircleMUD3 baseline itself, and
                the abandoned CircleMUD 3.1 "WipeMud" upgrade attempt) -
                kept for comparison/porting work without needing the full
                original archive. See reference/README.md.
autorun*, automaint, macrun.pl, vms_autorun.com
                Original CircleMUD/Disgracelands operational scripts for
                keeping the server running across restarts, on various
                platforms. Not audited or relied on yet - see TODO.md.
FAQ, ChangeLog  Upstream CircleMUD project documents, kept as-is.
```

## Player data

No player accounts, passwords, mail, or house/object saves are committed
here, deliberately, and never have been — see `.gitignore` and the first
commit's message for the reasoning (this was real ex-players' data:
password hashes, private in-game mail, connection hosts). A checkout with
no `lib/etc/players` is the *normal* fresh-install state, not a broken
one: CircleMUD auto-creates it on boot, and whoever registers the first
character is automatically promoted to Implementor (top wizard) — that's
original stock CircleMUD behavior (`src/db.c`, "if this is our first
player --- he be God"), not something added for this revival.

If you have access to the original archive and want the real 2001–2008
roster back for local testing, see `TODO.md` for how — it stays off git
either way.
