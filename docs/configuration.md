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

> **Current state:** the server has no game in it yet. Settings marked
> *(inert)* are accepted and validated but do not yet affect anything,
> because the subsystem they configure has not been built. They are here
> because the configuration surface was built first, deliberately — see
> `docs/proposals/go-port-plan.md` §10.

## Data locations

| Flag | Default | Meaning |
|---|---|---|
| `--lib-dir` | `lib` | Runtime data directory: world files, help text, boards, player data. The same directory the C server takes with `-d`. |
| `--player-dir` | *(empty)* | Player-data directory. Empty means "use `--lib-dir`". |
| `--world-dir` | *(empty)* | World-data directory. Empty means `<lib-dir>/world`. |

`lib/` is **mutable state** — players, houses, boards, mail, and any world
files edited in-game. Back it up; mount it as a volume in a container.

## Formats

| Flag | Default | Values |
|---|---|---|
| `--player-format` | `binary` | `binary`, `ascii` |
| `--world-format` | `classic` | `classic` |

*(inert)* — the persistence layer lands in Phases 1 and 2.

`binary` is the original `struct char_file_u` flat file the C server writes
today; `ascii` is the ascii_pfiles 2.1 one-text-file-per-player format. The
defaults are chosen so that pointing the Go server at an existing `lib/`
needs no migration and no flags. See
`docs/investigations/ascii-pfile-format.md` for what the ascii format
actually contains, and `docs/proposals/go-port-plan.md` §5 for why this is
pluggable at all.

An unknown format name is rejected at startup rather than deep inside boot.

## Listeners

At least one listener must be enabled or the server refuses to start. An
empty address disables a listener.

| Flag | Default | Meaning |
|---|---|---|
| `--listen-telnet` | *(disabled)* | Plaintext telnet. |
| `--listen-telnets` | `:4443` | TLS-wrapped telnet. |
| `--listen-ws` | *(disabled)* | WebSocket, for browser clients. |

*(inert)* — listeners land in Phase 3.

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
| `--tls-acme-domain` | *(empty)* | Obtain a certificate automatically via ACME (Let's Encrypt) for this domain. |
| `--tls-acme-cache` | `lib/.acme` | Where ACME caches issued certificates. Must persist across restarts. |

`--tls-cert`/`--tls-key` and `--tls-acme-domain` are mutually exclusive.
Setting only one half of the cert/key pair is an error.

## Connections

| Flag | Default | Meaning |
|---|---|---|
| `--max-players` | `300` | Maximum simultaneous players. |
| `--max-connections-per-ip` | `8` | Maximum simultaneous connections from one address. |
| `--login-grace-time` | `60s` | How long a connection may stay unauthenticated before being dropped. |
| `--trust-proxy-headers` | `false` | Trust `X-Forwarded-For`. Only enable behind a proxy you control — otherwise clients can forge their apparent address. |

*(inert)* — enforced from Phase 3.

## Engine

| Flag | Default | Meaning |
|---|---|---|
| `--pulse-interval` | `100ms` | Game loop tick. The C server's `OPT_USEC` was 100ms and everything in the game is timed in multiples of it. |

*(inert)* — the pulse loop lands in Phase 3.

Changing `--pulse-interval` changes the speed of the entire game — combat
rounds, regeneration, zone resets, mob activity. It is a flag for testing,
not a tuning knob.

## Behaviour

These correspond one-to-one with the C server's single-letter options.

| Flag | C equivalent | Meaning |
|---|---|---|
| `--mini-mud` | `-m` | Load a minimal world, for testing. |
| `--skip-rent-check` | `-q` | Skip the rent scan on boot (faster startup). |
| `--restrict` | `-r` | Allow no new player registrations. |
| `--no-specials` | `-s` | Suppress special procedure assignment. |

*(inert)* — meaningful from Phase 1 onwards.

## Security

| Flag | Default | Meaning |
|---|---|---|
| `--allow-legacy-passwords` | `true` | Accept pre-2008 DES `crypt(3)` password hashes. |

*(inert)* — password handling lands in Phase 2.

The original playerfile stores DES `crypt(3)` hashes, salted with the
character's own name and truncated to 10 stored characters, which means
only the first 8 characters of a password ever mattered. Those hashes have
to be accepted for the 2001–2008 roster to be able to log in at all. The
intent is that a successful login transparently rehashes to a modern
algorithm, after which this can be turned off. The server warns at startup
while it is enabled.

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
compiled into `src/config.c` — rent costs, level caps, the OK/NOPERSON
message strings, autosave behaviour. Those are the settings that genuinely
want a file rather than fifty flags, and none of them exist in the Go tree
yet. Choosing a file format before there is anything to put in it would
have been guessing.
