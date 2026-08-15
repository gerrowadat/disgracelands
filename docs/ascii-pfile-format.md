# The ascii_pfiles player-file format

A field-by-field reference for the text player-file format used by
`welmar/WipeMud` (in the archive) and produced by this repo's
`tools/bin2ascii.c`. Companion to `docs/pfile-conversion.md` (which covers
the *tools and what was verified*) and `docs/non-stock-features.md` (which
covers `Race:`/`Rmrt:`, the two fields that are Disgracelands-specific
extensions to this format rather than part of it upstream).

This is not Disgracelands' own invention: it's the public **ascii_pfiles
2.1** patch (by Alan K. Miles, building on an original by Chris Jacobson,
targeting stock CircleMUD 3.0 bpl17/19), archived in this project's parent
repo at `welmar/pfiles/ascii_pfiles_2.1/`. Everything in this document is
drawn from reading its reference implementation
(`welmar/pfiles/ascii_pfiles_2.1/full_src/db.c`'s `load_char()` and
`save_char()`, `sprintbits()`/`asciiflag_conv()`) and cross-checked
against real files in `welmar/WipeMud/lib/pfiles/`. **Not currently wired
into this repo's own `src/db.c`** — see `docs/pfile-conversion.md` and
`TODO.md` for that gap.

---

## Layout

```
lib/pfiles/
  plr_index          - one line per player, see below
  a/
    ass               - one file per player, named after them, lowercase
    aardvark
  z/
    zod
```

- Directory: first letter of the player's name, lowercased.
- Filename: the player's full name, lowercased, no extension.
- (From the reference source: `PLR_PREFIX "pfiles"`, `PLR_SUFFIX ""`,
  `SLASH "/"` — i.e. `pfiles/<c>/<name>` relative to `lib/`.)

## `plr_index`

One line per player:

```
<idnum> <name> <level> <flags-or-"0"> <last_logon_unix_time>
```

- `idnum` — the player's numeric ID (matches the `Id` field in their own
  file).
- `name` — as-created capitalization is not preserved here; matches the
  lowercase filename.
- `level` — kept in the index so things like `who`/`autowiz` don't need
  to open every player file just to sort by level.
- `flags` — the player's `Act`/`PLR_FLAGS` bits, letter-encoded (see
  "Bitflag encoding" below) — written as the literal single character
  `0` if no flags are set (an empty string here would misalign the
  `sscanf("%ld %s %d %s %d", ...)` that reads this line back, so the
  reference writer special-cases it).
- `last_logon_unix_time` — Unix timestamp, decimal.

The file ends with a line containing just `~` (terminator convention, see
below).

Real example (`welmar/WipeMud/lib/pfiles/plr_index`):

```
1 humbug 54 0 1043363755
2 zod 54 efghmnoqv 1043364802
```

## Player files: general rules

Each player file is a flat list of `Tag: value` lines. A handful of rules
apply throughout:

- **Tags are always exactly 4 characters**, left-padded with spaces where
  the real field name is shorter (`Ac  `, `Id  `, `Sex `, `Str `, `Int `,
  `Wis `, `Dex `, `Con `, `Cha `), followed by `: ` and the value.
- **Most numeric fields are omitted entirely if they equal the default**
  (`welmar/pfiles/ascii_pfiles_2.1/full_src/pfdefaults.h` — all defaults
  are `0`), to keep a fresh/plain character's file small. A few fields are
  written unconditionally regardless of value: `Name`, `Pass`, `Brth`,
  `Plyd`, `Last`, `Id`. Order in the file follows the order `save_char()`
  writes them in, but nothing reading the format should rely on order —
  `load_char()` reads line-by-line and dispatches on the tag, so fields
  can appear in any order or be absent.
- Unrecognized tags are logged as an error but otherwise skipped — the
  format tolerates unknown fields (e.g. hand-added comments) fairly
  gracefully, though there's no actual comment syntax.

## Field reference

