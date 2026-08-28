# Porting Disgracelands to Go

A plan for reimplementing the Disgracelands engine in Go: 64-bit safe,
pluggable player- and world-file formats, and packaged as a normal modern
service (flags, env vars, structured logs, containers) rather than a
2002-era autoconf tree driven by `autorun`.

This is a design/sequencing document. **Phases 0–5 (§10) are built** — every
slice of Phase 5 included — with a tail of small commands outside the slices
listed under it. Phases 6 and 7 are still a plan.
Each built phase carries a retrospective in §10 saying what actually landed
and where it diverged from what was planned. See `BUILDING.md` for how to
build and run what exists.

**As of 2026-08-23, the port is playable, and §0's fidelity decision has a
second half ("Fidelity, phase two") that narrows what it still governs.**
Read that row before assuming everything below still describes a strict
port — the phase write-ups in §10 are a historical record of how fidelity
was reached and stay accurate as history; they are not a live constraint on
work that starts from here.

---

## 0. Decisions already taken

These were settled up front and the rest of the plan assumes them:

| Question | Decision |
|---|---|
| **Fidelity** | Faithful core, known bugs fixed. Same game feel, same mechanics (remort bitmask, Paladin alignment, the balance tweaks in `docs/investigations/non-stock-features.md`); the `sprintf`-overlap class of bugs (`reference/moderncserver/README.md`'s "Known problems", never audited in the C and now deliberately not going to be) and integer-width bugs get fixed as they're encountered, each deviation recorded. Deliberate rules changes are a separate, later conversation. |
| **Repo layout** | Same repo, new top-level Go tree. The C tree in `reference/moderncserver/src/` stays buildable and authoritative for the whole port — it is the reference implementation and the parity oracle. |
| **Scripting** | Design the seam, defer the engine. Trigger/event interfaces get defined so DG Scripts, Lua, or anything else can drop in later; v1 ships no interpreter. The tree that was actually played has no DG Scripts (`docs/investigations/non-stock-features.md`), so nothing regresses. |
| **Protocols** | TLS-wrapped telnet, WebSocket, and telnet option negotiation (MSSP/MCCP/GMCP/MXP). |

**Decisions taken since, each reversing or settling something above:**

| Question | Decision |
|---|---|
| **Text encoding** | **UTF-8 throughout.** The server works in UTF-8 and nothing else. Old CP1252 data is converted once by `dlctl convert`, not decoded per-connection forever — the same principle the player formats follow (§5.2). The world loader still reads bytes transparently, because transcoding at load time would change what a writer later emits; `dlctl lint --type=world` reports anything not yet converted. Answers what was §13.1. |
| **Web client** | **Wanted, not merely kept open.** A self-hosted optional web front end is a real intention, so GMCP and the WebSocket transport get built properly rather than minimally. The reasoning is worth recording: telnet clients are ageing out, and a MUD whose only door is TinyFugue has a shrinking number of people who can walk through it. Answers what was §13.4. |
| **Legacy passwords** | **Accepted and upgraded on login; nobody is reset.** Answers what was §13.5. |
| **Fidelity, restated** | **As faithful to the patched C server as possible.** The row above says "faithful core"; this sharpens it. Where a choice exists between what the C server does and what a modern design would prefer, the C server wins, and a deviation needs a reason written down next to it. The existing exceptions stand — bugs are fixed, integer widths are made honest, credentials are modernised — but each is a deliberate, recorded departure rather than licence to redesign. When in doubt, read the C and do that. |
| **Old data directories** | **Converted, not carried.** `dlctl convert` takes an original CircleMUD `lib/` and produces a directory the server runs on. It refuses to guess: the binary formats it does not yet understand are copied byte for byte and reported, because a byte-level transcode of a struct dump corrupts it twice over. |
| **Fidelity, phase two (2026-08-23)** | **Narrowed, not dropped.** Phases 0–5 reached byte-for-byte behavioural agreement with the C server, and that work is not being revisited on fidelity grounds alone — the C oracles, table re-parsing and layout tools that got it there keep protecting it, and `docs/deviations.md`/`docs/weirdnumbers.md` stay the record of every departure made getting here. The port is playable now, and from this point new work is free to diverge from the C server to modernise the implementation — architecture, dependencies, protocols, tooling, roughly the decade and a half of how server software has moved on since this stack was designed — without needing a reason recorded anywhere. Two things stay fixed, not up for modernisation: **compatibility** (the on-disk formats, `--lib-dir` contents and archived credentials this repo already reads and writes) and **gameplay** (the mechanics and balance a returning player would recognise — the to-hit and damage tables, remort, alignment, the tweaks in `docs/investigations/non-stock-features.md`). A change that touches either of those is still a deviation, in `docs/deviations.md`, with its reasoning, exactly as before; a change that only touches implementation is not. |

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
  dlctl/            Offline tooling: format conversion, dump --type=pfile,
                    lint --type=world (`scheck`), autowiz, mudpasswd, listrent.
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

A `dlctl verify --type=pfile` subcommand cross-checks a converted playerfile
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

### 5.2 The server runs on `ascii`; `binary` is a conversion format

**Superseded decision.** This section originally made `binary` the default,
on the reasoning that pointing the Go server at an existing `data/` should
need no migration and no flags. That is a real convenience and it is not
worth what it costs.

The binary format's password field is **eleven bytes**. Not "awkwardly
small" — it cannot hold any modern hash at all, so a server running on it
can never store anything better than the DES `crypt(3)` it inherited.
Everything else in the record is fixed-width too: a twenty-byte name, an
80-byte title, 32 affect slots, 32-bit timestamps that overflow in 2038.
Keeping it as the live format would mean carrying every one of those limits
into a server being written in 2026 in order to save one conversion command.

So:

- **The server runs on `ascii` or better.** `--player-format=binary` is
  rejected at startup, with the conversion command in the error message.
- **The tooling reads and writes both.** Conversion needs both directions:
  going back is how you compare a converted roster against the C server, and
  how you undo a migration that turned out to be premature.
- **`binary` is still a full implementation**, not a stub. It is verified
  against a C compiler in both data models (§4), because reading the
  archived roster correctly is the whole point of it.

### 5.3 `binary` — the archived format

Byte-compatible with `struct char_file_u`, decoded per §4. Its job is to
read what the C server wrote, and to write it back for comparison.

Two details of it are load-bearing:

- **The password.** See §5.3.1 below — it has enough sharp edges to be worth
  its own section.
- **The `spare` slots** exist because `char_file_u` cannot grow without
  breaking compatibility, and the C server's own comments tell people to use
  them. Disgracelands did: the remort vector lives in one. They are carried
  through conversion rather than assumed to be junk.

#### 5.3.1 The stored password is a 10-character prefix

This is the single most implementation-critical detail in the format, and
getting it wrong fails silently in the worst possible way: every character on
the archived roster would be unable to log in, with a correct password, and
the server would report nothing but "wrong password".

The C server uses classic DES `crypt(3)`, which produces a **13-character**
hash. `MAX_PWD_LENGTH` is 10 and the field is 11 bytes, so
`interpreter.c:1532` stores only part of it:

```c
strncpy(GET_PASSWD(...), CRYPT(arg, GET_PC_NAME(...)), MAX_PWD_LENGTH);
*(GET_PASSWD(...) + MAX_PWD_LENGTH) = '\0';
```

Ten characters, then an explicit NUL. (`strncpy` would not terminate a
13-character source itself; the line after it does, so the field is properly
terminated and there is no garbage byte to strip — worth stating because the
obvious suspicion is wrong.)

Verification then compares the same ten, `interpreter.c:1462`:

```c
strncmp(CRYPT(arg, GET_PASSWD(ch)), GET_PASSWD(ch), MAX_PWD_LENGTH)
```

**So a Go implementation must compute the full 13-character DES hash and
compare only its first 10 characters.** Comparing all 13 rejects every
correct password on the entire roster.

Three further consequences:

- **The salt is the first two characters of the character's name.** Setting
  a password passes the name as the salt argument; checking one passes the
  stored hash, whose first two characters are that same salt. Both are the
  standard `crypt(3)` idiom, and both mean the salt is public and derivable
  from the character's name alone.
- **Only the first eight characters of a password ever mattered**, because
  that is what DES `crypt(3)` uses. A player's twenty-character passphrase
  was always eight characters of security.
- **Truncating the hash to 10 characters loses roughly 3 characters of
  digest**, which weakens it further — distinct passwords that collide in
  the first 10 characters of their hash are interchangeable at the login
  prompt. This is not a reason to panic about a game from 2002; it is a
  reason not to keep these hashes a moment longer than migration requires.

Conversion preserves the stored value exactly — verified, not assumed: a
binary record's password survives binary→ascii byte for byte, and comes back
classified as the legacy scheme. Nothing is lost in migration; what is stored
is simply already only ten characters.

### 5.4 `ascii` — what the server runs on

ascii_pfiles 2.1 compatible: one text file per player under
`data/pfiles/<letter>/<name>`, plus `plr_index`.
`docs/investigations/ascii-pfile-format.md` specifies it field by field, and
the implementation is written against that document rather than against the
reference C a second time.

No fixed-width fields, so no limits on names, titles, descriptions, affects
or skill numbers; timestamps are decimal text, so 2038 is not an event. It
stores a scheme-prefixed credential (`argon2id:$argon2id$...`), and a bare
unprefixed hash is DES by definition, since that is all the format ever held
before — a DES hash cannot contain a colon, so the two cannot be confused.

The index is rebuilt wholesale on every write rather than maintained
incrementally. It is slower and it cannot drift, and an index disagreeing
with the files is a class of bug that then simply does not arise.

### 5.5 Migration

```
dlctl convert --type=pfile --from-format=binary --from-dir=data \
                            --to-format=ascii    --to-dir=data
```

Replaces `reference/tools/bin2ascii.c`, and needs no 32-bit build: the
32-bit layout is a parameter of the decoder rather than a property of the
binary doing the decoding.

It refuses rather than truncates. Before writing each character it asks the
destination format what it can hold and reports anything that would not fit,
because a truncated name is a different character and finding that out after
the conversion is worse than not converting. `--dry-run` reports without
writing; `--force` overwrites characters already present.

Also `dlctl verify --type=pfile` (§4) and `dlctl dump --type=pfile`.

`dlctl passwd --type=pfile <name>` sets a character's password offline. Nothing in
the C or in the game can: `set` has no password field and the menu only ever
lets the owner change their own, which leaves an archived character whose
password nobody remembers with no way back in. It applies the same rule the
menu does and refuses any format that cannot hold an argon2id hash. See
`docs/deviations.md`.

### 5.6 The rest of the player-adjacent state ✅

`data/plrobjs` (rent/crash files, `objsave.c`), `data/house/` +
`data/etc/hcontrol`, `data/etc/board.*`, and the mail system each got the
same treatment: a small interface, a default implementation matching
today's on-disk format, in `internal/persist/`. They are smaller and less
interesting than the playerfile but they're the reason a "just swap the
playerfile" plan doesn't actually let you move a MUD anywhere — all of it
has to move together. See §5.7 for rent/crash (folded into the playerfile
itself, not a separate interface) and §5.8 for bans/boards/mail/houses.

`data/plralias` did not get this treatment, and was never going to: no
archived alias data exists anywhere to have a format for, and §5.7
explains where the `alias` command's own persistence ended up instead
(folded into the same file the roster is in, for both `ascii` and
`yaml`).

### 5.7 `yaml` — a second `Store`/`ObjectStore`, landed during Phase 5

`docs/design/data-format.md` §8's "one player, one file" is now built:
`yaml` (`internal/persist/player/yaml/`) is a second registered
`player.Store`, and also a `player.ObjectStore` — folding §5.6's `plrobjs`
into the same file as the roster rather than needing a second interface
implementation, since `ObjectStore` was already separate from `Store` and
nothing about either needed to change shape to let one type serve both.
`--player-format=yaml` boots the server (`cmd/dlmud/main.go` picks the
opened `Store` as the `ObjectStore` too when it satisfies that interface,
falling back to `binary` — §5.6's still-not-pluggable rent files —
otherwise), and `dlctl import --type=pfile`/`fmt` convert and canonicalise it.
`ascii` stays the default.

Landing this also closed two real gaps rather than only adding a format:
the `alias` command (§10's "what is not in it" list, `do_alias`/
`perform_alias`, `interpreter.c:693-845`) had never been ported at all —
there is no archived `plralias` data anywhere to have ported instead, so
this is new functionality riding the format rather than data recovered by
it — and rent/crash files stop discarding containment when running on
`yaml`, a deliberate, explicitly-scoped deviation
(`docs/deviations.md`, "Renting empties your bags and strips your body")
rather than a side effect of the format landing: `binary`/`ascii` are
unchanged, byte for byte, and a test proves it stays that way. See
`data-format.md`'s §11 table and its updated §8/§12 for exactly what
shipped against what that section originally sketched.

### 5.8 Bans, boards, mail, houses — pluggable, `yaml` added, landed during Phase 5

`docs/design/data-format.md` §9's four small struct-dump-or-block-file
formats — `internal/persist/bans`, `boards`, `mail`, `houses` — are now
each what §5.6 always meant by "a small interface": a `Store`
(`Register`/`Open`/`Formats`, the same registry shape as `world`/`player`),
the existing implementation moved unchanged into its own `classic`
subpackage, and a `yaml` implementation added beside it. One flag,
`--state-format`, selects for all four together, since in practice they
always move as one directory. `dlctl import --type=state`/`fmt` convert and
canonicalise.

Houses needed one real design decision the others didn't: its object
files (the same `obj_file_elem` record the rent files use) were passed
around as raw bytes before, encoded and decoded by `internal/server/
houses.go` itself. That moved *into* each format — `houses.Store`'s
`LoadObjects`/`SaveObjects` now speak `[]player.StoredObject` directly, so
`yaml` can build its contents from the same `player/yaml` schema §8's
players work already built, rather than reinventing it or working around
raw bytes it cannot format as YAML. See `data-format.md`'s §9 and §11
step 6a for exactly what shipped, including the one genuine behavioural
difference `yaml` has from `classic` there (an orphaned house's
contents do not survive a control-record removal the way a classic
`<vnum>.house` file quietly does, because there is no longer a separate
file for an orphan to hide in).

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
  syntax-check mode) becomes `dlctl lint --type=world`, usable in CI over
  `data/world/`.

