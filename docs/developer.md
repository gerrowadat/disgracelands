# Developing

How to work on the Go server day to day: how to get one running in front of
you, how to poke at it by hand, and what to run before pushing.

For building from source see `BUILDING.md`; for running one for real,
`docs/operations.md`; for the settings, `docs/configuration.md`.

## The Makefile

There is a `Makefile` at the root holding the handful of invocations you
would otherwise keep in shell history. Nothing depends on it — `go build
./...` and `go test ./...` are still the truth, and CI calls the underlying
commands directly rather than going through it — so it can be ignored
entirely, and changed freely.

```sh
make            # lists every target, with the current variable values
```

Common variables, settable on any target:

| Variable | Default | What it does |
|---|---|---|
| `LIB` | `data` | The data directory the server runs against. |
| `PORT` | `4000` | Plaintext telnet port. |
| `TLS_PORT` | `4443` | TLS port for `run-tls`. |
| `METRICS_PORT` | `9090` | `/metrics`, `/healthz`, `/readyz`. |
| `HOST` | `127.0.0.1` | Listen address. Loopback on purpose. |
| `LOG_LEVEL` | `debug` | `debug` for development; `info` is the server's own default. |
| `FLAGS` | — | Extra `dlmud` flags, e.g. `FLAGS="--restrict --log-format=json"`. |

## Getting a server in front of you

```sh
make run-mini     # tiny world, three zones, boots instantly
make connect      # in another terminal
```

`run-mini` passes `--mini-mud`, which switches every world subdirectory from
`index` to `index.mini`: 69 rooms, 51 mobiles, 59 objects, 3 zones instead
of 1,878 / 569 / 679 / 30. It is the right default for testing anything
that is not about the world itself — it loads in milliseconds, and a bug in
zone resets is much easier to see in three zones than in thirty.

The other run targets:

| Target | What you get |
|---|---|
| `make run` | The full world in `data/`, plaintext telnet. |
| `make run-mini` | The same directory, reduced world. |
| `make run-fresh` | A throwaway copy of the data with no players in it. |
| `make run-tls` | The TLS listener, with a self-signed local certificate. |
| `make run LIB=/path/to/lib` | Any other data directory (see below). |

All of them disable the TLS listener except `run-tls`, and enable plaintext
telnet, which the default configuration does not: a server started with no
arguments listens on TLS only and refuses to start without a certificate.
That is correct for production and useless for development, so the dev
targets invert it. They also bind loopback only, because a plaintext telnet
listener carries passwords in the clear and the server will tell you so on
every start.

Ctrl-C stops a run target: the terminal signals the whole process group, so
`go run` and the server both get it, and the server does its real graceful
shutdown on the way out. If you background one instead, kill the `dlmud`
process rather than `make` — `go run` will otherwise leave its child holding
the port.

### A character with powers

The first character created on an empty roster is made an Implementor
(level 34, all skills, `data/pfiles` empty). This is stock CircleMUD
behaviour — db.c's *"if this is our first player --- he be God"*, kept
deliberately in `internal/game/create.go` — and it is the fastest way to get
a god to test with.

You only get one per roster, though, so:

```sh
make run-fresh
```

builds `out/scratch-lib`, a copy of `data/` with the player state stripped
out, and runs against that. Log in, pick any name, and you are an
Implementor. `make clean` or `make scratch` throws it away and you can do it
again.

The copy deliberately excludes `pfiles/`, `plrobjs/`, `plralias/`, `house/`,
`etc/players` and `etc/plrmail`: partly because an empty roster is the whole
point, and partly because a local `data/pfiles` may hold the real ex-players'
password hashes and mail, which should not be getting copied around the
filesystem.

### Against a real data directory

```sh
make run LIB=/srv/disgracelands/lib
```

An *original* CircleMUD directory — an archive of the old server, say — needs
converting first: its player database is the 32-bit binary format the server
will not run on, and its text is not UTF-8.

```sh
make convert FROM=/path/to/old/lib TO=out/converted
make run LIB=out/converted
```

`dlctl convert` reports what it changed and leaves anything it does not
understand alone; `--dry-run` shows you the report without writing. There is
more on it in `docs/operations.md`.

### TLS

TLS is the default transport in production, so it is worth exercising
occasionally rather than only in production:

```sh
make run-tls        # generates out/dev/cert.pem on first use
make connect-tls    # openssl s_client, verifying against that certificate
```

