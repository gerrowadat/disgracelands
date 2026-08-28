/* Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
 * of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
 * Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
 * (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
 * and DikuMUD licenses; see LICENSE. Non-commercial use only.
 *
 * startoracle - dump what a character is when they first enter the world.
 *
 * roll_real_abils, do_start and advance_level between them decide every
 * number a level 1 character has, and they do it by drawing from the
 * generator in an order that is not obvious from any one of them: the ability
 * rolls are twenty-four draws whose *order* the sort throws away, a warrior
 * who rolls 18 strength takes a twenty-fifth, and advance_level draws hit
 * points, then **mana**, then movement, for every class -- including the two
 * that then throw the mana away, because a level 1 character gains none.
 *
 * The bodies below are reference/moderncserver/src/class.c's, lifted with the
 * char_data dereferences replaced by the plain values they would have
 * returned and nothing else changed. **moderncserver and not WipeMud-src**:
 * the two trees disagree here, WipeMud's advance_level having no mana draw
 * for a thief or a warrior and its do_start setting max_mana and max_move as
 * well as max_hit. moderncserver is the server that was played
 * (reference/README.md) and is the one this oracle is of.
 *
 *   startoracle <seed> <class> <count>
 *
 * class is 0..4 for magic-user, cleric, thief, warrior, paladin. Prints one
 * line per character, all from the one seeded stream, so a missing or extra
 * draw shows up as everything after it being wrong rather than as a single
 * bad value:
 *
 *   str stradd int wis dex con cha maxhit maxmana maxmove practices nextdraw
 *
 * nextdraw is number(0, 999999) taken straight afterwards, which is what
 * makes a draw-count difference visible at all -- the same argument
 * randoracle's alternating mode is built on.
 *
 * internal/game/startoracle_test.go compiles this and compares.
 *
 *   cc -O2 -o startoracle startoracle.c
 */

#include <stdio.h>
#include <stdlib.h>

#define CLASS_MAGIC_USER 0
#define CLASS_CLERIC     1
#define CLASS_THIEF      2
#define CLASS_WARRIOR    3
#define CLASS_PALADIN    4

#define MIN(a, b) ((a) < (b) ? (a) : (b))
#define MAX(a, b) ((a) > (b) ? (a) : (b))

typedef unsigned char ubyte;

/* --- verbatim from src/random.c --------------------------------------- */

#define m (unsigned long)2147483647
#define q (unsigned long)127773

#define a (unsigned int)16807
#define r (unsigned int)2836

static unsigned long seed;

void circle_srandom(unsigned long initial_seed)
{
    seed = initial_seed;
}

unsigned long circle_random(void)
{
    int lo, hi, test;

    hi = seed / q;
    lo = seed % q;

    test = a * lo - r * hi;

    if (test > 0)
        seed = test;
    else
        seed = test + m;

    return (seed);
}

/* --- verbatim from src/utils.c, minus the SYSERR log ------------------ */

int rand_number(int from, int to)
{
    if (from > to) {
        int tmp = from;
        from = to;
        to = tmp;
    }
    return ((circle_random() % (to - from + 1)) + from);
}

/* --- constants.c:657 and :720, the two columns these functions read --- */

struct con_app_type { int hitp; int shock; };
struct wis_app_type { int bonus; };

const struct con_app_type con_app[] = {
  {-4, 20}, {-3, 25}, {-2, 30}, {-2, 35}, {-1, 40}, {-1, 45},
  {-1, 50}, {0, 55}, {0, 60}, {0, 65}, {0, 70}, {0, 75},
  {0, 80}, {0, 85}, {0, 88}, {1, 90}, {2, 95}, {2, 97},
  {3, 99}, {3, 99}, {4, 99}, {5, 99}, {5, 99}, {5, 99},
  {6, 99}, {6, 99}
};

const struct wis_app_type wis_app[] = {
  {0}, {0}, {0}, {0}, {0}, {0}, {0}, {0}, {0}, {0},
  {0}, {0}, {2}, {2}, {3}, {3}, {3}, {4}, {5}, {6},
  {6}, {6}, {6}, {7}, {7}, {7}
};

/* The character, flattened out of struct char_data. */
struct pc {
    int chclass;
    int level, exp;
    int str, str_add, intel, wis, dex, con, cha;
    int max_hit, max_mana, max_move;
    int hit, mana, move;
    int practices;
    int cond[3];
};

/* --- class.c:1817, with the accessors substituted --------------------- */

