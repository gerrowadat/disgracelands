# Working on Disgracelands

Notes for anyone — human or agent — picking this up cold. The authoritative
documents are `docs/proposals/go-port-plan.md` (how the port got here, the
architecture it built, and — in its §10 Phase 7 — what is left),
`docs/proposals/yaml-only.md` (why the server reads one on-disk format, and
the compatibility contract that came with it), `docs/deviations.md` (every
deliberate difference from the C) and `docs/weirdnumbers.md` (every
surprising constant, with its C citation). This file is the working
practice around them.

**Where the next thing starts, as of 2026-08-29.** `yaml-only.md` was the
forward plan and its build is finished: rows 1–6 of its §7 have landed, and
the row left is cutting the v1.0.0 release, which is a decision rather than
a task. So the forward plan is `go-port-plan.md` §10's **Phase 7
(cutover)** again — it was downstream of the yaml-only work and no longer
is — and its preconditions are the map. The day-to-day work in front of
that is not in either document: it is the **open GitHub issues**, and
`docs/deviations.md`'s "Not deviations — gaps still to fill" and "What the
session-parity suite found", which is where issues get filed from. Read
those as the todo list and the proposals for what the work is for.

## What this project is

Disgracelands was a MUD played from 2001 to 2008: CircleMUD 3.0 bpl20, plus
OasisOLC, plus a pile of local modifications. `reference/` holds the original
source. We ported it to Go, and as of 2026-08-23 that port is playable.

**Through Phase 5, the governing rule was fidelity (plan §0): where the
patched C server and modern good design disagreed, the C won.** The bugs
were part of it — if the C computed something in a way that looked wrong,
the port computed it that way too, with the reason in `docs/weirdnumbers.md`
and a `file:line`. That work is done, it stays done, and nothing already
ported gets revisited on fidelity grounds alone — the practices below still
protect it.

