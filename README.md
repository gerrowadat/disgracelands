# Disgracelands — Reborn

Reviving Disgracelands, a CircleMUD-based MUD played (mostly via
[Redbrick](https://www.redbrick.dcu.ie/), DCU's student-run Internet
society) from roughly 2001 to 2008. This repo is the revived codebase,
seeded from the archive of the original server.

The project is a **Go port** of that server. The root of this repository is
that port; the original C server lives in `reference/moderncserver/`, where
it stays buildable, runnable, and authoritative until the port can do
everything it does.

## Status

**The C server works.** It compiles and boots on modern Linux and serves a
playable game. Until the Go port catches up, that is the game — and it stays
the reference implementation either way: where the two disagree, the C is
right by definition, because it is the one that was played.

**The data in this repo is stock.** `data/` is CircleMUD 3.0 bpl20's `lib/`,
unmodified — Midgaard, not Disgracelands. The world, help text and boards the
game actually ran on are in the private archive and are not committed here;
point `--lib-dir` at a converted copy to run the real thing (`dlctl convert`,
see `docs/operations.md`). Everything in `docs/investigations/` describes that
archive rather than what ships.

**The Go port is playable.** Phases 0–4 of
`docs/proposals/go-port-plan.md` are done and Phase 5 is all but finished. It
loads the world, listens on TLS or plaintext telnet, negotiates telnet
properly (hidden passwords, CHARSET, GMCP), logs in an archived character or
creates a new one through the full C creation flow, and shows the main menu —
description editor, background story, change password, delete character —
before putting them in the world. Characters autosave, and a dropped link
leaves the body in the world to reconnect to.

The rules core is there: zones reset and mobiles wander, scavenge and attack;
combat runs on the two-second round with the C's own to-hit and damage
tables; spells, skills, affects, position and regeneration, death and
corpses, equipment that actually changes your numbers, containers, food and
drink, following and grouping. A character can kill something, be paid for
it, and rise a level — which is Phase 4's criterion, and there is a test that
walks it end to end.

The game around it is there too: special procedures, so guildmasters teach and
guild guards turn you away; the channels and the socials; shops, banks,
renting at an inn and the rent files behind it; bulletin boards, mud mail and
player housing; and the immortal commands, from `goto` and `stat` through
`set`, `snoop`, `switch` and the site bans.

**305 of the C's 318 commands answer**, and every slice of Phase 5 is built.
Of the 13 that do not, nine are the OasisOLC and text editors that belong to Phase 6
and the rest is a tail of small things listed command by command in the plan's
§10.

The two servers load the world identically — every field of all 3,202
records — and `scripts/world-parity.sh` checks that in CI.

## Where to start

- **Building either server**: `BUILDING.md`
- **Running and administering the Go server**: `docs/configuration.md`,
  `docs/operations.md`
- **Working on the Go port**: `docs/developer.md` (and `make` for the dev
  targets it describes)
- **The port's design and phasing**: `docs/proposals/go-port-plan.md`
- **The C server**: `reference/moderncserver/README.md`
- **What's left that isn't a phase**: `TODO.md`
- **All documentation, with a map**: `docs/README.md`

## Repo structure

The rule: the root is the Go port. C code lives only under `reference/`.

```
cmd/            The dlmud server and dlctl tooling binaries.
internal/       Everything else in the Go port.
build/          Dockerfile and compose file.
scripts/        Development scripts, notably world-parity.sh.
docs/           This project's own documentation. The root is operator
                docs for the Go server; docs/proposals/ is work not yet
                done; docs/investigations/ is archaeology on the original
                codebase. See docs/README.md.

data/           Runtime game data: stock CircleMUD 3.0 bpl20 lib/, world
                files, help text and socials. Read by both servers, so it
                belongs to neither. Neither the real Disgracelands data nor
                any player data ships here - see "Player data" below.

reference/      Everything that is not the Go port.
  moderncserver/  The C server: the game as it actually is, and the
                  reference implementation the port is written against.
                  Buildable and runnable. See its README.md.
  tools/          C helper programs written for this revival (the binary
                  player-database-to-ascii_pfiles converter and a dumper).
                  Superseded by dlctl's pfile subcommands.
  CircleMUD3-src/ Code-only snapshot of the pre-upgrade baseline.
  WipeMud-src/    Code-only snapshot of the abandoned CircleMUD 3.1
                  upgrade attempt. See reference/README.md.
```

## Player data

No player accounts, passwords, mail, or house/object saves are committed
here, deliberately, and never have been — see `.gitignore` and the first
commit's message for the reasoning (this was real ex-players' data:
password hashes, private in-game mail, connection hosts). A checkout with
no `data/etc/players` is the *normal* fresh-install state, not a broken
one: CircleMUD auto-creates it on boot, and whoever registers the first
character is automatically promoted to Implementor (top wizard) — that's
original stock CircleMUD behavior (`reference/moderncserver/src/db.c`, "if
this is our first player --- he be God"), not something added for this
revival.

If you have access to the original archive and want the real 2001–2008
roster back for local testing, see `TODO.md` for how — it stays off git
either way.

## Licence

This repository is a derivative work of CircleMUD, itself a derivative work
of DikuMUD, and the CircleMUD and DikuMUD licences apply to all of it —
including the Go port, which contains none of CircleMUD's C code but
reimplements its mechanics, file formats and world data. Copyright in the
Disgracelands-specific material is Dave O'Connor's, and that ownership
cannot loosen the inherited terms: **non-commercial use only**, credits
intact and reachable in-game, and this licence shipped with any copy.

`LICENSE` states that, above a marker line, and then reproduces
`reference/moderncserver/doc/license.doc` verbatim and unmodified. Source
files written for this project carry a header saying the same; new ones must
too. `scripts/license-check.sh` checks all of it — the verbatim licence
text, the untouched stock C headers, our own headers, the credit files and
the login sequence — and runs in CI.

See `docs/proposals/go-port-plan.md` §12 for the reasoning and for what
compliance still requires of the unwritten parts of the port.
