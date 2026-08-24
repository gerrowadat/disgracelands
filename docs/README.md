# Documentation

Three kinds of document live here, split by directory.

## `docs/` — running the Go server

Operator documentation for the thing being built. Present tense: what it
does, how to configure it, how to run it.

- **[configuration.md](configuration.md)** — every flag and environment
  variable, what it does, and what the defaults are.
- **[operations.md](operations.md)** — starting it, containers, graceful
  shutdown, health and readiness, metrics, logs, backups, exposure, and
  getting started from an original CircleMUD directory: `dlctl convert`
  into the classic/ascii shapes the server already runs on by default, or
  `dlctl lib import` straight into `yaml`.
- **[developer.md](developer.md)** — working on the port: the `Makefile`'s
  dev targets, getting a server running locally against a tiny world or a
  throwaway data directory, poking at it by hand, and what to run before
  pushing.
- **[deviations.md](deviations.md)** — every intentional difference from the
  C server's behaviour, with the C line reference and the reasoning. The
  other half of the fidelity decision: it is what keeps "fixed a bug"
  distinguishable from "accidentally changed the game". Since 2026-08-23
  that decision covers gameplay and compatibility rather than the whole
  implementation — see the file's own header and the plan's §0.
- **[weirdnumbers.md](weirdnumbers.md)** — the catalogue of places where
  CircleMUD's arithmetic does not do what it appears to. Truncation nobody
  intended, comments describing numbers the code never produced, and the
  reasons several of them are reproduced rather than corrected. Read before
  porting anything with a division in it.

Building from source is in the top-level **`BUILDING.md`**, which covers
both the C tree and the Go tree.

> **Every slice of Phase 5 is built.** Login and creation, the main menu, a
> world that resets and mobiles that act, combat, spells, skills, affects,
> equipment, containers, food and drink, following and grouping; special
> procedures, the channels and socials, shops, banks, rent, boards, mail,
> houses, the immortal commands and remorting. 310 of the C's 318 commands
> answer, and the plan's §10 lists the other 8 one by one — seven OasisOLC
> editors and `slowns`, both declined rather than pending (Phase 6's own
> write-up in `proposals/go-port-plan.md` covers what got built instead).
> `configuration.md` and `operations.md` mark the handful of settings
> still *(inert)* for reasons of their own, unrelated to phase status.

## `docs/proposals/` — the plan, still moving

The phase order, still future tense for whatever has not landed yet. Partly
a record now: each finished phase carries a note of what it actually
contained, what it did not, and what reading the C changed about the plan.
The unfinished phases are still expected to change. §0's own "Fidelity,
phase two" is the exception to "still future tense" — it's a decision
already taken, dated 2026-08-23, about how work from here relates to the C.

- **[go-port-plan.md](proposals/go-port-plan.md)** — the design and phasing
  for reimplementing the engine in Go: 64-bit safety, pluggable player- and
  world-file formats, the concurrency model, licensing constraints, and the
  phase-by-phase sequence. Phases 0–5 are built and marked done with what
  each one did and did not contain, Phase 5's own slices and gaps listed
  command by command; Phase 6 (OasisOLC) was decided against, in favour of
  reloading edited world data into a running server without a restart —
  its own write-up covers what that became instead; Phase 7 (cutover) has
  not started.
- **[signal-handling.md](proposals/signal-handling.md)** — what each signal
  does and why: `SIGTERM`/`SIGINT` shutdown and its ordering contract,
  `SIGHUP` as a reload rather than the C's "die", `SIGUSR1`/`SIGUSR2` from
  `signal_setup`, `SIGQUIT` and the liveness probe as the replacement for
  the C's deadlock watchdog, and exit codes in place of `autorun`'s
  `.killscript` files. Also the line this port draws between files a signal
  may reload and world data only an in-game command may.

## `docs/design/` — decisions that landed

A design document moves here from `docs/proposals/` once the thing it
describes is built rather than planned — present tense, the way `docs/`
itself is, but explaining *why* the shipped shape is what it is rather than
just how to run it. Still corrected when reading the C or building the next
piece finds something the original document got wrong; not rewritten to
hide that it started as a proposal.

- **[data-format.md](design/data-format.md)** — a single yaml format for
  everything a lib-dir holds: the world, players, boards, mail, houses and the
  game tuning still compiled into `config.c`. Replaces the eight unrelated
  formats a CircleMUD `lib/` carries, and is a superset of all of them. YAML
  over a JSON data model, one file per zone, one file per player, vnums
  unchanged. Everything but game configuration and the `classic` export
  writer is built; §11 tracks the rest.
- **[data-format-versioning.md](design/data-format-versioning.md)** — the
  yaml format's own `major.minor.patch` stamp: a major bump refuses to
  boot, a minor bump is "own risk" and `dlctl data version` reports what
  changed, a patch bump is silent by construction. Layered on top of
  data-format.md §10.1's own per-file `schema: dl/<kind>@<major>` tag,
  which says what shape one file is in rather than what release the whole
  directory was last written by.

## `docs/investigations/` — what we found out

Archaeology. This project is a revival of a MUD played 2001–2008, seeded
from an archive of the original server, and a great deal of what is known
about that codebase had to be established by investigation rather than
read off a document. These are the write-ups, with the evidence.

They describe what *was* true of the original, and they are not updated as
the port progresses.

- **[circlemud-archive-report.md](investigations/circlemud-archive-report.md)**
  — the full investigation of the original archive: which of the several
  surviving trees was the one actually played, how that was determined, and
  the real-world timeline that follows from it.
- **[history.md](investigations/history.md)** — best-guess timeline of how
  Disgracelands got from 2001 to here.
- **[non-stock-features.md](investigations/non-stock-features.md)** —
  everything in the codebase that is not stock CircleMUD, established two
  independent ways. The definitive list of what a faithful port has to
  reproduce.
- **[pfile-conversion.md](investigations/pfile-conversion.md)** — the
  binary→ascii player-file conversion tools, and what was actually verified
  with them.
- **[ascii-pfile-format.md](investigations/ascii-pfile-format.md)** — the
  ascii_pfiles format, field by field. Read as a specification rather than
  a finding: it is what the Go implementation is written against.

## Not this directory

- **`reference/moderncserver/doc/`** (singular) is the original stock
  CircleMUD documentation — building, coding, running, wizhelp. Upstream
  reference material, kept as-is, not written by this project.
- **`reference/`** holds code-only snapshots of the two other
  Disgracelands-lineage codebases, for comparison during porting.