### 6.2 Why this seam earns its place

Beyond format-swapping for its own sake: a `Source` interface makes it
possible to load one of the 1,184 dated nightly world backups in the
archive (`TODO.md` §3) directly, to load a zone from a tarball for testing,
or to serve an embedded copy of the world in a single-binary demo build.
Those are the concrete near-term uses; a database-backed world is the
speculative one and shouldn't drive the design.

### 6.3 `yaml` — a second `Source`/`Sink`, landed during Phase 5

`docs/design/data-format.md` designs a YAML-over-JSON replacement for
`classic` and the rest of `lib/`'s formats; its own §11 argued the world
half should land before Phase 6's OLC writeback rather than after, so that
`Sink` gets implemented once rather than twice. That happened: `yaml`
(`internal/persist/world/yaml/`) is a second registered `world.Source`/
`world.Sink`, `--world-format=yaml` boots the server, and `dlctl import
--type=world`/`fmt` convert and canonicalise it. `classic` stays the default and
the parity oracle; players (§5.7/§5.8 above) and step 6a's four small
state formats have since landed too — the rest of `lib/`'s formats
(data-format.md §11 step 6b) are not attempted. See that document's §11
table for exactly
what landed and its §12 for what the round-trip fuzz testing found that
this plan didn't anticipate — CRLF, and two real limits in the YAML library
used to write it.

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
- **Telnet negotiation** as a proper state machine — built, in
  `internal/telnet` — rather than `comm.c`'s inline byte-stuffing. ECHO,
  CHARSET, GMCP, NAWS, TTYPE and suppress-go-ahead are implemented and
  answered by RFC 1143's Q method; MSSP, MCCP2/3 and MXP are not:
  - **MSSP** — server status for MUD listing sites. *(not built)*
  - **MCCP2/MCCP3** — zlib stream compression, which is also a
    denial-of-service surface, so it gets an output-size cap. *(not built)*
  - **GMCP** — out-of-band JSON. This is what a modern client or web
    frontend uses for prompts, room data, and vitals without screen-
    scraping, and it's the main reason to bother with negotiation at all.
  - **MXP** *(not built)*, **NAWS** (window size, for the pager), **TTYPE**,
    **CHARSET** (the world files are Latin-1 era; declare and transcode
    rather than emitting invalid UTF-8), and **ECHO** suppression during
    password entry — the last one being a correctness fix over what a raw
    socket does.
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

**What landed, and where.** There is no `internal/net`. The listeners
are `internal/server/listen.go` — `Listener` is a `net.Listener` plus
the transport label this section asked for, `ListenTelnet`/`ListenTLS`
construct the two that exist, and `Accept` is the one accept loop all of
them go through (which is also what makes the greeting unskippable,
§12). Splitting them out of `internal/server` would have bought a
package boundary and nothing else: the accept loop needs the `Server` it
is accepting for. Negotiation did get its own package,
`internal/telnet`, because it is a state machine with no dependency on
the server at all. Connection hygiene is `Limits` in the same file —
`--max-players`, `--max-connections-per-ip` (bucketed by /64 for IPv6,
see `perHostKey`) and `--login-grace-time`. Graceful shutdown moved out
to `internal/signals` and grew an exit-code contract;
`docs/design/signal-handling.md` is now the document for it, and it says
rather more than the paragraph above does. Two things in this section
are not built: the WebSocket listener (`--listen-ws` is accepted and
inert) and reverse DNS, which this port does not do at all rather than
doing it off the game goroutine — which is also why `slowns` is declined
(`docs/deviations.md`).

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
| `-c` | `dlctl lint --type=world` (own subcommand) | — |
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
**Built 2026-08-23** for the ten fields decided worth it, and an eleventh
(`max_bad_pws`) on 2026-08-28 once the behaviour behind it existed to tune
(`docs/deviations.md`
has the list and the reasoning field by field): a YAML **config file** —
`<lib-dir>/config/game.yaml`, in the data directory it configures
(`docs/design/data-format.md` §6; moved there 2026-08-28 from a repo-level
`config/` directory, with `--config` kept as a path override), shipped as a
fully-commented template in every `examples/` data directory — with every
value defaulting to today's `config.c` value, so no file at all, an empty
one, or a comments-only one reproduces current behaviour exactly
(`game.GameTuning`, `internal/game/tuning.go`). Hot-reload on `SIGHUP`
shipped with it rather than after, since the atomic-pointer publish
(`game.SetTuning`) that makes the config safe to read from three unrelated
goroutines (the world goroutine, `RunAutosave`'s own ticker, a report append
that deliberately does not run through `Server.background`) is the same
mechanism either way — there was no smaller version to build first. The
rest of `config.c` (`pk_allowed`, the room vnums, autowiz, ...) is still a
constant, each one a considered decision rather than an oversight.

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
- **Published on every `v*.*.*` tag** to `ghcr.io/gerrowadat/disgracelands`,
  for `linux/amd64` and `linux/arm64`, tagged `X.Y.Z`/`X.Y`/`latest`
  (`release.yml`'s `image` job). `GITHUB_TOKEN` is the only credential
  involved, and the repository does not have to be public for it: a package
  is created with its repository's visibility and can be made public
  separately. The arm64 image is cross-compiled by the Go toolchain running
  natively on the amd64 runner rather than built under QEMU, which
  `CGO_ENABLED=0` is what makes possible.

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
widths. `persist/world` interface + `classic` implementation. `dlctl lint
--type=world` and `dlctl dump --type=world`. Parity harness against a dump-and-exit path
added to the C server. *Done when: the Go loader reproduces the C loader's
view of all 47 zones exactly.*

**Met, and checked rather than argued.** `scripts/world-parity.sh` builds
both servers, has each dump the world it loaded, and diffs the results:

```
    2981 rooms, 944 mobiles, 1199 objects, 47 zones, 77 shops
    identical
```

Zero differing fields across all 5,248 records. It runs at every release
(`.github/workflows/release.yml` — it needs a full C build, which is more
than day-to-day CI does), and `make parity` runs it by hand.

**Those numbers are of the Disgracelands world**, which was what `data/` held
when this was written. The repo now ships stock CircleMUD 3.0 bpl20's `lib/`
instead, so the harness reports 1878 rooms, 569 mobiles, 679 objects, 30 zones
and 46 shops — 3,202 records, still identical. The criterion is the agreement,
not the count.

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

Two fields are excluded from the comparison, via `dlctl dump --type=world
--parity`, because the C server does not retain them: whether a mob used the
enhanced (`E`) format, which `parse_enhanced_mob()` consumes without
recording, and the espec key/value lines, which `interpret_espec()` folds
into ordinary fields and discards. The Go loader keeps both because they are
useful; comparing them could only ever produce noise.

**Phase 2 — Player loading. ✅ Done.** `persist/player` interface,
`binary` implementation (§4/§5.3), `ascii` implementation, `dlctl
convert|verify|dump --type=pfile`. Password verification and rehash-on-login logic,
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
  updates because positions are referenced elsewhere), and `dlctl
  dump|verify --type=pfile`.

**Also built:** the `ascii` format and `dlctl convert --type=pfile`, and with them
a decision that reversed §5.2 — the server runs on `ascii` and refuses to
start on `binary`. The binary format cannot store a modern hash at all (its
password field is eleven bytes), so keeping it as the live format would have
meant carrying every one of its fixed-width limits into a server written in
2026 to save a single conversion command.

Conversion is checked in both directions, on a record exercising every field
the binary format holds — including the remort vector, which lives in what
upstream calls a spare slot and is exactly the sort of thing a conversion
drops without anyone noticing until a character's skills stop working.

**Credentials are done.** `internal/auth` verifies a stored credential and,
on a correct legacy password, returns a replacement for the caller to save.
The policy is settled: legacy hashes are accepted, upgraded transparently on
login, and nobody is made to reset anything.

DES `crypt(3)` is implemented in `internal/auth/descrypt` rather than
imported. The standard library has none, `crypto/des` cannot be used as a
black box because the salt perturbs the E expansion *inside* the round
function, and cgo is ruled out by the static container build — which left a
dependency for an algorithm being actively retired, or 300 self-contained
lines that get deleted with it. Correctness is not argued: the tests compare
it against the system libcrypt over **9,680 password/salt pairs**, wherever
`gcc` and the libcrypt headers are present — and a release fails if it
skipped rather than ran (`release.yml`).

§5.3.1's warning earned its place twice over. The verifier compares only the
stored 10 characters, and there is a test that fails if it ever compares 13.
Separately, two successive drafts of the "wrong password is rejected" test
used passwords that differ only after the eighth character — which DES
cannot distinguish, so they are *correct* passwords. Both drafts failed for
that reason before the property sank in.

**Phase 2 is complete.**

**A note on verification.** The 32-bit checks skip on a machine without
32-bit libc headers, which is most of them. `release.yml` installs
`gcc-multilib` and fails if those checks skipped, so the layout the real data
is in is verified at every release even though it cannot be verified locally.
Day-to-day CI lets them skip exactly as a contributor's own 64-bit machine
does; a release is where a silent skip is caught.

**Phase 3 — Server skeleton. ✅ Done.** Pulse loop, session lifecycle,
listeners, negotiation, the login `nanny` state machine including the main
menu, and enough commands to look around and move (`look`, `north`, `who`,
`credits`, `help`, `quit`). *Done when: a real player can log in with an
archived character over TLS and walk from the Temple of Midgaard to New
Thalos.*

**Met, and walked rather than argued.** Against a server on the real world
files with an empty roster, over the TLS listener, driven by a scripted
client:

```
    Walker, credential "WaApIYE9Szu2A" (DES crypt(3), salted "Wa")
    logged in over telnets, menu choice 1, room 3001 The Temple Of Midgaard
    s s s e e s s e e e e e e e e e e e n n n          21 moves
    room 5411 Outside The Southern Gate Of New Thalos
    "upgraded a legacy password" character=Walker
    saved: Room: 5411, Pass: argon2id:$argon2id$v=19$...
```

Three things at once: a pre-2008 credential verified by the DES path and
silently upgraded to argon2id on the way in, the walk itself, and the
position surviving the save.

One qualification, since the criterion says *archived character*. The 108
real records are local-only and never in this repo, so this character was
created here and then given a genuine DES hash of the kind the roster holds
— the same hash the C's `crypt()` produces for that name and password. What
it does not exercise is the archive's own bytes, which is Phase 2's
criterion rather than this one.

Three things that are not optional in this phase, for reasons outside the
phase itself:

- **`credits` and `help circlemud` are licence compliance**, not features to
  defer (§12). They belong alongside `look` and `who`.
- **The greeting file must be emitted on every transport**, WebSocket
  included. A web client that renders its own splash screen and skips it is
  a licence violation, not a UI choice — worth a test in the parity harness.
- **GMCP is built properly**, because a web front end is intended rather
  than hypothetical (§0). Out-of-band structured data for prompts, vitals
  and room information is what lets a browser client be a real client
  instead of a screen-scraper.

Output is UTF-8 (§0). The protocol layer negotiates CHARSET and transcodes
*outbound* for clients that ask for something else, which is a per-connection
concern and the right place for it — as against decoding the world files on
every read, which is what `dlctl convert` exists to make unnecessary.

### What reading the C changed

**Character creation is two steps, not one, and the split is load-bearing.**
This was got wrong first and is worth stating plainly, because the wrong
version produced characters that looked entirely plausible.

`init_char` (db.c:2688) runs at the class prompt. It leaves an ordinary
character at **level zero**, with 100 mana, 82 movement points, 100 armour,
every ability at 25 and no hit points at all. If the roster is empty it
instead makes that character the Implementor outright — level 34, 7,000,000
experience, 500 hit points, every skill at 100%, no hunger or thirst.

`do_start` (class.c:1802) does not run then. It runs when the player chooses
"enter the game" from the menu, and only `if (GET_LEVEL(ch) == 0)`
(interpreter.c:1684) — so an Implementor never runs it, which is the only
reason they keep the statistics `init_char` gave them. It rolls the abilities,
sets `max_hit` to 10, grants the thief's six starting skills, and calls
`advance_level` once.

Three consequences that a reduced version gets wrong:

- **A level-one character has no mana of their own and 100 of it anyway.**
  `advance_level` guards the mana gain on `level > 1`, and `do_start` never
  touches `max_mana` — so the 100 from `init_char` is what they walk around
  with. A port that adds a level's mana at creation is visibly wrong; so is
  one that concludes a new character therefore has none.
- **`MAX(1, …)` on hit points and movement.** A magic-user with a low
  constitution can roll a negative hit-point gain, and a magic-user or cleric
  rolls `number(0, 2)` movement. Neither may cost them a level's progress.
- **Practices use the remort-aware `IS_<CLASS>` macros** (utils.h:505), which
  Disgracelands rewrote to consult `remort_vector`. A warrior who remorted
  through cleric practises at a cleric's rate. A plain `GET_CLASS(ch) ==`
  comparison silently removes a local feature.

**The main menu is part of the login sequence.** `CON_RMOTD` shows `MENU` and
waits (interpreter.c:1632); a player is not in the world until they choose to
be. Four of the six choices are things that can only be done from there — the
description editor, the background story, changing a password, and deleting
the character — so the menu is not a screen to skip past, it is where those
features live. All six are ported.

**Deviations are recorded in [docs/deviations.md](../deviations.md)**, created
in this phase rather than in Phase 1 as this plan originally said. It has the
Paladin decision below, the password rules, the limits the C has none of, and
the handful of internal differences worth knowing about.

**Three behaviours settled, all resolved towards the C server:**

| Question | Decision |
|---|---|
| **Character creation** | The **full C flow**: name, password, sex, class, rolled stats — `CON_QSEX` and `CON_QCLASS` included. Not a reduced version with class deferred. |
| **Saving** | On quit, and on a periodic autosave matching the C's `PULSE_AUTOSAVE` of 60 seconds. Writes happen off the world goroutine so a slow disk cannot stall the game. |
| **Link loss** | The character **lingers in the world** for a reconnect, as the C's `CON_DISCONNECT` does, rather than being removed at once. Needs a reconnect path in the login flow and a reaper to time linkdead bodies out. |

**This moves a phase boundary, and the plan should say so rather than let it
drift.** Class selection at creation needs the class tables, and those belong
to Phase 4 — including the two things that make Disgracelands' classes
non-stock: the Paladin as a fifth class, and how `remort_vector` interacts
with a character who has just been made
(`docs/investigations/non-stock-features.md`). So Phase 3 now pulls that much
of Phase 4 forward. The alternative was a creation flow that differs from the
C's, which the fidelity decision in §0 rules out.

**Paladin is reachable only by remort, not by creation** — and reading the C
to confirm that turned up a discrepancy the port has to make a decision
about.

`class_menu` (`class.c:93`) offers four classes: Cleric, Thief, Warrior,
Magic-user. Paladin is not among them, which matches the rule. But
`parse_class()` (`class.c:117`) accepts `'p'` and returns `CLASS_PALADIN`,
and creation uses that same function — so a player who typed the unadvertised
`p` at the creation prompt would have become a Paladin, bypassing remort
entirely. The parser is shared with `set class` in `act.wizard.c`, which is
presumably why the case is there at all.

So the C server's *intent* and its *behaviour* disagree, and "be faithful to
the C" does not settle it. The port follows the intent: **the creation prompt
accepts only the four listed classes, and `p` is rejected there**. Paladin
remains reachable by remort and by an implementor's `set class`, which is
where `parse_class` accepting it is correct.

This is a deviation and is recorded as one. It is the first case of the
fidelity rule in §0 running into a place where the C contradicts itself, and
the reasoning is worth keeping: reproducing the accident would let a new
player skip the mechanic the class exists to reward.

### What is not in it

A phase marked done without its gaps is worse than one not marked at all.
This was two once, and is one now — the other, corrected here rather than
left for a reader to trip over, is not a deviation, so it is not in
`docs/deviations.md`:

- **`--listen-ws` starts nothing.** The WebSocket transport is the one part
  of this phase not built, so the greeting requirement above holds for every
  transport that exists rather than for every transport intended. It is not
  rescheduled into a numbered phase: it belongs with the web client in the
  "Later" list, and the requirement travels with it.

The configuration gap is marked *(inert)* in `docs/configuration.md` rather
than left to be discovered at runtime. `--max-players` was the other one
this section used to name; it is enforced now — see Phase 6's write-up.

