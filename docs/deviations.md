# Deviations from the C server

The fidelity decision in [the port plan](proposals/go-port-plan.md) §0 says the
patched C server wins wherever it and modern design disagree. This file is
where the exceptions are written down.

Every entry names what the C does, with a line reference, what the Go server
does instead, and why. That last column is the point of the file: under a
"fix known bugs" fidelity rule, the only thing keeping *fixed a bug*
distinguishable from *accidentally changed the game* is a written record made
at the time.

Line references are into `reference/moderncserver/src/`, the patched tree as
it ran in 2008.

**Not in here:** anything where the Go server reproduces the C exactly,
including its bugs — trailing carriage returns in world files, `parse_mobile`
force-setting `MOB_ISNPC`, the uninitialised bytes `save_char` writes. Those
are fidelity, not deviation, and they live in the tests that assert them.

---

## Player-visible behaviour

### Paladin cannot be chosen at character creation

| | |
|---|---|
| **C** | `class_menu` (class.c:93) offers four classes and does not list Paladin. `parse_class` (class.c:117) accepts `'p'` anyway, and creation calls that same function — so typing the unadvertised letter made a Paladin. |
| **Go** | `ParseCreationClass` rejects `'p'`. `ParseClass` still accepts it, for remorting and an implementor's `set class`. |
| **Why** | The C contradicts itself: the menu says one thing and the parser does another, so "be faithful to the C" does not settle it. Paladin exists to reward remorting, and reproducing the accident would let a new player skip the mechanic the class is a prize for. The parser is shared with `set class`, which is presumably why the case is there at all. |
| **Where** | `internal/game/class.go`, asserted in `TestPaladinIsNotSelectableAtCreation`. |

### Passwords: minimum six characters, no maximum

| | |
|---|---|
| **C** | `nanny` (interpreter.c:1526) refuses a password shorter than 3 characters or longer than `MAX_PWD_LENGTH` (10). Traditional DES `crypt(3)` then truncates it to 8 characters regardless. |
| **Go** | Minimum 6, no maximum. |
| **Why** | Both C limits were consequences of the storage, not policy. The ten-character ceiling is the width of the field in `char_file_u`; the three-character floor was a defensible minimum when only the first eight characters were hashed anyway. Under argon2id the whole password is used and the stored form is not fixed-width, so neither limit has a reason to exist. Applies only to passwords being *set* — no existing character is locked out. |
| **Where** | `auth.BadPassword` in `internal/auth/policy.go`, called from `badNewPassword` (`internal/session/menu.go`) and from `dlctl pfile passwd`. |

### Passwords are argon2id; legacy DES hashes are accepted and upgraded

| | |
|---|---|
| **C** | Traditional DES `crypt(3)`, salted with the character's own name, compared over the first 10 stored characters. |
| **Go** | New passwords are argon2id. A stored DES hash still verifies, and is replaced with an argon2id one at the moment the correct password is typed — the only moment the plaintext is known. |
| **Why** | Plan §5.3.1. No resets: an archived character logs in with the password they had. The DES implementation is ported rather than taken from libcrypt, and is verified against the system one over 9,680 pairs, so this is a change of storage rather than of who can log in. |
| **Where** | `internal/auth`, `internal/auth/descrypt`. |

### Deleting a character removes the file

| | |
|---|---|
| **C** | Sets `PLR_DELETED` and saves (interpreter.c:1770). The record stays in the player file; the flag marks its slot reusable. |
| **Go** | Sets the flag on the outgoing record *and* removes the file. |
| **Why** | The tombstone exists because a record in the binary format is a fixed-width slot in one big file. The ascii format the server runs on has no slots — a character is a file — so removing it expresses the same intent in the format actually in use. The flag is still set on the way out so that a restored backup does not quietly bring somebody back with nothing to show why they went. |
| **Where** | `Server.Delete` in `internal/server/server.go`, `TestDeletingACharacter`. |

### A description survives an abandoned edit

| | |
|---|---|
| **C** | Menu choice 2 frees the existing description before the editor opens (interpreter.c:1698), so a player who disconnects mid-edit has lost it. |
| **Go** | The old text is kept until a new one replaces it. |
| **Why** | Nothing is gained by the early free, and the failure mode is silent data loss on a dropped connection. |
| **Where** | `internal/session/menu.go`. |

### Attacking an immortal doubles the damage

