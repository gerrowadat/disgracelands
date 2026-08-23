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

Every subsystem `dlctl` knows how to convert, run against `binary/`:

```sh
go run ./cmd/dlctl world import    --from-dir=examples/stock/binary/world      --to-dir=examples/stock/yaml/world
go run ./cmd/dlctl state import    --from-dir=examples/stock/binary/etc \
                                    --from-house-dir=examples/stock/binary/house \
                                    --from-misc-dir=examples/stock/binary/misc  --to-dir=examples/stock/yaml/state
go run ./cmd/dlctl names import    --from-path=examples/stock/binary/misc/xnames    --to-dir=examples/stock/yaml/config
go run ./cmd/dlctl messages import --from-path=examples/stock/binary/misc/messages  --to-dir=examples/stock/yaml/config
go run ./cmd/dlctl socials import  --from-path=examples/stock/binary/misc/socials   --to-dir=examples/stock/yaml/config
go run ./cmd/dlctl helpdb import   --from-dir=examples/stock/binary/text/help       --to-dir=examples/stock/yaml/text/help
```

`text/`'s eleven plain-text files (`motd`, `credits`, `greetings`, ...) are
not a pluggable format — both classic and yaml read them from the same
`text/<name>` path regardless of `--*-format` (`internal/server/text.go`)
— so they are copied across unchanged rather than converted. `binary/`'s
own roster, mail, boards, bans and houses are all empty (a fresh stock
install has none), so `state import` produced only `clock.yaml` and an
empty `houses.yaml`; there is no `pfiles/` to convert because there is no
roster.

Re-run the commands above after editing anything in `binary/` to keep
`yaml/` in sync — nothing checks the two stay matched automatically, the
same way nothing checks `data/`'s own single copy against anything.

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
