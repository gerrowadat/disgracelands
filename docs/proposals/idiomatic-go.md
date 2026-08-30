# Idiomatic Go: retiring the C's data model from memory

`yaml-only.md` retired the C's file formats from the server's **disk**.
This retires the C's data model from the server's **memory**: the bit
vectors, the untyped `int32`s standing in for enumerations, the
fixed-length arrays whose slots mean something different depending on what
is in slot zero, the `-1`s that mean "absent", and the handful of
structural habits the port inherited because it was reading a C struct at
the time.

It is the same argument one layer inwards, and it is licensed by the same
row of the same table: `docs/design/go-port-plan.md` §0's **"Fidelity,
phase two"**, which since 2026-08-23 has said that new work may modernise
the implementation freely, with no sign-off and no entry in
`docs/deviations.md`, provided it does not touch **compatibility** or
**gameplay**. Everything in this document is implementation. §2 is the
fence, and it is drawn carefully, because "implementation only" is very
easy to say and this is a change that could break the game in ways no test
would notice if the fence were drawn casually.

> **Status, 2026-08-30: in progress.** §7's step table is the tracker;
> each row is struck through as it lands. `go-port-plan.md` and
> `yaml-only.md` both moved to `docs/design/` on the day this was written;
> neither is extended with new work.
>
> - **Step 0, the resave fixture — done.** It found something on its
>   first run: see §7's row.
> - **Step 1, a type per flag domain — under way.** `Set[T]` and the
>   raw-bits helper boundary landed with the first domain, room flags.
>   Read §4.1.1, the OR trap, before converting another one. Done so far:
>   **room**, **exit/door**, **container**, **shop**, **paladin spec**,
>   **spell targeting**, **spell routines**, **item wear**, **item extra**,
>   **mob act**, **affect**.

---

## 0. Decisions taken

Settled before this document was written; the rest of it assumes them.

1. **The target is a data model somebody would design today for this
   game, not a translation of `structs.h` that happens to compile.** Where
   a C shape and a Go shape disagree, the Go shape wins — the exact
   reversal of the port plan's original fidelity row ("when in doubt,
   read the C and do that"), and exactly what the row beneath it,
   "fidelity, phase two", permits.

2. **The on-disk format does not move.** Not a field, not a key, not a
   default. This is a change to what the server holds in memory and
   nothing else. §2.1.

3. **The game does not move.** No damage number, no message string, no
   ordering a player could see. §2.2, and it is the harder of the two to
   guarantee.

4. **`internal/persist/player/binary` is out of scope entirely, forever.**
   Its structs *are* the C's memory layout — that is the whole point of
   `reference/tools/*layout.c` and the two data models the codec is tested
   under. Modernising it would be modernising a fossil. §2.3.

5. **Mechanical and compiler-checked before clever.** The two largest
   steps (§7 steps 1 and 2) are the ones where the compiler finds every
   call site for you. They go first, for that reason and no other.

6. **One domain per pull request.** Every step below is a large diff. A
   step that touches eleven flag domains is eleven pull requests, not one.
   The diffs are boring; the review value is entirely in whether the
   *right* boring thing happened, and that is unreadable at ten thousand
   lines.

---

## 1. Why now, and why this is not a rewrite

Three things changed, and together they moved this from "premature" to
"overdue".

**The C stopped being the target.** Through Phase 5 the port was measured
against the C server, and a data model shaped like the C's made that
measurement cheap: when the port and the C disagreed, having the same
shapes in front of you was how you found out which was wrong. That work is
finished. `docs/deviations.md` records the disagreements, the parity
suites are green and triaged, and the Go server is canonical. The shapes
have stopped paying for themselves.

**The modern model already exists — on disk.** This is the observation
that makes the whole document a move rather than a design. Read
`internal/persist/world/yaml/values.go`: it has `WeaponValues{Dice,
DamageType}`, `ContainerValues{Capacity, Closeable, Pickproof, Closed,
Locked, Key}`, `DrinkValues{Capacity, Current, Liquid, Poisoned}`,
`LightValues{Hours}` and `ChargesValues` — a per-item-type taxonomy,
reconstructed from `oedit.c`'s value menus because the C source contains
no such specification as data. Read `internal/game/object.go:315` and it
is `Values [NumObjValues]int32`. The same is true of flags: the yaml files
say `flags: [glow, hum]`, and `internal/game/flags.go:21` says `type Flags
uint64`. The same is true of class and sex: the player file says
`class: warrior`, and `internal/game/player.go:38` says `Class int32`.

**The format layer is doing the conversion twice, in opposite
directions, and losing the type on the way through.** `dlctl import`
reads letters, produces names, writes names; the loader reads names and
produces bits. The named, typed, self-describing model is built,
tested and shipping, and it stops at the package boundary.

That is why this is not a rewrite. It is moving a shape that exists,
one package inwards, and deleting the flattening step.

---

## 2. The fence

The two things "fidelity, phase two" holds fixed, and one this document
adds.

### 2.1 Compatibility: the on-disk format does not move

Nothing here changes a byte of yaml. Concretely — three of these four are
checks that already exist, and the one that does not is §6's step 0:

- `dlctl import` over `examples/stock/binary`, `examples/mini/binary`
  and `examples/torture/binary` — the last being the deliberately hostile
  legacy `lib/` — must reproduce the checked-in yaml **byte for byte**.
  `cmd/dlctl/import_test.go` already asserts this on every push, and
  `release.yml` re-checks it against a fresh build.
- A directory written by the pre-change server must load into the
  post-change server and re-save identically, and vice versa. **This is a
  stronger claim than the one above and nothing checks it today**: every
  existing check starts from a legacy source and ends at yaml. Nothing
  goes yaml → memory → yaml, which is exactly the path a memory-model
  change alters. §6 proposes the check.
- `.dlversion` does not change. A data-model refactor is not a format
  version bump, and if it turns out to need one, that is the signal that
  the fence has been crossed and the step is wrong.

**The bit positions are part of the format and do not move.** This is the
subtle half. `flags: [glow, hum]` is the readable form, but every flag
field also has a `flags_raw` sibling carrying bits the name table does not
cover, and the legacy importers still decode `asciiflag_conv`'s letters
into bit positions. So a named flag type may replace `1 << 7` in the
source with `ItemNoDrop` in a table — but `ItemNoDrop` is still bit 7,
forever, and the test that proves the name table matches `constants.c`
(§5) is what keeps it so.

### 2.2 Gameplay: the game does not move

Harder, because "the same game" is not a property any single assertion
covers. Three specific hazards, in increasing order of how easy they are
to miss:

**Output strings.** Anything a player can see. The parity transcripts
(`test/parity`) and the play suite (`test/play`) cover this well, and both
are release-only, which makes a data-model change one of the few kinds of
work that should run `make parity` and `make play` *locally, before
pushing* — see §6.

**RNG draw order.** The game is deterministic given a seed, and every
number it draws comes from `internal/rng` in a fixed sequence. A refactor
that changes the *order* or the *count* of draws changes every subsequent
roll in that tick. This is not hypothetical for this change specifically:
several steps below replace a `[N]T` array with a map or a slice, and
**anything that draws inside a range over a map has just become
non-deterministic**. `PlayerRecord.Skills` is already a map; do not add
more of them on any path that rolls.

**Integer width — and this is the one that could quietly ruin the
game.** `docs/weirdnumbers.md` is 57 findings long and a great many of
them are arithmetic that truncates, overflows or wraps at a width the C
chose. §7 step 8 proposes moving the rules layer from `int32` to `int`,
which on every platform this ships to is a *widening*. A widening changes
the answer wherever an overflow or a truncation was load-bearing —
silently, in the direction of "more correct", which is exactly the
direction this project spent five phases refusing. That step is
therefore last, is gated on the oracles rather than on the type checker,
and may well end as "declined": see §7 and §10.

### 2.3 The archive-shaped code does not move

`internal/persist/player/binary` is a codec for `fwrite`d C structs. Its
field widths, its padding and its two data models (ILP32 for the real
archived data, LP64 for a modern rebuild) are the format. `layout.go` and
`reference/tools/*layout.c` exist to keep it honest and a "cleanup" there
is a compatibility break wearing a tidy hat. The same goes, less
absolutely, for the other legacy readers under `internal/persist/*/classic`
and `.../ascii`: they may be tidied, but their *numbers* are the C's.

---

## 3. The catalogue

What is actually C-shaped, with evidence. Counts are as of 2026-08-30 on
`main`.

### 3.1 One bit-vector type for eleven unrelated domains

`internal/game/flags.go:21` defines `type Flags uint64` and every flag in
the game is that type. There are at least eleven distinct domains —
`PLR_*`, `PRF_*`, `ROOM_*`, `AFF_*`, mob `ACT_*`, item extra, item wear,
container, shop, spell targeting, reset — declared across
`playerflags.go` (84 constants), `object.go` (33), `spell.go` (22),
`mobflags.go` (18) and five other files.

They share a type, so they share a namespace of *values*:

```go
PlayerKiller Flags = 1 << 0   // playerflags.go:19
PrefBrief    Flags = 1 << 0   // playerflags.go:42
RoomDark     Flags = 1 << 0   // playerflags.go:92
```

`rec.PlayerFlags.Has(RoomDark)` compiles, runs, and is true of every
killer in the game. So does `room.Flags.Has(PrefBrief)`. There is no
diagnostic for this anywhere in the toolchain — not `go vet`, not
`staticcheck`, not `gosec` — because at the type level nothing is wrong.
This is the single largest correctness argument in the document, and it
is worth being precise about what it is *not*: there is no evidence any
such mistake is currently in the tree. The argument is that if one were,
nothing would find it, and the port's own history (`isname`, twice) is
about exactly that class of thing.

### 3.2 `int32` standing in for an enumeration

Roughly 789 uses of `int32` across `internal/game` and
`internal/session`. A large share are honest widths at the format
boundary. The rest are enumerations that never got a type:

| Domain | How it is declared | Where |
| --- | --- | --- |
| Class | `Class int32` | `player.go:38` |
| Sex | `Sex int32` | `player.go:37` |
| Race | `Race int32` | `player.go:39` |
| Item type | `ItemLight int32 = 1`, … 22 more | `object.go:26` |
| Apply location | `ApplyNone int32 = 0`, … 24 more | `affect.go:23` |
| Spell / skill number | `spell int32` as a parameter, ~20 functions | `magic.go:28`, `spell.go:156`, `affect.go:56`, … |
| Sector type | `SectorType int32` | `world.go:17` |
| Liquid | `liquid int32` | `drink.go:151` |
| Position (mob file) | `Position int32`, `DefaultPosition int32` | `world.go:106` |

