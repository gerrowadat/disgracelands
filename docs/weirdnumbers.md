# Weird numbers

CircleMUD's arithmetic is not obvious. Formulas that look like they mean one
thing compute another, comments describe numbers the code does not produce,
and a fair amount of the game's feel comes from truncation nobody intended.

This is a catalogue of the ones found so far, kept for two reasons. The first
is practical: every one of these is a place where reading the C across into Go
produces something that looks right, passes a plausibility check, and is
wrong. The second is that several of them **are the game** — players spent
seven years fighting against the code, not against the comments, and a port
that quietly corrects the arithmetic is a port of a different MUD.

Where an entry says *verified*, there is a test comparing the Go against a C
oracle built from the original function bodies — see `reference/tools/*.c`.
Where it says *reproduced*, the behaviour is deliberately kept; where it says
*deviation*, it is not, and [`deviations.md`](deviations.md) says why.

---

## The general shape of the problem

Three habits account for most of it.

**Integer division everywhere, including where a float was clearly meant.**
The codebase is from 1993 and division is written `/` regardless of the types
involved. Sometimes an author reached for a float constant to get a fraction
and did not notice the assignment threw it away again.

**Compound assignment into an `int`.** `x -= something_fractional` truncates
*at every step*, so the same terms subtracted in a different order give
different answers, and folding two of them together gives a third.

**Comments written from intent, code written later.** Several formulas have a
comment describing what the author meant and code doing something else. The
comments were never updated because nothing forced them to be.

---

## Combat

### `compute_thaco`: truncation after each subtraction

```c
calc_thaco -= (GET_INT(ch) - 13) / 1.5;   /* Intelligence helps! */
calc_thaco -= (GET_WIS(ch) - 13) / 1.5;   /* So does wisdom */
```

`calc_thaco` is an `int`. The right-hand side promotes to `double`, the
subtraction happens in `double`, and the assignment back into an `int`
truncates towards zero. So **the result truncates, not the adjustment**.

The obvious reading is to truncate the adjustment first — take
`int((18-13)/1.5) = 3` and subtract 3. That is wrong by one for most inputs,
and it still produces a perfectly plausible to-hit number, which is why it
survives inspection. Folding both terms together before subtracting gives a
third answer again.

*Verified*: 1,512,000 values across every class, level, strength band,
hitroll, intelligence and wisdom (`fightoracle.c`, `TestComputeTHAC0MatchesTheC`).

### The wounded-victim multiplier is not what its comment says

```c
/*
 * Position sitting  1.33 x normal
 * Position resting  1.66 x normal
 * ...
 * Position mortally 3.00 x normal
 */
if (GET_POS(victim) < POS_FIGHTING)
  dam *= 1 + (POS_FIGHTING - GET_POS(victim)) / 3;
```

Integer division. `POS_FIGHTING` is 7, so a sitting victim (6) gives
`1 + 1/3` = 1, resting (5) gives `1 + 2/3` = 1, and only sleeping and below
reach 2. The actual multipliers are **1, 1, 2, 2, 2, 3** — the comment's
1.33x and 1.66x do not exist and never have.

The comment even acknowledges the fragility: *"this is a hack because it
depends on the particular values of the POSITION_XXX constants"*. It depends
on more than that.

*Reproduced and verified.*

### Attacking an immortal doubles the damage

```c
/* You can't damage an immortal! */
if (!IS_NPC(victim) && (GET_LEVEL(victim) >= LVL_IMMORT))
  dam = dam*2;
```

Stock CircleMUD sets `dam = 0` here. This tree doubles it, and the original
comment was left in place when the line changed. Whether that was deliberate
or a typo is unknowable now, and it is what the game did.

*Not reproduced any more.* It was, for the whole of Phases 1-5, with a test
named after the surprise so nobody tidied it away. #261 reverses that
deliberately: the port sets `dam = 0`, which is stock CircleMUD's behaviour
and what this very comment says. It stays in this file because the constant
is still surprising and somebody reading `fight.c` will still find it — but
the entry now records a difference the port makes on purpose rather than one
it carries. [`deviations.md`](deviations.md) has the reasoning, under "An
immortal takes no damage, where the archive doubled it".

### Armour class has a granularity of ten

```c
victim_ac = compute_armor_class(victim) / 10;
```

Armour is stored in tenths and compared in whole points, so ten points of
armour class are one point of anything. `dex_app[].defensive * 10` exists to
put dexterity's contribution on the stored scale — and then this divides it
straight back out. A one-point defensive bonus from dexterity is worth exactly
one point of the comparison; nine points of magical armour are worth nothing
at all unless they cross a multiple of ten.

*Reproduced.*

---

### Backstab multiplies by 20 for immortals

`backstab_mult` returns 2 through 6 across the mortal levels and then **20**
for anyone at or above `LVL_IMMORT`. Not 7, not a continuation of the curve.

---

## Character progression

### No mana at level one

```c
if (GET_LEVEL(ch) > 1)
  ch->points.max_mana += add_mana;
```

`advance_level` is called at level one by `do_start`, and the guard means the
mana gain is skipped. `do_start` never touches `max_mana` either. So a level-1
character's mana is whatever `init_char` gave them — a flat 100, for everyone,
regardless of class.

This surprises in both directions. A port that adds a level's mana at creation
is visibly wrong; a port that concludes a new character therefore has *no*
mana is also wrong, and produces the memorable `0H 0M 0V` prompt that started
this catalogue.

### `full heal` is MAG_UNAFFECTS and `mag_unaffects` has never heard of it

```c
spello(SPELL_FULL_HEAL, "full heal", 200, 100, 5, POS_FIGHTING,
	TAR_CHAR_ROOM, FALSE, MAG_POINTS | MAG_UNAFFECTS,
	NULL);
```

`full heal` is a local addition (spell_parser.c:1007-1008), and whoever added
it gave it the same routine pair as `heal`. But `mag_unaffects`'s switch
(magic.c:910-929) has cases for `cure blind`, `heal`, `remove poison` and
`remove curse` and nothing else, so every `full heal` falls through to:

```c
default:
  log("SYSERR: unknown spellnum %d passed to mag_unaffects.", spellnum);
  return;
```

So on the archived server `full heal` healed you completely, did *not* cure
blindness despite carrying the flag that says it should, and wrote a SYSERR
to the syslog every single time it was cast. The port does the first two and
not the third — there is no logger on the command context and the
player-visible behaviour is identical either way.

Worth knowing because the obvious "fix" — adding a `SPELL_FULL_HEAL` case
alongside `SPELL_HEAL` — would be a gameplay change dressed up as tidying:
`full heal` would start curing blindness, which it never did.

### A hundred mana, floored again on every load

```c
if (ch->points.max_mana < 100)
  ch->points.max_mana = 100;
```

`store_to_char` (db.c:2254-2255) applies this to every character on the way in
from disk, before the stored affects go back on — so it raises the *base* every
affect modifier is applied to, and `char_to_store` writes that base straight
out again. A character who ends up under a hundred comes back with a hundred
and keeps it.

It is invisible almost all of the time, which is what makes it easy to leave
out: a hundred is the flat figure `init_char` gives everybody (above), and
`advance_level` only ever adds, so nothing in ordinary play takes a character
below it. It fires for records something else has written — a `set mana`, a
conversion from another server, a hand-edited file — and for those it is the
difference between a mage with 40 maximum mana and a mage with 100.

Ported in #295, along with the restore below; neither had an equivalent
anywhere in the Go server.

### An hour away and you come back whole

```c
if (!AFF_FLAGGED(ch, AFF_POISON) &&
      time(0) - st->last_logon >= SECS_PER_REAL_HOUR) {
  GET_HIT(ch) = GET_MAX_HIT(ch);
  GET_MOVE(ch) = GET_MAX_MOVE(ch);
  GET_MANA(ch) = GET_MAX_MANA(ch);
}
```

The last thing `store_to_char` does (db.c:2276-2287). Log off hurt, come back
an hour later, and you are full — of hit points, mana and movement at once —
unless you are poisoned.

Three things about it are easy to get wrong, and all three are ordering:

- it reads `st->last_logon`, the value *in the file*, not the `time(0)` that
  `store_to_char` has already written into `ch->player.time.logon` twenty
  lines earlier;
- it runs *after* `affect_to_char`, so the maxima it fills to include
  everything the character is wearing and carrying;
- the poison exemption reads the flags after the affects too, which is the
  only point at which anyone is poisoned at all — poison is an affect.

An hour is `SECS_PER_REAL_HOUR` (utils.h:116), real time, not mud time.

### `MAX(1, ...)` on the gains, and where it bites

```c
ch->points.max_hit += MAX(1, add_hp);
ch->points.max_move += MAX(1, add_move);
```

A magic-user with a very low constitution can roll a negative hit-point gain:
`con_app[0].hitp` is −4 and `number(3, 8)` can be 3. Magic-users and clerics
roll `number(0, 2)` movement, so one level in three gives none. Neither may
cost a level's progress, so both floor at one.

The floor is easy to drop when porting because it looks defensive rather than
load-bearing. It is load-bearing: without it, a third of a magic-user's levels
would grant no movement at all.

### Exceptional strength sits *after* the godly scores

`str_app[]` has 31 rows for 26 possible scores. Rows 0–25 are the plain
scores, and rows **26–30** are the five bands of exceptional strength that
only an 18-strength warrior can have:

| Percentile | Row |
|---|---|
| 18/01–50 | 26 |
| 18/51–75 | 27 |
| 18/76–90 | 28 |
| 18/91–99 | 29 |
| 18/00 | 30 |

So a character with strength 18 and a percentile of 100 reads row 30, not row
18 — and rows 19–25, the godly strengths reachable only by magic, sit
*between* the plain 18 and the exceptional bands. `STRENGTH_APPLY_INDEX`
exists to do this and is easy to skip, because indexing `str_app[GET_STR(ch)]`
compiles and returns plausible numbers.

*Verified against `constants.c`, with a separate test asserting the bands beat
a plain 18 and are ordered among themselves — a row-by-row comparison would
happily agree with a table shuffled the same way the C's is.*

### Mana per level is a float multiply cast back to an int

```c
add_mana = number(GET_LEVEL(ch), (int) (1.5 * GET_LEVEL(ch)));
```

The cast truncates, so the upper bound is `floor(1.5 × level)`. Integer
`level * 3 / 2` gives the same answer for every positive level, but only
because both truncate in the same direction; it is worth knowing they agree
rather than assuming it.

### The experience tables stop at 31 and are priced off a ceiling above it

```c
if (level > LVL_IMMORT)
  return EXP_MAX - ((LVL_IMPL - level) * 1000);
```

`EXP_MAX` is 10,000,000 and the mortal tables top out between 7 and 9.5
million depending on class, so
immortal levels are priced just below the ceiling and 1000 apart. It means
immortal levels can be added without touching a table, and it means an
out-of-range level returning 0 — which is what the C does after logging a
SYSERR — makes every "have I earned this?" comparison succeed.

*Deviation*: an out-of-range level is unreachable here rather than free.

### A player's abilities are clamped to 18, and an immortal's decay

`affect_total` ends with

```c
i = (IS_NPC(ch) ? 25 : 18);
GET_DEX(ch) = MAX(0, MIN(GET_DEX(ch), i));
```

— so a **player's** ceiling is 18 and a **mobile's** is 25. `init_char` gives
an implementor 25 in everything, and the first time anything totals their
affects — a spell landing, a shield going on — five of the six drop to 18 and
never come back, because the real values are clamped along with the rest.

Strength is the exception, and the exception is the interesting part: anything
above 18 is *converted* rather than discarded, ten percentile points per
point, capped at 100. Twenty castings of strength leave a character at 18/100
rather than at 58.

*Reproduced*, including the decay. `PlayerRecord.Mobile` carries the `IS_NPC`
distinction, because the two ceilings are the only place a record needs to
know.

### Splitting gold mints a coin

`do_split` charges the splitter for the shares it hands out and then gives
them the remainder they never gave away:

```c
GET_GOLD(ch) -= share * (num - 1);
...
if (rest) {
  ...
  GET_GOLD(ch) += rest;
}
```

