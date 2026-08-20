# A native data format for `data/`

A single file format for everything the Go server reads and writes: the
world, the players, the player-adjacent state that a MUD accumulates
around them, and the game tuning that is currently compiled into
`config.c`. It replaces the eight-or-so unrelated on-disk formats a
CircleMUD `lib/` directory carries — some text, some struct dumps, none of
them documented in the same place — with one, and it is a superset of all
of them: everything they can express, it can express, and it can express
more.

This is a proposal. Nothing described here is built. It slots in behind
the format registries that already exist (`internal/persist/world`,
`internal/persist/player`), as one more registered driver alongside
`classic`, `ascii` and `binary`, and it becomes the default once it can
load the world with byte-identical parity against the C server.

---

## 0. Decisions taken

These were settled before this document was written and the rest of it
assumes them.

| Question | Decision |
|---|---|
| **Scope** | **Everything.** World, players, rent, aliases, boards, mail, houses, socials, damage messages, help, screen text, banned names and game-rules configuration. One format, one `data/` tree. The point is that "point it at `data/` and run" is true without exceptions. |
| **Identity** | **Vnums stay canonical, and stay integers.** A room is `3001`. No string IDs, no namespaces, no translation layer. Extensibility comes from adding fields, not from changing what a record is called. |
| **Layout** | **One file per zone, holding everything in that zone** — the zone header, its resets, and every room, mobile, object and shop whose vnum falls in its range. A zone is the unit builders own and the unit OLC writes back. |
| **Surface syntax** | **YAML 1.2 over a JSON data model.** Files are `.yaml`. The data model is strictly JSON-compatible, so JSON Schema, the existing parity dump and GMCP all keep working, and any `.json` file is accepted verbatim — YAML is a superset of JSON. |
| **Prose** | **Stays prose.** Help entries, the MOTD, credits, news, policies and the greeting screens are plain UTF-8 text files, indexed from the data format rather than embedded in it. |
| **Colour** | **Symbolic `&` codes in the data, ANSI rendered at the socket.** Forced rather than chosen: a raw ESC byte is not a legal character anywhere in a YAML stream. Export to `classic` renders codes back to the raw escapes `screen.h` defines, which is what the C server expects. See §5. |

Naming: the format registers as **`native`**. `--world-format=native`,
`--player-format=native`.

---

## 1. Why replace `lib/` at all

`data/` today is CircleMUD 3.0 bpl20's `lib/`, unmodified. It works, the
`classic` loader reads it faithfully, and `scripts/world-parity.sh` proves
the Go and C loaders agree on all 3,202 records. So the case for replacing
it has to be made, not assumed.

**It is not one format, it is eight.** A `lib/` directory contains: the
`#vnum` / `~` / letter-bitflag world files; the `zone.lst` index; the
positional shop file with its `-1` list terminators; the tag-per-line
ascii player files; the `struct char_file_u` binary player database; the
`objsave.c` rent files, which are struct dumps with a header; the board
files, which are a different struct dump; the mail spool, which is a
linked list of 100-byte blocks written to a file; `hcontrol`, which is an
array of `struct house_control_rec`; and the socials and damage-message
files, which are two more bespoke line-oriented formats that share nothing
with each other. Each needs its own parser, its own writer, its own tests
and its own corruption modes — which is no longer a prediction: Phase 5
ported four of them, and they are `internal/persist/player/binary`,
`internal/persist/player/binary/objfile.go`, `internal/persist/boards`,
`internal/persist/houses` and `internal/persist/mail`, each with its own
`layout_test.go` checking a struct layout against `gcc -m32`.

**Half of it cannot survive a text conversion.** `dlctl convert` already
has to detect the struct dumps and copy them verbatim, because a
byte-level transcode of a `struct` corrupts it twice over — once by
rewriting bytes that were never characters, and again by changing the
length of text whose length is stored in a separate field. So the current
plan converts what it can and carries the rest as opaque blobs that only a
C compiler with the right `sizeof(long)` can read. That is not a
migration, it is a deferral.

**The struct dumps are 32-bit artefacts.** Rent files, board files and
`hcontrol` all embed `long` and `time_t` at whatever width the machine had.
`docs/proposals/go-port-plan.md` §4 exists because that width changed
underneath this game. Every one of those files has a 2038 problem and a
`sizeof` problem, and no amount of careful reading fixes a format whose
meaning depends on the compiler that wrote it.

The board format is the sharpest illustration, and the port's own package
comment says it plainly: a board file is a count followed by, for each
message, a raw `struct board_msginfo` — which contains a live `char *heading`
**pointer**, written straight to disk. The value is meaningless the moment
the process exits and the loader ignores it, but its *width* decides where
every subsequent field sits: four bytes on the i386 build the archive came
from, eight on any 64-bit rebuild. A format that cannot be read without
knowing which compiler wrote it is not a format, and this is the argument
for the whole proposal in one field.

**The positional formats cannot be extended.** Adding a field to the
object format means adding a number to a whitespace-separated line, and
every parser that counts fields has to be updated in lockstep — which is
precisely why `min_level` is an optional fourth number on a line that used
to have three, and why the loader has to accept both. The next field has
the same problem, and the one after that.

**It loses information the game already has.** Reset commands are the
clearest case: `E 1 3022 100 16` means "if the previous command succeeded,
equip the last-loaded mobile with object 3022, up to 100 in the world, in
wear position 16". Three of those four numbers are opaque, the dependency
on the previous command is a bare `1`, and which mobile "the last one"
refers to is implicit in file order. Builders get this wrong constantly and
the file cannot tell them they have.

None of that makes `classic` a bad format for 1993. It makes it a bad
format to build a 2026 server's future on, and the repo has already decided
the principle: **old formats are inputs to a converter, not things the live
server carries.** This proposal applies that principle to the other seven.

---

## 2. Why YAML, and what that does and does not mean

### 2.1 The data model is JSON

Everything in this format is a JSON value: objects, arrays, strings,
numbers, booleans, null. No YAML anchors, no aliases, no merge keys, no
tags, no non-string mapping keys, no multi-document files. The reason is
that the JSON data model is what the rest of the system already speaks —
the parity dump in `internal/persist/world/dump.go`, GMCP, and any future
HTTP API — and having one model with two surface syntaxes is enormously
cheaper than having two models.

A consequence worth stating plainly: **every `.json` file is a valid file
in this format**, because YAML 1.2 is a strict superset of JSON. A tool
that would rather emit JSON may do so and the loader will not notice.

### 2.2 The surface syntax is YAML, for exactly one reason

