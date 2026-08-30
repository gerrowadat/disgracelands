# Configuring the server

Every setting on `dlmud` can be given as a command-line flag or as an
environment variable. The environment variable name is derived from the
flag name — uppercase, dashes to underscores, `DL_` prefix — so
`--lib-dir` is `DL_LIB_DIR` and `--max-connections-per-ip` is
`DL_MAX_CONNECTIONS_PER_IP`.

**Precedence is flag > environment > default.** A flag given on the
command line always wins, including the short aliases in
[Compatibility with the C server](#compatibility-with-the-c-server).

`dlmud --help` prints the same list this document describes, with the
environment variable name in brackets after each description. If the two
ever disagree, `--help` is right and this file is stale — CI checks that
every flag appears here, but it cannot check that the prose is accurate.

> **Current state:** Phase 5 (the rest of the game) is done, so the rules
> core, the economy, communication, boards, mail, houses, special
> procedures and the immortal commands are all built. Phase 6 (OasisOLC)
> was decided against, in favour of `reloadmob`/`reloadzone`/`reloadobj`/
> `reloadshop` — edit the world files in your `--lib-dir` directly and
> reload them into the running server without a restart; see
> `docs/proposals/go-port-plan.md`'s own
> Phase 6 write-up. Phase 7 (cutover) has not started. Settings marked
> *(inert)* are accepted and
> validated but do not yet affect anything, for reasons of their own —
> `--tls-acme-*` needs an ACME client — not because a phase is unfinished.
> They are here
> because the configuration surface was built first, deliberately — see
> `docs/proposals/go-port-plan.md` §10.

## Data locations

| Flag | Default | Meaning |
|---|---|---|
| `--lib-dir` | `examples/stock/yaml` | Runtime data directory: world files, help text, boards, player data. The same directory the C server takes with `-d`, in this server's own format. |
| `--player-dir` | *(empty)* | Player-data directory. Empty means `<lib-dir>/players`. |
| `--world-dir` | *(empty)* | World-data directory. Empty means `<lib-dir>/world`. |
| `--config` | *(empty)* | Overrides where the game-tuning file is read from (see "A config file: `<lib-dir>/config/game.yaml`" below). Empty means `<lib-dir>/config/game.yaml`, and no file there means `config.c`'s own defaults. |

The default points at `examples/stock/README.md`'s checked-in stock world
so a fresh clone boots something playable with no setup — point
`--lib-dir` at your own directory for anything beyond trying the server
out. **That directory has to be one `dlctl import` produced**: pointing
`--lib-dir` at a CircleMUD `lib/` is refused at boot, with the exact
command to convert it. See "The data format" below.

Whatever `--lib-dir` points at is **mutable state** — players, houses,
boards, mail, and any world files edited in-game. Back it up; mount it as
a volume in a container.

## The data format

There is one, and there is nothing to select. The seven `--*-format`
flags this server used to have — `--player-format`, `--world-format`,
`--state-format`, `--names-format`, `--messages-format`,
`--socials-format`, `--help-format` — are **gone**, along with their
`DL_*_FORMAT` environment variables. A flag whose only valid value is its
default is noise, and leaving one invites a future reader to think the
seam is still live.

The format is the YAML-over-JSON one `docs/design/data-format.md`
describes: one file per zone for the world, one file per character
(folding in the roster entry and the rent/crash file both, §8), one
`state/` directory for the clock, boards, mail, bans, houses and reports,
`config/` for the disallowed-name list, the damage messages and the
socials, and `text/help/` for the help database.

If you set one of the removed environment variables, the server **fails
to start and names it**, rather than ignoring it — a container that has
had `DL_WORLD_FORMAT=classic` in its unit file for years quietly booting
on data it was not pointed at is the most likely way this release goes
wrong for somebody.

### Converting a CircleMUD lib/

`dlctl import` is the only path from an archived `lib/` to a directory
this server runs on, and it is one command:

```sh
dlctl import --from-dir=/srv/lib --to-dir=/srv/data
```

The source is never written to. Everything comes across in one pass —
world, roster (including rent and crash files and aliases), bans, boards,
mail, houses, reports, the clock, the disallowed names, the damage
messages, the socials and the help database — plus `text/`'s plain prose
copied unchanged and `config/game.yaml` carried over, and finally a
`.dlversion` stamp naming the release that wrote it.

It then **verifies its own output** by loading both directories and
comparing them subsystem by subsystem, and fails the import if they do
not agree (`--verify=false` turns that off). See `docs/operations.md`,
"Checking a conversion lost nothing".

Point `--lib-dir` at `/srv/data` and you are done. Pointing it at
`/srv/lib` instead gets you this, before anything is opened:

```
/srv/lib is a CircleMUD lib/ directory, not a Disgracelands data directory:
it has world/zone.lst (the C server's zone list), misc/socials (a classic
socials table). This server reads one on-disk format; convert it once (the
source is not written to) and point --lib-dir at the result — see
docs/operations.md:
    dlctl import --from-dir=/srv/lib --to-dir=<somewhere>
```

Each subsystem also converts on its own, with `--type=world`, `pfile`,
`state`, `names`, `messages`, `socials` or `help`, for a directory laid
out unusually or a conversion done in stages. `dlctl` keeps its own
`--from-format` for saying which legacy format a source is in: `classic`
for the world and the four text tables, `binary` or `ascii` for a roster.

**None of the legacy decoders are in the server binary at all.** They are
not deleted from the tree and never will be — `classic` is the world
parity oracle for as long as the C server is authoritative, and `binary`
is the only thing that can read an archived roster — but they belong to
`dlctl` now. A legacy format is *absent* from `dlmud`, not merely refused
by it.

### Reformatting a roster between the two legacy formats

`dlctl convert --type=pfile` still copies a roster between `binary` and
`ascii` without going near yaml, which is a real thing to want when
comparing a converted roster against the C server:

```sh
dlctl convert --type=pfile --from-format=binary --from-dir=/srv/lib \
                           --to-format=ascii    --to-dir=/srv/scratch
```

`dlctl convert` with no `--type` is retired. It used to modernise a whole
legacy directory in place, leaving the formats as they were; nothing runs
on that output now, so it points at `import` instead.

See `docs/investigations/ascii-pfile-format.md` for what the ascii format
contains, and `docs/proposals/yaml-only.md` for why there is one format.

### The yaml format's own version

`--lib-dir`'s directory may hold a `.dlversion` stamp — the
`major.minor.patch` release of the `dlctl` that wrote it, not a flag and
not per-subsystem; see `docs/design/data-format-versioning.md`. If present,
`dlmud` checks it once at boot against its own release version:

- **A different *major*** — in either direction — is a fatal error before
  anything else opens. A major version only loads data written by its own
  major version, so moving a directory across a major release means
  restamping it deliberately (`dlctl data version --dir=<lib-dir>
  --write`) after checking the release notes, rather than finding out
  halfway through boot.
- **A different *minor***, same major, is a logged warning and the server
  starts anyway. The two builds are a release apart; usually that means
  nothing for the data, occasionally it means one side is writing
  something the other will not read.
- **Anything else** — the same major and minor, any patch, or no stamp at
  all — is silent.

`dlctl data version --dir=<lib-dir>` answers the same question without
starting a server, and `--write` stamps a directory that predates the
mechanism or that an older release wrote.

Both halves of this need a *released* binary: a build made with `go run`,
`go test` or a plain `go build` has no version of its own, so it stamps
nothing and checks nothing (design doc §6).

An unknown format name is rejected at startup rather than deep inside boot.

## Listeners

At least one listener must be enabled or the server refuses to start. An
empty address disables a listener.

| Flag | Default | Meaning |
|---|---|---|
| `--listen-telnet` | *(disabled)* | Plaintext telnet. |
| `--listen-telnets` | `:4443` | TLS-wrapped telnet. |
| `--listen-ws` | *(disabled)* | The web interface: a welcome page, a browser terminal at `/play`, and the WebSocket upgrade that terminal speaks over. |

All three are live: `--listen-ws` alone is enough to start the server.

**Plaintext telnet is off by default and the server warns when it is on.**
Passwords cross the network in the clear on that listener; it exists for
period-correct clients and for byte-for-byte diffing against the C server,
not for general use.

**The defaults on their own are deliberately invalid.** Only the TLS
listener is enabled by default and it has no certificate, so an
unconfigured `dlmud` exits at startup with a message telling you what to
set. That is preferable to starting a server nobody can reach.

## The web interface

`--listen-ws` opens one `net/http` server, not a raw socket: `GET /` is a
welcome page, `GET /play` is a terminal rendered by
[xterm.js](https://xtermjs.org/) (loaded from a CDN, not vendored — see
`docs/deviations.md`), and `GET /ws` is the WebSocket upgrade that
terminal actually connects to. `/ws` is wired straight into the same
`Server.serve` every telnet connection goes through
(`internal/server/web.go`): the same login prompt, the same MOTD, the same
shutdown handling, over a connection that happens to be a WebSocket rather
than a raw socket. It renders like a telnet client because it is
substantively one — the server sends it the same ANSI colour codes any
other client gets, with no telnet option negotiation at all (a browser has
nobody to negotiate with).

Keystrokes go to the server one at a time as they are typed, which is what
lets the pager answer a single keypress without Enter, and it means the
page does its own echoing — xterm.js has none, where a telnet client's
terminal driver would. Two consequences a player notices:

- **The arrow keys do nothing, except up, which repeats the last command
  you typed.** A cursor key is an escape sequence, not a character; before
  this the page forwarded it to the game as command text (an arrow at the
  name prompt answered "Names may only contain letters.") and echoed it
  back into the terminal, where it moved the cursor. Now it is swallowed
  in the browser and never reaches either. Up-arrow repeats only when the
  line is empty — a repeat is sent as text plus an Enter, so it would run
  into a half-finished line rather than replace it — and never repeats
  anything typed with echo off, so a password can never be replayed.
- **Backspace erases, on the screen and in the game.** The byte goes to
  the server like every other keystroke, and the server drops it along
  with the character before it, which is what the C server's own
  `process_input` does (`comm.c:1787`) and what this port was missing
  until #233. Erasing back to an empty line therefore re-enables the
  up-arrow, as it should.

If `--tls-cert`/`--tls-key` (or, once implemented, `--tls-acme-domain`) are
set, the web interface serves HTTPS and `wss://`; otherwise it is plain
HTTP and `ws://`, the same either/or every other listener already offers.

| Flag | Default | Meaning |
|---|---|---|
| `--web-password` | *(empty)* | Password required to use the web interface at all, via HTTP Basic Auth in front of every route. Any username is accepted — this is one shared secret for "may reach the web interface", not a second account system on top of the game's own login. |
| `--web-captcha` | `false` | Require solving a simple arithmetic question at `/play` before `/ws` will open a session — raising the cost of pointing a script at the web port above pointing one at the telnet port. It is not meant to stop a determined attacker: the answer space is small enough to brute-force in seconds. |

Both are optional and independent. Running with neither set is a fully
open, unauthenticated way to reach the game's own login prompt over the
web — which is a legitimate choice for a small, trusted community, and
exactly why `Config.Warnings` says so out loud rather than silently
assuming it was deliberate.

## TLS

| Flag | Default | Meaning |
|---|---|---|
| `--tls-cert` | *(empty)* | Certificate file. Must be set with `--tls-key`. |
| `--tls-key` | *(empty)* | Private key file. |
| `--tls-acme-domain` | *(empty)* | Obtain a certificate automatically via ACME (Let's Encrypt) for this domain. *(inert)* |
| `--tls-acme-cache` | `examples/stock/binary/.acme` | Where ACME caches issued certificates. Must persist across restarts. *(inert)* |
| `--tls-reload-interval` | `1m` | How often to check `--tls-cert`/`--tls-key` for a newer file and reload. `0` disables the check. |

`--tls-cert`/`--tls-key` and `--tls-acme-domain` are mutually exclusive.
Setting only one half of the cert/key pair is an error.

**ACME is not implemented.** Configuring it fails at startup, saying to use
`--tls-cert` and `--tls-key`, rather than starting a server whose
certificate never arrives.

**The certificate reloads on its own.** Renewing `--tls-cert`/`--tls-key`
in place — a `cert-manager`/`certbot` rotation, or an ops team's own cron —
takes effect on the next handshake, no restart needed: the server polls
both files' mtimes every `--tls-reload-interval` and, if either changed,
reloads and swaps the certificate in. A connection already in progress is
never touched, and a bad or unparsable file at reload time is logged and
the certificate already serving connections is kept, so a mistake writing
the new file can't take down a server that's already up (issue #147).

## Connections

| Flag | Default | Meaning |
|---|---|---|
| `--max-players` | `300` | Maximum simultaneous connections, across every listener. |
| `--max-connections-per-ip` | `8` | Maximum simultaneous connections from one address. |
| `--login-grace-time` | `60s` | How long a connection may stay unauthenticated before being dropped. |
| `--trust-proxy-headers` | `false` | Trust `X-Forwarded-For` and `X-Forwarded-Proto` on `--listen-ws`. Only enable behind a proxy you control — otherwise clients can forge their apparent address. |

`--max-players`, the per-address limit and the login grace time are all
enforced, on the web interface exactly as on the telnet listeners.

`--trust-proxy-headers` applies to `--listen-ws` only — a telnet
connection carries no headers to trust. With it on, `/ws` resolves the
player's address from `X-Forwarded-For` once, at the upgrade, and
everything downstream follows from that one answer: **site bans**, the
`last_host` on the player's record, what `users` and the wizlog show, and
the `--max-connections-per-ip` bucket. `X-Forwarded-Proto: https` also
makes the captcha-cleared cookie `Secure`, which `r.TLS` alone cannot
know behind a proxy that terminates the TLS itself.

Two things worth knowing before turning it on:

- **The rightmost entry wins.** `X-Forwarded-For` is a list each proxy
  appends to, so the last element is the one *your* proxy wrote and
  everything to its left came from further out. With one proxy in front
  that is exactly the client; reading the leftmost instead would let any
  player forge their address, walk past a site ban and reset their
  per-address count by typing a header. This means the flag assumes **one**
  hop: with two proxies in front, the address it resolves is the outer
  proxy's. A boolean cannot express a chain depth, and a deployment with a
  chain wants a list of trusted proxy addresses rather than a different
  reading of the header.
- **Leaving it off behind a proxy is not neutral.** Every web connection
  then reports the proxy's own address, so `ban site` cannot reach a web
  player at all, banning the proxy locks out every web player at once, and
  the default `--max-connections-per-ip 8` caps the entire web interface
  at eight players sharing one bucket. The server warns at startup when
  `--listen-ws` has neither TLS nor this flag.

`--max-connections-per-ip` counts an IPv6 address by its own `/64`, not by
itself — the block an ISP actually hands one subscriber (RFC 6177), so
one machine's ordinary address rotation (RFC 4941) cannot walk straight
through the limit for free the way an address-exact count would let it.
An IPv4 address, and an IPv4-mapped IPv6 one, still count by themselves.

## Engine

| Flag | Default | Meaning |
|---|---|---|
| `--pulse-interval` | `100ms` | Game loop tick. The C server's `OPT_USEC` was 100ms and everything in the game is timed in multiples of it. |
| `--rng` | `modern` | Which generator the game rolls on: `modern` (Go's PCG) or `circle` (the C server's own, ported exactly). |
| `--rng-seed` | `0` | Seed for it. `0` means the clock, which is what the C server does. |

The pulse loop runs, and `dlmud_pulse_duration_seconds` measures it against
this budget. It drives command dispatch, autosave, combat rounds, mobile
activity, zone resets, regeneration and the mud clock — everything in the
game is a multiple of it.

Changing `--pulse-interval` changes the speed of the entire game — combat
rounds, regeneration, zone resets, mob activity. It is a flag for testing,
not a tuning knob.

### `--rng`, and why there is a choice

The C server has its own random number generator (`src/random.c`) — the
Park-Miller minimal standard, a Lehmer generator from 1988 — seeded once from
`time(0)` at boot. It is portable and fully deterministic: the constants are
chosen so no intermediate value overflows a signed 32-bit integer, which is
what let it produce the same sequence on a VAX and on everything since.

That portability is worth something here. `--rng=circle` with a fixed
`--rng-seed` makes this server roll **the same numbers the C server would**,
draw for draw — every damage roll, every hit roll, every ability score. That
is what the parity work compares against, and it is a far stronger check on a
combat formula than asserting the result landed in a plausible range.

`modern` is the default for ordinary play: `circle` is a generator from 1988
with known-weak low bits. That does not matter for a damage roll, and it is
still not something to be on by default without saying so.

Neither of these has anything to do with security. Passwords and TLS use
`crypto/rand` and always will.

A non-zero `--rng-seed` makes a run reproducible, which is useful for
reproducing a bug and a bad idea on a live server — players would learn the
sequence.

## Behaviour

These correspond one-to-one with the C server's single-letter options.

| Flag | C equivalent | Meaning |
|---|---|---|
| `--mini-mud` | `-m` | Load only the zones in `world/sets.yaml`'s `mini` set. |
| `--skip-rent-check` | `-q` | Skip the rent scan on boot (faster startup). |
| `--restrict` | `-r` | Allow no new player registrations. Sets the wizlock to 1, which is all `-r` is in the C (`comm.c:329`), so `wizlock 0` in-game reopens it. |
| `--no-specials` | `-s` | Suppress special procedure assignment. |

`--restrict` turns new characters away at the login prompt, and
`--no-specials` skips the assignment table entirely, so guildmasters,
shopkeepers, bankers and the rest are ordinary mobiles.

**`--mini-mud` loads a named subset of the zones.** The C server, and this
port's `classic` reader, do it by reading each world subdirectory's
`index.mini` instead of `index`. The `yaml` format has one file per zone,
so it does it by naming zones: `world/sets.yaml` holds named subsets and
`--mini-mud` selects the one called `mini`.

```yaml
# <lib-dir>/world/sets.yaml
schema: dl/sets@1
sets:
  mini:
  - 0
  - 12
  - 30
```

`dlctl import` writes that file for you, derived from the source's own
`index.mini` files, so a converted archive keeps the small world it
already had. On the shipped `examples/stock/yaml` it is zones 0, 12 and
30: 69 rooms, 51 mobiles, 59 objects and 3 zones, against 1,878 / 569 /
679 / 30.

**Asking for a subset the directory does not define is an error**, not a
quiet full load: `--mini-mud` against a `sets.yaml` with no `mini` set (or
no `sets.yaml` at all) refuses to boot and says so. That matches the C —
`index_boot` exits when the index file it was told to open is missing —
and it is the specific failure this flag had for a while: between the
yaml-only release and 2026-08-29 it was accepted, validated, passed to the
world source and then ignored, because only the `classic` reader ever read
it and the server had stopped linking `classic` (issue #274). A flag that
quietly does nothing looks exactly like a flag that worked.

`--skip-rent-check` skips `Server.SweepRentFiles`
(`internal/server/rentsweep.go`), the boot-time deletion of rent files
older than 30 real days and crash files older than 10 — `update_obj_file()`
(objsave.c:332), called from `db.c:457` under exactly this condition.

**On a default server the flag makes no difference, because the sweep does
not run at all.** It is skipped whenever rent is free, which is the default
(`free_rent`, below) and was the archive's own setting: the sweep enforces a
charge that is not being made, and on a freshly converted `lib/` it deleted
the stored possessions of every character who had not played in thirty days.
See `docs/deviations.md` for the full argument. Turn `free_rent` off in
`<lib-dir>/config/game.yaml` and the sweep — and this flag — behave exactly
as the C's do.

| Flag | C equivalent | Meaning |
|---|---|---|
| `--freeze-mobiles` | `-M` | Hold the mobiles still: no wandering, no scavenging, no mobile-activity dice. |

`--freeze-mobiles` is not a game setting and is not one of the C server's
own options: `-M` is a `<DoC>` addition made to the C tree for the
session-parity suite (`test/parity`, `docs/developer.md`), and this is its
counterpart. A wandering mobile's position depends on how many pulses have
elapsed since boot, so two servers started seconds apart disagree about
every room a janitor walks through; `mobile_activity` also rolls dice, so
leaving it running walks two fixed-seed generators out of step with each
other. Both servers drop the pulse entirely rather than entering it and
returning early, so neither rolls anything. **Nothing but a comparison
harness should set it** — a world whose mobiles never move is not the
game.

`--freeze-weather` is the same kind of lever and the same kind of `<DoC>`
addition (`-W`), for a reason worth stating separately: `weather_change`
rolls five dice every mud hour and sometimes six (`weather.c:88`), and the
harness plays its script at one server and *then* at the other. By the same
line of the script the second server has been running about a minute
longer, so it can have had a weather tick the first had not — five draws
that put the two generators out of step for everything afterwards. Nothing
in the game reads the sky except its own four messages; what `-W`
suppresses is the dice. **Nothing but a comparison harness should set this
either.**

## Security

| Flag | Default | Meaning |
|---|---|---|
| `--allow-legacy-passwords` | `true` | Accept pre-2008 DES `crypt(3)` password hashes. |

The original playerfile stores DES `crypt(3)` hashes, salted with the
character's own name and truncated to 10 stored characters — which means
**only the first 8 characters of a password ever mattered**, and the salt is
derivable from the character's name. Those hashes have to be accepted for
the 2001–2008 roster to be able to log in at all.

**A successful login replaces the hash with argon2id**, in place, without
the player noticing or being asked to reset anything. Once every active
character has logged in at least once, this can be turned off; the server
warns at startup while it is enabled.

Turning it off locks out anyone who has not logged in since the migration,
so it fails with a distinguishable error rather than looking like a wrong
password.

## Logging and diagnostics

| Flag | Default | Meaning |
|---|---|---|
| `--log-file` | `-` | Log destination. `-` means stderr. |
| `--log-format` | `text` | `text` for a terminal, `json` for OpenTelemetry-shaped records ([operations.md](operations.md#the-json-format)). |
| `--log-level` | `info` | `debug`, `info`, `warn`, `error`. `debug` also adds source file and line. |
| `--metrics-addr` | *(disabled)* | Serves `/metrics`, `/healthz`, `/readyz`. |
| `--debug-addr` | *(disabled)* | Serves `/debug/pprof`. **Never expose this**; the server warns when it is set. |

Log files are created mode 0600: the log records connecting hosts and
player names.

See `docs/operations.md` for what the endpoints return and what to alert
on.

## Other

| Flag | Meaning |
|---|---|
| `--version` | Print version information and exit. Skips validation, so it works on an unconfigured box. |
| `--help` | Print the option list and exit. |

## Compatibility with the C server

The C server's single-letter options work as aliases. They are not separate
settings — they write to the same place as the long forms, and they beat
the environment exactly as the long forms do.

| Alias | Long form |
|---|---|
| `-d <dir>` | `--lib-dir` |
| `-o <file>` | `--log-file` |
| `-m` | `--mini-mud` |
| `-q` | `--skip-rent-check` |
| `-r` | `--restrict` |
| `-s` | `--no-specials` |

Two C options have no flag equivalent:

- **`-c`** (syntax check) is now `dlctl lint --type=world`, so it can run
  in CI without starting a server.
- **A bare port number** (`circle -q 4000`) is rejected, with a message
  pointing at `--listen-telnet`/`--listen-telnets`. There are three
  listeners now and no sensible way to guess which one was meant.

## A config file: `<lib-dir>/config/game.yaml`

The game tuning is a YAML file of `reference/moderncserver/src/config.c`'s
runtime-tunable values — twelve fields, picked deliberately rather than
reopening `config.c` wholesale; see `docs/deviations.md` for which and why.

**It lives in the data directory**, at `<lib-dir>/config/game.yaml`, beside
`config/names.yaml` and the rest. That is where game configuration belongs
and deployment configuration does not: whether rent is free is a property
of *this game*, travels with the world, goes into the same backup and is
worth reviewing alongside it, in the way a listen address, a certificate
path or a log level never is (`docs/design/data-format.md` §6). Nothing on
the command line names it, and a server given nothing but `--lib-dir` is
configured by the world it is running.

Every example data directory ships the annotated template — the default
`--lib-dir` included, at `examples/stock/binary/config/game.yaml` — with
every key present but commented out at its `config.c` default. Copy it into
your own data directory and edit it rather than writing one from scratch.

The file is optional. No file, an empty file, or a comments-only file (the
shipped example, as-is) all reproduce `config.c`'s own values exactly —
every stock and archived `lib/` in existence has no `config/game.yaml` in
it, and every one of them has to boot. The precedence chain's config-file
slot sits between environment and defaults for exactly this reason.

`--config` (`DL_CONFIG`) overrides the path, for a deployment that wants the
file somewhere else — mounted from a secret store, shared between two
servers on one world. Unlike the default path, a `--config` that is not
there is a boot failure: it names a file that was asked for by name, so a
typo in it is a mistake rather than a directory that has never been tuned.

The running server rereads whichever file it is on `SIGHUP` and applies it
live, no restart needed. A file that fails to parse, or parses but fails
validation (`autosave_time: 0`, `max_bad_pws: 0`, `tunnel_size: 0`, a
negative cost, ...), is
logged and ignored — the server keeps running on whatever tuning it had
before, and a `SIGHUP` with no file to read at all says so and changes
nothing. `docs/operations.md` has how to send a signal under each runtime,
and what else can be reloaded without a restart.

```yaml
free_rent: false
min_rent_cost: 250
level_can_shout: 5
max_bad_pws: 3
tunnel_size: 2
```

Everything else in `config.c` — `pk_allowed`, the room vnums, autowiz, the
OK/NOPERSON message strings — is still a constant. Each was considered and
left that way on purpose, not overlooked.