| Tag | Meaning | Format |
|---|---|---|
| `Name` | Character name | string, always written |
| `Pass` | Password (already hashed by `crypt()` upstream of this) | string, always written |
| `Titl` | Title | string |
| `Desc` | Description shown on `look` | multi-line block, see below |
| `Sex ` | 0=neutral, 1=male, 2=female | int |
| `Clas` | Class index (0=Magic User, 1=Cleric, 2=Thief, 3=Warrior, 4=Paladin — Disgracelands-specific 5th class, see `docs/non-stock-features.md`) | int |
| `Race` | *(WipeMud only, not upstream ascii_pfiles)* Race index (0=Human, 1=Elf, 2=Gnome, 3=Dwarf) | int |
| `Levl` | Level | int |
| `Home` | Home town/starting location index | int |
| `Brth` | Birth (character-creation) time | Unix timestamp, always written |
| `Plyd` | Total seconds played | int, always written |
| `Last` | Last-login time | Unix timestamp, always written |
| `Host` | Last connection host, as `hostname:port` on this codebase (see `docs/non-stock-features.md` — stock format is hostname only) | string |
| `Hite` | Height | int |
| `Wate` | Weight | int |
| `Str ` | Strength | `value/percentile` (the "18/00"-style exceptional-strength bonus for warriors) |
| `Int ` | Intelligence | int |
| `Wis ` | Wisdom | int |
| `Dex ` | Dexterity | int |
| `Con ` | Constitution | int |
| `Cha ` | Charisma | int |
| `Hit ` | Current/max hit points | `current/max` |
| `Mana` | Current/max mana | `current/max` |
| `Move` | Current/max movement | `current/max` |
| `Ac  ` | Armor class | int |
| `Gold` | Gold carried | int |
| `Bank` | Gold in bank | int |
| `Exp ` | Experience points | int |
| `Hrol` | Hitroll bonus | int |
| `Drol` | Damroll bonus | int |
| `Alin` | Alignment (-1000..1000; also drives the Paladin fall-from-grace mechanic, see `docs/non-stock-features.md`) | int |
| `Id  ` | Numeric player ID | int, always written |
| `Act ` | Player flags (`PLR_*` — killer, thief, frozen, banned, etc.) | bitflags, see below |
| `Aff ` | Affected-by flags (`AFF_*` — poison, invisible, sanctuary, etc.) | bitflags, see below |
| `Pref` | Player preference flags (`PRF_*` — brief mode, autoexit, color, etc.) | bitflags, see below |
| `Thr1`–`Thr5` | Saving throw bonuses (5 categories) | int each, one tag per category |
| `Wimp` | Wimpy hitpoint threshold (auto-flee below this) | int |
| `Frez` | Level of the god who froze this player, if any | int |
| `Invs` | Invisibility level | int |
| `Room` | Non-default load room, if the player has one set | int |
| `Badp` | Consecutive bad password attempts | int |
| `Hung`/`Thir`/`Drnk` | Hunger/thirst/drunkenness condition timers | int each (omitted entirely for immortals — they don't need food/drink) |
| `Lern` | Practice sessions remaining | int |
| `Rmrt` | *(Disgracelands-specific, not upstream ascii_pfiles)* Remort vector bitmask — which classes' skills/spells this character can access. See `docs/non-stock-features.md`. | int (not letter-encoded, despite being a bitmask) |
| `Skil` | Skill/spell percentages | multi-line block, see below (omitted for immortals — they get all skills at 100% automatically on load) |
| `Affs` | Active spell/skill affects | multi-line block, see below |
| `PfIn`/`PfOt` | Custom poofin/poofout messages | string (only present if the patch was built with `ASCII_SAVE_POOFS` defined - not confirmed either way for the copy this project is based on) |

## Multi-line blocks

Three fields don't fit on one line and use a `Tag:\n...` block form
instead of `Tag: value`:

**`Desc:`** — the character's description, verbatim (CR characters
stripped), terminated by a line containing just `~` — the same
string-terminator convention CircleMUD world files use everywhere else.

