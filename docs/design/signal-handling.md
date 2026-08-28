# Signal handling

**Decided 2026-08-24.** This is what the signals mean and why, settled as a
set rather than one decision at a time. The dispatcher
(`internal/signals`), `SIGTERM`/`SIGINT` shutdown, `SIGHUP` configuration
reload and the exit codes are built; §9 lists the items still to build
against this design, each of which is scope rather than an open question.
`docs/operations.md` is the operator-facing half — what to send and what it
does — and this is the reasoning under it.

Under the plan's §0 "Fidelity, phase two" this is implementation, not
gameplay and not compatibility: nothing here changes an on-disk format or
anything a returning player would notice, so none of it needs a
`docs/deviations.md` entry on fidelity grounds. The C is cited throughout
because it is the thing being replaced, not because it is being reproduced.

## 1. What this replaces

The C traps eight signals in `signal_setup` (`comm.c:2165`): `SIGUSR1`
rereads the wizlists for autowiz, `SIGUSR2` unrestricts the game in an
emergency, `SIGVTALRM` is a 180-second deadlock watchdog that `abort()`s,
`SIGCHLD` reaps autowiz children, `SIGHUP`/`SIGINT`/`SIGTERM` all mean
"die" (`hupsig`, `comm.c:2120`, which logs `SYSERR:` and `exit(1)` —
saving nobody), and `SIGPIPE`/`SIGALRM` are ignored. Staying up at all was
the `autorun` shell script's job, and the in-game `shutdown` variants
communicated with it by touching files (`.killscript`, `.fastboot`,
`pause` — `act.wizard.c:1082`, `autorun:143`).

Two of those had Go counterparts before this document existed, wired
independently: `signal.NotifyContext` for `SIGINT`/`SIGTERM`, and a
goroutine of its own for the `SIGHUP` that re-reads the game tuning. Both worked,
and neither said what should happen to the other six, what a reload was
allowed to touch, or what an operator sends a server that has stopped
responding. Those are the questions this settles.

## 2. Four principles

**A signal is a request, not the work.** The C's handlers set a byte
(`reread_wizlists`, `comm.c:2087`; `unrestrict_game`, `comm.c:2093`) and the
game loop acts on it after the heartbeat (`comm.c:877`), because a handler
may not call anything that is not async-signal-safe. Go has no such
constraint, and the same shape still holds here for a different reason: the
world is owned by one goroutine. Every signal handler either publishes an
atomic value (`game.SetTuning`) or hands a closure to `engine.DoSync`. None
of them touches the world directly, and none of them blocks — the signal
goroutine must always be ready for the next signal, in particular for the
second `SIGINT` that kills a wedged shutdown.

**Every signal has a non-signal equivalent.** The runtime image is
distroless with no shell (`docs/operations.md`), so `docker exec ... kill`
and `kubectl exec ... kill` do not work. `dlctl` ships in that image
already, and gets a `dlctl signal <name>` subcommand: same PID namespace,
signal PID 1. Anything reachable only by `kill(1)` is unreachable in the
deployment this server actually targets, which is what makes this a design
constraint rather than a convenience.

**A reload may fail without taking the game with it.** Parse and validate
the new file completely; swap only if that succeeded; on failure log and
keep what was already there. This is what `LoadGameTuning` +
`game.SetTuning` and `Text.Reload` already do (the latter deliberately, so a
file deleted since boot leaves the old text rather than blanking it), and it
generalises to every reloadable file. A typo in a config file must never be
able to stop a running game.

**Signals reload files; commands reload the world.** §4.

