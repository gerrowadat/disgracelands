# Documentation

Three kinds of document live here, split by directory.

## `docs/` — running the Go server

Operator documentation for the thing being built. Present tense: what it
does, how to configure it, how to run it.

- **[configuration.md](configuration.md)** — every flag and environment
  variable, what it does, and what the defaults are.
- **[operations.md](operations.md)** — starting it, containers, graceful
  shutdown, health and readiness, metrics, logs, backups, exposure, and
  getting started from an original CircleMUD directory: `dlctl import`,
  which is the only path from an archived `lib/` to a directory this
  server runs on, and `dlctl verify --against` for checking the
  conversion lost nothing.
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
> write-up in `design/go-port-plan.md` covers what got built instead).
> `configuration.md` and `operations.md` mark the handful of settings
> still *(inert)* for reasons of their own, unrelated to phase status.

## `docs/proposals/` — the plan, still moving

What is going to happen. One document at a time, and it is empty between
plans rather than accumulating them.

**It is empty, as of 2026-08-30.** All three documents that lived here
have moved to `docs/design/`: `go-port-plan.md` because Phases 0–6 are
built and what is left of its Phase 7 is two deployment *decisions*
rather than work; `yaml-only.md` because every row of its §7 landed,
ending with **v1.0.0**; and `idiomatic-go.md` because seven of its nine
steps are built and the two the plan itself allowed to end otherwise did
— step 7 deferred, step 8 declined, both with reasons recorded.

So there is no forward plan right now, and that is a real state rather
than a gap. The work in front of anyone picking this up is the **open
GitHub issues**, and `deviations.md`'s "Not deviations — gaps still to
fill" and "What the session-parity suite found", which is where issues
get filed from. Read those as the todo list. The next thing to live here
is whatever somebody argues for next.

## `docs/design/` — decisions that landed

A design document moves here from `docs/proposals/` once the thing it
describes is built rather than planned — present tense, the way `docs/`
itself is, but explaining *why* the shipped shape is what it is rather than
just how to run it. Still corrected when reading the C or building the next
piece finds something the original document got wrong; not rewritten to
hide that it started as a proposal.

- **[go-port-plan.md](design/go-port-plan.md)** — the design and phasing
  for reimplementing the engine in Go: 64-bit safety, pluggable player-
  and world-file formats, the concurrency model, licensing constraints,
  and the phase-by-phase sequence. Phases 0–5 are built and marked done
  with what each one did and did not contain, Phase 5's own slices and
  gaps listed command by command; Phase 6 (OasisOLC) was decided against,
  in favour of reloading edited world data into a running server without a
  restart. **Phase 7 (cutover) never started and is not going to start as
  written**: its remaining preconditions are a hosting decision and a
  roster decision, both carried in `TODO.md`. Authoritative for the
  architecture and for what each phase did and did not contain; not
  extended with new plans. Moved here 2026-08-30 — its header says what
  that move does and does not claim.
- **[yaml-only.md](design/yaml-only.md)** — retiring `classic`, `ascii`
  and `binary` from the server, so it understands only `yaml`: what
  leaked, what "the conversion is exactly dead on" has to mean and how it
  got proved, the compatibility corpus and fuzz targets it built, and the
  rule for yaml fields the legacy formats cannot source. Breaking, and
  released as v1.0.0: the only route from a CircleMUD `lib/` to a running
  server is `dlctl import`. Every row of its §7 has landed. **§5 and §6
  are the live half** — the compatibility contract every new field and
  every new format test is still held to, and the fence `idiomatic-go.md`
  is drawn against. Its §7 table is worth reading for what each row turned
  out to be about; several were about something other than what they were
  written to be about, and the bugs they found are listed there.
- **[idiomatic-go.md](design/idiomatic-go.md)** — retiring the C's data
  model from the server's *memory*, the way `yaml-only.md` retired its
  formats from the server's disk: one bit-vector type standing in for
  thirteen unrelated flag domains, `int32` standing in for ten
  enumerations, `Values[4]` whose slots mean something different depending
  on slot zero, `-1` meaning "absent", and an object's five where-is-it
  fields. Seven of nine steps built; **step 7 deferred and step 8
  declined**, both on the terms the plan itself set. Its §5 — the rule
  that no step may weaken a C-derived test — is the half worth reading
  first, and the half that earned its place: step 6 merged two name
  tables and step 3 landed a rule change that every suite passed. Its
  status block lists what the document turned out to be wrong about,
  which is the point of keeping it.
- **[data-format.md](design/data-format.md)** — a single yaml format for
  everything a lib-dir holds: the world, players, boards, mail, houses and
  the game tuning that used to be compiled into `config.c`. Replaces the
  eight unrelated formats a CircleMUD `lib/` carries, and is a superset of
  all of them. YAML over a JSON data model, one file per zone, one file
  per player, vnums unchanged. It is the *only* format the server reads
  now (§11 step 7). One piece of the document is not built, and is
  declined rather than pending: the `classic` export writer
  (`yaml-only.md` §0.3). §11 tracks the rest.
- **[data-format-versioning.md](design/data-format-versioning.md)** — the
  `.dlversion` stamp a data directory carries: the `major.minor.patch`
  release of the `dlctl` that wrote it. A *differing* major refuses to
  boot in either direction, a differing minor is "own risk" and `dlctl
  data version` reports it, a differing patch is silent by construction.
  Layered on top of data-format.md §10.1's own per-file
  `schema: dl/<kind>@<major>` tag, which says what shape one file is in
  rather than what release the whole directory was last written by.
- **[signal-handling.md](design/signal-handling.md)** — what each signal
  does and why: `SIGTERM`/`SIGINT` shutdown and its ordering contract,
  `SIGHUP` as a reload rather than the C's "die", `SIGUSR1`/`SIGUSR2` from
  `signal_setup`, `SIGQUIT` left unhandled so the runtime dumps stacks, and
  exit codes in place of `autorun`'s `.killscript` files. Also the line this
  port draws between the files a signal may reload and the world data only
  an in-game command may. The dispatcher, the shutdown signals, the
  configuration reload and the exit codes are built; §9 tracks the rest.

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
- **[lib-directory-format.md](investigations/lib-directory-format.md)** —
  every file in the real archived `lib/`: what writes it, what reads it,
  and what its format actually is as opposed to what it is supposed to be.
  The empirical companion to `design/data-format.md`, which was written
  against the *stock* `lib/` and a partial survey; §9 says which parts
  should reach the Go server's data directory and corrects where that
  document guessed wrong.

## Not this directory

- **`reference/moderncserver/doc/`** (singular) is the original stock
  CircleMUD documentation — building, coding, running, wizhelp. Upstream
  reference material, kept as-is, not written by this project.
- **`reference/`** holds code-only snapshots of the two other
  Disgracelands-lineage codebases, for comparison during porting.
