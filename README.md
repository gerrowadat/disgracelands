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

**The C server works.** It compiles and boots on modern Linux and serves the
real Disgracelands login banner with the original world loaded. Until the Go
port catches up, that is the game.

**The Go port takes connections, and has no rules in it yet.** Phases 0–3 of
`docs/proposals/go-port-plan.md` are done: it loads the world, listens on
TLS or plaintext telnet, negotiates telnet properly (hidden passwords,
CHARSET, GMCP), logs in an archived character or creates a new one through
the full C creation flow, and shows the main menu — description editor,
background story, change password, delete character — before putting them
in the world to look, move, `who`, `credits`, `help` and `quit`. Characters
autosave, and a dropped link leaves the body in the world to reconnect to.

What is missing is the game itself: no combat, skills, spells or levelling,
no shops, boards or mail, and no zone resets — so the world is empty of
mobiles and objects and you are walking through the scenery. That is
Phase 4 onwards.

The two servers load the world identically — every field of all 5,248
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

data/           Runtime game data: world files, help text, boards, socials.
                Read by both servers, so it belongs to neither. No player
                data ships here - see "Player data" below.

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