| | |
|---|---|
| **C** | `damage()` (fight.c) has, under a comment reading *"You can't damage an immortal!"*, the line `dam = dam*2;`. |
| **Go** | The same. |
| **Why** | This is **not** a deviation — it is reproduced — but it is here because it looks like one and somebody will eventually "fix" it. Stock CircleMUD sets `dam = 0` there; this tree doubles it and the original comment was left in place when the line was changed. Whether that was deliberate or a typo, it is what players fought against for seven years, and the fidelity rule says the C wins. |
| **Where** | `ApplyDamage` in `internal/game/fight.go`, asserted in `TestAttackingAnImmortalDoublesTheDamage`. |

### The wounded-victim damage multiplier is not what its comment says

| | |
|---|---|
| **C** | `hit()` computes `dam *= 1 + (POS_FIGHTING - GET_POS(victim)) / 3` beside a comment listing 1.33x for a sitting victim, 1.66x resting, 2.00x sleeping, 2.33x stunned, 2.66x incapacitated, 3.00x mortally wounded. |
| **Go** | The same arithmetic, so the real multipliers are 1, 1, 2, 2, 2 and 3. |
| **Why** | Integer division. The comment has been wrong since 1993 and the code is what players experienced. Reproduced, and asserted against the C so nobody implements the comment by mistake. |
| **Where** | `Attack` in `internal/game/fight.go`, `TestThePositionMultiplierIsIntegerDivision`. |

### `practice` was a command and is a guildmaster again

**Resolved in Phase 5a.** For Phases 3 and 4 `practice` taught anywhere,
because special procedures did not exist and a character with no way to raise
a skill has no way to cast anything at all. The seam now exists, and the
teaching went back to `SPECIAL(guild)` where the C keeps it: the command lists
what you know and otherwise says *"You can only practice skills in your
guild."*

Kept rather than deleted because the shape is worth having on record — a
deviation taken deliberately, with a note of what would end it, and then
ended.

### `practice` and its own listing disagree about remorting

Not a deviation — reproduced — but startling enough to note. `list_skills` was
rewritten locally to walk the remort vector, so it shows every spell any of
your classes knows. `SPECIAL(guild)` was not, and still checks
`spell_info[n].min_level[GET_CLASS(ch)]`.

So a remorted character can **see** a spell in their practice list and be told
*"You do not know of that spell"* when they try to practise it. Both halves are
the C's, and both are here.

The guild *guard* did get the rewrite the guildmaster did not, so a remorted
character can walk into a guild whose master will then refuse to teach them.

### Names are refused with a reason

| | |
|---|---|
| **C** | `_parse_name` rejects a bad name with "Illegal name, please try another." `Valid_Name` (ban.c:255) and `reserved_word` (interpreter.c:952) each have their own message too, folded into the same "Invalid name, please try another." by `nanny`. |
| **Go** | Says which rule was broken: too short, too long, not all letters, reserved, or matching the xnames list. Also refuses `con`, `nul`, `aux` and `prn`. |
| **Why** | A player who typed an apostrophe deserves to know that is the problem. The reserved names (`con`/`nul`/`aux`/`prn`) are a filesystem concern the C never had, because a name is a filename in the ascii player format. |
| **Where** | `invalidName` in `internal/session/login.go`; the xnames substring check is `Server.DisallowedName` (`internal/server/names.go`), consulted separately at the same call site since it needs per-server data `invalidName` has no way to receive. |

`Valid_Name`'s other check — refusing a name that matches someone currently
`CON_PLAYING` on a live descriptor (ban.c:260-262) — is not ported. It is a
narrower race guard than the roster's own existence check (a mid-creation
name has no roster entry yet to catch it), and porting it needs a live-
connection registry threaded to the name-prompt call site, which nothing
in step 6b otherwise touches. Two connections racing to create the exact
same name at the exact same moment is the only case this leaves open; it
would need its own scoping pass.

### Output is UTF-8, and the client is asked what it reads

| | |
|---|---|
| **C** | Bytes in the world files go out untouched. Most of them were Latin-1 when they were typed. No CHARSET negotiation. |
| **Go** | Output is UTF-8. CHARSET (RFC 2066) is offered, and a client that asks for something else gets its output transcoded per connection. `dlctl convert` moves an old data directory to UTF-8 once, rather than decoding on every read. |
| **Why** | Plan §0. Transcoding is a property of a connection, not of the data. |
| **Where** | `internal/telnet/charset.go`. |

