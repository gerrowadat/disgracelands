/*
 * boardlayout.c -- the on-disk layout of a bulletin board file.
 *
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * Board_save_board (boards.c:416) fwrites a message count and then, per
 * message, a raw `struct board_msginfo` followed by the heading and body
 * bytes. So the board file format is that struct's memory layout, the same
 * way the player database is `struct char_file_u`'s.
 *
 * The wrinkle is the second member:
 *
 *     struct board_msginfo {
 *        int   slot_num;
 *        char *heading;
 *        int   level;
 *        int   heading_len;
 *        int   message_len;
 *     };
 *
 * A live pointer, written to disk. Its value is meaningless the moment the
 * process exits and Board_load_board ignores it — but its *width* is not
 * ignored, because everything after it moves. On the i386 build the archive
 * came from it is four bytes and the struct is 20; on any 64-bit rebuild it
 * is eight, the struct is 24, and every board file becomes unreadable.
 *
 * Prints one "name offset size" line per member, then the total.
 */

#include <stdio.h>
#include <stddef.h>
#include <stdlib.h>

struct board_msginfo {
   int	slot_num;
   char	*heading;
   int	level;
   int	heading_len;
   int	message_len;
};

#define SHOW(f) printf("%s %zu %zu\n", #f, offsetof(struct board_msginfo, f), \
		       sizeof(((struct board_msginfo *) 0)->f))

int main(void)
{
  printf("# pointer %zu\n", sizeof(char *));
  SHOW(slot_num);
  SHOW(heading);
  SHOW(level);
  SHOW(heading_len);
  SHOW(message_len);
  printf("sizeof %zu\n", sizeof(struct board_msginfo));
  printf("count %zu\n", sizeof(int));

  return (EXIT_SUCCESS);
}
