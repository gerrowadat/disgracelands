# Building

This repository holds two servers. This document is about the Go one, which
is what the root of the repository is.

For the **C server** — the reference implementation and the compatibility/
gameplay parity oracle the Go port is checked against — see
`reference/moderncserver/README.md`. It needs
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

### Platforms

`go build ./...` targets any platform Go does; what a release actually
*publishes* is three:

| `GOOS`/`GOARCH` | Ships as | Also an image |
| --- | --- | --- |
| `linux/amd64` | `.tar.gz` on the GitHub release | yes |
| `linux/arm64` | `.tar.gz` on the GitHub release | yes |
| `windows/amd64` | `.zip` on the GitHub release | no |

Nothing in the tree uses cgo, so every one of those cross-compiles from
any host with nothing installed but Go — no cross toolchain, no
emulation, no `apt`. That is also why the container image can be
distroless/static and why the pluggable formats are a compiled-in
registry rather than Go plugins (`docs/proposals/go-port-plan.md` §5.1).

```sh
make dist          # all three, into out/dist, with checksums
```

`scripts/build-dist.sh` is what that runs, and `release.yml` runs the same
script — so `make dist` is how to find out about a broken cross-compile
before a release does. It needs `zip` for the Windows archive; everything
else is Go and `tar`. Each archive holds `dlmud`, `dlctl`, `LICENSE`,
`README.md` and this file.

The archives are reproducible, with one input you have to supply
yourself. Everything inside is pinned — mtimes, permissions, member
order, the build date, and no VCS stamp — so the bytes are a function of
the commit, the version and **the Go toolchain that built them**. That
last one is not pinned by this repository, and a different patch release
of Go produces different code; the binary names the one it was built
with, so reproducing a published archive is:

```sh
dlctl version                       # ... go1.25.14
GOTOOLCHAIN=go1.25.14 VERSION=v1.2.3 COMMIT=<sha> ./scripts/build-dist.sh
```

Without that, `SHA256SUMS` would be a checksum of trust in the runner.

Other targets build and are simply not published — `linux/386`,
`linux/arm`, `darwin/amd64` and `darwin/arm64` all compile today. Adding
one to a release is a line in `PLATFORMS` in `scripts/build-dist.sh`.

Windows is a *build* target, not a tested one: the release checks that
both binaries compile and link for it, and the test suite runs on Linux
only. The server has no Unix-only runtime dependency (no cgo, no unix
sockets, no `syscall` beyond signal names), but the `SIGHUP` that reloads
the game tuning is a Unix signal and will never fire there — restart
instead.

Two binaries:

- **`dlmud`** — the server. Every option can also be set from the
  environment (`--lib-dir` ↔ `DL_LIB_DIR`); precedence is flag >
  environment > default. `--help` lists the lot.
- **`dlctl`** — offline tooling: world linting and dumping, player-file
  conversion, inspection and password setting, and converting a whole
  original data directory — into the classic/ascii shapes the server runs
  on by default (`dlctl convert`), or straight into `yaml` (`dlctl
  import`, `docs/design/data-format.md`).
  The jobs `reference/moderncserver/src/util/` and `reference/tools/` do
  today. Any subcommand added before the layer it needs reports which plan
  phase implements it rather than pretending to work.

## Checking against the C server

```sh
scripts/world-parity.sh
```

Builds both servers, has each dump the world it loaded, and diffs them. They
currently agree on every field of all 3,202 records of `examples/stock/binary`.
This runs at every release (`.github/workflows/release.yml`), not on every
push — day-to-day CI is correctness and lint only, and a full C build is not
that. Run it by hand (`make parity`) after touching either loader.

If it reports a difference, the Go loader is what is wrong: the C server is
the reference implementation and the one that ran the game.

## Current state: Phases 0–5 done, Phase 6 declined, Phase 7 not started

`dlmud` loads the world, takes connections over TLS or plaintext telnet, runs
the login, character-creation and main-menu sequence, resets zones, and runs
the game: combat, spells, skills, affects, equipment, death and corpses,
mobiles that act, special procedures, the channels and socials, shops, banks,
renting, boards, mail, houses, and the immortal commands. Characters autosave,
a linkdead body stays to reconnect to, and it shuts down cleanly on SIGTERM.

310 of the C's 318 commands answer and every slice of Phase 5 is built. What is
left — seven OasisOLC editors and `slowns` — is declined rather than pending:
Phase 6 was decided against, in favour of editing the world files in your
`--lib-dir` directly and reloading them into the running server without a
restart (`reloadmob`/`reloadzone`/`reloadobj`/`reloadshop`) — see
`docs/proposals/go-port-plan.md` §10 for the eight, and its own Phase 6
write-up for what it became instead. Phase 7 (cutover) has not started.

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
runtime's restart policy plus SIGTERM handling in the server. The image
declares `/data` as a volume and defaults to `--lib-dir=/data`; it ships no
world of its own, so that volume is where you mount one. It is mutable
state — players, houses, boards, mail — and an image rebuild must not lose
it.

## Where the game data lives

`examples/stock/binary/` at the repository root: world files, help text,
socials, and — locally, never committed — player data. Both servers read
it, and it is the Go server's own default `--lib-dir`. The C server
reaches it through a `lib` symlink, because its compiled-in default is
`lib` (`config.c`'s `DFLT_DIR`).

What ships there is **stock CircleMUD 3.0 bpl20's `lib/`**, unmodified —
see `examples/stock/README.md`, which also has the same world converted to
this project's own `yaml` format at `examples/stock/yaml/`, as a worked
example of both formats side by side. It is enough to build, boot, test
and compare both servers, which is what the tree needs it for. The
Disgracelands world and text are archive material and are not in this
repo; `dlctl convert` turns a copy of the archive into a directory either
server runs on, and `--lib-dir` points at it.
