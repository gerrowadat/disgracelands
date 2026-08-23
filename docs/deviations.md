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

### `show rent` does not print the rent file's own path

| | |
|---|---|
| **C** | `Crash_listrent`'s first line of output is `fname` — the on-disk path `get_filename` built for this exact rent-file format, e.g. `plrobjs/A-E/tenant.objs`. |
| **Go** | The listing starts straight from the rent code ("Rent"/"Crash"/"Cryo"/"TimedOut"/"Undef"); no path line at all. |
| **Why** | The player format is pluggable (`binary`/`ascii`/`native`), each with its own `ObjectStore` and its own idea of where a rent file lives — some of which, `native`'s, may not be a bare filesystem path at all. There is no one path to print without `session.Operator.ShowRent` reaching past the seam into a specific store's internals, which the interface exists to prevent. |
| **Where** | `internal/session/wizshow.go`'s `showRent`, `internal/server/operator.go`'s `Server.ShowRent`. |

### `show shops`'s detail view does not wrap long lists

| | |
|---|---|
| **C** | `handle_detailed_list` (shop.c:1246) breaks a `Rooms:`/`Produces:`/`Buys:` line once it would run past roughly 78 columns, continuing on the next line with a 12-space indent. |
| **Go** | Each list is one comma-joined line, however long. |
| **Why** | Every shop this port's world data actually defines has a short enough list that the C's own wrap would never trigger either — the C's threshold is on rendered width, this port's is "none" — so there is no behaviour difference for any real shop, only for a hypothetical one with enough rooms or products to run past a terminal's width. Reproducing an exact break column for a case nothing exercises was not worth the code. |
| **Where** | `internal/session/wizshops.go`'s `listDetailedShop`. |

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

### `reload` reads on the world goroutine

| | |
|---|---|
| **C** | `do_reboot` (db.c:195) calls `file_to_string_alloc` inline. |
| **Go** | The same, inline, on the world goroutine. |
| **Why** | This is a deliberate exception to the rule that I/O runs off-loop, and it is written down because the rule is otherwise absolute. A dozen small text files, an implementor-only command run about as often as the server is upgraded, and the alternative is a command whose effect arrives some time after it returns — worse to use, and unlike the C. The pulse budget is 100ms and the read is well inside it. |
| **Where** | `Text.Reload` in `internal/server/text.go`. |

### The canned text is the one thing in the server behind a lock