**"Movement ignores door state" stopped being true without this document
saying so.** `EX_CLOSED` reads as though nothing had ever ported it, and
by the time Phase 6 started reading the C for the specproc and pager work,
doors, their runtime state, and all five commands that operate them
(`open`/`close`/`lock`/`unlock`/`pick`) had already landed — `perform_move`'s
own `EX_CLOSED` check included, so a player has not been able to walk
through a closed door for a while. Left here as a corrected record rather
than moved, the same reason color's own stale note was fixed rather than
silently dropped (#110).

**Phase 4 — Rules core. ✅ Done.** Combat, magic, skills, classes including
the remort bitmask, affects, position/regen, death and corpses, zone resets,
mobile activity. The largest phase; the deviations log from the fidelity
decision filled up here. *Done when: a character can level.*

**Met.** `TestACharacterCanLevel` walks the whole path end to end: a mortal
kills something, is paid for it, crosses the boundary and rises, with
`advance_level` running and the title changing with it. Everything in the
phase stands behind that one test — the swing has to land, the death has to
be noticed, the kill has to be worth something, the tables have to say what
the next level costs.

### What reading the C changed, again

The pattern from Phase 3 repeated, and harder: **the C is not what it looks
like, and the difference is never in the obvious place.**

`docs/weirdnumbers.md` was written during this phase and is the single most
useful document in the repository. Twenty-odd entries when the phase closed
and 57 now, each one a place where the arithmetic does not do what it appears
to — `compute_thaco` truncating
after each subtraction rather than at the end, `graf`'s 60–79 band dividing by
20 where every other band divides by 10, a container's own weight counting
against its own capacity, a player's abilities being clamped to **18** rather
than 25 so an immortal's rolled 25s decay the first time anything totals them.

The rule that came out of it: **anything with a division, a cast, or a comment
describing numbers gets an oracle rather than a reading.** `reference/tools/`
holds the original function bodies with the `char_data` dereferences
substituted and nothing else changed, and the Go tests compare against them
across the whole input space where that is affordable. Verified this way so
far: 30,000 RNG draws across six seeds, 36,288 regeneration values, 1,512,000
to-hit values, 1,125 saving throws, 9,680 DES `crypt(3)` pairs, every ability
table, the title tables, the experience tables and `money_desc`. Phase 5 added
a fifth oracle, `shopprice.c`, and three layout tools for the on-disk formats
it built — see `reference/tools/README.md`.

**Two structural decisions worth keeping.**

*Affects are recomputed from stored real values*, not subtracted and re-added
as `affect_total` does. The C has nowhere to keep the unaffected figures and
recovers them by walking the list twice — correct only while nothing changes
between the passes, and things do. This is the one place the port reproduces a
routine's *outcome* rather than its method, and it is recorded in
`docs/deviations.md` with the reasoning.

*There is one damage path.* Every command that can hurt somebody — a swing, a
kick, a bash, a spell, poison, a god's `kill` — goes through `damage()`. Until
late in the phase each applied its own damage and none of them handled what
happens when the hit points run out, so a kick could kill a mobile and leave
it standing there dead with no corpse and nobody paid. The `session.Violence`
seam exists for that: the command says what it did, and one place decides what
it meant.

### What is not in it

- **Specprocs.** No `spec_procs.c` equivalent, so guildmasters, shopkeepers,
  the postmaster and the rest are inert. `practice` is a command here rather
  than something a guildmaster does, which is recorded in
  `docs/deviations.md`. They are the first slice of Phase 5, built on §8's
  `Trigger` interface — not deferred with the scripting interpreter, because
  the economy and the mail depend on them.
- **`steal` and `track`.** The two thief skills not ported. `steal` needs the
  killer/thief flag machinery and shopkeeper protection; `track` needs the
  breadth-first search the C keeps in `graph.c`, which is a file of its own.
  `hide`, `sneak`, `backstab`, `bash`, `kick`, `rescue` and `pick lock` are
  all here.
- **Scrolls, wands, staves and potions.** `do_use`, `do_quaff` and `do_recite`
  are the way into `call_magic` for everything above `MAX_SPELLS` — `identify`
  included, which is why that spell cannot be cast. They are `act.item.c`
  commands and go with Phase 5.
- **`junk` and `donate`**, the other two subcommands of `do_drop`, both of
  which need rooms that do not exist yet.
- **Socials**, which are Phase 5 and whose absence is visible in the command
  table's *order*: a mortal's `f` reaches `fart` in the C, so nothing here can
  reproduce what `f` means until they land.
- **`N.thing` targeting.** `get_number` splits a leading `2.` off any argument
  in every command that uses `generic_find`. It belongs with the rest of
  `generic_find` rather than in any one command.

All of these are in `docs/deviations.md` under "gaps still to fill" rather
than as deviations, because they are not decisions — they are simply not
built.

**Phase 5 — The rest of the game. ✅ Done.** Everything else a player could type
in 2008. *Done when: an archived character can do everything they did then,
except build.*

**Met, with the tail named rather than waved at.** Every slice below is built,
including the three mechanisms that were never slices at all and had to be
found. 308 of the C's 318 commands answered when Phase 5 finished. Of the 10
that did not, nine were the OasisOLC and text editors that belong to Phase 6
by design (`tedit` has since landed as Phase 6's own first slice — see
below), and the other nine are listed one by one under "What is not in it" —
none of them a subsystem, none of them blocking anything, all of them an
afternoon.

**Counted rather than remembered, and the earlier figures here were wrong.**
`cmd_info[]` holds 319 rows before the `"\n"` sentinel, one of which is
`RESERVED` — a placeholder that exists so a specproc can return command 0 and
is not typeable — so **318 commands**. 105 of them are `do_action`, and the
shipped socials file fills 104 of those (it also carries a `you` entry with no
table slot, which the C drops with a log and so does this port).

**308 of the 318 were implemented when Phase 5 finished**: 203 in
`internal/session/commands.go` plus the 105 socials. **310 now** — `tedit` was
Phase 6's first slice and `color` came with the colour work.

**Every slice below is built.** The eight left are listed under "What is not in
it": the seven remaining OasisOLC editors, which are Phase 6, and `slowns`,
which is declined rather than pending.

The authority for those numbers is no longer this paragraph. `notPorted` in
`internal/session/coverage_test.go` lists every unported row with a reason, a
test re-parses `interpreter.c` and requires the two to agree both ways, and a
second test reads the figure back out of the prose. Porting a command fails the
suite until the documents are corrected, which is how the count stopped drifting
after being wrong four separate times.

The slices are listed in dependency order rather than in order of importance:
the first one unblocks three of the others, and after that the work was
genuinely parallel.

The seam itself turned out to be smaller than the thing it unblocks. A special
is a function that gets first refusal on a command, or a tick with no command
at all — two entry points, one registry and a table. What made it worth doing
first is that four other slices are written *against* it rather than beside
it.

| Slice | What is in it | C files |
|---|---|---|
| **5a. Special procedures ✅** | The seam, the 205-row assignment table, and ten of the C's specials: guild, guild_guard, puff, fido, janitor, cityguard, snake, magic_user, thief, dump. `practice` is a guildmaster's again. What is left needs other subsystems — shopkeepers need shops, the postmaster needs mail, bankers need banking — or belongs to the archived world. `assign_kings_castle` is a zone-sized script and is untouched. | `spec_procs.c`, `castle.c`, `spec_assign.c` |
| **5b. Communication ✅** | `say` (and `'`), `tell`, `reply`, `whisper`, `ask`, `shout`, `holler`, `gossip`, `grats`, `auction`, `gsay`/`gtell`, `qsay`, and the seven channel toggles. `emote`, `write`, `page` and the immortal channels are not: `emote` is `do_echo` and goes with the immortal commands, and `write` needs the boards. | `act.comm.c` |
| **5c. Socials ✅** | `do_action`, the socials file, and `act()` itself — the `$n`/`$N` substitution every message in the game is written in, which the port had been faking with `%s` per audience. Settles a dozen command-table prefixes that were placeholders: `f` is `fart`, `ti` is `tickle`, `cl` is `clap`. `alias` is not done. | `act.social.c`, `comm.c` |
| **5d. Information and preferences ✅** | `commands`, `socials`, `diagnose`, `gold`, `levels`, `where`, `whoami`, `wizlist`, `immlist`, the canned-text commands (`motd`, `imotd`, `news`, `info`, `policy`, `handbook`, `version`, `clear`), the `PRF_*` toggles, `display`/`prompt`, `title`, `wimpy`, `save`, `report`, `split`, `toggle`, `visible`. Not done: `color` (nothing emits colour yet), `uptime` and `users` (immortal, 5i). | `act.informative.c`, `act.other.c` |
| **5e-i. Magic items and the last of do_drop ✅** | `use`, `quaff`, `recite` and `mag_objectmagic` — wands, staves, scrolls and potions, and with them everything above `MAX_SPELLS` including `identify`. Plus `junk` and `donate`. | `spell_parser.c`, `act.item.c` |
| **5e-ii. Objects carried across a reboot ✅** | `Crash_load`/`Crash_save`: rent, the object save files, and the menu's choice 1 telling a player what they lost. A new persistence format and its own `Store`. Renting at an inn moves to 5f with the rest of the specprocs. | `objsave.c` |
| **5f-i. Shops ✅** | `shop_keeper` and the four commands it intercepts: `buy`, `sell`, `list`, `value`. The keyword-expression evaluator, the prices, the keeper's bank, and the grouped inventory `list` counts. | `shop.c` |
| **5f-ii. Banking and the inn ✅** | `balance`, `deposit`, `withdraw`, `offer`, `rent` — the `bank`, `receptionist` and `cryogenicist` specials. Finishes 5e. `receive` waits for the postmaster in 5g. | `spec_procs.c`, `objsave.c` |
| **5g-i. Bulletin boards ✅** | `gen_board` and the four commands it intercepts, the board files, and the line editor that `write` and mail both need. | `boards.c` |
| **5g-ii. Mail ✅** | `mail`, `check`, `receive`, the postmaster, and the block-allocated mail file. | `mail.c` |
| **5g-iii. Houses ✅** | `house`, `hcontrol`, the house control file, the per-room object saves, and the trespassing check in movement. | `house.c` |
| **5h. The last of the rules ✅** | `steal` and `track` — the two thief skills left, and the only two that needed machinery of their own: the shopkeeper and player-thieving checks for one and `graph.c`'s breadth-first search for the other. Plus `order`, `enter` and `leave`. `remort` and `reroll` move to 5i, where `do_wizutil` lives. | `act.offensive.c`, `act.movement.c`, `act.other.c`, `graph.c` |
| **5i-a. Getting about ✅** | The command table's minimum levels — which are part of *matching*, not a check afterwards — plus `goto`, `at`, `transfer`, `teleport`, `invis`, `poofin`, `poofout` and the shared `find_target_room`. | `act.wizard.c`, `interpreter.c` |
| **5i-b. Looking at the innards ✅** | `stat` (room, object, character), `vstat`, `vnum`, and the fourteen name tables they print — now checked against `constants.c` by re-parsing it. `show`, `last` and `date` move to 5i-d, where the operational state they report lives. | `act.wizard.c`, `constants.c` |
| **5i-c. Changing things ✅** | `load`, `purge`, `advance`, `restore`, `zreset`, and all seven of `do_wizutil` — `reroll`, `pardon`, `notitle`, `mute`, `freeze`, `thaw`, `unaffect`. | `act.wizard.c` |
| **5i-d. Talking as a god ✅** | `echo`, `emote`, `send`, `gecho`, `wiznet` (and its `;` alias), `syslog`, `force`. | `act.wizard.c` |
| **5i-e. `set` ✅** | `do_set` and `perform_set`: fifty-two fields, each with its own level, PC/NPC restriction and range, checked against the C's table by re-parsing it. | `act.wizard.c` |
| **5i-f. Running the place ✅** | `snoop`, `switch`, `return` — the only commands in the game that reach past the character to the connection — plus `dc`, `wizlock`, `shutdown`, `date`, `uptime`, `last`. | `act.wizard.c` |
| **5i-g. Bans and `show` ✅** | `ban`, `unban`, the ban file and its enforcement at the name prompt; `show` and its ten fields. `show rent` and `show shops` wait on their own listings. | `ban.c`, `act.wizard.c` |
| **5i-h. `remort` ✅** | A local addition: a per-character bit vector of borrowed class skills, the `IS_<CLASS>` macros that read it, and `redeem` for a fallen paladin. The vector and the macros had been in since Phase 3 — this is the two commands that write them. **The last slice of Phase 5.** | `act.wizard.c`, `class.c` |
| **5j. The interpreter's own refusals ✅** | `command_interpreter`'s `else if` ladder between finding a command and running it: the frozen check, the switched-immortal check, and `minimum_position` — `cmd_info[]`'s second column, which nothing had been reading. Not a slice when Phase 5 was planned, because it is a property of every command rather than of any one; see below. | `interpreter.c` |
| **5k. Light and darkness ✅** | `world[].light`, `room_is_dark` and `CAN_SEE_IN_DARK`, and with them `look_at_room` in full: the pitch-black and blindness branches, `PRF_BRIEF`, `PRF_AUTOEXIT`, `PRF_ROOMFLAGS` and the two `<DoC>` room messages. Plus the four preference-based immortal toggles, `holylight` among them, without which nothing could switch the new behaviour on. The half of `CAN_SEE` that is about the room; the half about people is next. | `utils.c`, `handler.c`, `act.informative.c`, `act.other.c` |
| **5l. Seeing people ✅** | `CAN_SEE` itself and the display half of its call sites: `PERS`/`OBJS` inside `act()`, `list_char_to_char` and `list_one_char` in full, `list_obj_to_char`, `who` and `where`. Invisibility, hiding and the invis level all mean something now. Targeting — `get_char_room_vis` and the rest of `generic_find` — is the other half and is not in it; see below. | `utils.h`, `act.informative.c`, `comm.c` |
| **5m. What a typed word means ✅** | `isname` and `get_number`, the two pure functions every search in the game goes through, with a C oracle over 168 name pairings and 15 argument forms. `isname` was being read as a prefix match and is not one, so `get swo` had been picking up a sword since Phase 4. Groundwork for the targeting pass as well as a fix in its own right. | `handler.c` |
| **5n. Targeting ✅** | `game.Search`: the `*_vis` family's shared, decrementing count, with CAN_SEE and CAN_SEE_OBJ applied at every search. `FindInRoom`, `FindAnywhere` and `findObject` take a viewer; `2.sword` works; `0.name` means a player. The last of the three cross-cutting mechanisms. | `handler.c` |

### What is not in it

The 20 commands of the 318 that nothing answers to yet. Everything here is a gap rather than a decision,
so it is in `docs/deviations.md` under "gaps still to fill" rather than as a
deviation.

**Phase 6's, and correctly so.** `medit`, `oedit`, `redit`, `sedit`, `zedit`,
`olc` and `edit` are OasisOLC; `tedit` is the in-game text-file editor.

**The three mechanisms that never had a slice are now built.** Each was a
property of *every* command rather than of any one of them, which is exactly
why none of them got scheduled and why all three had to be found rather than
picked off a list: **minimum position** as 5j, **`CAN_SEE`** for display as 5l
and for targeting as 5n, and **`N.thing`** alongside it in 5n.

What is left of Phase 5 is the `remort` slice and the loose commands above.

### What 5n changed, and what it caught

`game.Search` is the C's count made explicit. The count is a **pointer** in the
C, handed down a chain of searches — `get_obj_vis` gives the same `int *` to
the inventory, then the room, then the world (handler.c:1148) — so `2.sword` is
the second sword across the whole search *order*, not the second in whichever
list holds it. A Search carries that state, and CAN_SEE is applied inside it,
so no caller can forget either.

Two things came out of doing it.

**A nil viewer sees everything, deliberately.** The C's `*_vis` functions
always have a `ch`; a search here with no viewer is not a pair of eyes at all —
a zone reset looking for the mobile it just made, a test asking what is in a
room. Filtering those through nobody's sight finds nothing, which is a silent
and very confusing failure, so it is spelled out instead.

**You cannot get your torch out in the dark.** `hold` looks the torch up with
`get_obj_in_list_vis`, which asks `CAN_SEE_OBJ`, which asks `LIGHT_OK` — and in
an unlit room that is false for everything, including the contents of your own
pack. So a torch has to be lit before you go down, and somebody who puts theirs
away in a cave has put it away for good. Nothing decides this anywhere; it
falls out of two unrelated functions agreeing. A test that had been lighting a
torch *in* the cellar started failing, which is how it was noticed.

### What 5j changed, and what it caught

`cmd_info[]`'s second column is `minimum_position`, and `command_interpreter`
gates every command on it (interpreter.c:636–661) with seven refusals chosen by
the position the character is *in*. Nothing had been reading it, so a sleeping
character could `kill` and a mortally wounded one could `buy`.

Two things about the shape of the fix are worth keeping.

**The values are generated, not transcribed.** `commandPositions` is all 318
rows of the C's table, extracted mechanically, and a test re-parses
`interpreter.c` and requires the map to be that column exactly — in both
directions, so a row the C does not have fails too. Three hundred values typed
by hand is three hundred chances to let somebody fight in their sleep, and
none of them would fail any other test: a command that runs when it should not
still runs correctly.

**It found four messages no player has ever seen.** `do_stand`, `do_sit` and
`do_rest` each have a `POS_SLEEPING` arm saying *"You have to wake up first"*,
and all three commands are `POS_RESTING` in the table — so the interpreter
stops a sleeping character first and those arms are unreachable. `do_flee`
opens with the identical comparison the interpreter has already made. This
port had all four, read faithfully out of the C, with tests asserting them.
That is the general lesson and it is now in `docs/weirdnumbers.md`: **a
command's own position check tells you what the interpreter already refused,
not what a player sees.**

Two adjacent branches of the same ladder went in with it, both also missing:
the **frozen** check (interpreter.c:629 — `freeze` set the flag and nothing
enforced it in the world) and the **switched-immortal** check (:634).

### What 5k changed, and what it caught

The same pattern once more: the thing that was missing was not a command but
the *state a command reads*.

`room_is_dark` needs `world[room].light`, and nothing here had it — so no room
was ever dark, `look` never printed *"It is pitch black..."*, and infravision,
`AFF_BLIND` and holylight were all set correctly by the things that grant them
and read by nothing. That is `LIGHT_OK`'s whole input, which is why this had to
come before `CAN_SEE` rather than with it.

Four findings came out of reading it, all in `docs/weirdnumbers.md`. The one
worth repeating here: **the light count is of lights worn in `WEAR_LIGHT` by
people in the room, and nothing else.** `obj_to_room` does not touch it, so a
burning torch dropped on the floor lights nothing and putting your torch down
puts the room out. A reasonable-looking implementation that counted lit objects
in the room would be wrong, and wrong in a way no test would catch unless it
was written to.

Going in after it: `look_at_room` had been ported without four of its gates.
`PRF_BRIEF` and `PRF_AUTOEXIT` were settable, listed by `toggle`, saved to the
pfile — and read by nothing, so `brief` and `autoexit` did nothing at all.
`PRF_ROOMFLAGS` had no command to set it. The exits line listed **closed** exits
and printed nothing rather than *"None! "* for a room with no way out. And the
two `<DoC>` room messages, for `ROOM_GOOD_REGEN` and `ROOM_PKILL`, were absent.

The four preference-based immortal toggles went in with it, because `holylight`
is half of `CAN_SEE_IN_DARK` and there was no way to switch it on.

### What 5l changed, and what it caught

`CAN_SEE` is six macros nested three deep, and taking them apart is worth doing
once. Two things the nesting hides:

- **Holylight appears twice and means different things.** `LIGHT_OK` does not
  consult it; `IMM_CAN_SEE` puts it *beside* the whole mortal test rather than
  inside it, so a god with holylight sees through darkness, invisibility and
  hiding in one step rather than three.
- **The invis-level test sits outside `IMM_CAN_SEE`**, which makes it the one
  thing holylight cannot defeat. A god cannot see an equal-or-higher god who is
  `invis`, whatever else they have on. That is the whole reason `invis` works
  against other immortals.

And one that is not in the macro at all: **`AFF_SNEAK` is not in `INVIS_OK`.**
Sneaking conceals *movement*, not the person, so somebody sneaking in front of
you is plainly visible. The three are granted close enough together that
assuming otherwise is easy, and there is a test that fails if anybody adds it.

`GET_REAL_LEVEL` (utils.h:268) exists for exactly one caller, `CAN_SEE`: a god
switched into a rat keeps their own level for the invis-level test and for
nothing else. It is asked of the connection through an optional interface,
because only sessions can be switched.

**Three more unreachable-or-inconsistent findings**, all in
`docs/weirdnumbers.md`. `do_look` and `look_at_room` guard themselves
differently — blindness before darkness in one, darkness before blindness in
the other, and different sentences for both, so a blind character gets a
different answer for typing `look` than for walking through a door. `do_look`'s
first branch is a *fifth* message nobody has ever seen, refused by the same
minimum-position gate as the four in 5j. And the "glowing red eyes" line turns
out to be reachable from exactly one of `list_char_to_char`'s two callers,
which is why the C's only comment on it is `/* glowing red eyes */`.

Going in after it, in `list_one_char`: the port had been printing "%s is
standing here" for everybody whatever they were doing, with no title, no
`(invisible)`, `(hidden)`, `(linkless)` or `(writing)` marker, no aura, and
nothing for a mobile fighting somebody. All of it is there now.

(`do_quit`'s own guards were the last of these and are now built — see below.
`do_gen_write` — `bug`/`idea`/`typo`, interpreter.c:247, :342, :520 — is also
now built, alongside its `state/reports.yaml` format:
docs/design/data-format.md §11 step 6b.)

**The small things left over.** None is more than an afternoon; they are here
because a command with no slice is a command nobody schedules.

| | |
|---|---|
| The last `do_gen_tog` toggle | `slowns` (interpreter.c:472). The other sixteen are done. `trackthru` landed with the setting moved onto `Live` — a package variable would have been a race between the several servers a test run builds, where the C's global is right for one server per process. `slowns` has nothing behind it at all: this port does no reverse DNS, so a command reporting success would be lying. |
| ~~Aliases~~ | All built: `:` for emote (interpreter.c:286), `take` for get (:503), and the C's two deliberate stumps `qui` (:421) and `shutdow` (:463). |
| ~~Immortal odds and ends~~ | All built: `users` (:528), `skillset` (:469), `reload` (:428), `wizhelp` (:553), `qecho` (:419) and `page` (:398). |
| Mortal odds and ends | `color` (:258), and only that: the `PRF_COLOR` bits are stored and `set color` works, but nothing emits colour, so the command has nothing to switch. `insult` (:346) and `alias` (:226) are built. **`hop` (:337) was never missing**: it is the one `do_action` row the shipped socials file does not fill, and `RegisterSocials` gives it a command anyway that answers "That action is not supported." — which is what the C does too. |
| ~~`mudlog`'s in-game half~~ | Done. `obs.WithWizVisEcho` + `Server.echoWizVis` deliver the echo, and #134 (2026-08-28) wired up every `mudlog()` call site in ported territory. `docs/deviations.md` has the audit. |

**Phase 6 — Building tools.** Originally scoped as an OasisOLC
equivalent — the seven in-game menu editors (`medit`/`oedit`/`redit`/
`sedit`/`zedit`/`olc`/`edit`) plus `Sink` writeback and the `gen*`
layer they need. **Decided against, deliberately, not merely deferred**:
building lets a builder edit the world files in the `--lib-dir` directly
(by hand, or via `dlctl import --type=world`/`fmt` into `yaml`) and bring a
change in without restarting — `reloadmob` (below) — rather than
reproducing a decades-old menu-tree UI screen by screen.
`Sink`/`WriteZone` already exist (§6.3, landed during Phase 5) and are
exactly what a real OLC would have saved through; nothing about this
decision leaves them stranded, it just means nothing in this tree drives
them from an in-game menu. `dlctl lint --type=world` and the world-parity
harness are what make offline editing safe either way.

**`tedit` ✅ — the phase's first slice, landed before the OLC decision.**
`do_tedit`'s nine canned text files (`credits`/`news`/`motd`/`imotd`/
`help` screen/`info`/`background`/`handbook`/`policies`), each at its own
C-table level, editable in-game through the line editor board `write`/
mail already use — extended with a seeded-buffer variant
(`beginEditorSeeded`) so the file's current content starts the edit
instead of an empty one, matching `string_write`'s own behaviour when
the pointer it is handed already points at something. A real,
previously-undocumented finding along the way: the archived server's
`CONFIG_IMPROVED_EDITOR` is hardcoded `1` — the improved line editor's
`/c`/`/l`/`/h`/`/a`/`/s` commands were always on, not stock — and this
port's line editor has never had them, invisibly until `tedit` became
the first caller to seed a non-empty buffer. Five landed in the slice
below and the remaining six followed it; all eleven work now.

