# Idiomatic Go: a model that owes nothing to `char_data`

Status: proposed, not started. **Follows
[`yaml-only.md`](yaml-only.md)** and cannot sensibly precede it — §1 below is
the whole argument for that ordering.

`go-port-plan.md` §0's "Fidelity, phase two" (2026-08-23) is what authorises
this work: from that date, new work is free to diverge from the C server to
modernise the implementation, without recording a reason. Two things stay
fixed and are not up for modernisation — **compatibility** and **gameplay**.
This plan is the first large exercise of that clause, and §3 is the operating
definition of those two carve-outs, because "modernise the implementation"
is a licence that gets dangerous the moment nobody has written down where it
stops.

The destination: a server whose types describe *this game* rather than the
memory layout of a 1993 C struct, running off a format it assumes is
extensible. **The problem of getting an old CircleMUD `lib/` to version
x.y.z of the yaml format does not go away — it moves, permanently, into
`dlctl`**, which is exactly where the awkwardness belongs and where it is
already accumulating.

---

## 0. Principles

Five, and the rest of the document is their application.

1. **The yaml schema is the contract; the Go model is free.** These are
   already separate — `internal/persist/world/yaml/doc.go`'s `roomDoc`,
   `exitSetDoc` and friends are document types with yaml tags, not the game's
   own structs. That separation is what makes this plan a refactor rather
   than a format change, and preserving it is the first rule: **a slice that
   would alter a `.yaml` file's bytes is not a refactor and does not belong
   in this plan.** It belongs under `yaml-only.md` §6's rules for changing
   the format.

2. **`dlctl` keeps the legacy problem.** Every C-shaped decoder stays,
   tested, in a package the server does not link. As the game model gets less
   C-shaped, those decoders get *more* conversion code, not less — that is
   the cost being deliberately paid, concentrated in one place instead of
   spread across `internal/game`.

3. **No behaviour change is the acceptance bar, and the existing tests are
   how it is proved.** §4.

4. **One concept per slice, each shippable on its own.** Not a branch that
   rewrites `internal/game`. Every slice in §5 leaves the server working and
   the suite green, and any of them can be the last one done without leaving
   a mess.

5. **Compiler-checked before hand-checked.** The good refactors here are the
   ones where the type system finds every call site and a missed one will not
   build. Where a change cannot be compiler-checked, §4's rule applies: get
   the oracle in place first.

---

## 1. Why this comes after yaml-only, not before

The model is C-shaped because until now something had to decode C-shaped
files *into* it. `game.PlayerRecord` carries `char_file_u`'s padding because
`binary` writes it back. `game.Flags` is one type for every flag domain
because `asciiflag_conv`'s letter encoding is one encoding for every flag
domain. Every field in `internal/game` has an explicit width because
`internal/game/types.go`'s own package comment says so: "Every field that is
ever serialized has an explicit width."

That sentence is the hinge. **Once the only thing the server serializes is a
yaml document type, the width lives on the document type**, and the model is
free to use `int` where `int` is the right answer. The widths do not stop
mattering — they stop mattering *here*. They move to `doc.go` and to
`persist/*/classic`, where a width is a fact about a file rather than a fact
about a game.

The same argument runs through every item in §2. Doing this work first would
mean either changing the model while three legacy codecs still decode into it
— tripling the blast radius of every slice — or building an adapter layer
between the new model and the old codecs, which is a second model to keep in
step with the first. Neither is worth it when the alternative is *wait*.

One consequence worth stating up front: yaml-only is a **breaking** change
for operators and this one is not. Nothing in here alters a file on disk,
a flag, or anything a player types. If a slice appears to, that is the signal
in §0.1 firing.

---

## 2. The inventory

Numbers are from the tree as it stands. Every one of these is
*implementation*, not gameplay or compatibility — §3 draws that line.

### 2.1 One `Flags` type for every flag domain

`game.Flags` is `uint64` (`internal/game/flags.go:21`) and it is the type of
room flags, mobile action flags, mobile affection flags, object extra flags,
object wear flags, player flags, player preference flags and exit flags —
**174 constants across at least eight domains, all mutually assignable.**

