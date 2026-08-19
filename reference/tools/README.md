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

There are two kinds of thing here.

## Oracles

These exist to be compiled *by the Go tests* and compared against. Each one
holds original C function bodies, lifted with the `char_data` dereferences
replaced by the plain values they would have returned and nothing else
changed, wrapped in a `main()` that dumps the answers across an input range.

The reason for them is `docs/weirdnumbers.md`: CircleMUD's arithmetic is
regularly not what it appears to be, and a formula read across into Go
produces something that looks right, passes a plausibility check, and is
wrong. Every oracle written so far has caught at least one real mistake.

- **`randoracle.c`** — `circle_random`/`circle_srandom` and `number()`, from
  `random.c`. Checked against `internal/rng` over 5,000 draws from
  each of six seeds, including `number()`'s modulo bias.
- **`fightoracle.c`** — `compute_thaco` and `hit()`'s position multiplier,
  from `fight.c` and `class.c`. Two `int -= double` compound assignments that
  truncate separately, and an integer-division multiplier whose own comment
  describes different numbers from the ones it produces. Checked over
  1,512,000 combinations.
- **`regenoracle.c`** — `hit_gain`, `mana_gain` and `move_gain` from
  `limits.c`, four truncating divisions apiece. Checked over 36,288
  combinations of age, position, class, hunger and poison.
- **`cryptoracle.c`** — the system `crypt(3)`, for checking the hand-written
  DES in `internal/auth/descrypt` against. Skips where the system libcrypt no
  longer does traditional DES.

If you are about to port anything with a division, a cast, or a comment
describing numbers in it, the next file in this directory is probably the one
you are about to write. `docs/developer.md` has the pattern.

## Player-file tooling

Written during the archive investigation, before the Go port could read the
binary format. **Superseded by `dlctl`'s `pfile` subcommands**, which need no
32-bit toolchain; they stay because they are what the findings in
`docs/investigations/pfile-conversion.md` were established with.

- **`bin2ascii.c`** — converts the classic binary player database
  (`struct char_file_u` records) into the portable `ascii_pfiles` text
  format. Must be built 32-bit — see the comment at the top of the file for
  why.
- **`pfiledump.c`** — reads and prints any `ascii_pfiles`-format player file.
  Ordinary native build.
- **`pfilegen.c`** — writes a synthetic binary player database with known
  values, so the Go decoder can be tested against a file the C actually
  wrote rather than one the Go encoder produced.
- **`pfilelayout.c`** — reports the on-disk layout of `struct char_file_u`:
  field offsets, sizes and padding, which is what the Go codec is written
  against.