```
Desc:
A grizzled old warrior stands here, sword in hand.
~
```

**`Skil:`** — one line per known skill/spell, `<skill_number> <percentage>`,
terminated by a `0 0` line:

```
Skil:
1 100
2 85
0 0
```

**`Affs:`** — one line per active affect, `<type> <duration> <modifier>
<location> <bitvector>` (matches `struct affected_type`'s five saved
fields), terminated by a `0 0 0 0 0` line:

```
Affs:
23 12 0 0 0
0 0 0 0 0
```

## Bitflag encoding (`Act`, `Aff`, `Pref`)

These three fields hold C `long` bitmasks. The reference writer
(`sprintbits()`) encodes them as a string of letters — one letter per set
bit, in bit order (lowest bit first), lowercase `a`–`z` for bits 0–25 and
uppercase `A`–`Z`(–`F`, in practice, since these fields cap around 32
bits) for bits 26 and up. A value of `0` is written as the literal digit
`0` (an empty string would break parsing).

The reader (`asciiflag_conv()`) accepts that letter form, **and** falls
back to reading a plain decimal number if the whole field is digits —
so a hand-edited or differently-generated file with `Pref: 2253040`
instead of the letter form is equally valid input. Real example, same
character, from a genuine file (`welmar/WipeMud/lib/pfiles/z/zod`):

```
Act : 128
Pref: efghmnoqv
```

`128` is a pure-digit string (bit 7 set) written as a plain number
because that's what a bare `%d`/`atol()` round-trip produces either
way; `efghmnoqv` is the letter form for a `Pref` value with bits 4, 5,
6, 7, 12, 13, 14, 16, and 21 set. Look up what each bit actually means in
`src/structs.h` (`PLR_*`, `AFF_*`, `PRF_*` defines) — this document
covers the encoding, not the flag catalog.

`tools/bin2ascii.c` (this repo's converter, working from the *older*
binary format that never had letter-encoded flags to begin with) currently
writes `Act`/`Aff`/`Pref` as plain decimal unconditionally, and `Rmrt` the
same way (matching the real `Rmrt:` fields observed, which are also plain
decimal, not letter-encoded, in every genuine example found). Per the
`asciiflag_conv()` fallback above, this is valid input, just not the
letter form a real `save_char()` would produce for `Act`/`Aff`/`Pref`.

## A complete real example

`welmar/WipeMud/lib/pfiles/z/zod`, real except the `Pass` value below,
which is redacted (`crypt()`-hashed or not, a real password field
shouldn't end up in a git repo — see the note at the end of this
document):

```
Name: Zod
Pass: [REDACTED - see note below]
Titl: God of Profanity and Vindicitveness
Sex : 1
Clas: 1
Race: 3
Levl: 54
Home: 1
Brth: 1043364802
Plyd: 0
Last: 1043364802
Hite: 48
Wate: 142
Alin: 12
Id  : 2
Act : 128
Room: -1
Pref: efghmnoqv
Lern: 159
Str : 11/0
Int : 11
Wis : 14
Dex : 8
Con : 8
Cha : 5
Hit : 5000/5000
Mana: 5000/5000
Move: 5000/5000
Ac  : 100
Gold: 99888
Exp : 9999000
Rmrt: 0
```

Note what's *not* here: no `Skil` block (this Zod is an implementor,
skills are auto-maxed on load per `docs/circlemud-archive-report.md` §5's
"if this is our first player --- he be God" logic, no need to save them),
no `Affs` block (nothing active), no `Hung`/`Thir`/`Drnk` (immortals don't
eat or drink), no `Desc` (never set one).

**On the redacted `Pass` field above**: an earlier version of this
document pasted the real value from the archive. That was a mistake and
has been scrubbed from git history, not just fixed going forward — see
`.gitignore`'s player-data policy and the first commit's message for why
real player data (including this) doesn't belong in this repo at all,
documentation included. If you're extending this document with another
real example, redact `Pass` (and anything else identifying, like `Host`)
the same way.
