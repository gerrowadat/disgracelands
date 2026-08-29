# Running and administering the server

How to run `dlmud`, what it exposes while running, and what to watch.

For the full settings list see `docs/configuration.md`. For building from
source see `BUILDING.md`.

> **Current state:** Phase 5 (the rest of the game) is done. The world
> resets, mobiles act, characters fight, cast, level and die, guildmasters
> teach, shopkeepers trade, and the boards, mail, houses, rent and the
> immortal commands all work. Phase 6 (OasisOLC) was decided against, in
> favour of editing the world files in your `--lib-dir` directly and
> reloading them live (`reloadmob`/`reloadzone`/`reloadobj`/`reloadshop`);
> Phase 7 (cutover)
> has not started. What is left of Phase 5 itself is a handful of small,
> named commands, listed one by one in `docs/proposals/go-port-plan.md`
> §10. Everything below about process management, health checking,
> logging and player data is real and works.

## Getting a binary

Every release attaches a `dlmud`/`dlctl` pair for `linux/amd64`,
`linux/arm64` and `windows/amd64`, plus a `SHA256SUMS`:

```sh
tar xzf disgracelands-v1.2.3-linux-amd64.tar.gz
sha256sum -c SHA256SUMS          # from the directory you downloaded into
```

The two Linux ones are also the container image
(`ghcr.io/gerrowadat/disgracelands`, see "Containers" below); Windows has
no image, which is most of why the archive exists. To build instead, or
for a platform not on that list, see `BUILDING.md`.

## Starting it

The shortest thing that actually starts, for local use:

```sh
dlmud --listen-telnets= --listen-telnet=:4000 --metrics-addr=:9090
```

That disables the default TLS listener (which would otherwise demand a
certificate) and enables plaintext telnet instead. **Do not do this on
anything reachable by strangers** — the server will warn you about it in
the log, once per start.

For anything real:

```sh
dlmud \
  --lib-dir=/srv/disgracelands/lib \
  --listen-telnets=:4443 \
  --tls-cert=/etc/dl/cert.pem --tls-key=/etc/dl/key.pem \
  --metrics-addr=127.0.0.1:9090 \
  --log-format=json
```

Bind `--metrics-addr` to loopback or a management interface. It is not
authenticated.

To let people play from a browser as well, add `--listen-ws` — it shares
the same `--tls-cert`/`--tls-key` if set, so it is HTTPS/`wss://` for free
on a server already running the TLS telnet listener:

```sh
dlmud \
  --lib-dir=/srv/disgracelands/lib \
  --listen-telnets=:4443 \
  --listen-ws=:8080 \
  --tls-cert=/etc/dl/cert.pem --tls-key=/etc/dl/key.pem \
  --web-password=hunter2 --web-captcha \
  --metrics-addr=127.0.0.1:9090
```

`--web-password` and `--web-captcha` are both optional; see
`docs/configuration.md`'s own section on the web interface for what each
actually defends against, which is more modest than either name suggests.

### If it exits immediately

That is usually deliberate. The server validates its configuration before
doing anything and refuses to start on a configuration that cannot work,
rather than starting something unreachable. The message names the flag.

The most common one is running `dlmud` with no arguments at all: the TLS
listener is on by default and has no certificate, so you get

```
dlmud: --listen-telnets needs a certificate: set --tls-cert and --tls-key, or --tls-acme-domain
```

## Containers

Every release publishes an image to GitHub's container registry, built for
`linux/amd64` and `linux/arm64`:

```sh
docker pull ghcr.io/gerrowadat/disgracelands:latest
docker run -v /srv/disgracelands/lib:/lib -p 4443:4443 \
  ghcr.io/gerrowadat/disgracelands:latest \
  --lib-dir=/lib --tls-cert=/lib/cert.pem --tls-key=/lib/key.pem
```

The tags are `X.Y.Z`, `X.Y` and `latest`. There is deliberately no `X`
tag: while this is 0.x, `0` would promise a compatibility guarantee it
cannot keep.

