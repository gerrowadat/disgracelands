# Player-file conversion: binary → ascii_pfiles

Covers the conversion tools and what's been verified. For the ascii
format itself, field by field, see `ascii-pfile-format.md`.

## What this is

CircleMUD3 (this tree's origin) stored all players as fixed-size
`struct char_file_u` records in one flat binary file, `data/etc/players`
(108 real characters, played 2001–2008 — see `circlemud-archive-report.md`
for the full history). That format is architecture-sensitive: it's a raw
`fwrite()` of a C struct, so reading it back correctly depends on matching
struct layout exactly, not just byte order.

`welmar/WipeMud` (the abandoned 2003 upgrade attempt) had already moved to
the `ascii_pfiles 2.1` third-party patch — one human-readable text file per
player instead of a binary blob. That's the right target format for
`Reborn` too: portable, diffable, and (bonus) directly git-trackable per
player if you ever want that.

## Tools (`reference/tools/`)

- **`bin2ascii.c`** — reads the binary `struct char_file_u` database and
  writes one ascii_pfiles-format file per player under `data/pfiles/<first
  letter>/<lowercased name>`, plus a `plr_index`. **Must be built 32-bit**
  (`gcc -m32 ...` — see the comment at the top of the file for why: the
  struct has several `long` fields that are 4 bytes on the 32-bit build
  that originally wrote this data and 8 bytes on a native 64-bit build,
  so a 64-bit `fread()` of the same struct silently misreads everything
  past the first `long` field. This is a real, concrete portability trap —
  distinct from, and in addition to, the SPARC/endianness question the
  archive report investigates and rules out for this specific data).
- **`pfiledump.c`** — reads and prints any ascii_pfiles-format file
  (doesn't care who wrote it or what architecture built it — it's just
  text). Ordinary native 64-bit build, part of `make utils`.

## What was actually verified

```
$ gcc -m32 -std=gnu89 -fcommon -w -Isrc -o bin/bin2ascii tools/bin2ascii.c
$ bin/bin2ascii data/etc/players lib/pfiles
bin2ascii: 108 records read, 108 non-empty players converted into lib/pfiles

$ bin/pfiledump welmar/WipeMud/lib/pfiles/z/zod    # genuine ascii pfile
welmar/WipeMud/lib/pfiles/z/zod   name=Zod   sex=1 clas=1 lvl=54 birth=2003-01-23 last=2003-01-23 tags=32

$ bin/pfiledump data/pfiles/z/zod                   # freshly converted
data/pfiles/z/zod                  name=Zod   sex=1 clas=3 lvl=34 birth=2006-02-10 last=2008-04-20 tags=247
```

Both a genuine WipeMud-produced ascii pfile and a freshly-converted
CircleMUD3 one parse cleanly. All 108 converted files were swept with
`pfiledump` and none produced a parse failure or an out-of-range field —
consistent with the endianness/struct-layout sanity check already done in
the archive report (§5).

Note the two "Zod" records are genuinely different characters/eras (class
1 vs 3, 2003 vs 2006-2008) — see the archive report's revised timeline
(§0): WipeMud and CircleMUD3 diverged after May 2003, they aren't the same
save continuing.

## What's NOT done yet

`Reborn`'s actual `reference/moderncserver/src/db.c` still uses the
original binary `load_char()`/`store_to_char()`/`char_to_store()` path at
runtime — the game doesn't read or write ascii pfiles when it's actually
running. The conversion above is a one-shot offline migration of the data,
proven readable, not yet a live-engine change.

Wiring ascii pfiles into the live login/save path is real, security-
adjacent work (it touches password handling) and was deliberately left
for a follow-up rather than rushed:

- Reference implementation: `welmar/pfiles/ascii_pfiles_2.1/full_src/db.c`
  (`load_char()` / `save_char()`, ~line 1953–2480) plus `diskio.c`/`.h`
  (buffered line-oriented file I/O helpers used by the ascii format) and
  `pfdefaults.h` (fallback values for fields the ascii format omits when
  they're zero, to keep files small).
- It's written against stock bpl17/19 `db.c`, which is very close to
  Reborn's (**Reborn's own `db.c` carries zero `<DoC>` local-mod tags** —
  see the archive report §7 — so this is one of the lower-risk files to
  patch), but every call site that constructs/consumes a `struct
  char_file_u` needs updating too: `comm.c`'s login "nanny" state machine,
  `interpreter.c`, `act.wizard.c` (force/load-style god commands),
  `house.c`, `mail.c`, and the `reference/moderncserver/src/util/` tools
  (`autowiz`, `mudpasswd`, `showplay`, `purgeplay`, `listrent`, `play2to3`
  all currently assume the binary format directly).
- `PLR_PREFIX`/`PLR_SUFFIX`/`SLASH` macros and the `data/pfiles/<letter>/`
  layout convention need adding (`bin2ascii` already writes to that
  layout in anticipation).
- The `Aff`/`Pref` bitflag fields in the reference implementation use a
  letter-encoded ascii bitfield (`asciiflag_conv`), not plain decimal —
  `bin2ascii.c` currently writes them as plain decimal for simplicity,
  which would need aligning with whatever encoding the ported `load_char`
  actually expects.