Ten coins split three ways is three each and one over. The two others gain
three apiece, the splitter loses six — and then gains one. A hundred coins in
the world before, a hundred and one after.

*Reproduced.* It is small, it is a hundred per cent reliable, and somebody
with a patient friend and an afternoon could have made real money out of it.

### Group experience rounds up, so a group mints experience

```c
tot_gain = (GET_EXP(victim) / 3) + tot_members - 1;
base     = MAX(1, tot_gain / tot_members);
```

`+ tot_members - 1` is the standard trick for making integer division round
up, so three people splitting ten points get **four each** — twelve points out
of a ten point kill. A group earns strictly more in total than a soloist
would, which is the incentive to group and looks accidental until you notice
it is not.

What is *missing* is more interesting: `group_gain` has no level-difference
bonus at all, where `solo_gain` gives up to double for killing something eight
levels above you. Killing something far above your level is worth more alone.

*Verified* against the arithmetic at every group size from one to twenty.

---

### advance_level rolls mana for classes that cannot have any

```c
  case CLASS_THIEF:
    add_hp += number(7, 13);
    add_mana = number(GET_LEVEL(ch), (int) (1.5 * GET_LEVEL(ch)));
    add_mana = MIN(add_mana, 10);
    add_move = number(1, 3);
    break;
  ...
  if (GET_LEVEL(ch) > 1)
    ch->points.max_mana += add_mana;
```

All five classes roll `add_mana`, and they roll it *between* the hit points
and the movement. A thief has no mana and never will; the roll happens
anyway, the number is capped at ten, and then — at level one — the guard four
lines further down throws it away. The guard is on the *addition*, not on the
roll.

That makes the order load-bearing in a way the arithmetic is not. Hoisting
the mana roll out of the switch on the grounds that every class computes it
identically — which they do — and taking it last hands the mana number to
`add_move` and the movement number to `add_mana`. A magic-user ends up with
83 movement where the C gives 84, and everything drawn afterwards is one step
along. Neither symptom points anywhere near `advance_level`.

`(int)(1.5 * GET_LEVEL(ch))` is a genuine float multiplication truncated back
to `int`, but for the positive levels this is ever called with it agrees with
`level * 3 / 2` in integer arithmetic, so there is no `shopprice`-style width
problem here.

*Source*: `class.c:1861-1909`. *Reproduced* in `game.AdvanceLevel`, checked
against `reference/tools/startoracle.c`.

### The two C trees disagree about this, and only one of them counts

`reference/WipeMud-src/`'s `advance_level` has no mana roll for a thief or a
warrior and no `CLASS_PALADIN` case at all, and its `do_start` sets max_mana
and max_move as well as max_hit. `reference/moderncserver/`'s does none of
those things. **moderncserver is the server that was played**
(`reference/README.md`); WipeMud is a snapshot of an abandoned 3.1 upgrade,
kept for comparison.

Recorded here because the difference is silent: both files are called
`class.c`, both are plausible, and a fix written from the wrong one compiles,
reads correctly and passes every test that does not have an oracle behind it.
One did, on 2026-08-28, and was reverted the same day.

## Regeneration

### `graf`'s 60–79 band divides by 20

```c
else if (age <= 79)
  return (p4 + (((age - 60) * (p5 - p4)) / 20));   /* 60..79 */
```

Every other band spans fifteen years and divides by fifteen. This one spans
twenty. It reads like a copy-paste error and is not — the band really is
`60..79` — but it is the single most likely place for a port to "fix"
something that is already correct.

*Verified at every age from 0 to 120.*

### Age is unreachable below seventeen

`age()` adds 17 to whatever `mud_time_passed` returns, so no character is ever
younger than seventeen. `graf`'s `age < 15` band is therefore dead code in
practice, and a port that tests the formula only through `Age()` will never
exercise it.

### The order of the final two adjustments

```c
if (AFF_FLAGGED(ch, AFF_POISON))
  gain /= 4;
if (IS_SET(ROOM_FLAGS(...), ROOM_GOOD_REGEN))
  gain += (gain * 1);
```

Poison quarters, *then* the room doubles. A poisoned character in a
good-regeneration room gets half of what they otherwise would — not a quarter,
and not the full amount. Reversing the two gives a different answer because
both truncate.

`gain += (gain * 1)` is doubling with the working left in.

*Verified across 36,288 combinations of age, position, class, hunger, poison
and room.*

### The class test is remort-aware

```c
if (IS_MAGIC_USER(ch) || IS_CLERIC(ch))
  gain /= 2;    /* Ouch. */
```

`IS_MAGIC_USER` is not `GET_CLASS(ch) == CLASS_MAGIC_USER` in this tree — it
consults `remort_vector`, so a warrior who remorted through cleric heals at a
caster's rate **for the rest of their life**, and gains mana at one as
compensation. The stock definitions are still in `utils.h`, commented out
directly above the replacements, which makes the plain reading very easy to
copy by mistake.

---

## The random number generator

### `number()` is biased, and the bias is the game

```c
return ((circle_random() % (to - from + 1)) + from);
```

A modulo reduction of a value that is not a multiple of the range is biased
towards the low end. For a d20 off a generator with period 2³¹−1 the bias is
immeasurable in play; for `number(1, 3)` it is not quite nothing. Replacing it
with a uniform reduction would be more correct and would produce different
numbers, which is the one thing a faithful port must not do.

*Reproduced and verified*, including across negative ranges and arguments
passed the wrong way round (which the C silently swaps after logging).

### The generator is exactly portable, and that is the whole game

`circle_random` is the Park–Miller minimal standard with the Schrage
decomposition — the constants are chosen so no intermediate value overflows a
signed 32-bit integer. That is why the same sequence came out of a VAX and
comes out of anything since, and it is why `--rng=circle` with a fixed seed
lets this server roll *the same numbers* as the C, draw for draw. Most of the
verification in this file rests on it.

*Deviation*: a seed of zero (or any multiple of *m*) leaves the C's generator
returning zero forever, and is mapped to 1 here.

---

## Spells

### Mana cost is computed from the mage's and cleric's levels, whoever is casting

```c
return MAX(SINFO.mana_max - (SINFO.mana_change *
        (GET_LEVEL(ch) - MIN(SINFO.min_level[0], SINFO.min_level[1]))),
       SINFO.mana_min);
```

`min_level[0]` and `min_level[1]` are the magic-user's and cleric's learning
levels, indexed by literal number. The caster's own class never enters into
it — so a paladin's costs are worked out from what a mage and a cleric would
have paid, and a spell no caster class learns is priced off `LVL_IMMORT`.

Whether that is a shortcut or an oversight is unknowable now. It is
reproduced.

### A class absent from the level list is barred, not merely unlisted

`spello()` fills every class's slot with `LVL_IMMORT` before
`init_spell_levels()` lowers the ones it names. So "this class has no entry"
and "this class cannot cast it below immortal level" are the same statement,
and a port that defaults a missing entry to 1 hands every spell to everybody.

### Skills live in the spell table with every number zero

`skillo(skill, name)` is `spello()` with zeros for mana, position, targets and
routines. A skill is therefore a spell with no cost, no target and nothing to
do — the table is a table of *things you can practise*, and the spells just
happen to be the ones with behaviour attached.

### `control weather` is a spell nobody ever wrote

```c
spello(SPELL_CONTROL_WEATHER, "control weather", 75, 25, 5, POS_STANDING,
	TAR_IGNORE, FALSE, MAG_MANUAL,
	NULL);
```

It is in the spell table (spell_parser.c:908-910), it is cleric 17
(class.c:2054), it has a name, a mana cost and a minimum position — and there
is no `spell_control_weather` function anywhere. Not in the archived server,
not in stock CircleMUD 3.0 bpl20, not in WipeMud. `call_magic`'s `MAG_MANUAL`
switch has ten cases and this is not one of them (spell_parser.c:294-306),
and the switch has no `default`, so the spell falls out of it and
`call_magic` returns 1 regardless.

A cleric who reached level 17 could therefore learn control weather, cast
it, be charged the twenty-five mana, and watch the weather do exactly what
it was going to do anyway. For seven years.

Worth writing down because the port's own honesty makes it easy to get
wrong in the other direction: `castSpell` says "Nothing seems to happen. (X
is not implemented yet.)" for a spell nothing handled, which is a good thing
to say about a gap here and the wrong thing to say about this one. The gap
is upstream, in 1993. #300.

### `identify` is spell 201, and cannot be cast

`do_cast` refuses any spell number above `MAX_SPELLS` (130), and
`SPELL_IDENTIFY` is 201 — up among the NPC spells. So `cast 'identify'`
answers *"Cast what?!?"* in the C, and the spell is reachable only from a
scroll, potion, wand or staff, which route through `call_magic` directly.
Several other useful things live up there too.

*Reproduced.* The report is tested directly until `use`, `quaff` and `recite`
exist.

### Enchant weapon's bonus is a boolean used as a number

```c
obj->affected[0].modifier = 1 + (level >= 18);
obj->affected[1].modifier = 1 + (level >= 20);
```

`(level >= 18)` is a C comparison, so the bonus is +1 below the threshold and
+2 at or above it, with nothing in between and no further growth however high
the caster goes. Damroll crosses over two levels later than hitroll, for no
reason the code gives.

### Drunk speech is a local mod, and it truncates at 240 characters

`do_say` (act.comm.c:52) is rewritten in this tree: above a drunkenness of
five, every `s` becomes `sh` and one time in three the sentence ends
"...*hic*.". It builds the slurred version into a 256-byte buffer and stops
copying at 240 characters, so a long enough sentence is cut off mid-word —
and never gets its hiccup, because the hiccup is appended after the loop.

*Reproduced*, including the truncation. Note also that the doubling makes the
output longer than the input, so it is the *slurred* length that hits the
limit: a sentence of two hundred characters with a lot of esses in it will be
cut where the same sentence without them would not.

### Charm's duration is charisma divided by intelligence

```c
af.duration = 24 * 2;
if (GET_CHA(ch))    af.duration *= GET_CHA(ch);
if (GET_INT(victim)) af.duration /= GET_INT(victim);
```

Forty-eight hours, multiplied by the caster's charisma and divided by the
victim's intelligence — so a charismatic mage charming something stupid holds
it for weeks of mud time, and the only guard against dividing by zero is the
`if`. An 18-charisma caster charming an 11-intelligence mobile gets 78 mud
hours; the same caster charming another 18 gets 48.

---

## Saving throws

### Lower is better, and a bonus is negative

The table value is a target the roll must come in under, so a level-1 mage
needs 70 and a level-30 one needs 28. The character's own bonuses are added to
that target, which means **a negative bonus is an improvement**. The C's
comment apologises for it:

> *"Negative apply_saving_throw[] values make saving throws better! Then, so do
> negative modifiers. Though people may be used to the reverse of that."*

### A perfect saving throw is not automatic

```c
if (MAX(1, save) < number(0, 99))
```

The floor of 1 means a target of zero — or of minus a thousand — still has to
beat a roll, and `number(0, 99)` can return 0. So there is always about a one
in a hundred chance of failing, however good the character.

### Mobiles save as warriors

`mag_savingthrow` starts `class_sav = CLASS_WARRIOR` and only overrides it for
players, so a mobile's own class never affects its saves. The C's comment on
this is *"NPCs use warrior tables according to some book"*.

---

## Light and darkness

### A lit torch on the floor lights nothing

`world[room].light` is the room's count of light sources, and everything about
darkness turns on it. What it counts is narrower than the name suggests: it is
adjusted in exactly five places, and every one of them is about a **light worn
in `WEAR_LIGHT` by a character who is in the room**.

```
handler.c:381   char_from_room   -- decrement when the leaver wears one
handler.c:403   char_to_room     -- increment when the arriver does
handler.c:539   equip_char       -- increment when one goes into the slot
handler.c:573   unequip_char     -- decrement when it comes out
handler.c:832   update_char_objects -- decrement when it burns out
```

`obj_to_room` and `obj_from_room` (handler.c:681, :698) do not touch it at all.
So a burning torch dropped on the floor contributes nothing, and **putting your
torch down plunges the room into darkness while it goes on burning at your
feet**. Carrying one in your pack is no better: the count is of the slot, not
of the object.