**A package takes the visibility of the repository it was first published
from, and keeps it.** The first images here were published while this
repository was still private, so the package may still be private even
though the repository is not: visibility is independent after that first
publish, and making the repository public does not carry the package with
it. If `docker pull` returns "denied", either make the package public from
its own page under the account's *Packages* tab, or log in with a token
that has `read:packages`:

```sh
echo "$GITHUB_TOKEN" | docker login ghcr.io -u <your-username> --password-stdin
```

Or build it yourself. A plain `docker build` produces an image for the
architecture you are on:

```sh
docker build -f build/Dockerfile -t disgracelands .
docker run -v /srv/disgracelands/lib:/lib -p 4443:4443 disgracelands \
  --lib-dir=/lib --tls-cert=/lib/cert.pem --tls-key=/lib/key.pem
```

Or for local development, `build/docker-compose.yml`:

```sh
docker compose -f build/docker-compose.yml up --build
```

Notes that matter:

- The image is **distroless/static and has no shell**. `docker exec ... sh`
  will not work. That is the point: the C server's approach to staying up
  was the `autorun` script looping in a shell, and this replaces it with
  the container runtime's restart policy plus real signal handling.
- **The image declares `/data` as a volume** and its default command is
  `--lib-dir=/data`. It ships no world of its own, so that volume — or
  whatever you point `--lib-dir` at instead, as the examples above do with
  `/lib` — is where a lib-dir has to be mounted. It is mutable state:
  players, houses, boards, mail, and any world file edited in-game. An
  image rebuild must not lose it.
- The container runs as **non-root**. The lib-dir must be writable by uid
  65532 (`nonroot`) — the server writes `pfiles/`, `plrobjs/`, `house/` and
  `etc/players` into it, and character creation is the first thing that
  fails if it cannot. `build/docker-compose.yml` overrides the user instead,
  because it bind-mounts a directory from your checkout that uid 65532 does
  not own.
- Health checks cannot use `curl`, for the same no-shell reason. Point an
  external check at `/readyz` on `--metrics-addr`, or use the `dlctl`
  binary in the image.

## Signals

Everything an operator does to a running server that is not typed in the
game is a signal. `docs/design/signal-handling.md` is the reasoning; this is
the use.

| Signal | What it does | When you send it |
|---|---|---|
| `SIGTERM` | Graceful shutdown: stop accepting, tell everyone connected, save, exit | Stopping the server. What `docker stop`, `systemctl stop` and a pod delete send for you |
| `SIGINT` | The same | Ctrl-C at a terminal. A *second* one during shutdown kills the process instead of being swallowed, so a shutdown that will not finish can still be interrupted |
| `SIGHUP` | Re-reads the game tuning (`<lib-dir>/config/game.yaml`, or `--config`) and applies it live | After editing that file. No restart, nobody disconnected |
| `SIGQUIT` | Not handled, on purpose: the Go runtime dumps every goroutine's stack and the process dies | The server has stopped responding. The stacks name the goroutine that is stuck, and they are what to attach to the bug report |

Anything else keeps its default disposition. In particular there is no
signal that reloads world data, and that is a deliberate line rather than a
gap — see "Reloading without a restart" below.

### Sending them

The image is **distroless and has no shell**, so `docker exec ... kill` and
`kubectl exec ... kill` do not work. Use the runtime's own mechanism:

| Running under | Reload (`SIGHUP`) | Stop (`SIGTERM`) | Stack dump (`SIGQUIT`) |
|---|---|---|---|
| Docker | `docker kill --signal=HUP <container>` | `docker stop <container>` | `docker kill --signal=QUIT <container>` |
| Compose | `docker compose kill -s HUP dlmud` | `docker compose stop` | `docker compose kill -s QUIT dlmud` |
| systemd | `systemctl reload dlmud` (with `ExecReload=/bin/kill -HUP $MAINPID`) | `systemctl stop dlmud` | `systemctl kill -s QUIT dlmud` |
| Kubernetes | no built-in path; `kubectl delete pod` restarts instead | `kubectl delete pod` | `kubectl delete pod` loses the stacks — prefer reproducing it somewhere you can signal |
| A bare process | `kill -HUP <pid>` | `kill <pid>` | `kill -QUIT <pid>` |

