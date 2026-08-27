# Things to do next

The project is a Go port of the Disgracelands server. What to do next is
mostly "the next phase", and that lives in
`docs/proposals/go-port-plan.md` §10 — Phases 0–4 are done and **every slice of
Phase 5 is built**. Phase 6 (OasisOLC) was decided against, in favour of
`reloadmob`/`reloadzone`/`reloadobj`/`reloadshop`; Phase 7 (cutover) has not
started. Its §10 also lists, command by command, the 8 of the C's 318
commands that nothing answers to yet — seven OasisOLC editors and
`slowns`, both declined rather than pending; see `docs/deviations.md`.

This file is for the things that are not phases: work on the C server in
`reference/moderncserver/`, and decisions that are still open.

## Still open

### 1. Bringing the real player roster back (local only)

The genuine 2001–2008 roster (108 characters) lives in the archive, not in
this repo: `../welmar/CircleMUD3/lib/etc/players`, in the original binary
format. To convert it locally:

```sh
go run ./cmd/dlctl convert --type=pfile \
  --from-format=binary --from-dir=../welmar/CircleMUD3/lib \
  --to-format=ascii    --to-dir=examples/stock/binary
```

That is Phase 2's replacement for the old `-m32` `bin2ascii` route, and it
needs no 32-bit toolchain; the C tool is still in `reference/tools/` and
`docs/investigations/pfile-conversion.md` explains why it needed one.
`dlctl convert` does the whole data directory, roster included, if that is
what you have.

The output (`examples/stock/binary/pfiles/`) is gitignored and stays local. No player data
has ever been committed here, deliberately: it is real people's password
hashes, private in-game mail and connection hosts.

A checkout with no `etc/players` in its lib-dir is the *normal* fresh-install state,
not a broken one. `db.c`'s "if this is our first player --- he be God"
(~line 2705) promotes whoever registers first to Implementor, which is how
you bootstrap.

### 2. Decide what, if anything, to take from WipeMud

`reference/WipeMud-src/` is the abandoned CircleMUD 3.1 upgrade attempt. It
has at least one feature the played tree never got: a race system
(`races.c`/`races.h`, referenced via the `Race:` field that `bin2ascii`
stubs to 0). Worth a look before deciding against it — diff
`reference/WipeMud-src/src` against `reference/moderncserver/src/`, since
the `<DoC>`-tag comparison in
`docs/investigations/circlemud-archive-report.md` §7 only covered files both
trees share, so anything WipeMud-only would not show up in it.

### 3. World data — pick a snapshot, or trust what is live

This is about the *archived* world, not the one in this repo: what ships
here is stock CircleMUD's own `lib/` (`examples/stock/binary/`, see
`README.md`), and no Disgracelands world data has ever been committed. The
archived world the port was seeded against is `welmar/CircleMUD3/lib/world`,
whatever state it happened to be in when the server stopped. The archive
also has 1,184 dated nightly world backups (`welmar/world-backups/`,
September–December 2002 survives) and 164 per-zone tarballs
(`welmar/zones/`) if a different point-in-time snapshot is wanted.

Not investigated beyond "it loads and boots" — though it now also loads
identically in both servers, and `dlctl lint --type=world` reports what is
wrong with it (0 errors, 20 warnings, 8 notes; see
`docs/investigations/lib-directory-format.md` §9), which is more than was
known before. The shipped stock world lints at 0 errors, 11 warnings, 12
notes — a different world, and a different set of findings
(`docs/operations.md`).

### 4. Hosting and exposure

Nothing here has been hardened for 2026. The Go port now takes logins, so
there is something to expose and a reason not to yet:
`docs/operations.md` has the current posture and
`docs/proposals/go-port-plan.md` §7 covers what the network layer will do
about it.

## C server only (`reference/moderncserver/`)

Nothing is running the game — not that tree, not the Go port, and nothing
has since 2008. Phase 7 (`docs/proposals/go-port-plan.md` §10) is about
what would have to be true before anything did again, not about swapping a
live service. So the C tree has no operational stake at all: its two jobs
are being the reference implementation and being the parity oracle. A
problem in it is worth writing down if it would mislead the port, not
because anyone could reach it. See its `README.md`.

### 5. `src/util/*` assume the binary player format

`autowiz`, `mudpasswd`, `listrent` and friends build fine (`make utils`) but
read `struct char_file_u` directly. Anything that changes the player format
in that tree has to change them too.

## Superseded

Kept here so it is clear these were decided rather than forgotten.

- **Wiring ascii pfiles into the C server's live login/save path.** This was
  the biggest remaining item and the Go port took it over: Phase 2 built
  both formats behind one interface, and the Go server runs on ascii and
  refuses to start on binary, converting with `dlctl convert --type=pfile` instead.
  Doing it in C as well would have been the same security-adjacent work
  twice.
- **Deciding how to run it across restarts.** `autorun` and friends are
  superseded by the container runtime's restart policy plus real SIGTERM
  handling; see `docs/operations.md`.
- **Auditing the C tree's suppressed `sprintf`-overlap build warnings.**
  `-w` hides genuine overlap and truncation patterns in `db.c`,
  `improved-edit.c`, `tedit.c`, `zedit.c`, `listrent.c` and `shopconv.c`.
  Decided against, and issue #143 closed with it. The item's own premise
  was that it "matters only for as long as that tree is the one actually
  running the game", and no tree is running the game. They stay listed
  under "Known problems" in `reference/moderncserver/README.md`, because
  anyone who builds and runs that tree locally should know they are there
  — as history, not as a fix to make. The Go port fixes this class of bug
  where it meets it and records each one (`docs/deviations.md`).
- **`dlctl import`'s five smaller importers not transcoding.** Fixed,
  not decided against: `--type=state`/`names`/`messages`/`socials`/`help`
  all take `--encoding` now and decode the same way `--type=world`/
  `pfile` already did, `import` with no `--type` passing its own flag
  through to all seven. See `docs/design/data-format.md` §11.1 for what
  each importer's free-text fields turned out to be.