*Source*: `utils.c:640`, `handler.c:381`.

### -1 hours of fuel is an eternal light, not a dead one

The test for "this light is on" is `GET_OBJ_VAL(obj, 2)` — non-zero, not
positive. The burnout timer is guarded separately on `> 0`
(handler.c:823). So value 2 of -1 is a light that is on and can never go out,
0 is a spent one, and the two readings have to agree or eternal lights either
stop working or burn down.

*Source*: `handler.c:823`.

### Dusk is dark, and dawn is not

`room_is_dark` treats `SUN_SET` as dark along with `SUN_DARK`, but not
`SUN_RISE`. Since `weather_and_time` sets `SUN_RISE` at hour 5 and `SUN_SET` at
hour 20, an outdoor room is lit from 05:00 and dark from 20:00 — the light
arrives an hour before full daylight and leaves an hour before full night.

*Source*: `utils.c:649`.

### Two different questions about seeing in the dark

There are two macros and they are not the same, which is easy to miss because
they read alike:

```c
#define CAN_SEE_IN_DARK(ch) \
   (AFF_FLAGGED(ch, AFF_INFRAVISION) || (!IS_NPC(ch) && PRF_FLAGGED(ch, PRF_HOLYLIGHT)))

#define LIGHT_OK(sub)	(!AFF_FLAGGED(sub, AFF_BLIND) && \
   (IS_LIGHT(IN_ROOM(sub)) || AFF_FLAGGED((sub), AFF_INFRAVISION)))
```

`CAN_SEE_IN_DARK` takes holylight; `LIGHT_OK`, which is what `CAN_SEE` is built
from, does not — holylight is let in one level up, at `IMM_CAN_SEE`, where it
bypasses the whole test rather than answering it. `look_at_room` asks the
first; deciding whether you can see a *person* asks the second. Using either
one for both jobs is wrong in a way that only shows up for a blind god.

*Source*: `utils.h:348`, `utils.h:426`.

### `do_look` and `look_at_room` disagree about being blind in the dark

Both check darkness and blindness before showing you anything, and they do not
agree on the order or on the words:

```c
/* do_look, act.informative.c:662 */
if (GET_POS(ch) < POS_SLEEPING)      "You can't see anything but stars!"
else if (AFF_FLAGGED(ch, AFF_BLIND)) "You can't see a damned thing, you're blind!"
else if (IS_DARK && !CAN_SEE_IN_DARK) "It is pitch black..." + list_char_to_char

/* look_at_room, act.informative.c:418 */
if (IS_DARK && !CAN_SEE_IN_DARK)     "It is pitch black..."
else if (AFF_FLAGGED(ch, AFF_BLIND)) "You see nothing but infinite darkness..."
```

Blindness first in one, darkness first in the other, and two different
sentences for being blind. Both are reachable: a blind character who *types*
`look` is told they are blind, and the same character who *walks into* a dark
room is told it is pitch black. Nothing suggests this was intended; it is two
people writing the same guard twice.

And the first branch of `do_look` is a **fifth unreachable message**, of the
family in "Four 'you have to wake up first' messages nobody has ever seen":
`look` and `read` are both `POS_RESTING` in the command table
(interpreter.c:355, :427), so `GET_POS(ch) < POS_SLEEPING` cannot hold by the
time `do_look` runs. Nobody has seen "You can't see anything but stars!"
either.

*Source*: `act.informative.c:662`, `act.informative.c:418`.

### The glowing red eyes are reachable from exactly one place

`list_char_to_char` has an `else if` for somebody you cannot see:

```c
else if (IS_DARK(IN_ROOM(ch)) && !CAN_SEE_IN_DARK(ch) &&
         AFF_FLAGGED(i, AFF_INFRAVISION))
  send_to_char("You see a pair of glowing red eyes looking your way.\r\n", ch);
```

Note whose infravision it is: **`i`'s, not `ch`'s** — the creature's own night
vision is what gives it away, not yours.

