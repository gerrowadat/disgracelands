# Porting Disgracelands to Go

A plan for reimplementing the Disgracelands engine in Go: 64-bit safe,
pluggable player- and world-file formats, and packaged as a normal modern
service (flags, env vars, structured logs, containers) rather than a
2002-era autoconf tree driven by `autorun`.

This is a design/sequencing document. **Phase 0 (§10) is built**; everything
from Phase 1 on is still a plan. See `BUILDING.md` for how to build and run
what exists.

---

## 0. Decisions already taken

These were settled up front and the rest of the plan assumes them:

| Question | Decision |
|---|---|
| **Fidelity** | Faithful core, known bugs fixed. Same game feel, same mechanics (remort bitmask, Paladin alignment, the balance tweaks in `docs/investigations/non-stock-features.md`); the `sprintf`-overlap class of bugs from `TODO.md` §3 and integer-width bugs get fixed as they're encountered, each deviation recorded. Deliberate rules changes are a separate, later conversation. |
| **Repo layout** | Same repo, new top-level Go tree. The C tree in `reference/moderncserver/src/` stays buildable and authoritative for the whole port — it is the reference implementation and the parity oracle. |
| **Scripting** | Design the seam, defer the engine. Trigger/event interfaces get defined so DG Scripts, Lua, or anything else can drop in later; v1 ships no interpreter. The tree that was actually played has no DG Scripts (`docs/investigations/non-stock-features.md`), so nothing regresses. |
| **Protocols** | TLS-wrapped telnet, WebSocket, and telnet option negotiation (MSSP/MCCP/GMCP/MXP). |

**One assumption flagged:** plain unencrypted telnet was *not* selected in
that list. This plan keeps a plaintext telnet listener implemented and
present, but **off by default**, enabled only with `--listen-telnet`. That
preserves the ability to connect with a 2002-era client (and to diff
against the C server byte-for-byte during parity testing) without exposing
plaintext credentials by accident. If the intent was to drop plaintext
telnet entirely, delete the listener — say so and it comes out.

---

## 1. What is actually being ported

From `wc -l`: ~44k lines of C across 60-odd `.c` files, plus
`reference/moderncserver/src/util/` and the OasisOLC `gen*.c`/`*edit.c`
online-building layer. Roughly:

| Area | C files | Notes |
|---|---|---|
| Network + main loop | `comm.c` (2595) | Being replaced wholesale, not ported. |
| Command dispatch, login | `interpreter.c` (1804) | The `nanny()` login state machine is the security-critical part. |
| World/player loading | `db.c` (3012), `objsave.c` (1175) | Split into the two pluggable format packages. |
| Game rules | `fight.c`, `magic.c`, `spell_parser.c`, `spells.c`, `class.c`, `limits.c`, `handler.c` | The faithful-port core. |
| Player commands | `act.*.c` (~9k total) | Largest volume, lowest risk, highly parallelisable. |
| Building/OLC | `oasis.c`, `gen*.c`, `*edit.c` (~6k) | Deferrable to a late phase; the world can be edited offline meanwhile. |
| Shops, houses, boards, mail | `shop.c`, `house.c`, `boards.c`, `mail.c` | Each has its own on-disk format and needs its own persistence seam. |
| Offline utilities | `reference/moderncserver/src/util/*`, `reference/tools/*` | Become Go subcommands of one `dlctl` binary. |

`castle.c` and the `spec_procs.c`/`spec_assign.c` special procedures are
hand-written per-mob Go functions in a registry — this is the natural home
for the scripting seam later.

---

## 2. Repo layout

```
cmd/
  dlmud/            The server binary.
  dlctl/            Offline tooling: format conversion, pfile dump,
                    world lint (`scheck`), autowiz, mudpasswd, listrent.
internal/
  game/             The world model + rules. No I/O, no formats.
    char.go  room.go  obj.go  zone.go
    combat/  magic/  skills/  shop/
  engine/           The pulse loop, event scheduler, command dispatch.
  net/              Listeners and protocol handling.
    telnet/  ws/  negotiation/
  session/          Descriptor lifecycle, login state machine, output queues.
  persist/          The pluggable-format machinery.
    player/         PlayerStore interface + registry.
      binary/       Default: struct char_file_u compatible.
      ascii/        ascii_pfiles 2.1 compatible.
    world/          WorldSource interface + registry.
      classic/      Default: the #vnum ~-terminated flat files in lib/world.
    objsave/  house/  board/  mail/
  config/           Flag + env + file resolution.
  obs/              Logging, metrics, health.
pkg/                Only if something genuinely wants an external consumer.
build/
  Dockerfile  docker-compose.yml
docs/proposals/go-port-plan.md    (this file)
```

`reference/moderncserver/src/`, `data/`, `reference/moderncserver/doc/`,
`reference/tools/`, `reference/` are untouched. `data/` stays the runtime
data directory both servers read — that is what makes side-by-side parity
testing possible.

Module path: `github.com/gerrowadat/disgracelands`, matching the remote.
Go version: 1.25, which is what the tree is built and tested against;
`log/slog`, `net/http`'s method-aware routing patterns and `iter.Seq2` are
all in use, so 1.23 is the realistic floor.

---

## 3. Architecture

### 3.1 Concurrency model

CircleMUD is a single-threaded loop pulsing every 100ms (`OPT_USEC`,
`structs.h:508`), with `PULSE_ZONE`/`PULSE_MOBILE`/`PULSE_VIOLENCE`/
`PULSE_AUTOSAVE` derived from it. Keep that. It is the reason the C code
can pass raw `struct char_data *` pointers around with no locking, and
reproducing it is what makes a faithful port tractable.

