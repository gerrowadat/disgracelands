# A lib/ built to break the conversion

`examples/stock/` is the realistic corpus and `examples/mini/` is the fast
one. This is the *hostile* one: a legacy CircleMUD `lib/` directory in
which every file contains something that has already gone wrong once in
this project, or is one line from a case that did.

It exists because of a hole `docs/proposals/yaml-only.md` §5.1 found and
which is not obvious until you look. Before this directory:

> There is no player file, no rent file, no board, no mail file and no ban
> list in any checked-in fixture in this repository.

The entire `binary`/`ascii` → `yaml` player and state conversion path —
the path the yaml-only release makes the *only* path from an archived
`lib/` to a running server — was tested only against fixtures each test
built for itself, which by construction contain what the test author
thought to include. And stock CircleMUD's own text is pure ASCII
throughout, which is exactly how the transcoding gap in five of seven
importers sat inert until somebody went looking
(`docs/design/data-format.md` §11.1). Same blind spot, same cause.

This is the primary compatibility corpus now. `stock` stays the realistic
one and `mini` the fast one.

## What it found

Three bugs in the first conversion ever run against it, and eight more
once `dlctl verify --against` and the fuzz targets went looking. They are
listed here rather than only in commit messages because the *kind* of bug
is the point: every one was silent, every one lost data or hung, and not
one could be reproduced with any other fixture in the tree.

The first three, from the import that produced `yaml/`:

- **`perm_affect` had no `_raw` escape hatch.** `wear_raw` and `flags_raw`
  carry a bit with no name in the yaml vocabulary; `perm_affect` did not,
  so an object with an unnamed permanent-affect bit came out of a
  conversion without it and nothing said so. Object `#5000` sets all 32.
  Fixed by adding `perm_affect_raw`.
- **Keyword fields were not transcoded, and were then mangled.** The
  world importer deliberately skipped keyword lists, "only where the C
  loader treats the field as free text rather than a keyword or symbol".
  But a keyword left in CP1252 is not valid UTF-8, so the yaml encoder
  substituted U+FFFD for the offending byte: `caf<0x92> sign` became
  `caf<REPLACEMENT> sign`, which matches nothing a player can type in
  either encoding and cannot be undone. Room `#5001`'s extra description
  is that keyword. Fixed by transcoding keywords too.
- **An unrecognised enhanced-mobile espec key vanished in silence.**
  Dropping it is correct — the set is closed by design and
  `interpret_espec` ignores anything outside it — but a conversion that
  loses a line should say which line. Mobile `#5001` has one. Fixed by
  reporting them.

And then, from `dlctl verify --against` comparing this directory to its
own converted copy, and from the four fuzz targets seeded off it:

- **The binary codec sign-extended every 32-bit flag field.** `varInt`
  widens a four-byte field through `int32`, so a stored `0xFFFFFFFF` —
  what a 32-bit `unsigned long` full of player flags looks like — became
  `-1` and then all sixty-four bits of a `game.Flags`. Symmetric on write,
  so the package's own byte-for-byte round-trip test could not see it.
  `Torturer` sets every bit of all three fields.
- **The yaml player format wrote `act_raw`/`affected_raw`/`prefs_raw` and
  never read them back.** A write-only escape hatch, which is worse than
  none: the file looks like it carries the bits.
- **`spec_flags` and `olc_zone` were not in the yaml player format at
  all.** Two Disgracelands-local fields living in what were `char_file_u`'s
  spare slots. Every builder's permitted OLC zone would have been lost by
  the migration.
- **The converted ban list came out backwards.** `Add` prepends, matching
  `ban.c`'s linked list, so replaying a newest-first list through it
  rebuilds the reverse.
- **A help entry's keywords were not transcoded either** — the same U+FFFD
  mangling as the world keywords, in a different importer, and the
  keywords are what the entry's own `.txt` filename is derived from.
