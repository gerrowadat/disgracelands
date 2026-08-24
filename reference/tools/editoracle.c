/* Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
 * of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
 * Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
 * (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
 * and DikuMUD licenses; see LICENSE. Non-commercial use only.
 *
 * editoracle.c -- improved_editor_execute(), parse_action(), format_text()
 * and replace_str(), from improved-edit.c. See reference/tools/README.md.
 *
 * All four are original bodies, lifted with nothing changed but the
 * substitutions this directory's other oracles make: `struct
 * descriptor_data` is cut down to the two members these functions touch
 * (`char **str` and `unsigned int max_str`), SEND_TO_Q appends to a string
 * instead of a socket queue, page_string does the same, and mudlog/log are
 * no-ops. The bodies themselves are byte-identical to
 * ../moderncserver/src/improved-edit.c.
 *
 * Why they are worth an oracle rather than a reading: the six commands
 * being ported against it (/d /e /f /i /n /r) are all line-range or
 * whole-buffer string surgery, and every one of them has at least one
 * result that a careful reading gets wrong. Found by writing this:
 *
 *   - The `if (*(d->str))` guards on /f /i /l /n are NULL-*pointer* tests,
 *     not "is the buffer empty" tests. A buffer emptied by /d 1-<last> is
 *     a live pointer to "", so /l on it prints a blank line and
 *     "0 lines shown." where /l after /c says "Current buffer empty."
 *
 *   - Walking to line N stops on the '\0' after the buffer's final '\n',
 *     which is a valid empty line, not the end. On a three-line buffer
 *     /d 4 reports "0 lines deleted." and /l 4 prints an empty listing —
 *     "out of range" starts at line 5, not line 4.
 *
 *   - /r's own space check is unsigned: `(strlen(t) - strlen(s)) +
 *     strlen(*d->str)`. A pattern longer than the whole buffer wraps the
 *     subtraction and the answer comes out near UINT_MAX, so the reply is
 *     "Not enough space left in buffer." rather than "String ... not
 *     found."
 *
 *   - PARSE_LIST_NUM prints its line number and the line itself on
 *     separate lines ("%4d:\r\n"), and prints no "N lines shown." footer,
 *     though PARSE_LIST_NORM does.
 *
 *   - /f's option scan is `while (isalpha(string[j]) && j < 2)`, over
 *     `str + 2` — so "/fi" indents and "/f i" does not.
 *
 * Three case shapes are deliberately *not* emitted, because they are
 * undefined behaviour in the C rather than behaviour to reproduce. A /d
 * with no argument at all, and a /l or /n whose argument is present but
 * all whitespace: `sscanf(string, " %d - %d ", ...)` returns EOF (-1) for
 * those, the switch that reads it has cases 0, 1 and 2 and no default, and
 * line_low/line_high are uninitialised locals. And /r on a freshly-opened
 * editor, which is a plain NULL dereference — see the skip in main(). All
 * three are in docs/deviations.md.
 *
 * Build:  cc -O2 -Wall -Werror -Wno-restrict -o editoracle editoracle.c
 *         (-Wno-restrict: PARSE_LIST_NUM's own `sprintf(buf, "%s%4d:\r\n",
 *          buf, ...)` reads and writes the same buffer, which is the C's,
 *          not a transcription slip.)
 * Output: one tab-separated row per case, on stdout, with \, CR, LF and TAB
 *         backslash-escaped in every field:
 *
 *         <in-buffer|NULL> <max_str> <command line> <return> <out-buffer|NULL> <sent text>
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>

/* --- the handful of things improved-edit.c gets from its headers ------- */

#define MAX_STRING_LENGTH	8192
#define MAX_INPUT_LENGTH	256
#define TRUE			1
#define FALSE			0
#define UPPER(c)   (((c)>='a'  && (c) <= 'z') ? ((c)+('A'-'a')) : (c) )
#define LOWER(c)   (((c)>='A'  && (c) <= 'Z') ? ((c)+('a'-'A')) : (c) )
#define IS_SET(flag,bit)  ((flag) & (bit))
#define MIN(a, b)  ((a) < (b) ? (a) : (b))

#define CREATE(result, type, number)  do {\
	if (!((result) = (type *) calloc ((number), sizeof(type))))\
		{ perror("calloc"); abort(); } } while(0)

