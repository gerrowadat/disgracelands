# tools/

Modern, revival-specific tooling — code written for this project itself,
not part of the original CircleMUD/Disgracelands codebase.

This is distinct from:

- **`../src/util/`** — the original CircleMUD-era utilities
  (`autowiz`, `mudpasswd`, `listrent`, etc.) that shipped with the game
  itself and build via `make utils`.
- **`../reference/`** — unmodified-by-us snapshots of the other
  Disgracelands-lineage codebases, kept for comparison.

Nothing here is wired into `src/Makefile.in` — each tool documents its
own build command in its header comment, since some (like `bin2ascii`)
have unusual requirements (32-bit compilation) that don't fit the normal
build.

## What's here

- **`bin2ascii.c`** — converts the classic binary player database
  (`struct char_file_u` records) into the portable `ascii_pfiles` text
  format. Must be built 32-bit — see the comment at the top of the file
  for why. Full writeup: `../docs/investigations/pfile-conversion.md`.
- **`pfiledump.c`** — reads and prints any `ascii_pfiles`-format player
  file. Ordinary native build.