The Go shape:

- **One goroutine owns the world.** All mutation of rooms, characters and
  objects happens on the game goroutine. No mutexes on game state, because
  nothing else touches game state.
- **Two goroutines per connection** (read and write), owned by `net/`.
  They never touch `internal/game`. They hand parsed input to the game
  goroutine over a channel and receive output over a per-session buffered
  channel.
- **Backpressure is explicit.** A slow client fills its output channel; on
  overflow the session is marked as lagging and dropped, rather than
  blocking the world (`comm.c`'s current behaviour is to grow an unbounded
  `txt_q`, which is a memory-exhaustion vector).
- **Ticks come from a `time.Ticker`**, with the pulse counter kept as
  `uint64`. Long-running work that must not stall the pulse (file writes,
  DNS lookup of connecting hosts, TLS handshakes) runs off-loop and posts
  results back as events.
- **`go test -race` in CI is non-negotiable**, and the game goroutine
  asserts its own identity in debug builds so an accidental cross-goroutine
  world access is caught immediately rather than at 3am under load.

This is the boring choice and it is the right one. An actor-per-entity or
lock-per-room design would be more "Go-ish" and would make combat, group
mechanics, zone resets and `char_from_room`/`char_to_room` dramatically
harder to port faithfully.

### 3.2 Pointers vs. IDs

The C code stores raw pointers (`ch->in_room` is an rnum index,
`ch->carrying` is a linked list of `obj_data *`). In Go, model entities as
structs held in slabs/maps keyed by stable IDs, with helper accessors —
this removes the whole class of dangling-pointer/extraction-order bugs
(`extract_char` deferred-extraction dance in `handler.c`) that the C code
manages by convention. Keep rnum/vnum as distinct named types
(`type RoomVnum int32`, `type RoomRnum int32`) so the compiler catches the
confusion that the C code only catches by careful reading.

### 3.3 Command dispatch

`interpreter.c`'s `cmd_info[]` table becomes a registry: each command
registers a name, minimum position, minimum level, and handler. Same
abbreviation-matching semantics (first match in table order — this is
player-visible behaviour and must be preserved exactly), but built from
per-package `init()` registration or an explicit `Register()` call at
startup rather than one giant array.

---

## 4. 64-bit safety

This is where the current tree is genuinely broken, and the port has to be
deliberate about it.

**The concrete existing trap** (already documented in
`docs/investigations/pfile-conversion.md`): `struct char_file_u` contains
`time_t birth`, `time_t last_logon`, and multiple `long` fields (`idnum`,
`act`, `affected_by`, `pref`, the `spare17..21` slots —
`structs.h:818–886`). These are 4 bytes in the 32-bit build that wrote the
real 2001–2008 data and 8 bytes on a native 64-bit build, so a 64-bit
`fread()` of the same file silently misreads everything past the first
`long`. `reference/tools/bin2ascii.c` has to be built `-m32` for exactly
this reason.

**Rules for the Go port:**

1. **No implementation-defined widths anywhere in a serialized struct.**
   Every persisted field gets an explicit Go type: `int32`, `uint32`,
   `int64`. `int` is allowed only for in-memory, never-serialized values.
2. **Legacy binary reads are explicitly 32-bit.** The default binary
   `PlayerStore` decodes the *historical* layout: `long` → `int32`,
   `time_t` → `int32`, with the original struct padding/alignment
   reproduced explicitly rather than inferred. Decoding is done field by
   field with `encoding/binary` against a documented offset table, not by
   casting a byte slice over a struct. The layout table is derived from
   `reference/tools/bin2ascii.c`, which is already proven correct against all 108
   real records.
3. **Byte order is pinned to little-endian** for the legacy format, with a
   note that the archive report (§5) already ruled out big-endian/SPARC
   origin for this specific data. Reading a big-endian-written file is not
   supported and should fail loudly rather than produce garbage.
4. **The Year 2038 problem is real here.** `time_t` as `int32` overflows in
   January 2038. Legacy files keep 32-bit timestamps on read *and* on write
   (that is what "compatible" means), but the in-memory model uses
   `time.Time`, and any new format writes `int64` seconds or RFC3339. This
   needs to be stated in the format docs, because a 2038-safe engine
   writing a 2038-unsafe file is a footgun waiting for someone in 2037.
