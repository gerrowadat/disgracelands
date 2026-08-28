# Deviations from the C server

The fidelity decision in [the port plan](proposals/go-port-plan.md) §0 says the
patched C server wins wherever it and modern design disagree. This file is
where the exceptions are written down.

**Since 2026-08-23** (§0's "Fidelity, phase two"), that rule covers
*gameplay and compatibility*, not the implementation as a whole: the port is
playable, and new work may modernise the stack — architecture, dependencies,
protocols, tooling — without an entry here, as long as it doesn't change
what a player experiences or what data the server reads and writes. Every
entry below this line, and everything already ported, was made under the
stricter rule that came before and is unaffected by the narrowing.

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

### A paladin's per-level gains are this port's, because the C has none

| | |
|---|---|
| **C** | `advance_level` (class.c:1941) switches on the class and has cases for magic-user, cleric, thief and warrior only. A paladin falls through: `add_hp` is the constitution bonus alone, `add_mana` and `add_move` stay zero, and the two `MAX(1, ...)` floors give one hit point and one movement point per level. No draws are taken. |
| **Go** | A paladin gains `10..14` hit points on top of the constitution bonus and `1..3` movement, and takes those two draws — the warrior's shape, one below the warrior's hit-point range, and with no mana draw, exactly as the warrior has none. |
| **Why** | The C contradicts itself here the same way it does over creating one. Paladin is unreachable at creation and is what remorting is *for*, so the fall-through is an oversight rather than a balance decision: a class you earn by giving up a character would gain one hit point a level, less than any class you can simply choose. Recorded rather than corrected back, because it is gameplay a returning player would notice either way and this is the reading the port has always had. |
| **Where** | `internal/game/class.go`'s `AdvanceLevel`, with the ranges in `TestFirstLevelGainsMatchTheCsRanges`. Written down 2026-08-28, while the creation oracle (#188) was going through the same switch; the behaviour is older than the entry. |

### Passwords: minimum six characters, no maximum

| | |
|---|---|
| **C** | `nanny` (interpreter.c:1526) refuses a password shorter than 3 characters or longer than `MAX_PWD_LENGTH` (10). Traditional DES `crypt(3)` then truncates it to 8 characters regardless. |
| **Go** | Minimum 6, no maximum. |
| **Why** | Both C limits were consequences of the storage, not policy. The ten-character ceiling is the width of the field in `char_file_u`; the three-character floor was a defensible minimum when only the first eight characters were hashed anyway. Under argon2id the whole password is used and the stored form is not fixed-width, so neither limit has a reason to exist. Applies only to passwords being *set* — no existing character is locked out. |
| **Where** | `auth.BadPassword` in `internal/auth/policy.go`, called from `badNewPassword` (`internal/session/menu.go`) and from `dlctl passwd --type=pfile`. |

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

### The improved line editor is all eleven commands, with three memory-safety exceptions

| | |
|---|---|
| **C** | `improved-edit.h` hardcodes `CONFIG_IMPROVED_EDITOR` to `1`, so `/a` `/c` `/d` `/e` `/f` `/h` `/i` `/l` `/n` `/r` `/s` were always available. |
| **Go** | All eleven, checked against the C command by command. |
| **Why** | Not a deviation in itself — it is here as the context for the entries that follow, and because "the improved editor is an optional feature" is the wrong assumption to start from. |
| **Where** | `internal/session/editor.go`, `reference/tools/editoracle.c`, `internal/session/editoracle_test.go`. |

That the editor was always on was found while porting `tedit` (Phase 6's
first slice), when it became the first caller to seed a non-empty buffer.
`internal/session/editor.go` ports `improved_editor_execute`
(`improved-edit.c:27`), `parse_action`, `format_text` and `replace_str`, wired
into `handleEditing` ahead of the plain `@`-terminated accumulate loop every
caller already had (`beginEditor`/`beginEditorSeeded`, used by board `write`,
mail, a note's own `write` and `tedit`). `/h`'s text is the C's own, unedited,
because every command it lists works.

The buffer is held flat — one string, `"\r\n"` between the lines and after
the last — rather than as a `[]string`, because `*d->str` is and because most
of what these commands do is defined against that: `/f` reflows across line
boundaries, `/r` substitutes across the whole thing, and `/d` `/e` `/i` `/l`
`/n` count `'\n'` characters rather than indexing a list. `editText` also
carries the C's own distinction between an *empty* buffer and *no* buffer,
which is not cosmetic: `if (*(d->str))` is a NULL-pointer test, so `/c` (which
frees) and `/d 1-<last>` (which truncates in place) leave the editor in states
that `/f` `/i` `/l` `/n` answer differently.

`reference/tools/editoracle.c` holds the four original bodies and
`internal/session/editoracle_test.go` compares 805 command-against-buffer
cases with it, both the text sent and the buffer left behind. The C's own
surprises that came out of it are in
[`weirdnumbers.md`](weirdnumbers.md#the-line-editor) and are reproduced, not
deviated from. The oracle is built `-O0`, not `-O2`: `PARSE_LIST_NUM`
accumulates with `sprintf(buf, "%s%4d:\r\n", buf, i - 1)`, whose destination
is also its `%s` argument, and modern gcc resolves that undefined behaviour
into something that keeps only the last line, where `-O0` calls glibc and the
self-copy at offset zero is a no-op — which is what the archived server's own
compiler did, and the only reading under which `/n` is a usable command.

The three exceptions follow. `format_text` carries a fourth of the same kind,
unreachable rather than deviated from: `while (strchr(".!?", *flow))` has no
guard on `*flow`, and `strchr(s, '\0')` finds the terminator and returns
non-NULL, so a buffer whose last word is not followed by whitespace walks off
the end of the allocation. Every buffer this editor builds ends `"\r\n"`, so
the C never gets there, and a Go string simply ends.

### A line-range argument with no number in it is read as no argument

| | |
|---|---|
| **C** | `sscanf(string, " %d - %d ", &line_low, &line_high)` (improved-edit.c:163,222,284) returns `EOF`, not `0`, when its argument is empty or all whitespace. All three switches reading it have cases `0`, `1` and `2` and no `default`, so a bare `/d`, or a `/l `/`/n ` whose argument is a lone space, goes on to use two uninitialised stack `int`s. |
| **Go** | `scanLineRange` reports zero conversions for that case, which is the `case 0` every one of those switches already has: "You must specify a line number or range to delete." for `/d`, the whole buffer for `/l` and `/n`. |
| **Why** | Undefined behaviour is not behaviour to reproduce, and this is what the code plainly meant — it is exactly what the same commands do with no argument at all. |
| **Where** | `scanLineRange` in `internal/session/editor.go`; the case shapes the oracle deliberately does not emit, in `reference/tools/editoracle.c`'s `main`. |

### `/r` on an editor buffer that does not exist yet

| | |
|---|---|
| **C** | `/r` is the one command `improved_editor_execute` does not wrap in `if (*(d->str))`, and `PARSE_REPLACE` dereferences the buffer unconditionally — `strlen(*d->str)` in its own space check (improved-edit.c:148). A fresh editor has no buffer until the first line is typed (`string_add`, modify.c:132), so `/r 'a' 'b'` as the very first thing typed is a NULL dereference. |
| **Go** | An absent buffer is treated as an empty one. |
| **Why** | The oracle segfaulted on exactly this before it learned to skip the case. Treating it as empty is the answer the same code gives one line later, once the buffer exists and holds nothing. |
| **Where** | `editorReplace` in `internal/session/editor.go`. |

### `/ra` does not write off the end of its own replace buffer

| | |
|---|---|
| **C** | `replace_str` allocates `replace_buffer` at exactly `max_size`, and after its `rep_all` loop — whose in-loop check budgets only up to `max_size` and does not count whatever is left after the last match — comes an unchecked `strcat(replace_buffer, jetsam)` (improved-edit.c:578,610). A `/ra` that grows the buffer past `max_size` writes past the allocation; the oracle died with `free(): invalid next size (fast)` on its own cases. |
| **Go** | A slice cannot overrun. The answer the code was trying to compute is produced instead. |
| **Why** | The oracle gives that one allocation slack so it survives to print an answer, every comparison it makes still being against `max_size`, so which branch it takes is unchanged. The *visible* half of the same bug is well-defined and is reproduced exactly: a `/ra` that runs out of room leaves the player's buffer truncated at the match it gave up on and reports the string as not found. See [`weirdnumbers.md`](weirdnumbers.md#a-ra-that-runs-out-of-room-truncates-the-buffer-and-denies-it-happened). |
| **Where** | `replaceStr` in `internal/session/editor.go`; `ORACLE_SLACK` in `reference/tools/editoracle.c`. |

### `/l` and `/n` do not go through the pager

| | |
|---|---|
| **C** | `PARSE_LIST_NORM` and `PARSE_LIST_NUM` both end in `page_string` (improved-edit.c:274,338). |
| **Go** | The listing is sent directly. |
| **Why** | Paging mid-edit would need `StatePaging` to remember what state to return to, which it does not — `session/pager.go`'s own doc comment names the identical gap for `background` — and a buffer within any caller's own length limit rarely runs past a screen. Separately, `%d` saturates at `INT_MAX` here where the C's overflows it; both put the line number far past the end of any buffer the editor can hold, so the answer is "out of range" either way. |
| **Where** | `editorList` and `editorListNumbered` in `internal/session/editor.go`. |

### The line editor's abort message is caller-specific

| | |
|---|---|
| **C** | `string_add`'s cleanup table (modify.c:191-205) picks a per-state function, so what an abort says depends on what was being edited. |
| **Go** | The same, through the `done` callback's `saved bool`, which is what `/a` needed: `tedit` prints `tedit_string_cleanup`'s "Edit aborted." and the room announcement (tedit.c:54-57); `mail` prints `playing_string_cleanup`'s "Mail aborted." (modify.c:226-231), which this port also prints for a `@`-terminated save with nothing typed, matching the same C branch; a note's own `write` stays silent, matching the C exactly (neither `PLR_MAILING` nor `mail_to >= BOARD_MAGIC` applies to it). |
| **Why** | The one difference is a board `write`, which prints "Post aborted." rather than the C's "Post not aborted, use REMOVE <post #>." (modify.c:239-243). That message assumes the empty-bodied post was already in the board's list — true in the C, where `Board_write_message` inserts it before editing starts, and not here, where the post is appended only on save. That is an earlier, separate choice, recorded in `boardWrite`'s own doc comment. |
| **Where** | `finishEditing` in `internal/session/editor.go`; `internal/session/boards.go`, `mail.go`, `tedit.go`. |

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
| **Why** | The player format is pluggable (`binary`/`ascii`/`yaml`), each with its own `ObjectStore` and its own idea of where a rent file lives — some of which, `yaml`'s, may not be a bare filesystem path at all. There is no one path to print without `session.Operator.ShowRent` reaching past the seam into a specific store's internals, which the interface exists to prevent. |
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
| Connections per address | no limit | configurable, refused with a message — an IPv6 address counts by its own /64, not by itself, since that is the block an ISP actually hands one subscriber (RFC 6177) | `Limits.MaxPerHost`, `perHostKey`, `internal/server/listen.go` |
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

### perform_dupe_check keeps the C's answers and not its mechanics

`perform_dupe_check` (`interpreter.c:1184`) is ported and behaves as the C
does: one character means one body on one socket, a dropped link
reconnects, a live connection is usurped, a switched one is unswitched, and
the losers are told "Multiple login detected -- disconnecting." Three of
its mechanics have no counterpart here, all for the same underlying reason
— the C is one thread and this is not.

- **The old descriptor's character pointer is not nulled.** The C's
  `k->character = NULL` (`:1211`, `:1218`) exists so that closing the
  displaced socket cannot extract, save or crash-save a body somebody else
  now owns. Here the dupe check runs on the world goroutine on behalf of a
  *different* connection, and `Session.character` is a plain field its own
  goroutine reads — so it sets an atomic flag instead
  (`Session.MarkDisplaced`), and the teardown checks it before calling
  `Leave`. Same effect, no write across the boundary.
- **"Is somebody playing this?" is asked of the body, not the
  connection.** The C tests `STATE(k) == CON_PLAYING`, because
  `d->character` and `ch->desc` are kept in exact correspondence and a dead
  socket is unlinked from `descriptor_list` in the same pass of the game
  loop that notices it. Here a disconnect is asynchronous on purpose (see
  above), so those two come apart for as long as the teardown takes, and a
  link that dropped a second ago would be found still "playing" and
  usurped — telling a returning player their own body had been stolen. The
  port adds `body.Client == that session` and `!session.Closed()` to the
  test, which routes both of those to the reconnect path where they belong.
- **The surplus-body extraction loop has no counterpart** (`:1312-1265`).
  The C walks `character_list` destroying every extra body with the same
  id, under its own comment that "theoretically none should be able to
  exist". None can exist here for a stronger reason than theory:
  `game.Live` indexes players by lowercased name, so two bodies of one
  name cannot be in the world at once — and with the check in place, a
  second body is never put there to begin with.

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

### The yaml format re-spaces a keyword list

Keywords are a list in the yaml format (`keywords: [gate, door]`, §4 of
`docs/design/data-format.md`) and a single space-separated string in the
classic files and in memory. Converting to yaml splits on whitespace and
converting back joins with one space, so a classic keyword string with a
trailing space or a doubled space between two words does not come back the
way it went in: `"cape wool woolen "` becomes `"cape wool woolen"`.

Nothing can observe the difference. The only consumer of the string is
`isname`, which walks it word by word (and had its own semantics pinned by
an oracle — see `reference/tools/`); a run of spaces yields no word, and a
trailing space yields no trailing word, so no keyword is gained or lost.
The alternative is storing keywords as an opaque string, which would give
up the readability and the per-keyword validation the list form exists
for, to preserve whitespace that has no meaning to the server. Four
records in the real world data have it, all trailing spaces a builder left
behind. (`internal/persist/world/yaml`.)

### `.dlversion` names a release, and an existing `1.0.0` stamp will now be refused

`.dlversion` (`docs/design/data-format-versioning.md`) used to hold a
format version this project maintained by hand, fixed at `1.0.0` for its
whole life. It now holds the `major.minor.patch` **release** of the
`dlctl` that wrote the directory, taken from `internal/buildinfo`, and
`dlmud` refuses to boot unless that stamp's major matches its own.

Two consequences worth stating rather than discovering:

- **Any directory already stamped `1.0.0` is refused by every `0.x`
  server**, because major 1 is not major 0. There is exactly one way it
  could exist — a hand-run `dlctl data version --dir=X --write` against
  the old scheme, which is the only thing that ever wrote a stamp — and
  the fix is to run the same command again with a current released
  `dlctl`. The two example worlds carried such a stamp and it has been
  deleted; a fresh `import` no longer produces one.
- **The refusal now runs in both directions.** The old rule only refused
  data from a *newer* major, on the reasoning that a new server reading
  old data is the ordinary case. Under a release semver that reasoning
  does not hold: 1.x and 2.x are two different agreements about what the
  files mean, and a build has no more business guessing at the one it was
  not written for than at one from the future. A differing *minor* warns,
  either way, and starts.

Why the release version at all, given it is the blunter instrument: it is
derived rather than declared, so it cannot be forgotten. A hand-maintained
number lives in one `var` that a change to a writer has no mechanical
reason to touch, and a missed bump produces not an error but silence —
two builds that disagree about the files and neither saying so. The cost
is false alarms, since most releases change nothing about the format;
that trade, and the unreleased-build hole it leaves (`go run` and `go
test` have no version, so they stamp nothing and check nothing), are
argued out in §1.1 and §6 of the design doc.

### A password can be set from outside the game

The C has no way for anyone but the owner to change a password: `set`
(act.wizard.c) has no such field, and `nanny`'s menu choice 4 is the only
writer. That is right for a live game and unworkable for an archive, where a
character's password is a DES hash from 2008 that nobody has. `dlctl
passwd --type=pfile <name>` sets one offline, under the same rule the menu applies
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

## What the session-parity suite found

`test/parity` (`make session-parity`, 2026-08-25) types the same scripts at
the C server and at this one and compares what they said, line for line —
the missing half of the world-parity harness, which compares only what the
two *loaded*. Everything below is a difference it found on its first green
run, listed here because Phase 7's precondition 2 is not "make the two
servers agree" but "every difference is either fixed or decided". Each one
has a matching entry in the suite's own triage table, so a difference that
gets fixed fails the suite with "delete the entry" rather than quietly
staying on this list.

**None of these are fixed.** They are gaps, not decisions — but as of
2026-08-26 every one of them has been *ruled on*, which is the half of
precondition 2 that needed a person rather than a harness. Twenty
differences are listed below; eighteen needed a ruling, and each carries
it. A fixed one keeps its entry, struck through and with what closed it —
the ruling is the record of the decision, and deleting it would lose why
the work was done:

- **Blocker** — fix before cutover. A returning player would not forgive
  it. Sixteen of the eighteen, which is the honest answer to "how close
  is this to being playable by someone who played the original": closer
  than the list looks, since most are one command each, and not as close
  as a green suite suggests. **Ten are fixed so far** — both halves of
  `look`, `remove all`, `get 2.sword`, the refusal wording, `who`, `exits`,
  the `'`/`:`/`hop` split, death and `flee` — leaving six.
- **Later** — a real gap, worth closing, not gating cutover.
- **Accepted** — the difference stands; this file is where it is
  recorded, and that is the whole disposition.

The two 64-bit reference-build entries at the end need no ruling: the
port is right and the thing it is compared against is wrong.

### The two that are in every transcript

- ~~**`quit` returns to the main menu in the C and disconnects here.**~~
  `do_quit` (act.other.c:180) ends in `extract_char`, which puts a playing
  descriptor back into `CON_MENU`; the port's `doQuit`
  (`internal/session/commands.go`) called `Session.Close`. A player who quits
  on the C server can enter the game again without dialling back in.
  *Ruling (2026-08-26):*
  **Blocker.** `quit` is among the most-typed commands in the game and the
  menu is where a returning player expects to land.
  **Fixed 2026-08-28** (#187). `doQuit` now extracts the character and calls
  `Session.ReturnToMenu`, which is `extract_char_final`'s
  `STATE(ch->desc) = CON_MENU; write_to_output(ch->desc, MENU)`
  (handler.c:931). The character stays attached to the connection, because
  the C only frees it when there is no descriptor (handler.c:988) — so
  choosing 1 puts the same record straight back into the world.

  Four things came with it, none of them obvious from the one line that
  changed.

  **The save moved.** The disconnect teardown (`Server.Leave`) used to do
  the save, the crash-save and the removal; a quitting session no longer
  reaches it at all. `Server.ExtractCharacter` does that work instead, and
  `Session.Extracted` is what tells the teardown to stand down —
  `close_socket` takes exactly the same branch, since `IS_PLAYING(d)` is
  false at `CON_MENU` and the C neither announces a lost link nor saves
  again (comm.c:1956). The flag is cleared on entering the world, so a
  player who quits to the menu and then plays on is an ordinary session.

  **Both snapshots are now taken inline**, on the world goroutine, rather
  than by the background write fetching them later. That is the difference
  quitting has from every other save here: the connection stays open, so the
  player can be back in the world writing to the same record before a
  deferred read of it ran. `-race` found it as soon as a test quit and
  re-entered on one connection. `crashSave` is split into `crashFileFor`
  (world goroutine) and `writeCrashFile` (anywhere) for it.

  **No prompt at the menu.** `make_prompt`'s last two branches are both
  `STATE(d) == CON_PLAYING` and its else writes an empty prompt
  (comm.c:1010, :1048, :1051). Nothing here had ever reached that, because
  a command's tail only ran for a playing session; now `session.prompt`
  answers "" for any non-playing state, as the C does.

  **`Live.Remove` ends the fights**, both ways round (handler.c:953-960).
  Its own `stop_fighting` is the easy half; the loop over `combat_list` is
  the one that matters, because a mobile left with `FIGHTING()` pointing at
  somebody no longer in the world swings at them every round forever.
  `do_quit` refuses while `POS_FIGHTING`, so what reaches it is renting and
  being extracted by a god — both of which went through `Remove` already.

  Not ported with it: `extract_char_final`'s loop closing any *other*
  descriptor logged in as the same character (handler.c:925-930).
  `perform_dupe_check` runs at login and already leaves only one, so
  nothing in this tree can reach it.
- **The C prepends a CRLF to any output that interrupts a prompt.**
  `process_output` sends its buffer from `i` rather than `i + 2` when
  `has_prompt` is set (comm.c:1459), and a descriptor is born with
  `has_prompt = 1` — "prompt is part of greetings" (comm.c:1404). Every
  separate write the C makes while answering one command therefore arrives
  with a blank line in front of it. The port has no equivalent and adds
  newlines of its own in other places. **The suite does not compare blank
  lines at all** because of this (`internal/parity/diff.go`,
  `withoutBlankLines`), which is the one place it is knowingly blind: a
  difference that is *only* whitespace would not be caught.
  *Ruling (2026-08-26):*
  **Accepted**, along with the blindness it forces on the suite.
  Reproducing `has_prompt`'s newline solely so that blank lines could be
  compared is a great deal of work for very little evidence; the blind
  spot is bounded and written down here, which is what makes it a decision
  rather than an oversight.

### Things a player would notice

- ~~**A mortal's hit points at creation differ.**~~ A level 1 thief rolled 23
  on the C and 19 here; a cleric, 20 and 19. Both servers are on the C's own
  generator from the same fixed seed and agree about the abilities `score`
  prints, so the dice were in step and the formula was not.
  *Ruling (2026-08-26):*
  **Blocker.** The generators are in step, so this is arithmetic that has
  been read wrong, not a design difference — exactly the shape CLAUDE.md
  says wants a C oracle rather than another reading.
  **Fixed 2026-08-28** (#188), by writing that oracle
  (`reference/tools/startoracle.c`) — and it found two bugs in
  `advance_level`, neither of which was in the hit-point formula, which was
  right all along.

  **The draw order is per class, and it is not the same for every class.**
  A magic-user and a cleric roll hit points, then *mana*, then movement
  (class.c:1948-1950, :1957-1959). A thief and a warrior roll hit points and
  then movement, and set `add_mana` to a plain zero with no draw at all
  (:1965-1967, :1971-1973). This port rolled hit points and movement in one
  tuple assignment and then mana afterwards, for every class. So a
  magic-user's mana roll became their movement and vice versa — 83 movement
  where the C gives 84 — and a thief or warrior took a draw the C never
  takes, putting every roll in the game after a character was created one
  step out of step. That second one is what the parity harness saw: not
  wrong arithmetic, a shifted stream.

  **`Start` did not set the base mana and movement.** `do_start` sets all
  three of max_hit, max_mana and max_move before calling `advance_level`
  (class.c:1897-1899); this set only max_hit. Invisible in play, because
  `init_char` had already set the other two to the same numbers — and
  invisible in the tests, because they measured the *gain* and got the right
  answer from a base of zero. It also meant `TestStartGivesEveryClassSomething
  ToLiveOn` asserted a new character has no mana at all, with a comment
  saying the C prompts `0M` for a new mage. The oracle says 100, which is
  what both servers have always actually shown.

  The oracle runs four classes over six seeds, 200 characters each, from one
  seeded stream, and prints a `number(0, 999999)` after every character.
  That trailing draw is the whole point and is the same trick `randoracle`'s
  alternating mode is built on: without it a missing draw agrees perfectly
  about the character it is missing from.
- ~~**Walking is free here.**~~ `do_simple_move` (act.movement.c) charges
  `need_movement` movement points per room, from the two rooms' sector
  types, and refuses with "You are too exhausted." when there are not
  enough. `Context.moveCharacter` charged nothing: the C's prompt counts
  down 84, 83, 82 across three rooms and this port's stayed at 84.
  *Ruling (2026-08-26):*
  **Blocker.** Movement cost is gameplay and balance, not presentation: it
  is what gates exploration, and the prompt currently displays a number
  that never changes.
  **Fixed 2026-08-28** (#189). `movement_loss[]` (constants.c:768) is
  ported and re-parsed out of the C by a test rather than eyeballed — it is
  a table of numbers indexed by sector, so a transposed pair would read as
  perfectly plausible terrain costs. `game.MovementCost` is
  `(movement_loss[from] + movement_loss[to]) / 2` and **that division
  truncates**, which is where the surprises are: city to field is
  (1+2)/2 = 1, as cheap as walking down the street, and only field to
  forest reaches 2. It is symmetric, so a loop costs the same either way
  round, and the minimum is 1, so nothing is free — see
  `docs/weirdnumbers.md`.

  Three guards, and they are not the same guard. The **refusal** is
  `!IS_NPC(ch)` alone (act.movement.c:130), so an immortal is refused too;
  the **charge** is additionally `GET_LEVEL(ch) < LVL_IMMORT`
  (act.movement.c:161), so an immortal never spends any and so never
  reaches the refusal. The check sits *before* the atrium check, which is
  the C's order — an exhausted player standing in an atrium is told they
  are exhausted, not that the house is private.

  The **wording** needed `do_simple_move`'s `need_specials_check`
  argument, which reaches nothing else this port has ported: `do_move`
  passes 0 and every other caller passes 1 (act.movement.c:249 against
  :233, :522, :535, :555), so somebody who walks into a wall of their own
  accord is "too exhausted" and somebody dragged after a leader is "too
  exhausted to follow" — and only if they have a master at all. That is
  `Context.moveCharacterChecking`.

  Still not ported, and unchanged by this: the boat, tunnel and godroom
  checks that surround it in `do_simple_move`, and `AFF_SNEAK` suppressing
  the leave and arrive messages. The tunnel one has its own entry below.
- **The C colours what it prints and the port does not.** New characters get
  colour turned on for them in both (interpreter.c:1616, a `<DoC>` local
  change the port has as `game.ApplyNewCharacterDefaults`), but the C wraps
  room names, exits and the fight in `CCCYN()`/`CCYEL()` at the call site
  (screen.h:42) while the port renders only the `&`-codes embedded in text
  (`internal/colour`, `Session.SendAt`). Nothing else in the suite is
  compared with the ANSI left in; the `colour` scenario is where it is
  compared instead of stripped.
  *Ruling (2026-08-26):*
  **Blocker.** A flat screen reads as a different game. Mechanical to fix
  — call site by call site — but there are many call sites.
  **Tracked:** #190.
- ~~**`who` prints the class abbreviation for immortals in the C**~~
  (`[Wa 34]` against `[ 34]`) ~~and counts in words~~: "One lonely
  character displayed." against "1 character playing.".
  *Ruling (2026-08-26):*
  **Blocker.** `who` is the first thing most people type on connecting.
  **Fixed 2026-08-27**: the line is `[%s %2d] %s %s` of CLASS_ABBR, level,
  name and title (act.informative.c:1163), and the count is words below two
  and digits above it — including "No-one at all!", which nothing here
  could reach.
- ~~**`exits` shows room vnums to an immortal in the C**~~ (`North - [ 1201]
  The Inn Of The Gods`) ~~and not here.~~
  *Ruling (2026-08-26):*
  **Blocker.** Builders need it, and it is a conditional on the viewer's
  level.
  **Fixed 2026-08-27**, along with two things nobody had listed. **A closed
  exit is not listed at all**: the loop's condition ends
  `&& !EXIT_FLAGGED(EXIT(ch, door), EX_CLOSED)` (act.informative.c:387), so
  a room whose only way out is a shut door answers " None." This port
  printed "East - The door is closed.", a line that appears nowhere in the
  C tree, and a test asserted it. And **blindness is checked first**, with
  do_look's own wording.
- ~~**`'` and `:` are commands in the C and `hop` is a social; here it is
  the other way round.**~~ Visible in `commands` and `socials`, which list
  them.
  *Ruling (2026-08-26):*
  **Blocker.** `'` is the say shorthand and is typed constantly, and the
  split feeds the command table's abbreviation order (`Command.CLine`), so
  this is more than a difference in what `commands` lists.
  **Fixed 2026-08-27**, and neither half was where it looked. `'` and `:`
  were in the table all along with the right `CLine`s — a *listing* filter
  dropped every one-character command, on the stated reasoning that they
  "read as noise in a list". And `hop` was classed by whether a `Social`
  was attached to it, where the C's test is on the function the row points
  at: `command_pointer == do_action || command_pointer == do_insult`
  (act.informative.c:1502). `hop` points at `do_action` and has no entry in
  the shipped socials file, so it had nothing attached and was being listed
  as a command.

  Chasing the last column of that listing found a real gap with nothing to
  do with `commands`: **the command table's minimum levels were typed by
  hand and three were wrong.** `handbook` and `imotd` are LVL_IMMORT in the
  C and `hcontrol` is LVL_GRGOD; all three were unrestricted here, so a
  mortal could read the immortals' handbook and any immortal could build
  houses. `snowball` is LVL_IMMORT too and was reachable by anyone. The
  table's *order* had been derived from the C since Phase 3 and its levels
  never were; `TestEveryCommandsMinimumLevelMatchesTheCSource` re-parses
  interpreter.c's fourth column now, so the next one fails a test instead.
- ~~**`look in <container>` lists the contents in the C**~~ ("bag
  (carried):", then each object) ~~and answers "You see nothing special
  about a bag." here. Same for a corpse.~~
  *Ruling (2026-08-26):*
  **Blocker.** A corpse whose contents cannot be checked is a broken game,
  and the extra descriptions are half of what the world's prose is for.
  **Fixed 2026-08-26**: `do_look` is a dispatcher rather than a command, and
  this port had collapsed its four branches into one — `look in` reached
  `look_at_target`, which describes a thing, rather than `look_in_obj`,
  which opens it. `internal/session/look.go` is the missing three
  (`look_in_obj`, `look_in_direction` and the shared `generic_find` over the
  three object lists); `look <direction>` had not been ported at all and
  falls out of the same dispatch.
- ~~**`remove all` removes everything in the C**~~ ~~and looks for an object
  called "all" here.~~
  *Ruling (2026-08-26):*
  **Blocker.** The `all` and `all.thing` forms are muscle memory; failing
  them is felt in the first minute of play.
  **Fixed 2026-08-27**: `do_remove` was the last of `find_all_dots`'s nine
  callers with neither mode. `perform_remove`'s own two refusals came with
  it, and neither was here before: a cursed item will not come off at all,
  and nothing comes off into hands that are already full — which is why
  `remove all` is not a loop that always succeeds.
- ~~**`get 2.sword` says the count back when it fails here**~~ ("You don't
  see a 2.sword here.") ~~and the C strips it~~ ("You don't see a sword
  here.").
  *Ruling (2026-08-26):*
  **Blocker**, with the refusal wording below it.
  **Fixed 2026-08-27**, and it was not a wording bug. `get_number` does not
  merely read the count off "2.sword" — it rewrites the caller's buffer,
  `strcpy(*name, ppos)` (handler.c:596) — and every FIND_INDIV branch in
  `act.item.c` hands one buffer to the search and then prints that same
  buffer. So six refusals said the count back here and none does in the C:
  `get`, `drop`, `junk`, `remove`, `wear`, `wield`. Which six was found by
  playing them at both servers rather than by reading, because whether a
  refusal is downstream of a search is not visible from the message.
  The larger half: **`get 2.sword` could never succeed here at all.**
  `getFromRoom` matched on the raw argument, so the count was part of the
  name it looked for. It now honours it, and the count picks where the run
  starts rather than how long it is — `get 3 2.sword` is "from the second
  sword, take three" (act.item.c:301).
- **Refusal wording**: `look in nothing` is "There doesn't seem to be a
  nothing here." in the C and "You do not see that here." here.
  `kill self` is "Your mother would be so sad.. :(" in the C, and here
  `self` does not resolve as a target at all ("They aren't here.").
  *Ruling (2026-08-26):*
  **Blocker.** The exact wording is part of what the game felt like, and
  `self` not resolving as a target at all is a gap in the target lookup
  rather than in a string.
  **Fixed**: `look in nothing` with the `look` work on 2026-08-26; `self`
  and `me` on 2026-08-27. The latter is `get_char_room_vis`'s own special
  case (handler.c:1068), whose entire comment in the C is `/* JE 7/18/94
  :-) :-) */`, and where it sits matters three times over: after
  `get_number`, so `2.self` is you and the count is read and discarded;
  before the zero-count branch, so `me` never reaches the players-only
  lookup; and before `CAN_SEE`, which makes yourself the one target a
  blinded character can still name. `get_char_world_vis` inherits all of it
  by delegating (handler.c:1091). `get 2.sword`'s own entry above covers
  the count-stripping half of this bullet.

  Two differences nobody had listed came out of writing the scenario for
  this, which is the argument for scenarios over assertions. **Looking at a
  character said their name and should say a pronoun**: the C is
  `act("You see nothing special about $m.", ...)` (act.informative.c:242),
  and `$m` is the objective pronoun of the person being looked at, so it is
  "about him.", "about her." or "about it." — never "about Zod." or "about
  a large dog." Three tests asserted the name, and it reads naturally
  enough that none of them was ever doubted. And **`look 0.anything` is
  "Look at what?"**, because generic_find gives up on a zero count before
  it searches at all (handler.c:1345); the `0.<name> means a player` branch
  is real but unreachable through `look`, and reachable through `hit`,
  which calls get_char_vis directly (act.offensive.c:108).
- ~~**`look at <object>` finds the object's extra description in the C**~~ —
  the ATM's own note — ~~and answers with the room's line for it here.~~
  *Ruling (2026-08-26):*
  **Blocker**; see `look in` above, which it was ruled on with.
  **Fixed 2026-08-26**: `look_at_target` searches extra descriptions on four
  lists — the room, worn equipment, the inventory, the floor — with a count
  shared across all four, and this port searched only the room's, only after
  failing to find an object. Two unlisted differences came out with it, both
  confirmed against the C rather than reasoned about: looking at an object
  answers `show_obj_to_char`'s "You see nothing special.." (two full stops,
  and the object unnamed) rather than this port's "You see nothing special
  about a long sword." or the room's long description, and a zero count —
  `0.thing`, or a bare leading dot — answers "Look at what?" before any
  search happens.
- **Objects are listed in a different order**, both on the floor and in an
  inventory.
  *Ruling (2026-08-26):*
  **Later** — the one difference of the seventeen that is not a cutover
  blocker. It is a consequence of where the port inserts into its lists,
  visible but harmless, with the caveat that ordering is what `2.sword`
  selects against, so it is not purely presentational.
  **Tracked:** #193.
- ~~**Death.** The C sends `death_cry` to the room~~ ("Your blood freezes as you
  hear the beastly fido's death cry.") ~~and the killer's own "is dead!
  R.I.P." once; the port sends the room's line to the killer twice and no
  cry at all.~~
  *Ruling (2026-08-26):*
  **Blocker.** The death cry is how the surrounding rooms learn something
  died, and the doubled line is a plain bug in `game.Act`'s audience
  resolution rather than a missing feature.
  **Fixed 2026-08-27**, and it was not `game.Act`. Two functions announce a
  death in the C and they announce different things: `damage()`'s position
  switch sends "$n is dead!  R.I.P." (fight.c:891) and `raw_kill` sends
  `death_cry` (fight.c:389). This port's `die` sent the R.I.P. line *as
  well*, so anything reaching it through damage() said it twice. The
  ruling's own reading was half wrong too: what the scenario actually
  exercises is an **implementor's** `kill`, whose gate is `GET_LEVEL(ch) <
  LVL_IMPL` (act.offensive.c:78 — LVL_IMPL, not LVL_IMMORT), and which
  reaches `raw_kill` directly with no damage, no experience and *no R.I.P.
  line at all*. This port reached raw_kill by calling damage() with a large
  enough number, which announced a death the C announces nowhere on that
  path. `session.Violence` grew a `RawKill` so the seam has the shape the C
  does, and `suffer` (bleeding out) now announces through the same
  `announcePosition` a fight does, because in the C that path is damage()
  too.
- ~~**`flee` picks a different exit on the two servers.**~~
  *Ruling (2026-08-26):*
  **Blocker.** Same seed, different exit means the draw order or the
  attempt loop diverges — a fidelity bug, and one worth chasing for what
  else it might indicate about the generators staying in step.
  **Fixed 2026-08-27.** It was worth chasing. The
  attempt loop was fine; the two generators were 289 draws apart before a
  player had typed anything. Both causes were measured rather than guessed,
  by counting draws on each side at boot and at the first `flee`.

  **288 of them were `number(x, x)`.** `Rand.Number` returned early for a
  zero-width range without touching the generator; the C has no such branch
  and reduces a real draw modulo 1. Every d1 in every mobile's hit dice —
  288 across a boot of the stock world — was a draw this port did not take.
  Fixed, with an oracle test that can see it: the existing one compares
  *values*, and `number(1, 1)` answers 1 either way, so it agreed perfectly
  while the generator fell behind. `randoracle` grew an alternating-range
  mode so a degenerate range can be interleaved with a real one, which is
  what makes a missing draw observable at all.

  **The 289th was the weather**, and it is why weather.c is ported now.
  `reset_time` rolls the barometric pressure once at boot (`dice(1, 50)` or
  `dice(1, 80)`) and `weather_change` rolls five more every mud hour —
  `dice(1, 4) + dice(2, 6) - dice(2, 6)` (weather.c:88), sometimes six, from
  `weather_and_time(1)` every 75 real seconds (comm.c:934). Fixing only the
  boot draw would have agreed for a short script and diverged partway
  through a long one, which is worse than a known difference, so the
  barometer, the sky and their four messages all landed together
  (`internal/game/weather.go`). Nothing in the game reads the sky except
  those messages; it exists because it rolls.

  With the generators in step, `flee` picks the same exit on both servers.
  The last difference was **the order of its two outputs**: the C's look
  happens inside `do_simple_move`, so a player sees the room and is then
  told "You flee head over heels." This port said it first.
- ~~**A shopkeeper's and a postmaster's messages carry the player's name
  here.**~~ The C builds them as `"%s %s"` of `GET_NAME(ch)` and the message
  (shop.c's `is_ok_char`, and the same shape in mail.c) and hands the result
  to `do_tell`, which eats the first word as the *addressee* — so the name
  never reaches the player, and `do_tell` capitalises the speaker. Here it
  was "the grocer tells you, 'Parityone You don't seem to have that.'"
  against the C's "The grocer tells you, 'You don't seem to have that.'".
  *Ruling (2026-08-26):*
  **Blocker.** Every shop and post office interaction shows it, and it
  reads as an obvious bug rather than as a quirk.
  **Fixed 2026-08-28** (#191), and it was two bugs sharing a symptom. The
  name is *routing*: `keeperTells` now does `do_tell`'s `half_chop`, drops
  the first word, and resolves it through a new `Live.FindPlayer` —
  `get_player_vis`, which skips mobiles before it compares anything, so a
  mobile answering to the customer's name cannot intercept the shop's
  reply. And the capital came free, because both the shopkeeper and the
  postmaster now deliver through `act("$n tells you, '...'")`
  (`deliverTell`, comm.c:110) instead of formatting the speaker's name in:
  `act` capitalises the finished line, and a mobile's own name is lower
  case. The postmaster was never the `"%s %s"` shape at all — mail.c and
  objsave.c call `act` directly — so for it and the receptionist the
  capital *was* the whole bug. Routing through `act` also means `$n`
  resolves per audience, so a keeper the customer cannot see says "Someone
  tells you", which is what the C has always done.

  `do_tell`'s refusals come with it: `is_tell_ok` now runs for a mobile
  speaker too (`tellIsOKFor`), so a customer with `notell` set, or standing
  in a soundproof room, gets no answer from the shop. That is the C's
  behaviour and it is odd enough to be worth knowing about — the refusals
  themselves go to the *keeper*, who has no client to receive them, which
  is why they are a return value rather than a `Send`.
- ~~**The improved editor prompts for each line with `]` in the C** and
  silently here, and the C confirms a sent letter with "Message sent!".~~
  *Ruling (2026-08-26):*
  **Blocker.** A silent editor gives no sign that anything was received,
  and the missing send confirmation is worse than it looks given that this
  build of the C is the one that eats the letter.
  **Fixed 2026-08-28** (#192). The prompt is `make_prompt`'s second branch,
  `else if (d->str) strcpy(prompt, "] ")` (comm.c:1008) — and the thing to
  notice is what it keys off. Not the connection state: the *pointer being
  written to*. So it fires for the improved editor, for `tedit`, for a board
  post and for the menu's description editor alike, and it sits above the
  `CON_PLAYING` test, so a descriptor at the menu writing a description gets
  `] ` and not the menu's silence. Both paths send it here now
  (`Session.handleEditing` and `handleEnterDescription`); the *first* one,
  after the command that opens the editor, already worked, because
  `Dispatcher.Do`'s tail calls `prompt` and `prompt` had the `StateEditing`
  case all along.

  "Message sent!" is `playing_string_cleanup`'s (modify.c:232). Worth more
  here than in the C: the reference build used for parity is the one whose
  `store_mail` core-dumps on 64-bit and delivers nothing while saying this
  (see "Two that are the reference build, not the port" below), so the line
  this port prints is the only true confirmation either server gives.

### Two that are the reference build, not the port

Both are the C server *as built on a modern 64-bit machine* differing from
the archived 32-bit one, so the port is right and the thing it is being
compared against is not:

- **Shop prices differ by one, and the keeper's stock lists in the opposite
  order.** The price is `cost * profit_buy` computed at whatever width the
  multiplication happens at — which is why the shop-price oracle is built
  `-m32 -mfpmath=387` (CLAUDE.md). This C is neither, and says 78 where the
  port and the archived server say 77.
- **This C build cannot store mail at all.** `store_mail` asserts that its
  header and data blocks are exactly `BLOCK_SIZE` and calls `core_dump()`
  when they are not (mail.c:311-315); on a 64-bit build they are not, and
  the log fills with "SYSERR: Assertion failed at mail.c:313!". The C takes
  the stamp money, says "Message sent!" and delivers nothing. The port
  delivers the letter.

---

## Not deviations — gaps still to fill

Listed here so they are not mistaken for deliberate differences.

- **Whatever `--lib-dir` points at is the on-disk contract**, decided
  rather than deviated: both servers read the same directory, which is
  what the world-parity harness and the Phase 7 shadow run depend on. In
  this repo that directory is `examples/stock/binary/`, the shipped
  example and the Go server's default.
- **`generic_find`'s combined forms are ported only where a command needed
  them.** `CAN_SEE` and `N.thing` both reach the search functions, so an
  invisible thief can neither be seen nor named and `2.sword` picks the
  second one. What the C keeps in `generic_find` is the *bitvector* — one
  call that searches inventory, equipment, the room and the world in a
  caller-chosen combination, and reports which of them it found the thing
  in. `Context.findObjectAndWhere` (`internal/session/look.go`) is that
  call for the three object bits, added because `look_in_obj` needs the
  "which list" answer to head a container's contents with "(carried)" /
  "(here)" / "(used)". **`look` is the only caller so far**; every other
  command still searches the lists it cares about in the order it wants,
  with a counter per list rather than one shared across them. That
  difference is reachable: `2.sword` with one worn and one carried is the
  carried one through `look` and the worn one through everything else.
  **Tracked:** #194.
- **Eight of the C's 318 commands are not implemented**, and the plan's
  §10 "What is not in it" lists every one with its `interpreter.c` line. In
  brief: the seven OasisOLC editors (Phase 6), plus `slowns`. `color` is
  off this list — the `internal/colour` engine landed and every outgoing
  line renders through it (`Session.SendAt`), so `color`'s own command has
  something to switch now; see go-port-plan.md's write-up of that work.
  **`hop` is not among them**: it is the one `do_action` row the shipped
  socials file does not fill, and `RegisterSocials` gives it a command
  anyway that answers "That action is not supported." — which is what the C
  does too. `alias` is off this list now — landed with the yaml player
  format (step 5 of `docs/design/data-format.md`), including
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

  Its persistence is everywhere the roster is: an alias survives a save
  under `ascii` (it grew an `Aliases:`-tagged section for exactly this),
  `yaml` (folded into the one file, §8) and `binary` (`plralias/`, one
  file per character beside the rent files).

  `binary` was the late one, and this paragraph used to record why it had
  been skipped: "zero archived instances of it exist anywhere in the
  surviving archive to build or verify one against", and building a format
  with no corpus behind it is what the "do not read the C and transcribe
  it" discipline warns off. That premise was simply wrong — the archive
  has alias files, and they hold hundreds of aliases. With a corpus the
  objection went away, and the codec was written against
  `reference/tools/aliasoracle.c` (`write_aliases` and `read_aliases`
  themselves) rather than by reading `alias.c` across, which is the point
  the discipline was making.

  Worth writing down, since it is what an eye would get wrong: the
  in-memory replacement always begins with the space `any_one_arg` stopped
  on and did not skip, and the file stores it without — `write_aliases`
  writes `strlen(replacement) - 1` and `replacement + 1`, `read_aliases`
  puts the space back with `*xbuf = ' '`. The `type` field is read and
  recomputed rather than stored, because this port derives simple-vs-complex
  from the replacement at use the same way `do_alias` derives it at
  creation (`interpreter.c:737`); no file `write_aliases` produced can
  disagree, and none in the archive does.

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

- **`syslog` has one real producer so far; every other `mudlog` call site in
  the ported commands is still a would-be one.** `mudlog()` had two jobs:
  write the line, and echo it to online immortals at or above a level whose
  own syslog verbosity (`PRF_LOG1`/`PRF_LOG2`, the two bits `do_syslog`
  sets) is high enough. The second is real now — `obs.WithWizVisEcho`
  (`internal/obs/log.go`) wraps the server's log handler so a record
  carrying both `obs.WizLevel` and `obs.WizType` reaches
  `Server.echoWizVis` (`internal/server/wizvis.go`), which applies the C's
  exact selection (online, `CON_PLAYING`, not switched into an NPC, at or
  above the level, not mid-edit, syslog verbosity at or above the
  message's own type) and sends it in green — but only `bug`/`idea`/`typo`
  (`internal/session/report.go`) actually tags a record this way yet.
  Every other command that calls `s.logger.Info`/`Error`/`Warn` for
  something the C would have `mudlog()`'d is a would-be producer the
  mechanism is ready for but nothing has wired up — auditing every such
  call site against its own `mudlog` in the C is its own pass, not
  attempted here.
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

- **A freed mail block is reused lowest-first, not most-recently-freed
  first.** The C keeps an explicit free list, built at boot by `scan_file`
  and pushed to by `read_delete`, and `pop_free_list` takes from its *front*
  (`mail.c:112`) — so the block freed last is the block filled next. `alloc`
  has no free list and scans for the lowest deleted block instead. Which
  block a given message lands in differs; nothing else does, and either
  server reads either file. Where it shows is that a file this port writes
  can only be compared byte for byte against one the C wrote while no block
  has yet been freed — after that the two disagree about the order and about
  nothing else.

- **A mail block link that names no block ends the message.** The link is a
  byte offset (see the weirdnumbers entry), and `linkIndex` rejects one that
  is negative, not a multiple of `BLOCK_SIZE`, or past the end of the file;
  `readChainLocked` also stops on a link that points back into the chain.
  The C would take the first of those to `read_from_file`'s "invalid filepos
  read" and mail disabled server-wide (`mail.c:196`), the second to an
  `fseek` past the end and an `fread` of whatever the buffer held, and the
  third round forever. A reader that already holds the whole file in memory
  gains nothing from reproducing a read fault, and a hang is not a behaviour
  worth being faithful to.

- **Mail is delivered in ascending block order.** See the weirdnumbers entry:
  the C's order depends on whether the server has been restarted since the
  message was sent.

- **Eleven of `config.c`'s constants are runtime settings now, named by
  name.** `free_rent`, `min_rent_cost`, `max_obj_save`, `auto_save`,
  `autosave_time`, the two corpse timers, `level_can_shout`,
  `holler_move_cost`, `max_filesize` and `max_bad_pws` (the last added
  2026-08-28, with the behaviour it governs — see below) moved from a
  compiled-in value to `game.GameTuning` (`internal/game/tuning.go`),
  overridable by the data directory's own `config/game.yaml` and
  hot-reloadable on `SIGHUP`. This is a deliberate, field-by-field reversal
  of "the archive wins" — a decision, not a format pass — made 2026-08-23.
  Every field still *defaults* to the archive's own `config.c` value, so an
  unconfigured server is unchanged. Everything else in `config.c`
  (`pk_allowed`, the room vnums, autowiz, ...) was considered and left a
  constant on purpose; see `docs/proposals/go-port-plan.md` §9.1 for the
  full survey.

  The one that mattered most while it was fixed: `free_rent` defaults YES,
  so nobody on the archived server ever paid rent. The receptionist says
  "Rent is free here.  Just quit, and your objects will be saved!" and
  stops. Every price in `Crash_offer_rent` was dead code at that
  setting — ported anyway, and now genuinely reachable by turning
  `free_rent` off in `<lib-dir>/config/game.yaml`.

- **The game tuning lives in the data directory**, at
  `<lib-dir>/config/game.yaml`. A new file in what `--lib-dir` points at, so
  it is written down here even though the C never had a file to differ from:
  it changes what a `--lib-dir` contains, which is the compatibility half of
  CLAUDE.md's phase-two rule. Shipped 2026-08-23 as a repo-level
  `config/game.yaml` named by `--config`, moved into the data directory
  2026-08-28 — the placement `docs/design/data-format.md` §6 had specified
  from the start, and the line it draws is the reason: **the data directory
  holds rules, the command line holds deployment.** Whether rent is free is
  a property of *this game*, travels with the world, belongs in its backup
  and is worth reviewing alongside it; which port to listen on and where the
  certificate lives are properties of *this deployment* and must not be in a
  directory that gets copied between them.

  Three consequences worth stating. The file is **optional** — every stock
  and archived `lib/` has no `config/game.yaml` in it, and all of them boot
  on `config.c`'s own values exactly, so a missing file is not an error.
  `--config` (`DL_CONFIG`) **stays**, as a path override for the deployment
  that wants the file elsewhere, and a `--config` that is missing *is* an
  error: it names a file somebody asked for by name. And `dlctl import`
  **carries the file across** unchanged rather than converting it (it is
  this project's own yaml in either format, like `text/`'s prose): a lib/
  that had been tuned must not come out of a format conversion quietly back
  on the defaults.

- **`tunnel_size` was picked for tunability, then found unbuilt.** The
  `TUNNEL` room flag is recognised (parsed, named) but nothing enforces an
  occupancy limit on it. The behaviour is missing from the port entirely,
  not merely hardcoded, so there is no constant to unhardcode yet. Deferred
  rather than built as a side effect of a config-file change: whoever ports
  do_gen_comm's tunnel check should wire it to a `GameTuning.TunnelSize` at
  the same time, rather than reopening `config.c` again for it.

  `max_bad_pws` was the other half of this entry until 2026-08-28, and is
  no longer a gap: the disconnect it governs is built (issue #135), and
  `GameTuning.MaxBadPws` was added with it, exactly as this entry asked
  for. The one difference from the C is the next entry.

- **A wrong password does not overwrite a player who is already logged in.**
  `nanny`'s CON_PASSWORD counts the attempt on the character's own record
  and saves it — `GET_BAD_PWS(d->character)++; save_char(d->character);`
  (`interpreter.c:1466-1467`). `d->character` there is the copy `load_char`
  made when the name was typed, so if the character being guessed at is
  *already playing*, that `save_char` writes their login-time snapshot over
  everything they have done since. Somebody else's typo could cost you the
  last half hour.

  This port bumps the tally on the record as it is *on disk* — load,
  increment, write back (`Server.RecordBadPassword`,
  `internal/server/server.go`) — and never touches the live character. The
  counter still counts, the notice still reports, and nothing is lost. The
  live player's own next save then writes their in-memory tally back over
  it, so attempts made while they were online are not reported to them
  afterwards; the C loses those too, by a different route. Failing to write
  the counter at all is not treated as a failed login, which is what the C
  does with `save_char`'s ignored return.

- **`auto_save` gates `do_save`'s duplication guard too, and now does here
  as well.** `config.c`'s comment on `auto_save` is really about two things:
  the periodic sweep (tunable, above) and `do_save`'s own guard — with the
  sweep on, a `save` from anyone at or below `LVL_IMMORT` writes their
  *aliases* and nothing else, "to stop two clients or a crash duplicating
  items" (`act.other.c:173-186`). Only the sweep was built at first, so
  `save` did a full write regardless of `auto_save`; that gap is closed
  (issue #137), and this entry stays as the record of it having been one.

  The `<=` is the C's own and it is deliberate: its comment says the code
  "assumes that guest immortals aren't trustworthy", so level 31 is *inside*
  the guard and only 32 and up get a real save. The one implementation
  difference left is not visible from the game: the C writes a separate
  `plralias/` file, whereas the two formats a server can actually run on
  (`ascii`, `yaml`) keep aliases on the player record, so "write only the
  aliases" is a read-modify-write of that record — load what is on disk,
  replace the alias list, write it back — in `Server.SaveAliases`. Same end
  state on disk, and the same thing the guard is protecting: whatever the
  sweep last wrote for points, gold and position stands, and no command can
  force a fresh copy of it out.

- **Renting empties your bags and strips your body — on `binary` and
  `ascii`.** `USE_AUTOEQ` is 0 in this tree (`structs.h:30`), so
  `struct obj_file_elem` has no `location` member. `Crash_save` still walks
  containers and still computes a location for every item, and the file has
  nowhere to record it — so everything comes back loose in inventory. Sixty
  lines of `Crash_load`'s `cont_row` machinery are dead code in this build.
  That is the C's behaviour, not a limitation of the port, and there is a
  test (`TestRentingEmptiesYourBags`) asserting it so that nobody "fixes" it
  for those two formats.

  **`yaml` fixes it, as a deliberate, user-approved deviation (step 5 of
  `docs/design/data-format.md`, scoped explicitly for this).**
  `internal/server/rent.go`'s object tree was never actually thrown away at
  runtime — `game.Object.Contents` holds real containment the whole time a
  character is in the world — it was only the round trip through storage
  that flattened it. `player.StoredObject` gained a `Contains` field that
  `binary`/`ascii` still always leave empty (their on-disk shape genuinely
  cannot hold it, so those two are unchanged, byte for byte, and the test
  above proves it), but that `yaml`'s codec, and `rent.go`'s
  `storedTreeFrom`/`restoreOneObject`, populate and honour for real. Running
  `--player-format=yaml` is what turns this on —
  `TestRentingUnderYamlKeepsTheRingInTheBag` is the same fixture as the
  `ascii`/`binary` test above, quit and logged back in under `yaml`,
  asserting the opposite outcome. Stock auto-equip (putting worn items back
  *on the body*, the other half of what `USE_AUTOEQ` would have covered) is
  **not** part of this fix — that's a separate deviation nobody has signed
  off on, so worn items still come back loose in inventory under every
  format, `yaml` included; see `internal/persist/player/yaml/doc.go`'s
  package comment for why there is deliberately no `equipment:` section.

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

