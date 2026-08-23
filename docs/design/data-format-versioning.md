# Versioning the yaml data format

`docs/design/data-format.md` §10.1 already tags every individual file with
`schema: dl/<kind>@<major>` — what *shape* one file is in. This document
is the layer above that: one `major.minor.patch` for the yaml packages as
a whole, stamped once per data directory, checked once at boot rather than
once per file. It is what lets a change to `internal/persist/*/yaml` say,
truthfully, how much of a problem it is for a directory a different build
wrote.

Implemented in `internal/persist/dataversion` and wired into `dlmud`'s
boot sequence and `dlctl data version`.

---

## 0. Decisions taken

| Question | Decision |
|---|---|
| **Shape** | **`major.minor.patch`, semver in spirit.** Three tiers, each with a different consequence, rather than the single incrementing integer §10.1 uses per file — a directory-level number has to describe *compatibility*, not just "newer/older", because unlike one file's shape, a whole yaml tree can drift in ways that are fine to run on and ways that are not. |
| **Where it lives** | **One file, `.dlversion`, at the root of the directory `--lib-dir` names.** Not per-subsystem. `PlayerFormat`, `WorldFormat`, `StateFormat` and the rest are independently selectable, but the code that implements `yaml` for all of them ships as one build of this one repo — there is no meaningful sense in which `internal/persist/player/yaml` is "version 1.2.0" while `internal/persist/world/yaml` is "version 1.4.0" in the same checkout. One number, describing what release of this codebase's yaml support last touched the directory. |
| **What owns the check** | **The server refuses to boot on a major mismatch; a linter answers the question ahead of time.** `dataversion.Check` is the one function both call — `cmd/dlmud`'s boot sequence, for the consequence that matters, and `dlctl data version`, so an operator can ask "will this start?" without starting it. |
| **What bumps which tier** | **Major: an older server would misread the data, not just fail to understand part of it — the same bar §10.1 sets for one file's own schema major, applied to the directory as a whole (a top-level directory renamed, a file split in two, `.dlversion` itself changing shape).** Minor: additive — a new optional field, a new document kind, a new flag value understood — nothing an older reader loses data over, only fails to take advantage of. Patch: no schema or behaviour change a reader could observe at all — a writer bug fixed, a validation message reworded, a default corrected to match what was already documented. |

---

## 1. Why this needs its own number

Fidelity to the C matters for what the *game* does; it says nothing about
what a *data format this project invented* should promise its own
tooling. Nothing in `reference/` has an opinion here — there is no C
citation for this document, and that is expected.

The problem `dataversion` answers: `data/` is not read by one program.
`dlmud` reads it live. `dlctl world/pfile/state/names/messages/socials/
helpdb import` and `fmt` read and write it offline. A build from six
months ago and a build from today can both point at the same directory —
an operator upgrading in place, a CI job checking out an older tag against
a fixture, a contributor with two worktrees. §10.1's per-file tag already
protects each individual file's *shape*; what it does not answer is "is
this whole directory, as a body of work, safe for this build to run on at
all" — a question that needs an answer before the first file is even
opened, not discovered one `schema:` mismatch at a time partway through
boot.

## 2. The three tiers, precisely

Given a directory stamped `D` and a running build that understands `S`
(`dataversion.Current`):

| Comparison | Outcome | Who says so |
|---|---|---|
| `D.Major > S.Major` | **Refuses to boot.** This build may misread the data outright — the same reasoning §10.1 already applies per file, now applied to the directory as a whole. | `dataversion.Check` returns a non-nil `error`; `cmd/dlmud`'s `run` returns it, which is a fatal exit before any store opens. |
| `D.Major == S.Major`, `D.Minor > S.Minor` | **Boots anyway, at your own risk.** Nothing refuses to read; some of what the directory contains may go unrecognised — an unknown-but-additive field, a document kind this build has never heard of. | `dataversion.Check` returns a non-empty warning and a nil error; `cmd/dlmud` logs it and continues. `dlctl data version --dir=…` prints the same finding without starting anything. |
| `D.Major == S.Major`, `D.Minor <= S.Minor` (any `Patch`) | **Fully compatible, silently.** By construction: a patch bump changes nothing a reader could observe, and a minor at or below what this build already knows introduces nothing this build has not already accounted for. | `dataversion.Check` returns `("", nil)`. |
| `D.Major < S.Major` | **Fully compatible, silently — for now.** The ordinary case: every directory starts at the version whatever server last wrote it understood, which is never newer than whatever ships tomorrow. Nothing has changed major yet, so there is no real "can a new build still read old-major data" question to have answered wrong. Whichever future change earns the first major bump has to answer it, honestly, at the time — this document does not pre-decide it. | `dataversion.Check` returns `("", nil)`; noted here so the gap is visible rather than silently assumed away. |
| No `.dlversion` present | **Fully compatible, silently.** Every directory that predates this mechanism, and every one running only `classic`/`ascii`/`binary` — all of which are unversioned by design (§3) — looks like this. | Same as the row above. |

## 3. Why `classic`/`ascii`/`binary` are not versioned

