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

Signals work the same locally as they do in a container, and two of them
are worth knowing while developing:

```sh
kill -HUP  $(pgrep -f 'dlmud .*--config')   # re-read --config, live
kill -QUIT $(pgrep dlmud)                   # dump every goroutine's stack
```

`SIGHUP` is how to try a `config/game.yaml` change without losing the
character you are logged in as. `SIGQUIT` is what to reach for when the
server stops answering: it is not handled, so the Go runtime prints every
goroutine's stack and dies, and the stack sitting in `engine.DoSync` or on
the world goroutine is usually the whole answer. `docs/operations.md` has
the full set and `docs/design/signal-handling.md` the reasoning.

Useful `FLAGS` when reproducing something specific:

| Flag | Why |
|---|---|
| `--restrict` | No new players — the `-r` of old runbooks. |
| `--no-specials` | Suppress special procedures, so guildmasters, shopkeepers and the rest are ordinary mobiles. |
| `--pulse-interval=1s` | Slows the game loop to human speed; makes pulse-driven behaviour observable. |
| `--log-format=json` | What production logs look like: the OpenTelemetry envelope, one record per line. |
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

### Two things the server's tests deliberately run cheap

`internal/server` is the big suite — several hundred tests, each of which
stands a real server up, dials it over a real socket and logs a character in.
Two of the game's own costs are turned down for it, both through options that
production never sets:

- **The password work factor.** `auth.Verifier.Cost` — `testAuth` in
  `server_test.go` — hashes at 8 MiB and one pass instead of the real 64 MiB
  and three. At the real factor a hash is about 140ms, and the suite makes
  several hundred of them; a CPU profile put 94% of its samples inside
  argon2. The hashes are still real argon2id, made and checked by the same
  code the server uses, and the parameters travel in the hash, so verifying
  is unaffected. `auth.DefaultCost` is what every non-test caller gets, and
  `internal/auth` asserts it is still RFC 9106's recommendation.
- **The combat round.** `server.Options.RoundLength` — `testRoundLength` —
  is 100ms instead of two seconds. A wait state is real elapsed time
  (`game.Character.Wait` stores a deadline and the dispatcher sleeps until it
  passes), so at the real length a single `kick` cost its test six seconds of
  doing nothing. `session.DefaultRoundLength` is what everything else gets,
  and `internal/session` asserts it is still PULSE_VIOLENCE.

Together these took the package from about 250 seconds under `-race` to about
35. Both knobs default to the real value when left zero, so the way to get
this wrong is to *set* one, not to forget to — which is the right way round.
A test that asserts on lag should count in `testRoundLength`, not in seconds.

### What runs when

CI used to run everything on every push and pull request, and got slow and
occasionally flaky doing it — `gcc-multilib` alone "takes longer than
everything else in this job put together, and on a bad day apt stalls
outright." Now it is split by how often each thing actually needs an
answer:

- **`go.yml`, every push and pull request:** `go build`, `go vet`, gofmt,
  `golangci-lint`, `go test -race`, and `license-check.sh --notices` (that
  every file written for this project carries its licence notice — the one
  part of the licence check a newly added file can fail, and the only part
  cheap enough to run this often). Fast, and every commit gets an answer.
  The 32-bit-only tests (in `internal/persist/player`, `boards`, `mail`,
  `houses`, and the shop-price test in `internal/game`) skip here exactly
  as they would on your own 64-bit machine without `gcc-multilib` — that is
  expected, not a gap.
- **`release.yml`, only when a release is being cut** (dispatched by
  `scripts/release.sh`, or by a `v*.*.*` tag push, or by hand via
  `workflow_dispatch`): everything above, unconditionally installing the
  32-bit toolchain and enforcing that those tests did *not* skip, plus
  world parity (`make parity`), the license check, the two doc-coverage
  checks, a check that `examples/stock/yaml`/`examples/mini/yaml` still
  match a fresh `dlctl lib import` of their binary source, the play
  regression suite (`make play`, below), a container build, and a
  cross-compile of both binaries for every published platform
  (`make dist`, below) — the licence check here is the full five, not
  just the notices — and only once all of that is green does it tag the
  commit, create the GitHub release, attach the archives to it and push
  the image. See "Cutting a release" below.

