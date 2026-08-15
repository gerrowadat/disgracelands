# Running and administering the server

How to run `dlmud`, what it exposes while running, and what to watch.

For the full settings list see `docs/configuration.md`. For building from
source see `BUILDING.md`.

> **Current state:** there is no game in the server yet. It boots, reports
> itself ready, serves diagnostics and shuts down cleanly. Everything about
> process management, health checking and logging below is real and works;
> anything about players connecting does not, yet. See
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

Nothing consumes it yet, because there are no sessions to echo to.

## Backups

Back up `lib/`. That is all the state there is.

Of particular note, and none of it in git for good reason:

- `lib/etc/players` (or `lib/pfiles/`) — the roster, including password
  hashes.
- `lib/plrobjs/`, `lib/plralias/` — player inventories and aliases.
- `lib/house/`, `lib/etc/hcontrol` — player housing and its contents.
- `lib/etc/plrmail` — in-game mail.
- `lib/world/` — the world itself, which changes if anyone builds in-game.

The repo deliberately ships no player data; see the "Player data" section
of the top-level `README.md`.

## Offline tooling

`dlctl` handles the jobs that do not need a running server — the work
`src/util/` and `tools/` do for the C tree. `dlctl` with no arguments lists
what it can do; subcommands that are not built yet say which phase
implements them rather than failing obscurely.

## Exposure

Nothing here has been penetration-tested, and the game layer that would be
the interesting attack surface does not exist yet. The sane posture until
it does, and for a while after:

- TLS listener only; leave `--listen-telnet` off.
- `--metrics-addr` and `--debug-addr` on loopback or not at all.
- Local, LAN, or VPN-only.

`docs/proposals/go-port-plan.md` §7 covers what the network layer will do
about connection limits, login rate limiting and ban lists as it is built.