#define RECREATE(result,type,number) do {\
  if (!((result) = (type *) realloc ((result), sizeof(type) * (number))))\
		{ perror("realloc"); abort(); } } while(0)

#define PARSE_FORMAT		0
#define PARSE_REPLACE		1
#define PARSE_HELP		2
#define PARSE_DELETE		3
#define PARSE_INSERT		4
#define PARSE_LIST_NORM		5
#define PARSE_LIST_NUM		6
#define PARSE_EDIT		7

#define STRINGADD_OK		0
#define STRINGADD_SAVE		1
#define STRINGADD_ABORT		2
#define STRINGADD_ACTION	4

#define FORMAT_INDENT		(1 << 0)

/* The two members of descriptor_data these functions actually touch. */
struct descriptor_data {
  char **str;
  unsigned int max_str;
};

/* improved-edit.c's scratch globals, from the C's own buffer pool. */
static char buf[MAX_STRING_LENGTH];
static char buf2[MAX_STRING_LENGTH];

/* Where SEND_TO_Q and page_string put their text instead of a socket. */
static char sent[MAX_STRING_LENGTH * 4];

/*
 * The one change to a body rather than to its surroundings, and the reason
 * for it: replace_str allocates `replace_buffer` at exactly max_size and
 * then, after its rep_all loop, runs an *unchecked* `strcat(replace_buffer,
 * jetsam)` for whatever is left after the last match. Its in-loop check
 * budgets only up to max_size and does not count that tail, so a /ra that
 * grows the buffer past max_size writes off the end of the allocation. This
 * oracle died with "free(): invalid next size (fast)" on its own /ra cases
 * before the slack went in.
 *
 * Every comparison replace_str makes is still against max_size, so which
 * branch it takes is unchanged; the slack only means the harness survives
 * to print the answer the code was trying to compute. That answer is what
 * the Go — which cannot overrun a slice — is required to reproduce. See
 * docs/deviations.md.
 */
#define ORACLE_SLACK	MAX_STRING_LENGTH

#define SEND_TO_Q(msg, d)	strcat(sent, (msg))
#define page_string(d, msg, keep)	strcat(sent, (msg))
#define mudlog(msg, t, l, f)	((void) 0)
#define log(msg)		((void) 0)

void parse_action(int command, char *string, struct descriptor_data *d);
void format_text(char **ptr_string, int mode, struct descriptor_data *d, unsigned int maxlen);
int replace_str(char **string, char *pattern, char *replacement, int rep_all, unsigned int max_size);

/* --- interpreter.c:906,1023,1075, for /e and /i's half_chop ------------ */

void skip_spaces(char **string)
{
  for (; **string && isspace(**string); (*string)++);
}

char *any_one_arg(char *argument, char *first_arg)
{
  skip_spaces(&argument);

  while (*argument && !isspace(*argument)) {
    *(first_arg++) = LOWER(*argument);
    argument++;
  }

  *first_arg = '\0';

  return (argument);
}

void half_chop(char *string, char *arg1, char *arg2)
{
  char *temp;

  temp = any_one_arg(string, arg1);
  skip_spaces(&temp);
  strcpy(arg2, temp);
}

/* --- original bodies, unchanged ---------------------------------------- */

int improved_editor_execute(struct descriptor_data *d, char *str)
{
  char actions[MAX_INPUT_LENGTH];

  if (*str != '/')
    return STRINGADD_OK;

  strncpy(actions, str + 2, sizeof(actions) - 1);
  actions[sizeof(actions) - 1] = '\0';
  *str = '\0';

  switch (str[1]) {
  case 'a':
    return STRINGADD_ABORT;
  case 'c':
    if (*(d->str)) {
      free(*d->str);
      *(d->str) = NULL;
      SEND_TO_Q("Current buffer cleared.\r\n", d);
    } else
      SEND_TO_Q("Current buffer empty.\r\n", d);
    break;
  case 'd':
    parse_action(PARSE_DELETE, actions, d);
    break;
  case 'e':
    parse_action(PARSE_EDIT, actions, d);
    break;
  case 'f':
    if (*(d->str))
      parse_action(PARSE_FORMAT, actions, d);
    else
      SEND_TO_Q("Current buffer empty.\r\n", d);
    break;
  case 'i':
    if (*(d->str))
      parse_action(PARSE_INSERT, actions, d);
    else
      SEND_TO_Q("Current buffer empty.\r\n", d);
    break;
  case 'h':
    parse_action(PARSE_HELP, actions, d);
    break;
  case 'l':
    if (*d->str)
      parse_action(PARSE_LIST_NORM, actions, d);
    else
      SEND_TO_Q("Current buffer empty.\r\n", d);
    break;
  case 'n':
    if (*d->str)
      parse_action(PARSE_LIST_NUM, actions, d);
    else
      SEND_TO_Q("Current buffer empty.\r\n", d);
    break;
  case 'r':
    parse_action(PARSE_REPLACE, actions, d);
    break;
  case 's':
    return STRINGADD_SAVE;
  default:
    SEND_TO_Q("Invalid option.\r\n", d);
    break;
  }
  return STRINGADD_ACTION;
}