```go
room.Flags.Has(ItemGlow)          // compiles
mob.ActionFlags.Has(RoomDark)     // compiles
obj.WearFlags = mob.AffectionFlags // compiles
```

Nothing in the type system objects, because in the C nothing could: a
`bitvector_t` is a `bitvector_t`. This is the largest single class of
preventable bug left in the tree and the one Go's type system removes most
cleanly.

### 2.2 `Values [4]int32` — the model knows less than the format does

`game.ObjDef.Values` and `game.Object.Values` are `[4]int32`
(`internal/game/world.go:177`, `object.go:315`), read by index in **84 places
outside tests**:

```go
dam += r.Dice(wielded.Values[1], wielded.Values[2])          // fight.go:141
capacity, filled, liquid := obj.Values[0], obj.Values[1], obj.Values[2]
return o != nil && o.Type == ItemLight && o.Values[2] != 0   // light.go:27
```

The sharp part is not that this is ugly. It is that **the persistence layer
already models this properly and the domain does not**:
`internal/persist/world/yaml/values.go` defines `WeaponValues`,
`ContainerValues`, `DrinkValues`, `LightValues`, `ChargesValues` and
`ArmorValues`, because `data-format.md` §4.3 decided object values should be
typed on disk. The knowledge exists in this repo. It is in the wrong package.

### 2.3 Bare `int32` as an identifier, across nine unrelated namespaces

Roughly 180 constants, none of which have a type of their own:

| Concept | Constants | Example |
|---|---|---|
| Spell numbers | ~70 | `SpellMagicMissile int32 = 32` |
| Apply locations | ~25 | `ApplySaveParalyse int32 = 20` |
| Item types | ~23 | |
| Liquid types | ~16 | |
| Attack types | ~15 | |
| Skill numbers | ~9 | (same numeric space as spells, deliberately) |
| Classes | ~6 | `ClassMagicUser int32 = 0` |
| Sexes | 3 | |
| Sectors | 2 | |

So the tables that key off them say very little:

```go
var savingThrowTable = map[int32][NumSaveTypes][]int32{}  // class → save type → level → value
var thacoTable       = map[int32][]int32{}                // class → level → thac0
var levelExperience  = map[int32][]int32{}                // class → level → exp
Skills               map[int32]int32                      // skill number → learned percent
```

Four different meanings of `int32` in the last line alone, and a compiler
that will happily let you look a spell up by class.

**This is the half-finished part, and that is the encouraging bit.** The port
already did exactly this work for the concepts it got to: `RoomVnum`,
`MobVnum`, `ObjVnum`, `ZoneVnum` and their rnum counterparts are distinct
types precisely so "the compiler enforces what the C code only documents"
(`types.go:30-33`); `Position`, `Direction` and `SaveType` are real types
with methods. The pattern is established, proven and stopped partway through.
This is finishing it, not introducing it.

### 2.4 Parallel name tables, position-locked by test

`internal/game/bitnames.go` holds `[]string` tables — `affectBitNames`,
`roomBitNames`, `actionBitNames`, `preferenceBitNames`, `wearBitNames`,
`applyTypeNames`, `sectorNames`, `positionNames`, and more — indexed by bit
position, ported from `constants.c`. `internal/game/yamlnames.go` holds a
second, parallel set for the yaml vocabulary: `yamlRoomFlagNames`,
`yamlSectorNames`, `yamlPositionNames` and the rest.

The two must stay index-for-index aligned, and today that is guaranteed by
`yamlnames_test.go` asserting it, plus a `""` convention for slots with no
name in either. That test is good work and it is compensating for a data
structure: two slices that must agree positionally are one table wearing a
disguise. A single table of records per concept — bit, C display name, yaml
name — cannot drift, and needs no test to say so.

### 2.5 `int32` in the model, for a reason that expires

Per `types.go`'s package comment, widths are explicit because the values get
serialized. After yaml-only they get serialized *by the document types*.
`Character.Position` is already `Position`; but levels, gold, hit points,
experience and skill percentages are `int32` in memory because a 2001 struct
said so. Some should stay sized (anything the game's own arithmetic
overflows deliberately — see `weirdnumbers.md`, and §3). Most should be
`int`.