---

## Protocol

### Telnet negotiation is parsed rather than passed through

| | |
|---|---|
| **C** | Sends `IAC WILL ECHO` and `IAC WONT ECHO` around a password (comm.c's `echo_off_str`) and otherwise leaves negotiation bytes in the input for the interpreter. |
| **Go** | The whole stream is parsed. Commands are removed and answered; anything the server did not offer is declined rather than ignored. |
| **Why** | Survivable in 1993 because nobody's client negotiated anything. Not survivable now: a client that offers window size has its NAWS bytes read as a command. |
| **Where** | `internal/telnet/parser.go`, `TestNegotiationNeverReachesTheInterpreter`. |

Answering is done by RFC 1143's Q method (`internal/telnet/negotiate.go`),
which is six states per option per side rather than a reflex reply. The naive
version — WILL answered with DO, DO answered with WILL — loops, because each
end reads the other's answer as a fresh request.

**Suppress-go-ahead is agreed on request and never volunteered**, which keeps
`telnet(1)` in line mode as it is against the C server. Offering it tips the
client into character-at-a-time mode, where the terminal stops echoing and
stops handling backspace and the server is expected to do both instead: a
player typing at the login prompt gets `^M` for Enter and `^?` for backspace.
A client that wants SGA still gets it the moment it asks.

### GMCP

Not in the C at all. Added because §0 intends a web front end, and a browser
client that has to scrape text is a terminal emulator with extra steps.
`Char.Vitals` goes out with every prompt and `Room.Info` with every look, both
gated on `Core.Supports`. See `internal/telnet/gmcp.go`.

---

## Limits the C has none of

Each of these turns an unbounded resource into a bounded one. The C's
behaviour in every case is to keep going until the machine stops.

| Limit | C | Go | Where |
|---|---|---|---|
| Output queue | `txt_q` grows without bound; one stuck client can exhaust memory | 256 pending writes, then the connection is dropped | `outputQueue`, `internal/session/session.go` |
| Connections per address | no limit | configurable, refused with a message | `Limits.MaxPerHost`, `internal/server/listen.go` |
| Time at the login prompt | idle timeout only, once in the game | a login grace period | `Limits.LoginGrace` |
| Telnet subnegotiation | n/a — not parsed | 16KB, then the subnegotiation is dropped rather than delivered truncated | `maxSubnegotiation`, `internal/telnet/parser.go` |
| Input line | `MAX_INPUT_LENGTH`, then the line is truncated | 64KB | `maxLineLength`, `internal/session/session.go` |
| Linkdead body | stands until an idle timeout | reaped after 2 minutes | `linkdeadTimeout`, `internal/server/server.go` |

---

## Internal, not player-visible

These change no behaviour a player can observe. They are recorded because
someone reading the two servers side by side will notice them.

### The random number generator, and one thing it does not reproduce

The C's generator (random.c) is ported exactly and verified against it —
`internal/rng` matches the C draw for draw — 5,000 values from each of six
seeds, 30,000 in all — and reproduces `number()`'s modulo bias across every
range tested. `--rng=circle` selects it; `modern`
(Go's PCG) is the default for ordinary play. See `docs/configuration.md`.

One deliberate difference. `circle_srandom` takes `time(0)` unchecked
(comm.c:406), so a seed of zero — or any multiple of *m* — leaves the
generator returning zero for the life of the process. Reproducing that would
mean reproducing a server whose every roll is the same number, so the
degenerate seeds are mapped to 1. The C could only reach one at 03:14:07 UTC
on 19 January 2038, or by being told to.

### Affects are recomputed from stored real values, not subtracted and re-added

`affect_total` (handler.c:209) walks the equipment and the affect list twice:
once with `add = FALSE`, which *subtracts* every modifier, and once with
`add = TRUE`, which adds them back. There is no record of the unaffected
figures anywhere — the character's current numbers are the only copy, and the
first pass is what recovers the base.

That is correct only as long as nothing changes between the two passes, and
things do: an affect expiring inside the same tick, an object whose applies
were edited by an immortal, a spell that modifies the character it is being
totalled for. Every such case leaves the character permanently a few points
richer or poorer, and the C has no way to notice.

Here the real values are stored (`RealArmor`, `RealHitRoll`, `RealAbilities`
and the rest) and the totals are rebuilt from them. The result is identical
whenever the C's version is correct, and correct where the C's is not. This is
the only place the port deliberately reproduces the *outcome* of a C routine
rather than its method, and it is here because the alternative is a class of
bug that cannot be tested for.

The one thing that still works the C's way is armour class from `ITEM_ARMOR`,
because there it is not a recompute at all: `equip_char` changes the
character's own figure and `unequip_char` changes it back. See
`docs/weirdnumbers.md`.

### Three skill numbers were wrong, and are now right

Not a deviation — a bug in this port, recorded because the shape of it is
worth remembering. `do_start` names six skills by symbolic constant, and this
port took their numbers from the comment beside them rather than from
`spells.h`. Three were wrong: sneak, steal and track were being written into
bash's, kick's and steal's slots.

Nothing caught it, because a skill number is just an integer and every value
looked plausible. `TestSpellNumbersMatchTheHeader` now re-parses `spells.h`
and compares, and `StartingSkills` uses named constants so the numbers appear
in exactly one place.

No saved character is affected: no player has ever practised on this server.
Had one, the damage would have been silent and permanent.

### The room's light count is counted, not carried

`world[room].light` is a counter the C adjusts in five places — `char_to_room`,
`char_from_room`, `equip_char`, `unequip_char` and the burnout timer
(handler.c:381, :403, :539, :573, :832). `Live.LightsIn` counts the room's
occupants instead, asking each one whether they are wearing a lit light.

The answer is the same: the five adjustments do balance, including the awkward
cases — a light that burns out is left worn with zero fuel and therefore stops
being counted by both the counter and the count, and the `IN_ROOM == NOWHERE`
branches that skip an adjustment are paired with a `char_to_room` that makes
it later.

Counted rather than carried because a counter that is adjusted from five places
can drift and a count cannot, and a drifted one is invisible: the room is
simply dark, or simply lit, with nothing to say why. Same reasoning as
`affect_total` above, and the same shape. Rooms hold a handful of people, so
the cost is nothing.

### Undoing a remort says something rather than nothing

| | |
|---|---|
| **C** | `do_remort` builds its confirmation with `snprintf(buf2, ...)` guarded on `undo == 0`, and then calls `send_to_char(buf2, ch)` **unguarded** (act.wizard.c:445–449). There is no `else`. So a god undoing a remort is sent whatever was last in that buffer — which is `buf2`, the argument they just typed, because `newclass` points into it. |
| **Go** | *"%s is no longer a %s."* |
| **Why** | Plan §0 puts the `sprintf`-overlap class of bug in the "fix and record" category rather than the "reproduce faithfully" one. This is squarely that: an uninitialised-buffer read whose output is whatever happened to be lying there. The character's own message is the C's and unchanged — only the god's line is invented, because the C has none to reproduce. |
| **Where** | `doRemort` in `internal/session/remort.go`, `TestRemortUndo`. |

### Ability tables are indexed with a bound

`advance_level` indexes `con_app[]` and `wis_app[]` with the raw score
(class.c:1866). Nothing in the game produces a score outside 0–25, so the C
never reads past the array — but reading off the end of a table is not
behaviour worth reproducing faithfully. `abilityIndex` clamps.
(`internal/game/apply.go`.)

### Saving happens off the world goroutine

`advance_level` and `do_start` call `save_char` themselves. In the port they
do not: a game rule that touches the disk would mean the world goroutine
blocking on it. The caller saves. (`internal/game/class.go`,
`internal/server/server.go`.)

### Flags are 64 bits

The C's `bitvector_t` is `unsigned long` — 32 bits on the platform this was
written for, 64 on modern Linux, which is exactly the silent width change plan
§4 exists to eliminate. `game.Flags` is always 64 bits, and the places that
must round-trip through a 32-bit representation say so. Note that
`asciiflag_conv` computes `1 << (26 + (c - 'A'))` into an `int`, so the C
server itself is broken above bit 31 and data using those bits cannot
round-trip to it whatever the port does. (`internal/game/flags.go`.)

### Binary player records cannot round-trip byte for byte

`save_char` writes an uninitialised stack local (db.c:2204), so two saves of
the same character differ in bytes that were never assigned. The acceptance
criterion is identity across every *significant* byte, not every byte.
(`internal/persist/player/binary`.)

### The server runs only on the ascii format or better

The binary format can still be read and written, because `dlctl` needs both
directions to convert between them. But a live server refuses to run on it.
(Plan §5.2.)

### A password can be set from outside the game

The C has no way for anyone but the owner to change a password: `set`
(act.wizard.c) has no such field, and `nanny`'s menu choice 4 is the only
writer. That is right for a live game and unworkable for an archive, where a
character's password is a DES hash from 2008 that nobody has. `dlctl pfile
passwd <name>` sets one offline, under the same rule the menu applies
(`auth.BadPassword`), refusing any format that cannot store an argon2id hash.

It stays out of the game deliberately. A god who could set another
character's password could log in as them, which is not something any of the
C's immortal levels grant; offline, it is available to whoever already has
the pfiles on disk and could have edited them by hand anyway. It is also not
safe to run against a live server — a logged-in character's record is in
memory and gets written back on the next save — and there is no lock to
enforce that, only the warning in the command's help.
(`cmd/dlctl/passwd.go`.)

---

## Not deviations — gaps still to fill

Listed here so they are not mistaken for deliberate differences.

- **`data/` is the on-disk contract**, decided rather than deviated: both
  servers read the same directory, which is what the world-parity harness and
  the Phase 7 shadow run depend on.
- **`generic_find`'s combined forms are not ported.** `CAN_SEE` and `N.thing`
  both reach the search functions now, so an invisible thief can neither be
  seen nor named and `2.sword` picks the second one. What the C keeps in
  `generic_find` and this port does not is the *bitvector* — one call that
  searches inventory, equipment, the room and the world in a caller-chosen
  combination, and reports which of them it found the thing in. Here each
  command searches the lists it cares about in the order it wants. The
  behaviour is the same for every command ported so far; the shape is not.
- **Eleven of the C's 318 commands are not implemented**, and the plan's
  §10 "What is not in it" lists every one with its `interpreter.c` line. In
  brief: the nine OasisOLC and text editors (Phase 6), `slowns`/`trackthru`,
  and a short tail of `users`, `skillset`, `reload` and `color`. **`hop` is not
  among them**: it is the one `do_action` row the shipped socials file does not
  fill, and `RegisterSocials` gives it a command anyway that answers "That
  action is not supported." — which is what the C does too. `alias`
  is off this list now — landed with the native player format (step 5 of
  `docs/proposals/data-format.md`), including `perform_alias`'s complex
  substitution grammar (`;`/`$1`-`$9`/`$*`/`$$`). `bug`/`idea`/`typo` are
  off it too — `do_gen_write` (step 6b), see the reports entry below.

  Its persistence is not quite everywhere the roster is, though: an
  alias survives a save under `ascii` (it grew an `Aliases:`-tagged section
  for exactly this) and `native` (folded into the one file, §8), but
  **`binary` has no `plralias`-equivalent codec at all** — `alias.c`'s
  format is a separate file the C keeps regardless of pfile format, and
  zero archived instances of it exist anywhere in `data/` to build or
  verify one against. Building a format with no corpus behind it is what
  the "do not read the C and transcribe it" testing discipline warns off,
  so it was not attempted; a character loaded from `binary` simply starts
  with no aliases.

  Two of `do_gen_tog`'s seventeen are among them, and both for the same
  reason: `slowns` and `trackthru` flip a server-wide **global** rather than a
  preference (act.other.c:1021, :1028). A global is right in the C, which is
  one server per process; here the tests build several servers in one, each
  with its own world goroutine, so a command writing a package-level variable
  is a race between them rather than a setting. Whichever lands first has to
  decide where the value lives — most likely on `Live`, beside the world it
  applies to. `slowns` additionally has nothing behind it: this port does no
  reverse DNS to slow down.

  One is worth calling out here rather than leaving in the list.
  **`color`** cannot usefully be written yet: the `PRF_COLOR` bits are
  stored and `set color` works, but nothing in a live session emits colour
  — `internal/game/colour.go`'s `{{...}}` engine (data-format.md §5) is
  data-format machinery for the world/player files, not something a
  session renders with yet — so the command would have nothing to switch.

- **`syslog` sets a preference nothing reads.** `mudlog()` had two jobs: write
  the line, and echo it to online immortals at or above a level. The second
  survives as far as the `wizvis` attribute on the log record
  (`internal/obs/log.go`) and stops there — nothing consumes it, so
  `PRF_LOG1`/`PRF_LOG2` are set and stored by `do_syslog` and no god ever sees
  a log line in-game. That is how immortals actually watched a running game,
  and every `mudlog` call site in the ported commands is a would-be producer
  — `bug`/`idea`/`typo` (step 6b) is the newest of them: it logs through
  `slog` at info level, but its `mudlog(..., CMP, LVL_IMMORT, FALSE)` half
  (act.other.c:904-905) — the in-game echo to online gods — goes nowhere,
  same as every command already on this list.
- **Nothing paginates.** `page_string` (`modify.c:436`) is the C's full
  terminal-height "--More--" pager — `credits`, `wizlist`, `immlist`,
  `background`, `news`, `policies`, `handbook` and now `help` all go
  through it there. None of this port's equivalents do: every long text is
  sent whole, in one write. This was never written down until `help`
  landed (step 6c) made it the most visible instance — individual help
  entries run to several KB, and a client with a short scrollback loses
  the top of one — but the gap is exactly as old as `credits`, the first
  of these commands built. `PAGE_LENGTH` (`comm.h:44`) is a fixed 22
  lines in the C, not a per-player preference — there is nothing to read
  back even if this were ported, only a constant to reintroduce.
- **Combat messages are real for the ordinary weapon swing, and only
  that.** `internal/server/violence.go`'s `s.hit` now ports `dam_message`
  and `skill_message`'s weapon-type half of `misc/messages` in full
  (step 6c), replacing the fixed `"You hit %s. [%d]"`/`"You miss %s."`
  strings with the real, tiered, sometimes-randomised text. Every *other*
  way to hurt somebody — kick, bash, backstab, a spell, anything reaching
  `Violence.Damage` rather than `.Swing` — still prints its own message
  and never calls `skill_message` at all, so those attack types' own
  `misc/messages` entries (spell/skill numbers, not weapon types) go
  unused. Each is its own future pass.

  An unarmed NPC always resolves to bare-hand attack text (`AttackHit`,
  "hit"/"hits") for the purposes of this message, even one the C would
  give a distinct verb via `mob_specials.attack_type`. Nothing in this
  tree parses a mobile's attack type from the world format at all —
  confirmed, not assumed: `MobDef` has no such field, and nothing reads
  one — so there was nothing to resolve it from. Worth a note when the
  world format's mob fields are next revisited.
- **Two special procedures are left.** The subsystems that were blocking the
  rest all landed in 5f and 5g, so the seam now carries the guildmasters,
  guild guards, Puff, fidos, janitors, cityguards, snakes, mobile mages,
  thieves, the dump, the shopkeeper, the banker, the receptionist, the
  cryogenicist, the postmaster and the boards — sixteen in all. Two stock ones
  are left, and both are assigned to vnums the shipped world really has: the
  **pet shop** (`pet_shops`, room 3031, its own two-room buy-a-follower
  mechanic) and the **mayor** (mob 3105, a scripted walk around Midgaard on a
  timer, opening and closing the gates). Neither blocks anything else.
  **`assign_kings_castle`** is a zone-sized script rather than a special and
  stays untouched. The local ones (`talkera`, `marblesa`, `remmob`, `cerberus`,
  `teleporter` and the rest) are attached to vnums that exist only in the
  archived world, so there is nothing here to attach them to.
- **`goto <object>` picks an arbitrary one when several answer to the name.**
  `find_target_room` falls back to `get_obj_vis`, which walks the C's
  `object_list` in creation order; this port walks a map, which has no order
  at all. It matters only for `goto sword` with several swords in the world,
  and keeping a creation-ordered list of every object alive to fix it is not
  worth the cost.

- **`stat` prints "Speaks: [0/0/0]" always.** The three tongue slots are in
  the player file and nothing in this tree ever sets them — `speak` was never
  ported and the C never writes them either. Printed rather than dropped,
  because `stat` prints what is stored.

- **`stat` prints no idle timer.** `char_specials.timer` counts ticks since
  the last command and is used only by the idle-timeout reaper, which this
  port does differently (`linkdeadTimeout`). The field is printed as zero to
  keep the line's shape.

- **`vstat mob` does not visit room zero.** The C reads a real mobile,
  `char_to_room`s it into room 0, stats it and extracts it (act.wizard.c:1305).
  This spawns it where the caller is standing and removes it immediately,
  which matters because `read_mobile` rolls hit points and `stat` prints
  them — so the mobile has to be made rather than read off the prototype.

- **`shutdown`'s five spellings collapse to two outcomes.** The C touches one
  of `FASTBOOT_FILE`, `KILLSCRIPT_FILE` or `PAUSE_FILE` on its way out and
  lets the wrapper shell script decide whether to start the server again.
  This port has no wrapper — the container runtime restarts it, see
  `docs/operations.md` — so `reboot` and `now` ask to come back and `die` and
  `pause` ask not to, and the answer is an exit code rather than a file.

- **A `select` ban is treated as `all`, and no longer has to be.** SELECT lets
  in only characters flagged `PLR_SITEOK`. When this was written nothing in
  the tree could set that flag, so refusing everybody was the safe reading of
  a half-implemented ban — letting them all through would have made `ban
  select` do nothing at all and say nothing about it. `set <name> siteok`
  landed with the rest of `set` in 5i-e, so the flag is now settable and the
  real check is a one-line lookup at the name prompt. Until someone writes it,
  the conservative behaviour stands; the comment at
  `internal/session/login.go` says the same.

- **`show rent` and `show shops` are not ported.** The first is
  `Crash_listrent`, which lists a rent file without loading it; the second is
  `show_shops` in shop.c. Both are listings of their own and neither is
  needed to run the server. `show houses` is `hcontrol show`, which is
  ported, so `show houses` answers "Sorry, I don't understand that." rather
  than duplicating it.

- **`show stats` does not report buffer counts.** `buf_largecount`,
  `buf_switches` and `buf_overflows` count the C's own string-buffer
  recycling, which this port does not have. The three lines are dropped
  rather than printed as zeroes.

- **A malformed `plr_index` line is skipped rather than fatal.** The index is
  derived data — `rebuildIndex` regenerates the whole thing from the player
  files — so a line lost to a parse error comes back the next time anybody
  saves. Refusing to read the file at all is not recoverable: it takes down
  every login and every character creation for everybody. The skipped lines
  are kept and reported rather than swallowed.

- **`set <name> passwd` does not echo the new password.** The C answers
  "Password changed to '<the password>'." in clear on the god's screen — and
  into a snooper's, and into whatever the god's client logs. The password is
  set and the acknowledgement says so without repeating it.

- **`set file <name>` is not supported.** The C loads a character who is not
  logged in, edits the record and writes it straight back. Doing that safely
  needs the same locking the login path has, for a command that is used once
  a year; `set` works on anybody who is logged in.

- **`set <name> idnum` cannot be used at all** — in the C either. The field
  is marked PC, and the handler refuses anything that is not an NPC, so the
  two conditions can never both hold. Ported as it stands rather than
  "fixed", since either reading changes behaviour.

- **`remort` is not ported.** A local addition with a per-character bit
  vector of borrowed class skills, the `IS_<CLASS>` macros that read it, and
  `redeem` for a fallen paladin. See the plan's 5i-h.

- **`redeem` is not ported.** It is the eighth branch of `do_wizutil`, a local
  addition clearing a `PSF_FALLEN` flag on a paladin — and the paladin class,
  the fallen state and the flag it lives in are all part of the remort work.

- **`hunt_victim` is not ported, because nothing calls it.** `graph.c:219`
  walks a mobile along a BFS path towards its prey, and in this whole tree
  the only references to `HUNTING` are the two lines in `handler.c` that
  *clear* it when somebody is extracted. Nothing ever sets it and nothing
  ever calls the function. `Live.FindFirstStep` is there if a hunting mobile
  is ever wanted.
- **A corrupt board file is reported, not deleted.** `Board_load_board` logs
  "Board file %d corrupt.  Resetting." and calls `Board_reset_board`, which
  `remove()`s it (boards.c:470). Deleting the only copy of everything anybody
  ever posted because one length field looked wrong is not a behaviour worth
  reproducing: the board starts empty, the file stays where it is, and the
  error goes in the log.

- **The boards are loaded at boot, not lazily.** The C loads them the first
  time anybody looks at one, from inside the special procedure, behind a
  `static int loaded` (boards.c:150). That is fine with globals and not fine
  on a world goroutine. The only visible difference is when a bad board file
  is reported.

- **Removing a house guest does not read past the end of the list.** The C's
  loop is `for (; j < num_of_guests; j++) guests[j] = guests[j+1];`
  (`house.c:551`), which touches `guests[num_of_guests]` — and with a full
  list of ten that is `guests[10]`, one past the array and therefore the
  first bytes of `last_payment`. The value is overwritten immediately by the
  decrement so it never mattered, but it is a genuine out-of-bounds read and
  there is nothing to be gained by reproducing it.

- **A renting character is not crash-saved on the way out.** Their things are
  already in the rent file and they are carrying nothing, so the crash-save
  the disconnect path would otherwise do writes an empty file over it. The C
  never had to think about this because `extract_char` does not crash-save;
  this port's disconnect handling does.

- **The mail file is held in memory and rewritten whole.** The C seeks around
  it block by block, which is right for 1993 and wrong for a server with one
  goroutine owning the world: `receive` would put a seek and two reads on
  that goroutine. The on-disk format is unchanged, so the C could still read
  it.

- **An emptied mail file is removed.** The C never shrinks its file; once it
  has grown to a high-water mark it stays there, full of blocks marked
  deleted. Removing it when nothing is left is the same thing to a reader and
  leaves no litter.

- **Mail is delivered in ascending block order.** See the weirdnumbers entry:
  the C's order depends on whether the server has been restarted since the
  message was sent.

- **The rent settings are constants, not options.** `free_rent`,
  `min_rent_cost` and `max_obj_save` are compiled in at the values
  `config.c` had. Making them configurable would be a feature; the archive's
  values are what the game was.

  The one that matters: **`free_rent` is YES**, so nobody on this server ever
  paid rent. The receptionist says "Rent is free here.  Just quit, and your
  objects will be saved!" and stops. Every price in `Crash_offer_rent` is
  dead code on these settings — and ported anyway, because the setting is one
  line and the path has to be right if it is ever turned off.

- **Renting empties your bags and strips your body — on `binary` and
  `ascii`.** `USE_AUTOEQ` is 0 in this tree (`structs.h:30`), so
  `struct obj_file_elem` has no `location` member. `Crash_save` still walks
  containers and still computes a location for every item, and the file has
  nowhere to record it — so everything comes back loose in inventory. Sixty
  lines of `Crash_load`'s `cont_row` machinery are dead code in this build.
  That is the C's behaviour, not a limitation of the port, and there is a
  test (`TestRentingEmptiesYourBags`) asserting it so that nobody "fixes" it
  for those two formats.

  **`native` fixes it, as a deliberate, user-approved deviation (step 5 of
  `docs/proposals/data-format.md`, scoped explicitly for this).**
  `internal/server/rent.go`'s object tree was never actually thrown away at
  runtime — `game.Object.Contents` holds real containment the whole time a
  character is in the world — it was only the round trip through storage
  that flattened it. `player.StoredObject` gained a `Contains` field that
  `binary`/`ascii` still always leave empty (their on-disk shape genuinely
  cannot hold it, so those two are unchanged, byte for byte, and the test
  above proves it), but that `native`'s codec, and `rent.go`'s
  `storedTreeFrom`/`restoreOneObject`, populate and honour for real. Running
  `--player-format=native` is what turns this on —
  `TestRentingUnderNativeKeepsTheRingInTheBag` is the same fixture as the
  `ascii`/`binary` test above, quit and logged back in under `native`,
  asserting the opposite outcome. Stock auto-equip (putting worn items back
  *on the body*, the other half of what `USE_AUTOEQ` would have covered) is
  **not** part of this fix — that's a separate deviation nobody has signed
  off on, so worn items still come back loose in inventory under every
  format, `native` included; see `internal/persist/player/native/doc.go`'s
  package comment for why there is deliberately no `equipment:` section.

- **Rent files are never swept.** `update_obj_file()` (objsave.c:332) runs at
  boot unless `-q` was given (db.c:457) and deletes any rent file older than
  `rent_file_timeout` days — 30 for a rent, 10 for a crash save. Nothing here
  does, so a character who stopped playing in 2003 still has their things.
  `--skip-rent-check` is accepted and marked *(inert)* in
  `docs/configuration.md` because this is what it would have skipped.

- **A dropped link crash-saves.** The C leaves a linkdead body standing and
  only writes its objects when the idle timeout forces a rent
  (`Crash_idlesave`). This port crash-saves on any disconnect, quit or not.
  Until the idle timeout lands, the alternative is that a link loss costs
  somebody everything they were carrying, which is a worse answer than a free
  save.

- **Shutdown saves everybody's objects, not only those who picked something
  up.** `Crash_save_all` writes for characters with `PLR_CRASH` set, a bit
  raised by `obj_to_char`. That is an optimisation for a machine that counted
  disk writes; a few hundred small files cannot miss anybody.
