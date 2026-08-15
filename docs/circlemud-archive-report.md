# Disgracelands Archive Report

Investigation of this directory's CircleMUD-derived trees, backups, and player
data, done in preparation for a possible revival. Written 2026-08-15,
revised same day after empirical player-file analysis overturned the
original timeline (see §0), then again after the first slice of the
revival itself actually got built (see §10).

All paths below are relative to the repo root (`/home/doc/git/disgracelands`).

---

## 0. Revision note — read this first

The original version of this report concluded `welmar/WipeMud` (CircleMUD
3.1) was "the most current" tree and recommended it as the revival baseline,
with `welmar/CircleMUD3` treated as an superseded pre-upgrade snapshot from
2001–2003.

**That conclusion was wrong, and the fix is important.** While building an
offset table for `struct char_file_u` to check the SPARC/endianness question
(§5), I decoded all 108 real character records in
`welmar/CircleMUD3/lib/etc/players` and found their birth/last-login
timestamps run from **March 2006 to April 2008** — years *after* the May
2003 `WipeMud` upgrade this report originally treated as the endpoint.
Cross-checking `welmar/CircleMUD3`'s own game log confirms it: `syslog`,
`log/restarts` (353 entries), and `log/levels` show continuous play and
restarts from ~2001 through at least March 2008. `welmar/WipeMud`, by
contrast, has restart logs only through May 2003 and an empty live syslog.

**Read together with §7's `<DoC>` tag counts (68 in `CircleMUD3` vs. 45 in
`WipeMud`), the real story is: the May 2003 upgrade to CircleMUD 3.1 was a
largely abandoned side-branch. `welmar/CircleMUD3` is the tree that was
actually played, built on, and modified for the game's entire life.** That
makes it the correct revival baseline — the opposite of the original
recommendation. §1, §7, and §8 below have been updated accordingly; §4's
timeline retains the original (incomplete) reconstruction for reference but
should be read with this correction in mind.

This also resolves the SPARC question (§5): decoded player values are
uniformly sane under a little-endian x86 layout (0 anomalies across 108
records), and the game's own connection logs show players connecting via
`redbrick.dcu.ie` shell hosts (`minerva`, `deathray`) as a jump box —
Redbrick, DCU's shell service, is well known for having run Sun/SPARC
hardware. That's almost certainly the source of the "SPARC" memory: SPARC
on the client/shell-account side, not on `welmar` (the MUD host itself),
which was FreeBSD/x86 throughout.

---

## 1. The short version

- This archive holds the remains of **Disgracelands MUD**, a CircleMUD-based
  game run under the account `mud` (home directory `welmar/`), announced in
  its own login banner as "Based on CircleMUD 3.1" and admin'd by someone
  going by **humbug**.
- There isn't one canonical "the code" — there are **five** CircleMUD source
  trees plus a pile of dated tarball snapshots, representing different points
  on the same timeline, not different games.
- **Decision (revised — see §0): `welmar/CircleMUD3` is the revival
  baseline**, not `welmar/WipeMud`. `CircleMUD3` is the tree that was
  actually played continuously from ~2001 to at least 2008 (108 real
  characters, restart logs spanning that whole range) and carries the most
  local modification (68 `<DoC>`-tagged blocks vs. `WipeMud`'s 45,
  including features that never made it into `WipeMud` at all — see §7).
  `WipeMud` (CircleMUD 3.1) was a May-2003 upgrade attempt, itself not
  stock (it has 45 `<DoC>` blocks of its own, confirmed against a real
  stock 3.1 mirror), but it appears to have been abandoned within weeks in
  favor of continuing to run `CircleMUD3`.
- `CircleMUD3` stores players in the classic binary `struct char_file_u`
  dump (§5) rather than `WipeMud`'s plain-text `ascii_pfiles` format — the
  plan is to apply the same `ascii_pfiles` conversion to `CircleMUD3`'s own
  (much richer) player data, gaining portability without losing the 2001–
  2008 population. See §8.
- Player files are **not SPARC binary dumps** in anything I could find here —
  everything I could fingerprint was built for **FreeBSD/i386** (little-
  endian x86, not SPARC). See §5 for detail and a flag on this, since it
  contradicts what you remembered.
