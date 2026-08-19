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
