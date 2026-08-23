# Working on Disgracelands

Notes for anyone — human or agent — picking this up cold. The authoritative
documents are `docs/proposals/go-port-plan.md` (what we are doing and in what
order), `docs/deviations.md` (every deliberate difference from the C) and
`docs/weirdnumbers.md` (every surprising constant, with its C citation). This
file is the working practice around them.

## What this project is

Disgracelands was a MUD played from 2001 to 2008: CircleMUD 3.0 bpl20, plus
OasisOLC, plus a pile of local modifications. `reference/` holds the original
source. We are porting it to Go.

**The governing rule is fidelity (plan §0): where the patched C server and
modern good design disagree, the C wins.** The bugs are part of it. If the C
computes something in a way that looks wrong, the port computes it that way
too, and the reason goes in `docs/weirdnumbers.md` with a `file:line`.

The only exceptions are safety and correctness-of-the-port issues — a buffer
overrun, an unescaped path — and every one of those goes in
`docs/deviations.md` with its reasoning. If you are about to "fix" something
in the C's behaviour, you are about to make a deviation; write it down or
don't do it.

## Do not read the C and transcribe it

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
answer depends on the width the multiplication happens at) and `isname`/
`get_number` (168 pairings). Adding one is cheap; see
`reference/tools/README.md`.

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
| `internal/persist/world` | Zone/mob/obj/shop file readers. |
| `internal/persist/player` | The roster and the rent files, behind interfaces. |
| `reference/` | The original C, the modernised C build, and the oracle tools. |
| `examples/stock/` | Stock CircleMUD's own world, checked in as both classic and yaml — the server's default `--lib-dir`. The real game data, as it was, never ships here; see `docs/investigations/`. |

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
- Commit messages are long prose: what the change does, *why the C does it
  that way*, and `file:line` citations. Name the things that cost you time —
  the traps above are all in commit messages first.
- Run before pushing:
  ```
  make lint                       # must be 0 issues
  go test -race -count=1 ./...    # must be green, and re-run if a race appears
  ```
- Wait for CI green, then squash-merge and delete the branch.

### CI

Two workflows, split by how often each needs to run. `.github/workflows/
go.yml` runs on every push and pull request — build, vet, gofmt, lint,
`go test -race`. That is the whole of it: fast, and every commit gets an
answer. `.github/workflows/release.yml` runs only when a tag matching
`v*.*.*` is pushed (or by hand via `workflow_dispatch`) and runs
everything else — the 32-bit ILP32/shop-price checks (`gcc-multilib`,
installed unconditionally there, not path-filtered: a release is exactly
the point where "probably didn't touch the layout code" stops being good
enough), the C-vs-Go world-parity check, the licence check, two
doc-coverage checks, a check that `examples/stock/yaml`/`examples/mini/
yaml` still match a fresh `dlctl lib import` of their binary source, and
a container build. `scripts/release.sh` (`make release BUMP=patch`)
is what actually cuts a release: it bumps the semver tag, regenerates the
example yaml worlds if they have drifted, runs the fast local checks,
and pushes the tag that triggers `release.yml`.

`make ci` runs `go.yml` locally in containers via `act`; see
`docs/developer.md` for `release.yml`'s own local equivalent. Do not
assume a slow or flaky check belongs back in `go.yml` because it used to
live there — that consolidation is why the split exists at all.

The licence check (`scripts/license-check.sh`) is not decorative: CircleMUD's
licence is non-commercial and requires credits intact and the creators named
in the greeting. It runs at release time now, not on every push — a real
tradeoff (a stripped header could sit unnoticed for many commits between
releases), made deliberately because the check almost never trips and
releases are frequent enough that it does not sit long either way. Move it
back into `go.yml` if that assumption stops holding.

## Where to write things down

- A surprising constant or formula, with its C citation →
  `docs/weirdnumbers.md`.
- A deliberate difference from the C → `docs/deviations.md`, with reasoning.
- Something not yet ported, that a reader would expect to be →
  `docs/deviations.md` too, so the gap is visible.
- Phase and slice status → `docs/proposals/go-port-plan.md`. Keep it current;
  it is the map future work is planned from.