They are the original CircleMUD formats, ported as read/write oracles
(`docs/deviations.md`'s "not deviations" framing), not designs this
project owns. Their shape is whatever `struct char_file_u` or
`fread_string` already fixed in 1993–2008 — there is nothing here to
version because there is no future revision of `classic` coming. A `.
dlversion` stamp answers "which release of *our own* format code wrote
this", and a directory running only the inherited formats was never
touched by that code.

A directory can be mixed — `--player-format=yaml` with `--world-format=
classic`, say — and still carry exactly one `.dlversion` at its root: the
stamp describes the yaml-format code available, not which subsystems
happen to be pointed at it right now. A future run with more of its
flags set to `yaml` does not need a new stamp for that reason alone.

## 4. The file itself

```yaml
schema: dl/dataversion@1
format: yaml
version: 1.0.0
```

Same shape §10.1 gives every other document in the tree (`schema:` first,
strict decoding — an unrecognised field is an error, per §10.2). `format`
is currently always `yaml`; it exists so a second future format (§0 of
`data-format.md` already reserves the name `yaml` "as opposed to `native`"
precisely because more than one is expected eventually) has somewhere to
say which versioning scheme it is using, without this file's own shape
having to change to make room.

`internal/persist/dataversion.Current` is the version compiled into this
build — `1.0.0` today, because there has been exactly one version so far
and this document is what establishes it. Bump `Current` — and only
`Current` — when a change to `internal/persist/*/yaml` earns one, by §0's
own rule for which tier. Nothing else in the mechanism changes: `Check`
already knows what to do with a new number, and no `switch` anywhere
needs a new case for "1.1" versus "1.0".

## 5. What actually happens today

Built:

- `internal/persist/dataversion` — `Version`, `Parse`, `Check`, `Write`,
  `Current`.
- `cmd/dlmud`'s boot sequence calls `Check(cfg.LibDir, dataversion.
  Current)` once, right after logging starts and before any store opens
  — a fatal error on a major mismatch, a logged warning on a minor one,
  silence otherwise.
- `dlctl data version` — bare, prints what this build understands;
  `--dir=X` reports `X`'s own stamp and the same verdict `Check` would
  reach, without booting anything; `--write` (with `--dir`) stamps a
  directory with `Current` — the adoption path for one that predates the
  mechanism, or a fixture a test wants pinned.

Not built, and worth naming rather than leaving to be discovered missing:

- **Nothing stamps a directory automatically.** `dlctl world/pfile/state/
  …/fmt` and `import` do not call `dataversion.Write` yet. Adopting the
  stamp today is a manual `dlctl data version --dir=data --write` after
  converting — reasonable while `Current` has only ever been `1.0.0` and
  there is nothing yet for an unstamped directory to be behind on, less
  reasonable once a second version exists. Wiring `Write` into the
  canonicalising tools is the natural next step the *first* minor or
  major bump should bring with it, not before — building the write path
  now, against a version number that has never changed, would be
  exactly the kind of untested machinery this project's own testing
  discipline (`CLAUDE.md`, "do not read the C and transcribe it" — the
  same caution applies to inventing a format nothing has exercised yet)
  warns against.
- **The minor-version warning is generic.** "This server only knows
  1.0.0" names the two versions and points at `dlctl data version`, but
  neither of them can yet say *what specifically* changed between them —
  because nothing has. The design leaves room for exactly that: the
  intended shape, once there is a second minor version to describe, is a
  small table in `internal/persist/dataversion` mapping each past minor
  bump to a one-line note of what it added, which `Check`'s warning and
  `dlctl data version`'s report both read from — populated by whoever
  earns the *next* minor bump, as part of earning it, not spoken for in
  advance.
- **No downgrade tool.** A directory stamped with a newer major than a
  given build understands has no `dlctl` command to bring it back —
  unlike `world export`'s intended role for `yaml → classic` (`data-
  format.md` §10.4, itself not built yet either). Whether a downgrade is
  even possible depends entirely on what the eventual first major bump
  turns out to remove, so there is nothing concrete to build against yet.

## 6. Relationship to the rest of the format's versioning

Three things now answer three different questions about the same tree,
deliberately not merged into one:

| Mechanism | Question it answers | Granularity | Checked |
|---|---|---|---|
| `schema: dl/<kind>@<major>` (§10.1) | Is this **one file** a shape this loader understands? | Per file, per document kind | Every time that file is read |
| `.dlversion` (this document) | Is this **directory**, as a release of this project's own format code, safe for this build to run on at all? | Once, for the whole directory | Once, at boot (or on demand via `dlctl data version`) |
| `dlctl world lint` / `dlctl data verify` (`data-format.md` §10.4, the latter not built) | Does this directory's **content** actually satisfy its own schema — references resolve, flags are known, nothing is silently wrong? | Whole directory, content-level | On demand, offline |

A future schema-major bump on one document kind and a future
`dataversion` major bump are not the same event and do not have to
coincide: most schema-major bumps will be additive at the directory level
(a new, optional-to-omit document kind existing somewhere is a minor
bump; an existing kind's shape changing incompatibly is a major bump at
*both* levels, because that is what "an older loader would misread it"
means at either granularity). The three-row table above is the map for
telling them apart when the time comes.