### The play regression suite

```sh
make play          # -race, server included; a couple of minutes
make play-fast     # the same thing without the race detector
```

`test/play` builds `cmd/dlmud`, starts it on a throwaway copy of
`examples/mini/` — the tutorial world, one feature per room — and drives
it over a real socket with a client that types what a player types. Every
assertion is a string a player would have seen.

It is deliberately the opposite of `internal/server`'s tests, and the two
are not interchangeable:

|  | `internal/server` | `test/play` |
| --- | --- | --- |
| The world | built in Go by `testWorld()` | read off disk, zones reset into it |
| The server | a `Server` struct wired field by field | the `dlmud` binary, flags and all — built with `-ldflags`, so it is a *released* binary |
| Reaches into the world goroutine | yes, via `inWorld` | no — socket in, socket out |
| Runs | every push | releases only |
| Costs | seconds | minutes |

The `-ldflags` in the second row is not decoration. A binary built without
them has no release version, and the `.dlversion` compatibility check
(`docs/design/data-format-versioning.md`) silently does nothing in a build
like that — so a bare `go build` would have made this suite blind to the
one boot refusal an operator upgrading across a major release actually
meets. `harness_test.go`'s `serverVersion` is the fixed release the child
claims to be, and `dataversion_test.go` writes stamps that differ from it
in one tier at a time.

That first row is the point. Nothing in `internal/server` loads a world
file, runs a zone reset, attaches a special procedure by vnum, resolves a
shop's keeper, reads `text/` off disk, parses a flag, or handles a signal.
A world that no longer boots, a zone that no longer populates, a special
that stopped being assigned, a converted directory that lost a file: all of
those pass every test in `internal/server` and fail the first thing a
player types. The suite found six bugs before it was finished being
written — a shutdown that saved nobody and took thirty seconds to do it, a
`dlctl lib import` that dropped `text/help/screen`, two commands the
tutorial told players to type that the server does not accept,
`perform_dupe_check` never having been ported (so a second login made a
*second body*, and both saved over the same pfile), and `do_visible`
missing its immortal branch.

Some things worth knowing before adding to it:

- **`c.do("look")` is the primitive**, not `send`+`expect`. It types a
  command and returns everything printed before the *next* prompt, so an
  assertion is about the command just typed. `internal/server`'s
  longest-running trap — an `expect` written after a second command
  matching the first command's reply, which has recurred at least nine
  times — cannot happen here.
- **`doUntil(cmd, marker)`** is for the commands that do not come back to a
  prompt: the editor, the pager, the menu.
- **`m.noServerErrors()`** at the end of anything that exercises a feature.
  A command that panics is contained by the world goroutine's `recover` and
  logged at ERROR (`internal/engine/engine.go:227`) — a player sees very
  little and a test asserting on output sees nothing at all. This is the
  cheap net that catches it anyway.
- **`bothFormats`** runs a test on `examples/mini/binary` and
  `examples/mini/yaml`. Use it for anything about the *data*; a test about
  a rule does not need to pay for a second boot.
- **`tourCommands`** in `tour_test.go` is the list of commands
  `examples/mini`'s own room descriptions tell a player to type. Two
  entries in it turned out to be commands the server does not accept. Add a
  room, add its quoted commands there.
- The suite creates an implementor called `Founder` before every test, so
  that the character the test creates is an ordinary mortal — `db.c`'s "if
  this is our first player --- he be God" would otherwise make the first
  one level 34. `m.god()` logs that implementor in when a test needs one.

### Session parity: `test/parity`

```sh
make session-parity        # both servers, every scenario, ~8 minutes
go test -tags=parity -count=1 -run TestSessionParity/shops ./test/parity/
```