void roll_real_abils(struct pc *ch)
{
  int i, j, k, temp;
  ubyte table[6];
  ubyte rolls[4];

  for (i = 0; i < 6; i++)
    table[i] = 0;

  for (i = 0; i < 6; i++) {

    for (j = 0; j < 4; j++)
      rolls[j] = rand_number(1, 6);

    temp = rolls[0] + rolls[1] + rolls[2] + rolls[3] -
      MIN(rolls[0], MIN(rolls[1], MIN(rolls[2], rolls[3])));

    for (k = 0; k < 6; k++)
      if (table[k] < temp) {
	temp ^= table[k];
	table[k] ^= temp;
	temp ^= table[k];
      }
  }

  ch->str_add = 0;

  switch (ch->chclass) {
  case CLASS_MAGIC_USER:
    ch->intel = table[0];
    ch->wis = table[1];
    ch->dex = table[2];
    ch->str = table[3];
    ch->con = table[4];
    ch->cha = table[5];
    break;
  case CLASS_CLERIC:
    ch->wis = table[0];
    ch->intel = table[1];
    ch->str = table[2];
    ch->dex = table[3];
    ch->con = table[4];
    ch->cha = table[5];
    break;
  case CLASS_THIEF:
    ch->dex = table[0];
    ch->str = table[1];
    ch->con = table[2];
    ch->intel = table[3];
    ch->wis = table[4];
    ch->cha = table[5];
    break;
  case CLASS_WARRIOR:
    ch->str = table[0];
    ch->dex = table[1];
    ch->con = table[2];
    ch->wis = table[3];
    ch->intel = table[4];
    ch->cha = table[5];
    if (ch->str == 18)
      ch->str_add = rand_number(0, 100);
    break;
  case CLASS_PALADIN:
    ch->cha = table[0];
    ch->wis = table[1];
    ch->str = table[2];
    ch->con = table[3];
    ch->dex = table[4];
    ch->intel = table[5];
    break;
  }
}

/* --- class.c:1941, with the accessors substituted --------------------- */

void advance_level(struct pc *ch)
{
  int add_hp, add_mana = 0, add_move = 0;

  add_hp = con_app[ch->con].hitp;

  switch (ch->chclass) {

  case CLASS_MAGIC_USER:
    add_hp += rand_number(3, 8);
    add_mana = rand_number(ch->level, (int)(1.5 * ch->level));
    add_mana = MIN(add_mana, 10);
    add_move = rand_number(0, 2);
    break;

  case CLASS_CLERIC:
    add_hp += rand_number(5, 10);
    add_mana = rand_number(ch->level, (int)(1.5 * ch->level));
    add_mana = MIN(add_mana, 10);
    add_move = rand_number(0, 2);
    break;

  case CLASS_THIEF:
    add_hp += rand_number(7, 13);
    add_mana = rand_number(ch->level, (int)(1.5 * ch->level));
    add_mana = MIN(add_mana, 10);
    add_move = rand_number(1, 3);
    break;

  case CLASS_WARRIOR:
    add_hp += rand_number(10, 15);
    add_mana = rand_number(ch->level, (int)(1.5 * ch->level));
    add_mana = MIN(add_mana, 10);
    add_move = rand_number(1, 3);
    break;

  case CLASS_PALADIN:
    add_hp += rand_number(10, 14);
    add_mana = rand_number(ch->level, (int)(1.5 * ch->level));
    add_mana = MIN(add_mana, 10);
    add_move = rand_number(1, 3);
    break;
  }

  ch->max_hit += MAX(1, add_hp);
  ch->max_move += MAX(1, add_move);

  if (ch->level > 1)
    ch->max_mana += add_mana;

  if (ch->chclass == CLASS_MAGIC_USER || ch->chclass == CLASS_CLERIC)
    ch->practices += MAX(2, wis_app[ch->wis].bonus);
  else
    ch->practices += MIN(2, MAX(1, wis_app[ch->wis].bonus));
}

/* --- class.c:1888, with the accessors substituted --------------------- */

void do_start(struct pc *ch)
{
  ch->level = 1;
  ch->exp = 1;

  /* set_title(ch, NULL) -- no draw. */
  roll_real_abils(ch);

  /* max_hit alone. do_start does *not* touch max_mana or max_move --
   * init_char set those, and it is WipeMud-src's do_start, not this one,
   * that resets all three. */
  ch->max_hit = 10;

  /* The class switch sets a thief's skills and draws nothing. */

  advance_level(ch);

  ch->hit = ch->max_hit;
  ch->mana = ch->max_mana;
  ch->move = ch->max_move;

  ch->cond[0] = 24;  /* THIRST */
  ch->cond[1] = 24;  /* FULL */
  ch->cond[2] = 0;   /* DRUNK */
}

/* ---------------------------------------------------------------------- */

int main(int argc, char **argv)
{
    unsigned long initial;
    long count, i;
    int chclass;

    if (argc != 4) {
        fprintf(stderr, "usage: %s <seed> <class 0..4> <count>\n", argv[0]);
        return 2;
    }

    initial = strtoul(argv[1], NULL, 10);
    chclass = atoi(argv[2]);
    count = strtol(argv[3], NULL, 10);

    circle_srandom(initial);

    for (i = 0; i < count; i++) {
        struct pc ch;
        int next;

        ch.chclass = chclass;
        ch.level = 0; ch.exp = 0;
        ch.str = ch.str_add = ch.intel = ch.wis = ch.dex = ch.con = ch.cha = 0;
        ch.max_hit = ch.max_mana = ch.max_move = 0;
        ch.hit = ch.mana = ch.move = 0;
        ch.practices = 0;
        ch.cond[0] = ch.cond[1] = ch.cond[2] = 0;

        do_start(&ch);
        next = rand_number(0, 999999);

        printf("%d %d %d %d %d %d %d %d %d %d %d %d\n",
               ch.str, ch.str_add, ch.intel, ch.wis, ch.dex, ch.con, ch.cha,
               ch.max_hit, ch.max_mana, ch.max_move, ch.practices, next);
    }

    return 0;
}
