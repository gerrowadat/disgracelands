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
- **`dlctl`** — offline tooling: world linting and dumping, and in a later
  phase player-file conversion and inspection. The jobs
  `reference/moderncserver/src/util/` and `reference/tools/` do today.
  Subcommands that need a persistence layer report which plan phase
  implements them.

## Checking against the C server

```sh
scripts/world-parity.sh
```

Builds both servers, has each dump the world it loaded, and diffs them. They
currently agree on every field of all 5,248 records. This runs in CI.

If it reports a difference, the Go loader is what is wrong: the C server is
the reference implementation and the one that has been running the game.

## Current state: Phase 1 done, no game yet

`dlmud` boots, reports itself ready, serves diagnostics and shuts down
cleanly on SIGTERM. The world loader is built (Phase 1) but nothing yet uses
it at run time: players cannot connect, because the listeners and the pulse
loop arrive in Phase 3. See `docs/proposals/go-port-plan.md` §10.

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

`data/` at the repository root: world files, help text, boards, socials, and
— locally, never committed — player data. Both servers read it. The C server
reaches it through a `lib` symlink, because its compiled-in default is `lib`
(`config.c`'s `DFLT_DIR`).
