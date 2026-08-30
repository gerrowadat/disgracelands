/*
 * castoracle.c -- do_cast's argument parsing (spell_parser.c:603-611),
 * with the interpreter's own handling of what `argument` even is.
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * Original bodies, lifted with nothing changed but the removal of the
 * CircleMUD headers they would otherwise drag in. skip_spaces() and
 * any_one_arg() are from utils.h/interpreter.c; LOWER() from utils.h.
 *
 * Why it is worth an oracle rather than a reading:
 *
 *   Three separate things have to be got right together, and getting any
 *   one of them wrong makes the other two look wrong too. This was read
 *   twice, confidently, and was wrong both times before it was compiled.
 *
 *   1. **`argument` has a leading space.** command_interpreter does
 *      `line = any_one_arg(argument, arg)` (interpreter.c:1019), and
 *      any_one_arg skips spaces at the *start* and returns a pointer to
 *      the character after the word it copied -- which is the space
 *      before the rest. So do_cast sees " 'magic missile' fido", not
 *      "'magic missile' fido".
 *
 *   2. **strtok skips leading delimiters.** That is what makes (1)
 *      matter: with the leading space, the first strtok returns that
 *      space (do_cast's comment calls it "blank") and the *second*
 *      returns the spell name. Without the leading space the first call
 *      would return the spell name and the second the target, and do_cast
 *      would cast the target. Reading the function with the wrong idea of
 *      its input therefore predicts that ordinary casting is broken,
 *      which is a useful signal that the reading is wrong and an easy one
 *      to talk yourself out of.
 *
 *   3. **find_skill_num matches a name with no words.** `cast '   '`
 *      reaches it with "   ", whose any_one_arg yields an empty first
 *      word, so the word loop never runs, `ok && !*first2` holds on the
 *      first table entry, and the C casts whichever spell sits lowest in
 *      spell_info[]. That is reachable from the keyboard and is not
 *      something anybody designed.
 *
 * Input:  a name table of "<number><TAB><name>" lines in table order,
 *         terminated by a blank line; then one *typed line* per line --
 *         the whole thing, "cast 'mag mis' fido", command word included,
 *         because what the command word does to the argument is half of
 *         what is being tested.
 * Output: one "<typed><TAB><outcome>" line per query, where outcome is
 *         "blank", "unenclosed", "unknown", or "<number><TAB><target>".
 *
 * Build:  cc -std=gnu89 -w -o castoracle castoracle.c
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

/* spell_parser.c; see skilloracle.c for this one's own notes. */
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

/*
 * do_cast's argument handling, from the point command_interpreter hands
 * over. `typed` is the whole line so that the command word's effect on
 * `argument` is part of what is reproduced -- see (1) in the header.
 */
static void do_cast(const char *typed)
{
  char buf[1024], cmdword[256];
  char *argument, *s, *t;

  strncpy(buf, typed, sizeof buf - 1);
  buf[sizeof buf - 1] = '\0';

  /* command_interpreter (interpreter.c:606, :1019) */
  argument = buf;
  skip_spaces(&argument);
  argument = any_one_arg(argument, cmdword);

  /* spell_parser.c:603-611, verbatim. */
  s = strtok(argument, "'");
  if (s == NULL) {
    printf("%s\tblank\n", typed);
    return;
  }
  s = strtok(NULL, "'");
  if (s == NULL) {
    printf("%s\tunenclosed\n", typed);
    return;
  }
  t = strtok(NULL, "\0");

  {
    int n = find_skill_num(s);
    if (n < 0)
      printf("%s\tunknown\n", typed);
    else
      printf("%s\t%d\t%s\n", typed, n, t ? t : "");
  }
}

static void chomp(char *line)
{
  char *nl = strchr(line, '\n');
  if (nl)
    *nl = '\0';
}

int main(void)
{
  char line[1024];

  while (fgets(line, sizeof line, stdin)) {
    char *tab;
    chomp(line);
    if (!*line)
      break;
    if (!(tab = strchr(line, '\t')))
      continue;
    if (top_spell >= MAX_SPELLS) {
      fprintf(stderr, "castoracle: more than %d names\n", MAX_SPELLS);
      return (1);
    }
    *tab = '\0';
    numbers[top_spell] = atoi(line);
    strncpy(names[top_spell], tab + 1, NAME_LEN - 1);
    names[top_spell][NAME_LEN - 1] = '\0';
    top_spell++;
  }

  while (fgets(line, sizeof line, stdin)) {
    chomp(line);
    do_cast(line);
  }

  return (0);
}