5. **Bitvectors get a real type.** `bitvector_t` is `unsigned long`
   (`structs.h:599`) — 32 bits on the platform this was written for, 64 on
   modern Linux. Flags in the world files are letter-encoded (`a`=bit 0,
   `b`=bit 1, …; see `data/world/obj/0.obj`'s `9 0 ae 0` line), which is
   width-agnostic on disk. In Go use a named `uint64` type with generated
   constants, and have the legacy binary codec mask to 32 bits on write so
   a flag added past bit 31 can't silently corrupt an old-format save. Flag
   sets that exceed 32 bits are a hard error in the legacy writer, not a
   truncation.
6. **`sh_int` is `int16`, `byte`/`ubyte` are `int8`/`uint8`** — and where
   the C code relies on wraparound or on a value being clamped by its own
   narrowness (hit points, `apply_saving_throw[5]`), the Go code clamps
   explicitly and logs. Several of the "balance tweaks" in
   `docs/investigations/non-stock-features.md` may depend on this; each
   needs checking against the C behaviour as it's ported.
7. **Overflow checks on arithmetic that reaches player-controllable
   magnitudes**: gold, experience, `played` seconds. The C code overflows
   silently; the Go code saturates at a documented cap.

A `dlctl pfile verify` subcommand cross-checks a converted playerfile
against the original binary field by field, so item 2 is testable rather
than assertable.

---

## 5. Pluggable player-file format

### 5.1 How "pluggable" is implemented

**Compiled-in implementations behind an interface, selected by name at
runtime — not Go's `plugin` package.** `plugin` requires byte-identical
toolchain and dependency versions between host and plugin, doesn't work
with `CGO_ENABLED=0` static builds, and is effectively unusable in a
distroless container. A registry gives the same operational flexibility
(`--player-format=binary|ascii|...`) with none of that pain, and anyone
wanting a bespoke format vendors the repo and registers one more
implementation.

```go
// internal/persist/player/store.go
type Store interface {
    Load(ctx context.Context, name string) (*game.PlayerRecord, error)
    Save(ctx context.Context, rec *game.PlayerRecord) error
    Exists(ctx context.Context, name string) (bool, error)
    Delete(ctx context.Context, name string) error
    List(ctx context.Context) iter.Seq2[IndexEntry, error]
    Close() error
}

type Factory func(cfg Config) (Store, error)

func Register(name string, f Factory)   // called from each impl's init()
func Open(name string, cfg Config) (Store, error)
```

`game.PlayerRecord` is the **format-neutral canonical model**: explicit
widths, `time.Time` for timestamps, typed flag sets, no fixed-length char
arrays. Every format converts to and from it. This is the single most
important design decision in this section — the moment a format's
idiosyncrasy (a 20-byte name field, a 10-byte password field) leaks into
the game model, pluggability is over.

Lossiness is handled explicitly: `Store` implementations declare their
`Capabilities()` (max name length, whether they can round-trip long
titles, whether they support fields added after 2008). Saving a record a
format can't represent is a logged, non-silent truncation, and `dlctl`
refuses a migration that would lose data unless `--allow-lossy` is passed.

### 5.2 Default: `binary`

Byte-compatible with the current `struct char_file_u` in `data/etc/players`,
decoded per §4. This is the default so that a fresh checkout of the Go
server, pointed at an existing `data/`, boots and reads the real roster with
no migration step and no flags.

Two important sub-details:

- **The password field.** `MAX_PWD_LENGTH` is 10 and `interpreter.c:1462`
  does `strncmp(CRYPT(arg, GET_PASSWD(ch)), GET_PASSWD(ch), MAX_PWD_LENGTH)`
  with the *character's name* as salt — classic DES `crypt(3)`, truncated
  to 10 stored chars, which means only the first 8 characters of a password
  ever mattered and the salt is public. The Go port must verify legacy
  hashes to let existing characters log in, then **transparently rehash to
  bcrypt or argon2id on successful login** and store the result in a new
  field. Legacy DES verification is behind `--allow-legacy-passwords`
  (default on, with a startup warning naming the count of accounts still on
  it), so it can eventually be turned off. New characters never get a DES
  hash. This is a "fix the known bug" case under the fidelity decision, not
  a rules change.
- **The `spare0..7` / `spare17..21` fields** exist precisely because
  `char_file_u` can't grow without breaking compatibility. The Go binary
  store keeps them addressable so anything currently squatting in them
  (worth grepping for before writing the codec) survives round-trip.

### 5.3 Second implementation: `ascii`

ascii_pfiles 2.1 compatible, one text file per player under
`data/pfiles/<letter>/<name>`, plus `plr_index`.
`reference/tools/bin2ascii.c` already produces this layout and it
round-trips against genuine WipeMud-written files
(`docs/investigations/pfile-conversion.md`).

**`docs/investigations/ascii-pfile-format.md` is the specification to
implement against** — directory layout, `plr_index`, the `tag: value`
convention, the default-omission rule, every field, the three multi-line
block types (`Desc`/`Skil`/`Affs`) and their terminators, and the bitflag
encoding. The Go codec should be written from that document and validated
against the real files, not reverse-engineered again from
`welmar/pfiles/ascii_pfiles_2.1/full_src/db.c`.

One asymmetry to carry over deliberately: `Act`/`Aff`/`Pref` have two
valid representations. `sprintbits()` writes one letter per set bit in bit
order (`a`–`z` for bits 0–25, uppercase above), and `asciiflag_conv()`
reads that form *or* falls back to a plain decimal number when the field
is all digits. `bin2ascii` writes plain decimal, which is valid input but
not what a real `save_char()` emits. **The Go reader must accept both
forms; the Go writer should emit the letter form** so its output is
byte-comparable against genuine files. Note the digit-string trap in the
worked example — `Act : 128` is ambiguous between "decimal 128" (bit 7)
and a letter string, and is only unambiguous because pure-digit fields
take the decimal branch. That branch ordering is load-bearing and needs a
test.

Building this second implementation early is what proves the interface is
actually an interface and not a binary-shaped hole.

### 5.4 Migration

`dlctl pfile convert --from=binary --to=ascii --in=data/etc/players
--out=lib/pfiles` replaces `reference/tools/bin2ascii.c`, natively 64-bit,
no `-m32` required. Plus `dlctl pfile verify` (§4) and `dlctl pfile dump`
(replaces `reference/tools/pfiledump.c`).

### 5.5 The rest of the player-adjacent state

`data/plrobjs` (rent/crash files, `objsave.c`), `data/plralias`,
`data/house/` + `data/etc/hcontrol`, `data/etc/board.*`, and the mail system
each get the same treatment: a small interface, a default implementation
matching today's on-disk format, in `internal/persist/`. They are smaller
and less interesting than the playerfile but they're the reason a
"just swap the playerfile" plan doesn't actually let you move a MUD
anywhere — all of it has to move together.

---

## 6. Pluggable world format

Same pattern, read-mostly:

```go
// internal/persist/world/source.go
type Source interface {
    Zones(ctx context.Context) iter.Seq2[*game.ZoneDef, error]
    Rooms(ctx context.Context) iter.Seq2[*game.RoomDef, error]
    Mobiles(ctx context.Context) iter.Seq2[*game.MobDef, error]
    Objects(ctx context.Context) iter.Seq2[*game.ObjDef, error]
    Shops(ctx context.Context) iter.Seq2[*game.ShopDef, error]
    Close() error
}

// Optional — implement only if the format supports writeback (OLC).
type Sink interface {
    WriteZone(ctx context.Context, z *game.ZoneDef) error
    // ...
}
```

Splitting read from write matters: OasisOLC saves edited zones back to
disk, so any format that wants to support in-game building implements
`Sink` too, and a read-only format (say, one backed by a tarball or an
embedded FS) simply doesn't — and the OLC commands report "world source is
read-only" instead of half-writing something.

### 6.1 Default: `classic`

The format in `data/world/` today: `zone.lst` plus `wld/`, `mob/`, `obj/`,
`shp/`, `zon/`. Records are `#vnum`,
`~`-terminated strings, letter-encoded bitflags, `S`/`$` terminators, with
the mob `E`-format extension and dice notation (`5d10+550`).

This parser is the single fiddliest piece of the whole port and deserves
real attention:

- The C parser is forgiving in undocumented ways and the real world files
  exploit that. **Write the Go parser against `data/world/` itself, not
  against the format documentation in
  `reference/moderncserver/doc/building.txt`** — where they disagree, the
  live data wins, and the disagreement gets recorded.
- Every parse must produce identical results to the C loader. The parity
  harness for this is: boot both servers, dump the loaded world to a
  canonical JSON form from each, diff. Getting a byte-identical dump out of
  the C side means adding a small dump-and-exit path to
  `reference/moderncserver/src/` — worth the intrusion, and it's additive
  so it doesn't disturb the reference build.
- `scheck` (`reference/moderncserver/src/util/scheck`, the `-c`
  syntax-check mode) becomes `dlctl world lint`, usable in CI over
  `data/world/`.

### 6.2 Why this seam earns its place

Beyond format-swapping for its own sake: a `Source` interface makes it
possible to load one of the 1,184 dated nightly world backups in the
archive (`TODO.md` §4) directly, to load a zone from a tarball for testing,
or to serve an embedded copy of the world in a single-binary demo build.
Those are the concrete near-term uses; a database-backed world is the
speculative one and shouldn't drive the design.

---

## 7. Networking

`comm.c` is replaced rather than ported. The new `internal/net`:

- **Listeners are independent and independently configurable.** Each is a
  `Listener` producing `net.Conn`-alikes; the session layer doesn't know
  which one a player arrived on beyond a recorded transport label (useful
  in wizard `users` output and logs).
  - `--listen-telnet=:4000` — plaintext, **off by default** (see §0).
  - `--listen-telnets=:4443` with `--tls-cert`/`--tls-key`, or ACME via
    `--tls-acme-domain` using `golang.org/x/crypto/acme/autocert`.
  - `--listen-ws=:8080` — WebSocket, `nhooyr.io/websocket` or
    `gorilla/websocket`, carrying the same line protocol so a browser
    client is just another descriptor. Behind the same TLS config, or
    plaintext behind a reverse proxy with `--trust-proxy-headers`.
- **Telnet negotiation** as a proper state machine in
  `internal/net/negotiation`, not `comm.c`'s inline byte-stuffing:
  - **MSSP** — server status for MUD listing sites.
  - **MCCP2/MCCP3** — zlib stream compression, which is also a
    denial-of-service surface, so it gets an output-size cap.
  - **GMCP** — out-of-band JSON. This is what a modern client or web
    frontend uses for prompts, room data, and vitals without screen-
    scraping, and it's the main reason to bother with negotiation at all.
  - **MXP**, **NAWS** (window size, for the pager), **TTYPE**, **CHARSET**
    (the world files are Latin-1 era; declare and transcode rather than
    emitting invalid UTF-8), and **ECHO** suppression during password entry
    — the last one being a correctness fix over what a raw socket does.
- **Connection hygiene**: per-IP connection limits, a handshake timeout, a
  login-attempt rate limiter, and read deadlines. `data/etc/badsites` (the
  existing ban list) is honoured, loaded through its own small persistence
  interface. Reverse DNS of connecting hosts moves off the game goroutine
  with a timeout — in the C code it's a blocking call in the accept path.
- **Graceful shutdown**: `SIGTERM` (which is what `docker stop` sends)
  triggers save-all-players, flush, close listeners, exit non-zero only on
  failure. The C server's answer to this is `autorun` and a `.killscript`
  file; containers need it done properly.

Hot-reboot / copyover (preserving player connections across a restart by
passing file descriptors) is **explicitly out of scope for v1** but the
session layer should keep its state serializable so it stays possible.

---

## 8. The scripting seam

No interpreter ships in v1. What ships is the shape one would plug into:

```go
type Trigger interface {
    Matches(ev Event) bool
    Fire(ctx context.Context, ev Event, w World) (Result, error)
}
```

with `Event` covering the points DG Scripts hooks (command intercept,
speech, greet, entry, death, give, drop, timer, random) and `World` being a
deliberately narrow capability interface — the scripting API surface is
decided *here*, once, rather than accreting. Special procedures
(`spec_procs.c`, `castle.c`) are implemented as Go `Trigger`s from day one,
which means the interface is exercised by real code before any interpreter
exists, rather than being a speculative abstraction nobody has used.

Scripts get a time and step budget from the outset, even with no engine
behind them — a trigger that can hang the game goroutine is the failure
mode to design out, not to discover.

---

## 9. Configuration and operations

### 9.1 Configuration

Precedence: **flags > environment > config file > defaults**. One
definition per setting, no drift between the three sources — the flag set
is defined once and the env var names are derived (`--lib-dir` ↔
`DL_LIB_DIR`). `spf13/pflag` + a small env binder, or `kong`; avoid pulling
in a large framework for what is a few dozen settings.

Everything currently reachable through `comm.c`'s single-letter options
(`comm.c:243–300`) gets a long-form equivalent:

| Old | New | Env |
|---|---|---|
| `-d <dir>` | `--lib-dir` | `DL_LIB_DIR` |
| `-o <file>` | `--log-file` (`-` = stdout) | `DL_LOG_FILE` |
| `-m` | `--mini-mud` | `DL_MINI_MUD` |
| `-c` | `dlctl world lint` (own subcommand) | — |
| `-q` | `--skip-rent-check` | `DL_SKIP_RENT_CHECK` |
| `-r` | `--restrict` (no new players) | `DL_RESTRICT` |
| `-s` | `--no-specials` | `DL_NO_SPECIALS` |
| `<port>` | `--listen-*` | `DL_LISTEN_*` |

Plus the new ones: `--player-format`, `--player-dir`, `--world-format`,
`--world-dir`, the TLS settings, `--log-format=text|json`,
`--log-level`, `--metrics-addr`, `--pulse-interval` (defaults to the
historical 100ms), `--allow-legacy-passwords`, `--max-players`,
`--max-connections-per-ip`.

Much of `reference/moderncserver/src/config.c` is compile-time game tuning
(rent costs, level caps, OK/NOPERSON message strings, autosave behaviour).
That becomes a **config file** — TOML or YAML, `--config`, with every value
defaulting to today's `config.c` value so an empty file reproduces current
behaviour exactly. Hot-reload of the safe subset on `SIGHUP` is a
nice-to-have, not v1.

### 9.2 Observability

- `log/slog`, structured, JSON by default in containers. The C `mudlog()`
  levels (`NRM`/`BRF`/`CMP`) map onto slog levels plus a `wizvis` attribute
  controlling whether the line also goes to online immortals — keeping that
  behaviour matters, it's how gods actually watch the game.
- `SYSERR:` lines become real errors with fields, not formatted strings
  someone greps for.
- Prometheus metrics on `--metrics-addr`: players online, connections by
  transport, pulse duration histogram (a pulse taking >100ms is the single
  most useful health signal a MUD has), commands/sec, save latency, zone
  reset duration, goroutine count.
- `/healthz` (process alive) and `/readyz` (world loaded, listeners
  accepting) on the same address, for container orchestration.
- `pprof` on a separate, non-public `--debug-addr`, off by default.

### 9.3 Containers

- Multi-stage `Dockerfile`: build with the full toolchain,
  `CGO_ENABLED=0`, ship on `gcr.io/distroless/static` or `scratch`.
  Non-root user. No shell in the runtime image — which is precisely why
  `autorun`'s "restart me in a loop" model is replaced by the container
  runtime's restart policy.
- `data/` mounted as a volume, since it's mutable state (players, houses,
  boards, mail, and OLC-edited world files). The world *content* can
  optionally be baked into the image via `embed.FS` and a
  `--world-format=embedded` source for immutable deployments.
