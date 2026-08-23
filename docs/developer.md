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
| `LIB` | `examples/stock/binary` | The data directory the server runs against. |
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
| `make run` | The full world in `examples/stock/binary/`, plaintext telnet. |
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
(level 34, all skills, `LIB/pfiles` empty). This is stock CircleMUD
behaviour — db.c's *"if this is our first player --- he be God"*, kept
deliberately in `internal/game/create.go` — and it is the fastest way to get
a god to test with.

You only get one per roster, though, so:

```sh
make run-fresh
```

builds `out/scratch-lib`, a copy of `LIB` (`examples/stock/binary/` by
default) with the player state stripped out, and runs against that. Log
in, pick any name, and you are an Implementor. `make clean` or `make
scratch` throws it away and you can do it again.

The copy deliberately excludes `pfiles/`, `plrobjs/`, `plralias/`, `house/`,
`etc/players` and `etc/plrmail`: partly because an empty roster is the whole
point, and partly because `LIB/pfiles` may hold the real ex-players'
password hashes and mail if `LIB` has been pointed at a converted archive,
which should not be getting copied around the filesystem.

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

For the `yaml` format instead — one file per zone and per character,
rather than the original CircleMUD file shapes `convert` above keeps —
point `make lib-import` at the *original* archive directly, not at
`out/converted`:

```sh
make lib-import FROM=/path/to/old/lib TO=out/yaml
make run LIB=out/yaml FLAGS="--world-format=yaml --state-format=yaml --names-format=yaml --messages-format=yaml --socials-format=yaml --help-format=yaml"
```

`docs/operations.md`'s own "Converting into the yaml format" section has
the full walkthrough, including a real, current gap worth knowing about
before trusting the result on an archive with actual accented text in it.

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
| `--no-specials` | Suppress special procedures, so guildmasters, shopkeepers and the rest are ordinary mobiles. |
| `--pulse-interval=1s` | Slows the game loop to human speed; makes pulse-driven behaviour observable. |
| `--log-format=json` | What production logs look like. |
| `--debug-addr=127.0.0.1:6060` | pprof. Never anywhere but loopback. |
| `--allow-legacy-passwords=false` | Reject pre-2008 DES hashes, to check the upgrade path's other branch. |

## What running writes

Everything the server persists goes under its `--lib-dir`. Locally that is
`examples/stock/binary/` by default, and the player-bearing parts of it
are gitignored on purpose: `pfiles/`, `plrobjs/`, `plralias/`, `house/`,
`etc/players`, `etc/plrmail`, under whatever `--lib-dir` is.

Two rules follow, and they are worth keeping in reflex:

- **Never commit player data.** Real hashes, real mail, real connection
  hosts, from real people who played in 2001. `git status` after a session
  should show nothing new under those paths; if it does, something wrote
  somewhere it should not have.
- **Test destructive things against `out/scratch-lib`**, not
  `examples/stock/binary/`. That is what `make run-fresh` is for.

The world files in `examples/stock/binary/world` are tracked, so anything that edits the world
in-game — a builder command, once those exist — shows up as a working-tree
change. That is intended: the world is source.

## Before you push

```sh
make check
```

Format check, `go build`, `go vet`, `golangci-lint` (skipped with a note if
it is not installed), `go test -race`, world lint, and the license check.
Green here does not guarantee CI (`.github/workflows/go.yml`) is green, but
red here guarantees it is not.

`-race` is not optional in the test target. The port keeps the C server's
single game goroutine (`docs/proposals/go-port-plan.md` §3.1), and the whole
safety argument for that design rests on nothing else touching world state —
which only the race detector can actually check.

### What runs when

CI used to run everything on every push and pull request, and got slow and
occasionally flaky doing it — `gcc-multilib` alone "takes longer than
everything else in this job put together, and on a bad day apt stalls
outright." Now it is split by how often each thing actually needs an
answer:

- **`go.yml`, every push and pull request:** `go build`, `go vet`, gofmt,
  `golangci-lint`, `go test -race`. Fast, and every commit gets an answer.
  The 32-bit-only tests (in `internal/persist/player`, `boards`, `mail`,
  `houses`, and the shop-price test in `internal/game`) skip here exactly
  as they would on your own 64-bit machine without `gcc-multilib` — that is
  expected, not a gap.
- **`release.yml`, only when a `v*.*.*` tag is pushed** (or by hand via
  `workflow_dispatch`): everything above, unconditionally installing the
  32-bit toolchain and enforcing that those tests did *not* skip, plus
  world parity (`make parity`), the license check, the two doc-coverage
  checks, a check that `examples/stock/yaml`/`examples/mini/yaml` still
  match a fresh `dlctl lib import` of their binary source, and a container
  build — then creates the GitHub release and pushes the container image
  to `ghcr.io/gerrowadat/disgracelands`. See "Cutting a release" below.

