# The `lib/` directory: what is stored where, and in what format

An investigation of the real Disgracelands `lib/` directory as preserved at
`~/scratch/lib/`, read against the C that produced it
(`reference/moderncserver/src`, CircleMUD 3.0 bpl20 + OasisOLC).

It covers every file in the tree, what writes it, what reads it, and what
its format actually is as opposed to what the format is supposed to be. The
addendum in §9 says which parts should reach the Go server's `data/`, in
what shape, and corrects the places where `docs/design/data-format.md`
guessed wrong about this data.

Written 2026-08-20. Supersedes nothing; it is the empirical companion to
`docs/design/data-format.md`, which was written against the *stock*
`lib/` shipped in `data/` and against a partial survey of an archive
snapshot.

---

## 0. Method, and the two results that came out of it

Per `CLAUDE.md`, nothing below is transcribed from reading C arithmetic.
Every structural claim is either (a) produced by running a loader over the
real files, or (b) decoded from the bytes and checked against the C's own
declaration.

Two results are worth stating before the detail, because they are the most
load-bearing things this investigation produced.

**The Go loader and the C server agree exactly on the real archived world.**
Both were pointed at `~/scratch/lib/world` and made to dump canonical JSON:

```
$ reference/moderncserver/bin/circle -J c.json -d <lib>
$ go run ./cmd/dlctl world dump --world-dir=<lib>/world --parity --out=go.json
2981 rooms, 944 mobiles, 1199 objects, 47 zones, 77 shops
IDENTICAL
```

Parity had previously only been demonstrated against the 3,202-record stock
world in `data/`. It now holds against the 5,248-record world the game was
actually played on, which is a materially stronger claim: this data exercises
paths the stock data does not (§2.6, §2.7, §7).

**A latent Go-vs-C divergence exists in zone assignment, and this data very
nearly triggers it.** `internal/persist/world/classic/classic.go:339` sorts
the zone table by `Bottom` and assigns each room to the *first* zone whose
range contains it. The C does not sort: `parse_room` (`db.c:909-924`) walks a
`static int zone` cursor forward through the zone table in **index-file
order**, and never goes back. The two agree only while the index is in
ascending vnum order with non-overlapping ranges.

The archive's index satisfies that — barely. Two zone files on disk would
break it, and both are present and only saved by being unlisted (§2.6).
Adding them to the index and re-running both loaders:

```
C  : 49 zones, room #0 -> zone 0  (0, 99,  'Limbo - Internal')
Go : 49 zones, room #0 -> zone 96 (0, 0,   'New Zone')
```

The Void changes owner, silently, in the Go loader only. Neither loader warns.
This is not in `docs/deviations.md`; the reasoning is in the code comment
above `assignRoomZones`, but the *consequence* is not written down anywhere.
See §9.6.

---

## 1. The tree, and what it is

```
lib/
  1                      Junk: a stray file from a mistyped tar command.
  README                 Stock CircleMUD's description of this layout.
  etc/                   Game-written state. 2.1 MB, almost all of it players.
  house/                 Player-house crash saves. 12 files, all zero bytes.
  misc/                  Damage messages, socials, banned names, bug reports.
  plralias/{A-E..ZZZ}/   Per-player alias files.
  plrobjs/{A-E..ZZZ}/    Per-player rent/crash files, plus three shell scripts.
  text/                  Screen text; text/help/ holds the help corpus.
  world/{wld,mob,obj,shp,zon}/   The world, plus zone.lst.
```

403 files, 5.4 MB. Every file is dated `Oct 21 2013`, the date of the archive
copy, not of the content.

`lib/1` contains `tar: can't add file 2 : No such file or directory` — someone
ran `tar` with the arguments in the wrong order and captured stderr into a
file called `1`. It is not data. It is included here only because a
directory-as-index loader would try to interpret it, which is §9.2's point.

`etc/.cvsignore` lists `hcontrol players plrmail badsites board.immort`, so
the tree was under CVS with the game-written state excluded. That is why the
`00` placeholder files exist in every `plralias/` and `plrobjs/`
subdirectory: 60 bytes each, reading "This is a placeholder file so the
directory will be created". CVS does not track empty directories, and the
game will not create them.

`plrobjs/` also contains three shell scripts — `purgeobjs`, `purgedir`,
`searchfor` — which drive `bin/delobjs` and `bin/listrent`. Operator tooling,
living in the data directory. Anything that treats `plrobjs/` as
"a directory of rent files" will find them.

---

## 2. `world/` — six formats, four of them positional

### 2.1 The index files, which are the actual manifest

