# Signal handling

**Status: proposal, 2026-08-24.** Parts of this are built — the dispatcher
(`internal/signals`), `SIGTERM`/`SIGINT` shutdown, `SIGHUP` game-tuning
reload and the exit codes — and this document is what makes them one design
rather than several decisions taken at different times. §9 tracks the rest.

Under §0's "Fidelity, phase two" this is implementation, not gameplay and
not compatibility: nothing here changes an on-disk format or anything a
returning player would notice, so none of it needs a `docs/deviations.md`
entry on fidelity grounds. The C is cited throughout because it is the
thing being replaced, not because it is being reproduced.

## 1. Where this starts from

The C traps eight signals in `signal_setup` (`comm.c:2165`): `SIGUSR1`
rereads the wizlists for autowiz, `SIGUSR2` unrestricts the game in an
emergency, `SIGVTALRM` is a 180-second deadlock watchdog that `abort()`s,
`SIGCHLD` reaps autowiz children, `SIGHUP`/`SIGINT`/`SIGTERM` all mean
"die" (`hupsig`, `comm.c:2120`, which logs `SYSERR:` and `exit(1)` —
saving nobody), and `SIGPIPE`/`SIGALRM` are ignored. Staying up at all was
the `autorun` shell script's job, and the in-game `shutdown` variants
communicated with it by touching files (`.killscript`, `.fastboot`,
`pause` — `act.wizard.c:1082`, `autorun:143`).

The Go server today wires two of those: `signal.NotifyContext` for
`SIGINT`/`SIGTERM` (`cmd/dlmud/main.go:181`) and a separate goroutine for
`SIGHUP`, which re-reads `--config`'s game tuning (`main.go:197`). There is
no `SIGUSR1`, no `SIGUSR2`, no watchdog, and no exit code that distinguishes
`shutdown reboot` from `shutdown die` — despite `Server.RebootWanted`'s own
doc comment saying the answer is an exit code (`internal/server/
operator.go:101`).

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

One dispatcher owns all of this: a single `signal.Notify` over one channel,
one goroutine, a `map[os.Signal]func()` of handlers, replacing the two
independent wirings in `main.go` today. `SIGTERM`/`SIGINT` keep cancelling
the context they cancel now — the shutdown ordering in §5 is hard-won and
does not change.

## 4. What a reload may touch

Three classes, and the line between the first two is the whole point of
this section.

**A. Whole-value files — a signal may reload these.** Game tuning
(`--config`), the canned text `LoadText` reads, the ban list, the
disallowed-name list. Each is read wholesale into a fresh value and swapped
behind an atomic pointer or a mutex; nothing in the running world holds a
pointer into them. There is no argument to supply and no way for the reload
to refuse, which is what makes them expressible as a signal at all.

`SIGHUP` does the lot, in one pass, each independently: a broken `bans` file
does not stop the tuning reload. One log line per subsystem, plus a summary.
A `SIGHUP` with no `--config` set still reloads the other three rather than
warning and doing nothing, which is what it does today.

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

**The budget becomes a flag.** `shutdownTimeout` is a 30-second constant
(`main.go:88`) and `docs/operations.md` already asks operators to set a
container grace period above it. Something that has to agree with an
external setting should not be a constant: `--shutdown-timeout`, defaulting
to 30s. (`docs/configuration.md` gets an entry with it; `release.yml`
checks flag coverage flag by flag.)

**Exit codes.** Three of them:

| Code | Meaning |
|---|---|
| 0 | Clean stop: `SIGTERM`, `SIGINT`, `shutdown`, `shutdown die`, `shutdown pause` |
| 1 | Boot failure, or a fatal error while running |
| 2 | Reboot requested: `shutdown reboot`, `shutdown now` |

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

`/healthz` should assert that the world goroutine turned recently — last
pulse within some multiple of `--pulse-interval`, generous enough that a
long zone reset or a slow disk is not a restart, tight enough that a real
deadlock is caught in under a minute. `/readyz` keeps its current meaning
(booted, listening, not shutting down). That, plus `SIGQUIT` for the
post-mortem, is the C's `abort()` with a stack trace attached and a
supervisor that brings it back.

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

`test/play` is the only place in the tree where a real process receives a
real signal (`harness_test.go:417` already sends `SIGTERM`), and that is
where these belong — the same reasoning that made the play suite worth
building. Add: a `SIGHUP` that changes tuning a running player can observe;
a `SIGHUP` with a corrupt config file that leaves the old values in place
and the server up; a `SIGUSR2` that clears a `wizlock` nobody can log in
past; exit code 2 from `shutdown reboot` against 0 from `shutdown die`; and
a second `SIGINT` during shutdown.

## 9. Work items

Ordered, each landable on its own.

1. ~~One signal dispatcher, replacing the two wirings in `main.go`.~~
   **Built 2026-08-24**: `internal/signals`, one channel and one goroutine,
   with `cmd/dlmud` supplying the handlers.
2. ~~Exit codes 0/1/2, honouring `RebootWanted`; `operations.md` with it.~~
   **Built 2026-08-24**, with `test/play`'s
   `TestTheExitCodeSaysWhetherToComeBack` — the exit code is a contract with
   whatever restarts the server, and only a real process has one.
3. `SIGHUP` widened from tuning to all four class-A files (§4).
4. `SIGUSR1` (wizlists) and `SIGUSR2` (emergency unrestrict).
5. `--shutdown-timeout`, plus `configuration.md`.
6. `/healthz` asserting world liveness (§6).
7. `dlctl signal <name>`.
8. `STOPSIGNAL` in the Dockerfile, grace-period guidance in `operations.md`.
9. Play tests throughout (§8).

## 10. Explicitly not doing

- **Copyover / hot reboot.** Preserving player connections across a restart
  by passing file descriptors stays out of scope (plan §7), and no signal
  here is reserved for it.
- **A signal that reloads the world.** §4B.
- **An HTTP admin API for reload or shutdown.** Unauthenticated mutation on
  a debug port is worse than a signal, and an authenticated one is a
  credential to manage for something `dlctl signal` already does.
- **`SIGCHLD` handling.** Nothing forks.