Nothing else needs one: the world goroutine owns the world. But the canned text
is read from *two* goroutines — commands, on the world goroutine, and the
greeting, on the connection goroutine before a session has a character — and
`reload` rewrites it while they do. So `Text` has an `RWMutex` and its readers
are one line each. One implementor-only command is enough to make an unguarded
field a race.

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
- **Eight of the C's 318 commands are not implemented**, and the plan's
  §10 "What is not in it" lists every one with its `interpreter.c` line. In
  brief: the seven OasisOLC editors (Phase 6), plus `slowns`. `color` is
  off this list — the `internal/colour` engine landed and every outgoing
  line renders through it (`Session.SendAt`), so `color`'s own command has
  something to switch now; see go-port-plan.md's write-up of that work.
  **`hop` is not among them**: it is the one `do_action` row the shipped
  socials file does not fill, and `RegisterSocials` gives it a command
  anyway that answers "That action is not supported." — which is what the C
  does too. `alias` is off this list now — landed with the native player
  format (step 5 of `docs/proposals/data-format.md`), including
  `perform_alias`'s complex substitution grammar (`;`/`$1`-`$9`/`$*`/`$$`).
  `bug`/`idea`/`typo` are off it too — `do_gen_write` (step 6b), see the
  reports entry below. `tedit` is off it too — Phase 6's first slice, see
  the improved-editor gap below. `trackthru`, `users`, `skillset` and
  `reload` are off it as well, each its own small slice with nothing more
  to say about it beyond what its own file's doc comment already does
  (`internal/session/toggle.go`, `users.go`, `skillset.go`, `wizops.go`)
  — this paragraph is the record of when they stopped being a list of
  names and became four working commands, so their being gone from the
  list is worth noting even without a story attached.

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

  One of `do_gen_tog`'s seventeen is among them: `slowns` flips a
  server-wide **global** rather than a preference (act.other.c:1021), and
  has nothing behind it besides — this port does no reverse DNS to slow
  down, so a command that reported success would be lying. `trackthru`
  (act.other.c:1028) was the same shape of problem — a global is right in
  the C, which is one server per process; here the tests build several
  servers in one, each with its own world goroutine, so a command writing
  a package-level variable would be a race between them rather than a
  setting — and it is built now: the value lives on `Live`, beside the
  world it applies to (`internal/session/toggle.go`'s own doc comment on
  `doTrackThrough`).

  **`color`** used to be worth calling out here the same way: the
  `PRF_COLOR` bits were stored and `set color` worked, but nothing in a
  live session emitted colour, so the command had nothing to switch. The
  `internal/colour` engine and `Session.SendAt` routing it through on
  every outgoing line closed that gap, and `color` moved off this list
  entirely — see go-port-plan.md's own write-up of that work for what it
  brought with it.

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
- **The pager now covers `credits`/`news`/`info`/`wizlist`/`immlist`/
  `handbook`/`policy`/`motd`/`imotd`/`help`, a bulletin board's message
  list and a message's own body, a shop's `list`, `practice`'s skill
  list, and three of `show`'s fields — not everything `page_string`
  (`modify.c:436`) does in the C.** Ported: `next_page`/`count_pages`/
  `paginate_string`/`show_string` (`internal/session/pager.go`), a real
  `StatePaging` connection state (mirroring `StateEditing`'s own shape),
  and `make_prompt`'s own paging branch (`comm.c:1067`, `"[ Return to
  continue, (q)uit, (r)efresh, (b)ack, or page number (N/M) ]"`) folded
  into `prompt(s)` itself — the same shared place `Dispatcher.Do`'s own
  tail already calls after every command, paging or not, so nothing
  extra had to be threaded through it. `PAGE_LENGTH`/`PAGE_WIDTH`
  (`comm.h:44-45`) are the C's fixed 22×80, not a per-player preference —
  there is nothing to read back even with this built, only the two
  constants.

  The follow-up pass wired the pager into the call sites `page_string`
  reaches beyond the canned texts, each checked against the C rather
  than assumed: `Board_show_board` and `Board_display_msg`
  (`boards.c:281,338`), `shop.c:874`'s `list`, and `list_skills`
  (`spec_procs.c:193`, which builds the whole listing into one buffer
  and pages it once, not a line at a time — the Go `listSkills` was
  restructured to match, since sending line-by-line and pretending it
  was one page would have been a silent behavioural gap). `do_show`
  (`act.wizard.c:2155`) turned out to be a mix, checked field by field
  rather than assumed uniform: **paginated** — `zones` (all three of its
  branches, self/specific-vnum/all, funnel through one shared
  `page_string` call) and the `errors`/`death`/`godrooms` trio, which
  share both the C's own `page_string` call and this port's Go helper
  (`showRooms`); **not paginated** — `player`, `stats` and `snoop`, all
  three plain `send_to_char` in the C and left as plain `Send` here.
  `show shops`' own summary table (`list_all_shops`, `shop.c:1242`) was
  built later, once `show shops` itself landed, and pages the same way;
  its detail view (`list_detailed_shop`) is a series of plain
  `send_to_char` calls in the C, not paginated, and is not here either.
  `show rent` (`Crash_listrent`) has no `page_string` call in the C at
  all — a single plain `send_to_char` — so it is not paginated here
  either, faithfully rather than by omission.

  **`background`'s own pager use (menu choice 3, `interpreter.c:1713`)
  is wired up too, from `CON_MENU` rather than `CON_PLAYING`.**
  Every *other* caller this port paginates runs from `CON_PLAYING`
  only, which is what let `StatePaging` get away without an answer to
  "what was I doing before" for as long as it did — the C never changes
  `STATE(d)` while paging at all, so it never had to ask either.
  `Session.pagerReturn` is the answer: `sendPaged` captures `s.state`
  before overwriting it with `StatePaging`, and `handlePaging` restores
  it — rather than the hardcoded `StatePlaying` every caller but this
  one would have been happy with — once the last page is shown or the
  reader quits. `menu.go`'s own handler for choice 3 sets `s.state =
  StateReadMOTD` *before* calling `SendPaged`, matching exactly what the
  C leaves `STATE(d)` as once `background`'s own `page_string` call
  returns (`interpreter.c:1712-1714`) — so `sendPaged` captures the
  right value without needing to know anything about this one caller in
  particular. The ordinary game prompt is not shown once paging closes
  back into a non-`CON_PLAYING` state — `Session.sendPromptIfPlaying`
  only sends it when `pagerReturn` actually was `StatePlaying` — and
  `users`' own listing shows the state paging really interrupted
  (`Session.ConnectedName`, consulting `pagerReturn`) rather than a
  blanket "Playing" that would be wrong for a reader still on the menu.

  A real bug this pass found rather than assumed away: `next_page`'s own
  algorithm only resets its column counter on `\r`, because every string
  the C holds is CRLF throughout. This port's own `text/` files are
  plain LF on disk (§7: "prose stays prose"), and pagination has to
  normalise to CRLF *before* counting, or a run of ordinary short LF
  lines racks up phantom column-overflow breaks on top of the real
  newline-driven ones — found by a live server test against the real
  archived `text/help/screen` breaking after eleven lines instead of
  the file's real twenty-one, not by inspection.
- **Combat messages are real everywhere `damage()` is reached.**
  `internal/server/violence.go`'s `s.hit` ports `dam_message` and
  `skill_message`'s weapon-type half of `misc/messages` in full (step
  6c), replacing the fixed `"You hit %s. [%d]"`/`"You miss %s."` strings
  with the real, tiered, sometimes-randomised text. `SkillDamage` (the
  same step) does the equivalent for `do_kick`/`do_bash`/`do_backstab`
  and every offensive spell (`internal/session/cast.go`'s `spellDamage`,
  mirroring `mag_damage`'s own C, which ends with
  `return (damage(ch, victim, dam, spellnum))` — magic.c:294, the
  identical dispatch a skill's number goes through) — `skill_message`
  alone, with no `dam_message` fallback, matching `damage()`'s own
  `!IS_WEAPON` branch: an attack type nothing is registered for produces
  genuine silence, not invented text. Confirmed against the real archive:
  every spell `game.SpellDamage` computes damage for has a registered
  entry, except the two local joke spells (`ouchie`, `immolate`), which
  are genuinely silent on a hit — the same rule, not a gap.

  An unarmed NPC always resolves to bare-hand attack text (`AttackHit`,
  "hit"/"hits") for the purposes of this message, even one the C would
  give a distinct verb via `mob_specials.attack_type`. Nothing in this
  tree parses a mobile's attack type from the world format at all —
  confirmed, not assumed: `MobDef` has no such field, and nothing reads
  one — so there was nothing to resolve it from. Worth a note when the
  world format's mob fields are next revisited.
- **Every stock special procedure the archived world actually uses is
  built.** The subsystems that were blocking the rest all landed in 5f and
  5g, so the seam now carries the guildmasters, guild guards, Puff, fidos,
  janitors, cityguards, snakes, mobile mages, thieves, the dump, the
  shopkeeper, the banker, the receptionist, the cryogenicist, the
  postmaster, the boards, the pet shop and the mayor — eighteen in all.
  **`assign_kings_castle`** is a zone-sized script rather than a special
  and stays untouched. The local ones (`talkera`, `marblesa`, `remmob`,
  `cerberus`, `teleporter` and the rest) are attached to vnums that exist
  only in the archived world, so there is nothing here to attach them to.

  The **pet shop** (`pet_shops`, `spec_procs.c:951`) is assigned to room
  3031 itself rather than to a mobile — Midgaard's has no keeper standing
  in it, just a sign — and the animals for sale live in room 3032, found
  by `IN_ROOM(ch) + 1` rather than any lookup, which the port reproduces
  the same blunt way (`internal/session/specprocs.go`'s `specPetShop`).
  One accepted gap in what it does once bought: the C also sets
  `IS_CARRYING_W`/`IS_CARRYING_N` on the new pet to already-maxed values,
  a cache-poisoning trick that stops it being given, wearing or wielding
  anything without needing a real "no carrying" mechanism. This port
  computes carried weight and count from what a character actually holds
  rather than caching them on the character, so there is no field to
  poison the same way — a bought pet here can, in principle, be handed an
  item and carry it, which the real game's pets never could. Small and
  cosmetic (nobody plays with a charmed puppy's inventory), not worth a
  new mechanism invented solely to reproduce a cache trick this port's
  model does not have.

  The **mayor** (mob 3105, `spec_procs.c:277`) is a scripted patrol around
  Midgaard, twice a day. What it is commonly described as doing —
  "opening and closing the gates" — turns out not to be what its own data
  does: the switch has cases for opening/closing a door named "gate", but
  neither of the mayor's two path strings (`open_path`/`close_path`,
  hand-checked character by character rather than assumed from the
  description) ever contains the letters that would reach them. The only
  difference between the dawn and dusk walks is two lines of dialogue
  ('e' versus 'E'). The port keeps the dead door-opening cases anyway —
  `internal/session/specprocs.go`'s `specMayor`, `gateDoor` — the same
  reason the C does: a hand-edited path in the world data could still use
  them, and reproducing the switch whole costs nothing extra; they are
  not separately tested, since nothing in the real archive reaches them
  either. The C keeps the walk's own progress in three `static` locals
  inside the special, shared across every call regardless of which
  mobile makes it — which only works because the real world spawns
  exactly one mayor. This port uses a map keyed by the mobile instead, so
  each instance's walk would stay independent if that ever stopped being
  true, without changing anything the real, single-mayor world can
  observe. `MoveMobile` (`internal/game/live.go`), the mayor's own
  movement, is shared with `wander`'s (`internal/server/mobact.go`) — see
  its doc comment for the corners cut relative to a player's own
  `do_simple_move` (no movement-point cost, no boat/tunnel/atrium/godroom
  checks), inert against the real data either way.
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

- **`show houses` is not ported**, on its own: `hcontrol show` already
  answers the same question, so `show houses` says "Sorry, I don't
  understand that." rather than duplicating it. `show rent` and `show
  shops` are off this list — see "Player-visible behaviour" above for the
  two small differences building them turned up.

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

- **The improved line editor's own commands are implemented, five of
  eleven.** The archived server's `improved-edit.h` has
  `CONFIG_IMPROVED_EDITOR` hardcoded to `1` — `/a` (abort), `/c` (clear),
  `/h` (help), `/l` (list) and `/s` (save) were always on, not a
  stock/optional feature, found while porting `tedit` (Phase 6's first
  slice). `internal/session/menu.go`'s `editorCommand` ports
  `improved_editor_execute` (`improved-edit.c:27`) for exactly those
  five — the ones that need no line-range editing machinery of their
  own — wired into `handleEditing` ahead of the plain `@`-terminated
  accumulate loop every caller already had (`beginEditor`/
  `beginEditorSeeded`, used by board `write`, mail, a note's own `write`
  and `tedit`). `/d` (delete), `/e` (edit a line), `/f` (format/word-wrap),
  `/i` (insert before a line), `/n` (numbered list) and `/r` (replace)
  are not built; typing one of those, or any other letter after a
  leading `/`, gets the C's own default case, "Invalid option." — and
  `/h`'s own text lists only the five that work, rather than advertising
  ones that do not (the C's own help text, `improved-edit.c:104-120`,
  lists all eleven regardless of `CONFIG_IMPROVED_EDITOR`, since it never
  varies with anything).

  `/l`'s optional line-range argument (`parse_action`'s
  `sscanf(string, " %d - %d ", &line_low, &line_high)`,
  `improved-edit.c:222`) is ported closely rather than exactly: Go's
  `strconv.Atoi` on the text either side of a `-`, not a literal
  re-implementation of scanf's own partial-match semantics. A leading
  digit run followed by garbage (`sscanf`'s own "parse what you can, stop
  at the first non-digit" behaviour) is not reproduced; `/l 3x` is treated
  as "not a number" here and falls back to listing the whole buffer, where
  the C would read `3` and list just that line. Typing a clean number is
  the overwhelmingly likely case and both readings still produce a
  listing rather than an error, so the difference was not judged worth
  the extra parsing code.

  `/l` also does not go through the pager (`page_string`, which the C's
  own `PARSE_LIST_NORM`/`PARSE_LIST_NUM` call): paging mid-edit would need
  `StatePaging` to remember what state to return to, which it does not —
  `session/pager.go`'s own doc comment names the identical gap for
  `background` — and a buffer within any caller's own length limit rarely
  runs past a screen anyway, so the listing is sent directly instead.

  `/a`'s abort message is caller-specific, because what "discard this
  edit" means differs by what the edit was for: `tedit` prints
  `tedit_string_cleanup`'s own "Edit aborted." and the room announcement
  (`tedit.c:54-57`); `mail` prints `playing_string_cleanup`'s "Mail
  aborted." (`modify.c:226-231`), which this port now also prints for a
  `@`-terminated save with nothing typed, matching the same C branch — a
  small fix alongside the main one, not a new gap; a board `write` prints
  "Post aborted." rather than the C's own "Post not aborted, use REMOVE
  <post #>." (`modify.c:239-243`), because that message assumes the
  empty-bodied post was already in the board's list, true in the C
  (`Board_write_message` inserts it before editing starts) and not here
  — this port appends only on save, an earlier, separate choice recorded
  in `boardWrite`'s own doc comment (`internal/session/boards.go`); a
  note's own `write`
  stays silent on abort, matching the C exactly (neither `PLR_MAILING`
  nor `mail_to >= BOARD_MAGIC` applies to it, so `playing_string_cleanup`
  has nothing to say either).