`db.h:64-70` defines `INDEX_FILE` as `index` and `MINDEX_FILE` as
`index.mini`, one pair per subdirectory. Each is a list of filenames, one per
line, terminated by a line containing `$`. `index.mini` is the `-m`
(mini-mud) list.

`world/zone.lst` is **not** an index. It is a two-column prose table of zone
numbers and names, maintained by hand, and nothing reads it. It is also out
of date: it lists zones 26, 32, 41, 56 that have no files, and omits 0, 21,
22, 23, 24, 80–99, 120–190 that do.

### 2.2 Rooms — `wld/*.wld`

```
#3001
The Temple Of Midgaard~
   You are in the southern end of the temple hall...
~
30 cdeh 0
D0
At the northern end of the temple hall is a statue and a huge altar.
~
~
0 -1 3054
E
plaque~
The plaque reads...
~
S
```

`#vnum`, name, description (both `~`-terminated), then
`<zone> <flags> <sector>`, then any number of `D<dir>` exit blocks
(description, keyword list, then `<door-flag> <key-vnum> <to-room>`) and `E`
extra-description blocks, terminated by `S`. File terminator is `$~`.

The first number on the flags line is written but **ignored** — the loader
derives the zone from the cursor walk in `db.c:916-925`.

Flags are the letter encoding: `a`–`z` are bits 0–25, `A`–`Z` are bits 26–51,
and a field of `0` means no bits. Across the whole archive, **no bitfield
anywhere uses an uppercase letter**; the highest bit in use is 20 (mob
affects). The 32-bit ceiling `Flags.ExceedsCRange()` guards is not a live
concern in this data.

### 2.3 Mobiles — `mob/*.mob`

`#vnum`, keyword list, short desc, long desc, description (four `~`-terminated
strings), then

```
abdlno d 900 E          <act-flags> <affect-flags> <alignment> <S|E>
33 2 2 1d1+30000 2d8+18 <level> <thac0> <AC> <hp-dice> <damage-dice>
80000 160000            <gold> <exp>
8 8 1                   <position> <default-position> <sex>
```

`S` ends the record there. `E` means a trailing block of `Key: value` lines
terminated by a line containing `E`.

944 mobiles: **555 `E`-format, 384 `S`-format**. The `E` keys in use, counted
from parsed records rather than by grepping, are exactly the eight the
proposal assumed: `BareHandAttack` (77), `Str` (38), `Dex` (37), `Int` (27),
`Wis` (25), `StrAdd` (4), `Con` (1), `Cha` (1).

**A grep-based survey gets this wrong**, and it is worth recording how.
Searching the mob files for `^[A-Za-z]+:` also returns `saying:` (in mob
1513's description) and `Command:` (in mob 8206's, quoting a spell called
`Word of Command`). Both are prose inside a `~`-terminated string, invisible
to the real parser and indistinguishable to a line-oriented one. The closed
set in `data-format.md` §4.7 is right; the method that would most naturally
be used to check it is not.

Many `E`-format mobiles have an **empty** key block — `E` on the stat line and
`E` immediately after `8 8 1`, with nothing between. Zone 30's shopkeepers are
all like this. A writer that emits `S` whenever there are no abilities will
not round-trip these byte-for-byte.

### 2.4 Objects — `obj/*.obj`

```
#3040
plate breast~
a breast plate~
A breast plate is lying on the ground.~
~
9 0 ad 0                <type> <extra-flags> <wear-flags> <perm-affect>
7 0 0 0                 <value0..3>
100 18000 150 0         <weight> <cost> <rent-per-day> <min-level>
```

then any number of `E` extra-description and `A` affect blocks, terminated by
the next `#` or `$`.

The fourth number on the last line is the local `min_level` extension; the
loader accepts three or four.

**Value slots.** `values[0..3]` mean different things per type, per
`do_stat_object` in `act.wizard.c`. Against that table, **77 of the 1,030
objects whose type gives its slots a meaning carry a non-zero number in a
slot the type ignores** — 7.5%, not the "one in a hundred" `data-format.md`
§4.3 estimated. A further 169 objects are of types (`TREASURE`, `OTHER`,
`TRASH`, `WORN`, `PEN`, `BOAT`, `FIREWEAPON`, `MISSILE`) where no slot has any
meaning at all.

The 77 are not evenly spread. **62 of them are keys**, and the pattern is a
convention:

```
#900  a golden key           values: [900, 0, 0, 0]
#1508 the Key to the Gates   values: [1508, 0, 0, 0]
#2105 a skeleton key         values: [2105, 0, 0, 0]
```

Builders wrote the key's own vnum into `value[0]`. `ITEM_KEY` uses no values
whatsoever; the server has never read those numbers. It is bookkeeping that
looks like data. The remaining 15 are 9 armours, 5 weapons and 1 light with
stray numbers in genuinely unused slots.

