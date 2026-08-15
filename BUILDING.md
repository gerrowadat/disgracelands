# Building Reborn (Disgracelands) on a modern Linux box

There are two trees here. This document is about the **C tree** (`src/`),
which is the game as it actually is. For the **Go tree** (`cmd/`,
`internal/`), which is the in-progress port, skip to
[Building the Go tree](#building-the-go-tree) at the end — it needs none of
the setup below, and the two builds do not interact.

This is CircleMUD 3.0 bpl20-era C code from ~2002 (patched with OasisOLC,
DG Scripts, and Disgracelands' own local mods — see the `<DoC>` tags
throughout `src/`). It predates C99 and was written against a much more
permissive compiler than anything from the last decade. GCC 14+ (this repo
was built and tested with GCC 15.2.0) turns several things this code relies
on into hard errors by default, so both `./configure` and `make` need
non-default flags.

## One-time setup

If cross-compiling 32-bit helper tools isn't needed, no extra packages are
required beyond a normal C toolchain (`gcc`, `make`).

## Configure and build

```sh
export CFLAGS="-std=gnu89 -fcommon -Wno-implicit-function-declaration -w"
export CC=gcc
./configure
cd src && make
```

`CFLAGS` has to be exported **before** `./configure` runs too, not just
before `make` — `configure`'s own "does the C compiler work" check uses a
K&R-style `main(){return(0);}` test program with no explicit return type,
which GCC treats as a hard error (`-Wimplicit-int` is an error by default
now) unless `-std=gnu89` is in effect.

Why each flag is needed:

- `-std=gnu89` — restores pre-C99 behaviour: implicit `int` return types,
  implicit function declarations as warnings instead of errors, and
  tentative definitions across translation units (`-fcommon` handles the
  common-symbol part of that; GNU89/GNU99 default to `-fcommon` on old GCC
  but modern GCC defaults to `-fno-common`, which breaks multiple `.c`
  files declaring the same global without `extern`).
- `-fcommon` — see above.
- `-Wno-implicit-function-declaration` — belt and braces; without
  `-std=gnu89` this alone won't save you (modern GCC hard-errors on it
  regardless of standard past a certain version), but combined with
  `-std=gnu89` it suppresses the warning noise too.
- `-w` — this code is *loud* under modern `-Wall` (`sprintf` overlap /
  truncation warnings, signed/unsigned comparisons, etc.) — all
  pre-existing and apparently harmless in practice, but not worth fixing
  wholesale right now. Drop `-w` if you want to see them.

## Known source fix already applied

`src/interpreter.c` included `constants.h` before `structs.h`. Every other
`.c` file in the tree gets this the other way around; `constants.h`
declares `extern const struct str_app_type str_app[];` and similar, which
need `structs.h`'s struct definitions in scope first. This was silently
tolerated by whatever compiler this last built cleanly on and is now a hard
error. Fixed by reordering the two `#include` lines.

## Running

```sh
cd .. # back to Reborn/
bin/circle -q 4000
```

`bin/` also gets `src/util/*` (`autowiz`, `mudpasswd`, `listrent`, etc.)
built via `make utils` (part of the default `make all` target already).
None of these are committed to git — they're build products, rebuild them
locally.

## Not yet done

- Nothing here is new, but note the C tree now carries one addition for the
  Go port: `src/worlddump.c` and the `-J <file>` option, which dump the
  loaded world as JSON and exit. It is read-only with respect to the game
  and is not reachable from normal operation. See
  `docs/proposals/go-port-plan.md` §11.
- The `sprintf`-into-shared-`buf` warnings throughout (`db.c`,
  `improved-edit.c`, `tedit.c`, `zedit.c`, `listrent.c`, `shopconv.c`, ...)
  are worth auditing properly before this runs unattended on the open
  internet again — several look like genuine (if old and apparently never
  triggered) buffer-overflow-shaped bugs, not just style complaints.
- No 64-bit-vs-32-bit audit of anything that touches saved binary data
  (the player database — see `docs/investigations/pfile-conversion.md`)
  has been done beyond the player-file struct itself. The Go port
  addresses this systematically rather than incrementally — see
  `docs/proposals/go-port-plan.md` §4.

---

# Building the Go tree

The in-progress port (`docs/proposals/go-port-plan.md`). Needs Go 1.25+ and
nothing else — no autoconf, no 32-bit toolchain, no `./configure`.

This section covers *building*. For running and administering the result,
see `docs/operations.md`; for the full settings list, `docs/configuration.md`.

```sh
go build ./...          # both binaries
go test -race ./...     # -race is not optional here, see the plan's §3.1
go run ./cmd/dlmud --help
```

Two binaries:

- **`dlmud`** — the server. Every option can also be set from the
  environment (`--lib-dir` ↔ `DL_LIB_DIR`); precedence is flag >
  environment > default. `--help` lists the lot.
- **`dlctl`** — offline tooling: world linting, player-file conversion and
  inspection. The jobs `src/util/` and `tools/` do today. Subcommands that
  need a persistence layer report which plan phase implements them.

To check the Go loader still agrees with the C one:

```sh
scripts/world-parity.sh
```

It builds both servers, dumps the world each one loaded, and diffs them.
This also runs in CI.

## Current state: Phase 0

**There is no game in it yet.** `dlmud` boots, reports itself ready, serves
diagnostics and shuts down cleanly on SIGTERM. That is the whole of Phase 0.
Phases 1 and 2 add world and player loading, Phase 3 the listeners and pulse
loop — see `docs/proposals/go-port-plan.md` §10.

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
runtime's restart policy plus SIGTERM handling in the server. `lib/` is a
volume, since it is mutable state.
