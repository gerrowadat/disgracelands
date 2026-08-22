/*
 * nameoracle.c -- isname() and get_number(), from handler.c.
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * Both are original bodies, lifted with nothing changed but the removal of
 * the CircleMUD headers they would otherwise drag in. LOWER() is reproduced
 * from utils.h.
 *
 * Why they are worth an oracle rather than a reading:
 *
 *   isname() *looks* like a prefix match and is not. The inner loop exits
 *   on `!*curstr` and only returns 1 when the namelist character underneath
 *   is non-alphabetic — that is, when the keyword has ended too. So "swo"
 *   does not match "sword". This port had it as a prefix match, with a
 *   comment saying the C matched prefixes, for four phases.
 *
 *   get_number() rewrites the string it is given *before* deciding whether
 *   the prefix was a number, so "foo.bob" leaves "bob" behind and returns 0
 *   — and 0 has a meaning of its own to every caller.
 *
 * Build:  cc -o nameoracle nameoracle.c
 * Output: one `input<TAB>output` line per case, on stdout.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

#define MAX_INPUT_LENGTH 256
#define LOWER(c) (((c) >= 'A' && (c) <= 'Z') ? ((c) + ('a' - 'A')) : (c))

/* --- original bodies, unchanged --------------------------------------- */

int isname(const char *str, const char *namelist)
{
  const char *curname, *curstr;

  curname = namelist;
  for (;;) {
    for (curstr = str;; curstr++, curname++) {
      if (!*curstr && !isalpha(*curname))
	return (1);

      if (!*curname)
	return (0);

      if (!*curstr || *curname == ' ')
	break;

      if (LOWER(*curstr) != LOWER(*curname))
	break;
    }

    /* skip to next name */

    for (; isalpha(*curname); curname++);
    if (!*curname)
      return (0);
    curname++;			/* first char of new name */
  }
}

int get_number(char **name)
{
  int i;
  char *ppos;
  char number[MAX_INPUT_LENGTH];

  *number = '\0';

  if ((ppos = strchr(*name, '.')) != NULL) {
    *ppos++ = '\0';
    strcpy(number, *name);
    strcpy(*name, ppos);

    for (i = 0; *(number + i); i++)
      if (!isdigit(*(number + i)))
	return (0);

    return (atoi(number));
  }
  return (1);
}

/* --- the sweep -------------------------------------------------------- */

static const char *namelists[] = {
  "sword long",
  "sword",
  "dragon fractal puff",
  "guard cityguard",
  "Zod",
  "a b c",
  "",
  NULL
};

static const char *words[] = {
  "sword", "swo", "s", "SWORD", "Sword", "long", "lon", "g",
  "dragon", "puff", "PUFF", "fractal", "frac",
  "guard", "cityguard", "city", "zod", "ZOD", "zo",
  "a", "b", "c", "d", "ab", "",
  NULL
};

int main(void)
{
  int n, w;
  char buf[MAX_INPUT_LENGTH];
  char *p;

  /* isname over every pairing. */
  for (n = 0; namelists[n]; n++)
    for (w = 0; words[w]; w++)
      printf("isname\t%s\t%s\t%d\n", words[w], namelists[n],
	     isname(words[w], namelists[n]));

  /* get_number: the number it returns *and* what it leaves in the string,
   * because it rewrites in place and both halves matter to the caller. */
  {
    static const char *cases[] = {
      "sword", "2.sword", "0.sword", "1.sword", "10.sword",
      "foo.sword", ".sword", "2.", "..sword", "2.3.sword",
      "-1.sword", "007.sword", "2 . sword", "", ".",
      NULL
    };
    int i;
    for (i = 0; cases[i]; i++) {
      strcpy(buf, cases[i]);
      p = buf;
      n = get_number(&p);
      printf("get_number\t%s\t%d\t%s\n", cases[i], n, p);
    }
  }

  return 0;
}