- **`dlctl fmt --type=pfile` deadlocked.** It called `Save` inside a
  `List` range loop; `List` holds the store's read lock for the whole
  iteration and `Save` wants the write lock. It hung on any directory with
  a character in it — and until this fixture, no checked-in directory had
  one.
- **Four more string shapes could not survive a YAML round trip**, found by
  `FuzzTextRoundTrip` within seconds of it first existing: a string that is
  nothing but whitespace, a whitespace-only first line, a tab anywhere in a
  single-line value, and a value of `---` or `...` (which crashed the
  encoder outright).
- **Every plain `string` field in every format silently ate tabs**, because
  quoting was left to the library. That one came from
  `FuzzBinaryRecordRoundTrip`, on a character whose *name* held a tab, and
  is why `internal/persist/yamlenc` exists.
- **An out-of-range enum was dropped or written unreadably.** An affect
  location, a sector, a sex, a class or a social's position outside its
  name table was written as nothing (and read back as zero) or as
  `unknown-104` (which the reader cannot resolve either). Now `#104`, the
  convention `SpellNameOrNumber` already used.

## What is in it, and why each thing is there

### `world/`

One zone, `#50`, vnums 5000–5099. Every record is described, in its own
description, by the case it exists to break — so `dlctl dump` on this
world is itself the documentation.

| Record | The case |
|---|---|
| room `#5000` | Every flag bit a `bitvector_t` can hold, set at once: `abcdefghijklmnopqrstuvwxyzABCDEF`, bits 0–31. `G` onward would be `1 << 32` in an `int`, which is undefined behaviour in the C rather than a bigger number, so this is the widest field that is hostile instead of simply broken. Unnamed bits have to come back through `flags_raw`. |
| room `#5001` | CP1252 in the room name, the description, an exit description **and an extra description's keywords** — every non-ASCII byte in it is one an archived world file really contains. |
| room `#5002` | A deliberate blank line before the closing `~`, so the loaded string ends in two CRLFs. `goccy`'s literal-block re-print right-trims every trailing newline regardless of the chomping indicator, so this must be written as a quoted scalar. |
| room `#5003` | A bare carriage return mid-description. Unrepresentable in a YAML block scalar at all — the spec folds CR, CRLF and LF alike on decode. `obj/0.obj`'s bug object in the real archive has fifteen in a row. |
| room `#5004` | Trailing whitespace on an unterminated final line (the `~` is on the last text line, so `fread_string` appends no CRLF). A literal block drops the spaces. |
| room `#5005` | A `#` at the start of a line inside a description: `count_hash_records` counts it, so the loader sizes its arrays for a record that is not there. |
| room `#5006` | ASCII art on an extra description *and* on an exit description — text nested deeper than a top-level list item's own fields, where `NestedText` cannot reliably emit an indentation indicator and falls back to quoting. Its north exit is a locked door, keyed to object `#5003`. |
| room `#5007` | A `*` at the start of a line inside a description (`get_line` skips those; `fread_string` does not), and a line opening with `{{`, which YAML would otherwise read as a flow mapping. |
| room `#5008` | The minimum: a name, an empty description, one exit, nothing else. |
| mob `#5000` | Every action and affection bit; hit and damage dice at the extremes `sscanf`'s literal `d`/`+` format can express. |
| mob `#5001` | `E` format with an espec for every ability the C names, plus one line with no colon at all (`interpret_espec` reads that as a keyword with no value). |
| obj `#5000` | Every extra and wear bit; all four values at the `int32` extremes; a permanent-affect bitvector of `INT32_MAX`; two `A` affect lines. |
| obj `#5001` | A drink container lighter than what it holds, which the loader *mutates* at load time (`parse_object` raises its weight). A genuine load-time change to world data, not a validation. |
| obj `#5002` | Three extra descriptions, so the C's prepend-and-therefore-reversed ordering is observable, with ASCII art in the middle one. |
| obj `#5003` | The minimum an object can be; also the key to `#5006`'s door. |
| obj `#5004` | A container, nested three deep by the zone's own resets. |
| shop `#50` | Both fields `docs/design/data-format.md` §4.5 calls "kept awkward on purpose". |