The stack dump goes to stderr, which means it lands wherever the rest of the
log does (`--log-file`, or the container's log). It is **fatal** — the
process is gone afterwards and whatever was in the world goroutine's queue
is lost, which is the right trade for a server that had already stopped
turning, and the wrong one for a server that is merely slow.

### Reloading without a restart

Three different things can be reloaded, by three different mechanisms, and
which one you get depends on what is being reloaded rather than on
preference.

| What | How | Notes |
|---|---|---|
| Game tuning (`<lib-dir>/config/game.yaml`) | `SIGHUP` | Takes effect on the next thing that reads it. See `docs/configuration.md` |
| The canned text: `greetings`, `motd`, `imotd`, `credits`, `news`, `wizlist`, `immlist`, `info`, `policy`, `handbook`, `background`, `help` | `reload <name>` in-game (implementor) | `reload all` does all twelve. It does **not** include the help database — that is `reload xhelp`, separately, exactly as in the C |
| World data: rooms, mobiles, objects, zones, shops | `reloadmob`, `reloadzone`, `reloadobj`, `reloadshop` in-game (greater god) | By vnum, after editing the files on disk |

**Why world data has no signal.** Reloading a mobile prototype is surgery on
the copies already walking around, and it can *refuse* — a mobile in combat
is not replaced underneath the fight. That needs a vnum to act on and
somebody to give the answer to, and a signal has neither.

Everything else — the flags, the listeners, `--lib-dir`, the data formats —
needs a restart.

**A reload that fails changes nothing.** A tuning file that will not
parse, or parses and will not validate (`autosave_time: 0`, a negative
cost), is logged as an error and ignored: the server keeps the values it
already had, and the players stay connected. A typo in a file you are
editing on a live server costs you the reload and nothing else. A `SIGHUP`
at a data directory with no `config/game.yaml` in it at all logs that there
was nothing to re-read, and likewise changes nothing.

## Shutdown

`SIGTERM` and `SIGINT` trigger a graceful shutdown: stop accepting, tell
everyone still connected, save the world, drain the writes already in
flight, exit.

**Give it time to finish.** The C server autosaved every 60 seconds, so an
ungraceful kill could lose up to a minute of play; handling `SIGTERM`
properly is the entire reason not to. The shutdown budget is 30 seconds.
Configure your supervisor to allow at least that:

- Docker: `--stop-timeout 45` or `stop_grace_period: 45s` in compose.
- systemd: `TimeoutStopSec=45`.
- Kubernetes: `terminationGracePeriodSeconds: 45`.

### Exit codes

| Code | Meaning |
|---|---|
| `0` | A clean stop: a signal, or `shutdown` / `shutdown die` / `shutdown pause` typed in the game |
| `1` | Boot failure, or a fatal error while running. The reason is on stderr, prefixed `dlmud:` |
| `2` | `shutdown reboot` or `shutdown now`: it stopped cleanly and is asking to be started again |

The 0/2 split is how an implementor inside the game reaches the thing that
restarts the server. The C did this by touching `.killscript` or
`.fastboot` for the `autorun` shell script to find afterwards; there is no
wrapper script here, so the exit code carries it and the restart policy
reads it.

**Which means the restart policy decides whether `shutdown die` is
obeyed.** Under `restart: on-failure`, `shutdown reboot` comes back by
itself and `shutdown die` stays down, which is the behaviour the two
commands are named for. Under `restart: always` or `unless-stopped` — and
under Kubernetes, where a `Deployment` restarts a pod whatever it exited
with — both come back and `die` is only a slow `reboot`. Pick `on-failure`
if the distinction matters to you; `build/docker-compose.yml` ships
`unless-stopped`, because a development server should come back from
anything.

## Health and readiness

Served on `--metrics-addr`.

| Endpoint | Meaning |
|---|---|
| `/healthz` | The process is alive. Always `200` if anything answers at all. |
| `/readyz` | The server is ready for players: `200`, or `503` with `not ready`. |

These are deliberately different. A server that is booting — loading the
world, scanning rent files — is *alive* but not *ready*, and so is one that
has begun shutting down. Restarting a booting server because it is not
ready yet is how you get a crash loop.

- **Liveness probe** → `/healthz`. Restart only if this fails.
- **Readiness probe** → `/readyz`. Take out of rotation, do not restart.

## Metrics

Prometheus format on `--metrics-addr/metrics`. Standard Go runtime and
process collectors, plus:

| Metric | Type | Meaning |
|---|---|---|
| `dlmud_build_info` | gauge | Always 1; version, commit and Go version as labels. |
| `dlmud_pulse_duration_seconds` | histogram | How long each game loop pulse took. |

**`dlmud_pulse_duration_seconds` is the metric to alert on.** A MUD's game
loop has a fixed budget — `--pulse-interval`, 100ms by default — and once
pulses routinely exceed it the world starts lagging behind real time for
every player at once. The histogram's buckets are derived from the
configured interval rather than fixed wall-clock values, so the bucket
boundary at the budget is meaningful whatever you set it to.

A reasonable first alert is "more than 1% of pulses exceeded the budget
over five minutes". Everything else — player counts, command rates, save
latency — arrives with the subsystems that produce it.

## Logs

Structured, via `log/slog`. `--log-format=json` for anything that ships
logs somewhere; `text` is friendlier at a terminal.

`--log-file` writes to a file, created mode 0600 because the log records
connecting hosts and player names. `-` (the default) writes to stderr,
which is usually what you want under a container runtime or systemd — let
the supervisor handle rotation and shipping.

### The JSON format

One OpenTelemetry [log record][otel-logs] per line, not slog's own
envelope:

```json
{"_time":"2026-08-24T09:14:02.417365991Z","severity_text":"INFO","severity_number":9,"_msg":"entered the world","resource":{"host.name":"mud1","process.pid":"7","service.name":"dlmud","service.version":"v0.1.0"},"attributes":{"character":"Zod","room":3001}}
```

| Field | What it is |
|---|---|
| `_time` | The record's timestamp, RFC 3339 in UTC with a fixed nine fractional digits, so the raw text sorts in time order. |
| `_msg` | The data model's Body. |
| `severity_text` / `severity_number` | `INFO` and 9, `WARN` and 13, and so on — the data model's [severity ranges][otel-severity]. |
| `resource` | What emitted the record: `service.name`, `service.version`, `host.name`, `process.pid`, plus anything from `OTEL_RESOURCE_ATTRIBUTES`. |
| `attributes` | Everything the call site logged. Grouped, so a command logging something called `severity_text` cannot collide with the record's own fields. |
| `code` | Caller file, line and function. Only at `--log-level=debug`. |

**`_time` and `_msg` are VictoriaLogs' names, not OpenTelemetry's** (which
calls them Timestamp and Body). They are the two fields VictoriaLogs will
not guess at — its `_time_field` and `_msg_field`, defaulting to exactly
these — so a line ingests with no per-source configuration at all:

```
dlmud --log-format=json --log-file=- 2>&1 | \
  curl -s -T - http://victorialogs:9428/insert/jsonline?_stream_fields=resource.service.name
```

In practice something else does the tailing — vector, vmagent, promtail,
the Docker or journald driver — and all any of them need is the same URL.
VictoriaLogs flattens nested JSON on ingestion, so the two grouped blocks
arrive as ordinary fields: `resource.service.name`, `attributes.character`.
A backend that wants OpenTelemetry's own names instead is one rename rule
away, and everything other than those two fields already carries a
[semantic convention][otel-semconv] name.

`service.name` defaults to `dlmud`. Two servers sharing a log backend
should differ, and the standard environment variables are how — there are
deliberately no flags for this, so the labels come from wherever the rest
of the deployment's labels do:

```
OTEL_SERVICE_NAME=dlmud-staging
OTEL_RESOURCE_ATTRIBUTES=deployment.environment=staging,service.namespace=mud
```

There is no trace correlation on the records, because nothing in the server
opens a span yet. When something does, `trace_id`/`span_id` belong on the
record alongside `severity_text`.

[otel-logs]: https://opentelemetry.io/docs/specs/otel/logs/data-model/
[otel-severity]: https://opentelemetry.io/docs/specs/otel/logs/data-model/#field-severitynumber
[otel-semconv]: https://opentelemetry.io/docs/specs/semconv/

Startup warnings are worth reading rather than filtering. The server warns
about exactly the things that are safe-but-questionable: plaintext telnet
enabled, legacy DES password verification enabled, pprof listening, the
web interface (`--listen-ws`) with neither TLS nor a trusted proxy in
front, and the web interface enabled with no `--web-password`.

### Watching the game from in-game

The C server's `mudlog()` did two things: wrote to the log, and echoed the
message to online immortals above a given level whose own `syslog`
verbosity (`PRF_LOG1`/`PRF_LOG2`, set by the in-game `syslog` command)
was high enough. Both jobs are real: log records carry a `wizvis`
attribute holding the minimum level and type a message needs, and
`obs.WithWizVisEcho` calls back into `Server.echoWizVis` for any record
that has one — applying the C's exact selection (online, playing, not
switched into an NPC, not mid-edit, syslog verbosity high enough) and
sending it in green to whoever qualifies.