Some domains *did* get a type — `Direction`, `Position` (the live one),
`WearPosition`, `SaveType`, `Location`, `Condition`, `CredentialScheme`,
the four vnum/rnum pairs. The pattern is established and half-applied;
this step is finishing it, not inventing it. Note in particular that
`game.Position` is a defined type and `MobDef.Position` is an `int32`
holding the same domain.

The cost is not theoretical. `Title(class, level, sex int32)`
(`titles.go:44`) takes three values from three unrelated domains, in an
order nothing enforces.

### 3.3 Fixed-length arrays whose slots are positional

- **`Values [4]int32`** (`world.go:177`, `object.go:315`). What each slot
  means depends on `Type`. There are **84 raw `Values[n]` accesses** in
  non-test code. `container.go:32-41` names all four of a container's
  slots and `corpse.go` uses those names — the exception that shows the
  rule; `identify.go`, `fight.go`, `equip.go`, `objectmagic.go`,
  `light.go` and `damage_messages.go` all index by literal.
  `fight.go:141` reads `wielded.Values[1], wielded.Values[2]` as damage
  dice, and there is nothing to stop it being handed a fountain.
- **`SavingThrows [5]int32`** and **`RealSavingThrows [5]int32`**
  (`player.go:80`, `:119`), indexed by a `SaveType` that *is* a defined
  type — the type exists and the array is not keyed by it.
- **`Conditions [3]int32`** (`player.go:141`): drunk, full, thirsty, by
  position, with `-1` meaning "does not apply". A `Condition` type exists
  in `regen.go:71`.
- **`Equipment [NumWears]*Object`** (`live.go:241`) and
  **`Worn *[NumWears]*Object`** (`player.go:93`) — the second a pointer to
  the first, so that `affect_total` can see the equipment of a character
  the record does not otherwise know about. That back-pointer is a real
  design smell and §3.6 is about it.
- **`Exits [NumDirections]*ExitDef`** (`world.go:20`). This one is
  arguably fine — see §3.10.

### 3.4 `-1` where the type could say "absent"

`NoRoom`, `NoObject`, `NothingRnum` (`types.go:45-52`) are the C's
`NOWHERE`/`NOTHING`; `Object.WornAt` is initialised to `-1`
(`object.go:349`); `ExitDef.Key` is `-1` for no key; `Conditions` uses
`-1` for "immortal, not applicable"; `spec.go:94` has `guildAnyClass
int32 = -999`. Each of these is a value inside the domain being used to
mean "outside the domain", which is the thing Go's second return value and
pointer-nilability exist to avoid.

The trade is not free — a sentinel is one word and `*T` is a pointer
chase and an allocation — and §4.4 says where the line goes.

### 3.5 Name tables indexed by bit position

`bitnames.go` holds `affectBitNames`, `extraBitNames`, `applyTypeNames`
and friends as `[]string`, indexed by bit or by enum value, transcribed
from `constants.c`. `yamlnames.go` holds a *second* set, parallel to the
first, for the yaml spellings.

Two parallel `[]string` tables that must agree bit-for-bit, plus the C
source they are both derived from, is three copies of one fact. But this
is also the place to be most careful, because those tables are what
`bitnames_test.go` re-parses `constants.c` to check — see §3.10 and §5.

### 3.6 One struct doing two jobs

`PlayerRecord` (`player.go:29`) is documented as "a saved character, in a
form no file format owns", and it is also the thing the rules operate on:
`SpellDamage(spell int32, caster *PlayerRecord, victim *PlayerRecord,
...)`, `RecomputeAffects(rec *PlayerRecord)`, `SavingThrow`, `Remort`. It
carries `Worn *[NumWears]*Object` and `Mobile bool` — two fields that are
explicitly not saved and exist only so the rules can work — and `Character`
(`live.go:220`) holds a `*PlayerRecord` alongside the runtime state that
did not fit.

This is a direct inheritance of the C's single `char_data`, split once and
not finished. The seam is in the wrong place: the save format's shape is
deciding what the rules can see, which is the same complaint `yaml-only.md`
§3.1 made about `player.Store` and `player.ObjectStore`.

`Object` and `ObjDef` have a milder version: `NewObject` copies eleven
fields out of the prototype so an instance can diverge from it
(`object.go:344-364`), and every reader then has to know whether to ask
the object or the prototype. `MinLevel()` asks the prototype;
`ActionDescription()` asks the object and falls back.

### 3.7 Invariants maintained by hand that the type system could hold

`Object`'s location is five fields — `Location`, `Room`, `Holder`,
`WornAt`, `Container` — of which exactly one set is meaningful, decided by
the first. The file's own doc comment says so: "An object is in exactly
one place at a time, and that invariant is the whole point of this file."
It is enforced by everything going through the `Put`/`Take` functions,
which is a convention, not a constraint. Go has had a way to make this a
constraint since interfaces existed, and a tidier one since generics.

