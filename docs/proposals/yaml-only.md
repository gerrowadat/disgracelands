# yaml only: retiring the legacy formats from the server

**This is the plan work is planned from.** It supersedes
`docs/proposals/go-port-plan.md` as the forward-looking document: that plan
took the port from nothing to a playable server across Phases 0–6 and stays
authoritative as the record of how, and as the design reference for the
architecture it describes, but the next thing this project does is in here.
Its Phase 7 (cutover) is not cancelled — it becomes downstream of this
change, and §8 below rewrites the one paragraph of it this invalidates.

It is also the detailed version of `docs/design/data-format.md` §11's steps
4 and 7, which said "not attempted" from the day the format landed until
this work closed them.

This is a **breaking change** and is proposed as **v1.0.0**. After it, the
only path from a CircleMUD `lib/` directory to a running Disgracelands
server is `dlctl import`. There is no in-place compatibility, no fallback
flag, and no auto-conversion.

**It is a compatibility change, so §0's own rules apply to it.**
`go-port-plan.md` §0's "Fidelity, phase two" (2026-08-23) frees new work to
modernise the implementation without recording a reason — but names two
things that stay fixed, and one of them is *compatibility*: "the on-disk
formats, `--lib-dir` contents and archived credentials this repo already
reads and writes". This changes what `--lib-dir` may contain. It is
therefore a deviation, it goes in `docs/deviations.md` with its reasoning,
and the nuance that makes it defensible rather than a straight breach is
worth stating precisely: **the repo goes on reading every archived format it
reads today.** Not one decoder is deleted. What changes is that `dlctl`
becomes the only thing that reads them, and the server reads one format.

---

## 0. Decisions taken

Settled before this document was written; the rest of it assumes them.

1. **The server understands exactly one on-disk format: `yaml`.** Every
   `--*-format` flag disappears from `dlmud`. There is nothing to select.

2. **`ascii` is retired from the server too, not just `binary`/`classic`.**
   It is the current default (`internal/config/config.go:229`) and it is not
   a struct dump, so this is a real cost — but "one live format" is the
   whole point, and two live player formats means two save paths, two
   crash-save paths and two sets of `Capabilities` to keep honest forever.
   `ascii` becomes what `binary` already is: a `dlctl` import source.

3. **Forward-only conversion, verified differentially.** No `classic`,
   `ascii` or `binary` *writers* are built (the tree has never had a
   `classic` writer at all). "Exactly dead on" is proved by comparing what
   each side **loads**, anchored on a C oracle wherever one exists, not by
   byte-diffing a reconstructed legacy file. §4 argues why that is the
   stronger claim as well as the cheaper one.

4. **A legacy `lib/` refuses to boot, with the command in the message.**
   No silent conversion into a scratch directory: the operator's archive is
   never written to, and the converted directory is somewhere they chose.

5. **v1.0.0.** One data format, a stable on-disk contract, the port's format
   question settled. This is also now self-consistent in a way it was not
   when the versioning document was written: `dataversion.Current()` derives
   the data-format stamp from the build's own release version
   (`internal/persist/dataversion/dataversion.go:87`), so cutting v1.0.0 is
   what makes a `.dlversion` of `1.0.0` exist. The release and the format
   reaching 1.0 are the same event, not two.

---

## 1. Why: the anachronisms that actually leak

The case is not aesthetic. This is the inventory, with citations, of legacy
format shape currently load-bearing in code that has no business knowing
about it.

**The whole directory layout is a function of a format name.**
`internal/config/subsystem.go:65` is
`Dir(libDir string, s Subsystem, format string) string` — nine subsystems,
each answering "where do I live" differently depending on whether the answer
is 2002 or now: players in `players/` or `etc/` or `pfiles/`, state in
`state/` or `etc/`, house objects in `state/` or `house/`, reports in
`state/` or `misc/`, names and messages and socials in `config/` or `misc/`.
Under yaml-only that function loses its third parameter and most of its
body: one layout, one subpath per subsystem. This is a better version of
this argument than the one that was true a month ago — the six ad-hoc `if
StateFormat == "yaml"` branches scattered through `cmd/dlmud/main.go` have
since been consolidated *into* this function, which is the right move and
also makes the residue easy to see. It is not a mess any more. It is one
clean function that exists only because there are two layouts.