- `docker-compose.yml` for local dev: server + a mounted `data/` + optional
  Prometheus/Grafana.
- `SIGTERM` handling per §7 so `docker stop` saves the game rather than
  losing up to a minute of play (`PULSE_AUTOSAVE` is 60s).
- Reproducible builds, version/commit stamped via `-ldflags`, exposed in
  the `version` command and in MSSP output.

---

## 10. Phasing

Each phase ends with something runnable. The C server stays the live
reference throughout; nothing is deleted from
`reference/moderncserver/src/` until the Go server has been playing for a
while.

**Phase 0 — Foundations. ✅ Done.** Go module, layout, CI (build, vet,
`-race` tests, lint, container build). `config` package with the full
flag/env surface. `obs` package. Nothing game-related. *Done when: `dlmud
--help` prints the complete option set and the container builds.*

Both criteria met. What landed, and the three decisions taken while
building it:

- **`internal/config`** — every setting declared once, with the environment
  variable name *derived* from the flag name rather than written out
  separately, so the two cannot drift. The `-d`/`-o`/`-m`/`-q`/`-r`/`-s`
  aliases write through to the same targets as their long forms rather than
  being separate settings, and a bare port argument is rejected with a
  message naming `--listen-telnet`. Bare defaults are deliberately *invalid*
  — the TLS listener is on by default and has no certificate — so an
  unconfigured server fails loudly instead of starting somewhere unreachable.