**`bug`/`idea`/`typo` are the one real producer so far.** The mechanism
is generic — anything that logs through `internal/obs` with a `wizvis`
attribute reaches an online god exactly the way it would in the C — but
auditing every other command that logs something the C would have run
through `mudlog()` and wiring it up is its own pass, not yet done.
Watching the game today mostly still means watching the log the server
writes; only reports reach an online god's screen live. See
`docs/deviations.md` for the exact mechanism and the honest count of how
many call sites are still would-be producers.

## Backups

Back up your `--lib-dir` (`examples/stock/binary/` if you are running on
the shipped default). That is all the state there is.

Of particular note, and none of it in git for good reason:

- `etc/players` (or `pfiles/`) — the roster, including password hashes.
- `plrobjs/`, `plralias/` — player inventories and aliases.
- `house/`, `etc/hcontrol` — player housing and its contents.
- `etc/plrmail` — in-game mail.
- `world/` — the world itself, which changes if anyone builds in-game.
- `config/game.yaml` — the game tuning, if this server has been tuned. It
  lives in the data directory precisely so it lands in this backup with the
  world it configures (`docs/configuration.md`).

The repo deliberately ships no player data; see the "Player data" section
of the top-level `README.md`.

## Offline tooling

`dlctl` handles the jobs that do not need a running server — the work
`reference/moderncserver/src/util/` and `reference/tools/` do for the C
tree. `dlctl` with no arguments lists what it can do; anything added ahead
of the layer it needs says which phase implements it rather than failing
obscurely.

