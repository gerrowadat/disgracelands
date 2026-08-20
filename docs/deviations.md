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
| **Where** | `badNewPassword` in `internal/session/menu.go`. |

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
| **C** | `_parse_name` rejects a bad name with "Illegal name, please try another." |
| **Go** | Says which rule was broken: too short, too long, or not all letters. Also refuses `con`, `nul`, `aux` and `prn`. |
| **Why** | A player who typed an apostrophe deserves to know that is the problem. The reserved names are a filesystem concern the C never had, because a name is a filename in the ascii player format. |
| **Where** | `invalidName` in `internal/session/login.go`. |

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

---

## Not deviations — gaps still to fill

Listed here so they are not mistaken for deliberate differences.

- **`data/` is the on-disk contract**, decided rather than deviated: both
  servers read the same directory, which is what the world-parity harness and
  the Phase 7 shadow run depend on.
- The main menu's six choices all work, but nothing behind them reads **mail**
  or handles **rent** yet — the C's menu choice 1 calls `Crash_load` and
  reports lost items. Phase 5e.
- **Nothing reads the visibility flags.** Invisibility, hiding, sneaking and
  infravision are all set correctly by the spells and skills that grant them,
  and no `CAN_SEE` equivalent consults them yet, so a hidden character is
  still listed in the room. Phase 5.
- **`N.thing` targeting is not implemented.** `get_number` splits a leading
  `2.` off any argument and makes the search take the *second* match, in every
  command that uses `generic_find`. `get 2 sword` (a count) works; `get
  2.sword` (the second sword) currently reads the whole word as a keyword and
  finds nothing. It belongs with the rest of `generic_find` rather than in any
  one command.
- **`alias`.** The per-character command aliases, saved alongside the
  character. `alias.c` and the `plralias/` directory.
- **Most special procedures.** The seam exists and ten of the C's specials are
  on it — guildmasters, guild guards, Puff, fidos, janitors, cityguards,
  snakes, mobile mages, thieves and the dump. Shopkeepers, the postmaster,
  bankers, pet shops, receptionists and the boards are not, because each needs
  a subsystem that is not built; the local ones (`talkera`, `marblesa`,
  `remmob`, `cerberus`, `teleporter` and the rest) are attached to vnums that
  exist only in the archived world. `assign_kings_castle` is a zone-sized
  script of its own and is untouched.
- **`remort` and `reroll` are not ported.** `reroll` *is* `do_wizutil`
  (`act.wizard.c:2034`) and `remort` is an implementor command in the same
  file, so both belong with the rest of Phase 5i rather than with the rules.
  `remort` is a large local addition — a per-character bit vector of borrowed
  class skills — and wants its own slice.

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

- **There is no receptionist to rent at.** The rent files themselves are
  wired in — quitting writes one, logging in reads it, unpaid arrears cost you
  the lot — but `gen_receptionist`, `offer` and `rent` are not ported, so the
  only rent code this port ever *writes* is `RENT_CRASH` and the only way to
  pay is not to have to. Renting proper arrives with the shop specprocs.

  Worth knowing: **renting empties your bags and strips your body.**
  `USE_AUTOEQ` is 0 in this tree (`structs.h:30`), so `struct obj_file_elem`
  has no `location` member. `Crash_save` still walks containers and still
  computes a location for every item, and the file has nowhere to record it —
  so everything comes back loose in inventory. Sixty lines of `Crash_load`'s
  `cont_row` machinery are dead code in this build. That is the C's behaviour,
  not a limitation of the port, and there is a test asserting it so that
  nobody "fixes" it.

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