This is exactly the case `data-format.md` §4.3 sets up ("a human decides
whether it was junk or whether it meant something") — and for the dominant
case the answer is now known.

### 2.5 Shops — `shp/*.shp`

The most fragile format in the tree: positional, `-1`-terminated lists, with
no field names and no way to tell where you are if you lose count.

```
CircleMUD v3.0 Shop File~      <- header line, first file only
#3100~
3100 3101 3102 -1              <- producing list, one per line, -1 ends it
1.1                            <- profit_buy
0.9                            <- profit_sell
-1                             <- buy-type list, -1 ends it
%s I haven't got such a drink.~   <- seven messages, in fixed order
... (six more)
0                              <- temper
0                              <- flags
3100                           <- keeper vnum
0                              <- trade_with
3106 -1                        <- room list, -1 ends it
6 22                           <- open1 close1
23 24                          <- open2 close2
$~
```

Four of the eighteen shop files (`19`, `81`, `90`, `95`, `97`) are **three
bytes**: `$~\n`. An empty shop file is legal and loads as zero shops.

The messages carry `%s` and `%d` and are fed to `printf` at runtime, so they
are format strings living in data.

### 2.6 Zones — `zon/*.zon`, and the six that do not load

```
#30
Northern Midgaard Main City~
3000 3099 15 2                 <bottom> <top> <lifespan> <reset-mode>
O 0 3094 1 3082 	(a suggestion board)
M 0 3040 1 3007 	(the bartender)
G 1 3016 100 -1 	(a hunk of cheese)
S
```

Everything after the required arguments on a reset line is a comment; the
builders used it consistently for the record's name, and OasisOLC writes it.
Lines beginning `*` are comments.

Across 47 loaded zones: **1,860 `M`, 1,159 `E`, 707 `D`, 537 `G`, 309 `O`,
157 `P`, 87 `R`** — 4,816 reset commands. **None** are disabled (`*`
opcode), and the loader resolved every vnum, so `renum_zone_table()` had
nothing to neuter.

**Six zone files exist on disk and are absent from the index**, along with
matching `wld`/`mob`/`obj` sets:

| Zone | Files present | What it is |
|---|---|---|
| 23, 90, 92, 147 | complete sets (90 also has a `.shp`) | Real content, deliberately switched off |
| 55 | `zon` only, range 5500–5599, named "New Zone" | An OasisOLC `zedit new` stub |
| 96 | `zon` only, **range 0–0**, named "New Zone" | Ditto, with a range that collides with Limbo |

This is `data-format.md` §3's argument, confirmed on the real thing: *the way
you disable a zone here is to unlist it, not to delete it*, and
directory-as-index would silently re-enable four zones of content plus two
broken stubs. Zone 96's `0 0` range is what triggers the Go/C divergence in
§0.

**The filename does not determine the vnum range.** Six room files hold rooms
belonging to other zones:

| File | Records outside its own hundred-block |
|---|---|
| `54.wld` | 191 (zone 54 spans 5400–5699 — it absorbed 55 and 56) |
| `25.wld` | 83 |
| `42.wld` | 53 |
| `72.wld` | 46 (all of zone 73, which has a `.zon` but no `.wld` of its own) |
| `40.wld` | 27 |
| `31.wld` | 6 |

Plus `190.mob` (6), `40.mob` (8), `40.obj` (5). Zone 73 is the sharpest case:
it is in the index, it has 13 door resets and 8 mobile resets, and every one
of its rooms lives in `72.wld`. Any importer that assumes `NN.wld` contains
rooms `NN00`–`NN99` will mis-file 416 records.

### 2.7 Stray files in `world/`

| File | What it is |
|---|---|
| `wld/30.mob` | A stale copy of `mob/30.mob`, in the wrong directory, differing by one flag letter (`abdl` vs `bdl` on mob 3159) |
| `wld/84.wld.DoCbackup` | A hand-made backup, 425 lines different from `84.wld` |
| `obj/25.obj.save`, `obj/42.obj.save` | OLC's own backups; `42` is identical to the live file, `25` differs by 399 lines |
| `mob/40.mobsave` | Identical to `40.mob` |

Five backup conventions, no two alike, none of them a suffix the loader knows
about. They are harmless only because the index is explicit.

---

## 3. `etc/` — game-written state

### 3.1 `players` and `players.beforewipe` — the binary character database

A flat array of `struct char_file_u` (`structs.h`), `fwrite`-dumped, no
header, no count, no version.

```
players             139,104 bytes = 108 records of 1288
players.beforewipe  1,965,488 bytes = 1526 records of 1288
```

1288 is the ILP32 size. `dlctl pfile verify` reads both cleanly: 108 and
**1,526** named characters, no empty slots, all on legacy `crypt(3)` DES
passwords. The 1,526-character pre-wipe roster is by far the largest single
piece of player history in the archive.

The `char_file_u` layout, its ILP32-vs-LP64 hazard and the local `Rmrt`
remort field are covered in `docs/investigations/ascii-pfile-format.md` and
`pfile-conversion.md`; they are not re-derived here.

### 3.2 `plrmail` — a hand-rolled allocator in a file

`mail.h:29,53-55`. Fixed 100-byte blocks. Block type `-1` = header, `-2` =
last data block, `-3` = deleted/free, anything else is the **byte offset** of
the next block. A header block is
`{long block_type; long next_block; long from; long to; time_t mail_time; char txt[81]}`
= 100 bytes on ILP32; a data block is `{long block_type; char txt[97]}`.

Decoding both spools:

| File | Blocks | Live mail |
|---|---|---|
| `plrmail` | 5 | **none** — every block is `DELETED` |
| `plrmail.beforwipe` | 49 | 3 chains (2002-10-16, 2002-10-30, 2004-03-08) |

**Deletion does not erase.** Every deleted block still holds its text, and it
is readable. `plrmail`'s five deleted blocks contain a complete abandoned
draft of remort instructions. `plrmail.beforwipe`'s 46 deleted blocks contain
players' pasted MUD session transcripts, bug reports and personal messages —
including one that opens "hey sean, doubt you'll ever get this".

Chains run backwards through the file (block 48's `next` is offset 4700 →
block 47, whose `next` is 4600 → block 46). Reading this format means
following offsets, not scanning.

Anyone converting this has to make a deliberate decision about the deleted
blocks. Following only the live chains discards them; converting every block
resurrects mail three people deleted twenty years ago. Neither is a default.

### 3.3 `board.*` — a struct dump containing a live pointer

`boards.c:66-82` defines six boards; **five files exist**. `board.pkill`
(vnum 3095) has no file: the board is compiled in but was never posted to.

Format: a 4-byte count, then one `struct board_msginfo` per message
(`boards.h`), then the heading and body strings back to back.

```
board.freeze, first message:
01 00 00 00   count = 1
1e 00 00 00   slot_num  = 30
e0 01 5a 08   heading   = 0x085a01e0   <-- a heap pointer, written to disk
22 00 00 00   level     = 34
20 00 00 00   heading_len = 32
3b 00 00 00   message_len = 59
"Fri Jul 30 (Humbug)     :: note\0" then the body
```

`data-format.md` §1 calls this "the argument for the whole proposal in one
field", and here it is in the real bytes: `0x085a01e0` is a FreeBSD/i386 heap
address from a process that exited in 2008. Its value is meaningless and the
loader ignores it, but its **width** decides where `level`, `heading_len` and
`message_len` sit.

### 3.4 `hcontrol` — zero bytes

`house.c` reads an array of `struct house_control_rec`. The file is empty, so
zero houses are registered — even though `house/` holds twelve `NNNN.house`
files for rooms 3070–3081, all also zero bytes. The house system was set up
and either never used or wiped.

### 3.5 `badsites` — the one text format in `etc/`

```
select prodigy 1050057197 Zod
select all 1050060243 Zod
select * 1050060259 Zod
select carbon.redbrick.dcu.ie 1067009360 Zod
select netsoc.tcd.ie 1067009378 Zod
```

`<ban-type> <site> <unix-date> <banned-by>`. Matching is by substring, so
`all` and `*` never matched anything.

### 3.6 `time` — one number

`682713460\n`. `db.c:483-500` reads it with `fscanf("%ld")` into
`beginning_of_time`, falling back to the hardcoded `650336715` if it is zero
or missing; `save_mud_time` (`db.c:531-545`) writes it back with
`fprintf("%ld\n")`. It is the game's epoch — 1991-08-19 here, against
CircleMUD's default of 1990-08-11 — and everything about in-game date, season
and daylight is derived from it. No name, no units, no marker.

---

## 4. `plralias/` — length-prefixed, and not line-oriented

`utils.c:531-545` buckets by first letter: `A-E`, `F-J`, `K-O`, `P-T`,
`U-Z` (six letters, not five), `ZZZ` for anything else. Filename is the
lowercased character name plus `.alias`. 21 alias files survive.

`alias.c` writes each entry as five lines:

```
<len(alias)>
<alias>
<len(replacement)>
<replacement>
<type>            0 = simple, 1 = complex (contains $*)
```

**The lengths are load-bearing and the file is not line-oriented.** The
writer emits `strlen(replacement) - 1` and skips the first character, because
the in-memory replacement carries a leading space that the reader re-adds
(`*xbuf = ' '` before `fgets`). And replacements can contain newlines —
`amel.alias` has

```
4
stab
36
backstab $*;remove knife;wield whip
<blank line>
1
```

where the declared 36 counts a trailing `\n` inside the value. A parser that
splits on newlines mis-reads this file from that point on.

Alias names as short as one character occur (`amel` has aliases `9` and `1`),
which makes a length-ignoring heuristic parser even easier to fool.

---

## 5. `plrobjs/` — the rent files, and what they do not contain

Same bucketing as `plralias/`, suffix `.objs`. 79 files.

A rent file is a 56-byte `struct rent_info` header followed by any number of
48-byte `struct obj_file_elem` records. Both sizes are confirmed empirically:
every file's length is `56 + 48n`.

```
obj_file_elem, ILP32:
  sh_int item_number   2 + 2 pad
  int    value[4]     16
  int    extra_flags   4
  int    weight        4
  int    timer         4
  long   bitvector     4
  struct obj_affected_type affected[6]   12   (byte + sbyte, packed)
                                        ---
                                         48
```

### 5.1 `USE_AUTOEQ` is 0, and this costs the archive real information

`structs.h:30` sets `USE_AUTOEQ` to 0 in this tree. The consequence is
severe and is not what `data-format.md` §8 assumes.

`Crash_rentsave` computes a wear location for every equipped item (`j + 1`
for wear slot `j`) and `Crash_save` computes a negative location for
container contents — and `Obj_to_store` then **discards it**, because the
`location` field is inside `#if USE_AUTOEQ`. `Obj_from_store` sets
`*location = 0` unconditionally.

So the archived rent files are a genuinely flat list. **There is no wear-slot
information and no container nesting in any of them.** Every player returns
with everything in inventory and every container empty. The negative-location
encoding `data-format.md` §8 describes as "ingenious and completely opaque"
is not present in this data at all.

### 5.2 The header leaks uninitialised memory

`struct rent_info rent;` is a stack local in `Crash_rentsave`,
`Crash_crashsave` and `Crash_cryosave`, and none of them memsets it or
assigns `nitems` or `spare0`–`spare7`. `Crash_write_rentcode` `fwrite`s the
whole thing.

Decoding all 79 headers:

- `rentcode`: 76 `RENT_CRYO`, 3 `RENT_RENTED`.
- `time`: 2006-02-10 to 2008-04-21.
- `nitems`: garbage. 48 files say 0, five say 2147483647, and the rest hold
  values like 135229440 and 134927257.
- `spare0`–`spare7`: 86 distinct values including `0xbfbfxxxx` (FreeBSD i386
  stack addresses) and `0x08xxxxxx` (heap).

The loader never reads `nitems` — `Crash_load` reads until EOF — so this has
never mattered. It matters now only as evidence: a format whose fields are
whatever was on the stack is not a format.

`Crash_crashsave` additionally leaves `gold`, `account` and
`net_cost_per_diem` uninitialised, so those fields are meaningful only in
`RENT_RENTED` and `RENT_CRYO` files.

---

## 6. `text/` and `text/help/`

Plain text, read wholesale by `file_to_string_alloc` and paged to the player.
`db.h:73-84` names them: `credits`, `news`, `motd`, `imotd`, `greetings`,
`info`, `wizlist`, `immlist`, `background`, `policies`, `handbook`.

`greetings` names CircleMUD, Jeremy Elson and the five DikuMUD creators —
which is what `scripts/license-check.sh` exists to protect, and it is intact
in the archive.

Two of these files disagree with each other about what the game is called:
`motd` says "Welcome to Disgracelands MUD", `immlist` says "reached
immortality on **RedbrickMUD**". Historical residue, not a format issue.

### 6.1 Help — and there is no level restriction

`text/help/index` lists five `.hlp` files. Each is a concatenation of
entries: a keyword line (space-separated keywords), the body, and a line
containing `#`. The file ends with `$`.

`db.c:1701-1735` (`load_help`) is the whole parser. Three things follow from
reading it:

- **The entry text includes its own keyword line** —
  `strcpy(entry, strcat(key, "\r\n"))` — which is why help output starts by
  repeating the keywords.
- **Every keyword on the line becomes a separate `help_table` entry**
  pointing at the same text, with a `duplicate` counter.
- **There is no level field.** bpl20's help format has no per-entry
  restriction, and `do_help` (`act.informative.c:953`) does a binary search
  and pages the result with **no level check of any kind**.

So `wizhelp.hlp` is loaded from the same index as everything else and its 51
entries are readable by any mortal who guesses a keyword. `data-format.md`
§7 says `min_level` "replaces the convention that wizard help lives in a
separately-named file that the loader treats differently" — the loader does
not treat it differently. There is no convention to replace; there is a
missing feature, and on revival it is a disclosure decision, not a format
one.

Entry counts: `commands.hlp` 99, `wizhelp.hlp` 51, `spells.hlp` 43,
`info.hlp` 20, `socials.hlp` 4.

`text/help/screen` is `HELP_PAGE_FILE`, the bare `help` output.

---

## 7. `misc/`

**`socials`** — 105 socials. Header line `<command> <hide> <min-victim-pos>`,
then messages, `#` meaning "none".

`act.social.c:216+` reads **three** messages unconditionally
(`char_no_arg`, `others_no_arg`, `char_found`) and, only if `char_found` is
not `#`, five more. That is why `applaud` is three lines and `accuse` is
eight.

The important coupling: a social's slot is `find_command(next_soc)`. **If the
command is not already in `interpreter.c`'s table as a `do_action` entry, the
social is logged as a SYSERR and dropped.** `misc/socials` is not
self-describing; half of each social lives in compiled code.

**`messages`** — 55 damage-message records. `M`, a damage number from
`spells.h`, then twelve message lines (death/miss/hit/god × attacker/victim/
room), `#` for none, all twelve always present. Lines starting `*` are
comments and only legal between records.

**`xnames`** — 24 disallowed name substrings, one per line.

**`bugs`, `ideas`, `typos`** — append-only logs,
`%-9s (%b %e) [%5d] %s`: reporter, date without a year, room vnum, text. 6
bugs, 8 ideas, 1 typo. The missing year makes them unsortable.

---

## 8. Cross-cutting findings

### 8.1 Encoding: one file, three bytes

Sweeping every non-binary file in the tree for bytes above 0x7E or ESC
returns exactly one file: **`world/wld/90.wld`, with three `0x92` bytes** —
CP1252 right single quotation marks, from text pasted out of Word:

```
This path loops back here! At least that’s what it looks like.
```

Everything else in `world/`, `text/`, `misc/`, `etc/badsites` and the alias
files is 7-bit ASCII. And `90.wld` is one of the **unlisted** files (§2.6), so
the CP1252 transcode `dlctl convert` performs has, in this snapshot, nothing
to do to any file the server loads.

`data-format.md` §7 said the archive's world/text/misc were "pure 7-bit
ASCII". Nearly right — the exception is real, and it is in a file the earlier
survey presumably skipped because the index does not mention it.

**Zero ESC bytes** anywhere outside the binary player database, confirming
§12 of the proposal.

### 8.2 Text pathologies, and why they matter more than expected

Counted over parsed prose fields, not raw bytes:

| Field | Trailing whitespace | Tabs | `\r\r` runs |
|---|---|---|---|
| room descriptions | **742 of 2,981 (25%)** | 7 | — |
| mob descriptions | 117 | 3 | — |
| mob long descriptions | 6 | — | — |
| object descriptions | 5 | — | 3 |
| object keywords | 1 | — | — |
| object short descriptions | 1 | — | — |

**A quarter of all room descriptions have a line ending in whitespace.**
`data-format.md` §2.4 and §10.3 flag this as "the residual risk … and it is
real" — a YAML block scalar cannot represent a line with trailing whitespace,
so a conforming emitter must fall back to double-quoted style. At 25% of
rooms, that is not an edge case the fuzz test will catch occasionally; it is
the common case, and one room description in four would be written as a
single escaped line unless the writer strips the whitespace (a data change) or
the format grows a way to express it.

Object #0's description contains sixteen consecutive `\r` bytes before a
`\n`. It is the stock "This object is BAD!" placeholder and nothing loads it,
but any round-trip test that includes it will exercise this.

### 8.3 The C's own complaints about this data

Booting the C server against the archive produces 207 instances of
`SYSERR: Object #-1 (<short desc>) uses '(null)' spell #N`, from
`check_object_spell_number` (`db.c:2936-2982`), across 44 distinct spell
numbers.

Two things about this are worth recording so nobody chases it:

- **The vnum is always `-1`**, because `check_object` runs on the prototype
  before its index entry exists. The message cannot be used to find the
  object.
- **It fires on the stock world too** — 133 times against the repo's `data/`.
  It is a stock-CircleMUD condition, not archive corruption. The archive's
  higher count is consistent with Disgracelands' seven custom spells.

`dlctl world lint` against the archive reports **0 errors, 20 warnings, 8
notes**. The warnings are the five unlisted-file groups, two shops in
nonexistent rooms (#3008 → room 3056, #5433 → room 6563), eleven shop
`producing` entries for objects that do not exist, and two exits locked by
nonexistent keys (room 12038, room 14258). The notes are eight drink
containers whose weight the loader raises to match their capacity.

Nothing in the archive fails to load.

---

## 9. Addendum — what should reach `data/`, and in what form

### 9.1 The starting position, which is not what it looks like

The repo's `data/` today is **stock CircleMUD bpl20's `lib/`**, not
Disgracelands'. `data/text/greetings` says "Your MUD Name Here";
`data/world` is 1,878 rooms and 30 zones against the archive's 2,981 and 47;
`data/pfiles` holds `puff`, `filthy` and `captain stolar`.

`docs/design/data-format.md` §1 opens "`data/` today is CircleMUD 3.0
bpl20's `lib/`, unmodified", which is accurate but easy to misread as *the
archive's* `lib/`. The real world has never been imported. That import — with
the C-parity result in §0 as its acceptance test — is the single highest-value
step available, and it is available now.

### 9.2 What should move, and what should not

| Source | Destination | Notes |
|---|---|---|
| `world/{wld,mob,obj,shp,zon}` (46 indexed files) | `data/world/*.yaml`, one per zone | The whole point. Parity-verified. |
| The four unlisted content zones (23, 90, 92, 147) | `data/world/`, `enabled: false` | Carry them, off, with the note. §9.3. |
| The two stub zones (55, 96) | **Drop**, with a line in the import report | Zero rooms, zero resets, and 96's range is actively dangerous. |
| `world/zone.lst` | **Drop** | Prose, unread, and wrong. Its "(Cont)" content is already in the zone ranges. |
| Backups (`*.DoCbackup`, `*.save`, `*.mobsave`, `wld/30.mob`) | **Drop** | That is what git is for now. |
| `lib/1`, `00` placeholders, `.cvsignore` | **Drop** | Artefacts of tar and CVS. |
| `plrobjs/{purgeobjs,purgedir,searchfor}` | `scripts/`, if anywhere | Operator tooling, not data. |
| `etc/players` (108) and `players.beforewipe` (1,526) | `data/players/` | §9.4. |
| `plralias/*.alias` | folded into the player file | 21 files. |
| `plrobjs/*.objs` | folded into the player file | 79 files. §9.4. |
| `board.*` (5 files) | `data/state/boards.yaml` | Six boards, five files; keep `pkill` as an empty board. |
| `plrmail`, `plrmail.beforwipe` | `data/state/mail.yaml` | §9.5 — needs a decision, not a default. |
| `hcontrol`, `house/*.house` | `data/state/houses.yaml`, empty | All zero bytes. Nothing to carry. |
| `badsites` | `data/state/bans.yaml` | Five entries. |
| `etc/time` | `data/state/clock.yaml` | `682713460` → an RFC 3339 epoch. |
| `misc/{bugs,ideas,typos}` | `data/state/reports.yaml` | 15 entries. Years are missing and cannot be recovered. |
| `misc/{socials,messages,xnames}` | `data/config/` | §9.7 for the socials coupling. |
| `text/*`, `text/help/*.hlp` | `data/text/`, one help entry per file | 217 help entries across five files. |

### 9.3 Corrections the evidence forces on `data-format.md`

Six, in rough order of how much work they change.

**§8's object model is wrong for this data.** `USE_AUTOEQ` is 0 (§5.1), so
the archived rent files carry no wear slots and no container nesting. The
proposal's `equipment: {wield: {...}}` block and its nested `inventory:` tree
have nothing to be populated from. The format should keep both — they are
right for data the *Go* server will write — but the importer must emit every
archived object as a flat inventory entry, and the migration notes should say
so plainly, because "Aragorn logged out wearing his sword" is not recoverable
and someone will otherwise assume the converter lost it.

**§4.3's lossiness estimate is off by an order of magnitude, and the answer
is known.** 7.5% of typed objects, not 1%, and 62 of the 77 are the key-vnum
convention (§2.4). Those 62 should convert to the typed form with the junk
*dropped* and one line in the import report, not preserved as `values:` raw
form — preserving them means 62 objects permanently opting out of the
readable representation to record a number no server has ever read. The other
15 should stay raw and get lint-reported individually.

**§7's help `min_level` has nothing to replace.** There is no level
restriction in bpl20's help (§6.1). Adding `min_level` is a new feature and a
behaviour change: wizard help is currently world-readable. That is worth
doing, and it is worth *saying* it is a change, in `docs/deviations.md`.

**§4.7's closed set of espec keys is right, but the check that produced it is
not reproducible by grep** (§2.3). Anyone re-verifying it against a new
snapshot should parse, not search.

**§2.4/§10.3's trailing-whitespace risk is the common case, not the edge
case** (§8.2). 25% of room descriptions. This should be decided before the
writer is built, not discovered by the fuzz test: either the writer falls back
to quoted style for a quarter of all rooms, or `dlctl world import` strips
trailing whitespace as a declared, reported, one-time normalisation. The
second is a data change and belongs in `docs/deviations.md`; it is probably
still the right answer, but it is the kind of thing that must be chosen out
loud.

**§3's "one file per zone" needs the vnum range, not the filename.** Six room
files hold other zones' rooms, 416 records in total (§2.6), and zone 73 has
no file of its own at all. The importer must group by the `.zon` ranges and
must not infer anything from `NN.wld`.

### 9.4 Players

`data-format.md` §8's one-player-one-file design holds up well against the
real data, and the fragmentation it complains about is real: a character with
aliases and rent is spread across four files in three directories, and the
`plralias`/`plrobjs` buckets do not even agree with the `pfiles` buckets the
ascii format uses.

Two additions the archive argues for:

- **Import both rosters.** 108 live characters and 1,526 pre-wipe. They are
  different populations, not versions of one; the pre-wipe file is the only
  record of the 2001–2006 playerbase. They want separate trees
  (`data/players/` and something like `data/players-prewipe/`), not a merge —
  idnums collide.
- **The credential scheme is uniform.** All 1,634 characters across both
  files are on legacy `crypt(3)` DES. `des-crypt10$…` per §8 covers every one;
  there is no second case to handle.

### 9.5 Mail needs a decision made by a person

§3.2: the live spool has **no live mail at all**, and both spools are mostly
deleted-but-readable blocks containing twenty-year-old private messages and
pasted session logs.

`state/mail.yaml` as specified would be an empty list for `plrmail` and three
messages for `plrmail.beforwipe`. That is almost certainly correct. But the
converter should *report* the deleted blocks rather than pass over them in
silence, because "the mail file was empty" and "the mail file contained 46
deleted blocks I chose not to read" are very different statements to make
about someone else's correspondence, and only one of them is honest.

### 9.6 Fix the zone-assignment divergence before the import, not after

§0 established that `assignRoomZones` and the C's cursor disagree when zone
ranges overlap or the index is out of order, and that enabling one file
already present in the archive triggers it.

Two things follow. `internal/persist/world/classic` should either reproduce
the C's forward-only cursor or keep the sort and document the difference in
`docs/deviations.md` with this reproduction as the example. And `dlctl world
lint` should report overlapping zone ranges and a zone table that is not
ascending — neither is currently detected, which is why the divergence is
silent on both sides.

This matters more once `yaml` exists, because §4.4's structural resets make
"which zone owns this room" a load-time question with a written-down answer,
and the two loaders would bake different answers into the converted files.

### 9.7 Two couplings the format has to carry or break deliberately

**Socials are half-compiled.** A social exists only if `find_command` locates
it in `interpreter.c`'s table (§7). `data/config/socials.yaml` as specified
in §7 of the proposal drops that half. Either the file carries enough for the
loader to synthesise the command entry (which is the right answer — it is
what `Command.CLine` already does for ordering), or the migration notes have
to say that adding a social still requires a code change, which would be a
strange thing for a data format that claims "point it at `data/` and run".

**Board definitions are compiled too.** `boards.c:66-82` holds the six vnums
and their read/write/remove levels; the files hold only messages.
`state/boards.yaml` should carry both, which the proposal's §9 already
implies but does not say — and it should carry `board.pkill` as an empty
board rather than omitting it, so that "five files on disk" does not quietly
become "five boards".

---

## 10. Summary of what is new here

| | |
|---|---|
| Go/C parity on the **real** world | 5,248 records, identical |
| Latent Go/C divergence found | zone assignment, reproducible, undocumented |
| Rent files carry no wear slots or nesting | `USE_AUTOEQ` is 0 |
| Rent headers leak stack addresses | `nitems` and 8 spares never initialised |
| Deleted mail is readable | 51 of 54 blocks across both spools |
| Help has no level restriction | `wizhelp.hlp` is world-readable |
| Trailing whitespace in room descs | 742 of 2,981 (25%) |
| Value-slot junk | 77 of 1,030 typed objects, 62 of them the key-vnum convention |
| Non-ASCII in the tree | 3 bytes, one unlisted file |
| Filename ≠ vnum range | 416 records in 9 files |
| Pre-wipe roster size | 1,526 characters |