### Checking the world files

```sh
dlctl lint --type=world --dir=/srv/data --format=yaml
```

Replaces `reference/moderncserver/src/util/scheck` and the C server's `-c`
mode, and unlike either it runs without starting a server, so it belongs in
CI.

Findings come in three severities:

| Severity | Meaning |
|---|---|
| `error` | The C server refuses to boot on this. Fix it first. |
| `warn` | The world is playable but something in it does not work — an exit to a room that was deleted, a reset command referring to a missing mob. |
| `info` | The loader changed the data in a way the file does not state. Not a defect. |

`--quiet` hides the notes; `--strict` exits non-zero on warnings as well as
errors. Exit status is non-zero if anything at the failing severity was
found, so it works directly as a CI step.

`lint --type=world` also reports world text that is not valid UTF-8,
which is how you find out a directory has not been through `dlctl import`
with the right `--encoding`. The server works in UTF-8; see "Converting an
old data directory" below.

The shipped world (`examples/stock/yaml`, the default `--lib-dir`) lints
clean. The *unconverted* source beside it,
`examples/stock/binary`, reports 0 errors, 11 warnings and 12 notes, and
the warnings are worth knowing about because they are facts about stock
CircleMUD rather than about the conversion:

- Four complete zones (23, 90, 92, 147) and two further `.zon` files exist
  in `examples/stock/binary/world/` but appear in no index, so nothing ever loads them. This
  is silent in the C server: a builder who adds a zone and forgets the index
  gets no error, just a world quietly missing their work.
