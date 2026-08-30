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
- **`startoracle.c`** — `roll_real_abils`, `do_start` and `advance_level`
  from `class.c`: everything a level 1 character is. Not arithmetic so much
  as **draw order** — hit points, then mana, then movement, for every class,
  including the two whose mana is thrown away again four lines later. It
  prints a `number(0, 999999)` after each character for the same reason
  `randoracle` has an alternating mode: without a following draw, a port
  that takes one too many agrees perfectly about the character it took it on
  and is wrong about every character after. Checked over five classes, six
  seeds and 200 characters each; it found a swapped mana/movement assignment
  (#188), and then caught a "fix" written from `../WipeMud-src/`, whose
  `advance_level` differs — no mana draw for a thief or a warrior, no
  paladin case. **Lift from `../moderncserver/`.**
- **`editoracle.c`** — `improved_editor_execute`, `parse_action`,
  `format_text` and `replace_str` from `improved-edit.c`: the whole of the
  improved line editor's eleven commands. Line-range and whole-buffer string
  surgery rather than arithmetic, and wrong in a different way at nearly
  every turn — a three-line buffer has a fourth line, an emptied buffer is
  not a freed one, `/n` prints its line number on a line of its own, and a
  `/ra` that runs out of room truncates the player's text and then reports
  the string as not found. Checked over 805 command-against-buffer cases,
  comparing both the text sent and the buffer left behind. **Built `-O0`,
  not `-O2`**: `PARSE_LIST_NUM`'s `sprintf(buf, "%s%4d:\r\n", buf, i - 1)`
  has its destination as its own `%s` argument, and modern gcc resolves that
  undefined behaviour into something that keeps only the last line, where
  `-O0` calls glibc and the accumulation works — which is what the archived
  server's compiler did.
- **`nameoracle.c`** — `isname` and `get_number` from `handler.c`, the two
  functions that decide what a typed word means. `isname` reads like a prefix
  match and is a whole-word one, which this port got wrong for four phases;
  `get_number` rewrites the caller's buffer before deciding the prefix was a
  number, so what it leaves behind matters as much as what it returns. Checked
  over 1,456 name pairings and 15 argument forms.

  **Its own corpus is the thing worth watching here, not its code.** Until
  2026-08-29 every namelist it swept was made only of letters and spaces, and
  over an alphabetic namelist `isname`'s real terminator (`!isalpha`) and the
  wrong one this port had (whitespace) cannot disagree — so 168 pairings
  agreed with a C they were not testing, for a year, and `look 6` did not
  match the stock newbie zone's `staircase stair 606 rs` (#277). The sweep now
  carries digits, punctuation, an apostrophe, a hyphen, doubled and trailing
  spaces and a namelist wrapped across lines by `fread_string`, and the Go
  test fails if it ever narrows back. An oracle is only as good as what it is
  swept over: when adding one, spend the effort on the inputs.

- **`skilloracle.c`** — `find_skill_num` from `spell_parser.c`, with
  `is_abbrev` and `any_one_arg` from `interpreter.c`. This is the other half
  of "what does a typed word mean": `nameoracle` covers what names a *thing*
  in the world, this covers what names a *spell*, and they are different
  rules — `isname` is whole-word, `find_skill_num` is two kinds of prefix at
  once.

  Two kinds, and the second is the one that gets missed. `is_abbrev(name,
  spell_info[index].name)` matches the whole typed string against the whole
  name, so `magic mis` works; then a second pass walks both a word at a time
  and requires each typed word to abbreviate the name-word in the same
  position, so `mag mis` and `b h` work too. This port had only the first
  branch, and refused 1,145 of the 1,549 per-word abbreviations of the game's
  own 71 spell names — including `cast 'mag mis'`, which is about as common
  a thing to type as this game has (#355). Found by writing this file;
  `docs/investigations/partial-matching.md` has the sweep. Checked over 1,569
  queries now: every per-word abbreviation, plus whitespace, tabs, case, runs
  of spaces, queries longer than the name, and every full spell name.

  Its name table comes in on stdin rather than being compiled in, so the Go
  test feeds it the port's own spell table and the two cannot drift apart the
  way they would if the names were duplicated here.

  It also found what the investigation had not: **an empty query matches the
  first spell**. With no words rule 2's loop never runs, `ok` is still true
  and `!*first2` holds, so `find_skill_num("")` is 1 — and `cast '  '` casts
  armor on the real server, because `do_cast`'s `strtok` hands the spaces
  straight through. Reproduced, unlike `isname`'s own empty-string case a few
  entries up, and the difference between those two calls is reachability
  rather than taste.

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
- **`mailoracle.c`** — `store_mail`'s own body, run to write a real mail
  file for the Go to read back. Where `maillayout.c` says where a block's
  fields are, this says what goes in them: the link joining a message's
  blocks is a byte offset into the file rather than a block number, which no
  struct layout reveals and which a port can get wrong while passing all of
  its own round-trip tests. Build `-m32`, for the same reason
  `maillayout.c` needs it.
- **`aliasoracle.c`** — `write_aliases` and `read_aliases`, run over aliases
  given on the command line. Not a layout tool: this format is `fprintf` and
  `fgets` rather than an `fwrite` of a struct, so it needs no `-m32` and
  runs anywhere `gcc` does. What it pins is the length-prefix convention and
  the leading space that goes with it — the file stores
  `strlen(replacement) - 1` bytes from `replacement + 1`, and the reader
  puts the space back.
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