**Session parity** (`scripts/session-parity.sh`) is in neither workflow.
Boots *both* servers on throwaway copies of `examples/stock/binary/` with
the same fixed RNG seed, plays a script from `testdata/parity/` at each,
and diffs what they said. Where they differ, the C server is right. Its
first run found differences that are not all fixed, so it would be a
permanently red job either way — run it by hand after changing anything a
player reads, which is most things. `--ignore-colour` sets aside the one
systematic difference — the C emits ANSI and this port does not — so the
rest is legible.

Adding a script is a file of lines to type in `testdata/parity/`. Two things
to avoid, both of which produce differences that are not differences:

- **Dice.** The seed is fixed on both sides, but the two servers do not
  consume the sequence in the same order, so a script that fights something
  is comparing rolls rather than wording.
- **Rooms that wandering mobiles pass through.** A janitor's position
  depends on how many mobile-activity pulses have elapsed, which depends on
  how fast each server booted. The Midgaard temple and the immortal rooms
  are all on a janitor's route. This is the harness's sharpest limitation
  and the obvious fix is a way to hold mobile activity still on both sides —
  the C has no flag for it, so it would be another `<DoC>` addition.

## Cutting a release

```sh
make release BUMP=patch      # or minor | major | v1.2.3
```

`scripts/release.sh`: checks you are on `main`, clean, and up to date with
`origin/main`; works out the next semver tag from the latest `v*.*.*` tag
reachable from `HEAD` (`v0.0.0` if there is none yet); regenerates
`examples/stock/yaml` and `examples/mini/yaml` from their `binary/` source
via `dlctl lib import` and commits the result if anything had drifted (a
`dataversion` bump, an edited binary source with no matching
regeneration); runs `make check` and, if a C compiler is available,
`make parity`, as a fast local pre-flight; then tags and pushes. The tag
push is what triggers `release.yml`, which is the authoritative gate — the
local pre-flight exists to fail fast, not to replace it.

## Running CI itself, locally

```sh
make ci                    # go.yml, every job, in containers
make ci-job JOB=test       # one job: test | lint
make ci-list               # what `make ci` would run
make ci-clean              # throw away the reused containers
```

This runs `.github/workflows/go.yml` (set `CI_WORKFLOW=.github/workflows/
release.yml` to run that one instead — slower, needs `gcc-multilib` and a C
build inside the container, and close enough to `release.sh`'s own local
pre-flight that it is rarely worth reaching for over just pushing a test
tag) with [`act`](https://github.com/nektos/act), which starts a container
per job and executes the real steps and the real actions. It needs Docker.
If `act` is not on your `PATH` the Makefile fetches the pinned version with
`go run`, the same way `make lint` handles `golangci-lint`.

Defaults live in `.actrc`, which is tracked. The runner image is
`catthehacker/ubuntu:act-latest` — act's own default for `ubuntu-latest` is a
Node slim image with no `sudo` and no `apt`, and `release.yml`'s jobs install
`gcc-multilib`. It is about 1GB and is pulled once.

`--reuse` is on, so job containers survive between runs and keep their Go
module cache, their apt lists and whatever `setup-go` downloaded. That is the
difference between a second run taking seconds and taking minutes.

It is also how a run goes stale, and the way it goes stale is worse than a
cold cache. **act keeps each job's working directory in a named Docker volume
that outlives the container, and populates it with `docker cp` — which
overwrites files but never removes them.** A file you deleted on your branch
stays in that volume and keeps getting compiled. What that looks like is
`undefined: New` in a `_test.go` that is not on disk and not in `git ls-tree`,
which is a genuinely disorienting twenty minutes; it could as easily look like
a *pass*, from a deleted file still satisfying a reference.

So: **if a job fails for a reason you cannot find on disk, run `make ci-clean`
before believing it.** That removes the containers *and* the `act-*` volumes,
which is what makes it work — removing the containers alone does not.

### What this reproduces, and what it does not

Worth knowing before you trust a green run:

- **`actions/checkout` copies your working tree, it does not clone.** act
  `docker cp`s the directory in, uncommitted changes and all. Usually what you
  want locally; it does mean a green `make ci` says nothing about whether you
  remembered to `git add`.
- **Artifact upload does not work**, and does not need to. `release.yml`'s
  container job logs `Unable to get the ACTIONS_RUNTIME_TOKEN`; that is the
  build-summary upload, and the build and the image it produces are
  unaffected.
- **The two publishing jobs never run under act**, deliberately. `publish`
  (the GitHub release) and `image` (the push to ghcr.io) are both `if:
  github.event_name == 'push'`, and act runs jobs as a `workflow_dispatch`.
  So a local `CI_WORKFLOW=.github/workflows/release.yml` run tells you the
  image *builds* — `full-suite` does that for the runner's own architecture,
  and checks the version it reports — and nothing at all about whether the
  push works. A release-candidate tag will not stand in for a real one
  either: the trigger pattern is `v[0-9]+.[0-9]+.[0-9]+`, so `v0.0.0-rc1`
  triggers nothing. A real patch bump is the only test of the push path, and
  it is reversible: delete the release, the tag and the package version.
- **A workflow bug can sit unnoticed for a while either way now.** `go.yml`
  runs on every push and PR, so a break there surfaces immediately; a
  break in something that only lives in `release.yml` surfaces at the next
  release instead. That is the trade this split makes on purpose — see
  `CLAUDE.md`'s own note on it — but it is worth remembering *why* three
  layout checks once sat red on `main` for five merges: a gate that only
  fires sometimes is a gate that can go quietly wrong. Every one of those
  checks greps a package path recursively (`/...`); keep it that way if you
  ever move one of the tests again, and check with `make ci-job
  JOB=full-suite CI_WORKFLOW=.github/workflows/release.yml` rather than by
  reading.

### Secrets

Do not put `GITHUB_TOKEN=""` in `.actrc` to quieten the warnings. act uses
that token to `git clone` the actions themselves, and an empty one is worse
than an absent one: it turns anonymous cloning into a failed password
authentication, and `release.yml`'s `full-suite` job (the one that builds
a container) dies fetching `docker/build-push-action`. Nothing in these
workflows needs API access beyond that. If the clones ever start getting
rate-limited, pass a real token for the run rather than storing one:

```sh
act -W .github/workflows/release.yml -j full-suite -s GITHUB_TOKEN="$(gh auth token)"
```

`.secrets`, `.vars` and `.env.act` — the files act reads secrets from by
default — are in `.gitignore` for the obvious reason.

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

   You do **not** need to write the command's minimum position: all 318 of the
   C's are already in `commandPositions` and `commandTable` fills it in by
   name. Do read it, though — a command's own position check is often testing
   what the interpreter has already refused, and porting that check faithfully
   produces a message no player ever saw. Four of those were found this way;
   see `docs/weirdnumbers.md`.
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
draws, 1,512,000 to-hit values, 36,288 regeneration values, 1,125 saving
throws, 9,680 DES `crypt(3)` pairs, 168 `isname` pairings, and shop prices
against a 32-bit x87 build of the C, because there `int * float` truncated to
`int` gives a different answer than the same line built for SSE.

**An oracle is worth writing for string code too, not only arithmetic.**
`isname` has no numbers in it at all and was still read wrong for four phases:
its loop has the shape of a prefix match and the semantics of a whole-word one.
The rule is really *anything whose behaviour you would have to simulate in your
head to be sure of*.

Where a table is transcribed rather than computed, the test re-parses the C
source and compares entry by entry, so a typo in a table is a failing test
rather than a subtly wrong game. `class.c`, `constants.c`, `interpreter.c`,
`spec_assign.c`, `handler.c`, `spells.h` and `act.wizard.c`'s `set` table are
all read that way.

Where the *format* is a C struct's memory layout — the player database, the
boards, the mail file, the house control file — there is a third kind of tool:
a small C program that prints the offsets the compiler actually chose, which
the Go codec must reproduce field for field under both data models.
`reference/tools/README.md` lists all three kinds.

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
  parser is written against the real files in `examples/stock/binary/world`, not against
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
| `make ci` / `make ci-job JOB=…` | `go.yml`, locally, in containers (`CI_WORKFLOW=...` for `release.yml`). |
| `make ci-list` / `make ci-clean` | What `make ci` would run; discard its reused containers. |
| `make release BUMP=patch\|minor\|major` | Bump the semver tag, regenerate the example worlds if stale, and push — see "Cutting a release". |
| `make clean` | Removes `out/`: binaries, scratch data directory, dev certificate. |

## Where things are

```
cmd/dlmud/          the server binary
cmd/dlctl/          offline tooling: convert, lib import, world/pfile/state/
                    names/messages/socials/helpdb import and fmt, lint/dump,
                    pfile commands, data version
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
examples/           runtime data: stock world, text, and (never committed) players
reference/          the C server and other lineage codebases, for comparison
docs/proposals/go-port-plan.md   the design and the phase order
docs/design/                     design decisions that have actually landed
```