Multi-line strings. This is a game made of prose — 3,202 records, most of
which carry at least one paragraph, plus a help corpus, plus screen text.
JSON has no multi-line string, and the two workarounds are both bad: one
escaped `\n`-laden line per description makes every `git diff` useless, and
an array of one string per line makes editing a paragraph a matter of
minding quotes and commas on every line.

YAML block scalars put the prose in the file as prose:

```yaml
desc: |
  You are in the southern end of the temple hall in the Temple of Midgaard.
  The temple has been constructed from giant marble blocks, eternal in
  appearance, and most of the walls are covered by ancient wall paintings
  picturing Gods, Giants and peasants.
```

That is the whole argument. Comments are a nice secondary benefit on the
files the server never rewrites, and irrelevant on the ones it does — see
§2.4.

### 2.3 What was rejected

| Candidate | Why not |
|---|---|
| **JSON with line arrays** | Zero new dependencies and trivially round-trippable, which is genuinely attractive. Rejected because builders hand-edit this data, and "edit a paragraph" should not mean "re-quote four lines and mind the commas". |
| **TOML** | Healthy Go libraries and no ambiguity anywhere, and it would be the right answer for flat configuration. A zone is not flat: rooms contain exits contain descriptions, objects contain affects and extra-descriptions. At 700 rooms in one file, `[[rooms.extra_descs]]` tells you the shape but not which room you are inside. |
| **JSON5, HJSON, KDL, NestedText** | Right ergonomics, no Go library any of us would want to bet the world data on — and the *writers* are consistently weaker than the readers, which matters more here because the server writes these files. |
| **CUE** | Real multi-line strings and a first-class Go implementation, but it is an evaluated configuration language. Excellent as a schema layer over this format later; wrong as the storage for 3,202 bulk records a program rewrites. |

### 2.4 The honest caveats

**Comments do not survive machine writeback.** When OLC rewrites a zone
file, hand-written comments in it are gone, in this format and in every
alternative above, short of node-level AST editing that is not worth
signing up for. So: files the server writes (world zones, players, boards,
mail, houses) may carry comments, but nobody should rely on them. Files the
server only reads (game configuration) keep comments permanently. This
should be documented where builders will see it, not buried here.

**YAML's type-inference footguns mostly do not apply**, because we decode
into typed Go structs rather than `map[string]any`. A field declared
`string` receives `no` as `"no"` and `3.0` as `"3.0"`; the Norway problem
is a problem for schemaless decoding, which we are not doing. Strict
decoding (unknown-field errors) is on by default — see §10.

**The residual risk is on the writer, and it is real.** MUD text contains
trailing whitespace, leading spaces (ASCII art in help files and the
`screen` file), and tabs. A block scalar cannot represent a line with trailing
whitespace, and emitters in the libyaml lineage correctly fall back to
double-quoted style when they see one. That fallback is *correct but ugly*,
and the failure mode if an emitter gets it wrong is silent text corruption.
This gets a round-trip fuzz test over the entire real corpus before the
format is declared load-bearing, and §10 makes it a CI gate. Colour is the
fourth member of that list and the one that cannot be handled this way at
all — a raw escape byte is not merely awkward in YAML, it is illegal, which
is what §5 is about.

### 2.5 Library

`goccy/go-yaml`. `gopkg.in/yaml.v3` last shipped **v3.0.1 in May 2022** and
is effectively unmaintained; `goccy/go-yaml` was at **v1.19.2 in January
2026** and is actively released. This would be the Go tree's first
non-trivial third-party dependency outside Prometheus and `golang.org/x`,
which is a cost worth naming — it buys the multi-line strings that are the
entire reason for the format choice, and its blast radius is one package.

---

## 3. The `data/` tree

```
data/
  data.yaml                  Format version, and what this directory is.

  world/
    0-limbo.yaml             One file per zone. Filename is a convenience;
    12-mount-moria.yaml      the vnum inside the file is authoritative.
    30-midgaard.yaml
    ...
    sets.yaml                Named subsets of zones (`mini`, for --mini-mud).

  config/
    game.yaml                Rules tuning: config.c, made data.
    messages.yaml            Damage/skill messages (was misc/messages).
    socials.yaml             Socials (was misc/socials).
    names.yaml               Disallowed names (was misc/xnames).

  text/
    motd.txt                 Prose, as prose. UTF-8, no wrapper.
    imotd.txt
    greetings.txt
    credits.txt
    news.txt
    policies.txt
    handbook.txt
    background.txt
    screen.txt
    help/
      help.yaml              Index: keywords, level, and the file for each entry.
      ac.txt                 One entry, one file.
      alias.txt
      ...

  players/
    a/aragorn.yaml           One player, one file — and *everything* about
    b/bilbo.yaml             that player is in it. See §8.
    ...

  state/
    boards.yaml              Message boards.
    mail.yaml                Undelivered mail.
    houses.yaml              House control.
    reports.yaml             bug / idea / typo submissions.
```

Three structural rules, applied everywhere:

**The directory is the index.** There is no `zone.lst`, no `plr_index`, no
`index` file in `help/` listing the other files. Zones load in ascending
vnum order, which is deterministic, is the order `classic` produces anyway,
and cannot drift from the files on disk. The player roster is built by
scanning `players/` at boot and cached in memory. This is the same
reasoning `go-port-plan.md` §5.4 gives for rebuilding `plr_index` wholesale
on every write — an index that disagrees with the files is a class of bug
that then simply does not arise — taken one step further to not having the
index at all.

**One writer per file.** Every file in this tree is written by exactly one
subsystem, and always in full, never appended to. Writes are
write-to-temp-then-`rename`, so a file is either the old version or the new
one and never a half-written mixture. This is what `objsave.c`'s
append-and-hope and `mail.c`'s free-block-list-in-a-file cost us: both can
be left inconsistent by a crash at the wrong moment, and neither can be
repaired by anything but a program that understands the format.

**Filenames are conveniences.** `30-midgaard.yaml` is named for humans;
the loader reads `zone.vnum` from inside it. A mismatch between the two is
a lint warning, not an error, and `dlctl world fmt` renames the file to
match. Nothing resolves a record by filename.

---

## 4. The zone file

This is the core of the format and everything else is smaller. Here is
room 3001, mobile 3059, object 3040 and shop 3000 from the stock world,
as this format expresses them — real records, not invented ones.

