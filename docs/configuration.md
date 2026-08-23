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

> **Current state:** Phase 5 is finished bar one slice, so the rules core, the
> economy, communication, boards, mail, houses, special procedures and the
> immortal commands are all built. Settings marked *(inert)* are accepted and
> validated but do not yet affect anything, because the subsystem they
> configure does not exist. They are here because the configuration surface was
> built first, deliberately — see `docs/proposals/go-port-plan.md` §10.

## Data locations

| Flag | Default | Meaning |
|---|---|---|
| `--lib-dir` | `lib` | Runtime data directory: world files, help text, boards, player data. The same directory the C server takes with `-d`. |
| `--player-dir` | *(empty)* | Player-data directory. Empty means `<lib-dir>/pfiles`. |
| `--world-dir` | *(empty)* | World-data directory. Empty means `<lib-dir>/world`. |

`data/` is **mutable state** — players, houses, boards, mail, and any world
files edited in-game. Back it up; mount it as a volume in a container.

## Formats

| Flag | Default | Values |
|---|---|---|
| `--player-format` | `ascii` | `ascii`, `native` |
| `--world-format` | `classic` | `classic`, `native` |
| `--state-format` | `classic` | `classic`, `native` |
| `--names-format` | `classic` | `classic`, `native` |
| `--messages-format` | `classic` | `classic`, `native` |
| `--socials-format` | `classic` | `classic`, `native` |
| `--help-format` | `classic` | `classic`, `native` |

`ascii` is the ascii_pfiles 2.1 one-text-file-per-player format. `classic`
is the original CircleMUD `.wld`/`.mob`/`.obj`/`.zon`/`.shp` flat-file world.
Both are what the archived `data/` is kept in, and both remain the default
so pointing the server at it needs no conversion step.

`native` is the YAML-over-JSON format `docs/proposals/data-format.md`
describes — for the world, one file per zone; for players, one file per
character, folding in the roster entry and the rent/crash file both (§8) —
read and written directly by the server, no conversion step needed to run
on it once converted. `--player-format=native` is also what turns on real
container nesting (an item saved inside a bag comes back inside it): every
other player format's on-disk shape has nowhere to record that, a
deliberate, documented deviation — see `docs/deviations.md`, "Renting
empties your bags and strips your body".

Convert an existing world directory once:

```sh
dlctl world import --from-dir=data/world --to-dir=data/world
```

Convert an existing roster once, into `ascii`:

```sh
dlctl pfile convert --from=binary --from-dir=data/etc \
                    --to=ascii    --to-dir=data/pfiles
```

— or into `native`, which also carries over any rent/crash file (read via
`binary`, since rent files are not pluggable the way the roster is — one
format for them regardless of `--player-format`, matching the C):

```sh
dlctl pfile import --from-dir=data/etc --to-dir=data/players
```

`--state-format` covers bans, boards, mail, player housing, the mud
clock and the bug/idea/typo reports together — one flag, since they end
up in one directory (`data/state/` under `native`) and there is no
reason to convert boards without mail. Convert an existing set once:

```sh
dlctl state import --from-dir=data/etc --from-house-dir=data/house \
                    --from-misc-dir=data/misc --to-dir=data/state
```

`--names-format` covers the disallowed-name list on its own
(`misc/xnames` under `classic`, `data/config/names.yaml` under `native`)
— its own flag because `config/` is a different directory than `state/`
is, not one that happens to move with the five stores above. Convert an
existing list once:

```sh
dlctl names import --from-path=data/misc/xnames --to-dir=data/config
```

`--messages-format` covers the `skill_message`/`dam_message` table on its
own (`misc/messages` under `classic`, `data/config/messages.yaml` under
`native`) — its own flag for the same reason `--names-format` is: it
shares `config/` with the disallowed-name list, but the two are otherwise
unrelated administrative concerns and do not need to move together.
Convert an existing table once:

```sh
dlctl messages import --from-path=data/misc/messages --to-dir=data/config
```

`--socials-format` covers the `do_action` table on its own (`misc/socials`
under `classic`, `data/config/socials.yaml` under `native`) — its own
flag for the same reason `--messages-format` is: it shares `config/`
with the disallowed-name list and the damage-message table, but the
three are otherwise unrelated administrative concerns and do not need to
move together. Convert an existing table once:

```sh
dlctl socials import --from-path=data/misc/socials --to-dir=data/config
```

`--help-format` covers the help database — `text/help/index` plus the
`.hlp` files it lists under `classic`, `text/help/help.yaml` plus one
`.txt` file per entry under `native`. Unlike the three flags above, both
formats live in the *same* directory rather than `misc/` versus
`config/`: they simply never read each other's files, so a converted
tree can sit right beside the classic one it came from. Convert an
existing archive once (`--to-dir` defaults to the same directory as
`--from-dir`, so this runs in place unless told otherwise):

