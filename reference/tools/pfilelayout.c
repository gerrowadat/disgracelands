/*
 * pfilelayout.c - report the on-disk layout of `struct char_file_u`.
 *
 * The binary player database is a raw fwrite() of that struct, so its file
 * format IS the struct's memory layout: every offset, every field width,
 * every byte of padding the compiler chose. Reimplementing a reader for it
 * means reproducing that layout exactly, and getting one offset wrong
 * silently misreads everything after it.
 *
 * Rather than compute those offsets by hand and hope, this program asks the
 * compiler and prints the answer as JSON. The Go decoder is written against
 * that output and a test asserts the two agree, so the layout is checked
 * rather than believed.
 *
 * IMPORTANT: build this 32-bit to get the layout that matters. The real
 * database was written by a 32-bit FreeBSD/i386 build, and the struct
 * contains several `long` fields plus - via struct affected_type - a
 * `next` POINTER, all of which change width under LP64. Building it native
 * on x86-64 prints a different and equally correct layout for a file nobody
 * has.
 *
 * Build (from the repository root):
 *   gcc -m32 -std=gnu89 -fcommon -w \
 *       -Ireference/moderncserver/src \
 *       -o /tmp/pfilelayout reference/tools/pfilelayout.c
 *
 * If -m32 fails for want of 32-bit headers, install them (on Debian and
 * derivatives: gcc-multilib). Building native still works and is still
 * useful - it shows what a modern rebuild would have done to the format -
 * but it is not the layout the archived data is in.
 *
 * Usage:
 *   /tmp/pfilelayout            # JSON to stdout
 */

#include "conf.h"
#include "sysdep.h"
#include "structs.h"
#include <stdio.h>
#include <stddef.h>

/* The struct is nested, so offsets are reported relative to the record. */
#define REC(field) ((long) offsetof(struct char_file_u, field))
#define SZ(field)  ((long) sizeof(((struct char_file_u *) 0)->field))

static int first = 1;

static void field(const char *name, long offset, long size, const char *kind)
{
  if (!first)
    printf(",\n");
  first = 0;
  printf("    {\"name\": \"%s\", \"offset\": %ld, \"size\": %ld, \"kind\": \"%s\"}",
	 name, offset, size, kind);
}

