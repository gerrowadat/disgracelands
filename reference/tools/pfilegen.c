/*
 * pfilegen.c - write a synthetic binary player database with known values.
 *
 * The Go decoder in internal/persist/player/binary has to read a file written
 * by a C program doing fwrite() on `struct char_file_u`. The archived database
 * is exactly that, but it is not in this repository and never will be - it is
 * real people's password hashes and private mail. This program stands in for
 * it: it writes records whose every field holds a value derived from the
 * record index, so a reader can be checked field by field against arithmetic
 * rather than against a fixture nobody can inspect.
 *
 * It is the same struct, filled by the same compiler, so it exercises the
 * real layout including every byte of padding.
 *
 * Build 32-bit to produce the format the archived data is in:
 *   gcc -m32 -std=gnu89 -fcommon -w -Ireference/moderncserver/src \
 *       -o /tmp/pfilegen reference/tools/pfilegen.c
 *
 * Build native to produce what a modern rebuild would write:
 *   gcc -std=gnu89 -fcommon -w -Ireference/moderncserver/src \
 *       -o /tmp/pfilegen reference/tools/pfilegen.c
 *
 * Usage:
 *   pfilegen <output-file> <record-count>
 *
 * The value in every field is a documented function of the record index i;
 * see fill_record() below, and keep it in step with the Go test that reads it
 * (codec_test.go's wantRecord).
 */