**Every signal below is a Unix signal, and one row of the table does not
survive the trip to Windows.** Go's `syscall` package defines `SIGHUP`,
`SIGINT`, `SIGTERM` and `SIGQUIT` there as numbers that exist and are
never delivered — harmless to name and to trap. It defines no `SIGUSR1`
or `SIGUSR2` at all, so anything that mentions them by name has to sit
behind a build constraint (`internal/signals/name_unix.go`) or the tree
stops compiling for `windows/amd64`, which is a platform a release ships
binaries for. That is not a hypothetical: it is what the release
cross-compile caught the first time it ran. Whatever §3's `SIGUSR1` and
`SIGUSR2` rows eventually do, they get implemented on the Unix side of
that split, and Windows gets whatever in-game or `dlctl` route exists
instead.

## 3. The table

| Signal | Disposition | The C | Why |
|---|---|---|---|
| `SIGTERM` | Graceful shutdown, exit 0 | `hupsig` → `exit(1)`, saving nothing | What `docker stop`, `systemctl stop` and a pod delete send. Exit 0 because a requested stop is not a failure, and `restart: on-failure` has to be able to tell them apart |
| `SIGINT` | Same as `SIGTERM`; a second one kills | `hupsig` | Already built (`main.go:498` stops relaying, so the second Ctrl-C hits the default disposition) |
| `SIGHUP` | Reload the re-readable files (§4) | `hupsig` → die | The conventional Unix meaning, and the one every process supervisor's `ExecReload=` assumes. Diverges from the C on purpose |
| `SIGUSR1` | Re-read `wizlist`/`immlist` | `reread_wizlists` (`comm.c:2087`), for autowiz | Kept as a narrow alias of the `SIGHUP` text reload, so an autowiz-shaped external tool has the signal it expects |
| `SIGUSR2` | Emergency unrestrict: clear the ban list, `wizlock 0`, reset the invalid-name counter | `unrestrict_game` (`comm.c:2093`, acted on at `:884`) | The lockout lever. Exactly the case where the in-game equivalent is unavailable, because being unable to log in is the problem. Logged loudly, and wizvis so anyone online sees it happen |
| `SIGQUIT` | **Not handled.** The Go runtime dumps every goroutine's stack and dies | `SIGVTALRM`/`checkpointing` (`comm.c:2109`) — one log line and `abort()` | This is the C's deadlock watchdog done properly: `kill -QUIT` on a wedged server names the goroutine sitting on the thing it is stuck on, instead of printing "infinite loop suspected". Deliberately loses the saves — a world goroutine that cannot turn cannot run them either |
| `SIGPIPE` | Nothing | `SIG_IGN` | The Go runtime already gives non-stdio descriptors `EPIPE` from the write instead of a signal. Same outcome, nothing to wire |
| `SIGCHLD` | Nothing | `reap` (`comm.c:2101`), for autowiz children | Nothing forks |
| `SIGALRM`, `SIGVTALRM` | Nothing | Ignored / the watchdog | The pulse is a Go ticker, not an interval timer. See §6 for what replaces the watchdog |

One dispatcher owns all of this: `internal/signals`, a single
`signal.Notify` over one channel, one goroutine, one handler per signal,
with `cmd/dlmud` supplying the handlers. Handlers run one at a time and must
return promptly — the signal that must not be delayed is the second
`SIGINT`, the one sent because the first shutdown is wedged — and stopping
the dispatcher restores the default dispositions, which is what makes that
second `SIGINT` fatal rather than swallowed.

## 4. What a reload may touch

Three classes, and the line between the first two is the whole point of
this section.

**A. Whole-value files — a signal may reload these.** Game tuning
(`<lib-dir>/config/game.yaml`), the canned text `LoadText` reads, the ban list, the
disallowed-name list. Each is read wholesale into a fresh value and swapped
behind an atomic pointer or a mutex; nothing in the running world holds a
pointer into them. There is no argument to supply and no way for the reload
to refuse, which is what makes them expressible as a signal at all.

`SIGHUP` does the lot, in one pass, each independently: a broken `bans` file
does not stop the tuning reload. One log line per subsystem, plus a summary.
*Built so far: the game tuning. The other three are §9 item 3, and until it
lands a `SIGHUP` at a data directory with no `config/game.yaml` in it warns
that there is nothing to re-read rather than reloading the rest.*

