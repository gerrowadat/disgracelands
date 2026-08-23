# One world, two formats

The same stock CircleMUD 3.0 bpl20 world — 30 zones, 1878 rooms, 569
mobiles, 679 objects, 46 shops — checked in twice, so there is a worked
example of both formats this server supports rather than just a spec for
them.

- **`binary/`** is CircleMUD's own `lib/` directory, exactly as it ships:
  the "classic" format `internal/persist/world/classic` and its siblings
  read. The name is CircleMUD's own historical shorthand for "the original
  distribution format" (`bin2ascii`, `ascii2bin`, ...) rather than a literal
  binary encoding — every file in here is plain ASCII, same as it always
  was. It is a copy of this repo's own `data/` directory, which is in turn
  fetched unmodified from
  `https://www.circlemud.org/pub/CircleMUD/3.x/Old-Betas/circle30bpl20.tar.gz`
  (sha256 `1cd2cf0268c27dd6e6ae4d996a620bbd56da2552beb434bf372b4c01cd8bb415`)
  — see PR #29 for why `data/` is stock rather than the archive's own world.
- **`yaml/`** is the same world, converted through this project's own
  yaml format (`docs/design/data-format.md`), one file per zone
  plus `config/`, `state/` and `text/help/help.yaml`.

## How `yaml/` was produced

One command, converting every subsystem `dlctl` knows how to against
`binary/` in one go:

```sh
go run ./cmd/dlctl lib import --from-dir=examples/stock/binary --to-dir=examples/stock/yaml
```

`lib import` is `world import`/`pfile import`/`state import`/`names
import`/`messages import`/`socials import`/`helpdb import`, run in that
order against the matching subdirectories of `--from-dir`, plus two things
none of those seven do on their own: `text/`'s eleven plain-text files
(`motd`, `credits`, `greetings`, ...) — not a pluggable format, since both
classic and yaml read them from the same `text/<name>` path regardless of
`--*-format` (`internal/server/text.go`) — are copied across unchanged
rather than converted; and, once every step above has actually succeeded,
`--to-dir` is stamped with a `.dlversion` file
(`docs/design/data-format-versioning.md`) recording which release of the
yaml packages wrote it — `examples/stock/yaml/.dlversion` is what that
stamp looks like. `binary/`'s own roster, mail, boards, bans and houses
are all empty (a fresh stock install has none), so the state step here
produced only `clock.yaml` and an empty `houses.yaml`; there is no
`pfiles/` to convert because there is no roster.

Re-run the command above after editing anything in `binary/` to keep
`yaml/` in sync — nothing checks the two stay matched automatically, the
same way nothing checks `data/`'s own single copy against anything. The
seven subsystem commands `lib import` wraps are still there individually,
for converting just one of them, or into a directory laid out differently
than `--to-dir`'s own subdirectory-per-subsystem default.

## Verifying the two are the same world

```sh
go run ./cmd/dlctl world lint --world-dir=examples/stock/binary/world --world-format=classic
go run ./cmd/dlctl world lint --world-dir=examples/stock/yaml/world   --world-format=yaml
go run ./cmd/dlctl world dump --world-dir=examples/stock/binary/world --world-format=classic --out=/tmp/binary.json
go run ./cmd/dlctl world dump --world-dir=examples/stock/yaml/world   --world-format=yaml   --out=/tmp/yaml.json
diff /tmp/binary.json /tmp/yaml.json
```

Both lint clean at 0 errors, and the dumps are identical except for one
already-documented, lossy library limitation: a description with a blank
line before its closing `~` (a sign or note with a trailing blank line —
45 lines out of the world's ~130,000-line dump here) loses that one blank
line on the way through `goccy/go-yaml`'s literal-block re-print, which
cannot be made to honour a `|+` "keep" chomping indicator. See
`docs/design/data-format.md` §12, "A second, genuinely lossy transform"
for the mechanism — this is the same finding at a larger sample size (the
real corpus that section cites has 3 such strings out of 12,372; stock's
own sign/note-heavy zones have more).

## Running the server against either

```sh
go run ./cmd/dlmud --lib-dir=examples/stock/binary --listen-telnet=:4000
go run ./cmd/dlmud --lib-dir=examples/stock/yaml --listen-telnet=:4000 \
  --world-format=yaml --state-format=yaml \
  --names-format=yaml --messages-format=yaml \
  --socials-format=yaml --help-format=yaml
```

`--player-format` is left at its default (`ascii`) in both: there is no
roster to speak of either way, and the first character created promotes
itself to Implementor the same way it always has (`db.c`'s "if this is our
first player --- he be God", `TODO.md`).
