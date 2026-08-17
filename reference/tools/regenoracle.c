/* Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
 * of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
 * Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
 * (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
 * and DikuMUD licenses; see LICENSE. Non-commercial use only.
 *
 * regenoracle - dump the C server's regeneration formulas.
 *
 * hit_gain, mana_gain and move_gain are integer arithmetic with truncating
 * division at four points each, and the results feed every fight in the game.
 * Reading them across and hoping is not good enough, so the bodies below are
 * limits.c's, lifted with the char_data dereferences replaced by the plain
 * values they would have returned and nothing else changed.
 *
 *   regenoracle sweep
 *   regenoracle graf <age>
 *   regenoracle <hit|mana|move> <age> <pos> <caster> <starving> <poison> <goodregen>
 *   regenoracle npc <hit|mana|move> <level> <poison> <goodregen>
 *
 * One value per line. internal/game/regen_test.go compiles this and compares.
 */

#include <stdio.h>
#include <string.h>
#include <stdlib.h>

#define POS_SLEEPING 4
#define POS_RESTING  5
#define POS_SITTING  6

/* --- verbatim from src/limits.c:53 ------------------------------------ */

int graf(int age, int p0, int p1, int p2, int p3, int p4, int p5, int p6)
{
    if (age < 15)
        return (p0);                                    /* < 15   */
    else if (age <= 29)
        return (p1 + (((age - 15) * (p2 - p1)) / 15));   /* 15..29 */
    else if (age <= 44)
        return (p2 + (((age - 30) * (p3 - p2)) / 15));   /* 30..44 */
    else if (age <= 59)
        return (p3 + (((age - 45) * (p4 - p3)) / 15));   /* 45..59 */
    else if (age <= 79)
        return (p4 + (((age - 60) * (p5 - p4)) / 20));   /* 60..79 */
    else
        return (p6);                                    /* >= 80 */
}

/* --- limits.c:128, with the accessors substituted --------------------- */

int hit_gain(int is_npc, int level, int years, int pos, int caster,
             int starving, int poisoned, int good_regen)
{
    int gain;

    if (is_npc) {
        gain = level;
    } else {
        gain = graf(years, 8, 12, 20, 32, 16, 10, 4);

        switch (pos) {
        case POS_SLEEPING: gain += (gain / 2); break;
        case POS_RESTING:  gain += (gain / 4); break;
        case POS_SITTING:  gain += (gain / 8); break;
        }

        if (caster)
            gain /= 2;

        if (starving)
            gain /= 4;
    }

    if (poisoned)
        gain /= 4;

    if (good_regen)
        gain += (gain * 1);

    return (gain);
}

/* --- limits.c:81 ------------------------------------------------------ */

int mana_gain(int is_npc, int level, int years, int pos, int caster,
              int starving, int poisoned, int good_regen)
{
    int gain;

    if (is_npc) {
        gain = level;
    } else {
        gain = graf(years, 4, 8, 12, 16, 12, 10, 8);

        switch (pos) {
        case POS_SLEEPING: gain *= 2; break;
        case POS_RESTING:  gain += (gain / 2); break;
        case POS_SITTING:  gain += (gain / 4); break;
        }

        if (caster)
            gain *= 2;

        if (starving)
            gain /= 4;
    }

    if (poisoned)
        gain /= 4;

    if (good_regen)
        gain += (gain * 1);

    return (gain);
}

/* --- limits.c:178 ----------------------------------------------------- */

int move_gain(int is_npc, int level, int years, int pos,
              int starving, int poisoned, int good_regen)
{
    int gain;

    if (is_npc) {
        gain = level;
    } else {
        gain = graf(years, 16, 20, 24, 20, 16, 12, 10);

        switch (pos) {
        case POS_SLEEPING: gain += (gain / 2); break;
        case POS_RESTING:  gain += (gain / 4); break;
        case POS_SITTING:  gain += (gain / 8); break;
        }

        if (starving)
            gain /= 4;
    }

    if (poisoned)
        gain /= 4;

    if (good_regen)
        gain += (gain * 1);

    return (gain);
}

/* ---------------------------------------------------------------------- */

int main(int argc, char **argv)
{
    if (argc < 2) {
        fprintf(stderr, "usage: %s graf <age>\n", argv[0]);
        fprintf(stderr, "       %s <hit|mana|move> <age> <pos> <caster> <starving> <poison> <goodregen>\n", argv[0]);
        fprintf(stderr, "       %s npc <hit|mana|move> <level> <poison> <goodregen>\n", argv[0]);
        return 2;
    }

    /* One process for the whole space: the Go side drives 38,000-odd
     * comparisons and spawning a process for each takes minutes. */
    if (!strcmp(argv[1], "sweep")) {
        int age, pos, caster, starving, poison, good;
        for (age = 17; age <= 100; age++)
            for (pos = 0; pos <= 8; pos++)
                for (caster = 0; caster <= 1; caster++)
                    for (starving = 0; starving <= 1; starving++)
                        for (poison = 0; poison <= 1; poison++)
                            for (good = 0; good <= 1; good++)
                                printf("%d %d %d %d %d %d %d %d %d\n",
                                       age, pos, caster, starving, poison, good,
                                       hit_gain(0, 0, age, pos, caster, starving, poison, good),
                                       mana_gain(0, 0, age, pos, caster, starving, poison, good),
                                       move_gain(0, 0, age, pos, starving, poison, good));
        return 0;
    }

    if (!strcmp(argv[1], "graf")) {
        int age = atoi(argv[2]);
        printf("%d\n", graf(age, 8, 12, 20, 32, 16, 10, 4));
        printf("%d\n", graf(age, 4, 8, 12, 16, 12, 10, 8));
        printf("%d\n", graf(age, 16, 20, 24, 20, 16, 12, 10));
        return 0;
    }

    if (!strcmp(argv[1], "npc")) {
        int level = atoi(argv[3]);
        int poison = atoi(argv[4]);
        int good = atoi(argv[5]);
        if (!strcmp(argv[2], "hit"))
            printf("%d\n", hit_gain(1, level, 0, 0, 0, 0, poison, good));
        else if (!strcmp(argv[2], "mana"))
            printf("%d\n", mana_gain(1, level, 0, 0, 0, 0, poison, good));
        else
            printf("%d\n", move_gain(1, level, 0, 0, 0, poison, good));
        return 0;
    }

    if (argc != 8) {
        fprintf(stderr, "wrong argument count\n");
        return 2;
    }
    {
        int age = atoi(argv[2]);
        int pos = atoi(argv[3]);
        int caster = atoi(argv[4]);
        int starving = atoi(argv[5]);
        int poison = atoi(argv[6]);
        int good = atoi(argv[7]);

        if (!strcmp(argv[1], "hit"))
            printf("%d\n", hit_gain(0, 0, age, pos, caster, starving, poison, good));
        else if (!strcmp(argv[1], "mana"))
            printf("%d\n", mana_gain(0, 0, age, pos, caster, starving, poison, good));
        else if (!strcmp(argv[1], "move"))
            printf("%d\n", move_gain(0, 0, age, pos, starving, poison, good));
        else {
            fprintf(stderr, "unknown formula %s\n", argv[1]);
            return 2;
        }
    }
    return 0;
}