This is the slice with the widest diff and the least intellectual content,
and §5 places it accordingly: last, once everything above it has settled.

### 2.6 The smaller ones

- **Ability tables indexed by raw score.** `conApplyHitPoints [26]int32`,
  `strApply [31]StrengthApply`, `dexApply [26]DexterityApply`,
  `intApplyLearn [26]int32` (`apply.go`). The lengths are the C's array
  bounds; the interesting range starts partway in. The *values* are gameplay
  and untouchable; the *indexing* is a C array bound.
- **`SavingThrows [5]int32`** (`player.go:80,119`) with a five-arm `switch`
  in `affect.go:256-265` mapping apply locations onto indices, when
  `SaveType` already exists as a type.
- **`damageTiers [10]damageTier`** selected by a ten-arm if-chain
  (`damage_messages.go:125-141`) over a sorted table that a loop or
  `sort.Search` reads in one line.
- **`fighting map[*Character]bool`** (`live.go:58`) — a set spelled as a map
  to `bool`, so `if fighting[c]` and `if fighting[c] == true` are both
  reachable and a `false` value is representable and meaningless.
- **Generics used once in the entire tree** (`persist/world/dump.go:356`), on
  Go 1.25. Not a goal in itself — but §6.1's flag design wants them, and
  their absence is a fair signal of how much of this code was written by
  reading C.

---

## 3. What must not be touched, and how to tell

"Fidelity, phase two" fixes compatibility and gameplay. Applied to this work,
that resolves into a short list which is worth memorising, because everything
on it *looks* exactly like the things in §2.

**Gameplay — do not touch, at any altitude:**

- **`Command.CLine` and the command table's ordering.**
  (`internal/session/commands.go:35-45`, 213 entries.) The table is sorted by
  its line number in `interpreter.c` because the interpreter matches the
  first entry a typed word prefixes, so the order *is* what every
  abbreviation means. It looks like the most C-ish thing in the tree. It is
  load-bearing muscle memory and it is derived-not-asserted on purpose. It
  stays exactly as it is.
- **Argument splitting.** `oneArgument`, `anyOneArg`, `isname`, `get_number`.
  How a typed line divides into words is player-visible, and `isname` in
  particular has already been read wrong once, for four phases
  (`CLAUDE.md`). Retyping their *signatures* is fine; changing what they
  return for any input is not.
- **Every number in `weirdnumbers.md`**, including the truncations and the
  overflows. If a width change in §2.5 alters an arithmetic result, the width
  change is wrong. That is not a theoretical risk: several of those findings
  *are* width behaviour.
- **The compiled damage-message tiers, to-hit tables, saving throws,
  experience curves and ability tables.** The containers may change; not one
  value may.

**Compatibility — do not touch:**

- **Flag bit positions and their yaml names.** §6.1 gives every flag domain
  its own type. It does not renumber a single bit, and it does not rename a
  single yaml identifier — those are the on-disk contract.
- **`CFlagLimit` and `ExceedsCRange`** (`flags.go:24,47`). These encode a
  fact about `asciiflag_conv`: the C server breaks above bit 31. They do not
  die, they **move** — to `persist/world/classic` and the linter, which is
  the only place that fact is still relevant once the server does not read
  that format.
- **The C oracles and the layout tools.** `reference/tools/`'s seventeen
  programs are how the numbers above are known to be right. Nothing in this
  plan touches them, and slices that change arithmetic types run them.

**The test:** if you cannot change it without a player noticing, or without a
converted file differing, it is not implementation. When unsure, the cheap
answer is to check whether a `test/play` scenario or a `testdata/parity`
script covers it — and if none does, that is itself the finding, and the
scenario gets written before the refactor.

---

## 4. How a refactor this size is proved safe

The reason this is attemptable at all is that the net is already under it:

- **1,513 test functions** across `internal/`, `cmd/` and `test/`.
- **74 end-to-end scenarios** in `test/play`, driving a real server over a
  real socket against `examples/mini`.
- **13 session-parity scripts** (`testdata/parity/`) playing the same input
  at the C server and this one and diffing what they say.
