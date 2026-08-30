/*
 * skilloracle.c -- find_skill_num(), from spell_parser.c, with the two
 * helpers it leans on: is_abbrev() from interpreter.c and any_one_arg()
 * from the same file.
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * All three are original bodies, lifted with nothing changed but the
 * removal of the CircleMUD headers they would otherwise drag in, and the
 * substitution of a name table read from stdin for the `spell_info[]`
 * global. LOWER() is reproduced from utils.h; skip_spaces() from utils.h
 * is inlined for the same reason.
 *
 * Why it is worth an oracle rather than a reading:
 *
 *   find_skill_num has *two* matching rules, not one, and the second is
 *   easy to miss because the first is the obvious one and returns on its
 *   own line. The first is `is_abbrev(name, spell_info[index].name)` --
 *   the whole typed string against the whole spell name, so "magic mis"
 *   finds "magic missile". The second walks both strings a word at a time
 *   with any_one_arg and requires each typed word to be an abbreviation of
 *   the spell-name word in the same position -- so "mag mis" finds it too,
 *   and so does "b h" for "burning hands".
 *
 *   That second rule is what a caster actually types, and this port had
 *   only the first. 1,145 of the 1,549 per-word abbreviations of the
 *   game's own 71 spell names were refused. See
 *   docs/investigations/partial-matching.md.
 *
 *   The loop's exit condition is also not what it looks like. It stops
 *   when *either* string runs out (`*first && *first2`), and the answer is
 *   `ok && !*first2` -- so a query with *fewer* words than the spell name
 *   matches ("cure" alone reaches "cure light" through this branch as well
 *   as through is_abbrev), while a query with *more* words than the name
 *   does not. Simulating that in your head is exactly what CLAUDE.md says
 *   not to do.
 *
 * Input:  a name table of "<number><TAB><name>" lines, in the table order
 *         the C iterates (ascending index), terminated by a blank line;
 *         then one query per line.
 * Output: one "<query><TAB><number>" line per query, -1 for no match.
 *
 * Build:  cc -std=gnu89 -w -o skilloracle skilloracle.c
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

/* utils.h */
#define LOWER(c) (((c) >= 'A' && (c) <= 'Z') ? ((c) + ('a' - 'A')) : (c))

#define MAX_SPELLS 400
#define NAME_LEN 128

static char names[MAX_SPELLS][NAME_LEN];
static int numbers[MAX_SPELLS];
static int top_spell = 0;

/* interpreter.c:1057 */
static int is_abbrev(const char *arg1, const char *arg2)
{
  if (!*arg1)
    return (0);

  for (; *arg1 && *arg2; arg1++, arg2++)
    if (LOWER(*arg1) != LOWER(*arg2))
      return (0);

  if (!*arg1)
    return (1);
  else
    return (0);
}

/* utils.h */
static void skip_spaces(char **string)
{
  for (; **string && isspace(**string); (*string)++);
}

/* interpreter.c */
static char *any_one_arg(char *argument, char *first_arg)
{
  skip_spaces(&argument);

  while (*argument && !isspace(*argument)) {
    *(first_arg++) = LOWER(*argument);
    argument++;
  }

  *first_arg = '\0';

  return (argument);
}

/*
 * spell_parser.c, with spell_info[index].name replaced by names[index] and
 * the returned index replaced by the number that came with it -- the C
 * returns the array index because that *is* the spell number there.
 *
 * The one deliberate change: the C's loop runs `index = 1; index <=
 * TOP_SPELL_DEFINE`, over a table with gaps whose unused entries are named
 * "!UNUSED!". This runs over exactly the entries it was given, in the
 * order it was given them, so the caller decides whether to include the
 * gaps. It matters only for a query that abbreviates "!UNUSED!", which is
 * to say a query starting with '!'.
 */
static int find_skill_num(char *name)
{
  int index, ok;
  char *temp, *temp2;
  char first[256], first2[256], tempbuf[256], namebuf[256];

  for (index = 0; index < top_spell; index++) {
    if (is_abbrev(name, names[index]))
      return (numbers[index]);

    ok = 1;
    temp = any_one_arg(strcpy(tempbuf, names[index]), first);
    temp2 = any_one_arg(strcpy(namebuf, name), first2);
    while (*first && *first2 && ok) {
      if (!is_abbrev(first2, first))
	ok = 0;
      temp = any_one_arg(temp, first);
      temp2 = any_one_arg(temp2, first2);
    }

    if (ok && !*first2)
      return (numbers[index]);
  }

  return (-1);
}

static void chomp(char *line)
{
  char *nl = strchr(line, '\n');
  if (nl)
    *nl = '\0';
}

int main(void)
{
  char line[512];

  while (fgets(line, sizeof line, stdin)) {
    char *tab;
    chomp(line);
    if (!*line)
      break;
    if (!(tab = strchr(line, '\t')))
      continue;
    if (top_spell >= MAX_SPELLS) {
      fprintf(stderr, "skilloracle: more than %d names\n", MAX_SPELLS);
      return (1);
    }
    *tab = '\0';
    numbers[top_spell] = atoi(line);
    strncpy(names[top_spell], tab + 1, NAME_LEN - 1);
    names[top_spell][NAME_LEN - 1] = '\0';
    top_spell++;
  }

  while (fgets(line, sizeof line, stdin)) {
    /* find_skill_num takes a char *, and any_one_arg writes through it via
     * strcpy into its own buffer -- but the first is_abbrev call reads the
     * caller's string directly, so it is passed a copy regardless. */
    char query[256];
    chomp(line);
    strncpy(query, line, sizeof query - 1);
    query[sizeof query - 1] = '\0';
    printf("%s\t%d\n", line, find_skill_num(query));
  }

  return (0);
}