`make parity` compares what the two servers **loaded**. This compares what
they **say**: it boots the C server and `dlmud` on their own throwaway
copies of `examples/stock/binary/`, types the same script at each, and
compares the transcripts line for line. Where they differ, the C server is
right.

It is the sibling of `test/play`, and the difference is what each is
evidence of. `test/play` asserts that the server says what *this project
believes* it should say — and the belief is a reading of the C, which is
what has been wrong repeatedly. This suite has no expected output in it at
all. The C server is the expectation.

**In neither workflow, deliberately — not even `release.yml`.** It needs a C
toolchain, starts two servers per scenario, and frames a command's output by
silence, which makes it the one thing in the tree whose timing depends on
how busy the machine is. Run it by hand after changing anything a player
reads. It builds the C server itself if `reference/moderncserver/bin/circle`
is missing or older than the source, and skips rather than fails if there is
no `gcc`.

**What it found on its first green run is in `docs/deviations.md`**, under
"What the session-parity suite found" — twenty differences, from `quit`
returning to the menu in the C to this port never charging movement points
for walking. Eighteen carry a ruling (2026-08-26): sixteen are cutover
blockers, one is for later, one is accepted. The other two need no ruling,
being the 64-bit reference build wrong rather than the port. Green
means *decided*, not *identical*, and the rulings are what make that true.

Four pieces of machinery make the comparison legible, and each replaced
something that had made the harness's findings unreadable before:

- **Both servers hold their mobiles still** — `--freeze-mobiles` here, `-M`
  in the C, a `<DoC>` addition made for this. A wandering mobile's position
  depends on how many pulses have elapsed since boot, so without it the two
  servers disagree about every room a janitor walks through, which is most
  of Midgaard. It also stops `mobile_activity` rolling dice, which is what
  lets a **fight** be compared round by round: with the seed fixed *and* the
  mobiles still, the two generators stay in step, and "a script that fights
  something is comparing rolls rather than wording" — which is what this
  document used to say — is no longer true.
- **Transcripts are compared a command at a time**, not a line at a time
  (`internal/parity/diff.go`). One extra blank line early in the login
  sequence used to report every line after it as differing too: the first
  run of the old harness produced forty findings that were one finding and
  thirty-nine consequences.
- **Blank lines are not compared at all**, because the C prepends a CRLF to
  any output that interrupts a prompt (comm.c:1459) and this port does not.
  That is the one place the suite is knowingly blind; `deviations.md` says
  so.
- **The mud clock is normalised away entirely** — the hour, the weekday and
  the calendar date (`internal/parity/session.go`, `Normalise`). The hour
  was normalised from the start, for the obvious reason: a mud hour is 75
  real seconds (`utils.h:109`) and the two transcripts are taken one server
  after the other, about fifteen seconds apart, so the hour between them
  differs about a fifth of the time. The date was **not**, and that is worth
  knowing about because of how it surfaced: a mud day is 1800 real seconds,
  which puts a rollover between the two transcripts roughly one run in a
  hundred and twenty, and it duly landed on one a day after the suite first
  went green — reporting the port's calendar as wrong when nothing was. Both
  servers compute the date from the same epoch with the same formula; one of
  them was simply asked later. Pinning the epoch would not have helped, since
  the divergence comes from *when each server is sampled* rather than from
  where its clock started. The cost is that this suite does not compare the
  mud calendar at all, which is the right trade anyway: `mud_time_passed` is
  pure arithmetic over elapsed seconds, and CLAUDE.md says what checks that —
  an oracle, not two transcripts fifteen seconds apart.

**Adding a scenario** is a script in `testdata/parity/` and a row in
`scenarios` (`test/parity/session_test.go`). A script is the lines a player
types, `#` for a comment, and one directive:

