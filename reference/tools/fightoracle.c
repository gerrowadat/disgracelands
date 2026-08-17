/* Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
 * of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
 * Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
 * (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
 * and DikuMUD licenses; see LICENSE. Non-commercial use only.
 *
 * fightoracle - dump the C server's combat arithmetic.
 *
 * compute_thaco has two `int -= double` compound assignments in it, each of
 * which truncates separately; hit() has an integer-division multiplier whose
 * own comment describes different numbers from the ones it produces. Neither
 * is the sort of thing to port by reading it across.
 *
 * The bodies below are fight.c's and class.c's, lifted with the char_data
 * dereferences replaced by the values they would have returned. The str_app
 * and dex_app rows are copied from constants.c.
 *
 *   fightoracle thaco <class> <level>
 *   fightoracle compute <class> <level> <str> <stradd> <hitroll> <int> <wis> <npc>
 *   fightoracle ac <armor> <dex> <awake>
 *   fightoracle multiplier <pos>
 *   fightoracle sweep-thaco
 *
 * internal/game/fight_test.go compiles this and compares.
 */

#include <stdio.h>
#include <string.h>
#include <stdlib.h>

#define CLASS_MAGIC_USER 0
#define CLASS_CLERIC     1
#define CLASS_THIEF      2
#define CLASS_WARRIOR    3
#define CLASS_PALADIN    4

#define POS_FIGHTING 7

#define MAX(a, b) ((a) > (b) ? (a) : (b))

/* --- str_app[].tohit and .todam, from constants.c:520 ----------------- */

static const int str_tohit[31] = {
    -5, -5, -3, -3, -2, -2, -1, -1, 0, 0,
    0, 0, 0, 0, 0, 0, 0, 1, 1, 3,
    3, 4, 4, 5, 6, 7, 1, 2, 2, 2, 3
};

/* --- dex_app[].defensive, from constants.c:600 ------------------------ */

static const int dex_defensive[26] = {
    6, 5, 5, 4, 3, 2, 1, 0, 0, 0,
    0, 0, 0, 0, 0, -1, -2, -3, -4, -4,
    -4, -5, -5, -5, -6, -6
};

/* --- class.c's thaco() ------------------------------------------------ */

int thaco(int class_num, int level)
{
    static const int mage[35] = {
        100, 20, 20, 20, 19, 19, 19, 18, 18, 18, 17, 17, 17, 16, 16, 16,
        15, 15, 15, 14, 14, 14, 13, 13, 13, 12, 12, 12, 11, 11, 11, 10,
        10, 10, 9
    };
    static const int cleric[35] = {
        100, 20, 20, 20, 18, 18, 18, 16, 16, 16, 14, 14, 14, 12, 12, 12,
        10, 10, 10, 8, 8, 8, 6, 6, 6, 4, 4, 4, 2, 2, 2, 1, 1, 1, 1
    };
    static const int thief[35] = {
        100, 20, 20, 19, 19, 18, 18, 17, 17, 16, 16, 15, 15, 14, 14, 13,
        13, 12, 12, 11, 11, 10, 10, 9, 9, 8, 8, 7, 7, 6, 6, 5, 5, 4, 4
    };
    static const int warrior[35] = {
        100, 20, 19, 18, 17, 16, 15, 14, 14, 13, 12, 11, 10, 9, 8, 7,
        6, 5, 4, 3, 2, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1
    };

    if (level < 0 || level > 34)
        return 100;

    switch (class_num) {
    case CLASS_MAGIC_USER: return mage[level];
    case CLASS_CLERIC:     return cleric[level];
    case CLASS_THIEF:      return thief[level];
    case CLASS_WARRIOR:
    case CLASS_PALADIN:    return warrior[level];
    }
    return 100;
}

/* --- fight.c's compute_thaco, with the accessors substituted ---------- */

static int strength_apply_index(int str, int add)
{
    if (add == 0 || str != 18)
        return (str < 0) ? 0 : (str > 25 ? 25 : str);
    if (add <= 50) return 26;
    if (add <= 75) return 27;
    if (add <= 90) return 28;
    if (add <= 99) return 29;
    return 30;
}

int compute_thaco(int is_npc, int chclass, int level, int str, int add,
                  int hitroll, int intel, int wis)
{
    int calc_thaco;

    if (!is_npc)
        calc_thaco = thaco(chclass, level);
    else
        calc_thaco = 20;

    calc_thaco -= str_tohit[strength_apply_index(str, add)];
    calc_thaco -= hitroll;
    calc_thaco -= (intel - 13) / 1.5;   /* Intelligence helps! */
    calc_thaco -= (wis - 13) / 1.5;     /* So does wisdom */

    return calc_thaco;
}

/* --- fight.c's compute_armor_class ------------------------------------ */

int compute_armor_class(int armorclass, int dex, int awake)
{
    if (awake)
        armorclass += dex_defensive[(dex < 0) ? 0 : (dex > 25 ? 25 : dex)] * 10;

    return (MAX(-100, armorclass));
}

/* --- hit()'s position multiplier -------------------------------------- */

int position_multiplier(int pos)
{
    if (pos < POS_FIGHTING)
        return 1 + (POS_FIGHTING - pos) / 3;
    return 1;
}

/* ---------------------------------------------------------------------- */

int main(int argc, char **argv)
{
    if (argc < 2) {
        fprintf(stderr, "usage: %s <thaco|compute|ac|multiplier|sweep-thaco> ...\n", argv[0]);
        return 2;
    }

    if (!strcmp(argv[1], "sweep-thaco")) {
        int chclass, level, str, add, hitroll, intel, wis;
        for (chclass = 0; chclass <= 4; chclass++)
            for (level = 0; level <= 34; level++)
                for (str = 3; str <= 18; str += 3)
                    for (add = 0; add <= 100; add += 25)
                        for (hitroll = -5; hitroll <= 10; hitroll += 5)
                            for (intel = 3; intel <= 25; intel += 2)
                                for (wis = 3; wis <= 25; wis += 4)
                                    printf("%d %d %d %d %d %d %d %d\n",
                                           chclass, level, str, add, hitroll, intel, wis,
                                           compute_thaco(0, chclass, level, str, add,
                                                         hitroll, intel, wis));
        return 0;
    }

    if (!strcmp(argv[1], "thaco")) {
        printf("%d\n", thaco(atoi(argv[2]), atoi(argv[3])));
        return 0;
    }

    if (!strcmp(argv[1], "compute")) {
        printf("%d\n", compute_thaco(atoi(argv[9]), atoi(argv[2]), atoi(argv[3]),
                                     atoi(argv[4]), atoi(argv[5]), atoi(argv[6]),
                                     atoi(argv[7]), atoi(argv[8])));
        return 0;
    }

    if (!strcmp(argv[1], "ac")) {
        printf("%d\n", compute_armor_class(atoi(argv[2]), atoi(argv[3]), atoi(argv[4])));
        return 0;
    }

    if (!strcmp(argv[1], "multiplier")) {
        printf("%d\n", position_multiplier(atoi(argv[2])));
        return 0;
    }

    fprintf(stderr, "unknown mode %s\n", argv[1]);
    return 2;
}