**B. World data — only commands may reload this.** Rooms, mobiles, objects,
zones, shops. Reloading a prototype is surgery on live instances, not a
value swap: `Server.ReloadMobile` can refuse with `session.ErrMobEngaged`,
`ReloadZone` reports what it moved. It needs a vnum to act on and somebody
to answer. **`SIGHUP` will never reload the world** — `reloadmob`,
`reloadzone`, `reloadobj` and `reloadshop` are the interface, they stay
in-game, and the "reload edited world data without a restart" capability
that Phase 6 became is theirs alone.

**C. Player data — never.** Live characters own their records; re-reading
one from disk would clobber whatever has happened since login.

## 5. Shutdown

The ordering is already built and already correct, and this document's job
is to name it as a contract so it is not quietly reordered later:

1. Cancel `ctx` — listeners close, every connected player is told "The
   server is shutting down. Come back soon!" and their socket is closed
   (`internal/server/listen.go:184`).
2. Stop relaying signals, so a second `SIGINT` kills a wedged shutdown.
3. Readiness off — `/readyz` starts answering 503.
4. `SaveEverything` **through the still-running world goroutine**: players,
   crash-saves, changed houses, the mud clock.
5. `WaitForWrites` — drain the background saves already in flight.
6. Only now stop the world goroutine. Its context is deliberately not
   derived from `ctx`; cancel it at step 1 and every save in step 4 blocks
   until the deadline and returns "context deadline exceeded", which is a
   silent shutdown rather than a slow one. `test/play`'s
   `TestShutdownSavesEveryoneStillInTheWorld` is what catches a regression
   here, and nothing short of a real binary and a real `SIGTERM` can see it.
7. Stop the diagnostics server.

**The budget is a setting, not a constant.** `shutdownTimeout` is 30
seconds and `docs/operations.md` asks operators to set a container grace
period above it. Something that has to agree with an external setting has no
business being compiled in, so it becomes `--shutdown-timeout`, defaulting
to the same 30s. *§9 item 5; it is still a constant in `main.go` until then.*

**Exit codes.** Three of them:

| Code | Meaning |
|---|---|
| 0 | Clean stop: `SIGTERM`, `SIGINT`, `shutdown`, `shutdown die`, `shutdown pause` |
| 1 | Boot failure, or a fatal error while running |
| 2 | Reboot requested: `shutdown reboot`, `shutdown now` |

`Server.RebootWanted` had said since it was written that "the answer is an
exit code instead" of the C's file-touching, and for a while there was no
exit code: both spellings exited 0, so the distinction the two commands are
named for did not survive the process. It does now, and
`test/play`'s `TestTheExitCodeSaysWhetherToComeBack` is what keeps it.

This is the replacement for the C's `.killscript`/`.fastboot`/`pause` files.
With `restart: on-failure`, `shutdown reboot` comes back by itself and
`shutdown die` stays down — the same distinction, expressed in the mechanism
the container runtime already has. Worth documenting the corollary in
`operations.md`: under `restart: always` both come back, so an operator who
wants `die` to mean die needs `on-failure` or `unless-stopped`.

## 6. The watchdog

The C aborted itself if the game loop stopped turning for 180 seconds of CPU
time (`setitimer(ITIMER_VIRTUAL, ...)`, `comm.c:2183`). The container-native
form of that is a liveness probe, and we do not currently have one that can
fail: `/healthz` answers 200 if the process answers at all, so a deadlocked
world goroutine looks healthy forever while every player sits frozen.

So `/healthz` asserts that the world goroutine turned recently — last pulse
within some multiple of `--pulse-interval`, generous enough that a long zone
reset or a slow disk is not a restart, tight enough that a real deadlock is
caught in under a minute. `/readyz` keeps its current meaning (booted,
listening, not shutting down). That, plus `SIGQUIT` for the post-mortem, is
the C's `abort()` with a stack trace attached and a supervisor that brings
it back. *§9 item 6; `/healthz` still answers 200 unconditionally until
then, which is the one place this document describes something the server
does not yet do.*