**Now that the port is playable, that rule narrows (plan §0, "Fidelity,
phase two").** New work is free to diverge from the C server to modernise
the implementation — architecture, dependencies, protocols, tooling — with
no sign-off and no entry anywhere required for that alone. Two things stay
fixed: **compatibility** (the on-disk formats, `--lib-dir` contents and
archived credentials this repo already reads and writes) and **gameplay**
(the mechanics and balance a returning player would recognise). A change
that touches either of those is a deviation and goes in
`docs/deviations.md` with its reasoning, exactly as before; a change that
only touches implementation does not. If you're not sure which bucket a
change is in, err on the side of writing it down — the file exists so that
question never has to be settled from memory later.

## Do not read the C and transcribe it

This still applies in full to anything gameplay- or compatibility-shaped —
touching combat, spells, saves, regen, the RNG, or any on-disk or wire
format. It has nothing to say about work that's purely architecture or
tooling and doesn't change what the C computed or stored, which is exactly
the kind of change the phase-two fidelity narrowing above now allows without
this machinery.

This is the single most useful thing learned so far. Reading C arithmetic
across into Go is unreliable — not occasionally, but routinely. All 57
findings in `docs/weirdnumbers.md` were found by testing, and several of them
had already been transcribed wrongly by eye first.

Three patterns catch what reading misses. Use them.

**C oracles.** `reference/tools/*.c` holds original C function bodies, with
`char_data` dereferences substituted for plain parameters, compiled and run
against the Go. Existing ones cover the RNG (30,000 draws over 6 seeds),
to-hit (1,512,000 values), regeneration (36,288), saving throws (1,125), DES
crypt (9,680 pairs), shop prices (which need `-m32 -mfpmath=387`, because the
answer depends on the width the multiplication happens at), `isname`/
`get_number` (168 pairings) and the improved line editor's eleven commands
(805 command-against-buffer cases, and built `-O0` on purpose: one of the C's
`sprintf`s has its destination as its own `%s` argument, and modern gcc
resolves that undefined behaviour differently from the compiler the archived
server used). Adding one is cheap; see `reference/tools/README.md`.

**Not only arithmetic.** `isname` has no numbers in it and was still read wrong
for four phases — its loop has the shape of a prefix match and the semantics of
a whole-word one, so `get swo` picked up a sword here and never did on the real
server. If you would have to simulate a function in your head to be sure of it,
that is the trigger.

**Table re-parsing.** Where the C holds data in a table, the test re-parses
the C source and compares entry by entry: `class.c`, `constants.c`,
`interpreter.c`, `spec_assign.c`, `handler.c`, `spells.h`, and `act.wizard.c`
for `do_set`'s field table. This is why the command table is sorted by
`interpreter.c` line number — abbreviation behaviour is *derived* from the C
rather than asserted about it, so a command inserted in the wrong place fails
a test instead of quietly shadowing another.

**Layout tools.** Where an on-disk format is an `fwrite` of a struct — the
pfile, the rent files, the boards, the mail file, the house control file — the
format *is* the memory layout. `reference/tools/*layout.c` prints the offsets
gcc chose and a test requires the Go codec to reproduce them, under both data
models. Never hard-code an offset.

If you are porting something numeric, table-driven or struct-shaped and you
are not doing one of these three things, expect to be wrong.

## Layout of the tree

| Path | What lives there |
| --- | --- |
| `internal/game` | The world and the rules. Pure logic, no I/O, no sessions. |
| `internal/session` | Commands and the login/menu state machine. |
| `internal/server` | The world goroutine, ticks, and the integration tests. |
| `test/play` | The play regression suite: a real `dlmud`, booted on `examples/mini`, driven over a socket. Build-tagged `play`, release-only, `make play`. |
| `test/parity` | The session-parity suite: the same scripts typed at *both* servers, transcripts compared. Build-tagged `parity`, release-only, `make session-parity`. |
| `internal/persist/world` | Zone/mob/obj/shop file readers. |
| `internal/persist/player` | The roster and the rent files, behind interfaces. |
| `reference/` | The original C, the modernised C build, and the oracle tools. |
| `examples/stock/` | Stock CircleMUD's own world, checked in as both classic and yaml. The `yaml/` one is the server's default `--lib-dir`; the `binary/` one is what the C server reads and what the parity harnesses stage from. The real game data, as it was, never ships here; see `docs/investigations/`. |
| `examples/torture/` | A deliberately hostile legacy `lib/` and its import: the compatibility corpus, per `yaml-only.md` §5.1. Add to it when a conversion bug is found. |

### The seams that matter

- **`session.Violence`** — one damage path for every command that can kill.
  Do not apply damage anywhere else; that hole was closed deliberately.
- **`session.Special` / `SpecialCall`** — the specproc seam (plan §8's
  Trigger), in the shape the built-ins actually need.
- **`Command.CLine`** — see above; the table's order is the C's.
- **`game.Act`** — the `perform_act` port. All 17 `$`-codes, resolved *per
  audience*. Anything said to more than one person goes through it.
- **`PlayerRecord.Worn`** back-pointer, so `affect_total` can see equipment;
  **`PlayerRecord.Mobile`** for the 18-vs-25 ability clamp.
- **`internal/persist/player/binary/layout.go`** — the pfile and the rent
  files are raw `fwrite`s of structs, so the *format is the memory layout*.
  Offsets are computed from a declaration plus a data model (ILP32 for the
  real archived data, LP64 for a modern rebuild), never hard-coded.

## Concurrency

The world is owned by a single goroutine. Everything that touches it goes
through `engine.DoSync`. `-race` is mandatory and its findings are never
flaky-until-proven — a race that reproduces one run in ten is still a race,
and at least one such was a genuine bug in the test suite rather than the
server.

Saves are pushed off the world goroutine on purpose so a slow disk cannot
stall the game.

## Traps in the test suite

These have each cost real time. They are all still easy to fall into.

**Never call `t.Fatal`, `t.Skip` or `t.FailNow` inside an `inWorld` closure.**
They call `runtime.Goexit`, which kills the *world* goroutine. Every
subsequent `DoSync` then blocks until the test binary times out, with nothing
pointing at the test that did it. Read values out of the closure and assert
outside it. `inWorld`'s doc comment says this in capitals; believe it.

**`expect` returns immediately if the marker is already in the transcript.**
So an assertion written after a second command happily matches the *first*
command's reply. Use `expectCount` for the n'th occurrence, or `settle()`
(sends `time`, waits for one more `o'clock`) to drain first. This one has
recurred at least nine times.

**`expect` is not a barrier for anybody else's buffer.** It waits for a write
to *this* client's socket; messages to other characters in the room are
separate writes on the world goroutine. Seeing your own reply does not mean
theirs has happened. Call `settle()` before reading another client's or
recorder's lines — otherwise the test passes locally and fails on a busier
machine, which is how this was found.

**Anything that happens after a command's reply needs its own wait.** `quit`
prints "Goodbye." from the command, but the disconnect handling — the save,
the crash-save, removing the character from the world — runs afterwards in
the connection goroutine's teardown. `waitForLogout` in `rent_test.go` is the
pattern; `eventually` in `client_test.go` is the general one, for a record
written to disk or a file appearing.

Be careful about *which* signal you wait on. Renting removes the character
from the world on the world goroutine and saves afterwards, off it — so
"gone from the world" does not mean "record written".

`srv.WaitForWrites()` is the barrier for anything written by a background
save: it waits for every counted write goroutine to finish. Prefer it to
polling with `eventually` when what you are waiting for is a file this server
wrote — but only once something has already synchronized with the world
goroutine first (`waitForLogout`, `inWorld`, or the like). Calling it right
after a socket-level `c.expect(...)` is itself a race: the world goroutine's
`background()` call (the `s.writes.Add(1)`) and the client noticing the
command's reply have no ordering relationship, so the test's own
`WaitForWrites()` can race that `Add(1)` — `sync.WaitGroup`'s own documented
unsafe case, "Add with the counter at zero, concurrent with Wait", and the
`-race` detector finds it exactly as reliably as any other data race (found
while adding a board format slow enough — a YAML marshal versus a raw byte
encode — to make the window wide enough to hit; the same call shape had been
sitting in `wizset_test.go` unnoticed). Reach for `eventually` polling the
state directly when there is no cheaper barrier to synchronize on first.

**Every background write must go through `Server.background`.** A bare
`go func()` that writes into a test's `t.TempDir()` outlives the test, and
the cleanup then fails with "directory not empty" — reported against
*whichever other test the scheduler was running by then*, which is why this
looked for a while like RNG flakiness somewhere else entirely. The same
counter is what stops a shutdown losing a save that was in flight.

**When a test and the implementation disagree, check the C before assuming
the implementation is wrong.** Five `act()` expectations turned out to be
wrong and the code right: `$o` is the keyword, not the short description;
`SANA` reads the keyword list; `$u` acts on the word *already written*; the
formats have no trailing period.

## Workflow

- **Always branch → PR → merge. Never commit direct to `main`.**
- **Assume the worktree you're handed is stale, and branch from
  `origin/main`, not from whatever is checked out.** A worktree can be
  sitting on a branch whose work already merged — squash-merged, so its
  commit is *not* an ancestor of `main` even though its tree is
  byte-identical to it. `git diff HEAD origin/main` coming up empty proves
  the content matches; it proves nothing about ancestry. Branching from
  that stale HEAD carries its old commit along as if it were new, and a PR
  opened from it shows that commit's whole diff a second time, on top of
  the actual change — GitHub computes the diff against the merge-base, and
  if the old commit was never really in `main`'s history the merge-base
  lands before it. Found the hard way on #184: `git diff` said clean,
  `git log origin/main..HEAD` did not, and the fix was `git fetch && git
  checkout -b <name> origin/main` before making the change at all —
  which is the rule now, every time, not just when something looks off.
  Mid-branch, `git rebase --onto origin/main <old-base>` is the recovery
  if this has already happened.
- Commit messages are long prose: what the change does, *why the C does it
  that way*, and `file:line` citations. Name the things that cost you time —
  the traps above are all in commit messages first.
- Run before pushing:
  ```
  make lint                       # must be 0 issues
  go test -race -count=1 ./...    # must be green, and re-run if a race appears
  make ci                         # runs go.yml itself, in containers, via act -- see below
  ```
- Wait for CI green, then squash-merge and delete the branch.

### CI

**Verify locally with `act`, not by pushing and watching GitHub.** GitHub
Actions is where a push or PR gets its *final* answer, not where it should
be discovered for the first time — `make ci` runs the actual
`.github/workflows/go.yml` file, in containers, via
[`act`](https://github.com/nektos/act) (`docs/developer.md` has the full
detail: `.actrc`'s defaults, the `act-*` volume-staleness trap, `make
ci-clean`). Run it before pushing, the same way `make lint`/`go test
-race` already are — a green `make ci` locally is what a green PR check
should be confirming, not the first place either gets checked. This
applies to an agent working on this repo exactly as much as it does to a
human: do not treat GitHub's own run as the place to find out whether
something works.

This is also a scope rule, not just a practice one: **day-to-day GitHub
Actions runs (`go.yml`, every push and pull request) are correctness and
lint only — build, vet, gofmt, lint, `go test -race`. Nothing broader runs
there, ever, except when a release is actually being cut.** `.github/
workflows/release.yml` runs only when a release is being cut, and runs
everything else — the
32-bit ILP32/shop-price checks (`gcc-multilib`, installed unconditionally
there, not path-filtered: a release is exactly the point where "probably
didn't touch the layout code" stops being good enough), the C-vs-Go
world-parity check, the licence check, three doc-coverage checks, a check
that `examples/stock/yaml`/`examples/mini/yaml` still match a fresh
`dlctl import` of their binary source, the session-parity suite
(`test/parity` — added there 2026-08-29, having run in no workflow at all
until five of its `known` entries went stale unread for two months, #268),
a container build, and a
cross-compile of both binaries for every published platform
(`linux/amd64`, `linux/arm64`, `windows/amd64`), which is also what
produces the archives attached to the release —
`scripts/build-dist.sh`, or `make dist` locally. That cross-compile is
the only thing in the tree that builds for anything but the host: `go
test ./...` cannot notice a Windows build break and neither can `make
check`, so a change touching `syscall`, file paths or process handling
is one worth running `make dist` after.
`scripts/release.sh` (`make release BUMP=patch`) is what actually cuts a
release: it works out the next semver version, regenerates the example
yaml worlds if they have drifted, runs the local checks, pushes `main`,
then dispatches `release.yml` with that version and waits for it.
**It does not tag.** `release.yml`'s `publish` job creates the tag, the
GitHub release and the notes, and its `image` job pushes to ghcr.io; both
are `needs:`-gated on the full suite, so a failed release leaves no tag,
no release, no notes and no package behind, and the version number is
still free for the next attempt. (Before 2026-08-24 it was the other way
round — tag first, tag push triggers the workflow — so every failed
release run left a real `v*.*.*` tag on a commit that had just been proved
not to release.) The tag-push trigger is still wired up, for re-verifying
a commit that is already tagged.

**The play regression suite is release-only too, and for a reason worth
knowing.** `test/play` (`make play`) builds `cmd/dlmud`, boots it on a
throwaway copy of `examples/mini`, and drives it over a real socket with
a client that types what a player types. It is the only thing in the
tree that runs the boot sequence at all — reading the world off disk,
resetting zones into it, attaching specials by vnum, loading `text/`,
parsing a flag, handling a signal — because `internal/server`'s tests
build their world in Go and wire the server up field by field. That is
what makes those tests fast and what makes them blind: a world that no
longer loads, a zone that no longer populates, a special that stopped
being assigned, or a converted directory that lost a file all pass every
test in `internal/server` and fail the first thing a player types. It
found six bugs before it was finished being written, including a
shutdown that saved nobody and a missing `perform_dupe_check` that let
one character be logged in twice, as two bodies, saving over each
other. **Add to it when you add a feature a player can type**;
`docs/developer.md` has the how.

If a check feels like it is missing from day-to-day CI, that is very
likely correct, not an oversight — it moved to `release.yml` on purpose
(2026-08-23, the same change that added this rule). **Do not add it back
to `go.yml`, and do not "run the release checks too, just to be safe" on
an ordinary PR** — run `make ci-job JOB=full-suite
CI_WORKFLOW=.github/workflows/release.yml` (or just `make check`/`make
parity` directly) locally instead, if a change genuinely needs that
broader verification before it lands. Cutting a real release
(`scripts/release.sh`) is the only thing that should trigger `release.yml`
on GitHub itself.

The licence check (`scripts/license-check.sh`) is not decorative: CircleMUD's
licence is non-commercial and requires credits intact and the creators named
in the greeting. It is **split across both workflows**, and the split is the
point:

- `go.yml`, every push and PR, runs `--notices` alone — check 3, that every
  file written for this project carries its notice. One grep over files git
  already lists; no C tree, no baseline diff.
- `release.yml` runs all five, as part of the release-only suite.

That asymmetry is the whole reasoning. A *newly added file* is what fails
the notice check, so it goes wrong on ordinary PRs — which is every PR —
whereas the other four go wrong only when someone edits `LICENSE`, the
credits or the greeting, which is almost never. The original 2026-08-23
split moved all five to release time on the reasoning that the check
"almost never trips", with a note to move it back if that stopped holding.
It stopped holding on 2026-08-24: #164 merged green with
`scripts/act-guard.sh` missing its notice, and the v0.1.0 release attempt
was what found it. **Only the notice loop came back — do not read this as
the day-to-day/release split being reversible piecemeal.**

## Where to write things down

- A surprising constant or formula, with its C citation →
  `docs/weirdnumbers.md`.
- A deliberate difference from the C → `docs/deviations.md`, with reasoning.
- Something not yet ported, that a reader would expect to be →
  `docs/deviations.md` too, so the gap is visible.
- A bug or a gap in the Go server → a GitHub issue, plus a doc entry for
  the reasoning if there is any. See "The Go server is canonical now"
  below; the issues are the todo list.
- Status of work still to come → `docs/proposals/go-port-plan.md` §10's
  Phase 7 preconditions, which is the forward plan again now that
  `yaml-only.md`'s build is done. Correct both when they turn out to be
  wrong about what landed; neither is extended with new plans.
- The on-disk format's own compatibility contract — what a new yaml field
  owes, what "the conversion is exactly dead on" means and how it is
  proved → `docs/proposals/yaml-only.md` §5 and §6, until somebody moves
  them into `data-format.md` where they belong.

**The Go server is canonical now, not the C.** Through Phase 5 a place
where the Go server fell short of the C was an artifact to note and move
past — the C was the target and the gap was expected, on the way to
closing it. That framing is gone. **A bug or a missing piece of
functionality in the Go server is filed as a GitHub issue
(`gh issue create`, labelled `bug` or `enhancement` to match the labels
already in use), not merely written down.** A doc entry records *why* a
difference exists; it does not get anything fixed, and stops being enough
once the C stops being the thing everyone actually plays. This does not
retire `deviations.md` or `weirdnumbers.md` — a *deliberate, reasoned*
difference from the C, or a surprising constant kept on purpose, still
belongs there exactly as before. It means a difference that is really a
bug or a gap gets both: the doc entry for the reasoning, if any, and an
issue for the fact that it needs fixing. `docs/deviations.md`'s own
"What the session-parity suite found" and "Not deviations — gaps still to
fill" sections are exactly where these have been hiding in plain sight —
several of their un-struck-through entries (a `quit` that disconnects
instead of returning to the menu, hit points rolled wrong at creation,
free movement, colour not reaching the C's own call sites, and so on)
were real bugs recorded as prose for months before finally becoming
issues #187-194 (2026-08-28). Read a "Blocker" or an un-fixed bullet
there as a todo list, not an archive.