It looks dead, because the only two callers of `list_char_to_char` are
`look_at_room`, which returns before its listing when the room is dark, and
`do_look` — which prints "It is pitch black..." and then calls it *anyway*
(act.informative.c:668). The C's whole comment on that line is `/* glowing red
eyes */`, and without it there is nothing to suggest the second branch is ever
taken. So the eyes are visible when you **type `look`** in a dark room and not
when you walk into one.

*Source*: `act.informative.c:351`, `act.informative.c:668`.

### `who` shows nobody at all if you are standing in the dark

`do_who` filters on `CAN_SEE(ch, tch)` (act.informative.c:1086), `CAN_SEE` is
built on `LIGHT_OK`, and `LIGHT_OK` asks whether **the viewer's own room** is
lit. The people on the who-list are all over the world and it makes no
difference: standing in an unlit room, a mortal with no infravision sees an
empty who-list and a count of zero.

The same applies to `where`. It is invisible in Midgaard, where every room is
`SECT_INSIDE` or `SECT_CITY` and therefore always lit, and it is startling the
first time somebody types `who` in a cave.

*Source*: `utils.h:426`, `act.informative.c:1086`.

### An implementor cannot see in the dark

`PRF_HOLYLIGHT` is set by `advance_level` (class.c:1920). The first character
on an empty roster is made level 34 by `init_char` and therefore never runs
`do_start`, which is the only thing that would call `advance_level` — so **the
founding implementor has no holylight** and stands in a dark room reading "It
is pitch black..." like anybody else, until they type `holylight`.

Every god who got there by being *advanced* has it. Only the first one does
not, and only because of the shortcut that made them.

*Source*: `class.c:1920`, `db.c:2705`.

## Objects, containers and equipment

### Armour class and applies are two different mechanisms

An `ITEM_ARMOR`'s value 0 is subtracted from the wearer's armour class by
`equip_char` and added back by `unequip_char` — a lasting change to the
character. The `A` lines on the same object are applies, and `affect_total`
recomputes those from scratch. Nothing marks the difference in the file
format; it is the object's *type* that decides which mechanism value 0 goes
through.

The multiplier on the armour half belongs to the slot, not the object:

```c
case WEAR_BODY: factor = 3; break;   /* 30% */
case WEAR_HEAD: factor = 2; break;   /* 20% */
case WEAR_LEGS: factor = 2; break;   /* 20% */
default:        factor = 1; break;   /* all others 10% */
```

The comments say percentages because they were, before the whole armour-class
scale was multiplied by ten and the divide-by-ten moved into
`compute_armor_class`'s caller.

*Reproduced.* The recompute is from stored real values rather than the C's
subtract-then-add pass, which is the one place this port deliberately does not
reproduce the *method* — the C's version drifts if anything changes between
the two passes.

### A drink container's weight is not its weight

The loader adds the volume of liquid to the object's weight at load time, so a
canteen declared as weighing 20 and holding 80 units arrives weighing 85. The
world files were authored against this behaviour, so the declared numbers look
wrong on their own.

*Reproduced*, and reported by `dlctl lint --type=world` as a note rather than a
warning, since it is intended.

### A container's capacity includes the container

A container's weight field is maintained cumulatively: `obj_to_obj` walks up
the chain adding the new object's weight to every container above it, so
`GET_OBJ_WEIGHT(bag)` is the bag *plus* everything in it. `perform_put` then
tests

```c
if (GET_OBJ_WEIGHT(cont) + GET_OBJ_WEIGHT(obj) > GET_OBJ_VAL(cont, 0))
```

— so the bag's own weight is charged against the bag's own capacity. A bag
declared as weighing 5 with a capacity of 100 holds 95, and a heavy chest
holds far less than its number says. Builders compensated by inflating the
capacity, which means correcting the arithmetic would silently make every
container in the world bigger.

*Reproduced.* `Object.TotalWeight` is recursive rather than cumulative, which
gives the same answer without the C's habit of leaving a container's weight
wrong after a botched `obj_from_obj`.

### Which checks apply depends on where the object was

`perform_get_from_container` reads:

```c
if (mode == FIND_OBJ_INV || can_take_obj(ch, obj)) {
```

`mode` is where the *container* was found. Take something out of a bag on the
floor and you are checked against the take flag, the item count and your
carrying weight; take the same thing out of the same bag while holding it and
only the item count is checked. So a no-take object can be carried anywhere in
a bag, and a bag can hold more weight than its owner could ever lift — which
is the arithmetic being consistent, since the weight was already theirs, and
also how you carry an anvil.

*Reproduced*, including the asymmetry.

### Pouring one container into another overshoots and corrects

`do_pour` fills the destination to its capacity outright, subtracts that much
from the source, and then checks whether the source has gone **negative** —
which is how it finds out there was not enough — and adds the shortfall back
to both. It arrives at the right answer by overshooting.

The same function also prints `"You pour the %s into the %s."` with **no
newline**, and names the destination with the word the player typed rather
than the object's own name. Both reproduced: the prompt runs on, exactly as it
did in 2001.

### A pile of coins never says how many

`create_money` names a pile from `money_desc`'s fourteen-entry table, so 100
coins and 200 are both "a small pile of gold coins". The count is revealed
only by picking it up, when `get_check_money` destroys the object and prints
"There were 150 coins." The thresholds are 1, 10, 20, 75, 200, 1000, 5000,
10000, 20000, 75000, 150000, 250000, 500000, 1000000, and then a phrase for
anything larger — which exists because somebody found out you could carry more
than a million.

*Verified* against the table in `handler.c`, both sides of every boundary.

---

## The reference tree itself

### The patched C server cannot boot on the data this repo ships

`boards.c:67-72` declares six bulletin boards, and two of them — 3094
"suggestion" and 3095 "pkill" — are Disgracelands additions whose *objects*
only ever existed in the archived world. `examples/stock/binary/` here is stock CircleMUD 3.0
bpl20, which has 3096 to 3099 and no more.

`init_boards` treats a missing board as fatal (boards.c:126), so the C server
dies the moment an immortal looks at the board room:

    SYSERR: Fatal board error: board vnum 3095 does not exist!

It is not a problem for the world-parity harness, which never boots into the
game — but the session-parity harness does, so the scratch copy it stages has
the two objects synthesised into it (`internal/parity/stage.go`, used by both
`test/parity` and `scripts/session-parity.sh`). `examples/stock/binary/`
itself is untouched.

*Source*: `boards.c:67`, `boards.c:126`.

### A world file out of vnum order is a world file the server cannot read

`real_object` binary-searches `obj_index`, which is built in the order the
records appear in the file. So a record whose vnum is out of order is loaded,
counted, and then invisible to every lookup — no error, no warning, just a
prototype nothing can find.

Discovered the hard way: the two synthetic boards above were first appended to
the end of `obj/30.obj`, after 3099, and produced exactly the same "does not
exist" as not adding them at all.

Worth knowing before anybody hand-edits a world file, and worth knowing that
`dlctl lint --type=world` does not check for it.

*Source*: `db.c`'s `real_object`.

## Naming things

### `do_users` hides nobody, because it asks the wrong character

```c
if (GET_INVIS_LEV(ch) > GET_LEVEL(ch))
  continue;
```

`ch` is the person *typing* `users`, on both sides. It reads as "skip anybody
whose invisibility level is above my own", which is what the surrounding checks
do — but the subject of the loop is `tch`, and this line never mentions it.

As written it can never fire: `set <name> invis` clamps the level to the
character's own (act.wizard.c), so `GET_INVIS_LEV(ch) <= GET_LEVEL(ch)` always
holds. A dead check, one identifier away from working.

It matters less than it looks, because the *listing* line below it does test
`CAN_SEE(ch, d->character)` and that catches invisible players properly. The
dead line would have hidden them from the count as well as the list.

Recorded before `users` is ported, so that whoever does it reproduces the dead
check rather than quietly fixing it — or fixes it deliberately and writes the
deviation down.

*Source*: `act.informative.c:1316`.

### `isname` is a whole-word match, and reads like a prefix one

```c
for (curstr = str;; curstr++, curname++) {
  if (!*curstr && !isalpha(*curname))
    return (1);
  ...
  if (!*curstr || *curname == ' ')
    break;
```

The loop walks a keyword character by character against the typed word and
breaks when the word runs out, which is exactly the shape of a prefix match. It
is not one: the *only* return of 1 needs `!*curstr` **and** the keyword
character underneath to be non-alphabetic — that is, both strings to have ended
together. `swo` against `sword long` reaches the break, skips to `long`, fails
there too, and returns 0.

So **`get swo` does not pick up a sword**, `kill dra` finds no dragon, and
`kill zo` finds no Zod. Every search in the game goes through this one
function, for objects and mobiles and players alike, because `player.name` is a
keyword list for a mobile and a plain name for a player.

This port had it as a prefix match for four phases, with a comment in
`carry.go` saying *"The C matches a prefix of any keyword, which is why `get
swo` picks up a sword"*, and a test asserting it. All three were wrong, and
`reference/tools/nameoracle.c` is what settled it — 168 pairings, compared
rather than read.

**And "the keyword has ended" means a non-*letter*, not whitespace — which
this port then got wrong a second time, for a year, behind that same
oracle.** `!isalpha(*curname)` is false for a digit, so a digit ends a
keyword: the C matches `6` and `60` against a keyword of `606`, and `a`
against `a1b`. The second version of `matchesKeywords` was `strings.Fields`
plus equality, and **every namelist in the oracle's own sweep was made only
of letters and spaces** — over which the two rules cannot disagree. 168
pairings agreed with a C they were not testing.

That is worth more than the fix. An oracle is only as good as the inputs it
sweeps, and a corpus of `"sword long"`, `"dragon fractal puff"` and
`"guard cityguard"` is a corpus with the hard case designed out of it — the
same shape as the transcoding gap that sat inert against every fixture in
the repo (`docs/design/data-format.md` §11.1) and as the fixtures
`docs/design/yaml-only.md` §5.1 was written about. It is reachable in the
shipped data, not theoretical: stock CircleMUD's newbie zone has an extra
description keyed `staircase stair 606 rs`, so on the real server `look 6`
matches it and here it did not.

Fixed 2026-08-29 (#277) by transliterating the C's two loops rather than
restating them — the inner loop breaks on a literal space and the outer one
skips a run of *alphabetic* bytes, so where a match may begin depends on
where the previous attempt gave up, which is not a thing to summarise in a
sentence — and by widening the sweep to digits, punctuation, an apostrophe,
a hyphen, doubled and trailing spaces and a namelist wrapped across lines by
`fread_string`. 1,456 pairings now, and the test fails if the sweep ever
narrows back to letters and spaces.

*Source*: `handler.c:56`.

### `find_skill_num` answers an empty spell name, and `cast '  '` reaches it

Two functions conspiring, neither of them obviously wrong on its own.

**`find_skill_num` returns the first spell for an empty name.** Its first
rule is `is_abbrev(name, ...)`, which rejects an empty `arg1`; its second
walks both strings a word at a time and does not run at all when the typed
string has no words, leaving `ok` TRUE and `first2` empty — so
`ok && !*first2` holds on the very first table entry. `find_skill_num("")`
is 1, which is armor.

**`do_cast` hands it whitespace.** The spell name comes from
`strtok(argument, "'")` and then `strtok(NULL, "'")`, and two properties of
strtok decide what arrives: it skips a **run** of delimiters rather than
one, and **only the quote is a delimiter** — a space is not. So:

```
cast ''             -> "Spell names must be enclosed..."   the run collapses
cast '  '           -> find_skill_num("  ")   ->  armor
cast ' '            -> find_skill_num(" ")    ->  armor
cast '' fido        -> find_skill_num(" fido")   the target becomes the spell
cast 'magic missile -> works; the closing quote is optional
```

`any_one_arg` then tokenises `"  "` away before either rule looks at it, so
find_skill_num cannot tell it from `""`. **`cast '  '` casts armor**, at
level one, for free.

This port refused the empty name in `SpellNumberByName` and found the quotes
by index in `ParseCastArgument`, which got all five of those lines wrong
(#358, #365). Both are now *reproduced*, and the refusal moved rather than
being deleted: `SpellNumberFromNameOrNumber` — the **format** lookup, which
the yaml readers call for a player's skills, a wand's charge spell and a
damage message's subject — refuses an empty name itself, so a blank
`spell:` in a file stays an error instead of silently becoming armor.

That split is the part worth keeping. One "what does this name mean"
function serves both a player's typing and a file's contents, and the right
answer differs: be the C where somebody is typing, refuse where something is
being parsed.

It is also the **opposite** call to the one made for `isname`'s own
empty-string case a few entries above, where this port refuses and the C
matches anything. The difference is reachability. Every `isname` caller
checks for an empty argument first, so refusing there costs nothing;
`find_skill_num`'s callers do not all check — `SPECIAL(guild)` and
`do_skillset` guard, `do_cast` does not.

Verified against `reference/tools/skilloracle.c` (1,745 queries) and
`reference/tools/castoracle.c` (41 typed lines, do_cast's three strtok
calls over every shape of quoting a player can type). Neither corpus could
express the case before this: the skill sweep had no empty or whitespace
query in it, and the cast sweep excluded tabs outright because the oracles
echoed the query unescaped. Both escape now, and both ask.

The earlier reasoning — that strtok skipping leading delimiters made an
empty name unreachable — is right about `cast ''` and wrong about
`cast '  '`, and it was reached by reading the function rather than running
it. It was then written into a comment twice, in opposite directions, before
anybody compiled the thing.

*Source*: `spell_parser.c`'s `find_skill_num` and `do_cast` (:604),
`interpreter.c:1057`'s `is_abbrev`. See
`docs/investigations/partial-matching.md` §4.5.

### `get_number` rewrites the argument before it decides the prefix was a number

```c
if ((ppos = strchr(*name, '.')) != NULL) {
  *ppos++ = '\0';
  strcpy(number, *name);
  strcpy(*name, ppos);          /* <- the caller's buffer, already rewritten */

  for (i = 0; *(number + i); i++)
    if (!isdigit(*(number + i)))
      return (0);               /* <- and only now is it rejected */
```

So `foo.sword` returns 0 *and leaves "sword" behind*. What that means depends
on who asked: `get_char_room_vis` reads 0 as **"a player with this name"**
(handler.c:1068) and searches the player list; every object search reads it as
**"give up"** and returns NULL immediately. One value, two meanings.

Three more, all `atoi`:

- `007.sword` is the **seventh** sword.
- `2.3.sword` is the second `3.sword` — only the first dot is consumed, and
  the rest is handed on with the dot still in it.
- `-1.sword` is 0, because `-` is not a digit, so it becomes a player search
  for somebody called "sword".

And the counter is a **pointer shared down a chain of calls**: `get_obj_vis`
passes the same `number` to the inventory search, then the room, then the
world. `2.sword` is therefore the second sword across the whole search *order*,
not the second in whichever list it ends up looking at.

*Source*: `handler.c:590`, `handler.c:1148`.

### `qui` and `shutdow` are commands, and they exist to refuse

```c
{ "qui"      , POS_DEAD    , do_quit     , 0, 0 },
{ "quit"     , POS_DEAD    , do_quit     , 0, SCMD_QUIT },
{ "shutdow"  , POS_DEAD    , do_shutdown , LVL_GRGOD, 0 },
{ "shutdown" , POS_DEAD    , do_shutdown , LVL_GRGOD, SCMD_SHUTDOWN },
```

Two rows each, the same function, and the shorter spelling passes no
subcommand — so it lands in the same body and takes the branch that says *"You
have to type quit--no less, to quit!"* or *"If you want to shut something down,
say so!"*.

They are there because the interpreter matches on prefixes. Without a stump at
the earlier line, `q` would leave the game and `shutdow` would stop the server.
With one, every abbreviation reaches the refusal instead, and only the word
typed in full does anything. It is a guard built out of the table's ordering
rather than out of a check, and it is invisible unless you notice that two rows
name the same function.

An immortal is exempt from the `quit` half — `subcmd != SCMD_QUIT &&
GET_LEVEL(ch) < LVL_IMMORT` — so a god may type `qui` and leave.

*Source*: `interpreter.c:421`, `act.other.c:143`.

### Undoing a remort the character never had *grants* it

```c
if (undo == 1)
        new_vector = old_vector ^ (int)pc_class_remort_masks[i];
else
        new_vector = old_vector | mask;
```

XOR, not AND-NOT. The guard above it refuses a grant when the bit is already
set, but it explicitly allows an undo when it is *not* — so `remort bob
-cleric` on a character who has never been a cleric turns the bit on and tells
them their clerichood has slipped away.

The XOR is still there, and **is no longer reachable through the command**.
`remort` grew a guard with #262 (`docs/deviations.md`, "`remort` changes the
class, resets the level and rebuilds the body"): an undo of a class the
character has never had is refused before it gets this far, so the only way
into the XOR now is with the bit already set, where it does what AND-NOT
would. The line is kept rather than rewritten because the arithmetic is the
C's and this file is the record of it; what changed is which arguments can
reach it.

*Source*: `act.wizard.c:437`.

### `remort` has two spellings of the same failure

The name-not-found message appears twice, one full stop apart: *"...they are
not logged in."* for the report form and *"...they are not logged in.."* for
the grant form. Both are in the C, four lines apart, and both are reproduced.

*Source*: `act.wizard.c:373`, `act.wizard.c:394`.

## Storage and limits

### Flags are `unsigned long`, and the letter encoding breaks at bit 31

`asciiflag_conv` computes `1 << (26 + (c - 'A'))` into an `int`. Bit 31 is the
sign bit and anything above it is undefined behaviour, so `'F'` is the last
letter the C server handles and everything from `'G'` on is broken *there*.
Data using those bits cannot round-trip to the C server whatever a port does.

Separately, `bitvector_t` is `unsigned long` — 32 bits on the platform this
was written for, 64 on modern Linux. The width silently changed under the
codebase somewhere around 1998 and nothing noticed because nothing used the
high bits.

### Passwords are compared over ten characters and hashed over eight

`MAX_PWD_LENGTH` is 10, which is the width of the field in `char_file_u`.
Traditional DES `crypt(3)` truncates the password to **eight** characters
before hashing. So the ninth and tenth characters of a password affect nothing
at all, and `nanny` refuses passwords longer than ten as though they would.

*Deviation*: no maximum here, and a six-character minimum instead of three.

### Carry capacity uses shifts

```c
#define CAN_CARRY_N(ch) (5 + (GET_DEX(ch) >> 1) + (GET_LEVEL(ch) >> 1))
```

`>> 1` rather than `/ 2`, which is the same for the non-negative values this
sees but is the kind of hand-optimisation that stops being equivalent the
moment something goes negative.

### Rent is prorated by a float and then truncated

```c
num_of_days = (float) (time(0) - rent.time) / SECS_PER_REAL_DAY;
cost = rent.net_cost_per_diem * num_of_days;
```

`cost` is an `int`, so the product is truncated toward zero rather than
rounded. A stay of 29 hours at 10 a day costs 12; a stay of six hours costs
nothing at all. The `float` — not `double` — is the C's, and at the sizes
involved it makes no difference, but it is what the archive says.

*Source*: `objsave.c:469`.

### Crash_load's return value is documented and then ignored

```c
/*
 * Return values:
 *  0 - successful load, keep char in rent room.
 *  1 - load failure or load of crash items -- put char in temple.
 *  2 - rented equipment lost (no $)
 */
```

The caller acts on `2` and nothing else (`interpreter.c:1690`).

The *behaviour* the other two describe is real, but it is achieved somewhere
else entirely: `gen_receptionist` sets `GET_LOADROOM(ch)` to the inn just
before it removes you (`objsave.c:1143`), and the entry sequence reads it,
uses it, and then clears it back to `NOWHERE` unless `PLR_LOADROOM` is set
(`interpreter.c:1676`). So renting brings you back to the inn exactly once,
quitting leaves you in the temple, and `Crash_load`'s 0-versus-1 has nothing
to do with it. The port keeps `RentCode.KeepsLoadRoom` available for the same
reason the C keeps the return value: it documents the intent.

Worth knowing because the natural way to write the Go — set the load room on
every save, so people come back where they were — is a different game, and
this port had it that way until the receptionist was written and the
contradiction showed up.

*Source*: `objsave.c:428`, `objsave.c:1143`, `interpreter.c:1676`.

### Shop prices depend on the width of a multiplication

```c
int buy_price(struct obj_data *obj, int shop_nr)
{
  return (GET_OBJ_COST(obj) * SHOP_BUYPROFIT(shop_nr));
}
```

An `int` times a `float`, truncated back to an `int`. 1.15 stored as a
float32 is exactly 1.1499999761581420898437500 — a hair *under* 1.15 — so a
hundred-coin item at a 1.15 markup is 114.99999761581420898, and what happens
next depends on where the product is kept:

| evaluated as | product | price |
| --- | --- | --- |
| `float` (SSE, `FLT_EVAL_METHOD` 0) | rounds to exactly 115.0 | **115** |
| x87 80-bit (`FLT_EVAL_METHOD` 2) | stays 114.999997… | **114** |

The archived server was a 32-bit i386 build, so the second column is what
players actually paid, and the port multiplies at `float64` to match. The
sell prices go the other way for the same reason: 0.15 as a float32 is a hair
*over* 0.15, so a hundred-coin item is still valued at 15.

Two lines of C, no division, no cast that looks suspicious, and the answer
depends on the machine. This one is the argument for the whole oracle
approach in one function.

*Verified*: `reference/tools/shopprice.c` against `BuyPrice`/`SellPrice`,
12,006 price pairs, built `-m32 -mfpmath=387`. CI installs the toolchain for
any change that can reach it.

### `show shops`'s Buy and Sell columns show the wrong number

```c
strcat(buf, " ##   Virtual   Where    Keeper    Buy   Sell   Customers\r\n");
...
sprintf(END_OF(buf2), "%s   %3.2f   %3.2f    ", buf1,
        SHOP_SELLPROFIT(shop_nr), SHOP_BUYPROFIT(shop_nr));
```

The header reads "Buy" then "Sell", but the values plugged in are
`SHOP_SELLPROFIT` first and `SHOP_BUYPROFIT` second — the two swapped
relative to their own column headings. `buy_price` and `sell_price`
(immediately above, in this same file) confirm which is which: a player's
*buy* price is `GET_OBJ_COST(obj) * SHOP_BUYPROFIT(shop_nr)`, so the column
headed "Buy" is showing the *sell* multiplier, and vice versa. The same swap
recurs in `list_detailed_shop`'s "Buy at: […], Sell at: […]" line
(`shop.c:1338-1339`), so it is systematic rather than a one-off typo.

Reproduced rather than fixed, in both places: §0's fidelity rule does not
carve out a display bug just because it is easy to spot once the two
neighbouring functions are read side by side.

*Source*: `shop.c:1227,1236-1237,1338-1339`, `shop.c:474-477,632-635`
(`buy_price`/`sell_price`). Ported as `internal/session/wizshops.go`'s
`listAllShops`/`listDetailedShop`.

### A board post's date has a weekday and no year

```c
sprintf(buf, "%6.10s %-12s :: %s", tmstr, buf2, arg);
```

`tmstr` is `asctime`'s output — "Thu Aug 20 01:23:45 2026". The `.10`
precision truncates it to the first ten characters, "Thu Aug 20", and the `6`
width does nothing at all because what is left is longer than six. So every
message on every board is dated by weekday and month-day, the year is thrown
away, and one of the two numbers in the format is dead.

*Source*: `boards.c:219`.

### A live pointer is written into every board file

`struct board_msginfo` has a `char *heading` as its second member, and
`Board_save_board` fwrites the whole struct. The address is meaningless the
moment the process exits and `Board_load_board` reads it and ignores it — but
its *width* decides where the three fields after it sit. Four bytes on the
i386 build the archive came from; eight on any 64-bit rebuild, which reads the
poster's level out of the pointer's second half.

*Verified*: `reference/tools/boardlayout.c` against the offsets in
`internal/persist/boards`, built `-m32`.

*Source*: `boards.h:19`, `boards.c:416`.

### A mail block is 100 bytes on every machine and means something different on each

```c
#define HEADER_BLOCK_DATASIZE \
  (BLOCK_SIZE - sizeof(long) - sizeof(struct header_data_type) - sizeof(char))
```

100 - 4 - 16 - 1 = **79** characters of message in a header block on the i386
build the archive came from. On a 64-bit rebuild it is 100 - 8 - 32 - 1 =
**59**. Both still produce hundred-byte blocks, so a file written by one and
read by the other lines up perfectly, the block chain resolves, every message
comes back the right length — and the text has twenty characters of the wrong
thing every hundred. There is no magic number, no length field and no
checksum anywhere in the format to catch it.

*Verified*: `reference/tools/maillayout.c` against
`internal/persist/mail`, built `-m32`.

*Source*: `mail.h:71`.

### A mail block points at the next one by byte offset, in a field that looks like an index

```c
void push_free_list(long pos)   /* #1 - What byte offset into the file the block resides. */
...
    data.block_type = target_address;       /* mail.c:375 */
    write_to_file(&data, BLOCK_SIZE, last_address);
```

A message longer than one block is a chain, joined through `next_block` in
the header and through `block_type` itself in each data block — "this works
much like DOS' FAT", as the comment above it says. What that field holds is
a **byte offset into the file**: `target_address` comes from
`pop_free_list`, and `scan_file` indexes with `block_num * BLOCK_SIZE`
(`mail.c:260`). So the second block of a message is 100, the third 200, and
a chain in a file that has had mail deleted from it can run *backwards* —
the free list is a stack, so blocks come back in reverse order.

Nothing in the format says so. The field is a `long` beside three other
`long`s, the values are small, and reading it as a block number gives 1, 2,
3 for exactly the case a fresh file produces — a hand-written test file, for
instance. A port that makes that assumption is perfectly self-consistent and
cannot read a single multi-block message the C ever wrote: the link is a
hundred times too large, fails a bounds check, and the chain stops at the
header, leaving the message truncated to 79 characters with no error
anywhere. `write_to_file` refuses a position that is not a whole number of
blocks (`mail.c:162`), which is the check that gives the units away if
anything does.

*Verified*: `reference/tools/mailoracle.c` — `store_mail`'s own body, run to
write a real file — against `internal/persist/mail/classic`, built `-m32`.

*Source*: `mail.c:76`, `mail.c:346`, `mail.c:375`.

### Which piece of mail you get next depends on how long the server has been up

`index_mail` pushes each new message onto the *front* of a per-player list
(`mail.c:233`) and `read_delete` walks to the *end* of that list and takes
from there (`mail.c:436`) — so within one run, mail is delivered oldest
first. But `scan_file` rebuilds that same list at boot by reading the file
from the start and prepending each header it finds, so after a reboot the
tail of the list is the *lowest-numbered block* rather than the oldest
message.

Those agree only while the file is growing. The mail file reuses freed
blocks, so once anybody has collected their post, a new message can land in a
low-numbered block and jump the queue — but only after a reboot. The port
delivers in ascending block order always, which is what the C does after
every restart; the alternative is delivery order that depends on uptime.

*Source*: `mail.c:213`, `mail.c:247`, `mail.c:436`.

### The mail header shouts in lower case

```
  To: recipient
From: sender
```

`get_name_by_id` returns a pointer into the C's player table, and `boot_db`
lowercases every name as it builds that table (`db.c:607`). So the names in a
mail header are always lower case, whatever the character actually calls
themselves. Nobody fixed it in seven years.

*Source*: `db.c:607`, `mail.c:461`.

### A pickpocket can take at most 1782 coins

```c
gold = (GET_GOLD(vict) * number(1, 10)) / 100;
gold = MIN(1782, gold);
```

A tenth of what the victim is carrying, at most — and then capped at 1782.
Not 1000, not 1500, not 2000. Nobody has ever explained it, and it has been
in DikuMUD since before CircleMUD forked.

*Source*: `act.other.c:1103`.

### The track roll cannot be beaten

```c
if (number(0, 101) >= GET_SKILL(ch, SKILL_TRACK))
```

`number(0, 101)` is 102 possible values against a skill capped at 100, and
the comparison is `>=`. So a perfect tracker still fails two rolls in every
hundred and two — and gets sent a *random direction* rather than told
nothing, because the failure branch picks one of the six at random and
reports it as a trail.

*Source*: `graph.c:191`.

### Abbreviations mean different things to mortals and to gods

```c
for (length = strlen(arg), cmd = 0; *cmd_info[cmd].command != '\n'; cmd++)
  if (!strncmp(cmd_info[cmd].command, arg, length))
    if (GET_LEVEL(ch) >= cmd_info[cmd].minimum_level)
      break;
```

The level check is *inside* the matching loop. A command above your level is
not refused — it is skipped, and matching carries on down the table. So an
immortal command sitting earlier than a mortal one shadows it for the people
who can use it and does not exist at all for everybody else.

The example that made this visible: `goto` is at line 313 and `gold` at 314,
so **an immortal typing `go` counts their money never again.** And a mortal
typing `goto` is told "Huh?!?" — the same answer as for a word that means
nothing, so the command's existence is never given away.

*Source*: `interpreter.c:623`.

### Four "you have to wake up first" messages nobody has ever seen

The interpreter refuses a command whose `minimum_position` is above yours
before the command runs, so a command's *own* position check is dead code
whenever it is testing the same thing or less.

Four of them are:

```c
{ "rest"     , POS_RESTING , do_rest     , 0, 0 },   /* interpreter.c:426 */
{ "sit"      , POS_RESTING , do_sit      , 0, 0 },   /* :468 */
{ "stand"    , POS_RESTING , do_stand    , 0, 0 },   /* :490 */
{ "flee"     , POS_FIGHTING, do_flee     , 1, 0 },   /* :297 */
```

`do_stand`, `do_sit` and `do_rest` each have a `case POS_SLEEPING:` arm saying
*"You have to wake up first!"* — but POS_SLEEPING is 4 and POS_RESTING is 5,
so a sleeping character is stopped by the interpreter with *"In your dreams, or
what?"* and never arrives. `do_flee` opens with `if (GET_POS(ch) <
POS_FIGHTING)`, which is the *identical* comparison the interpreter has already
made, so its *"You are in pretty bad shape, unable to flee!"* is unreachable
too. Nothing else in the tree calls any of the four.

This is worth an entry because of the shape of the mistake it produces rather
than the messages themselves. A port that implements `do_stand` faithfully and
does not implement the interpreter's gate shows players a message the real
server never showed — and every test written against that port agrees with it,
because the function *is* a correct port of the function. Four such
expectations were in this suite until minimum position was enforced, and all
four had been read straight out of the C.

The general form: **a command's own position check tells you what the
interpreter already refused, not what a player sees.** Read `cmd_info[]`'s
second column first.

*Source*: `interpreter.c:636`, `act.movement.c:555`, `act.offensive.c:304`.

### `mute` is called SCMD_SQUELCH

```c
{ "mute"     , POS_DEAD    , do_wizutil  , LVL_GOD, SCMD_SQUELCH },
```

The subcommand, the flag (`PLR_NOSHOUT`) and the message ("Squelch ON for
%s") all say squelch; the word a god types is `mute`. Somebody renamed the
command and left everything behind it alone.

Worth an entry only because the command-line test caught it: the port was
written as `squelch`, which is what every other name in that code path says,
and the check against `interpreter.c` refused it.

*Source*: `interpreter.c:371`.

### Demoting somebody costs them more than levels

```c
if (newlevel < GET_LEVEL(victim)) {
  do_start(victim);
  GET_LEVEL(victim) = newlevel;
```

`do_start` is the routine that sets a *brand new* character up: it rolls the
starting points, resets the skills and applies level one's gains. `advance`
runs it first and then stamps the new level on top — so being demoted from 30
to 29 gives you a level-one character's hit points with a 29 written on it.
Going *up* does nothing of the kind; it just hands the experience over.

*Source*: `act.wizard.c:1472`.

### A lesser god typing `force all` looks for somebody called "all"

```c
else if ((GET_LEVEL(ch) < LVL_GRGOD) || (str_cmp("all", arg) && str_cmp("room", arg))) {
  if (!(vict = get_char_vis(ch, arg, NULL, FIND_CHAR_WORLD)))
```

The level test and the keyword test are folded into one condition, so failing
*either* sends you down the single-victim branch. A god below LVL_GRGOD
typing `force all smile` is not refused — the game looks for a character
named "all", does not find one, and answers "No-one by that name here."

*Source*: `act.wizard.c:1815`.

### `wiznet *32 waves` is an emote *and* a level restriction

```c
case '*':
  emote = TRUE;
case '#':
```

No `break`. The `*` case falls through into `#`, so a line beginning with `*`
is checked for a leading level number exactly as `#` is. `wiznet *waves` is a
plain emote; `wiznet *32 waves` is an emote heard only at level 32 and above.
Nothing documents it and the usage message does not mention it.

*Source*: `act.wizard.c:1882`.

### A switched god cannot type `return`... unless the body is high enough level

The level used to match a command is the level of the *body you are in*
(`interpreter.c:623`), and `switch` puts a god inside somebody else. So a god
switched into a level-one dog is, for the interpreter's purposes, a
level-one dog: every immortal command becomes invisible and answers
"Huh?!?".

`return` works because it is deliberately level 0 — it is the one command in
the file with no minimum, and it has to be, or a god who switched into a rat
would be a rat until the server restarted.

The C's message for this case — "You can't use immortal commands while
switched" — is checked *after* a command has been found, so it only ever
fires when the borrowed body is itself high enough level to have matched the
command in the first place. Most switched gods never see it.

*Source*: `interpreter.c:623`, `interpreter.c:634`, `act.wizard.c:1206`.

### A site ban is a substring match

```c
if (strstr(hostname, banned_node->site))	/* if hostname is a substring */
```

Not a suffix, not a glob — `strstr`. Banning `example.com` also bans
`notexample.computer`; banning `1` bans a remarkable proportion of the
internet. The comment beside it says "if hostname is a substring", which has
the direction backwards as well.

This is why the ban list was always short and carefully written, and it is
reproduced exactly: a ban list carried across from the archive has to keep
meaning what it meant.

*Source*: `ban.c:96`.

### The ban file is written backwards so that reading it comes out forwards

`_write_one_node` recurses to the tail of the list before printing anything,
so the file is written oldest-first. `load_banned` then pushes each line onto
the *front* of the list as it reads. The two reversals cancel, and the list
survives a restart in the same order — which matters not at all for
behaviour, since `isbanned` takes the worst match whatever the order, and
entirely for the display, which is in list order.

*Source*: `ban.c:103`, `ban.c:63`.

### `set` tells you the number you asked for, not the one it stored

```c
} else if (set_fields[mode].type == NUMBER) {
  value = atoi(val_arg);
  sprintf(output, "%s's %s set to %d.", GET_NAME(vict),
	  set_fields[mode].cmd, value);
}

switch (mode) {
...
case 19:
  GET_GOLD(vict) = RANGE(0, 100000000);
```

The acknowledgement is formatted *before* the switch runs, and `RANGE` clamps
inside it. So `set someone gold -100` reports "set to -100." and stores 0, and
`set someone str 25` on a mortal reports 25 and stores 18. Every one of the
thirty-odd NUMBER fields behaves this way.

The three condition fields are the exception, and only because they re-format
the message after clamping — which reads like somebody noticing the problem
once and not going back for the rest.

*Source*: `act.wizard.c:2467`.

---

## The line editor

Every entry here is *verified* against `reference/tools/editoracle.c`, which
holds `improved_editor_execute`, `parse_action`, `format_text` and
`replace_str` unchanged; `internal/session/editoracle_test.go` compares 805
command-against-buffer cases with it, both what is sent and what the buffer
is left holding. The oracle is built `-O0` on purpose — see
[`deviations.md`](deviations.md) for why, and for the three memory-safety
places this port cannot follow.

### A three-line buffer has a fourth line

Every one of `/d` `/e` `/i` `/l` `/n` opens by walking to a line number the
same way:

```c
i = 1; s = *d->str;
while (s && i < line_low)
  if ((s = strchr(s, '\n')) != NULL) { i++; s++; }
if (s == NULL || i < line_low) { /* out of range */ }
```

`s++` past the buffer's final `'\n'` lands on the terminator, and a pointer
to `""` is not NULL. So line *N+1* of an *N*-line buffer exists, is empty,
and is perfectly editable. On three lines:

- `/d 4` answers **"0 lines deleted."** — `total_len` starts at 1, finds no
  `'\n'` to consume, and comes back down by one.
- `/e 4 text` and `/i 4 text` **append** a fourth line rather than refusing.
- `/l 4` prints the range header, a blank line and "0 lines shown."
- `/n 4` prints **nothing at all**: the tail it would emit is empty, and
  `page_string` sends nothing for an empty string (`modify.c:443-446`).

Line 5 is the first that is out of range.

*Source*: `improved-edit.c:174-183,342-360,390-412`. *Reproduced* —
`lineStart` (`internal/session/editor.go`) and
`TestEditorLineAfterTheLastOneExists`.

### An empty buffer and no buffer are different things

`if (*(d->str))` — the guard `/f` `/i` `/l` and `/n` are dispatched behind
(`improved-edit.c:55,61,70,76`) — is a NULL-*pointer* test, not a
"is there any text" test. `/c` frees the buffer and sets the pointer to NULL;
`/d 1-<last>` writes a `'\0'` at the front and leaves the pointer alone.

So after `/c`, `/l` says "Current buffer empty."; after deleting every line,
the same `/l` prints a blank line and "0 lines shown." Nothing on screen
distinguishes the two states beforehand.

*Source*: `improved-edit.c:41-48,196-208`. *Reproduced* — `editText`'s
`present` field, and `TestEditorEmptyIsNotAbsent`.

### `/n` puts the line number on a line of its own, and prints no footer

```c
sprintf(buf, "%s%4d:\r\n", buf, (i - 1));
strcat(buf, t);
```

`"%4d:\r\n"`, not `"%4d: "`. A numbered listing is twice as tall as the text
it lists:

```
   1:
First line.
   2:
Second line.
```

`PARSE_LIST_NUM` also tallies `total_len` up exactly as `PARSE_LIST_NORM`
does and then never prints it, so `/l` ends with "2 lines shown." and `/n`
does not.

*Source*: `improved-edit.c:325-326`. *Reproduced* — `editorListNumbered`,
and `TestEditorNumberedListing`.

### A `/r` pattern longer than the buffer means "not enough space"

```c
} else if ((total_len = ((strlen(t) - strlen(s)) + strlen(*d->str))) <= d->max_str) {
```

`strlen` returns `size_t` and `total_len` is `unsigned int`, so when the
replacement is shorter than the pattern the subtraction wraps. If the
pattern is longer than the entire buffer the sum is genuinely negative, comes
out near `UINT_MAX`, and fails the `<= d->max_str` test. The player is told
**"Not enough space left in buffer."** about a substitution that would have
made the buffer *smaller*, and never finds out the pattern simply was not
there.

The truncation width does not matter: `2^64 - k` narrowed to 32 bits is
`2^32 - k`, so ILP32 and LP64 agree.

*Source*: `improved-edit.c:148`. *Reproduced* — `editorReplace`'s `uint32`
arithmetic, and `TestEditorReplaceUnsignedSpaceCheck`.

### The same arithmetic makes one of `/r`'s three answers unreachable

`replace_str`'s own guard is `(strlen(*string) - strlen(pattern)) +
strlen(replacement) > max_size` — the same three terms as the check above,
in a different order, which modular arithmetic does not care about. So
anything that would make `replace_str` return `-1` has already been answered
"Not enough space left in buffer." by the caller, and **"ERROR: Replacement
string causes buffer overflow, aborted replace."** cannot be printed.

*Source*: `improved-edit.c:148,578-579`. *Reproduced*, in the sense that the
branch is ported and is equally unreachable here.

### A `/ra` that runs out of room truncates the buffer and denies it happened

`replace_str`'s `rep_all` loop measures each segment by writing a `'\0'` into
the caller's buffer and putting the character back afterwards. Its size check
sits between those two steps, and what it does on failure is `break`:

```c
temp = *flow; *flow = '\0';
if ((strlen(replace_buffer) + strlen(jetsam) + strlen(replacement)) > max_size) {
  i = -1;
  break;
}
```

The `'\0'` is never put back, so **the player's buffer is left cut off at the
match that overflowed**. And `i = -1` then meets the function's tail, `if (i
<= 0) return 0;` — which the caller reads as "no matches", so the message is
**"String 'e' not found."** about a string it found seven times before giving
up.

`PARSE_REPLACE`'s own check only ever budgets for a *single* replacement, so
this is exactly the branch that several substitutions at once run into.

*Source*: `improved-edit.c:590-597,613-618`. *Reproduced* — `replaceStr`
returns `(text[:flow], 0)`, and `TestEditorReplaceAllOverflowTruncates`.

### `/fi` indents and `/f i` does not

```c
while (isalpha(string[j]) && j < 2)
  if (string[j++] == 'i' && !indent) { indent = TRUE; flags += FORMAT_INDENT; }
```

`string` is `str + 2`, so for `/fi` it is `"i"` and for `/f i` it is `" i"` —
and a space is not a letter, so the loop ends before the option is seen. The
same scan gives `/r` its `a`: `/ra` replaces every occurrence and `/r a` does
not.

*Source*: `improved-edit.c:124-128,134-136`. *Reproduced* —
`TestEditorFormatOptionScan`.

### `/f` leaves four trailing spaces after a sentence

`format_text` sets `cap_next_next` when it steps off a `.`, `!` or `?`, and
the bottom of its loop turns that into two spaces before the next word. On
the *last* word of the buffer there is no next word, but the flag is only
cleared at the top of the following iteration's `if (*flow)` — which does not
run, because what is left is whitespace. So the two spaces are appended, the
loop goes round once more over the trailing `"\r\n"`, and appends two more.

`/f` on `"One line.\r\n"` produces `"One line.    \r\n"`.

*Source*: `improved-edit.c:558-567`. *Reproduced* — `formatText`.

---

## Movement

### A step's cost is the truncated average of two sectors, so half of them are free-ish

```c
/* move points needed is avg. move loss for src and destination sect type */
need_movement = (movement_loss[SECT(IN_ROOM(ch))] +
                 movement_loss[SECT(EXIT(ch, dir)->to_room)]) / 2;
```

`movement_loss[]` runs 1, 1, 2, 3, 4, 6, 4, 1, 1, 5 for Inside, City, Field,
Forest, Hills, Mountains, Swimming, Unswimmable, Flying, Underwater
(constants.c:768). The comment says "avg", the code says integer division,
and what a player experiences is the difference between the two.

Stepping off a city street into a field costs `(1 + 2) / 2` = **1**, exactly
as much as walking down the street — the half is thrown away. Field to forest
is `(2 + 3) / 2` = 2 and city to mountains is `(1 + 6) / 2` = 3. Because the
average is symmetric, a loop costs the same whichever way round it is walked,
which a rule charging for the destination alone would not do; and because the
cheapest sector costs 1, no step is ever free, which is what makes the number
on the prompt move at all.

The two guards either side of it are also not the same guard. The refusal
(`GET_MOVE(ch) < need_movement && !IS_NPC(ch)`, act.movement.c:130) does not
exempt immortals — the *charge* does (`GET_LEVEL(ch) < LVL_IMMORT`,
act.movement.c:161). An immortal is therefore refusable in principle and
never refused in practice, because they never spend anything.

*Source*: `act.movement.c:127`, `constants.c:768`. *Reproduced* as
`game.MovementCost` (`internal/game/movement.go`), with the table re-parsed
out of the C by `TestMovementLossMatchesTheCSource`.

### `has_boat` exempts gods one level higher than everything around it does

```c
  if (GET_LEVEL(ch) > LVL_IMMORT)
    return (1);
```

Every other level gate in `do_simple_move` reads `< LVL_IMMORT` — the
movement charge (act.movement.c:161), the death trap (act.movement.c:171).
`has_boat`'s is `> LVL_IMMORT`, **strictly greater**, so a newly-made
level-31 immortal is refused deep water and has to find a boat like a
mortal, while a level-32 god crosses it. Nothing marks the difference; the
two comparisons sit forty lines apart and read identically at a glance.

The inventory loop below it has the second surprise:

```c
  /* non-wearable boats in inventory will do it */
  for (obj = ch->carrying; obj; obj = obj->next_content)
    if (GET_OBJ_TYPE(obj) == ITEM_BOAT && (find_eq_pos(ch, obj, NULL) < 0))
      return (1);
```

`find_eq_pos(ch, obj, NULL)` answers the slot an unqualified `wear` would
use, or -1 for something that cannot be worn at all (act.item.c). So a boat
with *any* wear flag in the `find_eq_pos` list is skipped while it is merely
carried — the separate loop over the equipment is what picks it up again once
it is worn. A wearable boat in your backpack leaves you at the water's edge
holding the thing that would float you. Note also that neither loop is
guarded on `IS_NPC`, so a wandering mobile needs a boat too, and that
`AFF_WATERWALK` is checked before either.

*Source*: `act.movement.c:52-79`, `act.item.c` (`find_eq_pos`).
*Reproduced* as `session.hasBoat` (`internal/session/movement.go`), pinned by
`TestAPlainImmortalStillNeedsABoat` and `TestAWearableBoatHasToBeWorn`
(`internal/server/boat_test.go`).

### The same room flag is refused in two different wordings

```c
    send_to_char("You aren't godly enough to use that room!\r\n", ch);   /* act.movement.c:150 */
```

```c
      send_to_char("You are not godly enough to use that room!\r\n", ch); /* do_goto, do_teleport */
```

Three call sites read `ROOM_GODROOM` and there are two strings between them:
walking into one gets the contraction, being sent into one by `goto` or
`teleport` does not. There is no `#define`, so this is a typo that outlived
everybody who might have noticed it — and it is player-visible, which makes
it worth keeping rather than tidying.

The level is the other half of it: `GET_LEVEL(ch) < LVL_GRGOD`, not
`LVL_IMMORT`. A plain immortal is refused a god room exactly as flatly as a
mortal is, which is not what "godroom" suggests and not what the
neighbouring gates do.

*Source*: `act.movement.c:147-151`, `act.wizard.c` (`do_goto`,
`do_teleport`). *Reproduced* in `Context.moveCharacterChecking`, pinned by
`TestTheMovementGodRoomRefusalKeepsItsContraction`
(`internal/server/godroom_test.go`).

### The who-list's colour ignores the player's colour setting

```c
	  switch(prevclasses)
	  {
		case -1:
			  	sprintf(buf,"%s",BBLU);
				break;
```

Every other piece of colour in the game goes through `CCCYN(ch, C_NRM)` and
friends, which expand to the escape *or to nothing* depending on what the
reader has set `color` to (screen.h:37). `do_who`'s per-line colour — a
`<DoC>` local addition that says how many times somebody has remorted by what
colour their line is — writes `BBLU`, `KGRN`, `BGRN`, `KYEL`, `BYEL` and a
trailing `KNRM` directly. There is no macro and no test. A player with
`color off` gets the who-list in colour anyway.

The count itself has a quirk of its own. It adds one for every class bit in
the remort vector and then takes one off again, "Don't count current class,
which will be in the remort vector" — but the subtraction is guarded on
`if (prevclasses > 0)`, so a character with an empty vector stays at zero
rather than going to -1, which is the value the switch uses for an immortal.
Without that guard a brand new character would print in bright blue.

*Source*: `act.informative.c:1108-1161`. *Reproduced* in `session.whoColour`
and `game.RemortCount`.

## Time

### The clock's fallback epoch is a magic number with no explanation

```c
if (beginning_of_time == 0)
  beginning_of_time = 650336715;
```

`reset_time` reads a Unix timestamp from `lib/etc/time` as the point the mud
calendar is measured forward from. If the file is missing, unreadable, or its
first line parses to zero, this literal is used instead — a specific real
moment (11 August 1990, 01:05:15 UTC) with nothing in the source saying what
it commemorates. It is very likely when the original DikuMUD or an ancestor
codebase first booted, but nothing in this tree confirms that, so it is
recorded here as a magic number rather than a fact.

*Source*: `db.c:483-496`. Ported as `clock.DefaultEpoch`
(`internal/persist/clock/clock.go`).

### Saving the clock loses up to an hour, on purpose

```c
time_t mud_time_to_secs(struct time_info_data *now)
{
  time_t when = 0;
  when += now->year  * SECS_PER_MUD_YEAR;
  when += now->month * SECS_PER_MUD_MONTH;
  when += now->day   * SECS_PER_MUD_DAY;
  when += now->hours * SECS_PER_MUD_HOUR;
  return (time(NULL) - when);
}
```

`save_mud_time` does not remember the epoch it originally loaded; it
reconstructs one from the *current* `time_info`'s four integer fields, each
of which already discarded its own remainder when `mud_time_passed` computed
it (`mud_time_passed` truncates seconds into hours, hours into days, and so
on). So every time this runs — at shutdown and every real thirty minutes,
`PULSE_TIMESAVE` — the epoch written is not the one that was read; it drifts
forward by whatever fraction of the current mud-hour had already elapsed,
up to `SECS_PER_MUD_HOUR - 1` (74) real seconds.

The original author knew: `PULSE_TIMESAVE`'s own definition carries the
comment `/* should be >= SECS_PER_MUD_HOUR */` — the interval between saves
is deliberately no finer than the precision a save keeps, so the drift never
compounds into something a player could notice.

*Source*: `utils.c:353-363`, `db.c:534-544`, `structs.h:519`. Ported as
`MudTime.Seconds` (`internal/game/mudtime.go`) and `Live.SavedEpoch`
(`internal/game/live.go`).

---

## `generic_find` cannot see the invisible, except on your own body

`generic_find` (handler.c:1331) is the C's one-call search: it takes a
bitvector of places to look and reports which of them it found the thing in.
Its inventory and room branches both go through `get_obj_in_list_vis`, whose
loop has three nested conditions — the name matches, `CAN_SEE_OBJ`, then
decrement the count:

    for (i = list; i && *number; i = i->next_content)
      if (isname(name, i->name))
        if (CAN_SEE_OBJ(ch, i))
          if (--(*number) == 0)

Its **equipment** branch is one line and has no visibility test at all:

    if (GET_EQ(ch, i) && isname(name, GET_EQ(ch, i)->name) && --number == 0)

So an object a character cannot see is unfindable in their inventory and
findable on their body. Turn a ring invisible and drop detect invisible: it
vanishes from `look ring` while carried, and comes back the moment it is
worn. Nothing in the surrounding code suggests this is deliberate — the two
branches were simply written at different times — and it is reproduced rather
than tidied, per plan §0, because it is reachable and what it changes is what
a player can type.

It also means the count and the visibility filter disagree about the same
object: an invisible carried ring does not consume a number, an invisible
worn one does.

*Source*: `handler.c:1355-1362` (the equipment loop), `handler.c:1124-1145`
(`get_obj_in_list_vis`). Ported as `Search.EquippedObject`
(`internal/game/live.go`), which exists as a method of its own rather than as
`ObjectIn` over a slice for exactly this reason.

---

## `get_number` does not read the count, it removes it

`get_number` (handler.c:590) looks like a parser and is not one. Its job is
to answer "which match did they ask for", and it does that by **rewriting the
caller's string**:

    if ((ppos = strchr(*name, '.')) != NULL) {
      *ppos++ = '\0';
      strcpy(number, *name);
      strcpy(*name, ppos);          /* <- the argument is now "sword" */

Every `FIND_INDIV` branch in `act.item.c` hands one buffer to a `*_vis`
search — which calls `get_number` when it is passed a NULL count — and then
prints that same buffer in the refusal if the search failed. By then the
buffer no longer says what the player typed:

    get 2.sword     ->  "You don't see a sword here."

Six commands are downstream of a search this way and say "sword": `get`,
`drop`, `junk`, `remove`, `wear`, `wield`. Others are not, and say back
whatever was typed. Nothing in the message distinguishes them, which is why
the six were established by playing them at both servers
(`scripts/session-parity.sh`) rather than by reading — a reading would have
had to simulate the buffer's lifetime through two function calls per command,
nine times.

The same rewrite is why `look at 2.plaque` searches extra descriptions for
"plaque" (act.informative.c:604) and why `2.self` is you rather than nobody
(handler.c:1068): both sit after a `get_number` that has already consumed the
prefix.

*Source*: `handler.c:590-604`, `act.item.c:296-299`. Ported as
`game.GetNumber` and `session.namedWithoutCount`, the latter existing purely
to make the "the message sees the stripped buffer" step explicit rather than
implicit.

---

## `number(1, 1)` costs a draw

`number()` (utils.c:38) has no special case for a range with one value in it:

    return ((circle_random() % (to - from + 1)) + from);

`number(1, 1)` therefore takes a value from the generator, reduces it modulo
1, and answers 1. The draw is spent. An implementation that notices the answer
can only be 1 and returns early is *right about the value and wrong about the
generator*, and every subsequent draw on that server is one step ahead of the
C's.

This port had that early return. It cost **288 draws during a single boot of
the stock world** — `read_mobile` rolls `dice(hit, mana)` for every mobile a
zone reset creates (db.c:1824), and 288 of those dice are d1s. The two servers
were 289 values apart before a player had typed anything (the 289th is the
weather; see below), which is why `flee` picked a different exit on each.

**The existing oracle could not see it, and that is the interesting part.**
`reference/tools/randoracle.c` compares *values*: seed a fresh generator, ask
for `number(1, 1)` five hundred times, compare. Both answer 1 five hundred
times. They agree perfectly and are in completely different places. What
disagrees is whatever is asked *next*, which a single-range oracle never asks.
The oracle grew an alternating-range mode for exactly this — interleave the
degenerate range with `number(1, 100)` and the second column diverges on the
first pair.

The lesson generalises past this bug: **an oracle that compares outputs cannot
see a difference in how much state was consumed to produce them.** Anywhere
the C's draw count is part of the behaviour — which is anywhere two servers
have to stay in step — the test has to interleave.

The 289th draw was the weather, and it is why weather.c is ported now:
`reset_time` rolls the barometric pressure at boot (`dice(1, 50)` or
`dice(1, 80)`, db.c) and `weather_change` rolls five more every mud hour —
sometimes six, since the sky's own switch has a conditional `dice(1, 4)` in
four of its cases (`weather.c:88`). Nothing in the game reads the sky except
the four messages it prints. It is ported because it rolls.

*Source*: `utils.c:38-45`, `db.c:1824`. Ported as `rng.Rand.Number`, with
`TestAZeroWidthRangeStillDraws` (`internal/rng/rng_test.go`) as the
interleaved check.

---

## `mudlog`'s level argument is not only a threshold

```c
void mudlog(const char *str, int type, int level, int file)
{
  ...
  if (file)
    log("%s", str);
  if (level < 0)
    return;
```

`mudlog` does two things: writes the line to the log file, and echoes it in
green to every online immortal at or above `level` whose own `syslog`
verbosity is at least `type`. Read as a level threshold, `level` is
straightforward — `LVL_IMMORT` reaches everybody immortal, `LVL_GRGOD`
reaches almost nobody.

There is exactly one call site in the tree that passes something else:

```c
mudlog(buf2, BRF, -1, TRUE);          /* do_skillset */
```

`-1` looks like "even lower than LVL_IMMORT, so everybody sees it". It is
the opposite. The early return above fires first, so **`skillset` writes
its line to the log and shows it to nobody at all**, however high a god has
turned their syslog up — the one command in `do_wizutil`'s neighbourhood
that changes another character permanently, and the one whose line no
immortal ever saw.

Two further things about the same signature, both easy to miss:

- **`file` is not always TRUE.** Four call sites pass FALSE — `bug`/`idea`/
  `typo` (`act.other.c:905`), the auto zone reset (`db.c:1937`), autowiz
  (`limits.c:256`), and one that is `#if 0`'d out (`comm.c:1409`) — so
  those lines are echoed in game and never written down. This port writes
  every one of them to the structured log regardless; see
  [`deviations.md`](deviations.md).
- **The echo has no "except the actor" rule.** A god who freezes somebody
  and is watching syslog sees their own line come back, which is why the
  reply and the log text are so often built from the same `sprintf`.

*Source*: `utils.c:229-258`, `modify.c:344`. Ported as `Server.echoWizVis`
(`internal/server/wizvis.go`), whose own `level < 0` early return is the
first thing it does; `TestSkillsetEchoesToNobody`
(`internal/server/wizvis_test.go`) is the check.

## `nanny` builds the "new player" log line and then overwrites it

```c
    sprintf(buf, "%s [%s] new player.", GET_NAME(d->character), d->host);

        /* <DoC> */
        snprintf(buf, sizeof(buf), "A voice whispers in your ear, 'All hail %s, a newcomer!'", GET_NAME(d->character));
        send_to_all_color(buf, KCYN);
        ...
    mudlog(buf, NRM, LVL_IMMORT, TRUE);
```

`CON_QCLASS` writes the log line into `buf`, and then the local `<DoC>`
block that was inserted between the `sprintf` and the `mudlog` reuses the
same buffer for its broadcast. Twenty-three lines later the `mudlog` fires
against whatever `buf` holds now.

So on the server that was actually played, **"%s [%s] new player." was
never logged and never seen**; what reached the log and the gods' syslogs
was a second copy of the "All hail" line every player in the game had just
been shown in cyan. The host — the only reason the line was worth having —
was gone.

This is a *deviation*: the port logs what the call site was written to log.
Reproducing it would mean deliberately duplicating a broadcast into the
syslog, which is noise rather than behaviour, and no player-visible or
on-disk thing depends on it. [`deviations.md`](deviations.md) records it.

*Source*: `interpreter.c:1606-1629`. Ported in
`Session.handleQueryClass` (`internal/session/login.go`); the broadcast
itself is `Live.AnnounceNewPlayer` (`internal/game/announce.go`), sent from
`Server.Create` where the C sends it.

---

## `gain_exp` and `gain_exp_regardless` are copies that drifted

The two are the same forty lines twice over — the same levelling loop, the
same `is_altered`, the same `mudlog`, the same "You rise a level!" — and
the `<DoC>` broadcast inserted into both did not come out the same:

```c
/* gain_exp, limits.c:311 */
snprintf(buf,sizeof(buf),"A voice whispers in your ear, '%s has gained a level!'\r\n", GET_NAME(ch));
/* gain_exp_regardless, limits.c:368 */
snprintf(buf,sizeof(buf),"A voice whispers in your ear, '%s has gained a level!'", GET_NAME(ch));
```

**One ends in `\r\n` and the other does not**, and the same is true of the
plural pair at `:318` against `:375`. So on the real server a level earned
by killing something printed on a line of its own, and a level handed out
by `advance` — the only caller of `gain_exp_regardless` — ran straight into
whatever the reader saw next.

Nothing about either function suggests the difference; it is two hands
editing two copies. It is reproduced, under the fidelity rule, as two
methods rather than a boolean: `Live.AnnounceLevelGain` and
`Live.AnnounceLevelGainRegardless`, so a call site says which of the C's
two functions it is standing in. `internal/server/announce_test.go` asserts
both ends.

*Source*: `limits.c:306-318` against `:361-375`. Ported in
`internal/game/announce.go`.

---

## `wiznet @`'s "(Writing mail)" can never be printed

```c
	sprintf(buf1 + strlen(buf1), "  %s", GET_NAME(d->character));
	if (PLR_FLAGGED(d->character, PLR_WRITING))
	  strcat(buf1, " (Writing)\r\n");
	else if (PLR_FLAGGED(d->character, PLR_MAILING))
	  strcat(buf1, " (Writing mail)\r\n");
```

`wiznet @` lists the gods on the channel and marks whoever is in the line
editor. Two annotations, and the second is dead code.

`PLR_MAILING` has exactly one setter — `do_mail`'s
`SET_BIT(PLR_FLAGS(ch), PLR_MAILING)` (`mail.c:567`), whose own comment is
`/* string_write() sets writing. */` — and the very next statement calls
`string_write`, which sets `PLR_WRITING` (`modify.c:100-101`). The
editor's cleanup clears both together (`modify.c:218-219`), and a login
clears both again (`interpreter.c:1386`). **So no character ever carries
`PLR_MAILING` without `PLR_WRITING`**, the first arm always wins, and a
god writing a letter is reported as plain "(Writing)".

`do_who`'s own pair tests the same two bits *the other way round* —

```c
      if (PLR_FLAGGED(tch, PLR_MAILING))
	strcat(buf, " (mailing)");
      else if (PLR_FLAGGED(tch, PLR_WRITING))
	strcat(buf, " (writing)");
```

— so there the mail case does win and "(writing)" is the arm that never
fires for a letter. Two sites, opposite orders, each with a dead branch;
neither reads as wrong on its own, which is why this needs both of them
side by side to see.

*Reproduced*: twice, in opposite orders, on purpose. `writingSuffix`
(`internal/session/wizcomm.go`) is `wiznet @`'s if/else-if, and
`whoAnnotations` (`internal/session/commands.go`) is `do_who`'s — each in
its own site's order, each therefore keeping its own dead arm. The
temptation on porting the second one was to make the two agree; that would
have changed what a player sees on the who-list while they wrote a letter,
so the disagreement stands. `TestWhoSaysMailingRatherThanWriting`
(`internal/server/whoannotations_test.go`) pins it.

*Source*: `act.wizard.c:1907-1911`, `act.informative.c:1174-1176`,
`mail.c:567`, `modify.c:100-101,218-219`.

---

## `process_input` keeps five commands and `!` can only find four

```c
    int starting_pos = t->history_pos,
	cnt = (t->history_pos == 0 ? HISTORY_SIZE - 1 : t->history_pos - 1);

    skip_spaces(&commandln);
    for (; cnt != starting_pos; cnt--) {
      if (t->history[cnt] && is_abbrev(commandln, t->history[cnt])) {
	...
	break;
      }
      if (cnt == 0)	/* At top, loop to bottom. */
	cnt = HISTORY_SIZE;
    }
```

`HISTORY_SIZE` is 5 and its comment says "Keep last 5 commands"
(`structs.h:558`), which is true of what is *stored*. It is not true of
what `!<prefix>` can find.

`history_pos` is the slot the next line will be written into, so — once
the buffer has wrapped — it is also the slot holding the **oldest** of the
five. The walk starts at `history_pos - 1` and its condition is `cnt !=
starting_pos` with `starting_pos == history_pos`, so it visits exactly
`HISTORY_SIZE - 1` slots and stops one short. The oldest command is passed
over every single time.

Type five commands and `!` the first of them and you get "Huh?!?", from a
history that still has it. There is no off-by-one to fix at any single
line: `cnt` is initialised correctly, the wrap at the bottom is correct,
and the termination test is the natural one for a circular walk. The
buffer is simply one larger than the walk.

*Reproduced*: `recallHistory` (`internal/session/input.go`) is the same
walk with the same bound, and `TestTheHistoryOnlyEverFindsFourOfItsFive`
(`internal/server/input_test.go`) types five commands and requires the
first to be unreachable. That test was vacuous when first written — it
used the suite's own `settle()` helper, which types `time` and so put two
entries in the history per iteration, pushing the command out of the
buffer entirely and passing for the wrong reason. It is written without it
now, and was checked by mutation: adding a check of the skipped slot makes
it fail.

*Source*: `comm.c:1819-1834`, `structs.h:558`.

---

## "Line too long." is unreachable for a line of ordinary text

```c
    /* The '> 1' reserves room for a '$ => $$' expansion. */
    for (ptr = read_point; (space_left > 1) && (ptr < nl_pos); ptr++) {
      ...
      } else if (isascii(*ptr) && isprint(*ptr)) {
	if ((*(write_point++) = *ptr) == '$') {		/* copy one character */
	  *(write_point++) = '$';	/* if it's a $, double it */
	  space_left -= 2;
	} else
	  space_left--;
      }
    }

    *write_point = '\0';

    if ((space_left <= 0) && (ptr < nl_pos)) {
      char buffer[MAX_INPUT_LENGTH + 64];

      snprintf(buffer, sizeof(buffer), "Line too long.  Truncated to:\r\n%s\r\n", tmp);
```

Two numbers here are not what they look like.

**The limit is 254 characters, not 255.** `space_left` starts at
`MAX_INPUT_LENGTH - 1` = 255 and the loop runs while `space_left > 1`, not
`> 0` — the comment says why, a `$` needs room for two — so it stops with
one byte still unspent and the last character it writes is the 254th.

**And the message almost never fires.** It is gated on `space_left <= 0`,
and an ordinary character decrements by one, so a line of plain text stops
the loop at exactly `space_left == 1` and never reaches zero. The only way
below is the `$` branch, which subtracts two: from `space_left == 2` a `$`
lands on 0, from 1 it would land on -1 — except the loop has already
exited at 1. So a player who typed 300 characters of plain text on the
real server had them silently cut to 254 and was told nothing, and the
message they were meant to get could only ever appear for a line whose
254th character was a dollar sign.

*Reproduced, except for the silence*: `truncateInput`
(`internal/session/input.go`) cuts at the same 254, and this port prints
the message every time. That is a deliberate divergence and
[`deviations.md`](deviations.md) records it — this port has no
`$`-doubling at all, so being faithful here would mean porting a message
that can never appear.

*Source*: `comm.c:1786-1821`, `structs.h:560`.

---

## The guild guard's `-999` is a negative array index

`guild_info[][3]` (`class.c:196`) is a table of "which class may pass this
guild door", and two of its six rows are

```c
  {-999 /* all */ ,	5065,	SCMD_WEST},
  {-999 /* all */ ,     14279,  SCMD_UP},
```

Even in **stock CircleMUD** the comment does not say what it looks like it
says. `guild_guard` blocks when `GET_CLASS(ch) != guild_info[counter][0]`,
and no character's class is -999, so the test is true for everybody and the
guard turns everybody away. `/* all */` means "this row applies to all
classes", not "all classes may pass"; it reads as the second.

**Disgracelands rewrote the test and made it worse.** The local
`guild_guard` (`spec_procs.c:769-802`) checks the remort vector instead, so
a character who was once a thief may still enter the thieves' guild — which
is the good part of the change and `deviations.md` records it. The
expression is

```c
  ((int)GET_REMORT_VECTOR(ch) & (int)pc_class_remort_masks[(int)guild_info[i][0]])
      != pc_class_remort_masks[(int)guild_info[i][0]]
```

and with `guild_info[i][0]` of -999 that indexes `pc_class_remort_masks`
**999 entries before its start**. It is undefined behaviour, it reads
whatever is in the data segment there, and the answer decides whether the
Brass Dragon's guard lets you west.

Whatever the garbage `M` is, the test is `(vector & M) != M`. A mortal's
remort vector is 0 for anybody who has never remorted, so it reduces to
`0 != M` — true, and the guard blocks, unless the garbage happens to be
exactly 0. So the archived server almost certainly turned everyone away at
those two doors, which is the opposite of what a reader of `/* all */`
expects.

*Reproduced as blocking*, with no -999 anywhere: `internal/game/spec.go`'s
`guildInfo` carries a `BlocksEveryone bool` instead. Reading undefined
behaviour is not something a port can reproduce faithfully, so the port
picks the overwhelmingly likely branch and says so here rather than
carrying a negative index it would then have to explain.

*Source*: `class.c:196-212`, `spec_procs.c:769-802`.

---

## What to do about all this

The rule that has worked: **anything with a division, a cast, or a comment
describing numbers gets an oracle rather than a reading.**
`reference/tools/*.c` holds the original function bodies with the `char_data`
dereferences substituted and nothing else changed, and the Go tests compare
against them across the whole input space where that is affordable. Every
entry above marked *verified* is checked by `go test`, so on every CI run —
except the ones needing a 32-bit C build (the shop prices, the ILP32
layouts), which skip without `gcc-multilib` the same way they do on any
64-bit machine and are required not to skip at every release
(`.github/workflows/release.yml`).

It is not that the C is hard to read. It is that the wrong answer looks
exactly like the right one.
