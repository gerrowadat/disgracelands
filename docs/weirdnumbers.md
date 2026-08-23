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

*Reproduced*, with a test named after the surprise so nobody tidies it away.

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

*Reproduced*, and reported by `dlctl world lint` as a note rather than a
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
only ever existed in the archived world. `data/` here is stock CircleMUD 3.0
bpl20, which has 3096 to 3099 and no more.

`init_boards` treats a missing board as fatal (boards.c:126), so the C server
dies the moment an immortal looks at the board room:

    SYSERR: Fatal board error: board vnum 3095 does not exist!

It is not a problem for the world-parity harness, which never boots into the
game — but the session-parity harness does, so `scripts/session-parity.sh`
synthesises the two objects into its scratch copy. `data/` itself is untouched.

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
`dlctl world lint` does not check for it.

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

*Source*: `handler.c:56`.

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
them their clerichood has slipped away. Reproduced.

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

## What to do about all this

The rule that has worked: **anything with a division, a cast, or a comment
describing numbers gets an oracle rather than a reading.**
`reference/tools/*.c` holds the original function bodies with the `char_data`
dereferences substituted and nothing else changed, and the Go tests compare
against them across the whole input space where that is affordable. Every
entry above marked *verified* is checked on every CI run.

It is not that the C is hard to read. It is that the wrong answer looks
exactly like the right one.