### `etc/players`, `plrobjs/`, `plralias/`

The roster is four characters, and `plrobjs/`/`plralias/` are **siblings**
of `etc/` rather than children of it — the C's own layout, because it
resolves `LIB_PLROBJS` and `LIB_PLRALIAS` against its own cwd. This port
uses the other layout, and `dlctl import --type=pfile` guesses between
them; exercising the archived one is the point.

- **`Torturer`** — every field the format has, at its limit: all 32 affect
  slots occupied, all 200 skill slots, every reserved `spare` slot
  non-zero and distinct, a title using all 80 bytes of its field, a host
  using all 30 of its own, a real `crypt(3)` DES hash, and
  `2038-01-19T03:14:07Z` — the last instant a four-byte signed `time_t`
  can name. A rent file with a container inside a container inside a
  container, and an alias file with both alias shapes.
- **`Abbbbbbbbbbbbbbbbbb`** — nineteen letters, every byte of `char[20]`.
- **`Cddddddddddddddddd`** — eighteen, one short of the limit. An
  off-by-one in a fixed-width field is only visible with both.
- **`Nobody`** — level 0, no password, no skills, no affects, no title,
  zero timestamps. A crash file with the "lost to rent" header state and
  no objects, which is a different thing from having no file at all.

Two things this deliberately does *not* contain, because the format
cannot hold them and pretending otherwise would be misleading rather than
hostile:

- **A timestamp past 2038.** `binary`'s writer refuses one rather than
  wrapping it, so the boundary value is the hostile case the format
  actually admits. That the year after it is unrepresentable is not a gap
  in this corpus; it is the argument for the release it was built for.
- **A rent file that is genuinely nested on disk.** With `USE_AUTOEQ 0`,
  `struct obj_file_elem` has no `location` member, so `Crash_save`
  flattens every container before writing. The tree is built here anyway
  and flattened through `player.FlattenStoredObjects`, so what lands on
  disk carries the C's own contents-before-container *ordering* — which
  is the part a converter can still get wrong.

### `etc/`, `house/`, `misc/` — the state

- **`badsites`** — every ban type `ban.c` names, plus a site at
  `BANNED_SITE_LENGTH` exactly.
- **`board.mort`** — `MAX_BOARD_MESSAGES` (60) messages, so nothing can
  quietly drop the last one. **`board.immort`** — a body at
  `MAX_MESSAGE_LENGTH` (4096) and one that is empty. **`board.social`** —
  CP1252 in both the heading and the body. Both of those limits are
  properties of a fixed-size array rather than of the game, and `yaml`
  does not carry them across, which is why a board sitting *on* them is
  worth having.
- **`plrmail`** — a body spanning many of `mail.c`'s 100-byte blocks, a
  body landing exactly on a block boundary, and CP1252 text. There is no
  empty-body message: `store_mail` refuses one outright, so that is a
  message the C never wrote rather than one this corpus declines to
  include.
- **`hcontrol`/`house/`** — a house with contents, a house without, a
  house at `MAX_GUESTS`, a control record naming a room this world does
  not have, and **an orphan**: `house/5006.house` is a contents file with
  no control record naming it. That last one is the single place these
  two formats genuinely differ. `state/houses.yaml` nests a house's
  contents inside its own control entry, so contents belonging to no
  house have nowhere to go and are dropped on import. It is kept here
  deliberately: the difference is real, and a corpus that avoided it
  would leave it to be discovered by an operator.
- **`bugs`/`ideas`/`typos`** — all three kinds, CP1252 text, and an empty
  body.
- **`time`** — the clock's epoch, which `db.c` stores as a bare integer
  with nothing around it.

### `misc/socials`, `misc/messages`, `misc/xnames`, `text/help/`