**`reloadmob` ✅ — genuinely new capability, not a C port; the shape
Phase 6 actually took.** `interpreter.c` has nothing like it. An
implementor types `reloadmob <vnum>`; the server re-reads the world data
it booted from (whatever `--world-format` is configured), finds that
vnum's fresh definition, and — provided no current instance of it is
fighting — applies it to the running world without a restart.

The design turns on one fact about the Go model: `Character.MobDef`
is a *pointer* to the same object `Live.mobileDefs[vnum]` holds, and a
lot of live behaviour reads it continuously rather than only at spawn —
`ActionFlags` (AI), `Spec` (special-procedure dispatch, checked every
command), `LongDesc`/`Position` (room listings). Mutating that object's
fields *in place*, rather than replacing the map entry, means every
existing instance sees a behavioural or descriptive change for free,
with no per-instance code at all — the same way an affect already
changes what a live check sees mid-tick. That is also why the "nobody
fighting it" gate has to be all-or-nothing rather than per-instance:
there is no way to update a shared object for some readers and not
others. Numeric stats (`HitDice`/`DamageDice`/`Thac0`/etc.) are
snapshotted into each instance's own `*PlayerRecord` at spawn time,
`SpawnMobile`'s own long-standing behaviour (`internal/game/reset.go`)
— those do *not* update from an in-place `MobDef` mutation, so every
current, unengaged instance also has its derived stats recomputed
through the same helper `SpawnMobile` itself now calls
(`mobileRecord`), factored out for exactly this reuse. `Spec` is the one
field preserved explicitly across the mutation: it is set once at boot
from the assignment table (`AssignSpecials`), never from the world file
at all, so a fresh parse's own empty `Spec` must not overwrite it.

Room, inventory, equipment, followers and position are left exactly as
they are — refreshing a mob is not respawning it, and a builder fixing a
stat typo should not cost anybody their dropped loot. One accepted,
documented edge case: gold and experience reset to the definition's own
values along with everything else, so a player who stole from an
about-to-be-reloaded mob moments earlier sees the theft undone. Small,
honest, not worth engineering around for an implementor-only command.