```yaml
# data/world/30-midgaard.yaml
schema: dl/world@1

zone:
  vnum: 30
  name: Northern Midgaard Main City
  range: [3000, 3099]
  lifespan: 15          # minutes between reset attempts
  reset: always         # never | empty | always

rooms:
  - vnum: 3001
    name: The Temple Of Midgaard
    sector: inside
    flags: [no_mob, indoors, peaceful, no_magic]
    desc: |2
         You are in the southern end of the temple hall in the Temple of Midgaard.
      The temple has been constructed from giant marble blocks, eternal in
      appearance, and most of the walls are covered by ancient wall paintings
      picturing Gods, Giants and peasants.
         Large steps lead down through the grand temple gate, descending the huge
      mound upon which the temple is built and ends on the temple square below.
      To the west, you see the Reading Room.  The donation room is in a small
      alcove to your east.
    exits:
      north:
        to: 3054
        desc: |
          At the northern end of the temple hall is a statue and a huge altar.
      east:
        to: 3063
        desc: |
          In the east, you see a small alcove with a rickety wooden sign which
          reads "Midgaard Donation Room."
      south:
        to: 3005
        desc: |
          You look down the huge stone steps at the temple square below.
      west:
        to: 3000
        desc: |
          You see the Reading Room.
      down:
        to: 3005
        desc: |
          You see the temple square.

mobiles:
  - vnum: 3059
    keywords: [keeper, peacekeeper]
    short: the Peacekeeper
    long: |
      A Peacekeeper is standing here, ready to jump in at the first sign of trouble.
    desc: |
      He looks very strong and wise.  Looks like he doesn't answer to ANYONE.
    act: [spec, aware, stay_zone, memory, helper]
    affected: [detect_invis]
    alignment: 1000
    level: 17
    thac0: 3
    ac: 0
    hp: 5d6+225
    damage: 3d3+13
    gold: 2500
    exp: 30000
    position: standing
    default_position: standing
    sex: male
    abilities:
      bare_hand_attack: 10

objects:
  - vnum: 3040
    keywords: [plate, breast]
    short: a breast plate
    long: A breast plate is lying on the ground.
    type: armor
    wear: [take, body]
    armor:
      ac_apply: 7
    weight: 100
    cost: 18000
    rent: 150

shops:
  - vnum: 3000
    keeper: 3000
    rooms: [3033]
    sells: [3050, 3051, 3052, 3053, 3054]
    buys:
      - type: scroll
      - type: wand
      - type: staff
      - type: potion
    markup: 1.15          # multiplier on the price the shop sells at
    markdown: 0.15        # multiplier on the price the shop buys at
    hours: [[0, 28]]
    flags: [uses_bank]
    refuses: [evil]       # polarity is the C's: who the keeper will *not* serve
    messages:
      no_such_item_keeper: "%s Sorry, I haven't got exactly that item."
      no_such_item_player: "%s You don't seem to have that."
      do_not_buy: "%s I don't buy such items."
      keeper_broke: "%s That is too expensive for me!"
      player_broke: "%s You can't afford it!"
      buy: "%s That'll be %d coins, please."
      sell: "%s You'll get %d coins for it!"

resets: ...   # §4.4
```

Points worth drawing out.

### 4.1 Symbolic names everywhere a number was a code

`cdeh` becomes `[no_mob, indoors, peaceful, no_magic]`. Sector `0` becomes
`inside`. Item type `9` becomes `armor`. Wear bitfield `9` becomes
`[take, body]`. Position `8` becomes `standing`. Wear position `16` in a
reset becomes `wield`.

This is the single largest readability win in the format and it is not free
of risk, so:

- **The names are new tables, not the ones already there.**
  `internal/game/bitnames.go` holds `constants.c`'s tables — `affect_bits[]`,
  `extra_bits[]`, `apply_types[]` — and `SprintBit` prints them. Those are
  *display* strings and cannot be reused here: they contain spaces and
  hyphens (`DET-ALIGN`, `LIQ CONTAINER`), they are positional rather than
  named, `sprintbit` renders an empty field as `NOBITS `, and they are
  player-visible, so changing one to suit a file format would change what
  `identify` prints. The format needs a parallel table of identifiers
  (`detect_align`, `liq_container`) with a test asserting the two tables
  describe the same set of bits, so that a bit added to one and not the other
  fails rather than silently going unnameable.

  `bitnames.go` is still the useful starting point, because it is the record
  of *which bits exist in this tree* — including the local ones past where
  stock CircleMUD stops (`HOLY-SHIELD`, `SNEAK`, `HIDE`, `SILENCE`, `CHARM`,
  and the `UNUSED` slot its comment notes is not unused here). The tables
  still missing entirely — room flags, sector types, mob act flags,
  positions, shop flags, container flags — are real work and a prerequisite,
  not a detail. They are the same tables the OLC layer will need anyway.

- **An unknown name is an error, not a shrug.** `classic` silently ignores
  characters that are neither letters nor digits in a bitfield, because the
  C did and the real world files rely on it. `native` does not inherit that:
  a name it does not recognise fails the load with a file and line number.
  Forgiveness was a property of a format nobody could validate; this one can
  be validated.

- **Unnamed bits survive anyway.** A bit with no name in the table — and
  there are some, since CircleMUD's tables have gaps and Disgracelands added
  flags of its own — round-trips through an explicit escape hatch rather
  than being dropped:

  ```yaml
  flags: [no_mob, indoors]
  flags_raw: 0x40000        # bits with no symbolic name, preserved verbatim
  ```

  `dlctl world lint` reports every use of `flags_raw` as something to name.
  Losing a bit silently is how a room quietly stops being a death trap.

- **The 32-bit letter-encoding ceiling goes away.** `Flags.ExceedsCRange()`
  exists because `asciiflag_conv()` computes `1 << (26 + (c - 'A'))` into an
  `int`, so bit 31 is the sign bit and everything above it is undefined
  behaviour in the C server. `native` has no such limit: flags are a list of
  names and there are as many names as we care to define. Converting a world
  that uses bit 32 or above *back* to `classic` cannot work, and
  `dlctl world export --format=classic` refuses rather than truncating.

### 4.2 Exits are keyed by direction

`D0`–`D5` becomes `north`, `east`, `south`, `west`, `up`, `down`. A missing
key means no exit; there is no way to write an exit in direction 7 and have
it silently become north, which is what `classic` does today with an
unchecked `int` → array index (see `game.DirectionFromInt`, which exists to
refuse exactly that).

Doors keep the file's meaning rather than the runtime's:

```yaml
south:
  to: 3005
  door: pickproof        # absent = no door; regular | pickproof
  key: 3086
  keywords: [gate, door]
```

`door` is the raw 0/1/2 the file always held, named. The *runtime* state —
open, closed, locked — is not a property of the definition and is not
written here; it comes from `D` resets, which §4.4 makes explicit. This
matters because `game.ExitDef` currently carries both `DoorFlag` and
`State` and the distinction between them is easy to lose.

### 4.3 Object values are typed

`values: [7, 0, 0, 0]` on an armor means "AC apply 7". On a weapon it means
"3d5, damage type slash". On a drink container it means "capacity, current,
liquid, poisoned". Four integers whose meaning depends on another field is
the format's worst idea, and the one most worth fixing:

```yaml
type: weapon
weapon: { dice: 3d5, damage_type: slash }

type: container
container:
  capacity: 100
  closeable: true
  closed: true
  key: 3086

type: drink_container
drink: { capacity: 50, current: 50, liquid: water, poisoned: false }

type: light
light: { hours: 24 }        # -1 for eternal

type: wand                  # and staff
charges: { spell: fireball, level: 12, max: 3, remaining: 3 }
```

**Lossiness is the risk and it is handled by refusing to guess.** Real
world data has junk in value slots the type does not use, and a typed form
would silently discard it. So the rule is: the converter emits the typed
form **only when every slot the type does not use is zero**. Otherwise it
emits the raw form:

```yaml
type: other
values: [0, 3, 0, 17]       # always accepted, always preserved exactly
```

and `dlctl world lint` reports it, so a human decides whether it was junk
or whether it meant something. Both forms load; the typed form is canonical
where it is available.

### 4.4 Resets become structural

This is the biggest departure from `classic` and the one that most needs to
be justified as *equivalent* rather than merely nicer.

The stock encoding is a flat list of single-letter opcodes with an `if_flag`
that means "only run if the previous command succeeded", plus an implicit
"the last mobile loaded" that `G` and `E` refer to. So this:

```
M 0 3000 1 3033         Wizard Shop Keeper
G 1 3050 500                    Scroll Of Identify
G 1 3051 500                    Yellow Potion
M 0 3001 1 3009         Baker
G 1 3009 100                    Waybread
E 1 3022 100 16                 Long Sword
```

becomes this:

```yaml
resets:
  - mob: 3000
    room: 3033
    max: 1
    then:
      - give: 3050
        max: 500
      - give: 3051
        max: 500

  - mob: 3001
    room: 3009
    max: 1
    then:
      - give: 3009
        max: 100
      - equip: 3022
        slot: wield
        max: 100
```

`then` is a **chain, not a set**: each entry runs only if the one before it
succeeded, and a failure abandons the rest of the chain. That is precisely
what `if_flag: 1` means in the flat form, so the mapping is a bijection —
a run of consecutive `if_flag: 1` commands nests under the last `if_flag: 0`
command before them, and flattening reverses it exactly. `dlctl` converts in
both directions and the parity harness proves the round trip on the real
world.

What this buys, beyond legibility: "the last mobile loaded" stops being
implicit in file order, so a `give` that has drifted away from its `mob`
becomes a *structural* impossibility rather than a silent misbehaviour, and
`dlctl world lint` can say "this `equip` has no mobile" instead of the game
quietly equipping the wrong creature.

The full opcode set:

| Flat | Native | Meaning |
|---|---|---|
| `M` | `mob: <vnum>, room: <vnum>, max: N` | Load a mobile into a room |
| `O` | `object: <vnum>, room: <vnum>, max: N` | Load an object into a room |
| `G` | `give: <vnum>, max: N` | Into the last mobile's inventory |
| `E` | `equip: <vnum>, slot: <name>, max: N` | Onto the last mobile |
| `P` | `put: <vnum>, into: <vnum>, max: N` | Into the last object loaded |
| `D` | `door: <room>, dir: <name>, state: open\|closed\|locked` | Set door state |
| `R` | `remove: <vnum>, room: <vnum>` | Remove an object from a room |

`max` is CircleMUD's "maximum number of these that may exist in the world",
which is worth naming because a great many builders have read it as
"how many to load".

**Disabled commands.** `renum_zone_table()` rewrites an opcode to `*` when
its vnums do not resolve, destroying both the opcode and some of the
arguments. `native` has no equivalent because it does not need one: an
unresolvable vnum is a lint error at write time and a load error at read
time, reported with a line number rather than silently neutered. The parity
dump keeps emitting the `classic` behaviour for as long as `classic` is the
oracle.

### 4.5 Two shop fields kept awkward on purpose

Two shop details are deliberately *not* prettified. `refuses` keeps the C's
polarity — `with_who` is a bitvector of who the keeper will **not** serve,
and `TRADE_NOEVIL` is bit 1 — because inverting a negative bitvector into a
positive one is exactly the kind of transformation that gets a sign wrong in
one direction only, and the bug surfaces as a shop that serves the wrong
people. `markup`/`markdown` are the C's `profit_buy`/`profit_sell`, renamed
because those two names have never once told anybody which is which.

### 4.6 Text fidelity is the sharp edge

The `desc:` in the example above is written `|2`, not `|`, and that is not
a typo. CircleMUD indents the first line of each paragraph in a room
description by three spaces, so the block's first line is more indented than
the rest. YAML infers a block scalar's indentation from its first non-empty
line, which would take those three spaces as structure and then fail on the
following lines for being under-indented. The explicit indicator pins the
content indentation instead, and the three spaces survive as text.

This is §2.4's risk in concrete form, and it is worth seeing once in this
document because it is the class of bug that will otherwise be found by a
player noticing that every room description looks subtly wrong. The writer's
obligations that follow from it are in §10.3.

### 4.7 Enhanced mobiles

The `E` mob format's trailing `Key: value` block is a small closed set —
across the whole stock world it is `BareHandAttack`, `Str`, `StrAdd`, `Int`,
`Wis`, `Dex`, `Con` and `Cha` — so it becomes a typed mapping rather than a
list of raw pairs:

```yaml
abilities:
  bare_hand_attack: 10
  str: 18
  str_add: 100
```

The simple/enhanced distinction disappears: a mobile either has an
`abilities` block or it does not, and the writer emits `S` or `E` format
accordingly when exporting back to `classic`. An unrecognised key is a load
error, for the reason in §4.1 — the set is closed, and a typo in it
currently does nothing at all.

---

## 5. Colour

Colour needs its own section because it is the one piece of this format
that is *not* a re-encoding of something that already exists. There is no
colour convention in the data today to be faithful to, the Go server has no
colour support to preserve, and the obvious approach — put the escape codes
in the file, like the C server does — is not available. So this is a design
decision rather than a translation, and it is load-bearing for both
round-tripping and the web client.

### 5.1 What the C server actually does

Established by reading all three reference trees rather than assumed:

- **Colour is compiled in, not stored.** `screen.h` defines `KRED` as the
  literal `"\x1B[31m"` and `CCRED(ch, lvl)` as "emit that if this player's
  colour level is at least `lvl`". Every colour in the C server comes from a
  macro at a call site.
- **There is no in-band colour parser anywhere.** No `proc_color`, no
  `parse_color`, no `&`- or `@`-code translation, in `moderncserver`,
  `CircleMUD3-src` or `WipeMud-src`. Many CircleMUD derivatives added one;
  this lineage did not.