- **17 C oracle programs** (`reference/tools/`) pinning the RNG, to-hit,
  regeneration, saving throws, DES crypt, shop prices, `isname`/`get_number`
  and the struct layouts.
- **The world-parity harness**, C loader against Go loader, zero differing
  fields across 3,202 records.

That net was built to prove a *port* correct. It turns out to be exactly the
net a *refactor* needs, which is a genuine dividend of how this project was
built and worth saying plainly.

Three rules follow, and they are what keep this from becoming a rewrite:

**A refactor slice adds no new assertions about behaviour.** It may add tests
that pin the new *types* (that `RoomFlags` and `MobFlags` are not
interchangeable is a compile-time fact, so the test is that the tree builds).
It does not get to add a test asserting what the game does — because if the
existing suite did not already assert that, the slice has just changed
untested behaviour and nobody can tell.

**If an existing test has to change to accept a slice, stop and look.** Some
will legitimately change: a test that constructs a `game.Flags` literal will
name a type instead. That is a signature change and it is fine. A test whose
*expected value* changes is a behaviour change wearing a refactor's clothes,
and it goes back for a reason in `docs/deviations.md` or does not land.

**Arithmetic slices run the oracles first, not last.** §2.5 in particular:
before any width changes, the relevant oracle runs and is green, so a
difference afterwards has one candidate cause. `make parity` and
`make ci-job JOB=full-suite CI_WORKFLOW=.github/workflows/release.yml` are
the local commands; per `CLAUDE.md` they do not get added to `go.yml`.

---

## 5. The slices

Ordered by leverage over risk. Each is a branch and a PR; each leaves the
tree green.

| # | Slice | What it does | Done when |
|---|---|---|---|
| **1** | **Typed flag sets** (§2.1, §6.1) | One type per flag domain, 174 constants redistributed. No bit renumbered, no yaml name changed. | The tree builds, which is the proof: `room.Flags.Has(ItemGlow)` no longer compiles. Suite green with no expected value changed. |
| **2** | **Merge the two name vocabularies** (§2.4) | One record table per concept — bit, C display name, yaml name — replacing the position-locked slice pairs in `bitnames.go`/`yamlnames.go`. `yamlnames_test.go`'s alignment assertion is deleted because it becomes unrepresentable. | `stat`, `show` and the yaml writer all produce byte-identical output to today, checked against the real corpus rather than by inspection. |
| **3** | **Identifier types** (§2.3, §6.2) | `SpellID`, `ClassID`, `ApplyLocation`, `ItemType`, `LiquidType`, `AttackType`. Skills keep sharing spells' numeric space, because `spellTable` is deliberately one table for both — the type is `SpellID` and that is honest. The keyed tables in §2.3 get readable signatures. | The tree builds; `savingThrowTable` reads `map[ClassID][NumSaveTypes][]int32` and looking a spell up by class does not compile. |
| **4** | **Typed object values** (§2.2, §6.3) | Move `WeaponValues`/`ContainerValues`/… from `persist/world/yaml` into `game`; add typed accessors; convert the 84 `Values[N]` sites. Storage stays `[4]int32` for now — see §8. | No `Values[` index literal remains outside `game/object.go` and the codecs. `test/play`'s combat and container scenarios unchanged and green. |
| **5** | **The small structural ones** (§2.6) | `SavingThrows` keyed by `SaveType`; `damageTiers` selected by search rather than an if-chain; `fighting` as a set; ability tables given named accessors that state their offset instead of open-coding it. | Suite green, no expected value changed. `make parity` clean. |
| **6** | **Retire `LegacySpares` from `internal/game`** | Already scheduled as row 4 of `yaml-only.md`; listed here because this is the plan it thematically belongs to and it must not get done twice. | Whichever plan's row runs first; the other's is struck. |
| **7** | **Widths** (§2.5) | `int32` → `int` in the model wherever the width was serialization-driven and the arithmetic does not depend on it. Oracles first, per §4. | Every oracle green before and after, with the before-run recorded in the PR. Anything whose overflow is in `weirdnumbers.md` keeps its width, and gains a comment saying which finding requires it. |

