# Disgracelands — A CircleMUD-based Modern MUD Implementation

Disgracelands was a CircleMUD-based MUD played (mostly via
[Redbrick](https://www.redbrick.dcu.ie/), DCU's student-run Internet
society) from roughly 2001 to 2008. This repo is the revived codebase,
seeded from the archive of the original server.

The project is a **Go port** of that server, and, as of August 2026, a
playable one. The root of this repository is that port; the original C
server lives in `reference/moderncserver/`, where it stays buildable,
runnable and authoritative — it is what the game was, and it is what
answers any gameplay or compatibility question the port raises.

## Status

**Both servers work; neither is live.** The C server compiles and boots on
modern Linux and would serve a playable game if anyone started it; the Go
port, as of 2026-08-23, does too, in its own right — see below for what's
built. Nobody is running either for real players, and nothing has, in any
language, since 2008. What the C tree is for is being *right*: where it and
the port disagree about gameplay or compatibility, the C wins by
definition, because it is the one that was played — that's narrower than it
used to be, though, see "From here, the two servers are allowed to differ"
below. Phase 7 (`docs/proposals/go-port-plan.md` §10) is what would have to
be true before anything took real connections again, and it hasn't started.

**The data in this repo is stock.** `examples/stock/binary/` is CircleMUD
3.0 bpl20's `lib/`, unmodified — Midgaard, not Disgracelands — and it is
the Go server's default `--lib-dir`, so a fresh clone boots something
playable with no setup. `examples/stock/yaml/` is the same world again,
through this project's own `yaml` format, as a worked example of both
formats side by side (`examples/stock/README.md`). The world, help text
and boards the game actually ran on are in a private archive and are
not committed here; point `--lib-dir` at a converted copy to run the real
thing (`dlctl convert`, see `docs/operations.md`). Everything in
`docs/investigations/` describes that archive rather than what ships.

**Phases 0–5 of `docs/proposals/go-port-plan.md` are done**, every slice of
Phase 5 included. It loads the world, listens on TLS or plaintext telnet,
negotiates telnet properly (hidden passwords, CHARSET, GMCP), logs in an
archived character or creates a new one through the full C creation flow,
and shows the main menu — description editor, background story, change
password, delete character — before putting them in the world. Characters
autosave, and a dropped link leaves the body in the world to reconnect to.

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

**310 of the C's 318 commands answer.** Of the 8 that do not, seven are the
OasisOLC editors — decided against, in favour of editing the world files in
your `--lib-dir` directly and reloading them into the running server without
a restart (`reloadmob`/`reloadzone`/`reloadobj`/`reloadshop`) — and the eighth,
`slowns`, is declined for the same reason its own entry in
`docs/deviations.md` gives: this server does no reverse DNS to slow down.
Phase 6 (OasisOLC) was decided against on those terms.

The data itself is pluggable: run on the original `classic`/`ascii`
file shapes, or convert a whole `lib/` into one `yaml` directory — one
file per zone and per character — with `dlctl import`. See
`docs/design/data-format.md` and `docs/operations.md`.

**From here, the two servers are allowed to differ.** Reaching playable was
the point at which strict fidelity stopped being the right default for
everything: `docs/proposals/go-port-plan.md` §0 ("Fidelity, phase two") now
lets new work modernise the implementation — architecture, dependencies,
protocols, tooling, roughly a decade and a half of how server software has
moved on since this stack was designed — freely, holding only two things
fixed: **compatibility** (the on-disk formats and archived credentials the
port already reads and writes) and **gameplay** (the mechanics and balance a
returning player would recognise). Anything that touches either of those is
still recorded in `docs/deviations.md`, exactly as every deliberate
difference has been from the start.

The two servers load the world identically — every field of all 3,202
records — and `scripts/world-parity.sh` checks that at every release (`.github/workflows/release.yml`).

## Where to start

- **Building either server**: `BUILDING.md` — which also lists the
  platforms a release ships binaries for (`linux/amd64`, `linux/arm64`,
  `windows/amd64`; the container image covers the two Linux ones)
- **Running and administering the Go server**: `docs/configuration.md`,
  `docs/operations.md`
- **Working on the Go port**: `docs/developer.md` (and `make` for the dev
  targets it describes)
- **The port's design and phasing**: `docs/proposals/go-port-plan.md`
- **Design decisions that have actually landed**: `docs/design/`, starting
  with the `yaml` data format
- **The C server**: `reference/moderncserver/README.md`
- **What's left that isn't a phase**: `TODO.md`
- **All documentation, with a map**: `docs/README.md`

## Repo structure

The rule: the root is the Go port. C code lives only under `reference/`.

```
cmd/            The dlmud server and dlctl tooling binaries.
internal/       Everything else in the Go port.
test/           The two release-only suites that drive a real server over a
                real socket: test/play (regression, against examples/mini)
                and test/parity (the same scripts typed at both servers).
                Both are build-tagged, so a plain `go test ./...` skips them.
testdata/       Fixtures for those suites -- the parity session scripts.
config/         game.yaml: the shipped, fully-commented example of --config's
                game-tuning file. Every value is commented out, so it is a
                worked example rather than a default that applies.
build/          Dockerfile and compose file.
scripts/        Development scripts, notably world-parity.sh and build-dist.sh.
docs/           This project's own documentation. The root is operator
                docs for the Go server; docs/proposals/ is work not yet
                done; docs/design/ is design decisions that have actually
                landed; docs/investigations/ is archaeology on the
                original codebase. See docs/README.md.

examples/       Runtime game data, not the port's own code. stock/binary/
                is stock CircleMUD 3.0 bpl20 lib/ - world files, help text
                and socials - read by both servers and the Go server's
                default --lib-dir; stock/yaml/ is the same world again in
                this project's own yaml format. See
                examples/stock/README.md. Neither the real Disgracelands
                data nor any player data ships here - see "Player data"
                below.

reference/      Everything that is not the Go port.
  moderncserver/  The C server: the reference implementation and the
                  compatibility/gameplay parity oracle. Buildable and
                  runnable. See its README.md.
  tools/          C helper programs written for this revival (the binary
                  player-database-to-ascii_pfiles converter and a dumper).
                  Superseded by dlctl's --type=pfile commands.
  CircleMUD3-src/ Code-only snapshot of the pre-upgrade baseline.
  WipeMud-src/    Code-only snapshot of the abandoned CircleMUD 3.1
                  upgrade attempt. See reference/README.md.
```

## Player data

No player accounts, passwords, mail, or house/object saves are committed
here, deliberately, and never have been — see `.gitignore` and the first
commit's message for the reasoning (this was real ex-players' data:
password hashes, private in-game mail, connection hosts). A checkout with
no `etc/players` in its lib-dir is the *normal* fresh-install state, not a broken
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
the login sequence — and it is split across the two workflows. The notice
check alone (`--notices`: that every file written for this project carries
its header) runs on **every push and pull request**, because a newly added
file is the one thing that can fail it and every pull request adds files.
The other four run at every release, because they go wrong only when
someone edits `LICENSE`, the credits or the greeting.
(`.github/workflows/go.yml` and `release.yml`; `docs/developer.md`'s "What
runs when" has the split and why.) `make check` runs the full five locally.

See `docs/proposals/go-port-plan.md` §12 for the reasoning and for what
compliance still requires of the unwritten parts of the port.

## A Note on LLM Usage

Much of this code was generated by various Claude models. I've been writing go 
for several years, and could likely have produced a similar codebase if you
gave me several months of uninterrupted time and energy to do so -- I understand 
how the code works, and can make non-LLM patches and updates to it. I choose not
to: this project would 100% not have happened if I had to do it manually.
