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

There are three kinds of thing here.

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
- **`shopprice.c`** — `buy_price` and `sell_price` from `shop.c`, which are one
  line each and whose answer depends on the *width the multiplication happens
  at*: `int * float` truncated back to `int` is 115 with SSE and 114 in the
  x87's 80-bit registers, and the archived server was i386. Built `-m32
  -mfpmath=387` for that reason.
- **`mailgen.c`** — `store_mail` and the block-freeing half of `read_delete`
  from `mail.c`, writing mail files for `internal/persist/mail/classic` to be
  checked against. Not arithmetic: the thing it pins is that a link between
  two blocks is a *byte offset into the file*, not a block number. A codec
  that has that wrong is wrong symmetrically, so it round-trips its own files
  perfectly and cannot read a single multi-block message the real server
  wrote. Must be built `-m32`, for the same reason `maillayout.c` must.
- **`nameoracle.c`** — `isname` and `get_number` from `handler.c`, the two
  functions that decide what a typed word means. `isname` reads like a prefix
  match and is a whole-word one, which this port got wrong for four phases;
  `get_number` rewrites the caller's buffer before deciding the prefix was a
  number, so what it leaves behind matters as much as what it returns. Checked
  over 168 name pairings and 15 argument forms.

If you are about to port anything with a division, a cast, or a comment
describing numbers in it, the next file in this directory is probably the one
you are about to write. `docs/developer.md` has the pattern.

## Layout tools

Several of CircleMUD's on-disk formats are `fwrite`s of a struct, which means
**the format is the struct's memory layout** — padding holes included, and
those holes hold whatever the stack held. Each of these prints the offsets,
sizes and padding gcc actually chose, and the Go codec is required by a test to
reproduce them field for field under both data models (ILP32 for the archived
data, LP64 for a modern rebuild) rather than hard-coding them.

Like `bin2ascii`, the 32-bit half needs `gcc -m32`. CI installs the toolchain
only for changes that can affect these — see the `ilp32` step in
`.github/workflows/go.yml`, and add to its path filter if you add a tool here.

- **`pfilelayout.c`** — `struct char_file_u`, the player database.
- **`boardlayout.c`** — `struct board_msginfo`, the bulletin board files. The
  second member is a `char *`, so a live heap pointer is written into every
  board file and the record's size changes with the pointer width.
- **`maillayout.c`** — the mud mail file's header and data blocks. `BLOCK_SIZE`
  does not change with the data model but the split *inside* a block does, so
  the same file means different things to a 32- and a 64-bit server.
- **`houselayout.c`** — `struct house_control_rec`, the house control file. It
  has a padding hole at offset 6 under ILP32 that nothing ever writes.

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
  wrote rather than one the Go encoder produced. It memsets each record to
  0xAB first, so a reader that accidentally depended on padding being zero
  fails instead of passing.

`pfilelayout.c` is here too; it is listed under "Layout tools" above with its
three siblings, since that is what it is.
