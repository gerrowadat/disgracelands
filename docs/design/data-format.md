# A yaml data format for `data/`

A single file format for everything the Go server reads and writes: the
world, the players, the player-adjacent state that a MUD accumulates
around them, and the game tuning that is currently compiled into
`config.c`. It replaces the eight-or-so unrelated on-disk formats a
CircleMUD `lib/` directory carries — some text, some struct dumps, none of
them documented in the same place — with one, and it is a superset of all
of them: everything they can express, it can express, and it can express
more.

This started as a proposal in `docs/proposals/`, moved here once building it
started proving the design out rather than testing it. The world half
(§11 steps 1–3), the players
half (§11 step 5) and seven of the "rest of the state" formats — bans,
boards, mail, houses (§11 step 6a), plus xnames, the clock and reports
(§11 step 6b) — are now built. Each is `internal/persist/<name>/yaml/`,
registered as `yaml` alongside `classic`/`ascii` in the same per-package
registries this document always said they would slot into. `help` is now
a real feature too (§11 step 6c-i), and on `yaml` as well as `classic`
(§11 step 6c-vi) — its own one-file-per-entry format (§7), the different
shape everything else here deferred it for. Real damage messages (§11
step 6c-ii) — `dam_message`'s compiled table plus `skill_message`'s full
`misc/messages` lookup, covering the ordinary weapon swing, kick, bash,
backstab and every offensive spell too (§11 step 6c-v) — are built, and
on `yaml` as well as `classic` (§11 step 6c-iii,
`config/messages.yaml`, `--messages-format`). Socials (§11 step 6c-iv)
are on `yaml` too, the same way (`config/socials.yaml`,
`--socials-format`) — the feature (`do_action`) was already real from
Phase 5c. Game config (§6) is the one piece left, and deliberately so —
making `config.c`'s tuning configurable at all is a reversal of the
"archive wins" fidelity principle (`docs/deviations.md`'s existing "rent
settings are constants, not options" entry), not a format pass, and
needs its own decision rather than being folded in here; see §11 for
what landed and what is still a plan, and why those are staged
separately rather than
bundled in.

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
| **Colour** | **Named `{{red}}` codes in the data, ANSI rendered at the socket.** Symbolic markup is forced rather than chosen: a raw ESC byte is not a legal character anywhere in a YAML stream. The *spelling* is free, because the original data contains no colour codes to stay compatible with — so it is chosen for readability and for occurring zero times in the existing corpus, rather than borrowing the `&r` convention and its escaping burden. Export to `classic` renders codes back to the raw escapes `screen.h` defines, which is what the C server expects. See §5. |

Naming: the format registers as **`yaml`**. `--world-format=yaml`,
`--player-format=yaml`.

---

## 1. Why replace `lib/` at all

`data/` today is CircleMUD 3.0 bpl20's `lib/`, unmodified. It works, the
`classic` loader reads it faithfully, and `scripts/world-parity.sh` proves
the Go and C loaders agree on all 3,202 records. So the case for replacing
it has to be made, not assumed.

**It is not one format, it is eight.** A `lib/` directory contains: the
`#vnum` / `~` / letter-bitflag world files; the per-directory `index` and
`index.mini` files that say which of them to load — not `zone.lst`, which
looks like an index and is actually a builders' prose table; the
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
    zones.yaml               Which zones load, and which exist but do not.
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

**The directory is the index — for everything the server owns.** There is no
`plr_index` and no index file in `help/`. The player roster is built by
scanning `players/` at boot and cached in memory. This is the same reasoning
`go-port-plan.md` §5.4 gives for rebuilding `plr_index` wholesale on every
write — an index that disagrees with the files is a class of bug that then
simply does not arise — taken one step further to not having the index at
all.

**The world is the exception, and it took looking at a real data directory
to see why.** An earlier draft of this section applied the same rule to
zones: load every file in `world/`, in vnum order, no index. That is wrong,
and it is wrong in the worst way — silently, and only on real data.

A CircleMUD world directory is not a clean set of records. It is a working
directory that people edited for years, and a real one contains: complete
zone file sets that are present on disk but deliberately absent from the
index, because the way you disable a zone is to unlist it rather than delete
it; editor and operator backups sitting beside the files they back up, under
suffixes chosen ad hoc; at least one file of the wrong type in the wrong
subdirectory; and the prose `zone.lst` mentioned above, which is
documentation. Directory-as-index would load every one of those, and its
first visible effect would be silently re-enabling content somebody turned
off — possibly years earlier, possibly because it was broken.

So `world/` keeps an explicit manifest of which zones load. It is not a
transcription of CircleMUD's `index` files, because the thing worth keeping
is not the list but the *decision* the list encodes: a zone that exists and
does not load is a deliberate state, and the format says so out loud rather
than by omission.

```yaml
# data/world/zones.yaml
zones:
  - 30
  - 31
  - vnum: 42
    enabled: false
    note: unlisted in the source index; not loaded since at least the archive snapshot
```

Import reads the *source index*, never the directory listing, and reports
every on-disk file the index does not mention rather than quietly adopting
or quietly discarding it. `dlctl lint --type=world` flags a zone file with no
manifest entry, so the two cannot drift apart in the other direction either.

**One writer per file.** Every file in this tree is written by exactly one
subsystem, and always in full, never appended to. Writes are
write-to-temp-then-`rename`, so a file is either the old version or the new
one and never a half-written mixture. This is what `objsave.c`'s
append-and-hope and `mail.c`'s free-block-list-in-a-file cost us: both can
be left inconsistent by a crash at the wrong moment, and neither can be
repaired by anything but a program that understands the format.

**Filenames are conveniences.** `30-midgaard.yaml` is named for humans;
the loader reads `zone.vnum` from inside it. A mismatch between the two is
a lint warning, not an error, and `dlctl fmt --type=world` renames the file to
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
  C did and the real world files rely on it. `yaml` does not inherit that:
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

  `dlctl lint --type=world` reports every use of `flags_raw` as something to name.
  Losing a bit silently is how a room quietly stops being a death trap.

- **The 32-bit letter-encoding ceiling goes away.** `Flags.ExceedsCRange()`
  exists because `asciiflag_conv()` computes `1 << (26 + (c - 'A'))` into an
  `int`, so bit 31 is the sign bit and everything above it is undefined
  behaviour in the C server. `yaml` has no such limit: flags are a list of
  names and there are as many names as we care to define. Converting a world
  that uses bit 32 or above *back* to `classic` cannot work, and
  `dlctl export --type=world --to-format=classic` refuses rather than
  truncating.

  Worth knowing before anyone spends effort on it: the real world does not
  go anywhere near this. Every room and mobile bitfield in a snapshot of the
  original data uses lowercase letters only — bit 18 at the highest, which
  is where this tree's local flags sit. The ceiling is a property of the
  encoding to be rid of, not a live problem to solve.

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

and `dlctl lint --type=world` reports it, so a human decides whether it was junk
or whether it meant something. Both forms load; the typed form is canonical
where it is available.

Sampling the original data suggests this fires on the order of one object in
a hundred, which is the useful range: frequent enough that dropping the
slots silently would lose something real, rare enough that the lint output
is a list somebody can actually read to the end.

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
`dlctl lint --type=world` can say "this `equip` has no mobile" instead of the game
quietly equipping the wrong creature.

The full opcode set:

| Flat | Yaml | Meaning |
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
arguments. `yaml` has no equivalent because it does not need one: an
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
in the stock world *and* in a snapshot of the original one, which between
them contain several hundred enhanced mobiles, it is `BareHandAttack`,
`Str`, `StrAdd`, `Int`,
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

**Named codes in doubled braces**, `{{red}}` … `{{/}}`:

| Code | Meaning | ANSI |
|---|---|---|
| `{{/}}` | reset | `\x1B[0m` |
| `{{red}}` `{{green}}` `{{yellow}}` `{{blue}}` `{{magenta}}` `{{cyan}}` `{{white}}` | the seven `screen.h` colours | `\x1B[3Xm` |
| `{{bright-red}}` … `{{bright-white}}` | the bright variants | `\x1B[1;3Xm` |
| `{{sgr:…}}` | raw SGR passthrough — see §5.5 | `\x1B[…m` |
| `{{lbrace}}` | a literal `{{` | — |

**Why not the `&r` convention every other MUD uses.** Because there is
nothing to be compatible with. §5.1 establishes that this lineage never had
in-band colour codes, and §12 that the original data contains none — so the
usual reason to adopt `&` (players and builders already have it in their
fingers, and the files are full of it) does not apply here. That leaves two
criteria, YAML-safety and readability, and a single-character sigil loses on
both.

On safety, the numbers come from the real corpus. Every plausible
single-character sigil already occurs in builder-authored prose and would
need escaping — `&` 18 times, `^` 117, `<` 257, `(` 413, `%` 722, `$` 1,513.
`$` and `%` are additionally spoken for, by `act()`'s `$n`/`$M` codes and by
`printf` in the shop and damage messages. Doubled `{{` occurs **zero** times
in the entire corpus, so no escaping is required of any existing text and
`{{lbrace}}` exists only for completeness.

On readability, `{{bright-cyan}}` needs no table and `&C` does. That matters
more than it sounds: the people editing this are builders, not the person
who wrote the parser, and a file that can be read without a decoder ring is
most of what this format is for.

**The one syntactic wrinkle fails loudly, which is the point.** `{` is a
YAML flow-mapping indicator, so a *plain* scalar cannot begin with `{{`:

```yaml
name: {{red}}The Temple Of Midgaard{{/}}    # parse error, with a line number
```

Compare what a `&`-based scheme does with the same input — `&rThe` is
silently consumed as an anchor named `rThe` and the value quietly loses its
first four characters, with nothing reported anywhere. A loud failure at
load time is worth a great deal more than a tidier first character, and it
is the single strongest argument for this form over the conventional one.

The mitigations are consequently mild: the writer quotes any single-line
string beginning with `{{`, and that is all. **Block scalars are unaffected
entirely** — `{{red}}` is literal text anywhere inside one, including as the
very first characters — and since all prose in this format lives in block
scalars, the wrinkle is confined to short single-line fields like a room
name.

**Colour is allowed in prose fields only.** Not in keywords: a player types
keywords, and a colour code inside one silently breaks object and mobile
matching in a way that is very hard to diagnose. Validation rejects it
rather than trusting builders to know.

Worth keeping as the round-trip test case, because it is the most hostile
string in the corpus and it already exists — one of the stock socials is a
cartoon swear made entirely of the characters that mean something to
somebody: `#`, `@`, `*`, `"`, `&`, `^`, `$$`, `%`. It contains no `{{`, so
under this scheme it needs no escaping at all and must survive verbatim.

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
- **Unbalanced colour is lintable.** A description that opens `{{red}}` and
  never returns to `{{/}}` bleeds into whatever is printed next. That is the single
  most common colour bug in MUD data, it is trivial to detect on a symbolic
  grammar, and `dlctl lint --type=world` reports it.

### 5.5 Converting back to CircleMUD

The requirement is that `yaml` data can go back to something the C server
runs. Colour is the only part of this format where that is not simply a
matter of re-encoding, so:

**Export (`yaml` → `classic`) renders codes to raw ANSI.** `{{red}}` becomes
`\x1B[31m` in the written file. This is correct CircleMUD: the C server
passes ESC through untouched and its pager already skips it. Exporting
`{{red}}` *literally* would show players those seven characters, so rendering is
mandatory rather than an option — and it is the reason `screen.h`'s exact
byte sequences are the table above rather than some tidier ANSI of our own.

**Import (`classic` → `yaml`) recognises and demotes.** ESC sequences
matching `screen.h`'s table become named codes. Nothing in the source needs
escaping, because `{{` does not occur in it.

**Anything unrecognised survives via `{{sgr:…}}`.** If the archive turns out to
contain 256-colour or truecolour sequences — `\x1B[38;5;208m` — there is no
named code for them, so they import as `{{sgr:38;5;208}}` and export back to the
identical bytes. This is what makes import lossless in all cases rather
than only the easy one, and it means the format never has to refuse a file
because someone was ambitious with colour in 2004.

**The honest caveat: the round trip normalises.** `\x1B[1;31m` and
`\x1B[31;1m` are the same colour and both import as `{{bright-red}}`;
re-exporting emits
`screen.h`'s spelling. So `classic → yaml → classic` preserves *meaning*
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
  renderer that turns `{{red}}` into a `<span class="c-red">` is a lookup
  table;
  one that turns `\x1B[1;31m` into the same thing is an ANSI state machine
  that has to be right about every sequence a builder ever typed.
- **GMCP and MXP** want structured text for the same reason.
- **Everything that is not a terminal** — search, the parity dump, logs,
  `dlctl` — strips colour by matching one unambiguous bracketed grammar
  rather than by parsing escape sequences.

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

**Built 2026-08-23, and living here since 2026-08-28.** Ten of the fields
sketched above shipped — `docs/deviations.md` has which, and why the rest
of `config.c` stayed a constant — as flat keys rather than the sections
above, there being ten of them and no `schema:` stamp yet. The *location*
is this section's, unchanged: `<lib-dir>/config/game.yaml`, read at boot
and re-read on `SIGHUP`, optional (no file is `config.c`'s own behaviour
exactly), and carried across unconverted by `dlctl import`. It shipped
first as a repo-level `config/` directory named by `--config`, which was
the wrong place for exactly the reason the next paragraph gives; `--config`
remains as a path override. Every `examples/` data directory ships the
annotated template, in both formats.

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

A note on the transcode those files are supposed to need. `dlctl convert`
assumes CP1252 input, on the good evidence that the *stock* CircleMUD world
shipped in `data/` contains high bytes. A snapshot of the original
Disgracelands `lib/` does not: its world, text and misc directories are pure
7-bit ASCII, so the transcode is a no-op on the data that actually matters.
That is not a reason to drop the step — this is one snapshot of an archive
that holds over a thousand nightly ones, and a converter that assumes ASCII
and meets a curly quote does silent damage — but it does mean encoding is a
smaller risk than it looked, and `dlctl lint --type=world` reporting what it
transcoded is more useful than any amount of care taken up front.

Only help needs structure, because a help entry genuinely has fields —
what keywords reach it, and what level may read it. That structure goes in
an index, not in the text:

*(Checked against the real C, step 6c-i: `struct help_index_element`,
`db.h:207-211`, carries no level field at all — nothing in `do_help` ever
gates an entry by who is asking. `min_level` below is this sketch's own
invention, not something the archive has any data for; a real yaml
design should drop it rather than gate something the C never gated.)*

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
pfile and rent file disagree — which is how items get duplicated. Because
this is the one format where `Object instances nest` (above) is not just a
storage-layer nicety but something the *loader* can act on, it is also the
one format where an item saved inside a bag comes back inside it —
`internal/server/rent.go` never actually threw the container tree away at
runtime, only the round trip through `binary`/`ascii`'s flat rent file did.
Landed as a deliberate, explicitly scoped deviation (`docs/deviations.md`,
"Renting empties your bags and strips your body") rather than something
this section originally anticipated in that much detail — running
`--player-format=yaml` is what turns it on; `binary`/`ascii` are
byte-for-byte unchanged. Stock auto-equip (putting worn items back *on the
body*) is a separate deviation nobody has approved, so there is no
`equipment:` section either — see `internal/persist/player/yaml/doc.go`.

**No `plr_index`.** The roster is built by scanning `players/` at boot and
held in memory; §3 covers why.

**Credentials are scheme-prefixed for anything but the legacy DES hash.**
`go-port-plan.md` §5.4's actual decision, which this section's own example
above got slightly ahead of: a bare, unprefixed hash is DES by definition,
since that is all any format here has ever held and a DES `crypt(3)` hash
can never contain a colon — nothing has to infer it from a
`des-crypt10$`-style marker this section once sketched but neither `ascii`
nor `yaml` actually writes.

**`aliases:` is a list, not a map.** The example above shows `{name:
replacement}`; what shipped is `[{name, replacement}]`. Order is meaningful
— `do_alias` prepends, so a player's own `alias` listing is newest-first —
and a YAML mapping cannot preserve insertion order the way a sequence does.

**The field set matches what `internal/persist/player/ascii/codec.go`
actually persists, not this section's own illustrative sketch.** `race:` is
in the real schema (ascii writes it; the sketch above does not); there is
no `real_*`/base-values block, because neither `ascii` nor `binary` has
ever persisted one — a fresh login recomputes the live figures from
whatever *was* saved, the same as it always has.

---

## 9. The rest of the state

Small formats, listed for completeness because "all of it has to move
together" is the entire point.

Four of these are now built — step 6a, `internal/persist/{bans,boards,
mail,houses}`, each retrofitted to the `Store` interface/registry shape
`world` and `player` already had, `classic` moved to its own subpackage
unchanged, `yaml` added beside it, `--state-format` selecting between
them, `dlctl import --type=state`/`fmt` converting and canonicalising. All three
struct-dump ones were already ported and working before this — boards,
mail, houses — which is the right order: the C formats had to be readable
before there was anything to convert *from*, and those ports' test
oracles were most of the work a converter needed anyway.

**`state/boards.yaml` ✅** — replaces the `board.*` struct dumps. One file
holding every board's messages together, keyed by the name classic used as
a filename (`board.mort` etc.) — board *definitions* (vnum, read/write/
remove levels) stay a hardcoded Go table, matching the C's own compiled-in
`board_info[]`; there was never anything data-driven there to convert. The
stock `MAX_BOARD_MESSAGES` of 60 and `MAX_MESSAGE_LENGTH` of 4096 are
properties of a fixed-size array, not of the game, and do not come along.

**`state/mail.yaml` ✅** — replaces the 100-byte-block linked list in
`mail.c`. A flat list of `{to, from, sent, text}`, oldest first by
construction (append on send, take-the-first-match on receive) rather than
needing the reversal trick classic's own `findHeader` does to reproduce
the C's list-order quirk. The free-block list, the block chaining and the
`BLOCK_SIZE` arithmetic all vanish; they existed only because the file was
a hand-rolled allocator.

**`state/houses.yaml` ✅** — replaces `etc/hcontrol` *and* the
`<vnum>.house` object files together: a house's own control record and
its contents sit nested in the same entry, one file instead of a control
array plus a directory of per-room files. Contents reuse the player
format's object-instance schema (§8) directly, always flat — real
containment was step 5's own explicitly-scoped deviation for player rent
files, not extended here. The `MAX_HOUSES` of 100 and `MAX_GUESTS` of 10
go the same way as the board limits.

**`state/reports.yaml` ✅** — the `bugs`, `ideas` and `typos` files, which
were three append-only text logs with a timestamp convention that could
only hold a month and a day, no year. One list with a `kind`, a reporter,
a room, a body and (for anything filed after this landed) a real
timestamp — which makes them something a tool can triage instead of
something someone greps. Landed in step 6b alongside `do_gen_write`
(`bug`/`idea`/`typo`) itself, in that order: the format followed the
feature, the same lesson `alias` and containment already taught in step 5.

**`config/names.yaml` ✅** — `misc/xnames`, a list of disallowed name
substrings. Unchanged in substance; it is a list of strings either way.
Character creation now consults it (`Server.DisallowedName`, step 6b) —
this landed once the feature existed to put a format under, the same
order every other piece here insists on.

**`state/bans.yaml` ✅** — the siteban list (`etc/badsites`, `BAN_FILE` in
`db.h`, read and written by `ban.c`). A flat list of `{site, type, when,
by}` — the smallest of the four, and the first built for exactly that
reason. It was also missing from an earlier draft of this section entirely,
which is the hazard of enumerating a format's contents from the parts that
are interesting rather than from a directory listing.

**`state/clock.yaml` ✅** — the MUD clock. `db.c` keeps the game's epoch in
`etc/time` as a bare integer with nothing around it: no name, no units, no
format marker, one number in a file. It is the smallest thing in `lib/` and
it is genuinely global state — reboot without it and in-game time jumps —
so it needs somewhere to live that is not "a number in a file called
`time`". Landed in step 6b as an RFC 3339 timestamp, saved on the same
thirty-real-minute cadence and at shutdown the C uses
(`PULSE_TIMESAVE`/`comm.c:441`), including the C's own lossy
epoch-reconstruction on every save (`docs/weirdnumbers.md`'s "Saving the
clock loses up to an hour, on purpose").

---

## 10. Versioning, validation and the canonical writer

### 10.1 Schema versioning

Every file carries `schema: dl/<kind>@<major>`. The major version changes
only for a breaking change, which means one that an older loader would
misread rather than merely fail to understand. Additive changes — a new
optional field, a new flag name, a new object type block — do not bump it.

This is a per-file, per-*kind* tag: it says what shape one zone file, or
one player file, is in. It says nothing about which release of the yaml
packages as a whole last touched a directory, which is a separate,
coarser question — one `major.minor.patch` for every subsystem together,
checked once at boot rather than once per file read. See
`docs/design/data-format-versioning.md`.

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
undocumented leniency in the C parser and there was no validator. `yaml`
has a validator, so it can afford to be strict.

### 10.2.1 Adding a field, and what an absent one means

Strict decoding has a consequence worth stating separately, because it
constrains every future change to this format: **a format can only grow by
adding fields that are optional**, or by a major version bump. An
addition that is required is an addition that makes every existing
directory fail to load.

An optional field's default in Go is whatever the zero value happens to
be, and that is right often enough to be a trap. The first field whose
sensible default is `true`, or `-1`, or where "unset" and "zero" mean
different things, is silently wrong for every directory written before it
existed — and nothing reports it, because an absent field is exactly what
`omitempty` produces for a field that *is* at its default.

Three rules, from `docs/proposals/yaml-only.md` §6:

1. **A new optional field's default is declared explicitly, never
   inherited from Go's zero value.** If the zero value is the right
   default, say so where the field is declared; if it is not, the reader
   applies the real one after unmarshalling.
2. **A new field no legacy format can source is named in the importer's
   own output** — once, summarised — so an operator converting a real
   archive learns which values are this port's choice rather than their
   data. This is the same posture `import` already takes toward
   transcoding counts and toward the enhanced-mobile espec keys it drops.
3. **A minimal-document test per subsystem**: unmarshal a document
   holding only the required fields and assert every optional field
   against its declared default. `internal/persist/defaults` is that
   suite, one test per subsystem in one place, deliberately — the
   interesting failure is one subsystem quietly disagreeing with the
   others about what an absent field means.

There is nothing in the format today whose sensible default is not its
zero value, which is why rule 1 has no table behind it yet. Rule 3 is
what makes the first exception fail loudly instead of shipping.

### 10.3 The canonical writer

Because the server writes these files, the writer needs properties the
reader does not care about:

- **Deterministic.** Fixed key order (schema order, not alphabetical — a
  room's `vnum` and `name` first, its prose in the middle, its exits last).
  Records in vnum order. Two-space indentation. No maps ranged over. Running
  `dlctl fmt --type=world` twice produces the same bytes, and a zone the server
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
  `{`. The case that will actually occur is `{`, from a coloured room name
  written `{{red}}…`; §5.3 covers it, and notes that unlike the `&`
  alternative it fails loudly rather than silently.
- **Byte-preserving on round trip.** Every string that goes in comes back
  out identical, including trailing whitespace, tabs, leading indentation and
  colour codes. This is the property §2.4 flags as the real risk, and it is
  tested by fuzzing the writer over the entire real corpus, not by
  inspection.
- **Atomic.** Temp file, `fsync`, `rename`.

### 10.4 Tooling

```
dlctl import --from-dir=data-old --to-dir=data                     # every subsystem, one lib/ to one fresh yaml directory
dlctl import --type=world --from-dir=data-old --from-format=classic --to-dir=data
dlctl export --type=world --to-dir=data-classic --to-format=classic  # refuses on data loss (not built — §11 step 3)
dlctl lint --type=world --dir=data                                 # replaces scheck
dlctl fmt --type=world --dir=data                                  # canonicalise in place
dlctl data verify                                                  # every file, every schema (not built)
dlctl data version --dir=data                                      # which release wrote the directory, and whether this one will load it; docs/design/data-format-versioning.md
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
`internal/persist/world/yaml/` implements `world.Source` and `world.Sink`;
`internal/persist/player/yaml/` implements `player.Store`. Nothing in
`internal/game` changes, because the canonical model is already
format-neutral and that is the whole reason it exists.

| Step | What lands | Done when |
|---|---|---|
| **1. Vocabularies ✅** | Name tables for every flag set, sector, position, item type, apply location and wear slot, in `internal/game/yamlnames.go`, with round-trip tests (`yamlnames_test.go`) against the C-sourced display tables in `bitnames.go`/`object.go`. | Every bit in the stock world has a name or round-trips via `flags_raw`. |
| **1b. Colour ✅** | The `{{…}}` parser, ANSI renderer and stripper, keyed off `PrefColour1`/`PrefColour2`, plus `DisplayWidth` — `internal/game/colour.go`. | The swearing social in §5.3 survives parse → render → strip unescaped and unchanged (`colour_test.go`); no wrapping layer exists yet to consume `DisplayWidth`. |
| **2. World read ✅** | `yaml` as a `world.Source` (`internal/persist/world/yaml/`), plus `dlctl import --type=world` from `classic`, including CP1252→UTF-8 transcoding and ESC → named-code demotion. | `dlctl import --type=world` on the real `data` produces zero `dlctl lint --type=world --dir=data --format=yaml` findings, and `dlmud --world-format=yaml` boots, populates and serves a connection. |
| **3. World write ✅ (export not yet)** | `world.Sink` (`WriteZone`), the canonical writer (`text.go`'s `Text`/`NestedText`), `dlctl fmt --type=world`. `dlctl export --type=world` (yaml → classic) is **not built** — it needs a classic-format *writer*, which does not exist in this tree at all yet (`classic` has only ever been a reader). | `classic → yaml → classic` round-trips byte-identical at the in-memory/parity-dump level for the whole real world (`yaml/parity_test.go`, 30 zones / 3,202 records) with no lossy transform left to except (trailing blank lines used to be one; `text.go`'s `needsQuoting` now escapes them instead — see §12); `fmt` is idempotent, verified against the real corpus. |
| **4. Flip the default ✅** | `data/` converted in the repo, `classic` demoted to import-only. | Done, and taken past "flip the default" to **there is no other default**: `--world-format` does not exist any more, nor do the other six format flags (`docs/proposals/yaml-only.md` §3.1). `examples/stock/yaml` is `dlmud`'s `--lib-dir` default, and pointing it at a legacy `lib/` is refused at boot with the `dlctl import` line for that directory. |
| **5. Players ✅** | `yaml` player store (`internal/persist/player/yaml/`), implementing both `player.Store` and `player.ObjectStore` against one file (§8); `dlctl import --type=pfile`/`fmt --type=pfile`; the `alias` command (`interpreter.c`'s `do_alias`/`perform_alias`, previously unported — no archived alias data exists anywhere to have ported instead); real container nesting, format-gated on `yaml` as a user-approved deviation (`docs/deviations.md`). | `dlctl import --type=pfile` converts a `binary` roster (rent files included); `dlmud --player-format=yaml` boots, and a character created on it, quit and logged back in, keeps a bag's contents *inside* the bag — proven live (`TestRentingUnderYamlKeepsTheRingInTheBag`), not just at the codec level. `PlayerRecord`/`player.Store` needed no restructuring: `ObjectStore` was already a separate interface a format could additionally implement, and `StoredObject` grew one field (`Contains`) rather than being redesigned. |
| **6a. Bans, boards, mail, houses ✅** | `yaml` for each (`internal/persist/{bans,boards,mail,houses}/yaml/`), every one retrofitted to the `Store`/`Register`/`Open` shape `world`/`player` already had (`classic` moved to its own subpackage per format, unchanged); `--state-format`; `dlctl import --type=state`/`fmt`, converting all four together. Houses' contents reuse the player object-instance schema directly, always flat (containment stayed scoped to player rent files, §8's own note in this table's row 5). | `dlctl import --type=state`/`fmt` round-trip a synthetic fixture (no real archived data exists for any of these four — confirmed, not assumed, in the scoping survey); a live server integration test per format proves each one end to end (posting/reading a board message, sending/receiving mail, a ban refusing a connection, a house crash-save surviving a reload), all under `--state-format=yaml`. |
| **6b. xnames, the clock, reports ✅** | Three of step 6a's own deferred pieces, each small enough to build the feature and its format together in one pass: character creation now consults `misc/xnames`/`config/names.yaml` (`internal/persist/names`, `--names-format`); the MUD clock's epoch is persisted (`internal/persist/clock`, joining `--state-format`, `Live.SetBooted`/`SavedEpoch`); `do_gen_write` (`bug`/`idea`/`typo`) is implemented and gets `state/reports.yaml` (`internal/persist/reports`, joining `--state-format`). | A disallowed name is refused at creation; a persisted epoch survives a simulated restart within the C's own sub-hour rounding bound (`docs/weirdnumbers.md`); `bug`/`idea`/`typo` append, refuse NPCs/empty text/a full file, and round-trip through `dlctl import --type=state`/`fmt` — all proved with live server integration tests, not just at the codec level. |
| **6c-i. Help ✅** | The real keyword lookup: `do_help`'s binary search plus backward-walk over `text/help/index` and the `.hlp` files it lists, ported to `internal/game/help.go` and wired through `internal/server/text.go`; `help circlemud` now reaches the real archived `CIRCLE CIRCLEMUD CREDITS` entry instead of a special case. | `help`, `help <keyword>` and the ambiguous-prefix behaviour all proved against the real 216-entry, 86KB archive (`internal/game/help_test.go`), plus a live server test that `help circlemud`/`help credits`/`help circle` show the real credits text with no special case in the command (`internal/server/help_test.go`). |
| **6c-ii. Damage messages: weapon swing, kick, bash, backstab ✅** | `dam_message`'s compiled severity table and `skill_message`'s full `misc/messages` lookup, ported for `internal/server/violence.go`'s `s.hit` (`Damage`, the weapon-swing dispatch) and the three `SkillDamage` callers `do_kick`/`do_bash`/`do_backstab` — `skillHit`/`skillMiss`'s old fixed strings are gone entirely, and a miss for any of the four is no longer a separate code path (`amount 0` through the same `applyDamage` a hit uses, matching `hit()`'s/`do_kick`'s/etc.'s own `damage(ch, vict, 0, ...)` calls). | Verified against the real archive (`data/misc/messages`, 55 records, including its one backstab/bash entry and two kick variants) as well as synthetic fixtures — live server tests prove a registered entry wins on a miss/death blow and loses to the compiled table on an ordinary weapon hit, and that a non-weapon attack with nothing registered is genuinely silent (no fallback at all). |
| **6c-iii. `config/messages.yaml` ✅** | `internal/persist/messages`, mirroring `internal/persist/names`'s shape (a `Load`/`Save` pair, not a full `Store` registry — the C never writes `misc/messages` at runtime either) rather than `reports`'/`bans`' fuller one. New `game.AttackTypeName`/`AttackTypeFromName` name a record's attack type across the two numeric spaces `misc/messages` mixes — weapon types via `YamlAttackTypeNames()`, spells and skills via `SpellNameOrNumber` (already covering `SkillBackstab`/`SkillBash`/`SkillKick`, since `spellTable` is one table for both) — falling back to `#N` for either space's unnamed numbers. New `--messages-format` flag, its own rather than sharing `--names-format`'s, for the same reason `--names-format` did not join `--state-format`. `dlctl import --type=messages`/`fmt` mirror `names`' own two commands. | `dlctl import --type=messages` against the real 55-record archive produces byte-identical records read back through yaml — confirmed by a test that parses classic directly and compares, not by inspection. A live server test proves `LoadText(dir, "yaml")` — the same path `--messages-format=yaml` drives — resolves a real kick message through `config/messages.yaml`, not just that the codec round-trips in isolation. |
| **6c-iv. `config/socials.yaml` ✅** | `internal/persist/socials`, the same `Load`/`Save`-pair shape `messages`/`names` already use — `misc/socials` is never written at runtime either. No split numeric space to reconcile this time: `min_victim_position` is a single `Position` enum, named via `game.NameByValue`/`ValueByName` against `game.YamlPositionNames()`, the same table the world format already uses for a mobile's `position`/`default_position`. `no_arg`/`found`/`self` are each an object of `{char, others[, victim]}`, `omitempty` down to the whole block being absent for a social that does not fill it — `found`/`not_found`/`self` are naturally all empty together, since the C only ever reads them as one group gated on `CharFound != ""` (`Social.TakesTarget()`). New `--socials-format` flag, its own for the same reason `--messages-format` is. `dlctl import --type=socials`/`fmt` mirror `messages`' own two commands. | `dlctl import --type=socials` against the real 104-record archive (`data/misc/socials` — 104, not the 105 lines in the file; one is a stray `you` entry with no command-table slot the C itself drops, `docs/proposals/go-port-plan.md`'s own count) produces byte-identical records read back through yaml. A live server test proves `LoadText(dir, ..., "yaml")` resolves the real archive's `smile` entry with its real message through `config/socials.yaml`, not just that the codec round-trips in isolation. |
| **6c-v. Damage messages: every offensive spell ✅** | `SkillDamage` was already `skill_message` alone, no `dam_message` fallback, the same non-weapon path `do_kick`/`do_bash`/`do_backstab` use — `mag_damage`'s own C confirms it is the right one, ending with `return (damage(ch, victim, dam, spellnum))` (magic.c:294), the identical dispatch with a spell number standing in for a skill's. `internal/session/cast.go`'s `spellDamage` — the one caller left on `Damage`'s no-message path, printing its own fixed "You blast .../blasts you with..." text — now calls `SkillDamage` instead, spell number as attack type, with nothing else to change: no new server-side code, no format change, only the one caller. | Verified against the real archive: every spell `game.SpellDamage` computes damage for has a registered entry except the two local joke spells (`ouchie`, `immolate`) — confirmed by parsing the archive, not by inspection. A live test casts Magic Missile end to end and gets the real archive's own hit line back; `TestSkillDamageTreatsSpellNumbersLikeSkillNumbers` proves the registered/unregistered split (Magic Missile vs. Ouchie) holds for a spell number exactly as it already does for a skill's. |
| **6c-vi. `text/help/help.yaml`, one file per entry ✅** | `internal/persist/help`, the same `Load`/`Save`-pair shape `messages`/`socials`/`names` already use — nothing writes the help database at runtime either. New `game.HelpSlug` names each entry's file: every keyword joined by a space (not just the first, the doc's own illustrative example — checked against the real archive, this is what makes all 216 entries slug distinct rather than colliding six ways on a shared first token), lowercased, non-alphanumeric runs collapsed to one `-`; the writer disambiguates any residual collision with a numeric suffix and falls back to a positional name for the one keyword line that slugs to empty (`! ^`, pure punctuation). `min_level` — the doc's own worked example invents it, and its own annotation already says to drop it (`struct help_index_element`, db.h:207-211, has no level field) — is not in the format. Classic and yaml share `text/help/` itself rather than splitting `misc/`/`config/` the way `messages`/`socials`/`names` do, since classic there is already multi-file; `dlctl import --type=help`/`fmt --type=help` default `--to-dir` to the same base as `--from-dir`, mirroring `import --type=world`. (`--type=help`, not `helpdb`: that name only ever existed because `dlctl`'s own bare `help` is reserved for its usage listing, so a *subcommand* literally named `help ...` would have been unreachable — a `--type` value has no such collision, see `docs/operations.md`.) New `--help-format` flag. | `dlctl import --type=help` against the real 216-entry archive produces byte-identical records read back through yaml, and `fmt --type=help` is idempotent (identical `help.yaml` byte-for-byte on a second run) — both checked, not assumed. A live server test proves `LoadText(..., "yaml")` resolves the real archived `CIRCLE CIRCLEMUD CREDITS` entry — the same licence-obligation lookup 6c-i's own classic test proves — through `help.yaml` and its `.txt` files, not just that the codec round-trips in isolation. |
| **7. Retire ✅** | `ascii` and `binary` become `dlctl`-only; `classic` becomes import-only. | Done (`docs/proposals/yaml-only.md`). Note what this row's own wording needed sharpening about: retiring a format from the *server* is not deleting it from the *tree*. Every decoder here goes on being read, tested and released on every push; `dlctl` is simply the only thing that reads them, and they are not linked into `dlmud` at all — a legacy format is *absent* from the server rather than merely refused by it, and there is a test for that. §12's own "`classic` and `binary` are never deleted" paragraph below is unchanged. |

Steps 1–4 are worth doing before **Phase 6** of `go-port-plan.md`, which is
where OasisOLC and `Sink` writeback land: OLC writes zone files back, and it
would be perverse to implement a writer for `classic` and then a second one
for `yaml`. That makes Phase 5 — where steps 5 and 6a have now also
landed — the natural window. Step 6b can follow whenever, each piece on
its own schedule once the feature underneath it exists.

`classic` and `binary` are never deleted. `classic` is the world-format
parity oracle for as long as the C server is authoritative, and it is how
the 1,184 dated nightly world backups in the archive get read; `binary` is
the only format that can read the archived roster and rent files at all,
and remains the tooling's own path for reading them, even once a server
is running on `yaml`.

### 11.1 `dlctl import` — all seven at once, and a gap it exposed and closed

`dlctl import --from-dir=X --to-dir=Y` runs the seven importers above
in order, against `X`'s own `world/`/`etc/`/`misc/`/`house/`/`text/`
subdirectories, plus copying `text/`'s plain-prose files unchanged and
stamping `Y` with a `.dlversion` naming this build's own release
(`docs/design/data-format-versioning.md`) once everything else has
succeeded — see `docs/operations.md`'s own
getting-started walkthrough for running it against a real archive.

Found while writing that walkthrough, checked with a synthetic CP1252
fixture rather than assumed: **only two of the seven importers transcoded
non-UTF-8 text on their own.** `import --type=world` and `import
--type=pfile` each had their own `--encoding` flag and decoded CP1252 (or
whatever was named) the same way `dlctl convert` does; `--type=state`/
`names`/`messages`/`socials`/`help` read whatever bytes were in the
source file and wrote them straight into the `yaml` output, UTF-8
declaration and all. Pointed at a source file that is genuinely CP1252 (a
curly quote in a social, an accented name on the `xnames` list), the
result was a `.yaml` file that was not valid UTF-8 despite saying it was.
`examples/stock/` never surfaced this, because stock CircleMUD's own text
is pure ASCII throughout and ASCII is valid UTF-8 unchanged — the gap was
real but inert against every fixture in this repo, which is exactly the
kind of thing worth writing down rather than leaving for the first real
archive to find silently.

**Closed**: all seven `--type`s now take `--encoding` and decode the
same way, `import` with no `--type` passing its own flag through to
every one of them. Each of the five gained a `transcode*` helper
mirroring `import --type=world`'s
own `transcodeWorldStrings` — walking each format's actual free-text
fields (a board's `Heading`/`Body`, a mail message's `Text`, a report's
`Body`, a fight message's `Attacker`/`Victim`/`Room` per die/miss/hit/god
set, a social's eight message fields, an `xnames` entry, a help entry's
`Body`) and leaving alone what is not prose (a ban's hostname or admin
name, a house's numeric fields and vnum-only stored objects — checked
field by field, not assumed uniform with the ones that do carry text).
Verified against a genuinely non-ASCII fixture, not just a re-run of the
always-ASCII `examples/stock/` corpus, the same reasoning the original
finding used.

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

**~~Colour in the archive.~~ Answered — it is the cheap outcome.** A
snapshot of the original `lib/` has now been surveyed, and there is no
colour in any builder-authored content: the world files, the help corpus,
the screen text, the socials and the damage messages contain **zero** escape
bytes between them. The only real colour anywhere in the tree is a
double-figure count of SGR sequences inside one archived player database,
drawing on two distinct codes — a foreground colour and a reset — which is
players having typed escapes into their own titles and descriptions.

Three consequences. The scheme in §5 covers the real data comfortably —
and, since there is no existing colour convention to preserve, it was free
to be chosen for YAML-safety and legibility rather than for compatibility.
`{{sgr:…}}` is insurance rather than a requirement, and should be kept anyway,
because it costs nothing and one snapshot is not the whole archive. And
colour is a *player-data* problem rather than a world-data one, which means
it lands with step 5 of §11 and not step 2 — the opposite of where this
document originally implied the risk was.

One trap worth recording for whoever writes the importer: a naive
`grep -c $'\x1b'` over `lib/` returns a number roughly fifty times the true
one. The player database is a `struct` dump that is ~89% non-printable and
uses all 256 byte values, so `0x1b` occurs constantly inside integer fields
and means nothing. Escapes must be counted only in the text fields of a
parsed record, never over the file's bytes.

**CRLF in multi-line strings — a real, and unavoidable, lossy transform,
found by the round-trip fuzz test rather than anticipated here.**
`classic.readString` reproduces `fread_string`'s behaviour of appending
`"\r\n"` after every line of a multi-line field that does not carry the
terminating `~`, so a loaded room/mobile/object description is `\r\n`-joined
in memory — not `\n`-joined the way the file on disk is. YAML block scalars
cannot represent CRLF as distinct from LF at all: every implementation
tested normalises every line-break style to `\n` on decode, which is a
property of the data model, not a library quirk. `yaml`'s stored form is
therefore always LF-only (`internal/persist/world/yaml/text.go`'s
`ToStored`/`FromStored`), and the `\r\n` is re-derived on the way back into
a `game.*Def`, exactly the same relationship `classic`'s own file bytes
(`\n`) already have to its in-memory form (`\r\n`). No data is lost — the
transform is a straight `strings.ReplaceAll`, lossless in both directions,
verified against the CRLF fields that are actually in the real corpus (the
astral-plane and river-zone room descriptions).

**A second finding, which used to be a lossy transform and is not one any
more: "keep" chomping.** A string with two or more trailing newlines — a
sign or note description with a trailing blank line before its closing `~`
— cannot round-trip through `goccy/go-yaml` v1.19.2's *block-scalar* path
at all: the library re-parses and re-prints whatever a `BytesMarshaler`
returns while splicing it into the surrounding document, and its own
re-print of a literal block node unconditionally strips every trailing
newline before re-adding exactly one, regardless of what chomping
indicator asked for. `|+` is emitted and then ignored, re-verified against
this library version rather than taken from the note that first recorded
it.

What changed is the response, not the finding. `yaml` used to collapse
such a string to a single trailing newline on write and report the
normalisation; it now writes the string as a quoted, escaped scalar
instead (`text.go`'s `needsQuoting`), which the same library carries back
unchanged. The trailing blank line is a blank line on a player's screen,
and "reported rather than refused" is the right posture for a transform
with no alternative — this one had an alternative. Two more shapes reach
the same escape hatch, both found the same way: a **bare carriage
return**, which is unrepresentable in a block scalar because YAML folds
CR, CRLF and LF alike into `\n` on decode (§5.4 of the spec) — a world
file whose text carries stray CRs is a thing that exists — and **trailing
whitespace on a final line with no newline after it**, which that same
re-print drops. Real-corpus incidence of all three together: 61 strings
out of 12,372, against 5,347 that still write as literal blocks.

**A third finding, about the indentation indicator itself (§4.6): it is
only reliable at one nesting depth.** The same re-parse-and-splice
mechanism shifts a returned literal block by a depth-independent constant
rather than by an amount proportional to where the field actually lands, so
a hand-built `|2` header that is correct for a room's own `desc` (always two
YAML-file columns deep, since `rooms:` is always a top-level list) silently
mis-decodes for a field nested any deeper — an exit's `desc`, an
extra-description's `desc`. `yaml` uses the indicator only where that one
depth is guaranteed (`Text`, for a room/mobile/object/shop's own top-level
fields) and falls back to a quoted, escaped scalar for anything nested
deeper that would need one (`NestedText`) — correct everywhere, at the cost
of losing block-scalar readability for the rare content (four ASCII-art
signs in the real corpus) that needs it at that depth.

**Alias persistence is not quite everywhere the roster is.** `ascii` and
`yaml` both persist `alias`'s definitions (§8); `binary` does not, and
this is a real, permanent gap rather than a "not yet" — `alias.c`'s
`plralias/` file format has zero archived instances anywhere in `data/` to
build a codec against or verify one with, which is exactly the situation
this codebase's own testing discipline (`CLAUDE.md`, "do not read the C
and transcribe it") says not to build blind. A character loaded from
`binary` simply starts with none, the same as before this section's
step 5 landed at all.