- **`internal/obs`** — `log/slog` with text/JSON handlers, a Prometheus
  registry, and `/healthz` + `/readyz` (liveness independent of readiness).
  The pulse histogram's buckets are derived from the configured pulse
  interval rather than fixed, since what matters is the ratio to the budget.
  `mudlog()`'s in-game echo level survives as the `wizvis` attribute;
  nothing consumes it until the session layer exists.
- **`cmd/dlmud`** — boots, warns, reports ready, handles SIGTERM. Verified
  end to end in the container: `docker stop` shuts down cleanly in ~0.2s
  with exit 0.
- **`cmd/dlctl`** — command structure, with unimplemented subcommands
  reporting the phase that implements them rather than failing vaguely.
- **Container** — 13MB distroless/static, non-root, `CGO_ENABLED=0`, with
  `LICENSE` copied in per §12.1.

Three decisions made during the build, all revisable:

1. **Standard-library `flag`, not `pflag` or `kong`.** §9.1 left this open.
   Stdlib handles `--long-form` fine, and the env derivation is ~20 lines,
   so a dependency bought nothing. The cost is that `--flag` and `-flag` are
   equivalent and short aliases appear in `--help` alongside long forms.
2. **No config-file layer yet.** §9.1's config file is for the game tuning
   currently in `reference/moderncserver/src/config.c` — rent costs, level
   caps, message strings — and none of those values exist yet. The
   precedence chain has the slot; filling it before there is anything to
   put in it would have meant picking a format and a dependency for no
   benefit. Lands with the values it configures.