The certificate is self-signed for `localhost`/`127.0.0.1`, lives in `out/`,
and is trusted by nothing. `make clean` removes it; the next `run-tls` makes
a new one.

## Poking at a running server

`make connect` uses `telnet` if you have it and falls back to `nc`. Prefer
`telnet`: the server negotiates properly — ECHO off while you type a
password, CHARSET, GMCP — and `nc` neither answers nor hides the negotiation,
so passwords echo and you get `ÿû` sequences scattered through the greeting.
That is worth seeing once, and annoying every time after.

A real MUD client (Mudlet, TinTin++, blightmud) is the thing to use when you
are testing the protocol layer itself — GMCP in particular is only observable
from something that speaks it.

While it runs:

```sh
make health         # /healthz, /readyz and the dlmud_ metrics
```

`dlmud_pulse_duration_seconds` is the one that matters: the game loop's
budget is `--pulse-interval` (100ms), and pulses that routinely exceed it
mean the world is lagging behind real time for everyone at once. If you have
just written something that runs per pulse, look at it here before assuming
it is free.

Useful `FLAGS` when reproducing something specific:

| Flag | Why |
|---|---|
| `--restrict` | No new players — the `-r` of old runbooks. |
| `--no-specials` | Suppress special procedures. Inert until Phase 5a builds them. |
| `--pulse-interval=1s` | Slows the game loop to human speed; makes pulse-driven behaviour observable. |
| `--log-format=json` | What production logs look like. |
| `--debug-addr=127.0.0.1:6060` | pprof. Never anywhere but loopback. |
| `--allow-legacy-passwords=false` | Reject pre-2008 DES hashes, to check the upgrade path's other branch. |

## What running writes

Everything the server persists goes under its `--lib-dir`. Locally that is
`data/`, and the player-bearing parts of it are gitignored on purpose:
`data/pfiles/`, `data/plrobjs/`, `data/plralias/`, `data/house/`,
`data/etc/players`, `data/etc/plrmail`.

Two rules follow, and they are worth keeping in reflex:

- **Never commit player data.** Real hashes, real mail, real connection
  hosts, from real people who played in 2001. `git status` after a session
  should show nothing new under those paths; if it does, something wrote
  somewhere it should not have.
- **Test destructive things against `out/scratch-lib`**, not `data/`. That is
  what `make run-fresh` is for.

The world files in `data/world` are tracked, so anything that edits the world
in-game — a builder command, once those exist — shows up as a working-tree
change. That is intended: the world is source.

## Before you push

```sh
make check
```

Format check, `go build`, `go vet`, `golangci-lint` (skipped with a note if
it is not installed), `go test -race`, world lint, and the license check.
Green here does not guarantee CI is green, but red here guarantees it is not.

Four things CI does that `make check` does not:

- **World parity** — `make parity`. Builds the C server and diffs the world
  each loader holds. It needs a C toolchain and about a minute, which is why
  it is a separate target. Run it after touching `internal/persist/world/`.
  If it reports a difference, the Go loader is what is wrong.
- **The 32-bit codec checks** in `internal/persist/player`, which skip
  silently without `gcc-multilib`. They verify the layout the archived player
  database is actually in — and CI installs that toolchain **only for a change
  that could affect them**: the `binary` package itself, the two C programs it
  compiles, the headers those include, or the workflow. Everything else skips
  the install, which is otherwise the slowest thing in the job by a wide
  margin. A push to `main` always runs them, because a decision made from a
  diff is only as good as the diff.
- **The libcrypt comparison** in `internal/auth`, which skips unless the
  system libcrypt still does traditional DES. It is the only thing standing
  between a hand-written DES and "probably right".
- **`docs/configuration.md` covers every flag**, and the container build.

`-race` is not optional in the test target. The port keeps the C server's
single game goroutine (`docs/proposals/go-port-plan.md` §3.1), and the whole
safety argument for that design rests on nothing else touching world state —
which only the race detector can actually check.

## Porting a command

Nearly all the remaining work is "port one more command", and it has a shape.

1. **Read the C first, all of it.** Not the function you think you need — the
   one it calls, and the macros in that. `docs/weirdnumbers.md` exists because
   the arithmetic is regularly not what it appears to be, and half its entries
   were found by reading one level deeper than seemed necessary.
2. **Put the rules in `internal/game`** and the words in `internal/session`.
   The test for whether something is a rule: could a mobile do it without
   anybody being connected? Damage, affects and carrying capacity are rules;
   "You get $p." is not.