/*
 * Handle some editor commands.
 */
void parse_action(int command, char *string, struct descriptor_data *d)
{
  int indent = 0, rep_all = 0, flags = 0, replaced, i, line_low, line_high, j = 0;
  unsigned int total_len;
  char *s, *t, temp;

  switch (command) {
  case PARSE_HELP:
    sprintf(buf,
	    "Editor command formats: /<letter>\r\n\r\n"
	    "/a         -  aborts editor\r\n"
	    "/c         -  clears buffer\r\n"
	    "/d#        -  deletes a line #\r\n"
	    "/e# <text> -  changes the line at # with <text>\r\n"
	    "/f         -  formats text\r\n"
	    "/fi        -  indented formatting of text\r\n"
	    "/h         -  list text editor commands\r\n"
	    "/i# <text> -  inserts <text> before line #\r\n"
	    "/l         -  lists buffer\r\n"
	    "/n         -  lists buffer with line numbers\r\n"
	    "/r 'a' 'b' -  replace 1st occurance of text <a> in buffer with text <b>\r\n"
	    "/ra 'a' 'b'-  replace all occurances of text <a> within buffer with text <b>\r\n"
	    "              usage: /r[a] 'pattern' 'replacement'\r\n"
	    "/s         -  saves text\r\n");
    SEND_TO_Q(buf, d);
    break;
  case PARSE_FORMAT:
    while (isalpha(string[j]) && j < 2)
      if (string[j++] == 'i' && !indent) {
	indent = TRUE;
	flags += FORMAT_INDENT;
      }
    format_text(d->str, flags, d, d->max_str);
    sprintf(buf, "Text formatted with%s indent.\r\n", (indent ? "" : "out"));
    SEND_TO_Q(buf, d);
    break;
  case PARSE_REPLACE:
    while (isalpha(string[j]) && j < 2)
      if (string[j++] == 'a' && !indent)
	rep_all = 1;

    if ((s = strtok(string, "'")) == NULL) {
      SEND_TO_Q("Invalid format.\r\n", d);
      return;
    } else if ((s = strtok(NULL, "'")) == NULL) {
      SEND_TO_Q("Target string must be enclosed in single quotes.\r\n", d);
      return;
    } else if ((t = strtok(NULL, "'")) == NULL) {
      SEND_TO_Q("No replacement string.\r\n", d);
      return;
    } else if ((t = strtok(NULL, "'")) == NULL) {
      SEND_TO_Q("Replacement string must be enclosed in single quotes.\r\n", d);
      return;
    } else if ((total_len = ((strlen(t) - strlen(s)) + strlen(*d->str))) <= d->max_str) {
      if ((replaced = replace_str(d->str, s, t, rep_all, d->max_str)) > 0) {
	sprintf(buf, "Replaced %d occurance%sof '%s' with '%s'.\r\n", replaced, ((replaced != 1) ? "s " : " "), s, t);
	SEND_TO_Q(buf, d);
      } else if (replaced == 0) {
	sprintf(buf, "String '%s' not found.\r\n", s);
	SEND_TO_Q(buf, d);
      } else
	SEND_TO_Q("ERROR: Replacement string causes buffer overflow, aborted replace.\r\n", d);
    } else
      SEND_TO_Q("Not enough space left in buffer.\r\n", d);
    break;
  case PARSE_DELETE:
    switch (sscanf(string, " %d - %d ", &line_low, &line_high)) {
    case 0:
      SEND_TO_Q("You must specify a line number or range to delete.\r\n", d);
      return;
    case 1:
      line_high = line_low;
      break;
    case 2:
      if (line_high < line_low) {
	SEND_TO_Q("That range is invalid.\r\n", d);
	return;
      }
      break;
    }

    i = 1;
    total_len = 1;
    if ((s = *d->str) == NULL) {
      SEND_TO_Q("Buffer is empty.\r\n", d);
      return;
    } else if (line_low > 0) {
      while (s && i < line_low)
	if ((s = strchr(s, '\n')) != NULL) {
	  i++;
	  s++;
	}
      if (s == NULL || i < line_low) {
	SEND_TO_Q("Line(s) out of range; not deleting.\r\n", d);
	return;
      }
      t = s;
      while (s && i < line_high)
	if ((s = strchr(s, '\n')) != NULL) {
	  i++;
	  total_len++;
	  s++;
	}
      if (s && (s = strchr(s, '\n')) != NULL) {
	while (*(++s))
	  *(t++) = *s;
      } else
	total_len--;
      *t = '\0';
      RECREATE(*d->str, char, strlen(*d->str) + 3);

      sprintf(buf, "%d line%sdeleted.\r\n", total_len, (total_len != 1 ? "s " : " "));
      SEND_TO_Q(buf, d);
    } else {
      SEND_TO_Q("Invalid, line numbers to delete must be higher than 0.\r\n", d);
      return;
    }
    break;
  case PARSE_LIST_NORM:
    /*
     * Note: Rv's buf, buf1, buf2, and arg variables are defined to 32k so
     * they are probly ok for what to do here.
     */
    *buf = '\0';
    if (*string)
      switch (sscanf(string, " %d - %d ", &line_low, &line_high)) {
      case 0:
	line_low = 1;
	line_high = 999999;
	break;
      case 1:
	line_high = line_low;
	break;
    } else {
      line_low = 1;
      line_high = 999999;
    }

    if (line_low < 1) {
      SEND_TO_Q("Line numbers must be greater than 0.\r\n", d);
      return;
    } else if (line_high < line_low) {
      SEND_TO_Q("That range is invalid.\r\n", d);
      return;
    }
    *buf = '\0';
    if (line_high < 999999 || line_low > 1)
      sprintf(buf, "Current buffer range [%d - %d]:\r\n", line_low, line_high);
    i = 1;
    total_len = 0;
    s = *d->str;
    while (s && (i < line_low))
      if ((s = strchr(s, '\n')) != NULL) {
	i++;
	s++;
      }
    if (i < line_low || s == NULL) {
      SEND_TO_Q("Line(s) out of range; no buffer listing.\r\n", d);
      return;
    }
    t = s;
    while (s && i <= line_high)
      if ((s = strchr(s, '\n')) != NULL) {
	i++;
	total_len++;
	s++;
      }
    if (s) {
      temp = *s;
      *s = '\0';
      strcat(buf, t);
      *s = temp;
    } else
      strcat(buf, t);
    /*
     * This is kind of annoying...but some people like it.
     */
    sprintf(buf + strlen(buf), "\r\n%d line%sshown.\r\n", total_len, (total_len != 1) ? "s " : " ");
    page_string(d, buf, TRUE);
    break;
  case PARSE_LIST_NUM:
    /*
     * Note: Rv's buf, buf1, buf2, and arg variables are defined to 32k so
     * they are probly ok for what to do here.
     */
    *buf = '\0';
    if (*string)
      switch (sscanf(string, " %d - %d ", &line_low, &line_high)) {
      case 0:
	line_low = 1;
	line_high = 999999;
	break;
      case 1:
	line_high = line_low;
	break;
    } else {
      line_low = 1;
      line_high = 999999;
    }

    if (line_low < 1) {
      SEND_TO_Q("Line numbers must be greater than 0.\r\n", d);
      return;
    }
    if (line_high < line_low) {
      SEND_TO_Q("That range is invalid.\r\n", d);
      return;
    }
    *buf = '\0';
    i = 1;
    total_len = 0;
    s = *d->str;
    while (s && i < line_low)
      if ((s = strchr(s, '\n')) != NULL) {
	i++;
	s++;
      }
    if (i < line_low || s == NULL) {
      SEND_TO_Q("Line(s) out of range; no buffer listing.\r\n", d);
      return;
    }
    t = s;
    while (s && i <= line_high)
      if ((s = strchr(s, '\n')) != NULL) {
	i++;
	total_len++;
	s++;
	temp = *s;
	*s = '\0';
	sprintf(buf, "%s%4d:\r\n", buf, (i - 1));
	strcat(buf, t);
	*s = temp;
	t = s;
      }
    if (s && t) {
      temp = *s;
      *s = '\0';
      strcat(buf, t);
      *s = temp;
    } else if (t)
      strcat(buf, t);

    page_string(d, buf, TRUE);
    break;

  case PARSE_INSERT:
    half_chop(string, buf, buf2);
    if (*buf == '\0') {
      SEND_TO_Q("You must specify a line number before which to insert text.\r\n", d);
      return;
    }
    line_low = atoi(buf);
    strcat(buf2, "\r\n");

    i = 1;
    *buf = '\0';
    if ((s = *d->str) == NULL) {
      SEND_TO_Q("Buffer is empty, nowhere to insert.\r\n", d);
      return;
    }
    if (line_low > 0) {
      while (s && (i < line_low))
	if ((s = strchr(s, '\n')) != NULL) {
	  i++;
	  s++;
	}
      if (i < line_low || s == NULL) {
	SEND_TO_Q("Line number out of range; insert aborted.\r\n", d);
	return;
      }
      temp = *s;
      *s = '\0';
      if ((strlen(*d->str) + strlen(buf2) + strlen(s + 1) + 3) > d->max_str) {
	*s = temp;
	SEND_TO_Q("Insert text pushes buffer over maximum size, insert aborted.\r\n", d);
	return;
      }
      if (*d->str && **d->str)
	strcat(buf, *d->str);
      *s = temp;
      strcat(buf, buf2);
      if (s && *s)
	strcat(buf, s);
      RECREATE(*d->str, char, strlen(buf) + 3);

      strcpy(*d->str, buf);
      SEND_TO_Q("Line inserted.\r\n", d);
    } else {
      SEND_TO_Q("Line number must be higher than 0.\r\n", d);
      return;
    }
    break;

  case PARSE_EDIT:
    half_chop(string, buf, buf2);
    if (*buf == '\0') {
      SEND_TO_Q("You must specify a line number at which to change text.\r\n", d);
      return;
    }
    line_low = atoi(buf);
    strcat(buf2, "\r\n");

    i = 1;
    *buf = '\0';
    if ((s = *d->str) == NULL) {
      SEND_TO_Q("Buffer is empty, nothing to change.\r\n", d);
      return;
    }
    if (line_low > 0) {
      /*
       * Loop through the text counting \n characters until we get to the line.
       */
      while (s && i < line_low)
	if ((s = strchr(s, '\n')) != NULL) {
	  i++;
	  s++;
	}
      /*
       * Make sure that there was a THAT line in the text.
       */
      if (s == NULL || i < line_low) {
	SEND_TO_Q("Line number out of range; change aborted.\r\n", d);
	return;
      }
      /*
       * If s is the same as *d->str that means I'm at the beginning of the
       * message text and I don't need to put that into the changed buffer.
       */
      if (s != *d->str) {
	/*
	 * First things first .. we get this part into the buffer.
	 */
	temp = *s;
	*s = '\0';
	/*
	 * Put the first 'good' half of the text into storage.
	 */
	strcat(buf, *d->str);
	*s = temp;
      }
      /*
       * Put the new 'good' line into place.
       */
      strcat(buf, buf2);
      if ((s = strchr(s, '\n')) != NULL) {
	/*
	 * This means that we are at the END of the line, we want out of
	 * there, but we want s to point to the beginning of the line
	 * AFTER the line we want edited
	 */
	s++;
	/*
	 * Now put the last 'good' half of buffer into storage.
	 */
	strcat(buf, s);
      }
      /*
       * Check for buffer overflow.
       */
      if (strlen(buf) > d->max_str) {
	SEND_TO_Q("Change causes new length to exceed buffer maximum size, aborted.\r\n", d);
	return;
      }
      /*
       * Change the size of the REAL buffer to fit the new text.
       */
      RECREATE(*d->str, char, strlen(buf) + 3);
      strcpy(*d->str, buf);
      SEND_TO_Q("Line changed.\r\n", d);
    } else {
      SEND_TO_Q("Line number must be higher than 0.\r\n", d);
      return;
    }
    break;
  default:
    SEND_TO_Q("Invalid option.\r\n", d);
    mudlog("SYSERR: invalid command passed to parse_action", BRF, LVL_IMPL, TRUE);
    return;
  }
}


