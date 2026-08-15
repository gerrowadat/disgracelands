# Reference source trees

Code-only snapshots of the two other Disgracelands-lineage codebases,
kept here so they're available for comparison/porting work (see `TODO.md`
item 2) without needing the full multi-hundred-megabyte archive dump this
repo was seeded from.

- **`CircleMUD3-src/`** — the tree `Reborn` itself is based on (CircleMUD
  3.0 bpl20 + OasisOLC + DG Scripts + local `<DoC>`-tagged mods). Kept
  here too, unmodified, as a clean diff baseline against `../src` as
  `Reborn` evolves away from it.
- **`WipeMud-src/`** — the abandoned May 2003 upgrade to CircleMUD 3.1.
  Not the baseline `Reborn` is built from (see
  `../docs/circlemud-archive-report.md` §7 for why), but it has real local
  modifications of its own — notably a race system (`races.c`/`races.h`)
  that never existed in `CircleMUD3` — worth mining for anything worth
  porting forward.

## What's here vs. what isn't

Only source: `src/`, `cnf/`, `configure`, top-level `README`/`FAQ`/
`ChangeLog`, and the plain-text files under `doc/` (binary `.pdf`/`.ps.gz`
docs from `WipeMud`'s `doc/` were dropped — same content, just not in a
portable format worth carrying around). No `lib/` (world or player data),
no compiled binaries, no logs, no autoconf-generated `config.*` files —
those regenerate from `configure` per `../BUILDING.md`.

Neither of these trees has been test-built the way `../src` has. They're
here as reference material, not as something expected to compile
out of the box on a modern system without the same treatment `../src`
already got (see `../BUILDING.md`).