**The player model carries the binary format's reserved slots.**
`game.PlayerRecord.Spares` (`internal/game/player.go:181`) is a
`LegacySpares` of `[6]int32`/`[7]int32`/`[5]int64`
(`internal/game/player.go:247`) — literally `char_file_u`'s padding, in the
canonical, format-neutral model whose entire reason for existing is that no
format's idiosyncrasy leaks into it. It is there for an honest reason (the C
server's own docs tell people to use those slots, and Disgracelands did: the
remort vector lives in one), but that reason expires the moment `binary`
stops being something the *server* writes. Three of those slots are already
named fields alongside it — `RemortVector`, `SpecFlags`, `OLCZone` — so what
remains is padding preserved for a writer that will no longer exist. It
moves into `internal/persist/player/binary`, where it belongs, as part of
that format's own round-trip fidelity rather than the game's model.

**The rent files are not pluggable, and the server knows it.**
`cmd/dlmud/main.go:325-327` opens the roster, asks whether it happens to
also be a `player.ObjectStore`, and falls back to `binary.NewObjectStore`
when it is not. That fallback is the only reason `cmd/dlmud` imports
`internal/persist/player/binary` at all, and it means a server running on
`ascii` writes its rent files as 2001 struct dumps — which is why real
container nesting had to be format-gated as a deviation rather than simply
implemented (`docs/deviations.md`, "Renting empties your bags and strips
your body"). Under yaml-only the assertion always succeeds, and the branch,
the import, and the deviation's format gate all go.

**`LoadText` takes three format names.** `internal/server/text.go:168` is
`LoadText(dir, messagesFormat, socialsFormat, helpFormat string)`. Three
parameters that exist only to answer "is this 2002 or not".

**The bit-field vocabulary is fixed by the letter encodings.** `classic`
stores flags as letters (`abc` = bits 0,1,2), which caps a flag field at
what a `long` held on the machine that wrote it. `yaml` already has the
escape hatch for anything unnamed (`flags_raw`), but as long as `classic` is
a *live* format rather than an import source, adding a 33rd room flag is a
compatibility question rather than a one-line addition. This is the item
that makes the change worth doing *now* rather than eventually: it is the
one that blocks new work rather than merely carrying old weight.

**`dlctl convert` produces a shape nothing will run.** With no `--type` it
"turns a whole original CircleMUD data directory into one the server can run
on" (`cmd/dlctl/convert.go:31-34`) — modernising text in place while leaving
the formats classic and ascii. After this change that output is not runnable
by anything, and the command is either retired or narrowed to its
`--type=pfile` half.

**The test suite proves the wrong thing, twice over.** `newTestServer`
(`internal/server/server_test.go:566-567`) builds every `internal/server`
integration test "on ascii/binary — the server's real defaults", with
`classic` stores for boards, mail, bans and houses. And `test/play`'s
end-to-end suite runs its scenarios over `bothFormats`
(`test/play/harness_test.go:185`) — `miniClassic` and `miniYAML` — which is
exactly right *today* and becomes half-wasted work the day classic stops
being runnable.

Two things this section deliberately does **not** claim. `binary`'s ten-byte
password truncation (`go-port-plan.md` §5.3.1) is already off the live path,
and so is the 2038 timestamp problem: both were solved by the *first*
migration, when the server moved to `ascii` in Phase 2. This is the second
migration and its case is the six items above, not the password field.

---

## 2. What is already true

This change is smaller than it sounds, because the destination exists and
works. Every subsystem the server reads has a landed, tested `yaml`
implementation:

| Subsystem | yaml implementation | Selected by |
|---|---|---|
| World | `internal/persist/world/yaml` | `--world-format` |
| Players + rent/crash | `internal/persist/player/yaml` | `--player-format` |
| Bans, boards, mail, houses, reports, clock | `internal/persist/{bans,boards,mail,houses,reports,clock}` | `--state-format` |
| Disallowed names | `internal/persist/names` | `--names-format` |
| Fight messages | `internal/persist/messages` | `--messages-format` |
| Socials | `internal/persist/socials` | `--socials-format` |
| Help database | `internal/persist/help` | `--help-format` |
| Game tuning (`config/game.yaml`) | `internal/config` | never a format — yaml in both trees already |
| `text/`'s plain prose (motd, credits, …) | — | never a format |

`examples/stock/yaml` and `examples/mini/yaml` are complete, checked-in,
all-yaml data directories, regenerated and byte-compared against a fresh
`dlctl import` at release time (`cmd/dlctl/import_test.go`). `test/play`
already plays a full scenario suite against `examples/mini/yaml` on every
run.

So the work is not "build the yaml format". It is **remove the
alternatives, prove the conversion is exact, and make sure it stays exact.**
The second and third are where the effort is, and §4 and §5 are the
substance of this proposal.

---

## 3. What changes

### 3.1 `dlmud`

- **Delete** `--player-format`, `--world-format`, `--state-format`,
  `--names-format`, `--messages-format`, `--socials-format`,
  `--help-format`. Not "reject non-yaml values" — remove them. A flag whose
  only valid value is its default is noise, and leaving it invites a future
  reader to think the seam is still live.
- Each removed flag's environment variable goes with it. Because
  `internal/config` derives env names from flag names rather than declaring
  them separately (`go-port-plan.md` §10, Phase 0), that is automatic — but
  it means a deployment setting `DLMUD_WORLD_FORMAT=classic` gets silence.
  **Config must reject an unknown `DLMUD_*` variable that matches a removed
  flag's name, by name, with the migration command**, because otherwise the
  most likely failure of this release is a container quietly ignoring its
  own configuration.
- **`config.Dir` loses its `format` parameter** and most of its body
  (`internal/config/subsystem.go:65`). One layout: `world/`, `players/`,
  `state/`, `config/`, `text/`.
- **Delete** the `player.ObjectStore` type assertion and its `binary`
  fallback (`cmd/dlmud/main.go:325-327`). `player.Store` and
  `player.ObjectStore` stay separate interfaces — a reasonable split
  independent of formats, see §10 — but `cmd/dlmud` requires the opened
  store to satisfy both and fails at boot if it does not.
- `LoadText(dir string)` loses three parameters.
- **Add** legacy-layout detection at boot (§3.3).

### 3.2 The registries

`Register`/`Open`/`Formats` **stay**, in `world`, `player` and the state
packages. This is not hedging. `dlctl` needs them — it opens `classic`,
`binary` and `ascii` sources by name and always will — and a `Source` backed
by a tarball or an embedded FS is still something someone might want
(`go-port-plan.md` §6.2). What changes is *who registers what*:

- `cmd/dlmud` registers **only** the `yaml` implementations. Its blank
  imports of `.../classic` and `.../ascii` (`cmd/dlmud/main.go:52-64`) are
  deleted, so the legacy decoders are not linked into the server binary at
  all. Worth stating as a property and testing for: **a legacy format is not
  merely rejected by the server, it is absent from it.**
- `cmd/dlctl` registers everything.

The `classic`/`ascii`/`binary` packages are **never deleted**, for the
reasons `data-format.md` §11 already gives: `classic` is the world parity
oracle for as long as the C server is authoritative and is how the 1,184
dated nightly world backups get read; `binary` is the only thing that can
read the archived roster at all. Roughly 10,700 lines of format code and
tests keeps running on every push. This release removes it from the
*server*, not from the tree.

### 3.3 Refusing a legacy directory

On boot, before anything is opened: if `--lib-dir` has no `world/*.yaml` but
does have `world/zone.lst` or `world/wld/index`, or has `etc/players`, or
`misc/socials`, the server exits non-zero with the exact
`dlctl import --from-dir=<lib> --to-dir=<somewhere>` invocation for that
directory. Detection is on the *legacy marker files*, not on absence of
yaml, so a genuinely empty directory still gets the ordinary "no world data"
error rather than a confusing migration instruction.

`.dlversion` checking (`internal/persist/dataversion`) already handles the
other direction — data written by a different release — and is unchanged.

### 3.4 `dlctl`

The verb-plus-`--type` shape the CLI has settled into
(`import`/`fmt`/`lint`/`dump`/`verify`, `cmd/dlctl/layout.go:34`) is a good
fit for what this needs, so this is an extension rather than a new surface.

- **`dlctl verify` grows a comparison mode**, and the rest of `allTypes`.
  Today it is `--type=pfile` only and answers "does this decode"
  (`cmd/dlctl/verify.go:21`). It gains
  `--against=<dir> --against-format=<fmt>`: load both directories through
  their drivers and assert the loaded states are equal, subsystem by
  subsystem, reporting every difference rather than the first. With no
  `--type`, all of them, mirroring what `import` already does.

  This is the operator-facing form of §5's whole test architecture — the
  thing you run against *your* archive, which the repo's own fixtures
  cannot cover because the real data is private. It is also what makes §5's
  tests cheap to write: they are `verify --against` over a fixture.

  It also grows an eighth, non-subsystem `--type=copied`, added by #241:
  `import` *copies* `text/`'s prose, `config/game.yaml` and
  `text/help/screen` rather than converting them, so they have no loader to
  compare through and are compared as bytes — the one place in this
  comparison where bytes are the right question, for the same reason §4.1
  says they are the wrong one everywhere else. Nothing compared them at
  all until then, and losing `config/game.yaml` is losing a server's whole
  tuning.
- **`dlctl import` gains `--verify`, default on**, running the above
  immediately after importing and failing the import if it does not hold.
- **`dlctl convert` with no `--type` is retired** (§1): it produces a
  directory nothing runs. `dlctl convert --type=pfile` stays — reformatting
  a roster between `binary` and `ascii` without going near yaml is still a
  real thing to want when comparing against the C server.

---

## 4. What "exactly dead on" means

### 4.1 The chain of evidence, and why forward-only is enough

The claim we need is not "a converted file can be turned back into the
original bytes". It is: **a server running on the converted data behaves
identically to one running on the original.** Bytes are a proxy for that,
and a lossy one in both directions — two `classic` files differing only in
whitespace load identically, and two identical loads can be written back
differently by any writer that does not reproduce 1990s formatting quirks
exactly.

So the evidence is over *loaded state*, and over *observed behaviour*:

```
C loader  ==  Go classic loader  ==  Go yaml loader
    |               |                     |
world-parity.sh     |          parity_test.go / verify --against
 (the C oracle)     |
               being retired

C server  ==  Go server on classic  ==  Go server on yaml
        scripts/session-parity.sh    test/play's bothFormats
```

Both left-hand links already exist and run (`scripts/world-parity.sh`: 3,202
records, zero differing fields; `scripts/session-parity.sh` over
`testdata/parity/`'s scripts). The right-hand links exist for the world
(`internal/persist/world/yaml/parity_test.go`) and, end to end, for
`examples/mini` (`test/play/harness_test.go`'s `bothFormats`). §5 extends
them to everything else. Transitivity gives the claim we want, anchored on
the C — which is the discipline `CLAUDE.md` insists on everywhere else and
which a Go-to-Go byte round-trip would not have.

Building `classic`/`binary`/`ascii` writers to get a byte comparison would
add roughly 3,000 lines whose only consumer is a test. It is rejected, and
this paragraph exists so a future reader can see it was considered rather
than missed.

### 4.2 The text transforms — already resolved, and worth knowing why

An earlier draft of this proposal listed fixing a lossy text transform as
step one. **It has since been fixed on `main`, by exactly the approach that
draft recommended**, and the shape of that fix is the best available
evidence that "dead on" is achievable rather than aspirational. Recorded
here because the reasoning is what generalises, not the patch.

`goccy/go-yaml` re-parses and re-prints whatever a `BytesMarshaler` returns
while splicing it into the document, and its re-print of a literal block
strips trailing newlines regardless of the chomping indicator asked for.
Three string shapes cannot survive it: a run of trailing newlines (a
deliberate blank line in a room description), a bare carriage return, and
trailing whitespace on a final line with no newline after it. `yaml` used to
collapse the first and report the normalisation. It now writes all three as
quoted, escaped scalars instead (`internal/persist/world/yaml/text.go:155`,
`needsQuoting`), which the library carries back unchanged. Incidence: 61
strings out of 12,372, against 5,347 that still write as readable literal
blocks.

The judgement in that change is the one this whole proposal rests on: a
transform with *no* alternative can be documented and reported; a transform
with an alternative should not be. Quoting costs prettiness. A trailing
blank line is a blank line on a player's screen.

What remains is one transform that is genuinely unavoidable and genuinely
not lossy, and it should be reclassified rather than fixed. `classic`
reproduces `fread_string`'s CRLF-joining, so a loaded description is
`\r\n`-joined *in memory* while the file on disk is `\n`-joined. YAML cannot
represent CRLF distinctly — the spec folds CR, CRLF and LF alike on decode —
so `yaml` stores LF and re-derives CRLF on load. **That is exactly the
relationship `classic`'s own bytes already have to its own in-memory form.**
The loaded states are identical; only the stored bytes differ, and they were
always going to. Action: a test pinning it, and a line in `docs/deviations.md`
saying it is settled rather than outstanding.

### 4.3 What exactness cannot cover

One gap testing will not close, listed so nobody believes the claim is
broader than it is.

- **`ascii`'s and `yaml`'s field sets are this port's own**, not the C's, so
  `ascii → yaml` has no C oracle behind it the way `binary → yaml` does. The
  best available check is a Go-side round-trip plus the fact that both
  target the same `game.PlayerRecord`. Say so rather than implying the whole
  matrix has equal footing.

---

## 5. The compatibility test architecture

The existing tests are good — better than when this proposal was first
drafted — but they are pointed at a corpus that does not contain the hard
cases. Four changes.

### 5.1 The fixtures are missing the half this makes mandatory

This is the biggest hole and it is not obvious until you look.
`examples/stock/binary/etc` contains `hcontrol` and a `README`.
`examples/stock/binary/house` contains a `README`. **There is no player
file, no rent file, no board, no mail file and no ban list in any
checked-in fixture in this repository.** The entire `binary`/`ascii` →
`yaml` player and state conversion path — the part this release makes the
only path — is tested only against fixtures each test builds for itself,
which by construction contain what the test author thought to include.

And stock CircleMUD's text is pure ASCII throughout, which is exactly how
the transcoding gap in five of seven importers sat inert until someone went
looking (`data-format.md` §11.1). Same blind spot, same cause, and the
lesson was written down at the time: *the gap was real but inert against
every fixture in this repo.*

**Proposal: `examples/torture/`, a deliberately hostile legacy `lib/`,
checked in as `binary/` plus its imported `yaml/`, built to break the
conversion.** Generated by the C tools where a C tool can generate it —
`reference/tools/pfilegen.c` exists for exactly this shape of job, and the
layout tools (`pfilelayout.c`, `boardlayout.c`, `maillayout.c`,
`houselayout.c`) already pin the struct offsets — and hand-built where the
content is textual. At minimum:

- A roster with a maximum-length name, a name one short of the limit, a
  title using every byte of its field, all 32 affect slots occupied, every
  `spare` slot non-zero and distinct, a 2038-crossing timestamp, a real DES
  hash, and a level-0 and a level-max character.
- Rent files with deeply nested containers, an object with all four values
  at extremes, and a crash file with the "lost to rent" header state.
- Boards with a full message set, mail with multi-block bodies, an
  `hcontrol` with an orphaned house (the one case `yaml` behaves
  differently — `data-format.md` §9), a ban list with every ban type.
- World files exercising: CP1252 bytes in a description; CRLF mid-string; a
  trailing blank line before `~`; a bare CR; trailing whitespace on an
  unterminated final line — the last three being §4.2's `needsQuoting`
  cases, which currently have no fixture that deliberately contains them; a
  `#` at the start of a line inside a description (the
  `count_hash_records` over-count case, `go-port-plan.md` §10 Phase 1);
  ASCII art at every nesting depth including an exit description and an
  extra description; every bit set in every flag field; an `E`-format mob
  with every ability; dice notation at its extremes.
- `misc/socials`, `misc/messages`, `misc/xnames` and a help entry each with
  non-ASCII text, and a help keyword line that slugs into a collision.

Every one of those is a case that has already gone wrong once in this
project or is one line from a case that did. The fixture's `README` says
which, per file, in the style `examples/mini/README.md` already uses.

This becomes the primary compatibility corpus. `examples/stock` stays as the
realistic one and `examples/mini` as the fast one.

### 5.2 Differential tests, in Go, over all three corpora

For each subsystem: load the legacy directory through its driver, load the
imported yaml through its driver, deep-compare. That is
`dlctl verify --against` (§3.4) invoked from a test, which is why building
it as a command first is the right order — one implementation, two callers.

Plus, per subsystem: an **idempotence** check (`fmt` twice is byte-identical
— true for several, asserted for all) and a **stability** check (a fresh
import equals the checked-in yaml byte for byte — exists for `stock` and
`mini` at `cmd/dlctl/import_test.go`, extended to `torture`).

**These must be ordinary Go tests, not shell scripts.** `go test -race ./...`
already runs on every push and PR; a shell script would need adding to
`go.yml`, which `CLAUDE.md` explicitly forbids. Writing the suite in Go is
how it gets day-to-day coverage without amending the workflow — the scope
rule and the testing goal point the same way here, which is worth noticing
rather than treating as an obstacle to route around. Only what needs a C
toolchain (the oracles, `world-parity.sh`, `session-parity.sh`) stays in
`release.yml`, where it already is.

### 5.3 Fuzz targets — the tree has none

`grep -r 'func Fuzz'` returns nothing. Given that every text-transform
finding in `data-format.md` §12 was found by round-tripping a corpus rather
than by reasoning about the library, and that the corpus in question was
`examples/stock` — which §5.1 has just established does not contain the hard
cases — this is the gap most likely to be hiding the next one.

Three targets, seeded from all three corpora:

- `FuzzTextRoundTrip` — any string through `Text`/`NestedText` and back.
  Seed: all 12,372 real strings. This is the one that would have found
  §4.2's three shapes without a human noticing them.
- `FuzzClassicRecordRoundTrip` — arbitrary bytes through the `classic`
  reader, then the yaml writer, then the yaml reader; assert loaded-state
  equality for anything the classic reader accepts. Finds parser
  divergences, not just text ones.
- `FuzzBinaryRecordRoundTrip` — the same for `char_file_u`-shaped bytes,
  under both data models.

Seed corpora committed; crashers committed as found, which is normal Go
fuzzing discipline and gives the regression suite for free.

### 5.4 Move the live tests onto yaml

Two suites, opposite directions.

`internal/server`: `newTestServer` (`server_test.go:566-567`) switches to
`player/yaml` for both `Store` and `ObjectStore`, and `newTestServerWith`'s
four `classic` defaults switch to their `yaml` equivalents. The coupling is
concentrated in the harness, so this is contained with a large payoff —
**every integration test then proves the format the server ships on.** The
handful of tests that load `classic` *deliberately*, to compare against
`yaml` (`internal/server/{helpformat,socialsformat,damagemessages}_test.go`),
stay exactly as they are: they are differential tests already, and this
release makes them more important, not less.

`test/play`: `bothFormats` (`harness_test.go:185`) collapses to `miniYAML`,
and `miniClassic` goes. Losing coverage there is the point — that suite is
verifying that a scenario behaves the same on a format the server will no
longer run. What replaces it is §5.2's differential layer, one level down,
where the comparison is cheap and total instead of expensive and sampled.

`scripts/session-parity.sh` needs its Go side pointed at an imported yaml
copy while its C side keeps the binary one. Small change, and it makes the
harness compare the *shipping* Go configuration against the C for the first
time, which is what `go-port-plan.md` §11 wanted from it.

---

## 6. Fields that exist in yaml and not in the legacy formats

This is the capability the change is *for*, and it needs a rule before the
first field arrives, not after.

The mechanism exists in two tiers: `.dlversion`
(`internal/persist/dataversion`, a whole-directory stamp now derived from
the build's release version, with defined behaviour in both directions) and
the per-file `schema: dl/<kind>@<major>` tag. Readers use `yaml.Strict()`,
so an unknown field is an error — which is right, and means additions must
be additive-and-optional or a major bump.

Three rules, none currently written down:

1. **A new optional field's default is declared explicitly, never inherited
   from Go's zero value.** `omitempty` plus a zero value is right often
   enough to be a trap: the first field whose sensible default is `true`, or
   `-1`, or "unset differs from zero", is silently wrong for every existing
   directory. Defaults live in one table per subsystem, applied by the
   reader after unmarshalling.

2. **A new field no legacy format can source is named in the importer's own
   output.** When `dlctl import` writes a field it could not derive from the
   source, it says so — once, summarised — so an operator converting a real
   archive learns which values are this port's choice rather than their
   data. Same posture `import` already takes toward transcoding counts.

3. **A "minimal document" test per subsystem**: unmarshal a document holding
   only the required fields, and assert every optional field against the
   declared default table. This is what stops a default drifting when a
   struct is edited, and it is cheap.

`docs/design/data-format-versioning.md` already defines what a differing
major and minor *do*. This section fixes who decides and what a default is.

---

## 7. The steps

Each row is its own branch and PR. Rows 1–2 are prerequisites and are
independently shippable before the breaking change; rows 3–6 are the break;
row 7 is the release.

**Status: rows 1–6 have landed.** Row 7 — cutting the release — has not,
and is a decision rather than a task. Each row below carries what
actually happened, since several of them turned out to be about something
other than what they were written to be about.

| # | What lands | Done when |
|---|---|---|
| **1 ✅** | `examples/torture/` (§5.1) — `binary/` and its imported `yaml/`, with a `README` explaining each hostile case. | `dlctl import` on it succeeds, `dlctl lint --type=world` reports zero findings, and the checked-in `yaml/` matches a fresh import byte for byte. |
| **2 ✅** | `dlctl verify --against` across `allTypes` (§3.4); the differential, idempotence and stability tests over all three corpora (§5.2); the three fuzz targets and their seed corpora (§5.3). | `verify --against` is green on `stock`, `mini` and `torture`; the fuzz targets run clean for the agreed budget; all of it is Go tests running under the existing `go test -race ./...`. |
| **3 ✅** | Move both live suites onto yaml (§5.4): `newTestServer`, `newTestServerWith`, `bothFormats`, `session-parity.sh`. | `newTestServer` builds a yaml server; `test/play` runs `miniYAML` only; the full suite is green under `-race`; the deliberate `classic` differential tests are untouched. |
| **4 ✅** | Delete the seven `--*-format` flags and their env vars; reject a removed `DLMUD_*` variable by name (§3.1); strip `config.Dir`'s format parameter; delete the `ObjectStore` fallback and `cmd/dlmud`'s legacy blank imports (§3.2); move `LegacySpares` out of `internal/game` into `persist/player/binary` (§1); retire `dlctl convert`'s no-`--type` mode (§3.4). | `dlmud --help` mentions no format; a test asserts the legacy decoder packages are not reachable from `cmd/dlmud`; `DLMUD_WORLD_FORMAT=classic` fails loudly with the migration command; `Config`'s default `LibDir` moves from `examples/stock/binary` to `examples/stock/yaml`. |
| **5 ✅** | Legacy-layout detection and refusal at boot (§3.3). | Pointing `dlmud` at `examples/stock/binary` exits non-zero printing the exact working `dlctl import` line for it — asserted by running that line and booting the result. |
| **6 ✅** | The defaults rule, its per-subsystem tables and minimal-document tests (§6); the `docs/deviations.md` entry this change owes under "Fidelity, phase two" (header above), plus the settled CRLF transform and the removed rent-containment format gate; `data-format.md` §11 rows 4 and 7 marked done; `docs/operations.md`'s getting-started rewritten around the mandatory import; `docs/configuration.md` for the removed flags; `go-port-plan.md`'s Phase 7 rollback paragraph (§8). | The docs describe the shipped server. `make check` green. |
| **7** | `make release BUMP=v1.0.0`, with an upgrade note. | `release.yml` green, including the ILP32 checks, world parity, the licence check and the example-regeneration checks. |

Rows 1–2 are worth landing even if the rest slips: they make the current
conversion demonstrably exact, which is valuable whether or not the legacy
formats are retired this year.

---

## 8. What this breaks, and for whom

**Anyone running the Go server on a CircleMUD `lib/`.** They run
`dlctl import` once, point `--lib-dir` at the result, and their old
directory is untouched. Rent files, boards, mail, houses, aliases and the
roster all come across.

**Anyone running on `ascii`.** Same command, `--from-format=ascii`. This is
the group with the least warning, since `ascii` is today's default, and the
upgrade note should lead with it rather than with `binary`.

**The Phase 7 rollback story, which this invalidates.**
`go-port-plan.md:2186-2189` currently says: "the C server tree is still
buildable and `--lib-dir` is the same directory either format reads —
falling back is starting the C binary against the same data, not a migration
in either direction." **That stops being true.** After yaml-only, rolling
back from the Go server to the C server means running the C server against
the pre-migration archive and losing everything since, or building the
exporter §0.3 declines to build. A real cost, and the paragraph must be
rewritten rather than left for someone to discover mid-rollback.

The mitigation available without an exporter: `dlctl import` never writes to
its source, so the pre-migration directory is always intact, and the honest
rollback is "the C server, at the data as of the migration". For a game with
no live players that is adequate. It would not be for a running service, and
if Phase 7's precondition 4 (revive the archived roster, or start clean?)
ever resolves toward a real revival, the exporter question reopens with it.

**Nothing in the archive is at risk.** The real 2001–2008 data has never
been in this repo and is not touched by any of this.

---

## 9. What is not in it

- **No legacy writers.** §0.3, §4.1.
- **No auto-conversion at boot.** §0.4.
- **No deletion of `classic`/`ascii`/`binary`.** §3.2.
- **No new yaml fields.** §6 sets the rule for adding them; adding one is
  the next change, not this one.
- **No schema changes to any existing yaml document.** If the exactness work
  in rows 1–2 finds one is needed, that is a finding to record and size
  separately, not to fold in silently.
- **No database backend.** `data-format.md` §12's open question is
  unaffected either way.
- **Not a cutover.** `go-port-plan.md`'s Phase 7 preconditions are untouched
  by this except for the rollback paragraph in §8. Cutover remains a
  decision nobody has made.

---

## 10. Open questions

**Should `dlctl verify --against` be able to compare against the C server?**
It compares two Go loaders. For the world there is already a C oracle
(`world-parity.sh`); for the roster there is `pfiledump.c`. Wiring `verify`
to shell out to those when a C toolchain is present would make the
operator-facing tool as strong as the release-time check. Deferred because
it puts a C dependency inside a command an operator runs, which is exactly
what §5.2 keeps out of day-to-day CI.

**How long does the fuzz budget run, and where?** A short budget in
`go test` is nearly worthless; a long one belongs in `release.yml` or
nowhere. Proposal: seed-corpus-only (deterministic, fast) on every push, a
real budget at release. Decide in row 2.

**Does `examples/torture/` belong in the repo or get generated on demand?**
Checked in, per §5.1, on the grounds that a fixture you regenerate is a
fixture whose generator can drift. But it is binary data in a source tree
and the counter-argument is real. The C generators are checked in either
way.

**Should `player.Store` and `player.ObjectStore` stay separate?** §3.1 keeps
them apart. Once the only implementation satisfies both, merging them would
simplify `cmd/dlmud` further — but it would bake "one player, one file" into
the interface, which is a format decision an interface should not be making.
Left alone deliberately; noted because a reader will ask.

**What happens to `docs/proposals/` when this lands?** By this repo's own
convention a design document moves to `docs/design/` once the thing it
describes is built. This one describes a removal, so what it leaves behind
is mostly the test architecture in §5. Likely outcome: §5 and §6 move into
`data-format.md` as the format's own compatibility contract, and this file
stays in `proposals/` as the record of the decision.

---

## Related documents

- `docs/proposals/go-port-plan.md` — the port itself, Phases 0–6, superseded
  by this document as the forward plan and still authoritative as the record
  and the architecture reference. §5 and §6 (the pluggable seams) are what
  this change collects on; §10 Phase 7's rollback paragraph is what it
  invalidates.
- `docs/design/data-format.md` — the yaml format. This proposal is its §11
  steps 4 and 7; its §12 is where the text-transform findings in §4.2 live.
- `docs/design/data-format-versioning.md` — `.dlversion` and what a
  differing version does, which §6 builds on.
- `docs/deviations.md` — where this change's own entry goes, per the header
  above, alongside §4.2's settled CRLF transform and the removal of the
  rent-containment format gate.