## 7. Containers

- **PID 1.** The image's entrypoint is `dlmud` itself, so it runs as PID 1,
  where the kernel gives no default dispositions — a signal is discarded
  unless a handler exists. Everything in §3 that matters is handled
  explicitly, and the Go runtime installs its own handlers for the fatal
  ones, which is why `SIGQUIT` still dumps stacks. No `tini` is needed:
  nothing forks, so there are no orphans to reap (which is also why the C's
  `SIGCHLD` reaper has no counterpart).
- **`STOPSIGNAL SIGTERM`** in the Dockerfile. It is already the default;
  declaring it makes the contract visible next to the entrypoint.
- **Docker:** `docker kill --signal=HUP <container>` for a reload;
  `stop_grace_period` comfortably above `--shutdown-timeout`.
- **Kubernetes:** a pod delete sends `SIGTERM`;
  `terminationGracePeriodSeconds` above the budget. No `preStop` hook — the
  only thing to drain is player sockets, which step 1 closes itself. For
  `SIGHUP`/`SIGUSR2`, `kubectl exec -- dlctl signal hup`.
- **systemd:** `TimeoutStopSec` above the budget, `ExecReload=/bin/kill -HUP $MAINPID`.

## 8. Testing

A signal has no other kind of test than a real process receiving a real
one, so `test/play` is where these live — the same reasoning that made the
play suite worth building at all. `internal/signals` covers the dispatcher
itself (delivery, `Stop` restoring the default, handlers not overlapping),
and every test there signals the test binary with `SIGWINCH`: putting the
default disposition back is the thing under test, and a test that then sent
`SIGHUP` would kill the test binary rather than fail it. `SIGURG` looks
equally safe and is not — the runtime preempts goroutines with it.

`test/play` covers the wiring: the exit codes over all four spellings of
`shutdown`, a `SIGHUP` that changes tuning a player can feel in the same
session, and a `SIGHUP` with a corrupt file that leaves the old values in
place, the player connected and exactly one ERROR logged. Still to add with
the features they belong to: a `SIGUSR2` that clears a `wizlock` nobody can
log in past, and a second `SIGINT` during shutdown.

## 9. Still to build

**Built 2026-08-24**, in the change this document landed with: the
dispatcher (`internal/signals`, one channel and one goroutine, with
`cmd/dlmud` supplying the handlers), the `SIGTERM`/`SIGINT` shutdown it
took over, the `SIGHUP` configuration reload, and the exit codes — with
`test/play`'s `TestTheExitCodeSaysWhetherToComeBack` and the three
`SIGHUP` tests beside it.

The rest of the design is scope rather than an open question. Ordered, each
landable on its own:

1. `SIGHUP` widened from the game tuning to all four class-A files (§4).
2. `SIGUSR1` (wizlists) and `SIGUSR2` (emergency unrestrict), §3.
3. `--shutdown-timeout`, plus its `docs/configuration.md` entry — which
   `release.yml` checks flag by flag.
4. `/healthz` asserting world liveness (§6).
5. `dlctl signal <name>`, the no-shell escape hatch §2 requires.
6. `STOPSIGNAL` in the Dockerfile.
7. The two play tests §8 names as still missing, with the features they
   belong to.

## 10. Explicitly not doing

- **Copyover / hot reboot.** Preserving player connections across a restart
  by passing file descriptors stays out of scope (plan §7), and no signal
  here is reserved for it.
- **A signal that reloads the world.** §4B.
- **An HTTP admin API for reload or shutdown.** Unauthenticated mutation on
  a debug port is worse than a signal, and an authenticated one is a
  credential to manage for something a signal and an in-game command
  already do between them.
- **`SIGCHLD` handling.** Nothing forks.
