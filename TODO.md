# Things to do next

Current state: compiles clean on modern Linux, boots, serves the real
Disgracelands banner, world data loads. No player data ships with the
repo on purpose (see `.gitignore` and `docs/pfile-conversion.md`) — a
fresh checkout starts blank, and per `src/db.c`'s
"if this is our first player --- he be God" (~line 2705), whoever
registers first on a blank `lib/etc/players` is auto-promoted to
Implementor. That's the intended way to bootstrap a fresh install, not a
bug.

## Bringing real player data back in (optional, local-only)

The genuine 2001–2008 roster (108 characters) lives in the archive, not
this repo: `../welmar/CircleMUD3/lib/etc/players` (binary). To use it
locally:

```sh
gcc -m32 -std=gnu89 -fcommon -w -Isrc -o bin/bin2ascii src/util/bin2ascii.c
bin/bin2ascii ../welmar/CircleMUD3/lib/etc/players lib/pfiles
```

This produces `lib/pfiles/` (gitignored, stays local). It's not wired
into the live game yet — see the next item.

## 1. Wire ascii pfiles into the live login/save path

Right now `src/db.c` still reads/writes the original binary
`struct char_file_u` flat-file format (`lib/etc/players`) at runtime.
`bin2ascii`/`pfiledump` prove the ascii format is readable, but nothing in
the actual game uses it yet. This is the biggest real remaining piece of
work. Reference implementation, scope, and known gaps (the `Aff`/`Pref`
flag encoding in particular) are written up in `docs/pfile-conversion.md`.
Touches: `db.c` (low risk — zero local `<DoC>` mods), plus `comm.c`,
`interpreter.c`, `act.wizard.c`, `house.c`, `mail.c`, and the
`src/util/*` tools that currently assume the binary format directly.

Do this **before** letting real people create real characters again —
retrofitting a save-format change under a live population is a much worse
time to do it.

## 2. Decide what (if anything) to pull in from WipeMud

`reference/WipeMud-src/` is the abandoned CircleMUD 3.1 upgrade attempt.
It has at least one feature `CircleMUD3` never got: a race system
(`races.c`/`races.h`, referenced via `Race:` fields already stubbed to 0
in `bin2ascii`'s output). Worth a look before deciding it's not wanted —
diff `reference/WipeMud-src/src` against `src/` to see what else is there
that isn't in the `<DoC>`-tag comparison already done in
`docs/circlemud-archive-report.md` §7 (that comparison only covered files
both trees share; anything WipeMud-only, like `races.c`, wouldn't show up
in it).

## 3. Audit the build warnings that look like real bugs

`BUILDING.md` builds with `-w` to keep 2002-era warning noise down.
Several of the suppressed warnings are `sprintf`-into-shared-`buf`
overlap/truncation patterns in `db.c`, `improved-edit.c`, `tedit.c`,
`zedit.c`, `listrent.c`, `shopconv.c` — worth going through properly
before running this on the open internet again. Old code, apparently
never triggered in practice, but "apparently never triggered" isn't the
same as "safe."

## 4. World data — pick a snapshot, or trust what's live

`lib/world/` here is whatever was in `welmar/CircleMUD3/lib/world` when
this was seeded. The archive has 1,184 dated nightly world backups
(`welmar/world-backups/`, Sept–Dec 2002 survives) and 164 per-zone
tarballs (`welmar/zones/`) if a different point-in-time snapshot is
wanted instead. Not investigated further than "it loads and boots" so
far.

## 5. Decide on hosting / exposure

Nothing here has been network-hardened for 2026. Old `crypt()` usage,
old telnet-only protocol, no TLS, and item 3 above are all relevant before
this goes anywhere reachable by strangers. Local-only / LAN-only /
VPN-only for a while is the sane default.

## 6. `src/util/*` tools need rebuilding per-target

`autowiz`, `mudpasswd`, `listrent`, etc. build fine (`make utils`, part of
the default `make all`) but aren't committed (see `.gitignore` — they're
build products). If any of them get modified to understand the ascii pfile
format (item 1), make sure they still build cleanly afterward — several
assume the binary `struct char_file_u` layout directly.