- Two rooms have exits locked by key objects that do not exist.
- Two shops sell things that do not exist — shop #5484 lists ten of them —
  and two operate in rooms that do not exist. The C loader drops the missing
  products silently, so those shopkeepers have simply had nothing to sell
  for years.

The notes are all the drink-container weight adjustment, which is normal
CircleMUD behaviour rather than a problem — the loader raises a container's
weight when it is lighter than the liquid it holds.

### Converting an old data directory

**This is the only path from a CircleMUD `lib/` to a directory this
server runs on.** There is no in-place compatibility, no fallback flag
and no auto-conversion: `dlmud` reads one on-disk format, and pointing
`--lib-dir` at an archive is refused at boot with this command in the
message.

```sh
dlctl import --from-dir=/path/to/old/lib --to-dir=/srv/data
```

The source is never written to, and the destination is somewhere you
chose. One pass converts the lot:

- the **world** — one file per zone, CP1252 decoded to UTF-8, escape
  codes demoted to named colour markup;
- the **roster**, including every character's rent and crash file and
  their aliases, folded into one file per character;
- the **state** — bans, boards, mail, houses, the bug/idea/typo reports
  and the mud clock — into one `state/` directory;
- the **disallowed names**, the **damage messages** and the **socials**
  into `config/`, and the **help database** into `text/help/`;
- `text/`'s plain prose and `config/game.yaml` copied across unchanged, so
  a tuned directory keeps its tuning;
- and, once everything else has succeeded, a `.dlversion` stamp naming
  the release that wrote it (`docs/design/data-format-versioning.md`).
  Use a *released* `dlctl` for a directory you intend to run: an
  unreleased build has no version to write and stamps nothing.

`--encoding=latin1` if the source really is Latin-1 rather than CP1252 —
they differ only at bytes 0x80–0x9F, which is exactly where the curly
quotes a word processor inserts live, so the default is usually right.

Unlike the directory-level converter this replaced, **the struct-dump
formats are decoded rather than copied**. The boards, the mail, the house
contents, the rent files and the house control file all come across as
records, with the text inside them transcoded — which the old converter
could not do, and said so.

Each subsystem also runs on its own with `--type=world`, `pfile`,
`state`, `names`, `messages`, `socials` or `help`, for an archive laid
out unusually or a conversion done in stages. `--from-house-dir`/
`--from-misc-dir`/`--from-objs-dir`/`--from-alias-dir` override the
subdirectories `dlctl` would otherwise derive.

### Checking a conversion lost nothing

`import` does this itself, by default: once it has written the
destination it loads **both** directories and compares them, and fails
the import — leaving the output unstamped, so a server will not boot on
it — if they do not agree. `--verify=false` skips it. The same comparison
is `dlctl verify --against`, a command in its own right; see "Checking a
conversion lost nothing" under "Inspecting player data" below.

### Pointing dlctl at a directory

Every `dlctl` command above takes a base directory (`--dir`, or
`--from-dir`/`--to-dir` for the two that move data between two of them) —
never a leaf subdirectory. Point it at the archive root (or your `data/`),
and `dlctl` works out where a given `--type` actually lives under it, the
same way `dlmud --lib-dir` does. `dlctl` is the one program that still
knows *both* layouts, and which one it uses is what `--format` (or
`--from-format`) selects:

| `--type` | legacy | converted |
|---|---|---|
| `world` | `world/` | `world/` |
| `pfile` | `etc/` (`binary`) or `pfiles/` (`ascii`) | `players/` |
| `state` | `etc/`, `house/`, `misc/` | `state/` |
| `names`, `messages`, `socials` | `misc/` | `config/` |
| `help` | `text/help/` | `text/help/` |
| `copied` (`verify` only) | `text/`, `config/game.yaml`, `text/help/screen` | the same paths |

Pointing `--dir` straight at, say, a `pfiles/` directory does not work —
the two layouts do not agree closely enough on shape for that to be
reliable, which is why this indirection exists at all.

**A lib dir imported before 2026-08-24 has truncated mail; import it
again.** Until then the classic mail codec read a block chain's links as
block numbers where the C writes byte offsets (`docs/weirdnumbers.md`), so
every message longer than 79 characters stopped at its first block. There
was no error and nothing looks wrong afterwards: the `mail.yaml` that
`import --type=state` wrote is well formed, and the *message count* is
right — only the bodies are short. Mail is the only subsystem whose
on-disk format chains blocks together, so nothing else in the directory
is affected.

Re-run the import against the original `plrmail` into a scratch directory
and diff, rather than trying to spot the truncation by eye — a message that
runs to several blocks is a yaml block scalar, so its length is not
something one line of `awk` can tell you:

```sh
dlctl import --type=state --from-dir=/path/to/old/lib --to-dir=/tmp/mail-recheck
diff /tmp/mail-recheck/state/mail.yaml data-yaml/state/mail.yaml
```

No output means the old import was already correct. Otherwise take the new
file: every difference will be a message the old one cut short.

### Reformatting a roster between the two legacy formats

Neither is a format the server runs on any more — `dlctl import` is what
produces a directory it runs on. This is for comparing a converted roster
against the C server, which reads `binary` and nothing else:

```sh
dlctl convert --type=pfile --from-format=binary --from-dir=data \
                            --to-format=ascii    --to-dir=data
```

`--dry-run` reports what it would do without writing anything, which is
worth doing first. The conversion **refuses rather than truncates**: before
writing each character it asks the destination what it can hold and reports
anything that would not fit, because a truncated name is a different
character.

Passwords are carried across as-is. They are still legacy `crypt(3)` hashes
and upgrade individually on each character's next successful login.

Conversion runs in both directions.

`dlctl convert` with no `--type` is retired. It used to modernise a whole
legacy directory in place, leaving the formats as they were; nothing runs
on that output now, and the command says so and points at `import`.

### Inspecting player data

```sh
dlctl verify --type=pfile --dir=data --format=binary  # is this file what you think?
dlctl dump   --type=pfile --dir=data                  # list the roster
dlctl dump   --type=pfile --dir=data --name=zod
```

`verify` is the one to run before trusting a migration. It reports the record
size, how many characters are present, and how many are still on legacy
passwords — and it recognises a database written by a *64-bit* rebuild of the
C server, which would otherwise be read as 32-bit and silently misreport
every field past the first `long`.

Password hashes are never printed, by either command. They are real people's
credentials, they are DES with a public salt, and a terminal scrollback or a
CI log is not where they should end up.

### Checking a conversion lost nothing

```sh
dlctl verify --dir=/srv/lib --against=/srv/data                 # every subsystem
dlctl verify --dir=/srv/lib --against=/srv/data --type=pfile    # just the roster
```

With `--against`, `verify` loads **both** directories through their own
drivers and compares the states they load to, subsystem by subsystem,
reporting every difference rather than the first. `--dir`'s format
defaults per subsystem the way `import`'s source does — `binary` for the
roster, `classic` for everything else — and `--against-format` defaults to
`yaml`.

This is the check to run against your own archive, and it is worth
knowing why it compares *loaded state* rather than bytes. The claim worth
making is "a server running on the converted data behaves identically to
one running on the original", and bytes are a lossy proxy for that in both
directions: two `classic` files differing only in whitespace load
identically, and two identical loads can be written back differently by
any writer that does not reproduce 1990s formatting quirks exactly. See
`docs/proposals/yaml-only.md` §4.1.