- `!reconnect` hangs up and dials again. Each scenario gets its own freshly
  booted pair of servers, so the first character it creates is an
  implementor (`db.c`'s "if this is our first player --- he be God"); a
  scenario that needs a mortal too makes the god first, `quit`s, and
  reconnects. `mail.session` has four connections and two characters in it.
  Type `quit` before it: dropping a socket with a character still in the
  world is its own comparison, not an accident every scenario should be
  making.

Two knobs on the row: `quiet`, how long a server must be silent before its
answer is taken to be complete (a fight is not finished answering for
several two-second rounds, so `combat` waits three seconds), and `known`,
the triage list.

**`known` is the part worth understanding.** It is a list of *decisions*: a
pattern for lines the two servers are allowed to disagree about, and why —
matched against the differing line rather than against the command, because
one difference is rarely one command (a mortal's hit points differ, and so
does the prompt after every command in the script). A differing line no
entry matches fails the suite, and **an entry that matches nothing also
fails**, with "delete it": that is what makes the list shrink as things get
fixed instead of turning into a record of what used to be wrong. Every entry
points at `docs/deviations.md` for the finding itself.

### The release archives

```sh
make dist          # linux/amd64, linux/arm64, windows/amd64 into out/dist
```

`scripts/build-dist.sh` builds `dlmud` and `dlctl` for each published
platform, packages each pair with `LICENSE`, `README.md` and
`BUILDING.md`, and writes a `SHA256SUMS`. `release.yml` runs the same
script and attaches its output to the GitHub release; `BUILDING.md` has
the platform table and what is deliberately not on it.

Two things about it are worth knowing before changing it:

- **It is the cross-platform build check as well as the packaging step**,
  which is why it sits near the top of `full-suite` rather than next to
  the upload at the bottom. A platform that has stopped compiling should
  fail the release in the first minute, not after twenty. It is the
  *only* thing in the tree that builds for anything but the host —
  `go test ./...` cannot notice a Windows build break, and neither can
  `make check`.
- **The archives are reproducible given the same Go toolchain, and that
  is load-bearing for `SHA256SUMS`.** Six things had to be pinned, and
  every one of them was a real difference between two runs before it was
  fixed:

  | Pinned | Because |
  | --- | --- |
  | `-trimpath` | absolute paths of the build directory |
  | The build date | it is the *commit's*, not the wall clock — a binary stamped with the minute the runner started undoes the rest from the inside |
  | `-buildvcs=false` | Go stamps `vcs.revision`/`vcs.time`/`vcs.modified` from the working tree. v0.1.1 shipped binaries reporting `f161334-dirty` from a clean tagged commit because of it — see below |
  | mtimes, at 1980-01-01 | not the epoch: a zip's DOS timestamp cannot represent anything earlier and clamps silently, so 1970 made the tar and the zip disagree about the same file |
  | Permissions | `go build` and `cp` both mask against the builder's umask |
  | Member order, and `TZ=UTC` | `zip -r` walks in readdir order, which is the filesystem's to choose; a zip records local time with no zone alongside it |

  **Two things dirtied that stamp, and they are worth knowing separately.**
  On the runner, the C tree's `./configure` substitutes the `CFLAGS` it is
  given into `reference/moderncserver/src/Makefile` and
  `src/util/Makefile` — *tracked* files, checked in carrying different
  values — so every step after it sees a dirty tree. The archives are now
  built before it, from a pristine checkout, which is the fix that does
  not rely on remembering anything.

  Locally it is worse and quieter: **Go's VCS stamping does not follow
  linked git worktrees.** Building from one of `.claude/worktrees/*`
  stamps the *primary* checkout's `HEAD` and *its* dirtiness, whatever
  this worktree is on — a clean tree at `f2053fc` producing a binary that
  says `c1b87f0`, `modified=true`. Since this repo is routinely developed
  from worktrees, that alone would mean nobody could reproduce a release
  archive from a worktree at all. `-buildvcs=false` is what makes the
  stamp irrelevant in both cases; the reordering is what makes the
  release right even without it.

  **The Go toolchain version is the one input still left to the caller.**
  `release.yml` uses `setup-go` with `check-latest: true`, so a release
  is built by whatever 1.25.x was newest that day, and a different patch
  release produces different code. The binary names its own, which is
  what makes this workable rather than merely honest — `dlctl version`
  ends in `go1.25.14`, and `GOTOOLCHAIN=go1.25.14 VERSION=v1.2.3
  COMMIT=<sha> ./scripts/build-dist.sh` reproduces that release's
  archives exactly. Pinning it in `go.mod` instead would make the claim
  unconditional, at the cost of a toolchain bump becoming a commit.

## Cutting a release

```sh
make release BUMP=patch      # or minor | major | v1.2.3
```

`scripts/release.sh`: checks you are on `main`, clean, and up to date with
`origin/main`, and that `gh` is present and logged in; works out the next
semver version from the latest `v*.*.*` tag reachable from `HEAD`
(`v0.0.0` if there is none yet); regenerates `examples/stock/yaml` and
`examples/mini/yaml` from their `binary/` source via `dlctl lib import`
and commits the result if anything had drifted (a `dataversion` bump, an
edited binary source with no matching regeneration); runs `make check`,
`make play` and, if a C compiler is available, `make parity`, as a local
pre-flight; then pushes `main`, dispatches `release.yml` with that
version, and watches the run. `release.yml` is the authoritative gate —
the local pre-flight exists to fail sooner, not to replace it.

**The script does not tag anything.** `release.yml`'s `publish` job
creates the tag, after `full-suite` has gone green, then creates the
GitHub release with the archives `full-suite` built attached to it — the
same bytes the suite checked, handed over as a workflow artifact rather
than rebuilt, so what is published is what was tested. Both `publish` and
`image` are `needs:`-gated on `full-suite`. So a failed release leaves *nothing*
behind: no tag, no GitHub release, no generated notes, no package, and the
version number still free — fix the problem, merge the fix, and run `make
release BUMP=v1.2.3` again for the same version. It used to work the other
way round, tagging and pushing first and letting the tag push trigger the
workflow, which meant every failed release run left a real `v1.2.3` tag on
a commit that had just been proved not to release. A tag is the one part of
a release that other people fetch, and deleting a pushed one does not
reliably un-fetch it.

Two consequences of dispatching rather than tag-pushing, both handled in
the workflow and both easy to reintroduce:

- **`github.ref` is `refs/heads/main`, not a tag.** `docker/metadata-
  action`'s `type=semver` patterns read the version out of `github.ref`
  by default and match *nothing* on a branch — which is not a build
  failure, it is an empty tag list and a push of nothing. Both patterns in
  the `image` job pass `value=` explicitly instead.
- **Nothing pins which commit the runner checks out.** `--ref main` means
  main's tip *when the run starts*, so a merge landing in between would be
  released instead. `release.sh` passes the commit it checked, and
  `full-suite`'s first step refuses to go on if the checkout is anything
  else.

The `v*.*.*` tag-push trigger still exists, for re-verifying and releasing
a commit that is already tagged. On that path the tag necessarily comes
first; `publish` checks it points at the commit that was tested and skips
creating it. A `workflow_dispatch` without `publish` set runs the suite and
stops, which is how to exercise the workflow without cutting a release.

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

#### Worktrees, and the two ways act runs somebody else's code

This repo is routinely developed from several git worktrees at once, and
`act` has no idea they are different. Both halves of that are handled in the
Makefile now; both are worth knowing about, because the symptom is a CI
result that has nothing to do with your tree.

**The container is named after the workflow and the job, and nothing else.**
`createContainerName("act", "<workflow>/<job>")`, hashed — the working
directory is not in it (`pkg/runner/run_context.go:92`). With `--reuse`, the
container outlives the run that made it and keeps the workspace mount it was
*created* with, so every worktree on the machine shares one container per job
and all but the first get a run whose workspace belongs to somebody else.
act still copies the current tree in on top, so several checkouts of this
repo end up side by side in one container. `scripts/act-guard.sh` runs before
every act invocation and drops any act container whose workspace is a
different checkout of *this* repo, leaving unrelated projects' containers
alone. Switching worktrees therefore costs one cold run rather than a wrong
one. `flock` on the shared `.git` serialises runs, since two checkouts
sharing one container name cannot safely run at once whatever directory each
thinks it is in.

**golangci-lint's cache is keyed by package content, and records absolute
paths.** Two checkouts of the same repo at the same commit produce the same
keys, so one legitimately hits the other's entries — and replays them with
the *other* checkout's filenames. What that looks like is a lint failure in
`../<some-other-worktree>/internal/game/apply.go`, in a file the container
does not have and you cannot fix from where you are, sitting underneath
`failed to parse file: no such file or directory` warnings. `make ci` gives
each checkout its own cache store (`--cache-server-path`, under the shared
`.git`), and `make ci-clean` removes this checkout's.

That second one is the one that will waste your afternoon, because it
survives `make ci-clean` as it was before this — the containers are gone, the
volumes are gone, and the stale findings come back anyway from a cache
`actions/cache` restores into the fresh container. It reads exactly like the
volume-staleness trap above and is not it.

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
  (the tag and the GitHub release) and `image` (the push to ghcr.io) both
  require either a tag push or a `workflow_dispatch` with `publish` set,
  and act runs jobs as a plain `workflow_dispatch` with no inputs. So a
  local `CI_WORKFLOW=.github/workflows/release.yml` run tells you the
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

This still applies in full to anything gameplay- or compatibility-shaped.
It has nothing to say about work that's purely architecture or tooling and
doesn't change what the C computed or stored — see `CLAUDE.md` and the
plan's §0, "Fidelity, phase two", for what that distinction means and why
it changed on 2026-08-23.

The rule that has held, for the code this applies to: **anything with a
division, a cast, or a comment describing numbers gets an oracle rather
than a reading.**

`reference/tools/*.c` holds original C function bodies with the `char_data`
dereferences substituted and nothing else changed. The Go tests compile them
and compare across the whole input space where that is affordable — 30,000 RNG
draws, 1,512,000 to-hit values, 36,288 regeneration values, 1,125 saving
throws, 9,680 DES `crypt(3)` pairs, 168 `isname` pairings, 805 line-editor
commands, and shop prices against a 32-bit x87 build of the C, because there
`int * float` truncated to `int` gives a different answer than the same line
built for SSE.

**An oracle is worth writing for string code too, not only arithmetic.**
`isname` has no numbers in it at all and was still read wrong for four phases:
its loop has the shape of a prefix match and the semantics of a whole-word one.
The improved line editor's eleven commands are the same lesson at length:
`editoracle.c` turned up seven separate things a careful reading had got
wrong, including a three-line buffer having a fourth line and a `/ra` that
runs out of room silently truncating the player's text
(`docs/weirdnumbers.md`, "The line editor"). The rule is really *anything
whose behaviour you would have to simulate in your head to be sure of*.

**Watch what optimisation level you build an oracle at.** These are twenty-
year-old C, and some of it is undefined behaviour that a compiler of the
period resolved one way and a modern one resolves another. `editoracle.c` is
built `-O0` because `PARSE_LIST_NUM` accumulates with `sprintf(buf,
"%s%4d:\r\n", buf, i - 1)` — destination and `%s` argument the same buffer —
and gcc at `-O2` turns that into something that keeps only the last line,
where `-O0` calls glibc and the self-copy at offset zero is a no-op. `-O2`'s
answer would have made `/n` a broken command in this port for no reason but
the compiler being new. An oracle's job is to say what the archived server
did; where the C is undefined, that has to be decided deliberately rather
than inherited from whatever gcc is installed.

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
  one, including for files added but not yet committed, and `go.yml` runs
  that check (`--notices`) on every push and PR — a new file without a
  header turns the branch red rather than waiting for the next release.
  Copy the header from a neighbouring file when you add one. The `Makefile` carries one too,
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
| `make release BUMP=patch\|minor\|major` | Regenerate the example worlds if stale, check locally, then dispatch `release.yml` and wait — see "Cutting a release". |
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