### 3.8 `int32` as the default width, and 129 lint suppressions

There are **202 `//nolint:gosec` comments** in the tree. **73 are file
paths** (G304, an unrelated argument). The other **129 are integer
conversions** — G115 — and their own one-line explanations tell the
story: 36 of them say some form of "reinterpretation, not truncation",
11 say "world-data-scale", and the rest are variations on "truncation is
the format" and "truncation is the C's arithmetic".

Each is individually correct and individually reasoned. Collectively they
are the cost of carrying a 32-bit-shaped model through a 64-bit language,
and a good half of them are conversions between things that would not need
converting if the flag types and the enum types existed.

### 3.9 Package shape

`internal/session/commands.go` is 2,147 lines; `internal/session/session.go`
1,253; `internal/server/server.go` 1,233; `internal/game/live.go` 903.
`Live` is a god object with ten maps on it. This is the least
important item on the list and the most likely to attract effort, which
is why it is last and why §9 puts most of it out of scope.

### 3.10 What looks like a C-ism and must stay

The most valuable section here, because a keen pass over §3.1–3.9 would
break all of these, and several would not fail a test until much later.

- **`Command.CLine` and the command table's sort order.** It looks like a
  gratuitous dependency on a C file's line numbering. It is the *only*
  thing making command abbreviation derived rather than asserted: the
  interpreter matches the first entry a typed word prefixes, so the
  table's order decides what every abbreviation means. Sorting it any
  other way silently changes twenty years of muscle memory. Do not touch
  it. `commands.go:35-45` explains itself at length; believe it.
- **Bit positions.** §2.1. The names may become types; the numbers are
  the format.
- **The reset opcodes** `M`, `O`, `G`, `E`, `P`, `D`, `R` as `byte`
  (`world.go:219`), and `fourArgCommands = "MOEPD"`. These are the file
  format's alphabet. A `ResetOp` type over them is fine; renaming or
  reordering them is not.
- **Wear-slot order** (`object.go:136-161`). Player-visible: it is the
  order `equipment` prints.
- **The direction order** (`types.go:57-70`). It is the order exits appear
  in the file, as `D0`–`D5`.
- **`Exits [NumDirections]*ExitDef`.** A fixed array over a closed,
  ordered, six-element domain indexed by a defined type is *already* the
  idiomatic answer. Turning it into a map would be a downgrade and would
  introduce iteration-order non-determinism (§2.2). It is on this list to
  stop somebody sweeping it up with the rest of §3.3 by pattern-matching
  on `[N]T`.
- **The C-derived tests, all fifteen files of them.** §5.
- **Every number in `docs/weirdnumbers.md`.** The arithmetic stays wrong
  in exactly the ways it is currently wrong.

---

## 4. The target shapes

Sketches, not specifications. Each step in §7 settles its own details.

### 4.1 Flags → a generic set over a per-domain enum

```go
// internal/game/set.go, as built
type Set[T ~int] struct{ bits uint64 }

func NewSet[T ~int](vs ...T) Set[T]
func SetFromRaw[T ~int](bits uint64) Set[T] // the format boundary, and only there
func (s Set[T]) Raw() uint64                // ditto, in the other direction
func (s Set[T]) Has(v T) bool
func (s Set[T]) HasAny(vs ...T) bool
func (s Set[T]) HasAll(vs ...T) bool
func (s Set[T]) With(vs ...T) Set[T]
func (s Set[T]) Without(vs ...T) Set[T]
func (s Set[T]) Toggle(vs ...T) Set[T]
func (s Set[T]) Union/Intersect/Minus(o Set[T]) Set[T]
func (s Set[T]) Overlaps(o Set[T]) bool
func (s Set[T]) All() iter.Seq[T]           // ordered by bit, so deterministic
func (s Set[T]) Members() []T
```

The sketch had an `Unknown() uint64` for the `flags_raw` case; there is
none, because it would need a name table and every caller that has one is
already calling `NameBits`, which returns the unnamed remainder as its
second result. That is the same fact in the place that can compute it.

with domains declared as bit *indices* rather than masks:

```go
type RoomFlag int
const (
    RoomDark RoomFlag = iota
    RoomDeathTrap
    RoomNoMob
    ...
)
```

`Set[RoomFlag]` and `Set[PlayerFlag]` are different types, so §3.1's
mistake stops compiling. `Raw`/`Unknown` are what the persistence layer
uses and nothing else does, which is also where the remaining G115
suppressions belong.

**One generic type, not eleven hand-written ones — settled here rather
than left to the first PR, because it is the most consequential API
decision in the plan and it wants deciding once.** Eleven concrete types
(`RoomFlags`, `PlayerFlags`, `AffectFlags`, …) would read better at a call
site and in a panic, and would match a codebase that has stayed almost
entirely generics-free. They would also be roughly sixty-six
near-identical methods to keep in agreement, which is the duplication
§3.5 complains about, reinvented.

