# Disgracelands: a best-guess real-world timeline

This is a reconstruction, not a record — nobody kept a changelog of "what
ran when." It's built entirely from log timestamps, backup filenames,
binary version strings, and file contents found across the archive
(`welmar/` and siblings, one level up from this repo). Where the evidence
is thin, that's said explicitly. Corrections welcome — this is a first
pass, done 2026-08-15.

Everything here refers to paths in the **archive**
(`/home/doc/git/disgracelands/welmar/...`), not this repo — `Reborn` only
carries the code, deliberately not the historical logs/backups it's built
from. See `circlemud-archive-report.md` for the full investigation
this is distilled from.

## Timeline

### Pre-2001ish — the earliest surviving install

`welmar/oldmud/CircleMUD3` runs CircleMUD 3.0 bpl19 on FreeBSD 4.5/i386.
Its own `syslog` is tiny (~50KB) and its `log/restarts` is short — this
looks like either a short-lived early install, or (more likely) a tree
that was periodically archived/reset and this is just what survived from
one such reset point. No hard start date found; bpl19 itself was released
January 23, 2000, which is the earliest this tree could be.

### ~2001 (or earlier) – 2008 — the real, long-running Disgracelands

`welmar/CircleMUD3` (bpl20, patched with OasisOLC + DG Scripts + extensive
local `<DoC>`-tagged modifications — see the archive report §7) is the
tree that was actually played continuously for years:

- `welmar/CircleMUD3/log/restarts` has 353 entries. The earliest visible
  ones are dated "Aug 27" with no year printed by the log format itself,
  but `welmar/deleted` (a one-line note file at the account root) is
  timestamped **Mon Aug 27 17:48:16 IST 2001** — almost certainly the same
  event or close to it, which would put the start of this era at **August
  2001**.
- The binary player database `lib/etc/players` (108 characters, decoded
  in the archive report §5) has birth/last-login timestamps running from
  **March 2006 to April 2008**. `lib/etc/players.beforewipe` (1.9MB, much
  larger) represents an *even earlier* population that existed before a
  wipe that happened at some point within this same bpl20 era — i.e.
  there were at least two "generations" of characters on this one tree,
  not one continuous roster the whole time.
- `welmar/CircleMUD3/log/levels` shows real level-up activity as late as
  **March 16 2008** (`Halenger` reaching level 30, `Lofasz` reaching level
  26 that same week).
- Session logs (`syslog`) show players connecting via `redbrick.dcu.ie`
  shell hosts as a jump box — `minerva`, `deathray`, `murphy` all appear
  as client hostnames. Redbrick (Dublin City University's student-run
  Internet society) is well known for having run Sun/SPARC hardware for
  shell accounts historically. **This is almost certainly the source of
  any memory of "SPARC" being involved** — on the client/shell-account
  side, not on `welmar` itself, which fingerprints as FreeBSD/i386
  throughout (see archive report §5). Not proven, but it's the only
  SPARC-adjacent fact found anywhere in the archive.
- `welmar/scripts/crontab.mud` (nightly world backup + a multi-play
  checker cron job) references `~mud/CircleMUD3/lib/world` — consistent
  with `CircleMUD3` being the actively-administered tree, at least for
  whatever period that crontab was live. The world-backup tarballs it
  produced (`welmar/world-backups/`) only survive for **Sept–Dec 2002**,
  which either means the cron job stopped being useful/was disabled
  around then, or (more likely, given the game kept running for years
  after) later backups were pruned/rotated and didn't survive into this
  archive.
- `welmar/backups/` holds full-tree snapshots dated through **January
  2003** (`mud-src-20030125.tgz`), the last dated backup before the
  upgrade attempt below.

### May 13, 2003 — the CircleMUD 3.1 upgrade attempt ("WipeMud")

`welmar/CM3-beforeupg.tgz` and `welmar/wipe-beforeupg.tgz` are both
timestamped this exact day, minutes apart — a snapshot of `CircleMUD3`
immediately before, and the newly-renamed `WipeMud` immediately after, an
upgrade to CircleMUD 3.1 (the final official CircleMUD release, out
November 18, 2002). The name is on the nose: **this upgrade wiped the
character roster**. `welmar/WipeMud/lib/pfiles/` (now converted to the
portable ascii format via the `ascii_pfiles 2.1` patch) holds only 9
characters, identical between the "before wipe" backup and what's still
on disk — i.e. nothing was lost *after* that point, but the multi-year
population living in `CircleMUD3` was not carried over.

`welmar/WipeMud/log/restarts` has entries only through **May 13, 2003**.
Its own live `syslog` is empty. Whatever this branch was meant to become,
the evidence says it was used only briefly (if that) before being
abandoned in favor of continuing to run `CircleMUD3` — which is exactly
what the player-database dates above show happening for years afterward.

**Best guess: the 3.1 upgrade was tried, didn't stick (for reasons not
recorded here — could be anything from "everyone hated starting over" to
a technical problem), and `CircleMUD3` just kept being the real game.**

### 2003–2005 — a stale autorun path, probably harmless

`welmar/syslog` (the root autorun wrapper log, 124MB) shows repeated
failures from May 13, 2003 through September 1, 2005:
`/home/mud/CircleMUD3/autorun.sh: bin/circle: not found`. Read in
isolation this looked (in an earlier draft of the archive report) like
evidence the whole mud died in 2003. **It wasn't** — `CircleMUD3`'s own
internal logs prove it kept running and being played until at least 2008.
The likely explanation: whatever mechanism produced this specific log
(a cron entry or `inittab` line, not preserved in the archive) still
pointed at a stale path after the `CircleMUD3` → `WipeMud` rename, and
kept retrying uselessly in the background while the actual game was
started and administered some other way (manually, or via a different
script not captured here). Worth treating as an unresolved loose end
rather than a solved mystery.

### ~2008 onward — the trail goes cold

The last dated evidence of real play is March 2008 (`log/levels`). No
restart, syslog, or player-database evidence has been found past that
point. Whether the game kept running quietly with nobody leveling up, or
actually stopped around then, isn't determinable from what's preserved.

### 2011–2013 — archival, and a first revival attempt

- `c.tgz` (a backup of the `CircleMUD3` source+world tree) has a gzip
  timestamp of **July 8, 2011**.
- The root-level `CircleMUD3` directory (sibling of `welmar/` in the
  archive) is a separate rebuild attempt of the bpl20 codebase, compiled
  as a native **64-bit Linux** binary — its file timestamps cluster around
  **October 21, 2013**, matching most of the rest of the account's dotfiles
  and mail archives in the same snapshot. This looks like an earlier
  attempt (by the account owner) to get the old code building on a modern
  machine, which succeeded in producing a working binary but doesn't
  appear to have gone further than that.
- October 21, 2013 is the modification date on nearly everything in this
  archive — almost certainly the date the whole account was tarred up and
  preserved, marking the effective end of Disgracelands' active life and
  the start of it being "an archive" rather than "a server."

### 2026 — this revival

`Reborn/` (this repository) is seeded from `welmar/CircleMUD3` — the tree
that was actually played 2001–2008 — rather than the more "final" but
short-lived `WipeMud`/3.1 branch. See `circlemud-archive-report.md`
§7 for why, and `TODO.md` for what's left to do.
