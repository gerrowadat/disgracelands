/*
 * castparse.c -- do_cast's argument parse (spell_parser.c:604), which is
 * three strtok() calls and is not what it looks like.
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * The three calls are original, in their original order, with the
 * send_to_char()s replaced by the name of the message. Everything they
 * lean on is libc.
 *
 * Why it is worth an oracle rather than a reading:
 *
 *   strtok skips a *run* of delimiters, and only the quote is a delimiter
 *   here -- a space is not. Those two facts together decide four different
 *   answers that a port written from "find the quotes" gets wrong:
 *
 *     cast ''            -> "must be enclosed": the two quotes collapse
 *                           into one skipped run and there is no second
 *                           token at all.
 *     cast '  '          -> the spell name is "  ", which find_skill_num
 *                           tokenises away and answers as if it were empty
 *                           -- so this casts armor, the first spell in the
 *                           table.
 *     cast '' fido       -> the spell name is " fido". The empty quotes
 *                           vanish and the *target* becomes the spell.
 *     cast 'magic missile  (no closing quote) -> works. The second strtok
 *                           has no delimiter left and returns the rest of
 *                           the line, so the quote is only needed at the
 *                           front.
 *
 *   This port's ParseCastArgument found the first quote and then the
 *   second, which gets every one of those wrong (#358) -- and the third of
 *   them is why the empty-spell-name behaviour could not be fixed without
 *   fixing this too (#365).
 *
 * Note what is *not* here: the interpreter's own step. ACMD receives
 * `argument` as any_one_arg left it, which is a pointer at the space after
 * the command word -- do_cast does no skip_spaces before its strtok -- so
 * "cast 'x'" arrives as " 'x'". Each line of input below is taken as what
 * the player typed, and the leading "cast" is removed the same way.
 *
 * Input:  one typed command line per line of stdin.
 * Output: "<typed>\t<verdict>\t<spell>\t<target>", where verdict is
 *         "what-where", "unenclosed" or "ok". A tab in the input would
 *         break that, and one cannot occur: the interpreter has already
 *         split on whitespace by the time do_cast is reached.
 *
 * Build:  cc -std=gnu89 -w -o castparse castparse.c
 */

#include <stdio.h>
#include <string.h>

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
    char buf[512], *s, *t;
    chomp(line);

    /*
     * What command_interpreter leaves: any_one_arg copies the command word
     * out and returns a pointer at the character it stopped on, which is
     * the space. A line that is exactly "cast" leaves an empty string.
     */
    if (strncmp(line, "cast", 4) != 0) {
      printf("%s\tnot-cast\t\t\n", line);
      continue;
    }
    strcpy(buf, line + 4);

    s = strtok(buf, "'");
    if (s == NULL) {
      printf("%s\twhat-where\t\t\n", line);
      continue;
    }
    s = strtok(NULL, "'");
    if (s == NULL) {
      printf("%s\tunenclosed\t\t\n", line);
      continue;
    }
    t = strtok(NULL, "\0");

    printf("%s\tok\t%s\t%s\n", line, s, t ? t : "");
  }

  return (0);
}