#include "conf.h"
#include "sysdep.h"
#include "structs.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static void fill_record(struct char_file_u *r, int i)
{
  int j;

  /*
   * Deliberately memset to a non-zero byte first. The real files carry
   * whatever was in the struct's padding, and a reader that accidentally
   * depends on padding being zero would pass against a zeroed fixture and
   * fail against real data.
   */
  memset(r, 0xAB, sizeof(*r));

  sprintf(r->name, "Test%d", i);
  sprintf(r->title, "the Tester of %d", i);
  sprintf(r->description, "A test character, number %d.\n", i);
  sprintf(r->pwd, "ab%06d", i);          /* shaped like a crypt(3) hash */
  sprintf(r->host, "host%d.example.org", i);

  r->sex = (byte) (i % 3);
  r->chclass = (byte) (i % 4);
  r->level = (byte) (i % 34 + 1);
  r->hometown = (sh_int) (3000 + i);
  r->birth = (time_t) (1000000000 + i);
  r->played = 3600 * i;
  r->weight = (ubyte) (100 + i % 100);
  r->height = (ubyte) (150 + i % 50);
  r->last_logon = (time_t) (1100000000 + i);

  r->char_specials_saved.alignment = (i % 2) ? 1000 - i : -1000 + i;
  r->char_specials_saved.idnum = 1 + i;
  r->char_specials_saved.act = 1L << (i % 20);
  r->char_specials_saved.affected_by = 1L << (i % 15);
  for (j = 0; j < 5; j++)
    r->char_specials_saved.apply_saving_throw[j] = (sh_int) (i * 10 + j);

  /* Only a few skills, so the reader's "drop the zeroes" behaviour shows. */
  for (j = 0; j < MAX_SKILLS + 1; j++)
    r->player_specials_saved.skills[j] = 0;
  r->player_specials_saved.skills[1] = (byte) (50 + i % 50);
  r->player_specials_saved.skills[201 - 1] = (byte) (i % 100);

  r->player_specials_saved.PADDING0 = 0;
  for (j = 0; j < MAX_TONGUE; j++)
    r->player_specials_saved.talks[j] = 0;
  r->player_specials_saved.wimp_level = i;
  r->player_specials_saved.freeze_level = (byte) (i % 5);
  r->player_specials_saved.invis_level = (sh_int) (i % 35);
  r->player_specials_saved.load_room = (room_vnum) (3001 + i);
  r->player_specials_saved.pref = 1L << (i % 22);
  r->player_specials_saved.bad_pws = (ubyte) (i % 4);
  r->player_specials_saved.conditions[0] = (sbyte) (i % 25);
  r->player_specials_saved.conditions[1] = (sbyte) -1;
  r->player_specials_saved.conditions[2] = (sbyte) (i % 25);

  r->player_specials_saved.spare0 = (ubyte) i;
  r->player_specials_saved.spare1 = (ubyte) (i + 1);
  r->player_specials_saved.spare2 = (ubyte) (i + 2);
  r->player_specials_saved.spare3 = (ubyte) (i + 3);
  r->player_specials_saved.spare4 = (ubyte) (i + 4);
  r->player_specials_saved.spare5 = (ubyte) (i + 5);
  r->player_specials_saved.spells_to_learn = i * 2;
  r->player_specials_saved.remort_vector = i % 16;
  r->player_specials_saved.specflags = i * 3;
  r->player_specials_saved.olc_zone = 30 + i;
  r->player_specials_saved.spare10 = i * 10;
  r->player_specials_saved.spare11 = i * 11;
  r->player_specials_saved.spare12 = i * 12;
  r->player_specials_saved.spare13 = i * 13;
  r->player_specials_saved.spare14 = i * 14;
  r->player_specials_saved.spare15 = i * 15;
  r->player_specials_saved.spare16 = i * 16;
  r->player_specials_saved.spare17 = i * 17;
  r->player_specials_saved.spare18 = i * 18;
  r->player_specials_saved.spare19 = i * 19;
  r->player_specials_saved.spare20 = i * 20;
  r->player_specials_saved.spare21 = i * 21;

  r->abilities.str = (sbyte) (10 + i % 9);
  r->abilities.str_add = (sbyte) (i % 101);
  r->abilities.intel = (sbyte) (10 + i % 9);
  r->abilities.wis = (sbyte) (10 + i % 9);
  r->abilities.dex = (sbyte) (10 + i % 9);
  r->abilities.con = (sbyte) (10 + i % 9);
  r->abilities.cha = (sbyte) (10 + i % 9);

  r->points.mana = (sh_int) (100 + i);
  r->points.max_mana = (sh_int) (100 + i);
  r->points.hit = (sh_int) (50 + i);
  r->points.max_hit = (sh_int) (50 + i);
  r->points.move = (sh_int) (80 + i);
  r->points.max_move = (sh_int) (80 + i);
  r->points.armor = (sh_int) (-i);
  r->points.gold = i * 1000;
  r->points.bank_gold = i * 2000;
  r->points.exp = i * 100000;
  r->points.hitroll = (sbyte) (i % 20);
  r->points.damroll = (sbyte) (i % 20);

  /* Two affects, so the "stop at an empty slot" rule is exercised. */
  for (j = 0; j < MAX_AFFECT; j++) {
    r->affected[j].type = 0;
    r->affected[j].duration = 0;
    r->affected[j].modifier = 0;
    r->affected[j].location = 0;
    r->affected[j].bitvector = 0;
    r->affected[j].next = NULL;
  }
  r->affected[0].type = (sh_int) (1 + i % 50);
  r->affected[0].duration = (sh_int) (10 + i);
  r->affected[0].modifier = (sbyte) (i % 10);
  r->affected[0].location = (byte) (i % 20);
  r->affected[0].bitvector = 1L << (i % 15);
  r->affected[1].type = (sh_int) (51 + i % 10);
  r->affected[1].duration = (sh_int) (20 + i);
  r->affected[1].modifier = (sbyte) -(i % 10);
  r->affected[1].location = (byte) (i % 5);
  r->affected[1].bitvector = 0;
}

int main(int argc, char **argv)
{
  struct char_file_u rec;
  FILE *f;
  int count, i;

  if (argc != 3) {
    fprintf(stderr, "usage: %s <output-file> <record-count>\n", argv[0]);
    return (1);
  }
  count = atoi(argv[2]);
  if (count < 1) {
    fprintf(stderr, "%s: record count must be positive\n", argv[0]);
    return (1);
  }
  if (!(f = fopen(argv[1], "wb"))) {
    perror("fopen");
    return (1);
  }

  for (i = 0; i < count; i++) {
    fill_record(&rec, i);
    if (fwrite(&rec, sizeof(rec), 1, f) != 1) {
      perror("fwrite");
      fclose(f);
      return (1);
    }
  }

  fclose(f);
  fprintf(stderr, "%s: wrote %d records of %lu bytes\n",
	  argv[0], count, (unsigned long) sizeof(struct char_file_u));
  return (0);
}