- **The level is two bits, not a boolean.** `PRF_COLOR_1` and `PRF_COLOR_2`
  combine into `C_OFF`/`C_SPR`/`C_NRM`/`C_CMP`, and each call site declares
  the level at which its colour appears. `internal/game/playerflags.go`
  already documents this correctly and does nothing else with it.
- **The one concession to colour in data is the pager.** `modify.c`'s
  `next_page()` walks a string treating everything between `\x1B` and the
  next `m` as zero-width, so pagination does not break on embedded escapes.

That last point is the whole of the C server's data-borne colour model:
**raw ESC bytes may sit in the data, nothing translates them, and the pager
knows to skip them when counting.** Whatever a builder typed goes to the
socket.

Two facts bound what we have to support. The stock world contains **zero**
ESC bytes — verified across `data/`. And the Go tree renders no colour at
all: it has the preference flags and nothing behind them. The one escape
sequence it emits is `doClearScreen`'s home-and-clear
(`internal/session/informational.go`), which is sent raw and is not colour.
So there is no
installed base on either side to stay compatible with, and the archive is
the only unknown — if the real Disgracelands data carries colour, it
carries it as raw ESC, because its server could not have understood
anything else.

### 5.2 Raw escapes cannot go in the file, and that settles the design

YAML 1.2's printable character set excludes the C0 controls other than tab,
newline and carriage return. **ESC (0x1B) cannot appear literally anywhere
in a YAML stream** — not in a block scalar, not in a plain scalar, nowhere.
Verified rather than inferred: a block scalar containing a raw ESC is a
reader error, and an emitter handed one falls back to double-quoted style
and writes `\e`.

The consequence is decisive. A single colour code anywhere in a room
description would collapse that description from a block scalar into one
escaped single-line string — which is precisely the failure mode §2.2 chose
YAML to avoid. One coloured room would look like this:

```yaml
desc: "\e[31mYou are in the southern end of the temple hall.\e[0m\nThe temple has been\nconstructed from giant marble blocks.\n"
```

So colour markup **must** be symbolic. This is not a preference between
approaches; storing raw escapes and keeping the format readable are
mutually exclusive.

### 5.3 The markup

`&` followed by one character, with the code letters taken from `screen.h`
so the mapping to and from the C server is mechanical:

| Code | Meaning | ANSI |
|---|---|---|
| `&n` | normal / reset | `\x1B[0m` |
| `&r` `&g` `&y` `&b` `&m` `&c` `&w` | red, green, yellow, blue, magenta, cyan, white | `\x1B[3Xm` |
| `&R` `&G` `&Y` `&B` `&M` `&C` `&W` | the bright variants | `\x1B[1;3Xm` |
| `&&` | a literal `&` | — |
| `&{…}` | raw SGR passthrough — see §5.5 | `\x1B[…m` |

**Why `&`:** because the data already uses this shape of markup and `&` is
the one common sigil not yet taken. `act()` uses `$`-codes (`$n`, `$M`,
`$e`) with `$$` as the literal escape and a `SYSERR` log for an unknown
code — 1,480 of them in the stock data. Shop and damage messages use
`printf`'s `%s`/`%d` — 418 more. `@` and `^` appear in prose. So `&` follows
an established local precedent rather than inventing one, and `&&`-as-escape
and unknown-code-is-an-error both mirror what `act()` already does.

There are **13 literal ampersands** in the stock data: the "Hide & Tooth"
shop that runs through zone 65, a credits line, and one social. Conversion
escapes them to `&&`. That social is worth naming as the round-trip test
case, because it is the most hostile string in the corpus and it exists
already:

```
$n swears: #@*"*&^$$%@*&!!!!!!
```

It contains `&`, `$$`, `%`, `@`, `^`, `#` and `"`. Anything that survives it
intact will survive the rest of the corpus.

**Colour is allowed in prose fields only.** Not in keywords: a player types
keywords, and a colour code inside one silently breaks object and mobile
matching in a way that is very hard to diagnose. Validation rejects it
rather than trusting builders to know.

**One sharp edge, and it fails silently.** `&` is YAML's anchor indicator,
so a *plain* scalar that begins with a colour code is misparsed rather than
rejected — a coloured room name is exactly this case, and it is a
completely reasonable thing for a builder to write:

```yaml
name: &rThe Temple Of Midgaard&n     # parses as 'Temple Of Midgaard&n'
```

`&rThe` is consumed as an anchor named `rThe`, the rest becomes the value,
and nothing anywhere reports a problem. The three mitigations are all
required, not alternatives: the writer **always quotes** a single-line
string that begins with `&`; the loader **rejects an anchor** anywhere in a
file, which §2.1 already excludes from the data model and which turns this
from silent corruption into an error with a line number; and
`dlctl world lint` flags a value whose first characters look like a colour
code that has gone missing. Block scalars are unaffected — a colour code
anywhere inside one is literal text, including at the very start — which is
another reason prose belongs in block scalars even when it happens to fit
on one line.

### 5.4 Rendering

Colour resolves at send time, per player, from the two preference bits:

- Level `C_OFF` — every code is **stripped**, leaving the text.
- Level `C_NRM` and above — codes render to ANSI.

The C's four levels distinguish *call sites*, not data — `CCRED(ch, C_SPR)`
means "this bit of engine chrome appears even at the sparse setting". Data
carries no such distinction, so all data-borne colour renders from `C_NRM`
up, which leaves `C_SPR` meaning what it means today: engine colour only.

Three rules follow, and the third is the one the Go tree currently has no
answer to:

- **Stripping is the default for everything that is not a live player
  socket** — logs, the parity dump, `dlctl` output, GMCP payloads, anything
  searched or diffed.
- **Codes are zero-width.** Anything that wraps, pads, columnises or
  paginates counts display width, not bytes. This is exactly what
  `next_page()` does by hand in the C, and the Go server has no wrapping
  layer yet — so it should be built knowing this from the start rather than
  retrofitted, which is the substance of "we don't do colour well so far".
- **Unbalanced colour is lintable.** A description that opens `&r` and never
  returns to `&n` bleeds into whatever is printed next. That is the single
  most common colour bug in MUD data, it is trivial to detect on a symbolic
  grammar, and `dlctl world lint` reports it.

### 5.5 Converting back to CircleMUD

The requirement is that `native` data can go back to something the C server
runs. Colour is the only part of this format where that is not simply a
matter of re-encoding, so:

**Export (`native` → `classic`) renders codes to raw ANSI.** `&r` becomes
`\x1B[31m` in the written file. This is correct CircleMUD: the C server
passes ESC through untouched and its pager already skips it. Exporting `&r`
*literally* would show players the two characters `&r`, so rendering is
mandatory rather than an option — and it is the reason `screen.h`'s exact
byte sequences are the table above rather than some tidier ANSI of our own.