/*
 * Re-formats message type formatted char *.
 * (for strings edited with d->str) (mostly olc and mail)
 */
void format_text(char **ptr_string, int mode, struct descriptor_data *d, unsigned int maxlen)
{
  int line_chars, cap_next = TRUE, cap_next_next = FALSE;
  char *flow, *start = NULL, temp;
  char formatted[MAX_STRING_LENGTH];

  /* Fix memory overrun. */
  if (d->max_str > MAX_STRING_LENGTH) {
    log("SYSERR: format_text: max_str is greater than buffer size.");
    return;
  }

  /* XXX: Want to make sure the string doesn't grow either... */

  if ((flow = *ptr_string) == NULL)
    return;

  if (IS_SET(mode, FORMAT_INDENT)) {
    strcpy(formatted, "   ");
    line_chars = 3;
  } else {
    *formatted = '\0';
    line_chars = 0;
  }

  while (*flow) {
    while (*flow && strchr("\n\r\f\t\v ", *flow))
      flow++;

    if (*flow) {
      start = flow++;
      while (*flow && !strchr("\n\r\f\t\v .?!", *flow))
	flow++;

      if (cap_next_next) {
        cap_next_next = FALSE;
        cap_next = TRUE;
      }

      /*
       * This is so that if we stopped on a sentence .. we move off the
       * sentence delimiter.
       */
      while (strchr(".!?", *flow)) {
	cap_next_next = TRUE;
	flow++;
      }

      temp = *flow;
      *flow = '\0';

      if (line_chars + strlen(start) + 1 > 79) {
	strcat(formatted, "\r\n");
	line_chars = 0;
      }

      if (!cap_next) {
	if (line_chars > 0) {
	  strcat(formatted, " ");
	  line_chars++;
	}
      } else {
	cap_next = FALSE;
	*start = UPPER(*start);
      }

      line_chars += strlen(start);
      strcat(formatted, start);

      *flow = temp;
    }

    if (cap_next_next) {
      if (line_chars + 3 > 79) {
	strcat(formatted, "\r\n");
	line_chars = 0;
      } else {
	strcat(formatted, "  ");
	line_chars += 2;
      }
    }
  }
  strcat(formatted, "\r\n");

  if (strlen(formatted) + 1 > maxlen)
    formatted[maxlen - 1] = '\0';
  RECREATE(*ptr_string, char, MIN(maxlen, strlen(formatted) + 1));
  strcpy(*ptr_string, formatted);
}