3. **`--log-level=debug` implies `AddSource`.** Small, and easy to reverse
   if it turns out to be noisy.

**Phase 1 — World loading. ✅ Done.** `game` type definitions with explicit
widths. `persist/world` interface + `classic` implementation. `dlctl world
lint` and `dlctl world dump`. Parity harness against a dump-and-exit path
added to the C server. *Done when: the Go loader reproduces the C loader's
view of all 47 zones exactly.*

**Met, and checked rather than argued.** `scripts/world-parity.sh` builds
both servers, has each dump the world it loaded, and diffs the results:

```
    2981 rooms, 944 mobiles, 1199 objects, 47 zones, 77 shops
    identical
```

Zero differing fields across all 5,248 records. It runs in CI, so it stays
true.

The C side is `reference/moderncserver/src/worlddump.c` plus a `-J <file>`
option, which loads the world exactly as a real boot does — including
`renum_world()` and `renum_zone_table()`, whose effects are the interesting
part — then writes JSON and exits without opening a socket or touching
player data. It is marked `<DoC>` like every other local change to the C
tree.

Three corrections to what this document previously said:

- **47 zones, not 55.** The earlier figure counted files in `data/world/zon/`,
  including `index` and `index.mini`. Six `.zon` files are not listed in the
  index and the C server never opens them, so 47 is what actually loads.
- **The C server's boot log over-reports.** It prints 2988 rooms and 1200
  objects, but those come from `count_hash_records()`, which counts every
  line beginning with `#` to size a malloc — including `#` lines inside
  descriptions. `data/world` has seven of those in room files (ASCII-art
  signs in `wld/54.wld` and `wld/64.wld`) and one in `obj/142.obj`, so the C
  server allocates 2988 slots and fills 2981.
- **§13.1 (Latin-1 vs UTF-8) is answered for the loader.** It treats text as
  opaque bytes and neither validates nor transcodes, so `wld/90.wld`'s
  CP1252 apostrophes survive a round trip. What to *present* to a player
  remains a Phase 3 question for the protocol layer.

### What the parity harness caught

Building it was worth it on the first run. Three real bugs, two of them in
the Go loader, none of which any hand-written test had found:

1. **Trailing carriage returns were being eaten.** `fread_string` overwrites
   only a line's *final* character with CRLF, so CRs already in the line
   survive into the string. Several files have them — `obj/0.obj`'s bug
   object carries fifteen — and the C server shows them all. The Go reader
   trimmed them, differing from the C server on every such line.
2. **`MOB_ISNPC` was missing.** `parse_mobile()` force-sets it on every
   mobile regardless of the file. 560 of 944 mobs differed.
3. **Non-UTF-8 bytes did not survive the dump.** `encoding/json` replaces
   every invalid byte with U+FFFD, so two different corrupt bytes dumped
   identically — a parity format that can hide the differences it exists to
   find. Strings now escape byte by byte to pure ASCII, which the C side
   reproduces trivially.

Two fields are excluded from the comparison, via `dlctl world dump
--parity`, because the C server does not retain them: whether a mob used the
enhanced (`E`) format, which `parse_enhanced_mob()` consumes without
recording, and the espec key/value lines, which `interpret_espec()` folds
into ordinary fields and discards. The Go loader keeps both because they are
useful; comparing them could only ever produce noise.

**Phase 2 — Player loading. 🟡 In progress.** `persist/player` interface,
`binary` implementation (§4/§5.2), `ascii` implementation, `dlctl pfile
convert|verify|dump`. Password verification and rehash-on-login logic,
unit-tested against known vectors.

**The stated criterion was wrong and is corrected here.** It said "all 108
archived records round-trip binary→canonical→binary byte-identically". That
is impossible, and not because of any defect in a reader: `save_char()`
fwrites an *uninitialised stack local* (`db.c:2204`) after filling it field
by field with `strcpy`, so every record contains bytes that are stack
residue rather than data — the padding between fields, and everything after
the terminating NUL of each fixed-width string. Two saves of the same
character produce different files. No reader can reconstruct those bytes and
nothing is lost by not reconstructing them.

*Done when: every archived record survives decode→encode with every
**significant** byte unchanged (`significantBytes()` defines which those
are), decode→encode→decode is identical as a record, and binary→ascii
matches `bin2ascii` output.*

**Built so far:**

- **The layout engine.** The file format *is* the C struct's memory layout,
  so the offsets are computed from a declaration of the struct plus a data
  model — how wide `long`, `time_t` and pointers are — rather than written
  out as constants. The difference between the 32-bit format the archived
  data is in and the 64-bit format a modern rebuild produces is three
  integers, which is §4 reduced to one place.
- **Checked against a compiler, not asserted.** `reference/tools/
  pfilelayout.c` prints the offsets gcc actually chose, and a test requires
  the engine to reproduce them field for field.
- **Checked against real data that cannot be committed.**
  `reference/tools/pfilegen.c` writes records using the same struct, with
  every field a documented function of the record index, so the decoder is
  verified against C's own idea of the layout. It memsets each record to
  0xAB first, so a reader that accidentally depended on padding being zero
  would fail.
