# Versioning the yaml data format

`docs/design/data-format.md` §10.1 already tags every individual file with
`schema: dl/<kind>@<major>` — what *shape* one file is in. This document
is the layer above that: one `major.minor.patch` for a data directory as a
whole, stamped when the directory is written and checked once at boot
rather than once per file. The number is the **release version of the
`dlctl` that wrote it** (§1.1), so a server can say, before it opens a
single file, whether the tooling that produced this directory and the
tooling about to read it are the same generation.

The rule, in one line: **a differing minor version warns; a differing
major version refuses to load at all, in either direction.**

Implemented in `internal/persist/dataversion` and wired into `dlctl
import` (with no `--type`), `dlmud`'s boot sequence, and `dlctl data
version`.

---

## 0. Decisions taken

| Question | Decision |
|---|---|
| **Shape** | **`major.minor.patch`, semver in spirit.** Three tiers, each with a different consequence, rather than the single incrementing integer §10.1 uses per file — a directory-level number has to describe *compatibility*, not just "newer/older", because unlike one file's shape, a whole yaml tree can drift in ways that are fine to run on and ways that are not. |
| **Which number** | **The release semver of the build that wrote the directory** — `internal/buildinfo`'s version, which the Makefile stamps from `git describe --tags`. Not a format version this project maintains separately by hand. §1.1 has the reasoning and what it costs. |
| **Where it lives** | **One file, `.dlversion`, at the root of the directory `--lib-dir` names.** Not per-subsystem. `PlayerFormat`, `WorldFormat`, `StateFormat` and the rest are independently selectable, but the code that implements `yaml` for all of them ships as one build of this one repo — there is no meaningful sense in which `internal/persist/player/yaml` is "version 1.2.0" while `internal/persist/world/yaml` is "version 1.4.0" in the same checkout. One number, describing what release of this codebase's yaml support last touched the directory. |
| **What owns the check** | **The server refuses to boot on a major mismatch; a linter answers the question ahead of time.** `dataversion.Check` is the one function both call — `cmd/dlmud`'s boot sequence, for the consequence that matters, and `dlctl data version`, so an operator can ask "will this start?" without starting it. |
| **What bumps which tier** | **The project's own release tiers, which now have to mean this too.** Major: a differently-versioned build would misread the data, not just fail to understand part of it — the same bar §10.1 sets for one file's own schema major, applied to the directory as a whole (a top-level directory renamed, a file split in two, `.dlversion` itself changing shape). Minor: additive — a new optional field, a new document kind, a new flag value understood — nothing an other-versioned reader loses data over, only fails to take advantage of. Patch: no schema or behaviour change a reader could observe at all. |
| **Which way the comparison runs** | **Neither tier looks at direction.** A *differing* minor warns and a *differing* major refuses, whether the data is ahead of the build or behind it. §2 has the argument; it is a change from this document's first version, which only ever looked upward. |

---

## 1. Why this needs its own number

Fidelity to the C matters for what the *game* does; it says nothing about
what a *data format this project invented* should promise its own
tooling. Nothing in `reference/` has an opinion here — there is no C
citation for this document, and that is expected.

The problem `dataversion` answers: `data/` is not read by one program.
`dlmud` reads it live. `dlctl import`/`fmt`, each with a `--type` of
`world`/`pfile`/`state`/`names`/`messages`/`socials`/`help`, read and
write it offline. A build from six
months ago and a build from today can both point at the same directory —
an operator upgrading in place, a CI job checking out an older tag against
a fixture, a contributor with two worktrees. §10.1's per-file tag already
protects each individual file's *shape*; what it does not answer is "is
this whole directory, as a body of work, safe for this build to run on at
all" — a question that needs an answer before the first file is even
opened, not discovered one `schema:` mismatch at a time partway through
boot.

### 1.1 Why the release semver, and not a number of its own

The stamp is the writing build's own release version. `dlctl` 1.4.2 writes
`1.4.2`; `dlmud` 1.4.2 reads it and agrees; `dlmud` 2.0.0 refuses it.

The alternative — and this document's first version did it this way — is a
format version maintained by hand, bumped only when
`internal/persist/*/yaml` earns it, decoupled from releases entirely. That
is the more precise instrument: it says *the format* moved rather than
*the project* did, so a year of releases that never touch the yaml
packages leave every directory happily compatible. What it costs is that
the number is only ever as good as somebody's memory. It lives in one
`var` that a change to a writer has no mechanical reason to touch; nothing
fails if it is not bumped, and what a missed bump produces is not an error
but silence — two builds that disagree about the files and neither of them
saying so. A version nobody is forced to maintain is a version that will
eventually be wrong in the one direction the mechanism exists to catch.

The release semver has none of that precision and one property that
outweighs it: **it is already correct, always, without anybody
remembering.** It is derived, not declared. Every build has one, it moves
on its own, and the release process is where the project already thinks
about what a version number is promising.

What it costs, stated plainly rather than discovered later:

- **False alarms.** Most releases change nothing about the format, and two
  builds a minor apart will warn at each other regardless. The warning is
  therefore weaker evidence than the old scheme's would have been — it
  says "these were built at different times", not "the format moved".
  That is the trade: a warning that is sometimes noise, over a warning
  that is sometimes absent when it shouldn't be.
- **A major release is a hard cutover for data, whether or not it is one
  for the format.** Cutting 2.0.0 means no 2.x server reads a 1.x
  directory until `dlctl data version --write` restamps it — which, if the
  format genuinely did not change, is all the migration there is to do,
  but it is not nothing and it is not automatic. Deciding to bump major
  now carries this consequence, and whoever bumps it owns saying so in the
  release notes.
- **Unreleased builds have no version at all.** §6.

## 2. The three tiers, precisely

Given a directory stamped `D` (the release of `dlctl` that wrote it) and a
running build that is release `S` (`dataversion.Current`):

| Comparison | Outcome | Who says so |
|---|---|---|
| `D.Major != S.Major` | **Refuses to boot** — in *either* direction. This build may misread the data outright, and "misread" is not a direction: whichever side of a major bump a reader is on, the other side's files mean something it does not agree with. This is the rule the C-era "newer data only" instinct gets wrong, and the reason `S` will not quietly open a directory an older major wrote either. | `dataversion.Check` returns a non-nil `error`; `cmd/dlmud`'s `run` returns it, which is a fatal exit before any store opens. |
| `D.Major == S.Major`, `D.Minor != S.Minor` | **Boots anyway, at your own risk** — again in either direction. Nothing refuses to read. If `D` is ahead, some of what the directory contains may go unrecognised; if `D` is behind, some of what this build expects to find may simply not be there. Both are worth saying out loud and neither is worth refusing over. | `dataversion.Check` returns a non-empty warning and a nil error; `cmd/dlmud` logs it and continues. `dlctl data version --dir=…` prints the same finding without starting anything. |
| `D.Major == S.Major`, `D.Minor == S.Minor`, any `Patch` | **Fully compatible, silently.** By construction: a patch bump changes nothing a reader could observe. | `dataversion.Check` returns `("", nil)`. |
| No `.dlversion` present | **Fully compatible, silently.** Every directory that predates this mechanism, every one written by an unreleased build (§6), and every one running only `classic`/`ascii`/`binary` — all of which are unversioned by design (§3). | Same as the row above. |

Note what is *not* in that table any more: there is no "older is fine"
row. The first version of this document had one, on the reasoning that a
new build reading old data is the ordinary case and nothing had ever
bumped major anyway. That reasoning was about a hand-maintained format
number, where "older" reliably meant "a subset of what I know". Under a
release semver it does not: 1.x and 2.x are two different agreements about
what the files mean, and a build has no more business guessing at the one
it was not written for than at one from the future.

## 3. Why `classic`/`ascii`/`binary` are not versioned

They are the original CircleMUD formats, ported as read/write oracles
(`docs/deviations.md`'s "not deviations" framing), not designs this
project owns. Their shape is whatever `struct char_file_u` or
`fread_string` already fixed in 1993–2008 — there is nothing here to
version because there is no future revision of `classic` coming. A `.
dlversion` stamp answers "which release of *our own* format code wrote
this", and a directory running only the inherited formats was never
touched by that code.

A directory used to be able to be mixed — `--player-format=yaml` with
`--world-format=classic`, say — and it still carried exactly one
`.dlversion` at its root, because the stamp describes the yaml-format code
that wrote it rather than which subsystems happened to be pointed at it.
Since yaml-only there is no mixing left to do: the server reads `yaml`
throughout, and a partially converted directory is one `dlctl` produced
mid-migration rather than one anything runs. **The rule the mixed case
established is what still matters, and it did not change**: one stamp per
directory, describing the writer, not an inventory of which subsystems
have been converted so far. A later `dlctl import --type=...` filling in a
subsystem the last one skipped does not need a new stamp for that reason
alone.

## 4. The file itself

```yaml
schema: dl/dataversion@1
format: yaml
version: 1.4.2
```

Same shape §10.1 gives every other document in the tree (`schema:` first,
strict decoding — an unrecognised field is an error, per §10.2). `format` is
currently always `yaml`; it exists so a second future format (§0 of
`data-format.md` already reserves the name `yaml` "as opposed to `native`"
precisely because more than one is expected eventually) has somewhere to
say which versioning scheme it is using, without this file's own shape
having to change to make room.

`internal/persist/dataversion.Current` is not a constant to maintain: it
reduces `internal/buildinfo`'s version string to a semver. The Makefile
stamps that string in with `-ldflags` from `git describe --tags --always
--dirty`, so it arrives in any of several shapes — `v1.2.3` from the tag
itself, `v1.2.3-4-gabc1234` four commits later, either of those with
`-dirty` appended. All of them name release `1.2.3`, which is what
`Current` returns: the release a build descends from is the only release
its source has a name for, and a commit that changed the format without
tagging one has not made a claim about compatibility yet.

Nothing anywhere needs a new case when the number moves. `Check` compares
two `Version`s; no `switch` grows a branch for "1.3" versus "1.2".

## 5. What actually happens today

Built:

- `internal/persist/dataversion` — `Version`, `Parse`, `Current`, `Read`,
  `Check`, `CheckBuild`, `Write`.
- `cmd/dlmud`'s boot sequence calls `CheckBuild(cfg.LibDir)` once, right
  after logging starts and before any store opens — a fatal error on a
  major mismatch, a logged warning on a minor one, silence otherwise.
- `dlctl import` (no `--type`) calls `Write(--to-dir, Current)` once the
  whole conversion has succeeded, which is what puts a stamp on a directory in
  the ordinary course of things. It stamps nothing if any step failed
  (a half-converted directory should not claim to be a release's output)
  and nothing if this build has no release version (§6), saying so in
  both cases.
- `dlctl import --stamp-version=X` writes `X` instead of `Current`. It
  overrides the *number* and never the qualification: a failed step or a
  failed `verify` still refuses the stamp, exactly as without it. It takes
  anything `git describe --tags` prints — `v1.0.0`, `1.0.0`,
  `v1.0.0-4-gabc1234` all name release 1.0.0, via
  `dataversion.ParseRelease` — and refuses anything else *before* the
  conversion runs, so a typo costs a message rather than an import. See
  §6 for what it is for.
- `test/play` builds its server with `-ldflags` for this reason
  specifically, so that its child process *has* a release version and
  actually performs the check. `test/play/dataversion_test.go` is the
  only place the boot refusal is exercised end to end, as a process that
  exits with something to read: `internal/persist/dataversion`'s own
  tests cover the comparison, and cannot cover `cmd/dlmud` reaching it.
- `dlctl data version` — bare, prints what release this build is;
  `--dir=X` reports `X`'s own stamp and the same verdict `CheckBuild`
  would reach, without booting anything; `--write` (with `--dir`) stamps
  a directory with `Current` — the adoption path for one that predates
  the mechanism, and the migration path across a major bump.

Not built, and worth naming rather than leaving to be discovered missing:

- **Only `import` with no `--type` stamps.** The single-subsystem
  converters (`dlctl import --type=world/pfile/state/names/messages/
  socials/help`) and `fmt` do not call `dataversion.Write`, because none
  of them produces a whole data directory — each writes one corner of
  one, often into a directory another release built. `import` with no
  `--type` is the one command whose output is a complete `--lib-dir`, so
  it is the one command that can honestly say which release wrote the
  whole thing. Running a converter by hand leaves the stamp alone;
  `dlctl data version --dir=X --write` is how you update it deliberately.
- **The minor-version warning is generic.** It names the two releases and
  points at `dlctl data version`, but it cannot say *what* changed
  between them — and under a release semver it usually cannot, because
  usually nothing did (§1.1). Making it specific would mean the release
  process recording, per version, whether the format moved at all, which
  is the hand-maintained number §1.1 declined. The warning is deliberately
  a prompt to go and look, not a diagnosis.
- **No downgrade or migration tool beyond restamping.** `dlctl data
  version --write` sets the number; nothing rewrites the *files* across a
  major bump. That is fine exactly as long as major bumps do not actually
  change the format, which is the common case under §1.1 and cannot be
  relied on. Whichever release first bumps major *and* changes the yaml
  packages owes a real converter — nothing here plays the part `export
  --type=world` is meant to play for `yaml → classic` (`data-format.md`
  §10.4, itself not built either) — and this is where its absence is
  recorded until then. Whether a downgrade is even possible depends
  entirely on what that bump turns out to remove.
- **The example worlds are not stamped.** `examples/stock/yaml` and
  `examples/mini/yaml` are checked in, and regenerated by `scripts/
  release.sh` and `release.yml` using `go run ./cmd/dlctl` — an
  unreleased build, which writes no stamp. So neither carries one, and
  both drift checks pass `-x .dlversion` so that a regeneration run from
  a *released* `dlctl` does not look like world data changed. A shipped
  fixture pinned to whichever release happened to build it would be a
  standing source of exactly the false alarm §1.1 already concedes.

## 6. Unreleased builds have no version, and enforce nothing

`go run`, `go test`, `go install`, a plain `go build` — none of them pass
the Makefile's `-ldflags`, so `buildinfo` reports `devel` and `Current`
returns no version at all. This is not a rare corner: it is every test in
this repo, and every `make run` a contributor does.

Such a build has nothing to compare against, so it does not pretend to:

- `dlctl import` (no `--type`) writes no stamp, and says so on the way out. It does
  not invent `0.0.0` — a made-up number would propagate into a real
  directory and later be *checked*, which is worse than no number.
- `dlctl data version --write` refuses, and tells you to use `make build`
  or a released binary.
- `dlmud` performs no compatibility check. If the directory carries a
  stamp, it logs one line saying the check was skipped and why — an
  unstamped directory read by an unversioned build is two halves of the
  same silence, and there is no finding to report.

The hole is real: a developer build will happily open a directory a
release would refuse. It is accepted because the alternative — an
unreleased build guessing at a version and enforcing it — turns a
development convenience into data corruption's plausible cover story, and
because the enforcement matters where operators are, which is on released
binaries.

### 6.1 `--stamp-version`, for when the operator knows and the build does not

The one place that reasoning does not reach is preparing the data
directory *for* a release. An archived `lib/` is converted once, by
whatever `dlctl` is to hand — realistically `go run ./cmd/dlctl`, or a
`go build` from a working tree — and the result is the directory a
released server will boot on. The build has no version; the operator
knows precisely which one they mean.

`dlctl import --stamp-version=1.0.0` is that, and only that. Every
argument above still holds — nothing is *guessed*, nothing is inferred
from a build that cannot know, and the number is not enforced by the
build that wrote it. What has changed is who is answering: an operator
naming a release on the command line is a different act from a binary
inventing one, and the second is the thing §6 refuses.

Two properties keep it from becoming the hole it is patching:

- **It cannot manufacture a qualification.** It sets the version written
  at the end of a conversion that has already succeeded and verified. A
  failed subsystem, or a `verify --against` that reports a difference,
  refuses the stamp with the flag exactly as without it — an operator who
  most wants a stamp is the one who most needs not to get one.
- **It cannot name something no release could be.** The value goes
  through the same reduction `Current` applies to `git describe` output,
  so `1.0` or `latest` is refused rather than stored.

`dlctl data version --write` deliberately does *not* have the same
override, and the asymmetry is the point: it restamps an existing
directory, where a wrong number rewrites a claim somebody may already be
relying on, whereas `import` stamps a directory it has just built from
scratch and verified in the same breath.

## 7. Relationship to the rest of the format's versioning

Three things now answer three different questions about the same tree,
deliberately not merged into one:

| Mechanism | Question it answers | Granularity | Checked |
|---|---|---|---|
| `schema: dl/<kind>@<major>` (§10.1) | Is this **one file** a shape this loader understands? | Per file, per document kind | Every time that file is read |
| `.dlversion` (this document) | Was this **directory** written by tooling of the same generation as the build about to read it? | Once, for the whole directory | Once, at boot (or on demand via `dlctl data version`) |
| `dlctl lint --type=world` / `dlctl data verify` (`data-format.md` §10.4, the latter not built) | Does this directory's **content** actually satisfy its own schema — references resolve, flags are known, nothing is silently wrong? | Whole directory, content-level | On demand, offline |

A schema-major bump on one document kind and a project-major bump are not
the same event and need not coincide in either direction. A schema major
can move inside a minor release, and a major release can happen with the
schemas untouched — which, under §1.1, is the usual case. The two are
related only in one direction, and it is worth stating: **a change that
breaks a document's schema major is a change that must not ship in a
minor release**, because `.dlversion` would then wave through two builds
that genuinely disagree about the files, and the per-file `schema:` tag
would be left to catch it one file at a time partway through boot — which
it will, but as a failed start rather than a refused one. The three-row
table above is the map for telling the mechanisms apart; that sentence is
the one rule connecting them.