`reloadmob` is not a real C command, so it needed a small, explicit
carve-out in `internal/session/coverage_test.go` (`newCommands`,
mirroring `notPorted`'s own shape in the opposite direction) rather than
pretending it has an `interpreter.c` line — its `CLine` is synthetic
(the real `reload`'s own line, 428, plus one), placing it in
abbreviation-matching order right after the real, ported `reload` so a
bare `reload` keeps meaning exactly what it always has.

**`reloadzone` ✅ — the natural, larger follow-up, landed the same day.**
`reloadzone <vnum>` extends the same mechanism to every room and mobile
in a zone's vnum range at once, gated on `Live.ZoneIsEmpty` (already
built for `zreset`'s reset-mode-1 check, reused rather than
reimplemented) — no player anywhere in the zone — and the same "nothing
in range is fighting" check `ReloadMobile` already makes, now scanning
the whole range rather than one vnum. `Character.MobDef` being a shared
pointer is what makes mobile reload work; rooms turn out simpler still —
nothing outside `Live.rooms` holds a `*RoomDef` across a tick (checked,
not assumed), so a room's map entry can just be replaced outright rather
than mutated in place.

Deliberately conservative in what it applies, and worth stating plainly
rather than leaving implicit: a room or mobile vnum the fresh data
introduces that the running world does not already have is **skipped,
not created** — reload updates what exists, it does not import what is
new — and a vnum the fresh data no longer has (deleted from the file) is
**left as a stale entry**, not removed. Both still need a restart.
Reset-script re-application is not attempted — reloading a zone changes
what its `M`/`O`/`G`/`E` commands would create from now on, not what a
reset already made — and needs its own answer for the same reason zone
reload's own room/mobile carve-out did: re-running the list would create
duplicates of everything the zone already holds rather than refreshing
it, which is a different feature from this one.

**`reloadobj`/`reloadshop` ✅ — object and shop reload, landed once "what
does refreshing an existing object even mean" had an answer.** Objects
carry `Object`'s own shadow-field copy of their prototype's name/
description (unlike a mobile's `Record`, which is fully re-derived every
reload), and those fields hold real per-instance state ordinary gameplay
mutates continuously — a wand's remaining charges, a container's lock
state, a bless or curse. `ReloadObject` (`internal/game/reset.go`)
answers by touching only the prototype: existing instances keep whatever
they currently are, and pick up only the handful of fields `Object`
already reads live from `Def` rather than copies at spawn (`Spec`,
`MinLevel`, `RentPerDay`, the fallback `ActionDesc`) — a new spawn gets
the fresh definition in full. Unlike `reloadmob`, there is no engagement
refusal to make: nothing about an object can be "fighting", so there is
nothing a reload could interrupt.

A shop is simpler still, and for a different reason: `shopState`'s own
doc comment already says it plainly, "there is exactly one of each shop"
— no shared-prototype-versus-live-instance question ever arises, because
a shop is never instantiated the way a mobile or an object is.
`ReloadShop` (`internal/game/shopstate.go`) copies the whole
configuration across — prices, buy types, messages, temper, flags,
trade-with, rooms, keeper — leaving only the shop's actual till
(`shopState`'s `Bank`/`Sorted`) untouched, the same way `ReloadMobile`
leaves a mobile's room, inventory and position alone. A keeper change
needs one extra step `ReloadMobile`/`ReloadObject` do not: the new
keeper's prototype needs the `shop_keeper` special the way
`AssignShopkeepers` gives it at boot, since `ShopFor` resolves a shop's
keeper by matching `Keeper` against a live mobile's vnum on every call
rather than caching a binding — nothing else would ever set it. `Secondary`
is preserved across a reload the same reason `Spec` is on a mobile or an
object: no loader ever writes it (checked by grepping every loader, not
assumed), so a freshly parsed `ShopDef` always has it blank, and only a
real keeper change re-derives it.

Neither `reloadobj <vnum>` nor `reloadshop <vnum>` is wired into
`reloadzone`'s own zone-wide sweep — each is its own command, the same
shape `reloadmob` had before `reloadzone` extended it, and that
extension is not repeated here yet. `docs/deviations.md` was not the
right place for any of this: none of it is a deviation, it is simply new
capability with no `interpreter.c` row to compare against, the same as
`reloadmob`/`reloadzone` before it.

**The pager ✅ — `page_string`, ported in full, then wired into the rest
of its own call sites.** `next_page`/`count_pages`/`paginate_string`/
`show_string` (`modify.c`) landed as `internal/session/pager.go`, with a
real `StatePaging` connection state and `make_prompt`'s own paging
branch folded into `prompt(s)` — see `docs/deviations.md` for the two
real bugs this found (an LF/CRLF column-counting mismatch and
`isNumber("")`'s vacuous truth) rather than assumed away. First wired
into the canned texts and `help`; a follow-up pass wired it into the
call sites `page_string` reaches beyond those — a board's message list
and a message's own body, a shop's `list`, `practice`'s skill list, and
`do_show`'s `zones`/`errors`/`death`/`godrooms` fields — checked field
by field against the C rather than assumed uniform, since `do_show`
turns out to be a genuine mix of `page_string` and plain
`send_to_char`. `docs/deviations.md` has the precise per-field list.
Still not wired: `background`'s own `page_string` call, which pages
from `CON_MENU` rather than `CON_PLAYING` and so needs a real design
decision `StatePaging` has so far avoided — see §13.

**The pet shop and the mayor ✅ — the last two of 5a's specials, and now
every stock special the archived world actually uses is built.**
`pet_shops` (`spec_procs.c:951`) is assigned to room 3031 itself rather
than to a mobile — Midgaard's pet shop has no keeper standing in it —
and finds its stock in room 3032 by `IN_ROOM(ch) + 1`, which
`specPetShop` reproduces the same blunt way rather than by any lookup.
One accepted gap: the C also poisons the bought pet's cached
carry-weight/count fields so it can never be given anything; this port
computes those from what a character actually holds, so there is no
field to poison the same way, and a bought pet can in principle carry
an item the real game's pets never could — small and cosmetic, not
worth inventing a mechanism solely to reproduce a cache trick this
port's model does not have.

The **mayor** (mob 3105, `spec_procs.c:277`) turned out not to do what
its own name in the specials list suggests: reading `open_path`/
`close_path` character by character rather than assuming from "opening
and closing the gates" found that neither path string ever reaches the
switch's door-opening cases at all — the two paths differ only in two
lines of dialogue. The port keeps the dead cases anyway, the same
reason the C does (a hand-edited path could still use them), but does
not separately test them, since nothing in the real archive does
either. `MoveMobile` (`internal/game/live.go`) is new: the mayor's
scripted steps needed the same "move a mobile one room" logic `wander`
(`internal/server/mobact.go`) already had inlined, and `session` cannot
import `server` to reach it, so it moved to `game.Live` where both
`server` and `session` can reach it — a smaller `do_simple_move` than a
player's own move gets (no movement-point cost, no boat/tunnel/atrium/
godroom checks), documented as inert against the real data rather than
a risk quietly taken. `docs/deviations.md` has the full writeup of both.

**The improved line editor, all eleven commands ✅ — the gap `tedit`'s
own landing found.** `internal/session/editor.go` ports
`improved_editor_execute` (`improved-edit.c:27`), `parse_action`,
`format_text` and `replace_str`. `/a` `/c` `/h` `/l` `/s` — abort, clear,
help, list, save — landed first, being the five that need no line-range
editing machinery of their own; `/d` `/e` `/f` `/i` `/n` `/r` — delete,
edit, format, insert, numbered list, replace — followed. Anything else
after a leading `/` gets the C's own "Invalid option.", and `/h`'s text
is now the C's own, unedited, because every command it lists works.
`tedit`'s instructions line says what the C's `send_editor_help` actually
said ("/s or @ to save, /h for more options."), which stopped being a lie
the moment `/s` and `/h` became real.

The six needed a `reference/tools/editoracle.c` and would have been wrong
without it. Line-range string surgery is not arithmetic, and it turned
out to be wrong in a different way at nearly every turn: a three-line
buffer has a fourth line, so `/d 4` answers "0 lines deleted."; a buffer
emptied by `/d` is not the same object as one freed by `/c`, and the
guards on `/f` `/i` `/l` `/n` test the pointer; `/n` prints its line
number on a line of its own; a `/r` pattern longer than the buffer wraps
an unsigned subtraction and reports the buffer as full; and a `/ra` that
runs out of room leaves the player's text truncated at the match it gave
up on and then says the string was not found. `docs/weirdnumbers.md`'s
"The line editor" section has all of them with citations; the buffer is
held flat rather than as a `[]string` because that is what the commands
are defined against.

Abort needed a real design decision the plain `@`-terminated loop never
had to make: what to hand back when there is nothing to save.
`beginEditor`/`beginEditorSeeded`'s `done` callback grew a second
argument, `saved bool`, false only for `/a` — the text passed alongside
it is always `""` then, the observable equivalent of the C freeing
`*d->str` and restoring `d->backstr` (`modify.c:170-172`) without this
port ever having captured a "before" text to restore. Every caller
already treated an empty result as "nothing changed" (a note's own
`ActionDesc`, a board post, a piece of mail), so the four call sites
needed no new machinery, only a branch — except each one's *feedback* on
abort turned out caller-specific rather than generic: `tedit` gets
`tedit_string_cleanup`'s own "Edit aborted." plus the room announcement;
mail gets `playing_string_cleanup`'s "Mail aborted." (and, checked
against the same C branch, that message turned out to also cover a
`@`-terminated save with nothing typed — a small, honest fix landing
alongside the main one, since this port had never printed anything
there before); a board post gets "Post aborted." instead of the C's own
"Post not aborted, use REMOVE <post #>.", because that message assumes
the empty-bodied post is already sitting in the board's list — true in
the C, where `Board_write_message` inserts it before editing starts, and
not here, where a post is appended only once the editor closes
successfully (`boardWrite`'s own doc comment has that earlier decision);
a note's own `write` stays silent on abort, matching the C exactly,
since neither of `playing_string_cleanup`'s two branches applies to it.
`docs/deviations.md` has the full writeup, including `/l`'s own range
parsing (a close but not scanf-exact reading of
`sscanf(string, " %d - %d ", ...)`) and why it does not go through the
pager.

**A `select` ban ✅ — corrected, not merely built.** 5i-g's own "ban
enforcement at the name prompt" landed a `select` ban that refused
everybody, the same as `all` — documented in `docs/deviations.md` at the
time as the conservative reading of a half-implemented ban, since nothing
could set `PLR_SITEOK` yet to check it against. `set <name> siteok`
changed that in 5i-e, but placing the *real* check turned out to need
reading `interpreter.c` closely rather than acting on what the earlier
entry already said: `BAN_SELECT` is not checked at the name prompt at all
(`interpreter.c:1482-1490`) — it is checked at `CON_PASSWORD`, after a
password has already been verified, against the *loaded character's own*
flag. `handleGetName` no longer refuses anybody for `select`;
`handlePassword` does, once it knows who is actually logging in. A brand
new character is never touched by it regardless: creation already grants
`PLR_SITEOK` for free (`game.ApplyNewCharacterDefaults`, landed with
Phase 3's character-creation work — itself porting a Disgracelands
`<DoC>` addition, "Sometimes siteok is off for new players",
`interpreter.c:1623`), and a new character never reaches `CON_PASSWORD`
in the first place, so the flag's absence could never have applied to
one. Only an existing record nobody has cleared is ever refused, and
`set <name> siteok` is how an immortal clears one — which
`TestASelectBanChecksSiteOKAfterThePassword` (`internal/server/
bans_test.go`) exercises end to end, refusal and clearance both.

**`show rent` and `show shops` ✅ — the two listings 5i-g left out because
nothing needed them to run.** Neither turned out to need new modelling:
`show rent` (`Crash_listrent`, objsave.c:342) is a thin read of the same
`player.ObjectStore` `internal/server/rent.go` already reads and writes,
resolving each stored vnum's description and rent cost against the live
world's own object prototypes rather than a spent `read_object`/
`extract_obj` pair — `session.Operator` grew `ShowRent`, returning a small
`RentListing` DTO rather than the store's own types, the same seam
`ShowPlayer` already crosses the same way. `show shops` (`show_shops`,
shop.c:1350) needed only a `CustomerString` helper alongside
`internal/game/shopstate.go`'s existing shop rules (`ShopServesRoom`,
`ShopBank`, `BuyPrice`'s and `SellPrice`'s own `ProfitBuy`/`ProfitSell`) —
everything else `list_all_shops` and `list_detailed_shop` print was already
sitting in `ShopDef` from 5f-i.

Reading `buy_price`/`sell_price` right next to `list_all_shops`'s own
`sprintf` turned up a real bug in the stock C, not a port mistake: the
summary table's header reads "Buy" then "Sell", but the values plugged in
are `SHOP_SELLPROFIT` first and `SHOP_BUYPROFIT` second — swapped relative
to their own headings, and the same swap recurs in `list_detailed_shop`'s
"Buy at:/Sell at:" line. Reproduced rather than fixed, per §0, and written
up in `docs/weirdnumbers.md` with both call sites cited — the kind of
finding the fidelity rule exists for, just in a comparison of two
functions rather than an arithmetic one.

Two small, genuine differences, both in `docs/deviations.md`: `show rent`
does not print the rent file's own path, since the player format is
pluggable and there is no one filesystem path a `yaml` store necessarily
has; and `show shops`' detail view does not reproduce
`handle_detailed_list`'s column-wrapping, since nothing in this port's
world data has a `Rooms:`/`Produces:`/`Buys:` list long enough for the
C's own wrap to trigger either.

**`--max-players` ✅ — Phase 3's own remaining gap.** `sockets_connected >=
max_players` (comm.c:1337) is checked at `Accept`'s own accept loop, before
a connection is given a hostname or a per-address slot, matching where the
C makes the same check relative to the rest of `new_descriptor`. `registry`
(`internal/server/operator.go`, already `users`/`dc`'s own live-connection
list) grew a `count()` alongside its existing `list()`, since the check
runs on every single accepted connection and has no use for the ordering
`list()` provides. `Limits.MaxPlayers` is shared across every listener a
server runs, the same way `sockets_connected` counts every descriptor
regardless of which port it arrived on — a telnet connection and a telnets
one both draw from the one pool. Not reproduced: the C's check is
inherently race-free (`new_descriptor` runs to completion before the next
`accept()` in the same single-threaded loop), where this port's listeners
run concurrently, so two connections arriving on two different listeners
in the same instant could both pass the check before either is counted,
overshooting the limit by at most the number of listeners running. A soft
capacity guard rather than a hard security boundary, and not worth
synchronising across listeners for.

**`background` pages too ✅ — the pager's last named caller, and the
design question its own write-up left open.** Every other command this
port paginates runs from `CON_PLAYING` only, which is what let
`StatePaging` get away without an answer to "what was I doing before"
for as long as it did — the C never changes `STATE(d)` while paging
either, so it never had to ask. `background` pages from `CON_MENU`
(`interpreter.c:1712-1714`), and answering the question turned out to
be mechanical rather than a real fork in the road: `Session.pagerReturn`
is what `sendPaged` (`internal/session/pager.go`) captures `s.state` as
before overwriting it with `StatePaging`, and `handlePaging` restores
it — instead of the hardcoded `StatePlaying` that had been correct for
every caller but this one — once the last page is shown or the reader
quits. `menu.go`'s own choice-3 handler sets `s.state = StateReadMOTD`
*before* calling `SendPaged`, matching exactly what the C leaves
`STATE(d)` as once `background`'s own paging returns, so `sendPaged`
captures the right value without needing to special-case this caller at
all.

Two things fell out of generalising this that a version hardcoded to
`StatePlaying` would have gotten wrong silently: the ordinary game
prompt must not appear once paging closes back into a non-`CON_PLAYING`
state (`Session.sendPromptIfPlaying` checks `pagerReturn ==
StatePlaying` before sending one — nobody is playing yet, and a fake
HP/mana/move prompt between menu screens would be nonsense), and
`users`' own listing must show the state paging actually interrupted
rather than a blanket "Playing" — `Session.ConnectedName` consults
`pagerReturn` for exactly that, leaving the pure, session-less
`State.ConnectedName` as the fallback the coverage test's own C-table
comparison still uses.

**Rent files are swept at boot ✅ — `--skip-rent-check` had nothing to
skip until now.** `update_obj_file()` (objsave.c:332) is a boot-time pass
over the whole roster deleting whichever rent or crash file has sat
unclaimed past its own kind's timeout — 30 real days for a rent, 10 for
a crash, forced rent or idle-timeout save. `Server.SweepRentFiles`
(`internal/server/rentsweep.go`) ports it directly on top of
`player.ObjectStore`'s existing `LoadObjects`/`DeleteObjects`, needing no
interface changes at all: `Crash_clean_file`'s per-character logic is a
read, a switch on the rentcode, and a conditional delete, and this port
already had both halves. `RentCryo` has no case in the C's own
if/else-if, so a cryo-frozen character's things are never swept
regardless of age — checked against the C rather than assumed, since
"sweep everything eventually" would have been the obvious wrong guess.
Wired in from `cmd/dlmud/main.go` right after `BootReset`, gated on
`!cfg.SkipRentCheck` the same way the C gates it on `!no_rent_check`
(`db.c:456`) — `--skip-rent-check`'s own entry in `docs/configuration.md`
no longer says *(inert)*.

**`syslog`'s in-game echo ✅ — the seam, its first real producer, and
(#134) every other one.**
`mudlog()`'s second job — echoing a line to online immortals whose level
and own syslog verbosity (`PRF_LOG1`/`PRF_LOG2`) both qualify — had
nowhere to reach a live connection from `internal/obs`, since that package
cannot import `internal/session` without a cycle. `obs.WithWizVisEcho`
wraps whatever `slog.Handler` a caller already had with one that also
calls back into a `WizVisEcho` for any record carrying both `obs.WizLevel`
and the new `obs.WizType` (mudlog's own `type` argument, `OFF`/`BRF`/
`NRM`/`CMP` — a message's own minimum required verbosity, not a level);
`Server.echoWizVis` (`internal/server/wizvis.go`) is the callback,
supplied when `server.New` wraps `opts.Logger` right after building `s` —
after, because `s.connections` is what the callback needs to reach, and
main.go builds the logger before any `Server` exists to hand it to.

The selection is the C's, condition for condition, not approximated:
`STATE(i) != CON_PLAYING || IS_NPC(i->character)` is one check in the Go
too, since a switched god's *current* character is the mobile they are
inside — `IS_NPC` alone already excludes them, and the C has no separate
"switched" check to reproduce. `PLR_WRITING` (mid-edit) is excluded for
the same reason the C excludes it: a log line arriving inside somebody's
own text buffer would be worse than not seeing it. (That check was a
connection-state stand-in until #214 gave the flag a setter.)
`session.SyslogLevel`
(exported from `internal/session/wizcomm.go`, unexported until now since
nothing outside the package needed it) is `do_syslog`'s own two-bits-as-
one-number arithmetic, reused rather than re-derived.

`bug`/`idea`/`typo` (`internal/session/report.go`) is the seam's first
real producer, and needed its own small correction along the way: the
record's own message had been `"<kind> report"` with structured fields,
which is fine for the log file but is not what `mudlog`'s `buf` — `"%s %s:
%s"`, `GET_NAME(ch)`/`CMD_NAME`/`argument` — actually says, and
`WithWizVisEcho` echoes a record's message verbatim, the same string
mudlog's own `str` serves both jobs from. The message is now that exact
format, so what an online, qualifying immortal sees in-game is the real
text, not a placeholder.

**The rest of `mudlog()`'s call sites ✅ (#134, 2026-08-28).** The pass
that finished the above went through all 106 `mudlog()` calls in
`reference/moderncserver/src` and wired up every one in ported territory —
52 call sites in the Go, from `quit` and `purge` through the login
sequence, combat's death line, `Crash_load`'s seven, close_socket and the
zone reset queue.
`Context.wizlog`/`wizlogInvis` (`internal/session/wizlog.go`) and
`Server.wizlog`/`wizlogInvis` (`internal/server/wizvis.go`) are the two
helpers that make a call site one line; `wizlogInvis` carries the C's
much commoner `MAX(LVL_x, GET_INVIS_LEV(ch))` spelling of the level.
`docs/deviations.md` lists what is wired, what is not and why.

Two things the audit found that reading alone would not have.
`mudlog` returns *before* its echo loop when the level is negative
(`utils.c:238-239`), so `do_skillset`'s `mudlog(buf2, BRF, -1, TRUE)`
(`modify.c:344`) means "log it and show nobody" rather than "show
everybody" — `echoWizVis` had no such guard and would have broadcast it
to every mortal in the game. And `interpreter.c:1629`'s "new player" line
fires against a buffer a local `<DoC>` block overwrote three lines
earlier, so the real server logged a duplicate of the "All hail"
broadcast instead. Both are in `docs/weirdnumbers.md`.

The echo itself needed rebuilding to take the new producers.
`Server.echoWizVis` had been deciding who a line reaches inline, on
whatever goroutine logged it, reading live `PlayerRecord`s and session
fields as it went — safe only because `bug` runs as a command, on the
world goroutine already. It queues through `engine.Do` now and walks
`w.Players()` rather than the session list, and `Session.state` became an
`atomic.Int32` because the "mid-edit" exclusion reads it from the world
goroutine. `-race` found the old shape within one run once the login
producers existed.

Six gaps came out of the same pass and are filed rather than written
down: death traps do not kill (#209), the dispatcher reads `Record.Level`
off the world goroutine (#210), `wizlock` does not keep mortals out
because `Server.AllowedIn` is never called (#211), the three `<DoC>` cyan
broadcasts are unported (#212), nothing sets `PLR_KILLER` on an attack
(#213), and nothing sets `PLR_WRITING` either, which leaves four checks
on it dead (#214 — fixed, below).

**`PLR_WRITING` is a real flag ✅ (#214).** The C sets it in
`string_write` (`modify.c:100-101`) and clears it in `string_add`'s
cleanup (`:218-219`), so anybody in the line editor carries it. Nothing
in this port set it, and every check on it was dead: `tell` never
refused, the channels and `wiznet` never skipped, the room never said
"(writing)", `gain_condition` never held its tongue, and the syslog echo
had to stand in a connection-state check. `Session.beginEditor` sets it
now and `finishEditing` clears it, and `mail` sets `PLR_MAILING`
alongside as `do_mail` does (`mail.c:567`).

The awkward half was the clearing. Everything a *command* touches is
already serialised by `Dispatcher.Do`; the line editor is the one thing
a playing character drives from the session's own goroutine, line by
line, and its cleanup writes world state — the flag, a board's message
list, a note's action description. In the C all of that runs inside
`string_add`, in the game loop, like everything else. So
`CommandHandler` grew an `InWorld` hop and `finishEditing` runs its
whole cleanup through it, which fixes the board and note writes at the
same time as the flag.

Two findings came with it. `wiznet @`'s "(Writing mail)" arm can never
fire, because `do_mail` sets `PLR_MAILING` and then `string_write` sets
`PLR_WRITING`, and the site tests writing first — while `do_who` tests
the same two bits the other way round and so has the opposite dead arm.
Both orders are reproduced; `docs/weirdnumbers.md` has the write-up. And
the C's menu option 2 sets `d->str` and `CON_EXDESC` **directly** rather
than calling `string_write` (`interpreter.c:1706-1709`), so the
description editor never sets the flag on the real server either — which
is why this port does not set it there.

One gap came out of it: `do_who` prints none of its annotations at all,
not just the writing one (#216).

**`wizlock` keeps mortals out ✅ (#211).** Nanny tests `circle_restrict`
twice — `if (circle_restrict)` at CON_NAME_CNFRM (`interpreter.c:1421`)
and `GET_LEVEL(d->character) < circle_restrict` at CON_PASSWORD (`:1491`)
— and neither had anything calling it, so `wizlock 32` closed the game to
new names and let every mortal on the roster walk straight in.
`Server.AllowedIn` existed for the second and was reached only by its own
test; `NewCharactersAllowed` is new for the first, which replaces a
creation error dressed up as "Something went wrong creating your
character." with the C's own "Sorry, new players can't be created at the
moment.". Both `mudlog` lines came with them; what is
left unwired from #134's audit is `check_killer`'s pair, which needs
#213 first, and the 42 that belong to OasisOLC. `-r` folded into the same field, as the C has it
(`comm.c:329`): one global, so `wizlock 0` reopens a server started
restricted.

**The dispatcher's level read is on the world goroutine ✅ (#210).**
`Dispatcher.Do` made three reads of world state on the connection's own
goroutine before it entered `engine.DoSync` — the level the typed word is
matched at (`interpreter.c:623`), the wait state, and the numbers in the
prompt — and `advance` writing `GET_LEVEL` from `gain_exp_regardless`
(`limits.c:357`) raced the first of them. Two round trips now: one to read
what `command_interpreter` looks at before it runs anything, and one to run
it. The wait stays outside both, because it is a sleep and the world
goroutine must not sleep.

**The `<DoC>` cyan broadcasts are all in ✅ (#212).** Four of them, all
through `send_to_all_color` (`comm.c:2256`): the new-player hail
(`interpreter.c:1608`), the level-gain pair (`limits.c:311/318` and their
drifted copies at `:368/375`), the death trap (done with #209) and
`remort` (`act.wizard.c:465`), which was the only one reaching players at
all and was doing it through the *uncoloured* `send_to_all`. One
implementation now — `game.Live.Announce` — which is what carries the
colour threshold and the `PLR_WRITING` exclusion the C applies, the
latter live only because #214 made the flag real.

Two things came out of doing it. `gain_exp`'s `if (is_altered)` block
also says **"You rise a level!"**, and this port said it at one of its
three call sites: a kill did, the cityguard's award and `advance` did
not. All three go through `Live.AnnounceLevelGain` now, so they cannot
drift the way the C's own two copies did — and those two copies really
did drift, one whisper ending in `\r\n` and the other not
(`docs/weirdnumbers.md`, "`gain_exp` and `gain_exp_regardless` are copies
that drifted"), which is reproduced rather than tidied.

**Death traps kill now ✅ (#209).** `do_simple_move`'s closing
`log_death_trap(ch); death_cry(ch); extract_char(ch);`
(`act.movement.c:171-176`) is `Context.deathTrap`. Both halves of
`log_death_trap` came with it, which settled the third of #212's cyan
broadcasts; the rest followed above. `death_cry` moved to `game.Live.DeathCry` so that the
combat round and the trap share it; the trap does not otherwise touch
`die()`, because the C does not.

**`native` is `yaml` throughout ✅ — a rename, not a redesign, because the
name stopped being true.** §5.7/§5.8/§6.3 already called the format
`native`, back when it was the only alternative to `classic`/`ascii`/
`binary`; `docs/design/data-format.md` §0 had said "the format registers
as `yaml`" since it was written, and the code had simply never caught up.
Every `internal/persist/*/native` package is `*/yaml`; every
`--*-format=native` value, `FormatName` constant and `case "native"`
branch is `yaml`; `internal/game/nativenames.go` is `yamlnames.go`.
Mechanical, but not risk-free — "native" and "yaml" both appear as
substrings inside other words (`alternative`, `natively`, and this
document's own §4 "a native 64-bit build", about the ILP32/LP64
discussion, nothing to do with the data format at all), so a first pass
done as a bare word-boundary substitution corrupted several of them,
found and reverted by diffing against what the words were before rather
than trusting the regex.

**Versioning the yaml format ✅ — `docs/design/data-format-versioning.md`,
a `major.minor.patch` stamp for a data directory as a whole, distinct
from §10.1's existing per-file `schema: dl/<kind>@<major>` tag.**
`internal/persist/dataversion` reads and writes `.dlversion` at a data
directory's root. The number stamped is **the release semver of the
build that wrote it**, from `internal/buildinfo` — derived rather than
hand-maintained, which is the whole argument (§1.1 of the design doc):
a format version somebody has to remember to bump is one that fails
silently in exactly the direction the mechanism exists to catch.
`dlctl import` (no `--type`) stamps `--to-dir` once every step has succeeded, and
`dlmud`'s boot sequence compares that stamp against its own release —
**a differing major refuses to start, in either direction; a differing
minor logs a warning and starts anyway; a differing patch, or no stamp,
says nothing.** Both comparisons are on difference, not newer-ness: a
major bump means the two builds disagree about what the files mean, and
disagreement has no direction. `dlctl data version --dir=X` answers the
same question offline, and `--write` restamps — the migration path
across a major bump, and the adoption path for a directory that predates
the mechanism.

Named as real, current gaps rather than papered over: an **unreleased
build** (`go run`, `go test`, plain `go build`) has no version of its
own, so it stamps nothing and enforces nothing — accepted deliberately,
because a development build guessing at a version and then enforcing it
would be worse than not checking (design doc §6); the **minor-version
warning is generic**, and usually a false alarm, because most releases
do not touch the format at all; and **nothing migrates the files** across
a major bump, only the stamp.

**`dlctl import` ✅ — the seven format-specific importers, run
together against one `lib/`-shaped source, in one command.**
`import --type=world`/`pfile`/`state`/`names`/`messages`/`socials`/
`help`, run in that order against `--from-dir` (a base directory `dlctl`
resolves each `--type`'s own subpath under), plus `text/`'s plain-prose
files copied unchanged (never a pluggable format) and, once every step
has actually succeeded, a `.dlversion` stamp naming this build's release
written into `--to-dir` (nothing, from an unreleased build).
Verified against `examples/stock/`, not a synthetic fixture: regenerating
`examples/stock/yaml` from `examples/stock/binary` and diffing every file
against what is checked in is what proved the recipe, not an assumption,
and is now a standing test (`TestImportMatchesTheCheckedInExamples`).

Writing the getting-started walkthrough this landed with
(`docs/operations.md`) found a real gap rather than assuming the seven
importers were uniform: only `import --type=world`/`pfile` transcoded
non-UTF-8 source text on their own (`--encoding`, the same flag `dlctl
convert` uses); the other five assumed the source was already UTF-8 and
carried a raw CP1252 byte straight through into a `.yaml` file that then
claimed to be UTF-8 and was not. `examples/stock/`'s own world is pure
ASCII throughout, so nothing here had ever exercised the gap — found with
a synthetic fixture instead, a curly quote fed to `import --type=names`
and inspected byte for byte in the output. **Fixed since**: all seven
`--type`s take `--encoding` now, each decoding the specific free-text
fields its own format actually carries (see `docs/design/data-format.md`
§11.1 for the full field-by-field account) — `TODO.md`'s own entry for
this moved from "still open" to "superseded" accordingly.

**Phase 7 — Cutover. Not started.** The one honest complication this
section's original two sentences skipped over: "cutover" ordinarily means
swapping a *running* service, and there is not one. Disgracelands stopped
being played in 2008; neither the C tree nor the Go port has taken a real
connection since. What this phase actually has to answer is narrower and
more concrete than "switch the primary server" — it is *the decision and
the work that would need to happen before this could ever be exposed to
real players again*, whether that is a revival with the archived
2001–2008 characters or a fresh start.

Preconditions — what has to be true before the rest of this phase makes
sense to start:

1. **World parity clean.** Already true, checked at every release
   (`release.yml`). Not a gate that needs new work, just the one that
   must keep passing.
2. **Session parity triaged, not just run.** *Done, in the sense this
   precondition asked for — and it is the "triaged" half that is done,
   not the "agree" half.* `test/parity` (2026-08-25) plays ten
   scenarios at both servers, covering combat, shops, boards, mail end
   to end and housing as this precondition asked, and every difference
   it finds is now either absent or an entry in its own triage table
   pointing at `docs/deviations.md` ("What the session-parity suite
   found"). It is green, and green means *decided*, not *identical*:
   twenty-odd real differences are written down there, two of which
   are the 64-bit reference build rather than the port. Cutover does
   not need them all fixed; it needs somebody to have read that list
   and decided which of them a returning player would not forgive.

   **That reading has now happened (2026-08-26), and it is what makes
   this precondition actually met rather than merely prepared for.**
   Eighteen of the twenty differences needed a ruling — the other two
   are the 64-bit reference build being wrong rather than the port — and
   each carries its own in `docs/deviations.md`. Sixteen are
   **blockers**: fix before
   cutover. One is **later**: objects list in a different order from the
   C, a consequence of where this port inserts into its lists, harmless
   except that ordering is what `2.sword` selects against. One is
   **accepted**: the suite does not compare blank lines, because the C
   prepends a CRLF to any output interrupting a prompt (comm.c:1459) and
   reproducing that solely to compare whitespace buys almost no
   evidence — the one place the harness is knowingly blind, bounded and
   written down.

   Sixteen blockers is a larger number than "the port is playable"
   suggests, and a smaller job than it sounds: most are one command
   each. The ones that are not are the three this paragraph used to
   single out — `quit` returning to the menu, movement points never
   being charged, and call-site colour, which is mechanical but has a
   great many call sites — plus two that are bugs rather than gaps and
   should be read as such: a level 1 mortal's hit points are rolled
   from a formula that has been read wrong (the generators are in step,
   so it is arithmetic, and CLAUDE.md says what that wants — an oracle,
   not another reading), and `game.Act` sends the killer's own death
   line twice while sending no `death_cry` at all.
3. **The real archive's non-ASCII text survives conversion.** The
   importer gap this precondition was first written about is closed —
   all seven of `dlctl import`'s sub-importers take `--encoding` and
   transcode now (`TODO.md`'s "Superseded", `docs/design/data-format.md`
   §11.1). What is not closed is the exercise: `examples/stock/`'s own
   world is pure ASCII, so nothing here has ever run the conversion
   against data that matters — the real archive, twenty years old, is
   not ASCII throughout. Only relevant if reviving the archived roster
   (below); irrelevant to a fresh start.
4. **A decision, not a technical task: revive the archived roster, or
   start clean?** `TODO.md` §1 — the 108 real 2001–2008 characters exist
   only in the private archive and have never touched this repo. Reviving
   them means running the real conversion end to end at least once
   (which is what makes precondition 3 load-bearing rather than
   theoretical); starting clean means none of this phase's work is
   roster-shaped at all. Nothing else in this plan can answer this one —
   it is not a technical question.
5. **Exposure hardening actually done, not just designed.** §7 above
   describes per-address limits, a handshake timeout and the ban list as
   built; `docs/operations.md`'s own "Exposure" section is where the
   honest current state lives — check it directly rather than trusting
   this paragraph's summary of it, since it is exactly the kind of status
   line that goes stale between phases (its own `--max-players` line
   already had, once — see this phase's own landing for that fix). A
   real deployment needs a real hosting decision (this plan does not make
   one), TLS from a real certificate rather than a dev one, monitoring
   actually wired to something a person looks at, and a backup schedule
   for whatever `--lib-dir` the deployment uses — `docs/operations.md`'s
   own "Backups" section says what that means, `cron` and off-host
   storage are not built here.
6. **At least one tagged release exists.** `make release` (`docs/
   proposals/go-port-plan.md` §9.1's config work, `scripts/release.sh`)
   is what makes "deploy the Go server" mean something concrete rather
   than "deploy whatever `main` happens to be."

The cutover itself, once the preconditions hold:

1. **Stand up hosting** — a decision this document defers to whoever is
   actually doing it, not a default this plan should pick.
2. **Deploy the tagged release** there, pointed at a real `--lib-dir`
   (freshly converted from the archive, or an empty one for a fresh
   start — see precondition 4).
3. **Run both servers in parallel for a defined soak period.** With no
   real traffic to mirror, "shadow-run" means: play both, side by side,
   using the session-parity scripts as the acceptance floor and manual
   testing above that, watching the Go server's own metrics
   (`dlmud_pulse_duration_seconds` especially — §9.2) for anything a
   short local test would not surface.
4. **Go/no-go.** Any new parity difference found during the soak either
   gets fixed and the soak repeats, or gets triaged into
   `docs/deviations.md` the same way every other one has been — cutover
   is not blocked on zero differences forever, it is blocked on every
   difference being a *decided* one.
5. **Cut over.** Point whatever the real listener/domain is at the Go
   server exclusively. Keep the C tree buildable and runnable — it stays
   the reference implementation for any future fidelity question
   (`docs/deviations.md`'s own framing already assumes this, not
   something cutover changes) — but not serving connections.
6. **Retire `autorun`/`automaint`/`configure` from the operational
   path.** Already effectively decided (`TODO.md`'s "Superseded"
   section): the container runtime's restart policy plus real `SIGTERM`
   handling replaces what they did. This step is "stop running them,"
   not new work — they stay in `reference/moderncserver/` as what they
   always were, historical operational scripts for a tree this port
   is not the one running anymore.

Rollback, if cutover needs to be undone: the C server tree is still
buildable and `--lib-dir` is the same directory either format reads
(§13's "the on-disk contract" note) — falling back is starting the C
binary against the same data, not a migration in either direction.

Not part of this phase, and worth saying so rather than leaving it
implicit: this plan does not decide *whether* Disgracelands gets revived
for real players at all. That is a decision for whoever owns that
choice, on their own timeline. What this phase describes is what would
have to be true first, if and when that decision is made.

**Later (explicitly not v1):** a scripting *interpreter* behind the §8 seam —
the seam itself is Phase 5a and the built-in specials are its first
consumers — copyover/hot-reboot, the web client, additional persistence
backends, the WipeMud race system (`TODO.md` §2).

---

## 11. Testing and parity

- **Unit tests** for every format codec, with the real `data/` data as
  fixtures where it isn't player data, and synthesised fixtures where it
  is. Fuzz the world parser and the binary pfile decoder — both consume
  untrusted-ish input and both are exactly the kind of code where a
  malformed length field becomes a panic. *Built.*
- **C oracles for anything numeric.** Not in the original plan, and the
  single technique that has caught the most real mistakes: `reference/tools/`
  holds original C function bodies with the `char_data` dereferences
  substituted and nothing else changed, compiled by the Go tests and compared
  across the input space. Where a table is transcribed rather than computed,
  the test re-parses the C source instead — `class.c`, `constants.c`,
  `interpreter.c`, `spec_assign.c`, `handler.c`, `spells.h` and `act.wizard.c`'s
  `set` table are all compared that way rather than asserted about. See
  `docs/weirdnumbers.md` for
  why reading the arithmetic across is not good enough. *Built, and the rule
  now is that anything with a division, a cast or a comment describing numbers
  gets one.*
- **Session tests against a real socket.** Each command is exercised through
  the whole stack — telnet parser, login, dispatcher, world goroutine — by a
  test client that dials the listener. Slower than calling the function, and
  it is how the port's worst bugs were found: output escaped where it should
  not have been, a panicking command leaving a player with no prompt, a
  recover in the engine swallowing a test's own assertion. *Built,
  `internal/server/*_test.go`.*
- **Golden-file tests** for command output, matching the C server's text byte
  for byte. *Not built.* The session tests assert the strings inline instead,
  which is the same idea with worse ergonomics and no diff.
- **A scripted-session harness**: a list of commands in, transcript out, run
  against **both** servers and diffed. *Built, and then built again.*

  The first version — `scripts/session-parity.sh`, `internal/parity` and
  `dlctl parity session` — boots both servers on throwaway copies of the
  data directory with the same fixed RNG seed, plays a script at each,
  normalises away the handful of things two servers can never agree on, and
  diffs. It found the message of the day for a brand-new implementor (fixed
  in the change that built it) and **colour** — the C emitted it and this
  port emitted none — which brought three more with it, each invisible until
  the transcripts were laid side by side: **`compact` was another settable,
  listed, saved preference that nothing read**, so the blank line before the
  prompt was always there; **the room's people are listed newest-first**,
  because `char_to_room` prepends; and **`score` was missing "It's your
  birthday today."**.

  Then it sat, with one script in it and a list of differences nobody could
  read. **`test/parity` (2026-08-25) is the second version**, and the three
  things it changed are all about making a difference legible rather than
  about finding more of them:

  - **The mobiles are held still on both sides** — `-M` in the C, a `<DoC>`
    addition alongside `-S`, and `--freeze-mobiles` here. This was named
    above as "the next thing this harness wants" and it was: a janitor's
    position depends on how many pulses have elapsed since boot. It also
    stops `mobile_activity` rolling dice, which is what makes **a fight
    comparable round by round** — the previous limitation, "the two servers
    do not consume the RNG sequence in the same order", was a consequence of
    the mobiles moving rather than a fact about the two servers.
  - **Transcripts are compared a command at a time.** A single extra blank
    line in the login sequence used to report every line after it as
    differing too: forty findings, of which thirty-nine were consequences of
    the first.
  - **Every difference is either fixed or written down.** The suite carries
    a triage table of patterns for lines the two servers are allowed to
    disagree about, each pointing at its entry in `docs/deviations.md` — and
    an entry that stops matching anything fails the suite with "delete it",
    so the list shrinks as things get fixed instead of becoming a record of
    what used to be wrong.

  Ten scenarios: creation and first impressions, colour, objects, a fight,
  shops, banking and the inn, boards, mail end to end, houses, and what a
  mortal is refused. What that found is in `docs/deviations.md` under
  "What the session-parity suite found" — twenty-odd differences, including
  two that are the *reference build* rather than the port (this C is 64-bit,
  so its shop prices are off by one and its mail system cannot store a
  letter at all).

  **In neither workflow, on purpose**, including `release.yml`: it needs a C
  toolchain, starts two servers per scenario and frames answers by silence.
  `make session-parity` runs it. See `docs/developer.md`.

  The C server needed three additions to make this possible, all marked
  `<DoC>`: `-S <seed>` fixes the RNG seed, `-M` holds the mobiles still, and
  `-J` (Phase 1's) dumps the world. Zero keeps `time(0)` and an ordinary
  boot is unchanged.
- **Property tests** on the numeric core — combat damage, experience,
  saving throws — asserting no overflow and no negative-where-impossible
  across the full input range, which is where the 64-bit work either holds
  or doesn't. *Partly: the oracle sweeps cover the ranges, but as equality
  against the C rather than as properties.*
- **A deviations log** ([`docs/deviations.md`](../deviations.md), written in
  Phase 3 rather than Phase 1 — there was nothing to record until the server
  ran): every intentional difference from the C behaviour, with the C line
  reference, what it did, what Go does, and why. Under the "fix known bugs"
  fidelity decision this file is the deliverable that keeps "fixed a bug"
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

`scripts/license-check.sh` runs at every release (`release.yml`, not the
day-to-day `go.yml`) and verifies the five requirements
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
   those headers carry across to the corresponding Go files.
   `internal/persist/player/ascii` is the first case and does: its notice
   credits ascii_pfiles 2.1 to Alan K. Miles, after Chris Jacobson's
   original. OasisOLC's `gen*.c` layer is the next one due, whenever the
   online-building phase lands.
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

1. ~~**How faithful does OLC need to be**~~ **Settled: not built at all.**
   Decided against, in favour of `reloadmob`/`reloadzone` — edit the world
   data directly (by hand or via `dlctl import --type=world`/`fmt`), reload it
   into the running server without a restart. `Sink`/`WriteZone` (§6.3)
   are unaffected either way; nothing in this tree just happens to drive
   them from an in-game menu.
2. ~~**What does reloading an object or a shop mean?**~~ **Settled:
   refresh nothing an instance could have diverged from.** The question
   was real — `Object`'s shadow fields (`Keywords`/`ShortDesc`/
   `Description`/`Type`/`ExtraFlags`/`WearFlags`/`Values`/`Weight`/`Cost`/
   `Affects`/`PermAffect`) hold player-relevant state a mobile's
   disposable `Record` does not: a wand's remaining charges, a
   container's lock state, a light source's hours left. `ReloadObject`
   (`internal/game/reset.go`) answers by touching only the prototype —
   existing instances keep whatever they currently are, and pick up only
   the handful of fields `Object` already read live from `Def` before
   this (`Spec`, `MinLevel`, `RentPerDay`, the fallback `ActionDesc`); a
   fresh spawn gets the new definition in full. A shop turned out simpler
   than expected, for a reason not fully anticipated here: `shopState`'s
   own comment already says "there is exactly one of each shop", so
   there is no shared-prototype-versus-instance question for it at all —
   `ReloadShop` copies the whole configuration across and leaves only the
   till (`Bank`/`Sorted`) alone. `reloadobj <vnum>` and `reloadshop
   <vnum>` are their own commands, mirroring `reloadmob`'s original shape
   rather than folded into `reloadzone`'s sweep — see Phase 6's own
   write-up for the full account.
3. ~~`config/game.yaml` (§6) — deliberately set aside.~~ **Decided and
   built, 2026-08-23**, and moved into the data directory it configures
   2026-08-28, where §6 had put it all along: §9.1 and `docs/deviations.md`
   have what moved and why. Two behaviours picked for tunability turned out not to exist yet
   at all (`max_bad_pws`'s disconnect, `tunnel_size`'s occupancy limit) —
   `docs/deviations.md` covers those as their own gap, separate from the
   config-file work. `max_bad_pws`'s half was built on 2026-08-28 (issue
   #135), `GameTuning.MaxBadPws` added with it; `tunnel_size` is still
   open.
4. ~~**Does `background`'s own pager need wiring up?**~~ **Settled: yes,
   and the design cost was smaller than it looked from here.** The
   worry was that `StatePaging` had never had to answer "what state do I
   return to when the pager closes," since every other call site's
   answer was always "back to playing." `Session.pagerReturn` answers it
   generically — `sendPaged` captures `s.state` before overwriting it
   with `StatePaging`, `handlePaging` restores it — rather than needing
   a `background`-specific case at all; `menu.go`'s own choice-3 handler
   just has to set `s.state = StateReadMOTD` before calling `SendPaged`,
   matching what the C leaves `STATE(d)` as once `background`'s own
   `page_string` call returns. See Phase 6's own write-up for the two
   things a naive version would have gotten wrong silently (the ordinary
   game prompt appearing where it should not, `users` misreporting the
   state).

All the others are now decided; see §0.

**Settled since this list was written:** `data/` **stays the on-disk
contract**. Both servers read the same directory. That is what the
world-parity harness compares against and what the Phase 7 shadow run
depends on, and it is already how every phase built so far works. The
constraint it imposes — the layout is the C's, not one chosen fresh — is
accepted deliberately; `dlctl convert` is where the modernisation happens,
in place, rather than in a second directory shape to keep in sync.

---

## Related documents

Operator documentation for what has actually been built lives in `docs/`
itself — `docs/configuration.md` and `docs/operations.md`.

[`docs/deviations.md`](../deviations.md) records every intentional difference
from the C server, which is the other half of the fidelity decision in §0.

Background this plan draws on, all under `docs/investigations/`:

- `pfile-conversion.md` — the binary→ascii conversion tools and what was
  verified; the groundwork §5 builds on.
- `ascii-pfile-format.md` — the field-by-field ascii format spec; the
  implementation reference for §5.4.
- `non-stock-features.md` — the definitive list of what a "faithful" port
  has to reproduce.
- `circlemud-archive-report.md` — why this tree and not the other one.

And outside `docs/`:

- `TODO.md` — what's left that is not a phase: the roster decision,
  WipeMud, the world snapshot, hosting and exposure, and the one
  remaining C-tree item. The roster and exposure ones are this plan's
  business too (Phase 7's preconditions); WipeMud, the snapshot and
  `src/util/*` are not.
- `BUILDING.md` — both builds, C and Go.
- `reference/moderncserver/doc/license.doc` — the CircleMUD + DikuMUD
  licenses the port inherits; see §12.