- **The 32-bit record is 1288 bytes**, and 108 of those is 139,104 bytes —
  which is the 139KB `docs/investigations/circlemud-archive-report.md`
  independently recorded for the real file. That is the only check on the
  32-bit layout available without the archive, and it holds.
- `player.Store` + registry, the `binary` store (atomic saves, 0600, in-place
  updates because positions are referenced elsewhere), and `dlctl pfile
  dump|verify`.

**Still outstanding:** the `ascii` format, `dlctl pfile convert`, and the
password work — DES verification for the existing roster plus a modern
scheme to upgrade to. The binary format cannot store a modern hash at all
(its password field is eleven bytes), so moving off it is a *prerequisite*
for better password hashing rather than an independent improvement, which
was not obvious before building this.

**A note on verification.** The 32-bit checks skip on a machine without
32-bit libc headers, which is most of them. CI installs `gcc-multilib` and
fails if those checks skipped, so the layout the real data is in is verified
on every change even though it cannot be verified locally.

**Phase 3 — Server skeleton.** Pulse loop, session lifecycle, listeners,
negotiation, the login `nanny` state machine, and enough commands to look
around and move (`look`, `north`, `who`, `quit`). *Done when: a real player
can log in with an archived character over TLS and walk from the Temple of
Midgaard to New Thalos.*

**Phase 4 — Rules core.** Combat, magic, skills, classes including the
remort bitmask, affects, position/regen, death and corpses, zone resets,
mobile activity. The largest phase; the deviations log from the fidelity
decision starts filling up here. *Done when: a character can level.*

**Phase 5 — The rest of the game.** All remaining `act.*` commands,
shops, houses, boards, mail, aliases, socials, object save/rent.
Parallelisable across many small independent pieces.

**Phase 6 — Building tools.** OasisOLC equivalent, `Sink` writeback,
the `gen*` layer. Deferrable — offline editing plus a reboot works
meanwhile.

**Phase 7 — Cutover.** Shadow-run both servers against copies of the same
`data/`, compare. Then run the Go server as primary, keep the C tree as
reference. Retire `autorun`/`automaint`/`configure`.

**Later (explicitly not v1):** scripting engine behind the §8 seam,
copyover/hot-reboot, web client, additional persistence backends, the
WipeMud race system (`TODO.md` §2).

---

## 11. Testing and parity

- **Unit tests** for every format codec, with the real `data/` data as
  fixtures where it isn't player data, and synthesised fixtures where it
  is. Fuzz the world parser and the binary pfile decoder — both consume
  untrusted-ish input and both are exactly the kind of code where a
  malformed length field becomes a panic.
- **Golden-file tests** for command output. Player-visible text should
  match the C server's byte for byte; that is the cheapest possible
  regression net for a faithful port and it catches an enormous number of
  subtle mistakes.
- **A scripted-session harness**: a list of commands in, expected transcript
  out, run against both servers. This is the parity oracle for phases 3–5.
- **Property tests** on the numeric core — combat damage, experience,
  saving throws — asserting no overflow and no negative-where-impossible
  across the full input range, which is where the 64-bit work either holds
  or doesn't.
- **A deviations log** (`docs/deviations.md`, created in Phase 1): every
  intentional difference from the C behaviour, with the C line reference,
  what it did, what Go does, and why. Under the "fix known bugs" fidelity
  decision this file is the deliverable that keeps "fixed a bug"
  distinguishable from "accidentally changed the game". It belongs in
  `docs/` proper rather than here — it is a record of what the running
  server does, not a plan.

---

## 12. Licensing — a hard constraint, not an open question

**The Go port cannot be LGPL, GPL, MIT, or anything else of our choosing.**
It inherits the CircleMUD and DikuMUD licenses, and this constrains the
port's design in ways worth stating up front rather than discovering late.

`reference/moderncserver/doc/license.doc` is the CircleMUD License (Jeremy
Elson, 1994–2001, Johns Hopkins) plus the original DikuMUD license appended
below it. There is no GPL or LGPL text anywhere in this tree — `grep -rniE
"LGPL|GNU General Public|lesser general public"` across the whole checkout
returns nothing. `reference/moderncserver/src/licheck` still gates the C
build on accepting it.

The three requirements are non-commercial use, credit, and DikuMUD
compliance. The credit terms are specific and mechanical
(`reference/moderncserver/doc/license.doc:80–105`):

- `data/text/credits` must be preserved verbatim — additions allowed,
  removals and edits not — and displayed by the `credits` command.
- The `CIRCLEMUD` help entry must be intact and shown in full by
  `help circlemud` (currently `data/text/help/info.hlp:380`).
- The **login sequence** must name the DikuMUD and CircleMUD creators —
  defined as everything a player sees between connecting and playing.
- The license ships AS IS with any distribution, including derivative
  works.
- Copyright/authorship notices in source files must not be removed or
  changed.

And, directly on point for this project
(`reference/moderncserver/doc/license.doc:101–105`):

> Claims that any of the above requirements are inapplicable to a
> particular MUD for reasons such as "our MUD is totally rewritten" or
> similar are completely invalid. If you can write a MUD completely from
> scratch then you are encouraged to do so by all means, but use of any
> part of the CircleMUD or DikuMUD source code requires that their
> respective licenses be followed, including the crediting requirements.

A faithful port that reproduces CircleMUD's mechanics, file formats and
world data is squarely a derivative work, and the license anticipates the
rewrite argument explicitly. Non-commercial also means no donations, no
paid hosting, no "support the server" tip jar — worth knowing before
anyone plans how to pay for hosting under §9.