3. **Check the command table position.** `internal/session/commands.go` is
   ordered to match `interpreter.c`, because the interpreter matches the first
   entry a typed word is a prefix of and that ordering is twenty years of
   muscle memory. Every entry that is not in the obvious place has a comment
   saying which C line put it there, and
   `TestMovementAbbreviationsStillWin` asserts the ones that matter.
4. **Anything that can hurt somebody goes through `session.Violence`**, not
   through its own arithmetic. That interface exists because commands used to
   subtract hit points themselves and none of them noticed when the hit points
   ran out — a kick could kill a mobile and leave it standing there with no
   corpse and nobody paid.
5. **Reproduce the messages exactly**, including the ones that read oddly.
   "You start to use $p as a shield", the missing newline after "You pour the
   %s into the %s.", the typo in "incapacitated an will slowly die". Players
   read those for seven years.

### Testing against the C rather than against your reading of it

The rule that has held: **anything with a division, a cast, or a comment
describing numbers gets an oracle rather than a reading.**

`reference/tools/*.c` holds original C function bodies with the `char_data`
dereferences substituted and nothing else changed. The Go tests compile them
and compare across the whole input space where that is affordable — 30,000 RNG
draws, 1,512,000 to-hit values, 36,288 regeneration values, every saving
throw. Where a table is
transcribed rather than computed, the test re-parses the C source and compares
entry by entry, so a typo in a table is a failing test rather than a subtly
wrong game.

This is not belt and braces. Every oracle written so far has caught at least
one thing, and the mistakes it catches all look right.

## Conventions worth knowing

- **Every file written for this project carries the license notice** — the
  five-line header at the top of any `.go`, `reference/tools/*.c` or
  `scripts/*.sh`. `scripts/license-check.sh` fails the build on a missing
  one, including for files added but not yet committed. Copy the header from
  a neighbouring file when you add one. The `Makefile` carries one too,
  unchecked, on the same principle: a copy of any single file should still
  point at the terms it is under.
- **The C tree in `reference/` is reference, not a dependency.** It has its
  own build (`reference/moderncserver/README.md`), needs pre-C99 flags, and
  nothing in the Go tree calls it except `scripts/world-parity.sh`. Where the
  two disagree about the game, the C server is right by definition: it is the
  one that was played.
- **Where the documentation and the data disagree, the data wins.** The world
  parser is written against the real files in `data/world`, not against
  CircleMUD's `doc/building.txt`, and each divergence is commented where it
  matters.
- **World edits get linted.** `make world-lint` reports exits to deleted
  rooms and resets for mobs that no longer exist. Errors fail CI; warnings do
  not, because the shipped world has had several since long before this repo
  existed. `dlctl world lint --strict` makes warnings fail too.

## The rest of the Makefile

| Target | |
|---|---|
| `make build` | Both binaries into `out/`, version-stamped like the container build. |
| `make test` / `make test-fast` | With and without `-race`. `PKG=./internal/server/...` narrows it. |
| `make cover` | Coverage, opened in a browser. |
| `make fmt` / `make vet` / `make lint` / `make tidy` | The individual checks. |
| `make world-dump` | The loaded world as canonical JSON, in `out/world.json`. |
| `make roster` | The characters in the player directory. |
| `make ctl ARGS="pfile dump --name=Someone"` | Any `dlctl` command. |
| `make docker` / `make compose-up` / `make compose-down` | The container image and the local stack. |
| `make clean` | Removes `out/`: binaries, scratch data directory, dev certificate. |

## Where things are

```
cmd/dlmud/          the server binary
cmd/dlctl/          offline tooling: convert, world lint/dump, pfile commands
internal/config/    every setting, declared once
internal/persist/   world and player formats, one package per format
internal/game/      the game model and the rules ported from the C server
internal/engine/    the pulse loop
internal/server/    listeners, connections, the login flow, combat, ticks
internal/session/   per-connection state and the commands themselves
internal/telnet/    telnet negotiation, CHARSET, GMCP
internal/auth/      password verification; auth/descrypt is the DES port
internal/rng/       the two generators behind --rng
internal/obs/       metrics, health and readiness
internal/buildinfo/ version stamping
data/               runtime data: world, text, and (never committed) players
reference/          the C server and other lineage codebases, for comparison
docs/proposals/go-port-plan.md   the design and the phase order
```