- The MUD appears to have gone dark for good sometime after **May 2003**,
  when a directory rename broke the autorun cron job; it kept silently
  failing to restart for over two years (logged through September 2005)
  without anyone noticing.

---

## 2. Inventory of CircleMUD-type directories

| Path | CircleMUD version | Patched with | Binary arch (if present) | Role |
|---|---|---|---|---|
| `welmar/oldmud/CircleMUD3` | 3.0 bpl19 (`0x030013`) | stock-ish (no OLC/DG in this tree) | FreeBSD 4.5, i386 | Oldest surviving install. Tiny syslog (~50KB). |
| `welmar/CircleMUD3` | 3.0 bpl20 (`0x030014`) | **OasisOLC** + mail/houses, no DG Scripts (see below) | FreeBSD 5.4, i386 | The long-running **production** copy, pre-2003-wipe. Has the fullest world (53 zones) and the richest player history (see §5). |
| `welmar/testmud` | 3.0 bpl20 (`0x030014`) | same patch set as CircleMUD3 | binary not present in archive | Builder/dev sandbox, port 4438. Smaller world (46 zones), separate `log/` history running to Oct 2002. |
| `welmar/WipeMud` | **3.1** (`0x030100`, final release) | OasisOLC + **DG Scripts** + races + context-help OLC, **ascii_pfiles 2.1** applied | FreeBSD 4.6.2, i386 | Result of the May 2003 upgrade+wipe. Port 4444. Most current **code**, least populated **world state** (9 characters). |
| `CircleMUD3` (repo root) | 3.0 bpl20 (`0x030014`) | same as `welmar/CircleMUD3` | **Linux x86-64**, glibc, kernel 2.6.15-era | Source-identical to `welmar/CircleMUD3` (only `.o` files and 3 `conf.h` lines differ — `crypt.h`/`socklen_t` detection). This is your own later attempt (file dates Oct 2013) to get the old code building on modern Linux. Not a distinct lineage. |
| `c.tgz` (top level) | 3.0 bpl20 | same | — | Tarball of the same `CircleMUD3` tree (2,326 entries), no player/world divergence found. Redundant backup, safe to treat as a duplicate of `welmar/CircleMUD3`. |
| `circle-3.1-w-goodies.tar.gz` (top level) | n/a | n/a | n/a | **Not actually a tarball** — `file` reports it as an XHTML document (looks like a saved 404/error page from a long-dead download link). Misleading filename; contains no code. |

None of these are "stock" CircleMUD in the sense of an unmodified upstream
release — every runnable tree carries the **OasisOLC** (online building)
add-on, extremely common in the CircleMUD community circa 2000-2003.
**DG Scripts** (scripting/triggers) is a separate add-on that only made it
into `welmar/WipeMud` — `CircleMUD3` (the tree that was actually played)
never has it, confirmed by grep: no `dg_*.c` file, no `dg_*` symbol
referenced anywhere, no `dg_*.o` in its Makefile. See
`docs/non-stock-features.md` for the full breakdown of what's genuinely
custom vs. third-party add-on vs. present in one tree but not the other.
The one genuinely unmodified stock copy in the archive is buried in
`tarfiles/circle30bpl19.tar` (see §3), useful as a diff baseline if you
ever want to see exactly what Disgracelands changed.

### Non-CircleMUD material that looks related but isn't Disgracelands' own code

- `tarfiles/CurrentVersion.tar` — despite the name, this is **MudOS v21**
  (an LPMud driver), not CircleMUD.
