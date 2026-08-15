# Documentation

Three kinds of document live here, split by directory.

## `docs/` — running the Go server

Operator documentation for the thing being built. Present tense: what it
does, how to configure it, how to run it.

- **[configuration.md](configuration.md)** — every flag and environment
  variable, what it does, and what the defaults are.
- **[operations.md](operations.md)** — starting it, containers, graceful
  shutdown, health and readiness, metrics, logs, backups, exposure.

Building from source is in the top-level **`BUILDING.md`**, which covers
both the C tree and the Go tree.

> The Go server is at Phase 0: it boots, reports itself ready, serves
> diagnostics and shuts down cleanly, but there is no game in it yet. Both
> documents above mark which settings are *(inert)* pending later phases.

## `docs/proposals/` — work not yet done

Designs and plans. Future tense, and expected to change.

- **[go-port-plan.md](proposals/go-port-plan.md)** — the design and phasing
  for reimplementing the engine in Go: 64-bit safety, pluggable player- and
  world-file formats, the concurrency model, licensing constraints, and the
  phase-by-phase sequence. Phase 0 is built; Phases 1–7 are not.

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

- **`doc/`** (singular) is the original stock CircleMUD documentation —
  building, coding, running, wizhelp. Upstream reference material, kept
  as-is, not written by this project.
- **`reference/`** holds code-only snapshots of the two other
  Disgracelands-lineage codebases, for comparison during porting.
