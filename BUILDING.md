# Building

This repository holds two servers. This document is about the Go one, which
is what the root of the repository is.

For the **C server** — the game as it actually is, and the reference the Go
port is written against — see `reference/moderncserver/README.md`. It needs
pre-C99 compiler flags and its own `configure` run, and none of that
interacts with anything here.

## The Go tree

Needs Go 1.25+ and nothing else: no autoconf, no 32-bit toolchain, no
`./configure`.

```sh
go build ./...          # both binaries
go test -race ./...     # -race is not optional here, see the plan's §3.1
go run ./cmd/dlmud --help
```

This document covers *building*. For running and administering the result,
see `docs/operations.md`; for the full settings list,
`docs/configuration.md`. For working on it — a server running locally
against a tiny world, a throwaway data directory, and the checks to run
before pushing — see `docs/developer.md` and the `Makefile` it describes.

Two binaries:

- **`dlmud`** — the server. Every option can also be set from the
  environment (`--lib-dir` ↔ `DL_LIB_DIR`); precedence is flag >
  environment > default. `--help` lists the lot.
- **`dlctl`** — offline tooling: world linting and dumping, player-file
  conversion, inspection and password setting, and converting a whole
  original data directory.
  The jobs `reference/moderncserver/src/util/` and `reference/tools/` do
  today. Any subcommand added before the layer it needs reports which plan
  phase implements it rather than pretending to work.

## Checking against the C server

```sh
scripts/world-parity.sh
```

Builds both servers, has each dump the world it loaded, and diffs them. They
currently agree on every field of all 3,202 records. This runs in CI.

If it reports a difference, the Go loader is what is wrong: the C server is
the reference implementation and the one that has been running the game.

## Current state: Phases 0–4 done, Phase 5 all but finished

`dlmud` loads the world, takes connections over TLS or plaintext telnet, runs
the login, character-creation and main-menu sequence, resets zones, and runs
the game: combat, spells, skills, affects, equipment, death and corpses,
mobiles that act, special procedures, the channels and socials, shops, banks,
renting, boards, mail, houses, and the immortal commands. Characters autosave,
a linkdead body stays to reconnect to, and it shuts down cleanly on SIGTERM.

285 of the C's 318 commands answer. What is left — `remort`, the OasisOLC
editors, and a tail of small commands, plus `CAN_SEE` and `N.thing` targeting —
is listed one by one in `docs/proposals/go-port-plan.md` §10.

It needs at least one listener, and the TLS listener (on by default) needs a
certificate, so the shortest thing that actually starts is:

```sh
go run ./cmd/dlmud --listen-telnets= --listen-telnet=:4000 --metrics-addr=:9090
```

Plaintext telnet is implemented but off unless asked for; the server warns
when it is on. `--metrics-addr` serves `/metrics`, `/healthz` and `/readyz`.

## Container

```sh
docker build -f build/Dockerfile -t disgracelands .
docker compose -f build/docker-compose.yml up --build
```

The runtime image is distroless/static with no shell (~13MB), which is why
`autorun`'s restart-in-a-shell-loop model is replaced by the container
runtime's restart policy plus SIGTERM handling in the server. `data/` is a
volume, since it is mutable state.

## Where the game data lives

`data/` at the repository root: world files, help text, socials, and —
locally, never committed — player data. Both servers read it. The C server
reaches it through a `lib` symlink, because its compiled-in default is `lib`
(`config.c`'s `DFLT_DIR`).

What ships there is **stock CircleMUD 3.0 bpl20's `lib/`**, unmodified. It is
enough to build, boot, test and compare both servers, which is what the tree
needs it for. The Disgracelands world and text are archive material and are
not in this repo; `dlctl convert` turns a copy of the archive into a directory
either server runs on, and `--lib-dir` points at it.