**Know what is being bet, though: this would be the tree's first generic
type.** There is exactly one generic *function* in 127,000 lines —
`emptyIfNil[T any]` (`internal/persist/world/dump.go:356`) — and no
generic types at all. `Set[T]` would not be a quiet addition in a corner;
it would appear in every flag-bearing signature in `internal/game` and
`internal/session`, and in every stack trace through them. That is a real
change to how this codebase reads, taken deliberately. The fallback if it
reads badly in practice is the hybrid — an unexported generic core with
eleven thin named wrappers — and step 1's first PR is where that would
become obvious, on one domain, before the other ten follow it.

**The name tables and the letter encoding take raw `uint64`, not any flag
type.** Settled by building it: once every domain has its own set type
there is no single Go type left to write `SprintBit`, `NameBits`,
`ParseBitNames` or the `asciiflag_conv` letter codec in. Making each of
them generic buys nothing — none can do anything with `T` it cannot do
with the bits, and it would force the domain to be named at every call
site — and keeping eleven copies is §3.5's duplication reinvented. So
those four operate on bits, `Set.Raw`/`SetFromRaw` are the only
conversion, and every layer above them is written in the domain type.
That is also exactly where §4.1 puts the surviving G115 suppressions.

### 4.1.1 The OR trap

**A domain's constants are bit indices, so the C-shaped idiom still
compiles and means something else.**

```go
room.Flags.HasAny(RoomNoMob | RoomDeathTrap)   // compiles. asks about RoomIndoors.
room.Flags.HasAny(RoomNoMob, RoomDeathTrap)    // what was meant
```

`2 | 1` is `3`, which is a perfectly good `RoomFlag`. The types are
right, the arity is right, and the answer is silently about a different
flag — which is the same "no diagnostic anywhere in the toolchain" shape
§3.1 complains about in the model being retired, so trading one for the
other would be a poor bargain.

It is not hypothetical: converting the *first* domain produced two of
them, in `internal/game/live.go` (a mobile walking into a NOMOB room) and
`internal/session/spells.go` (summon into a private, death-trap or god
room). The first was caught by a test somebody wrote years ago. Nothing
at all was watching the second.

Go cannot make this a type rule — any `~int` supports `|`, and the OR of
two valid indices is another valid index — so it is a source scan:
`internal/game`'s `TestNoFlagConstantIsCombinedWithAnOperator` parses
every Go file in the tree and rejects `|`, `&`, `&^` or `^` between two
constants of the same converted domain. It derives the constant list from
the `const` declarations rather than a hand-maintained list, so **each
domain a later step converts is covered the moment it lands**, with
nothing to remember. Run it before pushing a conversion; it is part of
`go test ./...`.

**Where the compiler does catch it, and where it does not.** Learned on
the second domain, and it is the single most useful thing to know when
converting the remaining nine, because it says where to look.

An `A | B` is an ordinary value of the domain type. So it fails to
compile wherever the surrounding code wants a *set* — a return value, a
struct field, a function parameter — since `Set[ExitFlag]` and
`ExitFlag` are unrelated types:

```go
func DoorState(doorFlag int32) ExitFlags {
    case 2: return ExitIsDoor | ExitPickproof   // does not compile
```

and it compiles perfectly wherever the surrounding code wants a
*variadic* `...T` — `Has`, `HasAny`, `HasAll`, `With`, `Without`,
`Toggle`, `NewSet`:

```go
exit.State.Clear(ExitClosed | ExitLocked)       // compiles. index 3.
```

Every conversion so far has split along exactly that line: room flags
produced two silent ones and both were `HasAny` calls; exit flags
produced seven, of which three were the compiler's and four were not;
container flags produced four and the compiler caught **none** of them,
because a container's flags live in an object value slot rather than a
field, so every site is either a variadic call or an `int32(...)`
conversion. `go build` and `go vet` were both clean and four assertions
were about the wrong flag.

**A clean build is not evidence.** The variadic call sites are the whole
of the risk surface, and they are easy to enumerate — grep the domain's
constants for the seven method names above before trusting the build.

### 4.1.2 Two hazards the scan does not look for

The OR trap is the one with a check. These two have none, and both were
found by reading and by tests rather than by tooling.

