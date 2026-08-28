# A tutorial world, in both formats

A small, invented zone — nothing to do with the archive or with stock
CircleMUD's own Midgaard — built to exercise one feature of the game per
room and walk a fresh character through trying it. Sixteen rooms, one
corridor, no dead ends: north from the start to the end, and south is
always the way back.

Checked in twice, the same way `examples/stock/` is: `binary/` is the
classic CircleMUD file shapes, hand-written directly against
`internal/persist/world/classic`'s own reader rather than generated, and
`yaml/` is `binary/` converted with `dlctl import`. See
`docs/design/data-format.md`.

## The tour

| Room | Teaches |
|---|---|
| The Testing Grounds | Where you start. What the zone is and how it resets. |
| Hall of Movement | `north`/`south`/... and their short forms, `look`, `exits`. |
| The Door Room | `open`/`close` on an unlocked door. |
| The Locked Vault | `unlock ... with <key>`, then `open` — two different states. |
| The Armory | `get`, `wear`, `wield`, `inventory`, `equipment`, `remove`, `drop`. |
| The Cluttered Closet | `put`/`get ... from` a container; `examine` or `look in` to see what is inside one (plain `look <container>` does neither — a real gap between what the room used to say and what `do_look` actually does, found by testing rather than assumed, and then moved again once `look in` was ported; see below). |
| The Dining Hall | `get` then `eat`, `fill ... from`, `drink`. |
| The Sparring Ring | `kill`, a corpse, `get all corpse`, `recite`/`quaff`/`use` a scroll/potion/wand/staff. |
| The Resting Room | `rest`, `sleep`, `wake`, `stand`. |
| Outside the Guildhall | `practice`, against a guildmaster who teaches whatever the caller's own class knows. |
| The General Store | `list`, `buy`, `sell`, `value`, against a real shop with real stock. |
| The Bank of Testing | `balance`, `deposit`, `withdraw` — against an ATM, not a teller; see below. |
| The Travelers' Rest | `rent`, and why it says rent is free here (`free_rent`, same as the real game). |
| The Post Office | `mail <name>`, `check`, `receive`. |
| The Notice Board | `look board`/`read board`/`write board`/`remove board <n>`. |
| Graduation Hall | The end, and a note that socials, `say`/`tell`, and `group`/`follow` were never given a room because they do not need one. |

## Five things this was wrong about before being tested

All five are recorded here rather than silently fixed, because CLAUDE.md's
own rule — test rather than read — is exactly what caught them, and losing
the story would be losing the reason the rule is there. Two were found the
first time this zone was played by hand; two more only turned up when
`test/play` started typing every command in the table above on every run;
and the fifth was not wrong when it was written at all — the ground moved
under it, which is what makes it the most useful of the five.

**`look in <container>` did not list what is inside it.** The room text
used to say it did. Reading `internal/session/informative.go` first would
have caught this (`lookAtTarget`'s own comment said `look in` and `look at`
"both mean 'describe that'"), but it was the live playtest — `look in
chest` printing the chest's description instead of the ring inside it —
that actually found it, and `examine` (`do_examine`, `act.informative.c`)
was the command the room text was changed to recommend instead.

The port was the thing at fault, though, not the room: `do_look` in the C
is a *dispatcher*, and `look in` is its own branch into `look_in_obj`
(`act.informative.c:679-690`). #177 ported that branch, so `look in chest`
lists the ring now, exactly as the room text originally claimed. The room
text names both commands today — which is the fifth entry below, and the
reason this one is worth keeping rather than deleting as fixed.

