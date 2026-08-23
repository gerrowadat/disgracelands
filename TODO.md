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
go run ./cmd/dlctl pfile convert \
  --from=binary --from-dir=../welmar/CircleMUD3/lib/etc \
  --to=ascii    --to-dir=examples/stock/binary/pfiles
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

`data/world/` is whatever was in `welmar/CircleMUD3/lib/world` when this was
seeded. The archive has 1,184 dated nightly world backups
(`welmar/world-backups/`, September–December 2002 survives) and 164 per-zone
tarballs (`welmar/zones/`) if a different point-in-time snapshot is wanted.

Not investigated beyond "it loads and boots" — though it now also loads
identically in both servers, and `dlctl world lint` reports what is wrong
with it (0 errors, 11 warnings), which is more than was known before.

### 4. Hosting and exposure

Nothing here has been hardened for 2026. The Go port now takes logins, so
there is something to expose and a reason not to yet:
`docs/operations.md` has the current posture and
`docs/proposals/go-port-plan.md` §7 covers what the network layer will do
about it.

### 5. `dlctl lib import`'s five smaller importers don't transcode

Found while writing `docs/operations.md`'s getting-started walkthrough for
converting a real archive, and checked with a synthetic CP1252 fixture
rather than assumed: `dlctl lib import` wraps seven importers, and only
two of them — `world import` and `pfile import` — have their own
`--encoding` flag and transcode non-UTF-8 text on the way in. `state
import`/`names import`/`messages import`/`socials import`/`helpdb import`
read whatever bytes are in the source file and write them straight into
the `yaml` output. Pointed at a genuinely CP1252 source (a curly quote in
a social, an accented name), the result is a `.yaml` file that is not
valid UTF-8 despite saying it is. `examples/stock/` never surfaces this
— stock CircleMUD's own text is pure ASCII — but a real, twenty-year-old
archive usually is not. See `docs/design/data-format.md` §11.1 for the
full write-up; the fix is giving those five importers the same
`--encoding` flag and decode step `world`/`pfile import` already have.

## C server only (`reference/moderncserver/`)

These matter only for as long as that tree is the one actually running the
game. See its `README.md`.

### 6. Audit the build warnings that look like real bugs

It builds with `-w` to suppress 2002-era warning noise. Several of the
suppressed warnings are `sprintf`-into-shared-`buf` overlap and truncation
patterns in `db.c`, `improved-edit.c`, `tedit.c`, `zedit.c`, `listrent.c`
and `shopconv.c`. Old code, apparently never triggered in practice — but
"apparently never triggered" is not "safe".

### 7. `src/util/*` assume the binary player format

`autowiz`, `mudpasswd`, `listrent` and friends build fine (`make utils`) but
read `struct char_file_u` directly. Anything that changes the player format
in that tree has to change them too.

## Superseded

Kept here so it is clear these were decided rather than forgotten.

- **Wiring ascii pfiles into the C server's live login/save path.** This was
  the biggest remaining item and the Go port took it over: Phase 2 built
  both formats behind one interface, and the Go server runs on ascii and
  refuses to start on binary, converting with `dlctl pfile convert` instead.
  Doing it in C as well would have been the same security-adjacent work
  twice.
- **Deciding how to run it across restarts.** `autorun` and friends are
  superseded by the container runtime's restart policy plus real SIGTERM
  handling; see `docs/operations.md`.