int replace_str(char **string, char *pattern, char *replacement, int rep_all, unsigned int max_size)
{
  char *replace_buffer = NULL;
  char *flow, *jetsam, temp;
  int len, i;

  if ((strlen(*string) - strlen(pattern)) + strlen(replacement) > max_size)
    return -1;

  CREATE(replace_buffer, char, max_size + ORACLE_SLACK);	/* + ORACLE_SLACK: see below */
  i = 0;
  jetsam = *string;
  flow = *string;
  *replace_buffer = '\0';

  if (rep_all) {
    while ((flow = (char *)strstr(flow, pattern)) != NULL) {
      i++;
      temp = *flow;
      *flow = '\0';
      if ((strlen(replace_buffer) + strlen(jetsam) + strlen(replacement)) > max_size) {
        i = -1;
        break;
      }
      strcat(replace_buffer, jetsam);
      strcat(replace_buffer, replacement);
      *flow = temp;
      flow += strlen(pattern);
      jetsam = flow;
    }
    strcat(replace_buffer, jetsam);
  } else {
    if ((flow = (char *)strstr(*string, pattern)) != NULL) {
      i++;
      flow += strlen(pattern);
      len = ((char *)flow - (char *)*string) - strlen(pattern);
      strncpy(replace_buffer, *string, len);
      strcat(replace_buffer, replacement);
      strcat(replace_buffer, flow);
    }
  }

  if (i <= 0)
    return 0;
  else {
    RECREATE(*string, char, strlen(replace_buffer) + 3);
    strcpy(*string, replace_buffer);
  }
  free(replace_buffer);
  return i;
}