- `tarfiles/lima-1.0a9+driver.tar` — the **Lima** LP-style codebase.
- `welmar/zones1/*` and `welmar/areas/*` — large collections of `.are`/`.zon`
  files from other MUD codebases (Merc, ROM, SMAUG, Ack, Envy, etc.) and
  third-party CircleMUD area packs. These read as a **builder's reference
  library** (ideas/conversion fodder for zone building), not zones that were
  ever live on Disgracelands. Only `welmar/zones` (164 dated `.tgz` files,
  matching Disgracelands' own zone numbers) and `welmar/world-backups`
  (1,184 dated snapshots) represent the actual game world.
- `welmar/ancala` / top-level `ancala` / `submissions/` — individual builder
  zone submissions (`.wld`/`.mob`/`.obj`/`.zon` + doc files), i.e. raw
  material that may or may not have been merged into the live world.

---

## 3. Version identification detail

CircleMUD embeds its version as a hex macro in `structs.h`:
`#define _CIRCLEMUD 0x MMmmPP` (Major/Minor/Patchlevel).

- `0x030013` = 3.0 pl19 (`welmar/oldmud/CircleMUD3`)
- `0x030014` = 3.0 pl20 (`welmar/CircleMUD3`, `welmar/testmud`, root `CircleMUD3`, `c.tgz`)
- `0x030100` = 3.1 (`welmar/WipeMud`) — the final CircleMUD release,
  per `welmar/WipeMud/release_notes.3.1.txt`, dated November 18, 2002,
  "9 years and a day" after CircleMUD 2.20.

For an unmodified reference copy of 3.0 bpl19 to diff against, see
`tarfiles/circle30bpl19.tar` — this is a plain upstream tarball with no
Disgracelands-specific changes, useful for isolating exactly what your
OLC/local patches changed. See `docs/non-stock-features.md` for that
work already done.

---

## 4. Reconstructed timeline

Based on ChangeLog versions, restart/syslog timestamps, backup filenames,
and the crontab, the story appears to be:

1. **Pre-2001ish** — MUD runs as `welmar/oldmud/CircleMUD3`, bpl19, on
   FreeBSD 4.5/i386.
2. **2001–2003** — Upgraded to bpl20 (`welmar/CircleMUD3`), patched with
   OasisOLC (not DG Scripts — see §2), on FreeBSD 5.4/i386. This is the long-lived
   production era: `welmar/scripts/crontab.mud` backs up
   `~mud/CircleMUD3/lib/world` nightly (`welmar/scripts/world_backup.sh`),
   producing the 1,184 files in `welmar/world-backups` (Sept 2002 – Dec
   2002 is the surviving range) and the 164 per-zone tarballs in
   `welmar/zones`. `welmar/testmud` runs in parallel as the builder sandbox.
   Several dated full-tree backups exist in `welmar/backups/` (`before-olc.tgz`,
   `mud-backup-02032002.tgz`, `entiremud-20020922.tgz`,
   `circlemud3-20021130.tgz`, `mud-src-20030125.tgz`, etc.) — `before-olc.tgz`
   in particular predates the OasisOLC patch being applied, if you ever want
   truly-stock Disgracelands.
3. **A first character wipe** happened at some point *within* this bpl20
   era: `welmar/CircleMUD3/lib/etc/players.beforewipe` (1.9MB binary
   database) is noticeably larger/older than the `players` file that
   replaced it (139KB) — i.e. there were two wipes, not one.
4. **May 13, 2003** — Upgrade to CircleMUD 3.1, renamed to `WipeMud`
   (the name is on the nose: this upgrade **wiped the playerbase**).
   `welmar/CM3-beforeupg.tgz` and `welmar/wipe-beforeupg.tgz` are the
   before-and-after snapshots taken minutes apart that day. The `ascii_pfiles
   2.1` patch (`welmar/pfiles/ascii_pfiles_2.1/`) was applied as part of this
   upgrade, converting player files from raw binary structs to plain text
   key/value files. Post-wipe, only **9 characters** exist (`ass`, `chard`,
   `dubious`, `humbug`, `manic`, `marvin`, `mellow`, `test`, `zod`) —
   confirmed identical between the "before wipe" backup and the live
   `welmar/WipeMud/lib/pfiles/` today, so nothing further was lost after
   that point, but this wipe did discard the multi-year playerbase that
   existed in `welmar/CircleMUD3`.
5. **After the 3.1 upgrade**, `welmar/syslog` (the root autorun log, 124MB)
   shows the autorun script repeatedly failing:
   `/home/mud/CircleMUD3/autorun.sh: bin/circle: not found` — the directory
   was renamed to `WipeMud` but whatever was invoking autorun (cron/inittab,
   not present in this archive) kept the old `CircleMUD3` path. This
   failure recurs from May 13, 2003 through the last entry, **September 1,
   2005**, at 1-minute retry intervals in places — i.e. the game was very
   likely **dead in practice** from shortly after the upgrade, even though
   nobody turned anything off on purpose.
6. **2011–2013** — `welmar/DoC-code` contains a stub `fix_player.c` and an
   `htonl_test.c` — evidence you'd already started poking at player-file
   byte-order conversion previously, but `fix_player.c` as it stands doesn't
   actually do anything (it reads a buffer and prints its length, no fix
   logic). Root-level `CircleMUD3` is a separate, later (`Oct 2013` file
   dates) attempt to get the bpl20 code compiling on 64-bit Linux — it
   builds, but that's as far as it seems to have gone.

---

## 5. Player files: format, and the SPARC question

**I could not find any SPARC involvement anywhere in this archive.** Every
compiled `circle` binary I could fingerprint is 32-bit x86:

| Tree | Binary |
|---|---|
| `welmar/oldmud/CircleMUD3/bin/circle` | ELF 32-bit i386, FreeBSD 4.5 |
| `welmar/CircleMUD3/bin/circle` | ELF 32-bit i386, FreeBSD 5.4 |
| `welmar/WipeMud/bin/circle` | ELF 32-bit i386, FreeBSD 4.6.2 |
| `CircleMUD3/bin/circle` (root, your 2013 rebuild) | ELF 64-bit x86-64, Linux |

No `config.log`/`config.cache`/`config.status` anywhere mentions a
`sparc-sun-*` host triplet, and there's no SPARC binary or Solaris reference
in `.bash_history`/`.tcshrc`/`.zshrc` either. **x86 (FreeBSD) is little-endian,
same as the machine you'd revive this on**, so there is no byte-order
mismatch for the trees captured here.

I'm flagging this rather than just quietly overriding your memory — it's
possible there was an earlier hosting era on a Sun/SPARC box (Redbrick,
mentioned in `crontab.mud`'s `MAILTO=doc@redbrick.dcu.ie`, historically did
run Sun hardware for shell accounts) that predates everything captured in
this archive, or you might be thinking of a different project. Worth
double-checking your memory here — see the questions at the end.

**That said, the binary player files are still fragile for a different
reason.** The pre-3.1 trees (`welmar/oldmud`, `welmar/CircleMUD3`,
`welmar/testmud`) store players as a flat file of C structs
(`struct char_file_u`, defined in `src/structs.h:972`), written with a raw
`fwrite()` — one fixed-size record per player, no self-describing format:

```
struct char_file_u {
   char name[...], description[...], title[...];
   byte sex, chclass, level;
   sh_int hometown;
   time_t birth; int played;
   ...
   struct char_special_data_saved char_specials_saved;
   struct player_special_data_saved player_specials_saved;
   struct char_ability_data abilities;
   struct char_point_data points;
   struct affected_type affected[MAX_AFFECT];
   time_t last_logon;
   char host[...];
};
```

This is portable only insofar as the **compiler's struct packing/alignment,
`time_t`/`int` widths, and field order** exactly match what wrote it. Even
staying on x86, a different compiler version or a 32-bit-vs-64-bit `time_t`
can silently misalign this. Practical implication: **don't try to just
`fread()` these files with a modern 64-bit build** — reproduce the original
32-bit struct layout (or write a field-by-field converter) before trusting
the data. `welmar/CircleMUD3/lib/etc/players` (139KB) is the file that
matters most if you want the richer, pre-wipe roster; `players.beforewipe`
(1.9MB) is the even older, larger snapshot from before the first in-era wipe.
The fullest copy of the surrounding player data (aliases, saved objects, mail)
is in `welmar/CM3-beforeupg.tgz`.

By contrast, `welmar/WipeMud/lib/pfiles/*` (the post-upgrade files) are
**plain ASCII** (`Name: Zod`, `Pass: ...`, one field per line) — architecture-
independent, human-readable, and safe to carry forward as-is. The patch that
did this, `ascii_pfiles 2.1`, is sitting right there in `welmar/pfiles/` if
you want to see exactly what it changed, or apply the same patch to a bpl19
base to get the older/richer roster into the same safe text format.

---

## 6. What backup material exists, if you want the fullest possible restore

- **World data**: `welmar/world-backups/` (1,184 nightly tarballs, Sep–Dec
  2002 range surviving) and `welmar/zones/` (164 per-zone tarballs, matching
  live zone numbers) are your best source for reconstructing the world as it
  stood at a particular date, if the live `lib/world` in `welmar/CircleMUD3`
  turns out to be missing or corrupted anything.
- **Whole-tree backups**: `welmar/backups/` holds 8 dated full/src
  snapshots from Feb 2002 through Jan 2003, including one
  (`before-olc.tgz`) from *before* the OasisOLC patch was applied — useful
  if you ever want a "more stock" Disgracelands baseline.
- **The two upgrade-day snapshots**: `welmar/CM3-beforeupg.tgz` (full
  bpl20 tree, binary pfiles, richest player data) and
  `welmar/wipe-beforeupg.tgz` (full WipeMud tree at the moment of the 3.1
  cutover, already-wiped 9-character roster).

---

## 7. Baseline decision, and the stock-3.1 comparison behind it

**See `docs/non-stock-features.md` for the full, feature-by-feature
version of this section** — every `<DoC>`-tagged change in both trees,
read and characterized, not just counted. It confirms the "~21 dropped
mods" finding below was directionally right but understates it: several
of those aren't just untagged in `WipeMud`, the functionality is
genuinely gone (most notably the entire Paladin alignment mechanic and
all 7 new spells).

You asked me to check whether `WipeMud` actually carries local modifications
before treating it as "just stock 3.1" — good instinct, since a stock
final release would be a worse pick than the modded, well-loved bpl20 code.

**It has local mods, and they're substantial.** All four trees
(`oldmud`, `CircleMUD3`, `testmud`, `WipeMud`) use a consistent
`/* <DoC> */ ... /* </DoC> */` comment convention to bracket local changes
against upstream CircleMUD — this is presumably your own tagging convention
(it lines up with the `welmar/DoC-code/` directory name too). Counts of
tagged blocks:

| Tree | `<DoC>` blocks |
|---|---|
| `welmar/oldmud/CircleMUD3` (bpl19) | 32 |
| `welmar/CircleMUD3` (bpl20, pre-wipe) | 68 |
| `welmar/testmud` (bpl20) | ~same as CircleMUD3 |
| `welmar/WipeMud` (3.1) | 45 |

I pulled a stock CircleMUD 3.1 mirror
([Julio-Rats/CircleMUD](https://github.com/Julio-Rats/CircleMUD), which
identifies itself as an unofficial mirror of the 3.1 engine) and checked
`act.wizard.c`: real upstream 3.1 has **no** `do_remort` function and **no**
`<DoC>` tags anywhere. `WipeMud`'s copy has both — a full custom
remort/reincarnation system (`ACMD(do_remort)`, `pc_class_remort_masks[]`,
etc.), plus 44 other tagged local changes across `fight.c`, `limits.c`,
`interpreter.c`, `structs.h`, `comm.c`, `db.c`, `class.c`, and more. This is
not a stock release with your name on the directory — it's your actual
customized engine, current up to the point of the 2003 upgrade.

**One thing worth checking before you commit to this baseline**: comparing
per-file `<DoC>` counts between the pre-wipe `CircleMUD3` (68 blocks) and
`WipeMud` (45 blocks) shows a chunk of tagged mods that don't seem to have
survived the bpl20→3.1 merge — entire files that had local mods in
`CircleMUD3` show **zero** `<DoC>` tags in `WipeMud`:

| File | CircleMUD3 mods | WipeMud mods |
|---|---|---|
| `spell_parser.c` | 7 | **0** |
| `spec_procs.c` | 4 | **0** |
| `magic.c` | 3 | **0** |
| `spells.c` | 2 | **0** |
| `spells.h` | 2 | **0** |
| `act.item.c` | 1 | **0** |
| `spec_assign.c` | 1 | **0** |
| `utils.c` | 1 | **0** |

That's roughly 21 blocks of custom spell/spec-proc behavior that may have
been dropped (or folded in untagged, or genuinely superseded) during the
upgrade — worth a manual diff of those specific files against `CircleMUD3`
before going live, in case something you cared about (a custom spell, a
special-procedure hook) quietly didn't make the jump. `WipeMud` also gained
a few new tagged spots `CircleMUD3` didn't have (`db.c`, `screen.h`,
`sysdep.h` — 4 blocks total), presumably added specifically for the 3.1
merge or the ascii-pfiles conversion.

**Bottom line, per your rule ("if it's got a bunch of local mods from me,
use WipeMud and port accounts across"): use `welmar/WipeMud` as the engine,
restore/port the player roster and world from pre-wipe `welmar/CircleMUD3`**,
and budget some time to manually reconcile the ~21 spell/spec-proc mods
above rather than assume they carried forward silently.

---

## 7a. Remaining open questions — I need your call on these

1. **Am I right that there's no SPARC machine in this archive's history?**
   Everything I can fingerprint is FreeBSD/x86. If you specifically remember
   a Sun/SPARC box being involved, it may predate what's preserved here —
   worth flagging in case there's a tarball or backup I haven't located
   (e.g. off in `Maildir`, or a machine name I should search for).
2. **Do you want the reference area libraries** (`welmar/zones1`,
   `welmar/areas` — Merc/ROM/SMAUG/etc. area packs) **kept**, or are they
   just clutter from past building sessions that can be archived/dropped
   from a revival?
3. **Where do you want to host the revival?** (local Docker/VM, a VPS, etc.)
   This affects the concrete steps in §8 but I don't want to assume.

---

## 8. Proposed steps to revive Disgracelands

Baseline is decided (§7): **`welmar/WipeMud` as the code, `welmar/CircleMUD3`
(pre-wipe) as the world/player data source.** Shape of the work:

1. **Reconcile the ~21 dropped `<DoC>` mods** (§7 table — `spell_parser.c`,
   `spec_procs.c`, `magic.c`, `spells.c`/`.h`, `act.item.c`,
   `spec_assign.c`, `utils.c`) by diffing those files between `CircleMUD3`
   and `WipeMud` before doing anything else, so you know upfront whether
   anything you'd notice missing (a custom spell, a special-procedure
   hook) needs to be manually re-applied to `WipeMud`.
2. **Get `WipeMud` compiling on a modern OS.** The existing binaries are all
   FreeBSD/i386; your 2013 attempt at the *bpl20* codebase (root
   `CircleMUD3`) shows this lineage *can* be coaxed onto modern 64-bit
   Linux with a few `conf.h` tweaks (`crypt.h` detection, `socklen_t`).
   `WipeMud` hasn't had that attempt made yet — re-run `./configure` and
   expect similar small portability fixes (K&R-style prototypes,
   `sprintf`/`strcpy` warnings-as-errors on modern GCC/Clang, `bool`/`byte`
   typedef clashes with system headers).
3. **Port the player accounts across**: `WipeMud` already uses the portable
   `ascii_pfiles` text format, so the job is converting `CircleMUD3`'s
   binary `players` (139KB, or `players.beforewipe`, 1.9MB, if you want the
   even older/richer roster) into that same text format, not the other way
   around. Write (or finish `welmar/DoC-code/fix_player.c`) a small
   standalone C program that `#include`s the exact `structs.h` from
   `welmar/CircleMUD3/src` (compiled 32-bit, matching struct layout),
   reads each fixed-size `struct char_file_u` record, and re-emits it in
   `ascii_pfiles` format. Do this offline, verify a handful of characters
   by hand (e.g. `Zod`/`humbug`, who exist in both eras) before trusting
   the bulk conversion. Cross-check `welmar/CircleMUD3/lib/plralias` and
   `lib/plrobjs` (equipment, split by name-range like the pfiles) alongside
   the player database — they need to travel together, and their format
   (name-range subdirectories) is unchanged between bpl20 and 3.1.
4. **Restore world data** from `welmar/CircleMUD3/lib/world` (or a specific
   dated tarball from `welmar/world-backups`/`welmar/zones` if you want a
   particular remembered snapshot rather than the latest) into `WipeMud`'s
   `lib/world` — diff `WipeMud`'s own (post-wipe) `lib/world` against it
   first in case any building happened after the upgrade that you'd want to
   keep.
5. **Fix the autorun/cron path bug** that likely killed the mud in the first
   place (§4 step 5) before relying on autorun for uptime.
6. **Smoke-test** in a scratch copy first (mirroring how `testmud` was used
   originally) before pointing anything at the "real" data.

---

## 9. Notes on things that turned out to be red herrings

- `circle-3.1-w-goodies.tar.gz` — not a real archive (see §2).
- `tarfiles/CurrentVersion.tar` — MudOS, not CircleMUD.
- `welmar/mud@136.206.15.21` — byte-identical in size/timestamp to
  `welmar/wipe-beforeupg.tgz`; almost certainly the same backup mailed out
  under a different filename, not a separate snapshot.
- `submissions/trust.patch` (a trust-level command patch) — no evidence it
  was ever applied to any of the live trees; `structs.h` shows no trust
  field in any of them. Treat as an unapplied submission only.

---

## 10. Reborn — what's actually been built

A new `Reborn/` directory (repo root, sibling of `welmar/`) now exists as
its own git repository, seeded from `welmar/CircleMUD3` per §7's decision.
Current state, across 4 commits:

1. **Imported and cleaned up**: stripped the stale `lib.prewipe` snapshot,
   FreeBSD-only prebuilt binaries, and log/syslog contents that don't
   belong in source control.
2. **Compiles clean on modern Linux** (GCC 15.2.0, x86-64). Two things
   were needed: `interpreter.c` had `constants.h` included before
   `structs.h` (every other file gets this right; a genuine latent bug in
   the original code, now fatal under a strict compiler — fixed by
   reordering) and both `./configure` and `make` need
   `CFLAGS="-std=gnu89 -fcommon -Wno-implicit-function-declaration -w"`
   to tolerate this code's 2002-era K&R-ish style. Documented in
   `Reborn/BUILDING.md`.
3. **Runs and serves the real Disgracelands banner.** Booted locally,
   loaded all 55+ zones (including "King Welmar's Castle" — almost
   certainly where the `welmar` account/directory name actually comes
   from, nothing SPARC-related), and served the login prompt over a raw
   socket connection.
4. **Player-file conversion built and verified**: `Reborn/tools/bin2ascii.c`
   converts the real 108-character binary database into the
   `ascii_pfiles` text format (must be built 32-bit — see below);
   `Reborn/tools/pfiledump.c` reads ascii pfiles back. Both a genuine
   WipeMud-produced ascii pfile and a freshly-converted CircleMUD3 one
   parse cleanly; all 108 converted characters pass a clean sweep.
   Full detail in `Reborn/docs/pfile-conversion.md`.

**A concrete non-SPARC portability trap turned up along the way**: `struct
char_file_u` has several `long`-typed fields (`idnum`, `act`, `affected_by`,
some `player_special_data_saved` spares). `long` is 4 bytes on the original
32-bit FreeBSD build and 8 bytes on a native 64-bit Linux build — so a
naive 64-bit `fread()` of the old binary file silently misreads everything
past the first `long` field. This is an ILP32-vs-LP64 struct-layout
mismatch, not byte order, and it would have bitten on a 64-bit FreeBSD
rebuild too, SPARC or not. `bin2ascii` is deliberately built 32-bit to
avoid it, and the resulting ascii files are architecture-independent going
forward.

**Not done yet** (see `Reborn/docs/pfile-conversion.md` for the honest
scope note): the live game binary still reads/writes the old binary format
at runtime — the conversion above is a proven-correct one-shot offline
migration, not yet wired into `db.c`'s actual login/save path. That's
real, password-adjacent work spanning several files
(`comm.c`, `interpreter.c`, `act.wizard.c`, `house.c`, `mail.c`, the
`src/util/` tools), and was deliberately left as documented follow-up
rather than rushed. `Reborn`'s own `db.c` has zero local `<DoC>` mods
(§7), so it's one of the lower-risk files to eventually patch.