There is one exception, and it is reported as an eighth line called
**`copied`**: `text/`'s plain prose (`motd`, `news`, `policies`, ...),
`config/game.yaml` and `text/help/screen` are *copied* by `import` rather
than converted, so there is no loader to compare them through and either
the bytes arrived or they did not. Those files were compared by nothing at
all before this — `text/greetings` and `text/credits` were covered by
accident, because the server refuses to start without them, and the other
nine were not, and neither was the tuning. A directory that came out of a
conversion without its `config/game.yaml` is a server quietly back on
`config.c`'s defaults, and `import --verify` used to call it clean. Ask
for that comparison on its own with `--type=copied`.

`dlctl import` runs the same comparison itself, on what it has just
written, and **fails the import if it does not hold**. That is the default;
`--verify=false` turns it off. It is the default because after the
yaml-only release `import` is the only path from an archived `lib/` to a
running server, run once, by somebody with no way to tell a complete
conversion from a nearly-complete one — and every conversion bug found so
far has been silent.

An import that fails verification leaves the output directory in place but
does **not** stamp it with a release version, so a server will not boot on
it.

### Setting a password from outside the game

```sh
dlctl passwd --type=pfile --dir=data Someone
printf '%s\n' "$NEW" | dlctl passwd --type=pfile Someone    # scripted
```

At a terminal it prompts twice with echo off, exactly as the game's menu does;
piped, it reads one line and skips the confirmation, because there is nobody
there to have made a typo. Either way it writes an argon2id hash.

Nothing in the game can do this — `set` has no password field and the menu
only ever lets the owner change their own — which leaves an archived character
whose 2008 password nobody remembers with no way back in. It applies the same
rule the menu does (`auth.BadPassword`: six characters minimum, no maximum)
and refuses any format that cannot store an argon2id hash.

**Do not run it against a live server.** A logged-in character's record is in
memory and gets written back on the next save, which would silently undo the
change. There is no lock enforcing that, only this warning and the one in the
command's help. See `docs/deviations.md`.

### Comparing against the C server

```sh
dlctl dump --type=world --dir=lib --out=go.json
```

Writes the loaded world as canonical JSON: deterministic ordering, values
as they are *after* load-time adjustments and reference resolution, and
absent exits explicitly null so a missing exit cannot be confused with an
exit to nowhere. Strings are escaped byte by byte, because a classic-format
world is not UTF-8 and a dump that "fixed" those bytes could hide a real
difference.

The C server dumps the same format with `reference/moderncserver/bin/circle
-J <file>`, which loads the world exactly as a real boot does and then
exits without opening a socket. To compare the two:

```sh
scripts/world-parity.sh
```

It builds both servers if needed, dumps from each, and diffs:

```
    1878 rooms, 569 mobiles, 679 objects, 30 zones, 46 shops
    identical
```

This runs at every release (`.github/workflows/release.yml`, not the
day-to-day `go.yml` — see `docs/developer.md`). If you change the Go
loader and it starts reporting differences, the Go loader is wrong — the
C server is the reference implementation, and it is the one that ran the
game.

## Exposure

Nothing here has been penetration-tested, and there is now something to
attack: the listeners, the telnet parser and the login flow all take input
from strangers, and the roster they authenticate against holds twenty-year-
old password hashes. The sane posture for now, and for a while yet:

- TLS listener only; leave `--listen-telnet` off.
- `--metrics-addr` and `--debug-addr` on loopback or not at all.
- Local, LAN, or VPN-only.

Most of §7 is built: per-address connection limits, a login grace period, a
handshake timeout, `--max-players` (checked at accept time, a soft cap —
see its own entry in `docs/proposals/go-port-plan.md`'s Phase 6 write-up
for the one race it does not close), and the ban list honoured at the
name prompt with `ban`/`unban` to maintain it in-game. Still missing: a
login-attempt rate limiter — the grace period and the per-address cap are
what stand in for one. See `docs/configuration.md` for which settings are
live.
