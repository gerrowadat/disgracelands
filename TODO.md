# Things to do next

The project is a Go port of the Disgracelands server. What to do next is
mostly "the next phase", and that lives in
`docs/proposals/go-port-plan.md` §10 — currently **Phase 2, player
loading**.

This file is for the things that are not phases: work on the C server in
`reference/moderncserver/`, and decisions that are still open.

## Still open

### 1. Bringing the real player roster back (local only)

The genuine 2001–2008 roster (108 characters) lives in the archive, not in
this repo: `../welmar/CircleMUD3/lib/etc/players`, in the original binary
format. To convert it locally:

```sh
cd reference/moderncserver
gcc -m32 -std=gnu89 -fcommon -w -Isrc -o bin/bin2ascii ../tools/bin2ascii.c
bin/bin2ascii ../../../welmar/CircleMUD3/lib/etc/players ../../data/pfiles
```

The `-m32` is not optional — see `docs/investigations/pfile-conversion.md`
for why, and note that Phase 2 replaces this with `dlctl pfile convert`,
which needs no 32-bit toolchain.

The output (`data/pfiles/`) is gitignored and stays local. No player data
has ever been committed here, deliberately: it is real people's password
hashes, private in-game mail and connection hosts.

A checkout with no `data/etc/players` is the *normal* fresh-install state,
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
with it (0 errors, 20 warnings), which is more than was known before.

### 4. Hosting and exposure

Nothing here has been hardened for 2026, and the Go port has no game in it
yet, so there is nothing to expose. When there is:
`docs/operations.md` has the current posture and
`docs/proposals/go-port-plan.md` §7 covers what the network layer will do
about it.

## C server only (`reference/moderncserver/`)

These matter only for as long as that tree is the one actually running the
game. See its `README.md`.

### 5. Audit the build warnings that look like real bugs

It builds with `-w` to suppress 2002-era warning noise. Several of the
suppressed warnings are `sprintf`-into-shared-`buf` overlap and truncation
patterns in `db.c`, `improved-edit.c`, `tedit.c`, `zedit.c`, `listrent.c`
and `shopconv.c`. Old code, apparently never triggered in practice — but
"apparently never triggered" is not "safe".

### 6. `src/util/*` assume the binary player format

`autowiz`, `mudpasswd`, `listrent` and friends build fine (`make utils`) but
read `struct char_file_u` directly. Anything that changes the player format
in that tree has to change them too.

## Superseded

Kept here so it is clear these were decided rather than forgotten.

- **Wiring ascii pfiles into the C server's live login/save path.** This was
  the biggest remaining item and the Go port takes it over: Phase 2 builds
  both formats behind one interface, with the binary format as the default
  so an existing `data/` needs no migration. Doing it in C as well would be
  the same security-adjacent work twice.
- **Deciding how to run it across restarts.** `autorun` and friends are
  superseded by the container runtime's restart policy plus real SIGTERM
  handling; see `docs/operations.md`.
