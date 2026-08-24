/*
 * aliasoracle.c -- the per-character alias file, written and read by the
 * original code.
 *
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * Unlike the other file formats in this tree, this one is not an fwrite of a
 * struct: alias.c uses fprintf and fscanf/fgets, so there is no layout to
 * derive and no *layout.c to derive it. What it has instead is a
 * length-prefix convention with one adjustment in it that is very easy to
 * read past:
 *
 *     int repllen = strlen(temp->replacement) - 1;
 *     fprintf(file, ... "%d\n%s\n" ..., repllen, temp->replacement + 1, ...);
 *
 * and, coming back the other way:
 *
 *     *xbuf = ' ';
 *     fgets(xbuf + 1, length + 1, file);
 *
 * The in-memory replacement always starts with a space -- do_alias builds it
 * with any_one_arg, which stops on the separating whitespace without
 * skipping it -- and the file stores it without. Miss the pairing in either
 * direction and every replacement loses or gains a leading character, which
 * for a simple alias is invisible until someone uses it.
 *
 * So this is the oracle for the round trip: write_aliases and read_aliases,
 * bodies unchanged apart from the char_data dereferences the alias list
 * hangs off, run over aliases given on the command line.
 *
 * Usage: aliasoracle <outfile> <name> <replacement> [<name> <replacement>...]
 *
 * The replacement is given the way it is held in memory, leading space and
 * all. Prints the file it wrote, byte for byte, as escaped text; then what
 * read_aliases makes of it, one field per line.
 *
 * No -m32 needed: there is no struct here, so nothing depends on the data
 * model.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define ALIAS_SIMPLE	0
#define ALIAS_COMPLEX	1
#define ALIAS_SEP_CHAR	';'
#define ALIAS_VAR_CHAR	'$'

struct alias_data {
  char *alias;
  char *replacement;
  int type;
  struct alias_data *next;
};

/* write_aliases (alias.c:22), with GET_ALIASES(ch) as a parameter. */
static void write_aliases(const char *fn, struct alias_data *aliases)
{
  FILE *file;
  struct alias_data *temp;

  remove(fn);

  if (aliases == NULL)
    return;

  if ((file = fopen(fn, "w")) == NULL) {
    fprintf(stderr, "couldn't save aliases in '%s'\n", fn);
    exit(1);
  }

  for (temp = aliases; temp; temp = temp->next) {
    int aliaslen = strlen(temp->alias);
    int repllen = strlen(temp->replacement) - 1;

    fprintf(file, "%d\n%s\n"	/* Alias */
		  "%d\n%s\n"	/* Replacement */
		  "%d\n",	/* Type */
		aliaslen, temp->alias,
		repllen, temp->replacement + 1,
		temp->type);
  }

  fclose(file);
}

/* read_aliases (alias.c:55), returning the list rather than storing it. */
static struct alias_data *read_aliases(const char *fn)
{
  FILE *file;
  char xbuf[8192];
  struct alias_data *head, *t2;
  int length;

  if ((file = fopen(fn, "r")) == NULL)
    return (NULL);

  head = calloc(1, sizeof(struct alias_data));
  t2 = head;

  for (;;) {
    /* Read the aliased command. */
    fscanf(file, "%d\n", &length);
    fgets(xbuf, length + 1, file);
    t2->alias = strdup(xbuf);

    /* Build the replacement. */
    fscanf(file, "%d\n", &length);
    *xbuf = ' ';		/* Doesn't need terminated, fgets() will. */
    fgets(xbuf + 1, length + 1, file);
    t2->replacement = strdup(xbuf);

    /* Figure out the alias type. */
    fscanf(file, "%d\n", &length);
    t2->type = length;

    if (feof(file))
      break;

    t2->next = calloc(1, sizeof(struct alias_data));
    t2 = t2->next;
  };

  fclose(file);
  return (head);
}

static void print_escaped(const char *label, const char *s, int len)
{
  int i;
  printf("%s ", label);
  for (i = 0; i < len; i++) {
    if (s[i] == '\n')
      printf("\\n");
    else if (s[i] == '\\')
      printf("\\\\");
    else
      printf("%c", s[i]);
  }
  printf("\n");
}

int main(int argc, char **argv)
{
  struct alias_data *head = NULL, *tail = NULL, *a;
  const char *fn;
  FILE *f;
  char buf[65536];
  size_t n;
  int i;

  if (argc < 4 || (argc % 2) != 0) {
    fprintf(stderr, "usage: %s <outfile> <name> <replacement> [<name> <replacement>...]\n", argv[0]);
    return (2);
  }
  fn = argv[1];

  /* Build the list in the order given, which is the order the file gets. */
  for (i = 2; i < argc; i += 2) {
    a = calloc(1, sizeof(struct alias_data));
    a->alias = argv[i];
    a->replacement = argv[i + 1];
    a->type = (strchr(a->replacement, ALIAS_SEP_CHAR) ||
	       strchr(a->replacement, ALIAS_VAR_CHAR)) ? ALIAS_COMPLEX : ALIAS_SIMPLE;
    if (tail)
      tail->next = a;
    else
      head = a;
    tail = a;
  }

  write_aliases(fn, head);

  if (!(f = fopen(fn, "rb"))) {
    fprintf(stderr, "cannot reopen '%s'\n", fn);
    return (1);
  }
  n = fread(buf, 1, sizeof(buf), f);
  fclose(f);
  print_escaped("file", buf, (int) n);

  for (a = read_aliases(fn); a; a = a->next) {
    print_escaped("alias", a->alias, (int) strlen(a->alias));
    print_escaped("replacement", a->replacement, (int) strlen(a->replacement));
    printf("type %d\n", a->type);
  }
  return (0);
}
