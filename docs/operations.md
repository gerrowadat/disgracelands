# Running and administering the server

How to run `dlmud`, what it exposes while running, and what to watch.

For the full settings list see `docs/configuration.md`. For building from
source see `BUILDING.md`.

> **Current state:** Phase 5 (the rest of the game) is done. The world
> resets, mobiles act, characters fight, cast, level and die, guildmasters
> teach, shopkeepers trade, and the boards, mail, houses, rent and the
> immortal commands all work. Phase 6 (OasisOLC) was decided against, in
> favour of editing `data/world` directly and reloading it live
> (`reloadmob`/`reloadzone`/`reloadobj`/`reloadshop`); Phase 7 (cutover)
> has not started. What is left of Phase 5 itself is a handful of small,
> named commands, listed one by one in `docs/proposals/go-port-plan.md`
> §10. Everything below about process management, health checking,
> logging and player data is real and works.

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
the file. Those are copied byte for byte and listed at the end of the run:

```
5 file(s) are binary formats this cannot convert yet. They have been
copied exactly as they are, because a byte-level conversion would corrupt
them — they hold struct fields and length-prefixed text, not characters.

  etc/board.mort
    message board: a struct dump with length-prefixed text; read as-is, but its text is not transcoded
```

**The server reads all five of those formats as they are**, so a converted
directory is fully usable — the copy is not a placeholder. What is still
outstanding is narrower than it sounds: three of the five (the rent files,
the house contents and the house control file) hold no text at all, and for
the other two — the boards and the mail — the CP1252 text *inside* the
records is left as it was. Transcoding that means decoding each record,
converting its strings and rewriting the lengths stored beside them, which
is a per-format job rather than something a directory-level converter can do.

Converting into a directory that already has something in it needs
`--force`, and converting a directory into itself is refused outright — a
conversion that failed part way would otherwise leave it half done.

### Converting into the yaml format

`dlctl convert` (above) modernises a `lib/` in place — CP1252 to UTF-8,
the player database reformatted as `ascii` pfiles — but keeps everything
in the original CircleMUD file shapes: `classic` world files, one board
per file, a struct-dump mail file. `dlctl lib import` goes further and
produces a single `yaml` directory instead — one file per zone and per
character, `config/`/`state/`/`text/help/help.yaml` for the rest — read
and written directly by the server with no further conversion step. See
`docs/design/data-format.md`.

Point it straight at the original archive, not at `dlctl convert`'s own
output — the two do not chain, since `dlctl convert` relocates the
roster to `pfiles/` and `lib import` expects it where the archive itself
keeps it:

```sh
dlctl lib import --from-dir=/path/to/old/lib --to-dir=data-yaml
```

That is the seven `world import`/`pfile import`/`state import`/`names
import`/`messages import`/`socials import`/`helpdb import` commands, run
in order against `--from-dir`'s own `world/`/`etc/`/`misc/`/`house/`/
`text/` subdirectories, plus the plain-text files under `text/` copied
across unchanged and, once everything else has succeeded, `--to-dir`
stamped with this build's own format version
(`docs/design/data-format-versioning.md`).

**Check the result for text that did not get transcoded.** Only two of
the seven importers — `world` and `pfile` — have their own `--encoding`
flag and decode CP1252 the way `dlctl convert` does; the other five
assume the source is already UTF-8. A real archive with a curly quote in
a social or an accented name on the disallowed-name list will carry that
byte straight through into a `.yaml` file that is not actually valid
UTF-8. This is real and current, not a hypothetical — see
`docs/design/data-format.md` §11.1 and `TODO.md` for the exact gap and
which importers still need it. Check with:

```sh
find data-yaml/config data-yaml/state data-yaml/text/help -type f \
  -exec sh -c 'iconv -f UTF-8 -t UTF-8 "$1" >/dev/null || echo "not valid UTF-8: $1"' _ {} \;
```

Nothing printed means nothing slipped through. `world` and `pfile`'s own
output needs no such check — `dlctl world lint` already reports invalid
UTF-8 in world text, and covers `--to-dir/world` the same way it covers
any other `--world-dir`.

**A lib dir imported before 2026-08-26 has truncated mail; re-import it.**
The classic mail codec read a block chain's links as block numbers, and in
the C they are byte offsets into the file (`docs/weirdnumbers.md`). Every
message longer than 79 characters therefore stopped at its first block, with
no error anywhere — the yaml `state import` writes is well-formed and short.
Nothing else was affected; mail is the only subsystem whose on-disk format
chains blocks together. Re-run the import against the original `plrmail` and
compare the message count and lengths:

```sh
grep -c '^- ' data-yaml/state/mail.yaml
```

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

### Setting a password from outside the game

```sh
dlctl pfile passwd --player-dir=data/pfiles Someone
printf '%s\n' "$NEW" | dlctl pfile passwd Someone    # scripted
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
dlctl world dump --world-dir=lib/world --out=go.json
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
C server is the reference implementation, and it is the one that has been
running the game.

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
