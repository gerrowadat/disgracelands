# Running and administering the server

How to run `dlmud`, what it exposes while running, and what to watch.

For the full settings list see `docs/configuration.md`. For building from
source see `BUILDING.md`.

> **Current state:** the world resets, mobiles act, and characters fight,
> cast, level and die — Phase 4 is done. The economy, communication, mail and
> special procedures are Phase 5, so shopkeepers and guildmasters stand there
> inert. Everything below about process management, health checking, logging
> and player data is real and works. See
> `docs/proposals/go-port-plan.md` §10 for what arrives when.

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
- **`/lib` is a volume.** It is mutable state — players, houses, boards,
  mail, and any world file edited in-game. An image rebuild must not lose
  it.
- The container runs as **non-root**. `/lib` must be writable by uid 65532
  (`nonroot`).
- Health checks cannot use `curl`, for the same no-shell reason. Point an
  external check at `/readyz` on `--metrics-addr`, or use the `dlctl`
  binary in the image.

## Shutdown

`SIGTERM` (what `docker stop` and `systemctl stop` send) and `SIGINT`
trigger a graceful shutdown: stop accepting, save, close, exit 0.

**Give it time to finish.** The C server autosaved every 60 seconds, so an
ungraceful kill could lose up to a minute of play; handling `SIGTERM`
properly is the entire reason not to. The shutdown budget is 30 seconds.
Configure your supervisor to allow at least that:

- Docker: `--stop-timeout 45` or `stop_grace_period: 45s` in compose.
- systemd: `TimeoutStopSec=45`.
- Kubernetes: `terminationGracePeriodSeconds: 45`.

A second `SIGINT` during shutdown kills the process rather than being
swallowed, so a wedged shutdown can still be interrupted from a terminal.

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

Startup warnings are worth reading rather than filtering. The server warns
about exactly the things that are safe-but-questionable: plaintext telnet
enabled, legacy DES password verification enabled, pprof listening, a
WebSocket listener with neither TLS nor a trusted proxy in front.

### Watching the game from in-game

The C server's `mudlog()` did two things: wrote to the log, and echoed the
message to online immortals above a given level. That second job is how
gods actually watch a running game, and it is preserved — log records carry
a `wizvis` attribute holding the minimum level that should see them
in-game.

Nothing consumes it yet. There are sessions to echo to now, but the
immortal side — who is watching, and at what level — arrives with
`act.wizard.c`, which is Phase 5i.

## Backups

Back up `data/`. That is all the state there is.

Of particular note, and none of it in git for good reason:

- `data/etc/players` (or `data/pfiles/`) — the roster, including password
  hashes.
- `data/plrobjs/`, `data/plralias/` — player inventories and aliases.
- `data/house/`, `data/etc/hcontrol` — player housing and its contents.
- `data/etc/plrmail` — in-game mail.
- `data/world/` — the world itself, which changes if anyone builds in-game.

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
dlctl world lint --world-dir=lib/world
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

`world lint` also reports world text that is not valid UTF-8, which is how
you find out a directory still needs converting. The server works in UTF-8;
see `dlctl convert` above.

The shipped world currently reports **0 errors, 11 warnings, 12 notes**. The
warnings are worth knowing about:

- Four complete zones (23, 90, 92, 147) and two further `.zon` files exist
  in `data/world/` but appear in no index, so nothing ever loads them. This
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

If you have an original CircleMUD `lib/`, convert the whole thing once:

```sh
dlctl convert --from=/path/to/old/lib --to=data --dry-run   # look first
dlctl convert --from=/path/to/old/lib --to=data
```

That does three things: reformats the binary player database as ascii
pfiles, converts text from CP1252 to UTF-8, and copies everything else
across. `--encoding=latin1` if the source really is Latin-1 rather than
CP1252 — they differ only at bytes 0x80–0x9F, which is exactly where the
curly quotes a word processor inserts live, so the default is usually right.

**It refuses to guess.** Several files in a CircleMUD data directory are
struct dumps rather than text — the message boards, player mail, house
contents, rent files. Running a byte-level transcode over one of those
corrupts it twice: once by rewriting bytes that were never characters, and
again by changing the length of text whose length is stored separately in
the file. Those are copied byte for byte and listed at the end of the run,
each with the phase that will handle it:

```
5 file(s) are binary formats this cannot convert yet. They have been
copied exactly as they are, because a byte-level conversion would corrupt
them — they hold struct fields and length-prefixed text, not characters.

  etc/board.mort
    message board: a struct dump with length-prefixed text; converted when boards are implemented
```

Converting into a directory that already has something in it needs
`--force`, and converting a directory into itself is refused outright — a
conversion that failed part way would otherwise leave it half done.

### Converting only the player roster

The server runs on the ascii format and refuses to start on the original
binary one — see `docs/configuration.md`. An existing data directory is
converted once:

```sh
dlctl pfile convert --from=binary --from-dir=data/etc \
                    --to=ascii    --to-dir=data/pfiles
```

`--dry-run` reports what it would do without writing anything, which is
worth doing first. The conversion **refuses rather than truncates**: before
writing each character it asks the destination what it can hold and reports
anything that would not fit, because a truncated name is a different
character.

Passwords are carried across as-is. They are still legacy `crypt(3)` hashes
and upgrade individually on each character's next successful login.

Conversion runs in both directions, which is how you compare a converted
roster against the C server, or undo a migration.

### Inspecting player data

```sh
dlctl pfile verify --player-dir=data/etc     # is this file what you think?
dlctl pfile dump   --player-dir=data/pfiles  # list the roster
dlctl pfile dump   --player-dir=data/pfiles --name=zod
```

`verify` is the one to run before trusting a migration. It reports the record
size, how many characters are present, and how many are still on legacy
passwords — and it recognises a database written by a *64-bit* rebuild of the
C server, which would otherwise be read as 32-bit and silently misreport
every field past the first `long`.

Password hashes are never printed, by either command. They are real people's
credentials, they are DES with a public salt, and a terminal scrollback or a
CI log is not where they should end up.

### Comparing against the C server

```sh
dlctl world dump --world-dir=lib/world --out=go.json
```

Writes the loaded world as canonical JSON: deterministic ordering, values
as they are *after* load-time adjustments and reference resolution, and
absent exits explicitly null so a missing exit cannot be confused with an
exit to nowhere. Strings are escaped byte by byte, because `data/world` is
not UTF-8 and a dump that "fixed" those bytes could hide a real difference.

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

This runs in CI. If you change the Go loader and it starts reporting
differences, the Go loader is wrong — the C server is the reference
implementation, and it is the one that has been running the game.

## Exposure

Nothing here has been penetration-tested, and there is now something to
attack: the listeners, the telnet parser and the login flow all take input
from strangers, and the roster they authenticate against holds twenty-year-
old password hashes. The sane posture for now, and for a while yet:

- TLS listener only; leave `--listen-telnet` off.
- `--metrics-addr` and `--debug-addr` on loopback or not at all.
- Local, LAN, or VPN-only.

`docs/proposals/go-port-plan.md` §7 covers what the network layer will do
about connection limits, login rate limiting and ban lists as it is built.
