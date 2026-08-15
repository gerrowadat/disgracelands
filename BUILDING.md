# Building Reborn (Disgracelands) on a modern Linux box

This is CircleMUD 3.0 bpl20-era C code from ~2002 (patched with OasisOLC,
DG Scripts, and Disgracelands' own local mods — see the `<DoC>` tags
throughout `src/`). It predates C99 and was written against a much more
permissive compiler than anything from the last decade. GCC 14+ (this repo
was built and tested with GCC 15.2.0) turns several things this code relies
on into hard errors by default, so both `./configure` and `make` need
non-default flags.

## One-time setup

If cross-compiling 32-bit helper tools isn't needed, no extra packages are
required beyond a normal C toolchain (`gcc`, `make`).

## Configure and build

```sh
export CFLAGS="-std=gnu89 -fcommon -Wno-implicit-function-declaration -w"
export CC=gcc
./configure
cd src && make
```

`CFLAGS` has to be exported **before** `./configure` runs too, not just
before `make` — `configure`'s own "does the C compiler work" check uses a
K&R-style `main(){return(0);}` test program with no explicit return type,
which GCC treats as a hard error (`-Wimplicit-int` is an error by default
now) unless `-std=gnu89` is in effect.

Why each flag is needed:

- `-std=gnu89` — restores pre-C99 behaviour: implicit `int` return types,
  implicit function declarations as warnings instead of errors, and
  tentative definitions across translation units (`-fcommon` handles the
  common-symbol part of that; GNU89/GNU99 default to `-fcommon` on old GCC
  but modern GCC defaults to `-fno-common`, which breaks multiple `.c`
  files declaring the same global without `extern`).
- `-fcommon` — see above.
- `-Wno-implicit-function-declaration` — belt and braces; without
  `-std=gnu89` this alone won't save you (modern GCC hard-errors on it
  regardless of standard past a certain version), but combined with
  `-std=gnu89` it suppresses the warning noise too.
- `-w` — this code is *loud* under modern `-Wall` (`sprintf` overlap /
  truncation warnings, signed/unsigned comparisons, etc.) — all
  pre-existing and apparently harmless in practice, but not worth fixing
  wholesale right now. Drop `-w` if you want to see them.

## Known source fix already applied

`src/interpreter.c` included `constants.h` before `structs.h`. Every other
`.c` file in the tree gets this the other way around; `constants.h`
declares `extern const struct str_app_type str_app[];` and similar, which
need `structs.h`'s struct definitions in scope first. This was silently
tolerated by whatever compiler this last built cleanly on and is now a hard
error. Fixed by reordering the two `#include` lines.

## Running

```sh
cd .. # back to Reborn/
bin/circle -q 4000
```

`bin/` also gets `src/util/*` (`autowiz`, `mudpasswd`, `listrent`, etc.)
built via `make utils` (part of the default `make all` target already).
None of these are committed to git — they're build products, rebuild them
locally.

## Not yet done

- The `sprintf`-into-shared-`buf` warnings throughout (`db.c`,
  `improved-edit.c`, `tedit.c`, `zedit.c`, `listrent.c`, `shopconv.c`, ...)
  are worth auditing properly before this runs unattended on the open
  internet again — several look like genuine (if old and apparently never
  triggered) buffer-overflow-shaped bugs, not just style complaints.
- No 64-bit-vs-32-bit audit of anything that touches saved binary data
  (the player database — see `docs/pfile-conversion.md`) has been done
  beyond the player-file struct itself.
