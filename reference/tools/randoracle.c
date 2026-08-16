/* Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
 * of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
 * Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
 * (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
 * and DikuMUD licenses; see LICENSE. Non-commercial use only.
 *
 * randoracle - dump the C server's random number generator.
 *
 * The Go port claims to reproduce circle_random() exactly, which is what
 * makes numeric parity in combat possible at all. That claim is only worth
 * something if it is checked against the actual C, so this is the actual C:
 * the body below is copied from reference/moderncserver/src/random.c with
 * nothing changed but the surrounding main().
 *
 *   randoracle <seed> <count>          dump raw circle_random() values
 *   randoracle <seed> <count> <lo> <hi>  dump number(lo, hi) values
 *
 * One value per line. internal/rng/rng_test.go compiles this and compares.
 */

#include <stdio.h>
#include <stdlib.h>

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

int number(int from, int to)
{
    if (from > to) {
        int tmp = from;
        from = to;
        to = tmp;
    }
    return ((circle_random() % (to - from + 1)) + from);
}

/* ---------------------------------------------------------------------- */

int main(int argc, char **argv)
{
    unsigned long initial;
    long count, i;

    if (argc != 3 && argc != 5) {
        fprintf(stderr, "usage: %s <seed> <count> [<lo> <hi>]\n", argv[0]);
        return 2;
    }

    initial = strtoul(argv[1], NULL, 10);
    count = strtol(argv[2], NULL, 10);

    circle_srandom(initial);

    if (argc == 3) {
        for (i = 0; i < count; i++)
            printf("%lu\n", circle_random());
    } else {
        int lo = atoi(argv[3]);
        int hi = atoi(argv[4]);
        for (i = 0; i < count; i++)
            printf("%d\n", number(lo, hi));
    }

    return 0;
}