**The banker is not a mobile.** `spec_assign.c`'s own table (transcribed in
`internal/game/specassign.go`) attaches `"bank"` to two *object* vnums —
`3034`/`3036`, "two bank tellers' counters" — not to any mobile. A `bank
teller` NPC standing in a room, however convincingly described, never had
the special procedure that makes `balance`/`deposit`/`withdraw` do anything;
every command returned the do-nothing default ("Sorry, but you cannot do
that here!") until this was rebuilt as `#3034`, an automatic teller
machine, wear flags `0` so it cannot be carried off, exactly the shape the
real archive's own ATM object already has
(`examples/stock/binary/world/obj/30.obj`, the real `#3034`).

**The post office was documenting a command that does not exist.** The
room text said `mail send Someone`; the command is `mail Someone`
(`postmaster_send_mail`, `mail.c`, ported in `internal/session/mail.go`),
and `one_argument` reads `send` as the addressee, so the tutorial's own
instruction answered "No one by that name is registered here!". **And
`eat bread` off the floor does not work either** -- `do_eat` wants the
food in your inventory, so the Dining Hall needed a `get` in front of it.
Both were found the third time round, by `test/play` typing every quoted
command in this table against a real server (`tourCommands` in
`test/play/tour_test.go` is that list, and is what stops it happening a
fourth time).

**And the fifth was right when it was written, and stopped being right.**
The room text used to end "plain `look` only describes a container, it does
not open it", and `test/play`'s `TestContainers` asserted exactly that: that
`look chest` printed the chest's long description. Both were correct at the
time and both were checked. #177 then compared this port's `do_look` against
the real C server with `scripts/session-parity.sh` and found two things it
had never had — `look in` as its own branch, and `show_obj_to_char`'s
`SHOW_OBJ_ACTION` default, which answers "You see nothing special.." with
two full stops and does not name the object (`act.informative.c:131`).
Printing the long description is what `look` at the *room* does, not what
looking at the object does.

`test/play` is release-only (CLAUDE.md, the `go.yml`/`release.yml` split),
so #177 merged green with the old expectation still in the file, and the
next `make play` — run for an entirely unrelated reason — is what found it.
That is the split working as designed rather than a hole in it, but it does
mean a play-suite expectation, or a line of room text, can go stale silently
for as long as nobody cuts a release. It was not even alone: the same run
turned up three `who` expectations left behind by #179, which had put the
count line and the class abbreviation right in the same way. It was filed
as issue #198 and written
up here rather than quietly edited, because "it was tested, it was right,
and the ground moved" is a different story from the four above, and a more
likely one to repeat.

All of these were found by connecting a real client and typing every
command this README claims works, not by reading the world files and
assuming they would. The `internal/game/specassign.go`/`internal/game/board.go` tables
are the reason the mail room, the post office and the notice board *did*
work first try: `1200`/`1201`/`3020` (receptionist/postmaster/guild) and
`3099` (the mortal board, matching `board.mort` in
`internal/game/board.go`'s own `Boards` table) are real rows in those
tables, chosen deliberately rather than picked at random — see "Vnums" below.

## Vnums

Two things had to be true simultaneously, and they pulled in different
directions:

- **The mortal start room is `game.MortalStartRoom`, hardcoded to `3001`**
  (`internal/game/world.go`) — not a config value, not something a world
  file can override. A fresh character who logs into a world with no room
  `3001` ends up "nowhere at all", which is not a tutorial. Room `100`
  became room `3001`, and the rest of the corridor followed it up to
  `3016`.
- **Specials are attached by a fixed table keyed by vnum**
  (`internal/game/specassign.go`'s `MobileSpecials`/`ObjectSpecials`,
  `internal/game/board.go`'s `Boards`), matching the real
  `spec_assign.c` — not by anything a world file itself can declare. A
  brand new mobile invented for this zone would never become a
  guildmaster, a banker, a receptionist or a postmaster; only a real row
  of that table does. So the mobiles and the ATM that need a special
  keep the real table's own vnums: `1200` (receptionist), `1201`
  (postmaster), `3020` (guild), `3034` (bank), `3099` (board).

Nothing about reusing those vnums here collides with the real archive or
with `examples/stock/` — this is its own directory, loaded on its own, and
`--world-dir` only ever points at one world at a time.

The one thing this forced that is easy to get wrong: **a yaml zone file
only carries the mobiles, objects and shops whose vnum falls inside that
zone's own `bot`–`top` range** (`internal/persist/world/yaml/writer.go`).
The rooms here needed `3001`–`3016`; the specials needed `1200`–`3099`;
this zone's own ordinary mobiles and objects (the training dummy, the
shopkeeper, the sword, the chest, ...) sit at `130`–`180` and `140`–`158`,
chosen before the vnum-reuse requirement above was fully worked out. The
zone header is `100 3099`, wide enough to cover all three ranges at once
— found the hard way, by converting to `yaml` and discovering only 1 of
5 mobiles and 2 of 19 objects had survived the trip, because the original
header was the tighter `3001 3099` and everything below `3001` had
silently fallen outside it.

## Reproducing `yaml/` from `binary/`

```sh
dlctl import --from-dir=examples/mini/binary --to-dir=examples/mini/yaml
```

`classic → yaml` world dumps are identical — `dlctl dump --type=world` on each
and `diff` the two — the same check `examples/stock/README.md` runs, and
for the same reason.

## Running it

```sh
dlmud --lib-dir=examples/mini/binary --listen-telnet=:4000

dlmud --lib-dir=examples/mini/yaml --listen-telnet=:4000 \
  --world-format=yaml --state-format=yaml --names-format=yaml \
  --messages-format=yaml --socials-format=yaml --help-format=yaml
```

`--player-format` is left at its default (`ascii`): there is no roster
here either, and the first character created is promoted to Implementor
the same way `examples/stock/`'s is (`db.c`'s "if this is our first
player --- he be God").

`config/game.yaml` is the shipped, fully-commented example of the game
tuning (`docs/configuration.md`) — the same file `examples/stock/` ships,
in both formats, because the tuning lives in the data directory it
configures. Every value in it is commented out at its `config.c` default,
so it changes nothing until something is uncommented: copy it into your own
data directory, edit it, and `SIGHUP` the server. The Travelers' Rest's own
room description above is about `free_rent`, which is the first line in
it.

`misc/socials` and `misc/messages` are the real archive's own files,
copied across unchanged — socials and combat messages are not zone
content, so there was nothing to invent, and a tutorial that could not
`smile` or see a real hit message would be a worse one. `text/` and
`text/help/` are the real stock screens and help database, for the same
reason: `background` tells this zone's own story, but `credits`,
`greetings` and the rest exist to satisfy the licence and did not need
reinventing.