**The good news:** the tree is compliant, and that is now checked rather
than asserted. `data/text/greetings` carries both sets of creators above the
name prompt, `data/text/credits` is the stock text with Disgracelands'
additions after it, and the `CIRCLEMUD` help entry is intact.

`scripts/license-check.sh` runs in CI and verifies the five requirements
that can be verified from the tree: that `LICENSE` still ends with
`doc/license.doc` byte for byte, that no stock C file's leading comment
block differs from the pre-upgrade baseline import (78 files), that every
file written for this project carries its own notice, that the credit files
are present and name the DikuMUD creators, and that the login-sequence file
does too. The in-game half — that `credits` and `help circlemud` actually
display those files, on every transport — cannot be checked until Phase 3
implements the commands, and gets its own test then.

**Copyright and its limits.** Copyright in the Disgracelands-specific
material, the original 2001–2008 server and this revival alike, is Dave
O'Connor's. That ownership does not loosen anything above: a derivative work
cannot be relicensed on terms more permissive than what it derives from, so
the CircleMUD and DikuMUD licenses govern the Go port as fully as they
govern the C tree. `LICENSE` says so in a preamble above a marker line, with
the upstream license reproduced unmodified below it — prepended rather than
interleaved, because the license requires itself to be distributed AS IS.

**What this means concretely for the port:**

1. `LICENSE` in the Go tree carries `reference/moderncserver/doc/license.doc`
   unchanged, below a preamble that names this project's copyright, states
   that it is a derivative work, and spells out the inherited requirements.
   Nothing in the preamble narrows the license below it, and the marker line
   between them says so. Any `go.mod`-adjacent licensing metadata says the
   same.
2. The `credits` and `help circlemud` commands are **not optional
   features** to be deferred to a late phase — they are license
   compliance, and belong in Phase 3 alongside `look` and `who`.
3. The login sequence rendering (§7) must emit `data/text/greetings` before
   the name prompt on **every** transport — telnet, TLS and WebSocket
   alike. A web client that renders its own pretty splash screen and skips
   the greeting file is a license violation, not a UI choice. Worth a test
   in the parity harness.
4. `reference/moderncserver/src/licheck`'s build-time acceptance gate has
   no natural equivalent in a `go build`, and shouldn't be reinvented as
   one. The requirement it enforces is *distribution* of the license, which
   the `LICENSE` file satisfies.
5. Third-party add-ons carry their own terms on top: OasisOLC and (if ever
   pulled in) DG Scripts, context-sensitive help, and ascii_pfiles each
   have author credits in their headers — see the table in
   `docs/investigations/non-stock-features.md`. Per requirement 5 above,
   those headers carry across to the corresponding Go files. No Go file
   derives from one yet; the ascii player format (§5.3) will be the first,
   and it must credit ascii_pfiles in its header.
6. Publishing the repo publicly is fine and always was; it's *charging for
   it or taking donations for it* that isn't.
7. Every source file written for this project — Go, the C files added to
   the reference tree, the shell scripts — opens with a five-line notice
   naming this copyright, the CircleMUD and DikuMUD copyrights, and
   `LICENSE`. New files must do the same; `scripts/license-check.sh` fails
   the build otherwise. The notice sits above the package doc comment,
   separated by a blank line, so `go doc` output is unaffected.

If there's a specific relicensing announcement making some CircleMUD
release LGPL, that would change all of this — but it would need to be a
source that supersedes the license file shipped in this tree, and this
tree is the one being ported.

---

## 13. Open questions

Not blocking the plan, but they need answers before or during the phases
they touch:

1. **Latin-1 vs UTF-8.** The world files, help text and player descriptions
   are 8-bit-clean but not UTF-8. Does the Go server transcode on load
   (clean, but changes the on-disk world if OLC ever writes back), on
   output per-client via CHARSET (faithful, more complex), or neither?
   (Phase 1.)
2. **Does `data/` stay the on-disk contract**, or does the Go server get its
   own data directory layout with a migration? Staying compatible is what
   makes side-by-side parity testing work, so this plan assumes it stays —
   but it does constrain things. (Phase 1.)
3. **How faithful does OLC need to be** — a port of OasisOLC's exact menu
   trees, or a modern equivalent that produces the same files? (Phase 6.)
4. **Is a web client actually wanted**, or is WebSocket support just about
   keeping the option open? Affects how much goes into GMCP, and note the
   greeting-file requirement in §12.3. (Phase 3.)
5. **Password reset path for the 2001–2008 roster.** Those DES hashes are
   8-effective-characters with a public salt; anyone who had an account
   then may not be reachable now. Force-reset on first login, or accept the
   legacy hash and upgrade transparently? This plan assumes the latter,
   which is friendlier and weaker. (Phase 2.)

---

## Related documents

Operator documentation for what has actually been built lives in `docs/`
itself — `docs/configuration.md` and `docs/operations.md`.

Background this plan draws on, all under `docs/investigations/`:

- `pfile-conversion.md` — the binary→ascii conversion tools and what was
  verified; the groundwork §5 builds on.
- `ascii-pfile-format.md` — the field-by-field ascii format spec; the
  implementation reference for §5.3.
- `non-stock-features.md` — the definitive list of what a "faithful" port
  has to reproduce.
- `circlemud-archive-report.md` — why this tree and not the other one.

And outside `docs/`:

- `TODO.md` — what's left in the C tree; items 1, 3 and 5 are largely
  superseded by this plan, items 2 and 4 are not.
- `BUILDING.md` — both builds, C and Go.
- `reference/moderncserver/doc/license.doc` — the CircleMUD + DikuMUD
  licenses the port inherits; see §12.
