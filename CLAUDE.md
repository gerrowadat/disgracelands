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
across into Go is unreliable — not occasionally, but routinely. Thirty-six
findings in `docs/weirdnumbers.md` were found by testing, and several of them
had already been transcribed wrongly by eye first.

Two patterns catch what reading misses. Use them.

**C oracles.** `reference/tools/*.c` holds original C function bodies, with
`char_data` dereferences substituted for plain parameters, compiled and run
against the Go. Existing ones cover the RNG (30,000 draws over 6 seeds),
to-hit (1,512,000 values), regeneration (36,288), saving throws (1,125) and
DES crypt (9,680 pairs). Adding one is cheap; see `reference/tools/README.md`.

**Table re-parsing.** Where the C holds data in a table, the test re-parses
the C source and compares entry by entry: `constants.c`, `spec_assign.c`,
`interpreter.c`, `handler.c`. This is why the command table is sorted by
`interpreter.c` line number — abbreviation behaviour is *derived* from the C
rather than asserted about it, so a command inserted in the wrong place fails
a test instead of quietly shadowing another.

If you are porting something numeric or table-driven and you are not doing one
of these two things, expect to be wrong.

## Layout of the tree

| Path | What lives there |
| --- | --- |
| `internal/game` | The world and the rules. Pure logic, no I/O, no sessions. |
| `internal/session` | Commands and the login/menu state machine. |
| `internal/server` | The world goroutine, ticks, and the integration tests. |
| `internal/persist/world` | Zone/mob/obj/shop file readers. |
| `internal/persist/player` | The roster and the rent files, behind interfaces. |
| `reference/` | The original C, the modernised C build, and the oracle tools. |
| `data/` | The real game data, as it was. |

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
recurred at least eight times.

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
pattern.

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

`.github/workflows/go.yml` runs lint, test, parity, licence and container
jobs. The **32-bit toolchain is installed only when the change can affect the
ILP32 layout** — `gcc-multilib` was hanging the runner otherwise. The gate is
a path filter in the `ilp32` step: if you add a reference tool or a source
file that feeds the binary layouts, add it to that regex or your layout check
silently will not run.

The licence check (`scripts/license-check.sh`) is not decorative: CircleMUD's
licence is non-commercial and requires credits intact and the creators named
in the greeting.

## Where to write things down

- A surprising constant or formula, with its C citation →
  `docs/weirdnumbers.md`.
- A deliberate difference from the C → `docs/deviations.md`, with reasoning.
- Something not yet ported, that a reader would expect to be →
  `docs/deviations.md` too, so the gap is visible.
- Phase and slice status → `docs/proposals/go-port-plan.md`. Keep it current;
  it is the map future work is planned from.