**Import (`classic` → `native`) recognises and demotes.** ESC sequences
matching `screen.h`'s table become `&` codes; a literal `&` in the source
becomes `&&`.

**Anything unrecognised survives via `&{…}`.** If the archive turns out to
contain 256-colour or truecolour sequences — `\x1B[38;5;208m` — there is no
named code for them, so they import as `&{38;5;208}` and export back to the
identical bytes. This is what makes import lossless in all cases rather
than only the easy one, and it means the format never has to refuse a file
because someone was ambitious with colour in 2004.

**The honest caveat: the round trip normalises.** `\x1B[1;31m` and
`\x1B[31;1m` are the same colour and both import as `&R`; re-exporting emits
`screen.h`'s spelling. So `classic → native → classic` preserves *meaning*
exactly and *bytes* only where the original already used `screen.h`'s
spelling. Under §10.4's posture this is a warning rather than a refusal —
it is a change of representation, not a loss of data — and `dlctl` reports
how many sequences it normalised so the number can be eyeballed against
expectation.

### 5.6 What this buys beyond the file being readable

The symbolic form is not merely a workaround for §5.2. It is the
representation the rest of the plan wants anyway:

- **The web client.** `go-port-plan.md`'s decision record calls a self-hosted
  web front end "wanted, not merely kept open". A browser cannot use ANSI. A
  renderer that turns `&r` into a `<span class="c-red">` is a lookup table;
  one that turns `\x1B[1;31m` into the same thing is an ANSI state machine
  that has to be right about every sequence a builder ever typed.
- **GMCP and MXP** want structured text for the same reason.
- **Everything that is not a terminal** — search, the parity dump, logs,
  `dlctl` — strips colour by matching a two-character grammar rather than by
  parsing escape sequences.

The general principle is the one this format applies everywhere else:
**store the intent, render the encoding at the edge.** Raw ANSI in the data
is a rendering decision frozen into storage in 1993, and it has exactly the
same shape as the fixed-width password field and the letter-encoded
bitflags — a property of the output device of the day, written into the
file where the meaning should have been.

## 6. Game configuration

`docs/configuration.md` reserves a slot for a config file "between
environment and defaults" and says it will arrive with the values that
justify it. These are those values: the tuning compiled into
`reference/moderncserver/src/config.c`.

```yaml
# data/config/game.yaml
schema: dl/config@1

play:
  pk_allowed: false
  pt_allowed: false
  level_can_shout: 1
  holler_move_cost: 20
  tunnel_size: 2
  max_exp_gain: 100000
  max_exp_loss: 500000
  track_through_doors: true

corpses:
  npc_decay_minutes: 5
  pc_decay_minutes: 10

idle:
  void_after: 8            # ticks before an idle player goes to the void
  rent_after: 48
  max_level: god           # immortals above this are never idled out

rent:
  free: true
  min_cost: 100
  max_objects_saved: 30
  crash_file_timeout_days: 10
  rent_file_timeout_days: 30

rooms:
  mortal_start: 3001
  immortal_start: 1204
  frozen_start: 1202
  donation: [3063]
  death_traps_are_dumps: false

saving:
  autosave: true
  autosave_minutes: 5

login:
  max_bad_passwords: 3
  siteok_everyone: true

wizlist:
  min_level: god
  autowiz: false
```

The line between this file and the flags in `docs/configuration.md` is
worth stating because it will otherwise be relitigated every time something
new is added:

**`data/config/game.yaml` holds rules. Flags and environment hold
deployment.** Whether player-killing is allowed is a property of *this
game*, travels with the world, belongs in the backup, and should be
reviewable in a pull request. Which port to listen on, where the TLS
certificate is and what the log level is are properties of *this
deployment*, differ between the laptop and the container, and must not live
in a directory that gets copied between them. `--mortal-start-room` would
be a mistake; `--listen-telnets` in a data file would be a worse one.

The C's `OK`/`NOPERSON`/`NOEFFECT` strings and the login menu are text, not
configuration, and go to `data/text/` under §7.

---

## 7. Prose stays prose

Help entries, the MOTD, the greeting screen, credits, news, policies and
the handbook are text written by humans for humans, and wrapping them in a
data format makes them harder to read, harder to grep and harder to diff
while gaining nothing. They stay as UTF-8 `.txt` files.

Only help needs structure, because a help entry genuinely has fields —
what keywords reach it, and what level may read it. That structure goes in
an index, not in the text:

```yaml
# data/text/help/help.yaml
schema: dl/help@1

entries:
  - keywords: [ac, "armor class"]
    file: ac.txt
  - keywords: [advance]
    file: advance.txt
    min_level: god
  - keywords: [alias, aliases]
    file: alias.txt
```

This replaces `text/help/index`, `help.hlp`, `wizhelp.hlp` and the rest.
The old `.hlp` files are concatenations of entries separated by `#`, which
means a one-word fix to one entry is a diff in a 200KB file and two people
cannot edit two entries without conflicting. One entry, one file fixes
both, and `min_level` replaces the convention that wizard help lives in a
separately-named file that the loader treats differently.

Socials and damage messages *are* structured, and become
`data/config/socials.yaml` and `data/config/messages.yaml`:

```yaml
# socials.yaml
- command: accuse
  hide: false
  min_victim_position: standing
  no_arg:
    char: Accuse who??
  found:
    char: You look accusingly at $M.
    others: $n looks accusingly at $N.
    victim: $n looks accusingly at you.
  not_found:
    char: Accuse somebody who's not even there??
  self:
    char: You accuse yourself.
    others: $n seems to have a bad conscience.
```

The stock format's `#` placeholder for "no message here" becomes an absent
key, and the twelve-lines-always-present rule of `messages` becomes
"whichever of `death`, `miss`, `hit`, `god` you supply".

---

## 8. Players: one player, one file

Everything about a player goes in that player's file. Today a single
character is spread across `pfiles/<letter>/<name>`, an entry in
`etc/plr_index`, `plralias/<range>/<name>.alias`, and up to two files in
`plrobjs/` — and losing one of them silently loses part of the character.

