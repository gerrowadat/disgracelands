# Documentation

Three kinds of document live here, split by directory.

## `docs/` — running the Go server

Operator documentation for the thing being built. Present tense: what it
does, how to configure it, how to run it.

- **[configuration.md](configuration.md)** — every flag and environment
  variable, what it does, and what the defaults are.
- **[operations.md](operations.md)** — starting it, containers, graceful
  shutdown, health and readiness, metrics, logs, backups, exposure.
- **[developer.md](developer.md)** — working on the port: the `Makefile`'s
  dev targets, getting a server running locally against a tiny world or a
  throwaway data directory, poking at it by hand, and what to run before
  pushing.
- **[deviations.md](deviations.md)** — every intentional difference from the
  C server's behaviour, with the C line reference and the reasoning. The
  other half of the fidelity decision: it is what keeps "fixed a bug"
  distinguishable from "accidentally changed the game".
- **[weirdnumbers.md](weirdnumbers.md)** — the catalogue of places where
  CircleMUD's arithmetic does not do what it appears to. Truncation nobody
  intended, comments describing numbers the code never produced, and the
  reasons several of them are reproduced rather than corrected. Read before
  porting anything with a division in it.

Building from source is in the top-level **`BUILDING.md`**, which covers
both the C tree and the Go tree.

> The Go server is at the end of Phase 5, bar one slice. Login and creation,
> the main menu, a world that resets and mobiles that act, combat, spells,
> skills, affects, equipment, containers, food and drink, following and
> grouping; special procedures, the channels and socials, shops, banks, rent,
> boards, mail, houses, and the immortal commands. 285 of the C's 318 commands
> answer; `remort` is the slice still open and the plan's §10 lists the other
> 32 one by one, along with three mechanisms — minimum position, `CAN_SEE` and
> `N.thing` targeting — that belong to every command rather than to any of
> them. `configuration.md` and `operations.md` mark which settings are
> *(inert)* pending later phases.

## `docs/proposals/` — the design, and the plan

The design decisions and the phase order. Partly a record now: each finished
phase carries a note of what it actually contained, what it did not, and what
reading the C changed about the plan. The unfinished phases are still future
tense and still expected to change.

- **[go-port-plan.md](proposals/go-port-plan.md)** — the design and phasing
  for reimplementing the engine in Go: 64-bit safety, pluggable player- and
  world-file formats, the concurrency model, licensing constraints, and the
  phase-by-phase sequence. Phases 0–4 are built and marked done with what
  each one did and did not contain; Phase 5 is mapped into slices, all but one
  of them finished, with its gaps listed command by command; 6 and 7 are not
  started.
- **[data-format.md](proposals/data-format.md)** — a single native format
  for everything in `data/`: the world, players, boards, mail, houses and
  the game tuning still compiled into `config.c`. Replaces the eight
  unrelated formats a CircleMUD `lib/` carries, and is a superset of all of
  them. YAML over a JSON data model, one file per zone, one file per
  player, vnums unchanged. Nothing here is built.

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
