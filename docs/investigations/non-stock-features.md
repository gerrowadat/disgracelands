# Non-stock features: what Disgracelands actually changed

An enumeration of everything in the two source trees
(`reference/CircleMUD3-src` — "the regular mud", the one actually played
2001–2008 — and `reference/WipeMud-src` — the abandoned May 2003 upgrade
attempt) that isn't in stock CircleMUD, and which of the two trees each
feature is actually in. See `circlemud-archive-report.md` for how these two
trees relate, and `history.md` for the timeline.

## Method

Two independent checks, cross-referenced:

1. **File-level diff against real stock CircleMUD.** `tarfiles/circle30bpl19.tar`
   in the archive is an untouched upstream 3.0 bpl19 tarball; a stock 3.1
   mirror ([Julio-Rats/CircleMUD](https://github.com/Julio-Rats/CircleMUD))
   was used for the few things only present from 3.1 on. Comparing file
   lists finds whole files that don't exist upstream at all.
2. **The `<DoC>`/`</DoC>` comment convention.** Both trees mark local
   changes to otherwise-stock files this way (see
   `circlemud-archive-report.md` §7). Every tagged block in both
   trees was read and characterized for this document — not just counted.

**Caveat**: `<DoC>` tags are not a complete inventory. Some custom content
sits in files that are *new* rather than modified (no tag needed - the
whole file is the change), and a few individual lines of custom content
turned up sitting untagged next to tagged ones (e.g. the `ouchie` and
`immolate` spell registrations in `spell_parser.c` have no tags of their
own, immediately next to ones that do). Treat this as thorough, not
exhaustive.

---

## Third-party add-on packages (not bespoke, but still non-stock)

Both trees are built on top of public CircleMUD community patches, not
hand-rolled from scratch:

| Package | In CircleMUD3? | In WipeMud? | What it is |
|---|---|---|---|
| **OasisOLC** | ✅ | ✅ | Online building/editing (`redit`/`oedit`/`medit`/`zedit`/`sedit`/`tedit`, the `gen*.c` generic-editor framework) — build the world in-game instead of hand-editing flat files. |
| **DG Scripts** | ❌ | ✅ | A full scripting/trigger engine for mobs/objects/rooms (`dg_*.c`, 10 files). Public patch, versioned internally as "DG Scripts 0.99 pl9, 07/02", authored by George Greer (`egreen`) — one of the actual CircleMUD core maintainers, not a random third party. **Not present in the tree that was actually played.** |
| **Context-sensitive help for OLC** | ❌ | ✅ | `context_help.c`, credited in its own header comment to "Welcor" (a known CircleMUD-community patch author, not local). |
| **ascii_pfiles 2.1** | ❌ (native binary `struct char_file_u` dump) | ✅ | Converts player saves from a raw binary struct to one human-readable text file per player. See `pfile-conversion.md`. |

One correction to an earlier draft of the archive report: **CircleMUD3
does not have DG Scripts.** No `dg_*.c` file exists in it, nothing in it
calls a DG Scripts function, and its `Makefile.in` never references a
`dg_*.o`. DG Scripts is WipeMud-only.

One false lead ruled out: `autoeq_conv.c`/`autoeq_struct.h` (present only
in `CircleMUD3-src`) look at a glance like a local feature, but they're
just a **stock, disabled** CircleMUD utility — `structs.h` has
`#define USE_AUTOEQ 0` with a comment straight from upstream ("We need
this feature for CircleMUD 3.0 to be complete but we refuse to break
binary file compatibility"), and the conversion tool isn't even wired into
the Makefile. Not a Disgracelands feature; not evaluated further.

---

## Bespoke Disgracelands features — in the regular mud (CircleMUD3)

Everything below was actually live for years, in the tree with the real
2001–2008 population.

### The remort (multiclass) system

The headline feature. An implementor-run `remort` command
(`act.wizard.c`) grants a character access to a second (third, fourth...)
class's skills and spells on top of whatever they already have, tracked
as a bitmask (`remort_vector`, one bit per class) rather than replacing
their class outright. This touches a lot of the codebase:

- `IS_MAGIC_USER`/`IS_CLERIC`/`IS_THIEF`/`IS_WARRIOR` (`utils.h`) check
  the remort bitmask, not just the character's current class — so a
  remorted character is simultaneously "a thief" and "a warrior" for
  every purpose those macros gate.
- Spellcasting (`spell_parser.c`, `do_cast`) checks whether *any* of a
  character's remorted classes qualifies them to cast a given spell at
  their level, not just their current class.
- The skill/spell list (`spec_procs.c`, `list_skills`) shows spells/
  skills from every remorted class, not just the current one.
- Guild access (`spec_procs.c`, `guild_guard`) lets a remorted character
  into any guild for a class they've completed.
- Equipment class-restrictions (`class.c`, `invalid_class`) were
  deliberately narrowed to check *only* the character's current class
  (not the full remort history) — the one place remort access is
  intentionally *not* all-encompassing.
- The `who` list (`act.informative.c`) color-codes names by remort count
  (white → dark green → bright green → dark yellow → bright yellow for
  0–4+ prior classes), immortals in blue.
- New characters get their starting class stamped into their remort
  vector on creation (`interpreter.c`), so the system has a sane baseline
  from character #1.

### The Paladin class and its alignment mechanic

Stock CircleMUD ships 4 classes (Magic User, Cleric, Thief, Warrior).
Disgracelands added a 5th: **Paladin** (`class.c`, `structs.h`
`NUM_CLASSES` bumped to 5), with real mechanical weight behind it, not
just a name:

- Two new player-state flags, `PSF_UNWORTHY` and `PSF_FALLEN`
  (`structs.h`), tracked continuously against the character's alignment
  every time they try to cast a spell (`spell_parser.c`, `do_cast`):
  - Alignment drops below **-350**: permanently **Fallen** (until an
    implementor manually redeems them) — broadcast server-wide
    ("Rejoice! The sinner X has been cast out!"), spells refused from
    then on.
  - Alignment between -350 and 0 while flagged: **Unworthy**, spells
    temporarily refused ("You must repent your sins...").
  - Alignment climbs back above 600: automatically un-Unworthy'd
    ("Welcome back, friend!").
  - `who` output shows `(UNWORTHY)`/`(FALLEN)` next to a Paladin's name.
- `wizutil redeem` (`act.wizard.c`, `SCMD_REDEEM`, Greater-God+) manually
  clears the Fallen flag on a player.
- Three Paladin-flavored spells (`spells.h`/`magic.c`): **Holy Smite**
  (hitroll/damroll buff), **Holy Shield** (defensive buff), plus general
  new spells usable more broadly: **Dispel Magic** (strips all affects
  from the target — also the mechanism behind the `dispeller` mob special
  below), **Full Heal**, **Silence** (blocks spellcasting entirely while
  it lasts), **Ouchie** and **Immolate** (damage spells, untagged but
  clearly custom by name).

### Room flags and environment

Three new room flags (`structs.h`, `constants.c`):

- `GOOD_REGEN` — doubles HP/mana/move regeneration per game hour while
  standing in the room (`limits.c`); entering one prints "You feel a
  soft, warm feeling in your bones."
- `PKILL` — marks a room as player-killing-enabled; entering one warns
  "You have entered a `[Player Killer]` room. Beware!"
- `CAN_QUIT` — (flag exists; the specific `quit`-blocking check tied to
  it wasn't traced further than the flag definition itself for this pass)

### Balance / anti-exploit tweaks

- **Grouping**: a party member more than 7 levels below the group leader
  can't be kept in the group ("$N cannot hope to keep up with you!") —
  blocks power-leveling low characters via high-level groups.
- **XP-per-kill cap**: a single kill can't grant more than 10% of the XP
  needed for the character's next level ("You can only understand so
  much...").
- **No XP from PvP**: `gain_exp`'s solo/group-kill paths only award
  experience when at least one side of the fight is an NPC — killing
  another player nets no XP. PvP kills get a `brag()` taunt broadcast
  instead of a reward (mobs brag when they kill a player, too), and get
  their own distinguishable `PKILL:` prefix in the admin kill log
  (`fight.c`), separate from ordinary NPC-kill log lines.
- **Fleeing from a PvP fight** doesn't cost the fleeing player XP the way
  fleeing from an NPC fight does (`act.offensive.c`).
- **Item level requirements**: wearing an item requires
  `GET_LEVEL(ch) >= GET_OBJ_LEVEL(obj)` (`act.item.c`) — not a stock
  CircleMUD check.
- **Inventory cap on quit/rent**: `MAX_RENT` (28, `structs.h`) counted
  recursively through worn and carried containers; over the cap and
  you can't `quit` until you drop something (mortals only —
  immortals are exempt) (`act.other.c`).
- **`summon`** can only target other players, not NPCs (`spells.c`) —
  stock CircleMUD's summon spell works on both.
- **Mobs recover 5% of their max HP on landing a killing blow**
  (`fight.c`) — a combat-pacing tweak, not present upstream.

### Server-wide "voice whispers" announcements

A recurring pattern (`comm.c`'s `send_to_all_color`, new infrastructure
itself): the whole server gets told, in color, whenever —

- a new character is created ("A voice whispers in your ear, 'All hail
  X, a newcomer!'", `interpreter.c`),
- anyone gains a level (`limits.c`, both the normal and the
  admin-override `gain_exp_regardless` paths),
- anyone dies to a death trap (`utils.c`, `log_death_trap`),
- a Paladin falls from grace (`spell_parser.c`).

### Admin / moderation tooling

- Connection host strings are stored as `hostname:port`
  (`comm.c`), not just hostname — matches exactly what's in the real
  player data (e.g. `minerva.redbrick.dcu.ie:35240`) and lines up with
  the `multiwatch`/`check_multis.pl` scripts found in the archive for
  spotting multiple characters played from the same shared shell account.
- `PLR_BANNED` (`structs.h`) — a per-player ban flag distinct from
  site-level banning.
- New-character creation explicitly permits sites that are only
  `BAN_SELECT`-banned to still register (only `BAN_NEW`/`BAN_ALL` block
  it) (`interpreter.c`).

### Custom mob special procedures (`spec_assign.c`/`spec_procs.c`)

- `dispeller` — an NPC that casts Dispel Magic on whoever it's fighting
  mid-combat (comment in the source: "Annoying cunt monster that dispels
  people").
- `teleporter` — declared alongside it; not traced further than the
  declaration for this pass.

### New-character defaults

New players are created with color, HP/mana/move display, and
`autoexit` **on** by default (`interpreter.c`) — stock CircleMUD starts
these off, requiring the player to discover and toggle them manually.

---

## WipeMud-only additions

Features that exist in the abandoned 3.1 branch and **never made it into
the tree that was actually played**:

### Race system (`races.c`, new file)

Four playable races — Human, Elf, Gnome, Dwarf — chosen at character
creation, each with a small stat trade-off:

| Race | Modifier |
|---|---|
| Human | none |
| Elf | +1 Dex, -1 Con |
| Gnome | +1 Int, -1 Wis |
| Dwarf | +1 Con, -1 Cha |

Plus race-appropriate height/weight ranges rolled at creation, and
race-restricted equipment (`ITEM_ANTI_HUMAN`/`_ELF`/`_DWARF`/`_GNOME`
object flags, checked the same way the stock class-restriction flags
are). `bin2ascii.c`'s converter already stubs a `Race: 0` field in its
output in anticipation of this (§ `pfile-conversion.md`).

### DG Scripts, context-sensitive OLC help, ascii pfiles

Covered in the add-on table above — genuinely new capability relative to
CircleMUD3, but public patches integrated wholesale, not bespoke
Disgracelands design.

---

## What CircleMUD3 has that WipeMud lost

This is the part worth flagging clearly if `WipeMud` (or anything derived
from it) is ever used as a baseline again: the May 2003 upgrade appears to
have branched from an **earlier, less-developed snapshot** of
Disgracelands' own code, and several of the features above simply never
made the jump — this isn't a matter of missing `<DoC>` comments, the
functionality itself is absent, verified by grepping for the relevant
symbols directly:

| Feature | Present in WipeMud? |
|---|---|
| Remort *infrastructure* (flags, macros, `remort_vector` field, ascii-pfile `Rmrt:` save/load, `remort`/`redeem` commands) | ✅ Yes |
| Remort *actually gating anything* (spellcasting, guild access, skill list, item restrictions) | ❌ No — `spell_parser.c`, `spec_procs.c`, `class.c`'s `invalid_class` fix all carry zero remort-awareness in WipeMud |
| Paladin class *name* (`CLASS_PALADIN` in the class list) | ✅ Yes (one reference, in `class.c`) |
| Paladin alignment mechanic (Fallen/Unworthy, `PSF_FALLEN`/`PSF_UNWORTHY` actually checked anywhere) | ❌ No — the flags exist as declared bits but nothing in `spell_parser.c` (or anywhere else) reads or sets them |
| The 7 new spells (Holy Smite, Holy Shield, Dispel Magic, Full Heal, Silence, Ouchie, Immolate) | ❌ No — none of the `SPELL_*` defines exist in WipeMud's `spells.h` at all |
| `brag()` (PvP taunt broadcast) | ⚠️ Declared in `comm.h`, never implemented in `comm.c`, never called anywhere — dead prototype |
| `ROOM_GOOD_REGEN`, `MAX_RENT`, item-level-on-wear check | ✅ Yes (room flags and the constant are still defined; wear-level check not independently re-verified beyond the flag/constant existing) |
| `dispeller` mob special | Declared (`spec_assign.c`); its actual PvE effect depends on `SPELL_DISPEL_MAGIC` existing, which it doesn't in WipeMud — likely non-functional if assigned to a mob there |

Practical read: if a future decision ever revisits "should we use WipeMud
instead," the honest cost isn't just the empty player roster (already
covered in `circlemud-archive-report.md`) — it's losing the entire
Paladin class mechanic and the new spells outright, and reducing the
remort system to bookkeeping with nothing plugged into it.