```yaml
# data/players/a/aragorn.yaml
schema: dl/player@1

id: 42
name: Aragorn
credential: "argon2id$v=19$m=65536,t=3,p=4$..."

identity:
  title: the Ranger
  sex: male
  class: warrior
  level: 34
  remort: [thief]         # was Rmrt, a bitmask
  home: 3001
  description: |
    A grizzled old warrior stands here, sword in hand.

times:
  created: 2001-11-03T21:14:07Z
  last_login: 2008-06-19T02:31:55Z
  played: 1847293s
  last_host: "136.206.1.2:4000"

body:
  height: 183
  weight: 187
  str: 18
  str_add: 100            # the "18/00" exceptional-strength percentile
  int: 13
  wis: 11
  dex: 16
  con: 17
  cha: 12

pools:
  hit:  { current: 412, max: 412 }
  mana: { current: 100, max: 100 }
  move: { current: 82,  max: 96 }

combat:
  ac: -47
  hitroll: 9
  damroll: 11
  alignment: 350
  saves:
    paralyse: -12
    rod: -10
    petrify: -11
    breath: -13
    spell: -14
  wimpy: 40

wealth:
  gold: 1207
  bank: 84000
  exp: 4102993

conditions:
  hunger: 21
  thirst: 20
  drunk: 0

flags:
  act: [siteok]
  affected: [sanctuary, detect_invis]
  prefs: [autoexit, color_complete, compact]

practice_sessions: 3
invisibility_level: 0
frozen_by_level: 0
bad_password_attempts: 0

skills:
  bash: 85
  kick: 100
  "magic missile": 95

affects:
  - spell: sanctuary
    duration: 12
    modifier: 0
    location: none
    sets: [sanctuary]

aliases:
  gbb: get bread bag
  gac: get all corpse

equipment:
  wield:  { vnum: 3022 }
  body:   { vnum: 3040, condition: { weight: 98 } }

inventory:
  - vnum: 3032           # a bag
    contains:
      - vnum: 3009
      - vnum: 3010
```

Notes on the parts that are not just a rename:

**Skills and affects are keyed by name.** `Skil: 1 100` means "skill number
1, at 100%", and skill number 1 is whatever `spells.h` said it was on the
day the file was written. Renumbering the spell table today silently
rewrites every character's abilities. Names do not have that failure mode.

**Object instances nest.** `objsave.c` writes a flat list and encodes
container depth in a `location` field with negative numbers, which is
ingenious and completely opaque. Nesting says the same thing and cannot
express an impossible tree.

**Object instances are deltas.** An entry is `{vnum: 3040}` plus only what
differs from the prototype — a `condition` block for values, weight, timer,
extra flags and affects that have changed. An unmodified object is four
words. This is also the shared object-instance schema used by corpses,
house crash files and anything else that has to persist an object, which is
one schema where `lib/` has three.

**Rent and crash saves fold in.** There is no separate `plrobjs/`; the
inventory *is* the rent file. A crash save rewrites the player file. At a
few kilobytes per player and a few hundred players this is not a
performance question, and it removes the possibility of a character whose
pfile and rent file disagree — which is how items get duplicated.

**No `plr_index`.** The roster is built by scanning `players/` at boot and
held in memory; §3 covers why.

**Credentials are scheme-prefixed**, as `go-port-plan.md` §5.4 already
decided for `ascii`. A legacy DES hash converts to
`des-crypt10$<10 chars>` — explicitly named as the truncated ten-character
form documented in §5.3.1 of that plan, so nothing has to infer it from the
absence of a prefix.

---

## 9. The rest of the state

Small formats, listed for completeness because "all of it has to move
together" is the entire point.

All three of the struct-dump ones are now ported and working —
`internal/persist/boards`, `internal/persist/mail`, `internal/persist/houses`
— so none of this is urgent, and that is the right order: the C formats had
to be readable before there was anything to convert *from*. What those ports
buy this proposal is a precise specification of each format and a test
oracle for it, which is most of the work of writing the converter. What they
do not change is the case for replacing them, which is §1's: every one is a
memory layout, and `layout_test.go` exists in each package because the only
way to know where a field sits is to ask a C compiler.

**`state/boards.yaml`** — replaces the `board.*` struct dumps. A board is a
vnum, its read/write/remove levels, and a list of messages, each with a
poster, a timestamp, a heading and a body as a block scalar. The stock
`MAX_BOARD_MESSAGES` of 60 and `MAX_MESSAGE_LENGTH` of 4096 are properties
of a fixed-size array, not of the game, and do not come along.

**`state/mail.yaml`** — replaces the 100-byte-block linked list in
`mail.c`. A list of `{to, from, sent, body}`. The free-block list, the
block chaining and the `BLOCK_SIZE` arithmetic all vanish; they exist only
because the file was a hand-rolled allocator.

**`state/houses.yaml`** — replaces `etc/hcontrol`, an array of
`struct house_control_rec` complete with eight `spare` longs. Becomes a
list of `{room, atrium, exit, owner, guests, built, last_payment, mode}`.
The `MAX_HOUSES` of 100 and `MAX_GUESTS` of 10 go the same way as the board
limits.

**`state/reports.yaml`** — the `bugs`, `ideas` and `typos` files, which are
currently three append-only text logs with a timestamp convention. One list
with a `kind`, a reporter, a room and a body, which makes them something a
tool can triage instead of something someone greps.

**`config/names.yaml`** — `misc/xnames`, a list of disallowed name
substrings. Unchanged in substance; it is a list of strings either way.

---

## 10. Versioning, validation and the canonical writer

### 10.1 Schema versioning

Every file carries `schema: dl/<kind>@<major>`. The major version changes
only for a breaking change, which means one that an older loader would
misread rather than merely fail to understand. Additive changes — a new
optional field, a new flag name, a new object type block — do not bump it.

The loader refuses a major version it does not know, by name and with the
file path, rather than parsing what it can and producing a subtly wrong
world.

### 10.2 Unknown fields are an error

Strict decoding, on by default. A typo'd key is the single most likely
authoring mistake in a format like this, and the cost of silently ignoring
one is a builder wondering for a week why their flag did nothing.
`--lax` exists for loading data written by a *newer* server, and says
loudly what it ignored.

This is the opposite of `classic`'s deliberate forgiveness, and the reason
it can be: `classic` had to be forgiving because the real data exploited
undocumented leniency in the C parser and there was no validator. `native`
has a validator, so it can afford to be strict.

### 10.3 The canonical writer

Because the server writes these files, the writer needs properties the
reader does not care about:

- **Deterministic.** Fixed key order (schema order, not alphabetical — a
  room's `vnum` and `name` first, its prose in the middle, its exits last).
  Records in vnum order. Two-space indentation. No maps ranged over. Running
  `dlctl world fmt` twice produces the same bytes, and a zone the server
  rewrote without changing produces an empty diff.
- **Block scalars for anything multi-line**, and quoted style *only* where a
  block scalar cannot represent the string exactly. Three choices have to be
  made per string and all three are fidelity-critical: an **indentation
  indicator** (`|2`) whenever the first line is more indented than the rest,
  per §4.6; a **chomping indicator** — `|` for exactly one trailing newline,
  `|-` for none, `|+` to keep several — because CircleMUD strings differ in
  all three ways and a description that gains or loses a blank line is a
  visible change; and the fallback to double-quoted style when any line has
  trailing whitespace, which a block scalar cannot express at all.
- **Quoted style for any single-line string beginning with an indicator
  character** — `&`, `*`, `!`, `%`, `@`, `` ` ``, `#`, `-`, `?`, `:`, `[`,
  `{`. The `&` case is the one that will actually happen, it is a coloured
  room name, and §5.3 explains why it is the dangerous one: it does not
  fail, it silently returns the wrong string.
- **Byte-preserving on round trip.** Every string that goes in comes back
  out identical, including trailing whitespace, tabs, leading indentation and
  colour codes. This is the property §2.4 flags as the real risk, and it is
  tested by fuzzing the writer over the entire real corpus, not by
  inspection.
- **Atomic.** Temp file, `fsync`, `rename`.

### 10.4 Tooling

```
dlctl world import --from=data-old --format=classic --to=data
dlctl world export --to=data-classic --format=classic   # refuses on data loss
dlctl world lint                                        # replaces scheck
dlctl world fmt                                         # canonicalise in place
dlctl data verify                                       # every file, every schema
```

`export` is not a nicety. It is what makes this format safe to adopt: as
long as the C server is the parity oracle, being able to go back is how a
disagreement gets diagnosed, and it is how a premature migration gets
undone. It refuses rather than truncates — a world using flag bits above 31,
or an object with a typed value block that `classic` cannot represent, is
reported and not written, matching the posture `go-port-plan.md` §5.5 takes
for player conversion.

A JSON Schema is generated from the Go types for each `dl/<kind>` and
shipped in the repo, so editors give builders completion and inline errors.
This is a real part of the value: the format is only as good as the
experience of writing it by hand, and a schema-aware editor turns "what
flags exist?" from a documentation lookup into an autocomplete.

---

## 11. Getting there

The existing registries mean this lands as a driver, not a rewrite.
`internal/persist/world/native/` implements `world.Source` and `world.Sink`;
`internal/persist/player/native/` implements `player.Store`. Nothing in
`internal/game` changes, because the canonical model is already
format-neutral and that is the whole reason it exists.

| Step | What lands | Done when |
|---|---|---|
| **1. Vocabularies** | Name tables for every flag set, sector, position, item type, apply location and wear slot in `internal/game`, with round-trip tests. | Every bit in the stock world has a name or is reported. |
| **1b. Colour** | The `&`-code parser, the ANSI renderer keyed off `PrefColour1`/`PrefColour2`, the stripper, and a display-width function that the wrapping layer is built on rather than retrofitted to. | The swearing social in §5.3 survives parse → render → strip, and width is counted correctly across every code including `&{…}`. |
| **2. World read** | `native` as a read-only `world.Source`, plus `dlctl world import` from `classic`, including ESC → `&` demotion. | `import` then load produces a parity dump byte-identical to loading `classic` directly. |
| **3. World write** | `world.Sink`, the canonical writer, `dlctl world fmt`, `dlctl world export`. | `classic → native → classic` round-trips the whole world byte-for-byte — exactly, for the stock world, which has no colour in it; modulo the escape normalisation of §5.5 for anything that does — and `fmt` is idempotent. |
| **4. Flip the default** | `--world-format=native`, `data/` converted in the repo, `classic` demoted to import-only. | CI runs the parity harness against a converted `data/`. |
| **5. Players** | `native` player store, `dlctl pfile convert --to=native`, aliases and rent folded in. | An archived roster survives `binary → ascii → native` with every field verified. |
| **6. The rest** | Boards, mail, houses, reports, socials, messages, help, game config. | `dlctl convert` reports zero `unsupported` files on a real `lib/`. |
| **7. Retire** | `ascii` and `binary` become `dlctl`-only; `classic` becomes import-only. | The server has one format for everything. |

Steps 1–4 are worth doing before **Phase 6** of `go-port-plan.md`, which is
where OasisOLC and `Sink` writeback land: OLC writes zone files back, and it
would be perverse to implement a writer for `classic` and then a second one
for `native`. That makes Phase 5 — currently in progress — the natural window.
Steps 5–6 can follow whenever.

`classic` is never deleted. It is the parity oracle for as long as the C
server is authoritative, and it is how the 1,184 dated nightly world
backups in the archive get read.

---

## 12. Open questions

**Is one file per zone right when a zone is large?** Midgaard is 100 rooms
and the file will be perhaps 200KB. The archive's real zones may be bigger.
If a zone file gets unpleasant to work in, the fallback is a per-zone
*directory* with `zone.yaml`, `rooms.yaml`, `mobiles.yaml`, `objects.yaml`,
`shops.yaml` — which the loader can support with no format change at all,
since it is the same documents in more files. Worth building the loader so
that stays possible.

**Should the world be one document or many?** Loading 31 files and merging
is what is described. An alternative is a single `world.yaml`; it makes
whole-world operations trivial and everything else worse. Not proposed, but
noted, because the loader's shape decides whether it stays available.

**Does `state/` want to be a database?** Boards, mail and reports are
append-mostly and are the only things here with any write contention.
`go-port-plan.md` calls a database-backed world the speculative use case and
says it should not drive the design; the same applies. But if any part of
`data/` ends up in SQLite, it is this part, and the driver seam is where
that would happen.

**Timestamps.** RFC 3339 above (`2001-11-03T21:14:07Z`), which is readable
and unambiguous but is not what any existing format stores. The alternative
is Unix seconds, which round-trips the old files exactly and tells a human
nothing. RFC 3339 is proposed on the grounds that the conversion is
lossless in both directions anyway.

**Durations.** `played: 1847293s` above. A Go-style `513h8m13s` is more
readable and parses with `time.ParseDuration`, but is not a JSON number and
is not obviously better for a value nobody reads by eye. Low stakes, needs
picking once and applying everywhere.

**Colour in the archive.** §5 establishes what the *server* does — nothing
in-band, raw ESC or nothing — and that the stock world contains no escapes
at all. What the private archive's own world and help text contain has not
been surveyed, and it is the one input that could still move the design.
Two outcomes are cheap and one is not: no colour at all, or plain
`screen.h` colour, both convert straight to `&` codes. If it turns out
builders were embedding raw escapes heavily *and* inconsistently — half-open
sequences, cursor movement, anything that is not SGR — then import needs a
policy for escapes that are not colour, which `&{…}` deliberately does not
cover. Worth an hour with `grep -c $'\x1b'` over the archive before step 2
of §11, because it is the cheapest possible way to find out.