int main(void)
{
  printf("{\n");
  printf("  \"pointer_size\": %ld,\n", (long) sizeof(void *));
  printf("  \"long_size\": %ld,\n", (long) sizeof(long));
  printf("  \"time_t_size\": %ld,\n", (long) sizeof(time_t));
  printf("  \"record_size\": %ld,\n", (long) sizeof(struct char_file_u));
  printf("  \"constants\": {\n");
  printf("    \"MAX_NAME_LENGTH\": %d,\n", MAX_NAME_LENGTH);
  printf("    \"EXDSCR_LENGTH\": %d,\n", EXDSCR_LENGTH);
  printf("    \"MAX_TITLE_LENGTH\": %d,\n", MAX_TITLE_LENGTH);
  printf("    \"MAX_PWD_LENGTH\": %d,\n", MAX_PWD_LENGTH);
  printf("    \"HOST_LENGTH\": %d,\n", HOST_LENGTH);
  printf("    \"MAX_SKILLS\": %d,\n", MAX_SKILLS);
  printf("    \"MAX_AFFECT\": %d,\n", MAX_AFFECT);
  printf("    \"MAX_TONGUE\": %d\n", MAX_TONGUE);
  printf("  },\n");
  printf("  \"fields\": [\n");

  /* char_player_data, inlined into the record */
  field("name",        REC(name),        SZ(name),        "char[]");
  field("description", REC(description), SZ(description), "char[]");
  field("title",       REC(title),       SZ(title),       "char[]");
  field("sex",         REC(sex),         SZ(sex),         "byte");
  field("chclass",     REC(chclass),     SZ(chclass),     "byte");
  field("level",       REC(level),       SZ(level),       "byte");
  field("hometown",    REC(hometown),    SZ(hometown),    "sh_int");
  field("birth",       REC(birth),       SZ(birth),       "time_t");
  field("played",      REC(played),      SZ(played),      "int");
  field("weight",      REC(weight),      SZ(weight),      "ubyte");
  field("height",      REC(height),      SZ(height),      "ubyte");
  field("pwd",         REC(pwd),         SZ(pwd),         "char[]");

  /* char_special_data_saved */
  field("cs.alignment",         REC(char_specials_saved.alignment),         SZ(char_specials_saved.alignment),         "int");
  field("cs.idnum",             REC(char_specials_saved.idnum),             SZ(char_specials_saved.idnum),             "long");
  field("cs.act",               REC(char_specials_saved.act),               SZ(char_specials_saved.act),               "long");
  field("cs.affected_by",       REC(char_specials_saved.affected_by),       SZ(char_specials_saved.affected_by),       "long");
  field("cs.apply_saving_throw", REC(char_specials_saved.apply_saving_throw), SZ(char_specials_saved.apply_saving_throw), "sh_int[]");

  /* player_special_data_saved */
  field("ps.skills",        REC(player_specials_saved.skills),        SZ(player_specials_saved.skills),        "byte[]");
  field("ps.PADDING0",      REC(player_specials_saved.PADDING0),      SZ(player_specials_saved.PADDING0),      "byte");
  field("ps.talks",         REC(player_specials_saved.talks),         SZ(player_specials_saved.talks),         "bool[]");
  field("ps.wimp_level",    REC(player_specials_saved.wimp_level),    SZ(player_specials_saved.wimp_level),    "int");
  field("ps.freeze_level",  REC(player_specials_saved.freeze_level),  SZ(player_specials_saved.freeze_level),  "byte");
  field("ps.invis_level",   REC(player_specials_saved.invis_level),   SZ(player_specials_saved.invis_level),   "sh_int");
  field("ps.load_room",     REC(player_specials_saved.load_room),     SZ(player_specials_saved.load_room),     "room_vnum");
  field("ps.pref",          REC(player_specials_saved.pref),          SZ(player_specials_saved.pref),          "long");
  field("ps.bad_pws",       REC(player_specials_saved.bad_pws),       SZ(player_specials_saved.bad_pws),       "ubyte");
  field("ps.conditions",    REC(player_specials_saved.conditions),    SZ(player_specials_saved.conditions),    "sbyte[]");
  field("ps.spare0",        REC(player_specials_saved.spare0),        SZ(player_specials_saved.spare0),        "ubyte");
  field("ps.spare1",        REC(player_specials_saved.spare1),        SZ(player_specials_saved.spare1),        "ubyte");
  field("ps.spare2",        REC(player_specials_saved.spare2),        SZ(player_specials_saved.spare2),        "ubyte");
  field("ps.spare3",        REC(player_specials_saved.spare3),        SZ(player_specials_saved.spare3),        "ubyte");
  field("ps.spare4",        REC(player_specials_saved.spare4),        SZ(player_specials_saved.spare4),        "ubyte");
  field("ps.spare5",        REC(player_specials_saved.spare5),        SZ(player_specials_saved.spare5),        "ubyte");
  field("ps.spells_to_learn", REC(player_specials_saved.spells_to_learn), SZ(player_specials_saved.spells_to_learn), "int");
  field("ps.remort_vector", REC(player_specials_saved.remort_vector), SZ(player_specials_saved.remort_vector), "int");
  field("ps.specflags",     REC(player_specials_saved.specflags),     SZ(player_specials_saved.specflags),     "int");
  field("ps.olc_zone",      REC(player_specials_saved.olc_zone),      SZ(player_specials_saved.olc_zone),      "int");
  field("ps.spare10",       REC(player_specials_saved.spare10),       SZ(player_specials_saved.spare10),       "int");
  field("ps.spare11",       REC(player_specials_saved.spare11),       SZ(player_specials_saved.spare11),       "int");
  field("ps.spare12",       REC(player_specials_saved.spare12),       SZ(player_specials_saved.spare12),       "int");
  field("ps.spare13",       REC(player_specials_saved.spare13),       SZ(player_specials_saved.spare13),       "int");
  field("ps.spare14",       REC(player_specials_saved.spare14),       SZ(player_specials_saved.spare14),       "int");
  field("ps.spare15",       REC(player_specials_saved.spare15),       SZ(player_specials_saved.spare15),       "int");
  field("ps.spare16",       REC(player_specials_saved.spare16),       SZ(player_specials_saved.spare16),       "int");
  field("ps.spare17",       REC(player_specials_saved.spare17),       SZ(player_specials_saved.spare17),       "long");
  field("ps.spare18",       REC(player_specials_saved.spare18),       SZ(player_specials_saved.spare18),       "long");
  field("ps.spare19",       REC(player_specials_saved.spare19),       SZ(player_specials_saved.spare19),       "long");
  field("ps.spare20",       REC(player_specials_saved.spare20),       SZ(player_specials_saved.spare20),       "long");
  field("ps.spare21",       REC(player_specials_saved.spare21),       SZ(player_specials_saved.spare21),       "long");

  /* char_ability_data */
  field("ab.str",     REC(abilities.str),     SZ(abilities.str),     "sbyte");
  field("ab.str_add", REC(abilities.str_add), SZ(abilities.str_add), "sbyte");
  field("ab.intel",   REC(abilities.intel),   SZ(abilities.intel),   "sbyte");
  field("ab.wis",     REC(abilities.wis),     SZ(abilities.wis),     "sbyte");
  field("ab.dex",     REC(abilities.dex),     SZ(abilities.dex),     "sbyte");
  field("ab.con",     REC(abilities.con),     SZ(abilities.con),     "sbyte");
  field("ab.cha",     REC(abilities.cha),     SZ(abilities.cha),     "sbyte");

  /* char_point_data */
  field("pt.mana",      REC(points.mana),      SZ(points.mana),      "sh_int");
  field("pt.max_mana",  REC(points.max_mana),  SZ(points.max_mana),  "sh_int");
  field("pt.hit",       REC(points.hit),       SZ(points.hit),       "sh_int");
  field("pt.max_hit",   REC(points.max_hit),   SZ(points.max_hit),   "sh_int");
  field("pt.move",      REC(points.move),      SZ(points.move),      "sh_int");
  field("pt.max_move",  REC(points.max_move),  SZ(points.max_move),  "sh_int");
  field("pt.armor",     REC(points.armor),     SZ(points.armor),     "sh_int");
  field("pt.gold",      REC(points.gold),      SZ(points.gold),      "int");
  field("pt.bank_gold", REC(points.bank_gold), SZ(points.bank_gold), "int");
  field("pt.exp",       REC(points.exp),       SZ(points.exp),       "int");
  field("pt.hitroll",   REC(points.hitroll),   SZ(points.hitroll),   "sbyte");
  field("pt.damroll",   REC(points.damroll),   SZ(points.damroll),   "sbyte");

  /*
   * affected[] is an array of struct affected_type, which ends in a `next`
   * POINTER. That pointer is written to disk along with everything else -
   * it is meaningless data, but it occupies space and its width is part of
   * the format. Reporting the element stride explicitly is the point.
   */
  field("affected",         REC(affected),         SZ(affected),         "affected_type[]");
  field("affected.0.type",      REC(affected[0].type),      SZ(affected[0].type),      "sh_int");
  field("affected.0.duration",  REC(affected[0].duration),  SZ(affected[0].duration),  "sh_int");
  field("affected.0.modifier",  REC(affected[0].modifier),  SZ(affected[0].modifier),  "sbyte");
  field("affected.0.location",  REC(affected[0].location),  SZ(affected[0].location),  "byte");
  field("affected.0.bitvector", REC(affected[0].bitvector), SZ(affected[0].bitvector), "long");
  field("affected.0.next",      REC(affected[0].next),      SZ(affected[0].next),      "pointer");
  field("affected.1.type",      REC(affected[1].type),      SZ(affected[1].type),      "sh_int");

  field("last_logon", REC(last_logon), SZ(last_logon), "time_t");
  field("host",       REC(host),       SZ(host),       "char[]");

  printf("\n  ]\n}\n");
  return (0);
}