Slices 1–3 are the ones worth doing whatever happens to the rest: they are
where the type system starts paying, they are almost entirely
compiler-checked, and none of them can change a number.

---

## 6. What "idiomatic" means concretely

Three designs, because "make it idiomatic" without a shape is how a refactor
becomes an argument.

### 6.1 Flags: one generic set type, one bit type per domain

```go
// Bit is any flag domain's bit-index type.
type Bit interface{ ~uint8 }

// Set is a bitfield over one domain. Set[RoomFlag] and Set[MobFlag] are
// different types, so the compiler stops what flags.go's single Flags
// type has always allowed.
type Set[B Bit] uint64

func (s Set[B]) Has(b B) bool      { return s&(1<<b) != 0 }
func (s Set[B]) Set(b B) Set[B]    { return s | 1<<b }
func (s Set[B]) Clear(b B) Set[B]  { return s &^ (1 << b) }

type RoomFlag uint8
const (
    RoomDark RoomFlag = iota
    RoomDeath
    // ...
)

type RoomFlags = Set[RoomFlag]
```

Two things this buys beyond the obvious. The bit *index* becomes the
constant, rather than `1 << n` written out 174 times — which is what makes
§2.4's merged table indexable by the constant itself. And each domain's names
hang off its own bit type as a method, so `Set.Names()` works generically
without a lookup table passed in at every call site.

The `uint64` storage is unchanged, so nothing on disk moves.

That snippet was compiled rather than sketched, on this tree's Go 1.25:
`Set[RoomFlag]` and `Set[MobFlag]` are distinct types, and passing a
`MobFlag` to a `Set[RoomFlag]`'s method is
`cannot use MobSentinel (constant 0 of type MobFlag) as RoomFlag value`.
Worth checking before proposing, because "the compiler will catch it" is the
entire argument for slice 1 and a generic shift count is exactly the kind of
thing that is legal in one's head and not in the language.

### 6.2 Identifiers: named types, and the honesty about skills

```go
type SpellID   int32   // spells and skills share one numeric space (spellTable)
type ClassID   int32
type ItemType  int32
type AttackType int32
```