/* --- the harness -------------------------------------------------------- */

/* escape writes a field with \, CR, LF and TAB backslash-escaped, so that a
 * buffer full of line endings survives a tab-separated row. */
static void escape(const char *s)
{
  for (; *s; s++)
    switch (*s) {
    case '\\': fputs("\\\\", stdout); break;
    case '\r': fputs("\\r", stdout); break;
    case '\n': fputs("\\n", stdout); break;
    case '\t': fputs("\\t", stdout); break;
    default:   putchar(*s); break;
    }
}

/* one case: run `line` against a copy of `start` and print the row.
 * start == NULL is the C's own freshly-opened editor, where *d->str is NULL
 * until the first line of text is added (string_add, modify.c:132). */
static void run(const char *start, unsigned int max_str, const char *line)
{
  struct descriptor_data desc;
  char *text = NULL;
  char input[MAX_INPUT_LENGTH];
  int ret;

  if (start != NULL) {
    CREATE(text, char, strlen(start) + 3);
    strcpy(text, start);
  }
  desc.str = &text;
  desc.max_str = max_str;

  /* improved_editor_execute writes through its argument, so it gets a
   * mutable copy, exactly as string_add hands it one. */
  strncpy(input, line, sizeof(input) - 1);
  input[sizeof(input) - 1] = '\0';

  *sent = '\0';
  ret = improved_editor_execute(&desc, input);

  if (start != NULL)
    escape(start);
  else
    fputs("NULL", stdout);
  printf("\t%u\t", max_str);
  escape(line);
  printf("\t%d\t", ret);
  if (text != NULL)
    escape(text);
  else
    fputs("NULL", stdout);
  putchar('\t');
  escape(sent);
  putchar('\n');

  free(text);
}