```sh
dlctl helpdb import --from-dir=data/text/help --to-dir=data/text/help
```

**The server will not start on `--player-format=binary`**, and says so with
the conversion command in the error. The binary format is the original
`struct char_file_u` flat file the C server writes, and its password field
is eleven bytes — it cannot hold a modern hash at all, and every other field
in it is fixed-width. It remains fully readable and writable by `dlctl`,
because conversion needs both directions; it is simply not something a live
server should be stuck behind. See
`docs/proposals/go-port-plan.md` §5.2.

See `docs/investigations/ascii-pfile-format.md` for what the ascii format
contains, and `docs/proposals/go-port-plan.md` §5 for why formats are
pluggable at all.

An unknown format name is rejected at startup rather than deep inside boot.

## Listeners

At least one listener must be enabled or the server refuses to start. An
empty address disables a listener.

| Flag | Default | Meaning |
|---|---|---|
| `--listen-telnet` | *(disabled)* | Plaintext telnet. |
| `--listen-telnets` | `:4443` | TLS-wrapped telnet. |
| `--listen-ws` | *(disabled)* | WebSocket, for browser clients. *(inert)* |

The telnet listeners are live. **`--listen-ws` is inert**: the address is
accepted, but no WebSocket listener is started, so a server configured with
that alone exits with "no listeners could be started".

**Plaintext telnet is off by default and the server warns when it is on.**
Passwords cross the network in the clear on that listener; it exists for
period-correct clients and for byte-for-byte diffing against the C server,
not for general use.

**The defaults on their own are deliberately invalid.** Only the TLS
listener is enabled by default and it has no certificate, so an
unconfigured `dlmud` exits at startup with a message telling you what to
set. That is preferable to starting a server nobody can reach.

## TLS

| Flag | Default | Meaning |
|---|---|---|
| `--tls-cert` | *(empty)* | Certificate file. Must be set with `--tls-key`. |
| `--tls-key` | *(empty)* | Private key file. |
| `--tls-acme-domain` | *(empty)* | Obtain a certificate automatically via ACME (Let's Encrypt) for this domain. *(inert)* |
| `--tls-acme-cache` | `data/.acme` | Where ACME caches issued certificates. Must persist across restarts. *(inert)* |

`--tls-cert`/`--tls-key` and `--tls-acme-domain` are mutually exclusive.
Setting only one half of the cert/key pair is an error.

**ACME is not implemented.** Configuring it fails at startup, saying to use
`--tls-cert` and `--tls-key`, rather than starting a server whose
certificate never arrives.

## Connections

| Flag | Default | Meaning |
|---|---|---|
| `--max-players` | `300` | Maximum simultaneous connections, across every listener. |
| `--max-connections-per-ip` | `8` | Maximum simultaneous connections from one address. |
| `--login-grace-time` | `60s` | How long a connection may stay unauthenticated before being dropped. |
| `--trust-proxy-headers` | `false` | Trust `X-Forwarded-For`. Only enable behind a proxy you control — otherwise clients can forge their apparent address. *(inert)* |

`--max-players`, the per-address limit and the login grace time are all
enforced. `--trust-proxy-headers` has nothing to apply to until the
WebSocket listener exists.

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
| `--mini-mud` | `-m` | Load a minimal world, for testing. |
| `--skip-rent-check` | `-q` | Skip the rent scan on boot (faster startup). *(inert)* |
| `--restrict` | `-r` | Allow no new player registrations. |
| `--no-specials` | `-s` | Suppress special procedure assignment. |

`--mini-mud` loads each world subdirectory's `index.mini` instead of
`index`, `--restrict` turns new characters away at the login prompt, and
`--no-specials` skips the assignment table entirely, so guildmasters,
shopkeepers, bankers and the rest are ordinary mobiles.

`--skip-rent-check` is the one still inert. The rent files themselves are
built and wired in, but the thing the flag turns off — `update_obj_file()`
(objsave.c:332), the boot-time sweep that deletes rent files older than
`rent_file_timeout` days, called from `db.c:457` under exactly this
condition — has no equivalent here. There is nothing yet for the flag to skip.

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
| `--log-format` | `text` | `text` or `json`. |
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

- **`-c`** (syntax check) is now `dlctl world lint`, so it can run in CI
  without starting a server.
- **A bare port number** (`circle -q 4000`) is rejected, with a message
  pointing at `--listen-telnet`/`--listen-telnets`. There are three
  listeners now and no sensible way to guess which one was meant.

## A config file?

There isn't one yet. The precedence chain has a slot for it between
environment and defaults.

It will arrive with the values that justify it: the game tuning currently
compiled into `reference/moderncserver/src/config.c` — rent costs, level
caps, the OK/NOPERSON message strings, autosave behaviour. Those are the
settings that genuinely want a file rather than fifty flags, and none of
them exist in the Go tree yet. Choosing a file format before there is
anything to put in it would have been guessing.