`SpellID` covering skills is not a compromise, it is the C's actual design
and `spell.go` already documents it ("a spell and a skill are both just a
`spellTable` number underneath"). Giving them one type says that; giving them
two would be a lie the code would then have to work around.

The win is in the tables:

```go
var savingThrowTable map[ClassID][NumSaveTypes][]int32
var thacoTable       map[ClassID][]int32
Skills               map[SpellID]Percent
```

### 6.3 Object values: typed views over unchanged storage

```go
type WeaponValues struct{ NumDice, SizeDice int32; AttackType AttackType }

// Weapon reports the object's weapon values, and whether it is one.
func (o *Object) Weapon() (WeaponValues, bool)
func (o *Object) Light()  (LightValues, bool)
func (o *Object) Drink()  (DrinkValues, bool)
```

so `fight.go:141` becomes

```go
if w, ok := wielded.Weapon(); ok {
    dam += r.Dice(w.NumDice, w.SizeDice)
}
```

Storage stays `[4]int32` in this plan. Replacing it with a discriminated
union is the obviously "more idiomatic" move and it is deliberately **not**
proposed: Go has no sum types, so the honest encodings are an interface
(which puts an allocation and a type switch on the hot path of every object
in the world) or a struct with a type tag and six mutually exclusive fields
(which is `[4]int32` with more names and worse memory). The accessors get all
of the readability and all of the type safety at the call sites — which is
where the 84 bugs would be — for none of that. Revisit only with a concrete
complaint the accessors do not answer.

---

## 7. What this does to the yaml format

Nothing, by construction, and that is the invariant to defend.

Every slice in §5 stops at the document types. When slice 1 gives room flags
their own type, `roomDoc.Flags` is still `[]string` and the yaml still says
`flags: [dark, indoors]`. When slice 4 introduces `WeaponValues` in `game`,
`values.go` stops defining its own copy and uses that one — the marshalling
is unchanged. Slice 7 changes widths in memory; `doc.go` keeps the widths the
format specifies.

Two practical consequences:

- **`dataversion` does not move.** No slice bumps the format version, because
  no slice changes a document. A PR in this plan that touches `.dlversion`
  or a `schema:` tag has gone wrong.
- **`examples/*/yaml` must come out byte-identical.** The existing
  regeneration check (`cmd/dlctl/import_test.go`) is therefore this plan's
  cheapest and strongest guard rail, and every slice runs it. It is not
  currently in day-to-day CI — it is a release check — so slices run it
  locally, per `CLAUDE.md`'s scope rule, rather than being added to `go.yml`.

The reverse direction is where the extensibility this is all for actually
shows up. Once the model is not a mirror of a struct, adding a game concept
is a Go change plus a document-type change plus a default (`yaml-only.md`
§6), and the C tree does not get consulted about it at all. That is the
point of the exercise, and it is worth being explicit that it does not arrive
until the model stops being a mirror — which is slices 1 through 4.

---

## 8. What is not in it

- **No sum type for object values.** §6.3.
- **No storage change for object values.** The `[4]int32` stays; only the
  call sites change. Flipping storage is visible to both codecs and is a
  separate proposal if it is ever worth one.
- **No change to `Command.CLine`, argument splitting, or any table's
  values.** §3.
- **No renumbering, renaming, or reorganising of anything on disk.** §7.
- **No `Character`/`Live` restructuring.** They are already better than
  `char_data`: `Character` is 20-odd fields with an interface for the client,
  `Live` is typed maps behind a single goroutine. There is a real
  conversation to have about `session.Context`'s width — it carries a dozen
  injected dependencies — but it is a Go design question, not a C-ism, and
  bundling it here would blur the line §3 draws.
- **No new dependencies.** Everything above is stdlib and language features.
- **Not a performance exercise.** Some of this will be marginally faster and
  none of it is being done for that. If a slice makes something slower in a
  way that matters, that is a finding, not a goal.

---

## 9. Open questions

**Does `game.Flags` survive at all?** §6.1 replaces its uses, but
`ParseFlags`'s letter decoding still has to exist somewhere for `classic`
to use. Probably: the parser moves to `persist/world/classic`, generic over
the destination bit type, and `game.Flags` disappears. Worth confirming
before slice 1 rather than discovering during it.

**How much does slice 7 actually change?** The claim "most widths are
serialization-driven" is an assertion in this document, not a measurement.
Before that slice starts, someone should count how many `int32` fields in
`internal/game` are load-bearing for arithmetic in `weirdnumbers.md`. If it
is a large fraction, the slice shrinks to a comment-writing exercise, which
would be a perfectly good outcome — the widths would then be *documented* as
deliberate rather than inherited, which is most of the value.

**Should the flag bit type be `uint8`?** §6.1 uses it, which caps a domain at
256 bits and makes the constants small. `uint64` storage caps it at 64
anyway. Fine today; the question is whether any domain is near 64 and wants
a `[]uint64` or a map-backed set instead. Item extra flags are at 33 of 64,
which is the closest and not close.

**Does `test/play` need more scenarios before slice 4?** §3's test says a
thing not covered by a scenario or a parity script should get one written
first. Object values reach combat, containers, drinks, lights and wands;
`test/play` covers combat and containers well and the others less so. Sizing
that gap is slice 4's first task, not its last.

---

## Related documents

- `docs/proposals/yaml-only.md` — the plan this follows, and the reason it
  can be attempted. Its row 4 and this plan's slice 6 are the same work; §5
  says so.
- `docs/proposals/go-port-plan.md` — the port that built all of this. §0's
  "Fidelity, phase two" is this plan's mandate and §3 is its boundary; §4
  (64-bit safety) is why the widths in §2.5 exist in the first place.
- `docs/design/data-format.md` — the yaml format, which §7 promises not to
  touch. Its §4.3 is where object values were typed on disk before they were
  typed in memory.
- `docs/deviations.md` — where a slice goes if it turns out to change
  behaviour after all. §4's second rule is the trigger.
- `docs/weirdnumbers.md` — read before slice 7. Several of its findings are
  width behaviour, and they are the reason that slice is last.