int main(void)
{
  /* The buffers. Each is what string_add would have built from the lines
   * typed into it: every line ends "\r\n", and the last one does too. */
  static const char *buffers[] = {
    NULL,
    "",
    "One line.\r\n",
    "First line.\r\nSecond line.\r\nThird line.\r\n",
    "alpha\r\nbeta\r\ngamma\r\ndelta\r\nepsilon\r\n",
    "the quick brown fox. jumps over the lazy dog? and then some! more\r\n",
    "  leading spaces\r\n\r\nblank line above\r\n",
    "repeat repeat repeat\r\nrepeat again\r\n",
  };

  /* The command lines. Anything the six new commands can be given, plus the
   * five already ported, so this covers the whole switch. */
  static const char *lines[] = {
    "/h", "/c", "/x", "/", "/a", "/s",
    /* /d */
    "/d1", "/d 1", "/d 2", "/d 3", "/d 4", "/d 5", "/d 0", "/d -1",
    "/d 1-1", "/d 1-2", "/d 2-3", "/d 1-3", "/d 2-99", "/d 1-99",
    "/d 3-2", "/d x", "/d 2 - 3", "/d 3x",
    /* /e */
    "/e", "/e 1 changed", "/e 2 changed", "/e 3 changed", "/e 4 changed",
    "/e 5 changed", "/e 0 changed", "/e 1", "/e x changed", "/e -1 x",
    "/e2 no space", "/e 2 UPPER Case Kept",
    /* /f */
    "/f", "/fi", "/f i", "/fx", "/fii", "/f 1",
    /* /i */
    "/i", "/i 1 inserted", "/i 2 inserted", "/i 3 inserted", "/i 4 inserted",
    "/i 5 inserted", "/i 0 inserted", "/i 1", "/i x inserted", "/i2 no space",
    /* /l and /n */
    "/l", "/n", "/l 1", "/n 1", "/l 2", "/n 2", "/l 4", "/n 4", "/l 5", "/n 5",
    "/l 0", "/n 0", "/l 1-2", "/n 1-2", "/l 2-3", "/n 2-3", "/l 2-99", "/n 2-99",
    "/l 3-2", "/n 3-2", "/l x", "/n x", "/l -1", "/n -1", "/l 3x", "/n 3x",
    /* /r */
    "/r", "/r 'line'", "/r 'line' ", "/r 'line' 'LINE'", "/ra 'line' 'LINE'",
    "/r 'repeat' 'x'", "/ra 'repeat' 'x'", "/r 'nothere' 'x'",
    "/ra 'nothere' 'x'", "/r 'e' 'EEEE'", "/ra 'e' 'EEEE'",
    "/r 'a-very-long-pattern-far-longer-than-the-whole-buffer-is' 'x'",
    "/rx 'line' 'LINE'", "/r no quotes at all", "/r 'line' 'LINE' 'extra'",
  };

  size_t b, l;

  for (b = 0; b < sizeof(buffers) / sizeof(buffers[0]); b++)
    for (l = 0; l < sizeof(lines) / sizeof(lines[0]); l++) {
      const char *line = lines[l];

      /*
       * Skipped: undefined behaviour, not behaviour. `sscanf(string,
       * " %d - %d ", ...)` returns EOF for an argument that is empty or
       * all whitespace, the switch reading it has no default, and
       * line_low/line_high are then uninitialised locals. A bare /d hits
       * that; /l and /n guard with `if (*string)` so only a
       * whitespace-only argument does.
       */
      if (!strcmp(line, "/d") || !strcmp(line, "/d ") ||
	  !strcmp(line, "/l ") || !strcmp(line, "/n "))
	continue;

      /*
       * Skipped for the same reason: /r is the one command with no
       * `if (*(d->str))` guard in improved_editor_execute, and PARSE_REPLACE
       * dereferences the buffer unconditionally — `strlen(*d->str)` in its
       * own space check. On a freshly-opened editor that pointer is still
       * NULL, so `/r 'a' 'b'` as the very first thing typed segfaults the
       * server. This oracle crashed on exactly that before the skip went in.
       */
      if (buffers[b] == NULL && line[1] == 'r')
	continue;

      run(buffers[b], MAX_STRING_LENGTH, line);
    }

  /*
   * The small-max_str cases, where the overflow branches every one of these
   * commands carries are actually reachable. 40 is a little over the
   * three-line buffer's own length.
   */
  {
    static const char *tight[] = {
      "/i 2 xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "/e 2 xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "/e 2 x",
      "/i 2 x",
      "/r 'Second' 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'",
      "/ra 'line' 'xxxxxxxxxxxxxxxxxxxxxx'",
      "/f",
      "/d 2",
    };
    size_t i;
    for (i = 0; i < sizeof(tight) / sizeof(tight[0]); i++) {
      run("First line.\r\nSecond line.\r\nThird line.\r\n", 40, tight[i]);
      run("First line.\r\nSecond line.\r\nThird line.\r\n", 41, tight[i]);
      run("First line.\r\nSecond line.\r\nThird line.\r\n", 42, tight[i]);
    }
  }

  /*
   * replace_str's *second* overflow check, the one inside the rep_all loop
   * (`strlen(replace_buffer) + strlen(jetsam) + strlen(replacement)`).
   * PARSE_REPLACE's own check only budgets for a single replacement, so
   * this is the branch several substitutions at once run into — and it
   * sets i = -1, which the tail then reads as `i <= 0` and reports as
   * "String ... not found." with the buffer untouched.
   */
  {
    static const char *bufr = "repeat repeat repeat\r\nrepeat again\r\n";
    unsigned int m;
    for (m = 36; m <= 60; m += 2) {
      run(bufr, m, "/ra 'repeat' 'xxxxxxx'");
      run(bufr, m, "/r 'repeat' 'xxxxxxx'");
    }
    /* Eight matches, each three characters longer: enough growth for the
     * in-loop check to trip partway through rather than not at all. */
    for (m = 39; m <= 66; m += 3)
      run(bufr, m, "/ra 'e' 'EEEE'");
  }

  /*
   * /f's word wrap, which only shows itself past 79 columns, and its
   * sentence handling, which only shows itself with terminators in the
   * text.
   */
  {
    static const char *wrap[] = {
      "one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen\r\n",
      "a short one. another sentence here! and a third? plus a trailing fragment\r\n",
      "word\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\nword\r\n",
      "supercalifragilisticexpialidociousandthensomemoretomakethiswordfarlongerthanseventynineactualcolumns yes\r\n",
      "  \r\n\r\n   \r\n",
      "trailing spaces   \r\n",
      "...\r\n",
      "ends in a period.\r\n",
    };
    size_t i;
    for (i = 0; i < sizeof(wrap) / sizeof(wrap[0]); i++) {
      run(wrap[i], MAX_STRING_LENGTH, "/f");
      run(wrap[i], MAX_STRING_LENGTH, "/fi");
    }
  }

  return 0;
}
