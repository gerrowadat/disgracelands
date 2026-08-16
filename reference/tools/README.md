# reference/tools/

Revival-specific C tooling — written for this project, not part of any
original CircleMUD or Disgracelands tree.

That is what distinguishes it from its neighbours:

- **`../moderncserver/src/util/`** — the original CircleMUD-era utilities
  (`autowiz`, `mudpasswd`, `listrent`, etc.) that shipped with the game and
  build via `make utils`.
- **`../CircleMUD3-src/`, `../WipeMud-src/`** — unmodified snapshots of the
  other lineage codebases, kept for comparison.

Nothing here is wired into `../moderncserver/src/Makefile.in`. Each tool
documents its own build command in its header comment, because some — like
`bin2ascii` — have requirements (32-bit compilation) that do not fit the
normal build.

**These are superseded by Phase 2 of the Go port**, which reimplements both
as `dlctl` subcommands needing no 32-bit toolchain. They stay until that is
proven against the same 108 records.

## What's here

- **`bin2ascii.c`** — converts the classic binary player database
  (`struct char_file_u` records) into the portable `ascii_pfiles` text
  format. Must be built 32-bit — see the comment at the top of the file
  for why. Full writeup: `../docs/investigations/pfile-conversion.md`.
- **`pfiledump.c`** — reads and prints any `ascii_pfiles`-format player
  file. Ordinary native build.