All four carry CP1252, because all four went through `dlctl import`
untranscoded until `docs/design/data-format.md` §11.1.

`misc/socials` has **no comment lines**, and that is a fact about the
format rather than an oversight: `boot_social_messages` has no comment
syntax at all, so a `*` line there would be read as a social with the
wrong field count and fail the load. `misc/messages`, sitting beside it,
*does* have comments. That asymmetry between two neighbouring files is
exactly the kind of thing that gets assumed away.

`text/help/torture.hlp` contains a keyword line of pure punctuation
(`! ^`), which slugs to the empty string and gets the writer's positional
fallback, and two entries (`SLUG COLLISION` and `SLUG-COLLISION`) whose
keyword lines slug to the same file name and get the numeric-suffix
disambiguation. Neither case occurs anywhere in the real 216-entry
archive.

### `config/game.yaml`

Deliberately **not** at `config.c`'s defaults. `dlctl import` copies this
file rather than converting it, and a conversion that silently dropped it
would leave a server back on the defaults — which a fixture whose tuning
*is* the defaults could not detect.

## Reproducing it

`binary/` is generated, not hand-edited:

```sh
go run ./internal/fixtures/torture --out=examples/torture/binary
go run ./cmd/dlctl import --from-dir=examples/torture/binary --to-dir=examples/torture/yaml
```

Both halves are checked. `internal/fixtures/torture`'s own test
regenerates `binary/` and diffs it byte for byte, and
`cmd/dlctl/import_test.go` regenerates `yaml/` from it and does the same
— the standard `examples/stock` and `examples/mini` are already held to.
A second test asserts that the three invisible string shapes (a blank
line before a tilde, a bare carriage return, a trailing space) are
literally still in the committed bytes, because every one of them is the
kind of thing an editor or a reformat silently removes, and losing one
would leave the corpus passing everything while having stopped testing
what it is for.

The generator is Go rather than C, which is a departure from what
`yaml-only.md` §5.1 proposed, for a reason worth knowing: the
regenerate-and-diff test above is only worth anything if it runs on every
push, and a C generator for the ILP32 struct dumps needs `gcc-multilib`,
which by `CLAUDE.md`'s day-to-day/release split is installed in
`release.yml` and nowhere else. The layout knowledge that would have
justified the C is already C-anchored regardless:
`reference/tools/{pfile,board,mail,house}layout.c` print the offsets gcc
chooses and a test in each package requires the Go codec to reproduce
them, under both data models.

## What it is not

Not a world anybody should play, and not a world that has to be
*sensible*.

It does boot — `dlmud --lib-dir=examples/torture/yaml` loads it, populates
the zone and serves a connection — but several things in it are inert at
runtime by design rather than by accident, and it is worth knowing which:
the boards convert but never load, because `board_info[]` is a compiled-in
table keyed by object vnum (`3094`–`3099`) and this zone has no such
objects; three of the five socials are dropped at boot for the same shape
of reason, having no slot in the C's own command table; and there is no
room `3001`, so a character created here starts nowhere at all. All of
that is fine. What this corpus asserts is that the *conversion* is exact,
and every one of those files converts.

 A mobile with every action flag set at once is contradictory;
an object weighing `INT32_MAX` is absurd. Both load, which is the only
property being asserted. If you want somewhere to log in and try
commands, that is `examples/mini`.

## Findings the classic side is *supposed* to report

`dlctl lint --type=world --dir=examples/torture/binary --format=classic`
reports two findings and both are intentional:

- a warning that five pieces of world text are not valid UTF-8 — which is
  the corpus doing its job, and what `dlctl import`'s `--encoding` fixes;
- a note that object `#5001` is the drink container whose weight the
  loader raises.

The `yaml/` side lints at zero of everything, which is the actual bar:

```sh
go run ./cmd/dlctl lint --type=world --dir=examples/torture/yaml --format=yaml
```