**A zero literal changes meaning.** Under masks, `0` is "no flags" and
`Has(0)` is trivially true; under indices, `0` is the domain's *first
flag*. `object.go`'s `wearFlagFor` had `WearLight: 0` — "anything can be
a light" — which a naive conversion would have turned into "the light
slot requires ITEM_WEAR_TAKE", compiling cleanly. The answer is an
element type that can express emptiness: the table is
`[NumWears]WearFlagSet` and the entry is `{}`, with `Set.Contains`
(whose empty case is the C's `IS_SET(x, 0)`) doing the test.

**A `1 << i` shift over a name table stops being a mask.** Where a table
is indexed by bit position, the old idiom was
`flags.Has(1 << uint(i))` — and `1 << uint(i)` is still assignable to the
new flag type, so it keeps compiling and asks about bit *i+1*'s worth of
value. `shopstate.go`'s `matchesShopWord` had exactly this; the fix is
`Has(ExtraFlag(i))`, because when the table and the domain are both
indexed by bit position the index *is* the flag. A shop that would not
buy a glowing sword is what caught it.

Both shapes share a cause with the OR trap — an integer expression that
was arithmetic on masks and is now a value in a small enum — and neither
is detectable by the same scan, because neither involves two constants of
the domain. **When converting a domain, grep it for `0` and for `1 <<`
as well as for the variadic methods.**

### 4.2 Enumerations get types, `String`, and a text marshaller

```go
type Class int
type ItemType int
type Apply int
type SpellID int
```

with `String()` for logs and `stat`, and `MarshalText`/`UnmarshalText`
where the yaml layer already writes a name. The `[]string` tables in
`bitnames.go` stay tables — they are what the C-reparse tests compare —
but become indexed by the type.

`RemortVector int32` (`player.go:172`) becomes `Set[Class]`, which is what
it has always been.

### 4.3 Object values become the taxonomy that already exists on disk

Lift `internal/persist/world/yaml/values.go`'s types into `internal/game`
(the dependency runs the right way: `game` imports nothing of ours but
`colour` and `rng`), and give `Object` a typed accessor per item type
alongside a raw escape hatch:

```go
func (o *Object) Weapon() (WeaponValues, bool)
func (o *Object) Container() (ContainerValues, bool)
...
func (o *Object) RawValues() [4]int32   // still the truth; still what saves
```

**Mirror the format's own rule rather than inventing a second taxonomy.**
`values.go` types five of the game's twenty-three item types and
deliberately leaves the rest raw; it emits the typed form only when every
slot the type does not use is genuinely zero, because a corpse is a
container whose fourth value is `-1` and rounding that to zero would lose
it. A game-side
model that typed more types, or typed them differently, would be a second
authority disagreeing with the first — which is the `bitnames.go`/
`yamlnames.go` duplication (§3.5) repeated in a place where it would cost
data rather than confusion.

### 4.4 Absence

`(T, bool)` for lookups, `*T` for optional fields on structs that are
rarely populated, and defined types with a `Valid()` where the sentinel
has to survive to disk. The vnum sentinels (`NoRoom`, `NoObject`) are
mostly the *third* case: they are written to files. What can go is their
use as in-memory "absent" — an exit's `ToRoom` of `NoRoom` should be a
`nil` exit, and `Object.WornAt = -1` should be the location union not
carrying a slot at all (§4.5).

### 4.5 The object location becomes one value

```go
type Placement interface{ placement() }

type InRoom     struct{ Room RoomVnum }
type CarriedBy  struct{ Holder *Character }
type WornBy     struct{ Holder *Character; At WearPosition }
type InContainer struct{ Container *Object }
```

One field on `Object`, `nil` for nowhere. Five fields collapse to one,
the "exactly one is meaningful" invariant becomes unrepresentable
otherwise, and the `Put`/`Take` functions go from being the convention
that maintains it to being the only way to write it.

### 4.6 Split `PlayerRecord`

A saved-state struct that the persistence layer owns, and an entity the
rules operate on that holds it. `Worn` and `Mobile` move to the entity,
where they already belong; `Character` stops being "the runtime bits that
did not fit in the record".

This is the largest and least mechanical step, and it is the one most
likely to be deferred or dropped. §7 puts it late for that reason.

### 4.7 Width

`int` in the rules layer; explicit widths only at the persistence
boundary, where they are facts about a format. Read §2.2's third hazard
before starting, and §10's fourth open question before finishing.

---

## 5. The safety net, and the rule that protects it

This project's central practice — `CLAUDE.md`, "Do not read the C and
transcribe it" — produced three kinds of machinery, and **all three are
load-bearing for this change specifically**:

- **Ten C oracles** in `reference/tools/`: the RNG over 30,000 draws,
  to-hit over 1,512,000 values, regeneration over 36,288, saving throws,
  DES crypt, shop prices (at 32-bit x87 precision), `isname`/`get_number`,
  the line editor, aliases, mail and character creation.
- **15 test files that re-parse the C source** and compare table entry by
  entry: `class.c`, `constants.c`, `interpreter.c`, `spec_assign.c`,
  `handler.c`, `spells.h`, `act.wizard.c`.
- **Four layout tools** in the same directory, which pin the binary codecs
  to gcc's own struct offsets under both data models.

Plus 1,672 test functions, the play suite (`test/play`) and the session
parity suite (`test/parity`).

**The rule: a step in this plan may not delete, weaken, or route around
any of them.** This is the single biggest risk in the document and it is
a *seductive* risk rather than an obvious one, because every one of those
tests looks like exactly the sort of thing an idiomatic-Go pass should
tidy away. `applyTypeNames []string` is a flat table of magic strings
indexed by an integer; the modern instinct is a `map[Apply]string` or a
`//go:generate stringer` run. Do that and `apply_test.go` stops re-parsing
`constants.c`, and the day somebody inserts a value in the wrong place
nothing says so.

If a table's shape must change, **the test changes with it in the same
commit and goes on deriving its expectation from the C.** A commit that
changes a table and simplifies its test is the failure mode this section
exists to name.

Two corollaries:

- **`reference/` is not touched by any step here.** The C tree, the
  oracles and the layout tools stay exactly as they are.
- **Coverage is a precondition, not a deliverable.** Where a step touches
  something with no oracle and no re-parse test, write the test *first*,
  against the current behaviour, and let it be the thing that proves the
  refactor was a no-op.

---

## 6. What proves a step landed

Every step in §7 answers all of these, and the first three are the ones
that make the "no gameplay change" claim in §2.2 real rather than
asserted. Note that they do *not* all run at the same cadence: the point
of the split below is that an expensive check asked for too often is an
expensive check that gets skipped.

1. **`make play` is green, on every pull request.** It is the only thing
   in the tree that boots the real server off disk — reading the world,
   resetting zones, attaching specials, parsing a flag — and it found six
   bugs before it was finished being written. It is cheap enough to pay
   per PR, and it is the check most likely to catch a step that broke
   boot rather than arithmetic.
2. **`make parity` is green, once per *domain* rather than once per pull
   request.** The session parity suite builds a C tree, and steps 1 and 2
   are something like twenty PRs between them; asking for it on every one
   is asking for it to be skipped. Run it when a flag domain or an enum
   domain is finished — a boundary where a transcript difference is still
   easy to bisect, and where there is one anyway.

   Both are release-only suites, and running them directly and locally is
   exactly the carve-out `CLAUDE.md` describes for `make ci` and `-race`:
   reach for the broader check when the change genuinely needs it. **Do
   not add either to `go.yml`.**
3. **The example worlds are byte-identical.** `dlctl import` over
   `examples/stock/binary` and `examples/mini/binary` reproduces the
   checked-in yaml exactly, and `examples/torture` imports unchanged.
4. **`make lint` at zero, `make test-fast` green.** As always.
5. **`go test -race ./internal/server/` where the step touches anything
   the world goroutine owns** — most of them do.

One new check is worth building, at step 0, for the reason §2.1 gives:
every existing format check runs binary → yaml, and the path a
memory-model change actually alters is yaml → memory → yaml. **A
load-and-resave fixture.** Load each of `examples/stock/yaml`,
`examples/mini/yaml` and `examples/torture/yaml` through the server's own
readers, write them straight back out, and diff. It sits beside
`cmd/dlctl/import_test.go`'s fixture table, which already names all three
directories; it runs in `go test ./...`; and it turns "the format did not
move" from a review question into a red build.

---

## 7. The steps

Each row is one or more pull requests. The order is chosen so that the
compiler does as much of the work as possible, as early as possible, and
so that the riskiest step is last and separable.

| | Step | What it is | Risk |
| --- | --- | --- | --- |
| **0** | ~~**The resave fixture**~~ **Done.** | §6's load-and-resave test, over all three corpora and all seven subsystems: `cmd/dlctl`'s `TestFmtLeavesTheCheckedInCorporaAlone`. It failed the first time it ran — `dlctl fmt --type=state` was the only caller in the tree that could bring a `state/bans.yaml` into existence, so formatting a converted directory added a file the conversion had deliberately not written. Fixed in `bans/yaml`'s `Rewrite`, not in the corpus. | None. Do this first; everything after it leans on it. |
| **1** | **A type per flag domain** — **under way** | §4.1. `Flags` → `Set[RoomFlag]`, `Set[AffectFlag]`, `Set[PlayerFlag]`, … One domain per PR, eleven or so PRs. `Raw()`/`SetFromRaw` at the persistence boundary only. **Done: room flags** (with `Set[T]` itself and the raw-bits helper boundary), **exit/door flags**, **container flags**, **shop flags**, **paladin spec flags**, **spell targeting and routine flags**, **item wear flags**, **item extra flags**, **mob act flags**, **affect flags**. | Low, high volume, *and one real hazard*: §4.1.1's OR trap, which the first domain hit twice. The compiler finds every site; it does not find that one. `bitnames_test.go` must still re-parse `constants.c`. |
| **2** | **A type per enumeration** | §4.2. Class, Sex, Race, ItemType, Apply, SpellID, Sector, Liquid, and `MobDef.Position` onto the existing `game.Position`. `RemortVector` becomes `Set[Class]`. | Low, high volume. Watch the table-indexed tests (§5). |
| **3** | **Typed object values** | §4.3. Lift `values.go`'s taxonomy into `game`, typed accessors, raw kept as the stored truth. 84 positional accesses go. | Medium. The five-types-only rule is a constraint, not a starting point to improve on. |
| **4** | **Absence over sentinels** | §4.4, in the places §3.4 lists, excluding the vnum sentinels that reach disk. | Medium. Each one is a small semantic argument; do not batch them. |
| **5** | **The object placement union** | §4.5. Five fields to one. | Medium. Everything already goes through `Put`/`Take`, which is what makes it tractable. |
| **6** | **Deduplicate the name tables** | §3.5. One source of truth for a bit's C name and its yaml name, still checked against `constants.c`. | Medium, and the one most likely to eat its own test. Read §5 twice. |
| **7** | **`PlayerRecord` split** | §4.6. Saved state and rules entity become different types. | High, and not mechanical. May be deferred indefinitely; nothing else here depends on it. |
| **8** | **Width** | §4.7. `int32` → `int` in the rules layer only. Retires most of the 129 G115 suppressions. | **Highest.** §2.2's third hazard. Gated on the oracles, not the type checker. May end as "declined" — see §10. |
| **9** | **Package shape** | §3.9, and only the parts §9 does not exclude. | Low, low value. Last for a reason. |

Steps 1 and 2 are the bulk of the benefit and nearly all of the volume.
If this plan stalls after step 3, it will still have been worth doing.

---

## 8. What this breaks, and for whom

**For a player: nothing.** That is the whole content of §2.2, and §6 is
how it is proved rather than hoped.

**For an operator: nothing.** No flag changes, no file changes, no
`.dlversion` bump, no migration. A server can be upgraded and downgraded
across any of these steps.

**For anyone reading the C alongside the Go:** this is the real cost, and
it should be stated plainly rather than minimised. Through Phase 5 the two
trees had the same shapes, and that correspondence is how every one of the
57 entries in `docs/weirdnumbers.md` got found. After step 2 a reader
holding `structs.h` in one hand will no longer find `ch->player.chclass`
next to `Class int32`; they will find `Class Class` — a field whose type
is its own name — and the mapping is one hop further away.

Three things make that acceptable rather than a reason to stop. The
translation is one hop, not a redesign — bit positions, table indices and
value slots are all unchanged (§2.1, §3.10). The C-derived tests remain
the bridge, and §5 makes preserving them a hard rule rather than an
aspiration. And the correspondence has already stopped paying for itself:
the C is no longer the target, the oracles have been written, and the
findings have been found.

**For an in-flight branch:** steps 1 and 2 touch nearly every file in
`internal/game` and `internal/session`. Anything long-lived should land
first or expect to be rebased through a wall. This is a scheduling
constraint, and it is the argument for doing steps 1 and 2 quickly once
started rather than spreading them over months.

---

## 9. What is not in it

- **Anything under `reference/`.** §5.
- **`internal/persist/player/binary`.** §2.3.
- **The on-disk format.** §2.1. A yaml change is a `data-format.md`
  change and goes through `yaml-only.md` §6's rules, not this document.
- **Gameplay changes of any kind**, including obvious improvements. A
  bug found while refactoring is a GitHub issue and a separate PR, per
  `CLAUDE.md`.
- **Rewriting the concurrency model.** The single world goroutine and
  `engine.DoSync` stay exactly as they are. `go-port-plan.md` §3.1 is
  still the design.
- **A general package reorganisation.** Step 9 is deliberately narrow.
  `internal/game`, `internal/session`, `internal/server` and
  `internal/persist` are the right four seams and are not being
  relitigated; splitting a 2,000-line file inside one of them is the
  whole of the scope.
- **Error handling and `context` plumbing.** Both are already
  conventional here and neither is C-shaped.
- **Replacing the specproc seam** (`session.Special`/`SpecialCall`) or
  the scripting seam. `go-port-plan.md` §8's Trigger design is untouched.

---

## 10. Open questions

**~~One generic `Set[T]` or eleven concrete flag types?~~ Settled: one
generic `Set[T]`.** §4.1 carries the reasoning and, more usefully, what
the decision costs — it makes this the tree's first generic type, in a
codebase with one generic function and no generic types. Left here struck
through rather than deleted, because the reason it was a question is the
reason to revisit it if `Set[RoomFlag]` turns out to read badly across a
few hundred call sites.

**Does `PlayerRecord` keep its name?** If step 7 happens, the saved struct
and the rules entity both want it. Probably `player.Saved` in the
persistence layer and `game.Character` absorbing the rest — but that makes
`game.Character` large, which is the shape the split was trying to escape.

**How far does the value taxonomy go?** §4.3 says mirror the format: five
types typed, the other eighteen raw. That is right for step 3 and
probably wrong forever. Extending it means extending `data-format.md`
§4.3 *first*, on disk, and only then inward — the opposite order to
everything else here.

**Does step 8 (width) happen at all?** The honest answer is "not until
somebody has demonstrated it is safe", and the demonstration is expensive:
it means an oracle sweep over every arithmetic path `docs/weirdnumbers.md`
touches, at both widths, showing the answers agree. That is a real piece
of work and it is worth it only if the 129 suppressions are actually
costing something. They may not be. **Declining step 8 and keeping
`int32` in the rules layer is a legitimate outcome**, and if it is taken,
it belongs in `docs/deviations.md` — not because it is a deviation, but
because "we kept the C's widths on purpose, here is why" is exactly the
kind of thing that gets re-litigated from memory in two years.

**Where does the `Set` type live?** `internal/game` has no sub-packages
today and adding one for a bitset is a small architectural decision with
a long tail. In `game` itself is simpler and probably right.

---

## Related documents

- `docs/design/go-port-plan.md` — §0's "Fidelity, phase two" is what
  licenses this; §3.1 (concurrency), §4 (integer widths) and §5 (the
  pluggable seams) are the architecture it must not disturb. Its §4 is
  worth reading before step 8 in particular: this port already had one
  argument about integer width, and won it in the other direction.
- `docs/design/yaml-only.md` — the same argument one layer out. Its §5
  (the compatibility test architecture) and §6 (the rules for new fields)
  are §2.1's fence, and are still live.
- `docs/design/data-format.md` — the format this may not move. §4.3 is
  the object-value taxonomy step 3 lifts inward.
- `docs/weirdnumbers.md` — 57 reasons step 8 is last.
- `docs/deviations.md` — where nothing from this document goes, unless
  §10's last-but-one question is answered "no".
- `CLAUDE.md` — "Do not read the C and transcribe it", and the traps in
  the test suite. Both apply unchanged.
