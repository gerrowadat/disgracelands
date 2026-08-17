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

### Backstab multiplies by 20 for immortals

`backstab_mult` returns 2 through 6 across the mortal levels and then **20**
for anyone at or above `LVL_IMMORT`. Not 7, not a continuation of the curve.

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

### A drink container's weight is not its weight

The loader adds the volume of liquid to the object's weight at load time, so a
canteen declared as weighing 20 and holding 80 units arrives weighing 85. The
world files were authored against this behaviour, so the declared numbers look
wrong on their own.

*Reproduced*, and reported by `dlctl world lint` as a note rather than a
warning, since it is intended.

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
