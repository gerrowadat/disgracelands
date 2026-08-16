/* ************************************************************************
*   File: worlddump.c                                                     *
*  Usage: Dump the loaded world as canonical JSON, then exit.             *
*                                                                         *
*  Not part of stock CircleMUD. Written for the Go port (see              *
*  docs/proposals/go-port-plan.md) so that "the Go loader agrees with the  *
*  C loader" is a thing you can run rather than a thing you assert:        *
*  both servers dump the same format and the two files are diffed.        *
*                                                                         *
*  This file only reads the loaded world. It must not change any of it -   *
*  the whole point is to report what the real boot sequence produced.      *
*                                                                         *
*  Keep in step with internal/persist/world/dump.go. Where the two         *
*  disagree, the diff will be full of noise and prove nothing.            *
*                                                                         *
*  Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative  *
*  work of CircleMUD, Copyright (C) 1993, 94 by the Trustees of the Johns *
*  Hopkins University. CircleMUD is based on DikuMUD, Copyright (C) 1990, *
*  1991.                                                                  *
*                                                                         *
*  All rights reserved. See doc/license.doc, and LICENSE at the           *
*  repository root, for complete information - including the requirement  *
*  that this work not be used commercially.                               *
************************************************************************ */

#include "conf.h"
#include "sysdep.h"

#include "structs.h"
#include "utils.h"
#include "db.h"
#include "shop.h"

/* From db.c */
extern struct room_data *world;
extern room_rnum top_of_world;
extern struct char_data *mob_proto;
extern struct index_data *mob_index;
extern mob_rnum top_of_mobt;
extern struct obj_data *obj_proto;
extern struct index_data *obj_index;
extern obj_rnum top_of_objt;
extern struct zone_data *zone_table;
extern zone_rnum top_of_zone_table;

/* From shop.c */
extern struct shop_data *shop_index;
extern int top_shop;

void dump_world_json(FILE *f);

/*
 * Render one byte as JSON.
 *
 * Every byte outside printable ASCII becomes \u00XX of its own value, so the
 * output is pure ASCII and byte-exact. This matters more than it looks: the
 * world files are not UTF-8 (lib/world/wld/90.wld holds 0x92, a CP1252
 * apostrophe) and any encoder that "fixes" such bytes would make two
 * different corrupt bytes compare equal.
 *
 * Must produce identical output to escapeByte() in
 * internal/persist/world/jsonstring.go.
 */
static void json_escape(FILE *f, const char *s)
{
  const unsigned char *p;

  fputc('"', f);
  if (s != NULL) {
    for (p = (const unsigned char *) s; *p; p++) {
      switch (*p) {
      case '"':  fputs("\\\"", f); break;
      case '\\': fputs("\\\\", f); break;
      case '\n': fputs("\\n", f);  break;
      case '\r': fputs("\\r", f);  break;
      case '\t': fputs("\\t", f);  break;
      case '\b': fputs("\\b", f);  break;
      case '\f': fputs("\\f", f);  break;
      default:
	if (*p < 0x20 || *p > 0x7e)
	  fprintf(f, "\\u%04x", (unsigned int) *p);
	else
	  fputc(*p, f);
	break;
      }
    }
  }
  fputc('"', f);
}

/*
 * Render a bitvector in the letter encoding sprintbits() uses: one letter per
 * set bit in bit order, or the literal "0" when nothing is set, because an
 * empty field would break the reader.
 *
 * Must match Flags.String() in internal/game/flags.go.
 */
static void json_flags(FILE *f, bitvector_t flags)
{
  int bit, any = 0;

  fputc('"', f);
  if (flags == 0)
    fputc('0', f);
  else
    for (bit = 0; bit < 64; bit++)
      if (flags & (((bitvector_t) 1) << bit)) {
	any = 1;
	if (bit < 26)
	  fputc('a' + bit, f);
	else if (bit < 52)
	  fputc('A' + bit - 26, f);
	else
	  fprintf(f, "<bit%d>", bit);
      }
  if (flags != 0 && !any)
    fputc('0', f);
  fputc('"', f);
}

/* Convert a real number back to the vnum it came from, or -1. */
static int room_vnum_of(room_rnum rnum)
{
  if (rnum == NOWHERE || rnum < 0 || rnum > top_of_world)
    return (-1);
  return (world[rnum].number);
}

static int mob_vnum_of(mob_rnum rnum)
{
  if (rnum == NOBODY || rnum < 0 || rnum > top_of_mobt)
    return (-1);
  return (mob_index[rnum].vnum);
}

static int obj_vnum_of(obj_rnum rnum)
{
  if (rnum == NOTHING || rnum < 0 || rnum > top_of_objt)
    return (-1);
  return (obj_index[rnum].vnum);
}

static void dump_extra_descs(FILE *f, struct extra_descr_data *list)
{
  struct extra_descr_data *e;
  int first = 1;

  fputs("[", f);
  /*
   * The loader builds this list by prepending, so walking it forwards yields
   * reverse file order - which is exactly what the running server sees, and
   * what the Go loader reverses its slice to reproduce.
   */
  for (e = list; e; e = e->next) {
    if (!first)
      fputs(",", f);
    first = 0;
    fputs("{\"keywords\":", f);
    json_escape(f, e->keyword);
    fputs(",\"desc\":", f);
    json_escape(f, e->description);
    fputs("}", f);
  }
  fputs("]", f);
}

static void dump_rooms(FILE *f)
{
  room_rnum nr;
  int door, first = 1;

  fputs("\"rooms\":[", f);
  for (nr = 0; nr <= top_of_world; nr++) {
    if (!first)
      fputs(",", f);
    first = 0;

    fprintf(f, "{\"vnum\":%d,\"zone\":%d,\"name\":",
	    world[nr].number, zone_table[world[nr].zone].number);
    json_escape(f, world[nr].name);
    fputs(",\"desc\":", f);
    json_escape(f, world[nr].description);
    fputs(",\"flags\":", f);
    json_flags(f, world[nr].room_flags);
    fprintf(f, ",\"flag_bits\":%lu,\"sector\":%d,\"exits\":[",
	    (unsigned long) world[nr].room_flags, world[nr].sector_type);

    for (door = 0; door < NUM_OF_DIRS; door++) {
      static const char *dirnames[] = {
	"north", "east", "south", "west", "up", "down"
      };

      if (door > 0)
	fputs(",", f);
      if (world[nr].dir_option[door] == NULL) {
	fputs("null", f);
	continue;
      }
      fprintf(f, "{\"dir\":\"%s\",\"desc\":", dirnames[door]);
      json_escape(f, world[nr].dir_option[door]->general_description);
      fputs(",\"keywords\":", f);
      json_escape(f, world[nr].dir_option[door]->keyword);
      /*
       * exit_info is EX_ISDOOR / EX_ISDOOR|EX_PICKPROOF / 0 by the time we
       * see it; map it back to the file's 0/1/2 so both dumps agree on the
       * source value rather than on one loader's encoding of it.
       */
      fprintf(f, ",\"door_flag\":%d,\"key\":%d,\"to_room\":%d}",
	      IS_SET(world[nr].dir_option[door]->exit_info, EX_PICKPROOF) ? 2 :
	      IS_SET(world[nr].dir_option[door]->exit_info, EX_ISDOOR) ? 1 : 0,
	      world[nr].dir_option[door]->key,
	      room_vnum_of(world[nr].dir_option[door]->to_room));
    }

    fputs("],\"extra_descs\":", f);
    dump_extra_descs(f, world[nr].ex_description);
    fputs("}", f);
  }
  fputs("]", f);
}

static void dump_mobiles(FILE *f)
{
  mob_rnum nr;
  int first = 1;

  fputs("\"mobiles\":[", f);
  for (nr = 0; nr <= top_of_mobt; nr++) {
    struct char_data *m = mob_proto + nr;

    if (!first)
      fputs(",", f);
    first = 0;

    fprintf(f, "{\"vnum\":%d,\"keywords\":", mob_index[nr].vnum);
    json_escape(f, m->player.name);
    fputs(",\"short_desc\":", f);
    json_escape(f, m->player.short_descr);
    fputs(",\"long_desc\":", f);
    json_escape(f, m->player.long_descr);
    fputs(",\"desc\":", f);
    json_escape(f, m->player.description);

    /*
     * MOB_ISNPC is dumped as-is. The loader force-sets it on every mobile,
     * and the Go loader does the same, so masking it off here would only
     * hide a flag both servers really hold - and would differ from the file
     * for the many mobs that list it explicitly anyway.
     */
    fputs(",\"act_flags\":", f);
    json_flags(f, MOB_FLAGS(m));
    fprintf(f, ",\"act_bits\":%lu", (unsigned long) MOB_FLAGS(m));
    fputs(",\"aff_flags\":", f);
    json_flags(f, AFF_FLAGS(m));
    fprintf(f, ",\"aff_bits\":%lu", (unsigned long) AFF_FLAGS(m));

    /*
     * The simple/enhanced distinction is not retained anywhere in the loaded
     * mob: parse_enhanced_mob() consumes the E block and folds its effects
     * into ordinary fields. Dumped as null rather than guessed at, and the Go
     * dumper's --parity mode omits it for the same reason.
     */
    fprintf(f, ",\"alignment\":%d,\"enhanced\":null", GET_ALIGNMENT(m));

    fprintf(f, ",\"level\":%d,\"thac0\":%d,\"hitroll\":%d",
	    GET_LEVEL(m), 20 - GET_HITROLL(m), GET_HITROLL(m));
    fprintf(f, ",\"ac\":%d,\"ac_scaled\":%d", GET_AC(m) / 10, GET_AC(m));
    fprintf(f, ",\"hit_dice\":\"%dd%d+%d\"", GET_HIT(m), GET_MANA(m), GET_MOVE(m));
    fprintf(f, ",\"damage_dice\":\"%dd%d+%d\"",
	    m->mob_specials.damnodice, m->mob_specials.damsizedice, GET_DAMROLL(m));
    fprintf(f, ",\"gold\":%d,\"exp\":%d", GET_GOLD(m), GET_EXP(m));
    fprintf(f, ",\"position\":%d,\"default_position\":%d,\"sex\":%d",
	    GET_POS(m), GET_DEFAULT_POS(m), GET_SEX(m));
    /*
     * Especs are interpreted into ordinary fields by interpret_espec() and
     * the original key/value lines are not kept, so there is nothing to dump.
     * The Go dumper suppresses them too; comparing them needs the espec
     * interpretation itself to be ported first.
     */
    fputs(",\"especs\":[]}", f);
  }
  fputs("]", f);
}

static void dump_objects(FILE *f)
{
  obj_rnum nr;
  int i, first = 1;

  fputs("\"objects\":[", f);
  for (nr = 0; nr <= top_of_objt; nr++) {
    struct obj_data *o = obj_proto + nr;
    int naffects = 0;

    if (!first)
      fputs(",", f);
    first = 0;

    fprintf(f, "{\"vnum\":%d,\"keywords\":", obj_index[nr].vnum);
    json_escape(f, o->name);
    fputs(",\"short_desc\":", f);
    json_escape(f, o->short_description);
    fputs(",\"desc\":", f);
    json_escape(f, o->description);
    fputs(",\"action_desc\":", f);
    json_escape(f, o->action_description);

    fprintf(f, ",\"type\":%d,\"extra_flags\":", GET_OBJ_TYPE(o));
    json_flags(f, GET_OBJ_EXTRA(o));
    fprintf(f, ",\"extra_bits\":%lu,\"wear_flags\":", (unsigned long) GET_OBJ_EXTRA(o));
    json_flags(f, GET_OBJ_WEAR(o));
    fprintf(f, ",\"wear_bits\":%lu,\"perm_affect\":%d",
	    (unsigned long) GET_OBJ_WEAR(o), (int) GET_OBJ_PERM(o));

    fputs(",\"values\":[", f);
    for (i = 0; i < 4; i++)
      fprintf(f, "%s%d", i ? "," : "", GET_OBJ_VAL(o, i));
    fputs("]", f);

    fprintf(f, ",\"weight\":%d,\"cost\":%d,\"rent_per_day\":%d,\"min_level\":%d",
	    GET_OBJ_WEIGHT(o), GET_OBJ_COST(o), GET_OBJ_RENT(o), GET_OBJ_LEVEL(o));

    /*
     * Unused affect slots are APPLY_NONE with a zero modifier. The file only
     * had as many 'A' lines as there are real affects, so trailing empty
     * slots are dropped to match what the Go loader stored.
     */
    fputs(",\"affects\":[", f);
    for (i = 0; i < MAX_OBJ_AFFECT; i++)
      if (o->affected[i].location != APPLY_NONE || o->affected[i].modifier != 0) {
	fprintf(f, "%s{\"Location\":%d,\"Modifier\":%d}", naffects ? "," : "",
		o->affected[i].location, o->affected[i].modifier);
	naffects++;
      }
    fputs("],\"extra_descs\":", f);
    dump_extra_descs(f, o->ex_description);
    fputs("}", f);
  }
  fputs("]", f);
}

static void dump_zones(FILE *f)
{
  zone_rnum nr;
  int cmd, first = 1;

  fputs("\"zones\":[", f);
  for (nr = 0; nr <= top_of_zone_table; nr++) {
    if (!first)
      fputs(",", f);
    first = 0;

    fprintf(f, "{\"vnum\":%d,\"name\":", zone_table[nr].number);
    json_escape(f, zone_table[nr].name);
    fprintf(f, ",\"bottom\":%d,\"top\":%d,\"lifespan\":%d,\"reset_mode\":%d,\"commands\":[",
	    zone_table[nr].bot, zone_table[nr].top,
	    zone_table[nr].lifespan, zone_table[nr].reset_mode);

    for (cmd = 0; zone_table[nr].cmd[cmd].command != 'S'; cmd++) {
      struct reset_com *z = &zone_table[nr].cmd[cmd];
      int a1 = z->arg1, a2 = z->arg2, a3 = z->arg3;
      int has_arg3 = 0;

      if (cmd > 0)
	fputs(",", f);

      /*
       * A command renum_zone_table() disabled is dumped as its disabled self
       * and nothing more. Two things are lost when it rewrites the opcode to
       * '*': which command it used to be, and therefore how many arguments it
       * took; and the arguments themselves, which were partly overwritten
       * with real numbers before the failure was noticed. Neither side can
       * reconstruct them, so both sides dump nulls and the comparison stays
       * meaningful - the fact that the command is dead is the part that
       * matters.
       */
      if (z->command == '*') {
	fprintf(f, "{\"command\":\"*\",\"disabled\":true,\"if_flag\":%d,"
		"\"arg1\":null,\"arg2\":null,\"arg3\":null}", z->if_flag);
	continue;
      }

      /* Arguments are real numbers by now; convert each back to its vnum. */
      switch (z->command) {
      case 'M': a1 = mob_vnum_of(a1); a3 = room_vnum_of(a3); has_arg3 = 1; break;
      case 'O': a1 = obj_vnum_of(a1); a3 = room_vnum_of(a3); has_arg3 = 1; break;
      case 'G': a1 = obj_vnum_of(a1); break;
      case 'E': a1 = obj_vnum_of(a1); has_arg3 = 1; break;
      case 'P': a1 = obj_vnum_of(a1); a3 = obj_vnum_of(a3); has_arg3 = 1; break;
      case 'D': a1 = room_vnum_of(a1); has_arg3 = 1; break;
      case 'R': a1 = room_vnum_of(a1); a2 = obj_vnum_of(a2); break;
      default: break;
      }

      fprintf(f, "{\"command\":\"%c\",\"disabled\":false,\"if_flag\":%d,\"arg1\":%d,\"arg2\":%d,\"arg3\":",
	      z->command, z->if_flag, a1, a2);
      if (has_arg3)
	fprintf(f, "%d}", a3);
      else
	fputs("null}", f);
    }
    fputs("]}", f);
  }
  fputs("]", f);
}

static void dump_shops(FILE *f)
{
  int nr, i, first = 1;

  fputs("\"shops\":[", f);
  for (nr = 0; nr <= top_shop; nr++) {
    if (!first)
      fputs(",", f);
    first = 0;

    fprintf(f, "{\"vnum\":%d,\"producing\":[", SHOP_NUM(nr));
    for (i = 0; SHOP_PRODUCT(nr, i) != NOTHING; i++)
      fprintf(f, "%s%d", i ? "," : "", obj_vnum_of(SHOP_PRODUCT(nr, i)));
    fputs("]", f);

    fprintf(f, ",\"profit_buy\":\"%.6f\",\"profit_sell\":\"%.6f\"",
	    SHOP_BUYPROFIT(nr), SHOP_SELLPROFIT(nr));

    fputs(",\"buy_types\":[", f);
    for (i = 0; SHOP_BUYTYPE(nr, i) != NOTHING; i++) {
      fprintf(f, "%s{\"type\":%d,\"keyword\":", i ? "," : "", SHOP_BUYTYPE(nr, i));
      json_escape(f, SHOP_BUYWORD(nr, i));
      fputs("}", f);
    }
    fputs("]", f);

    fputs(",\"messages\":[", f);
    json_escape(f, shop_index[nr].no_such_item1);   fputs(",", f);
    json_escape(f, shop_index[nr].no_such_item2);   fputs(",", f);
    json_escape(f, shop_index[nr].do_not_buy);      fputs(",", f);
    json_escape(f, shop_index[nr].missing_cash1);   fputs(",", f);
    json_escape(f, shop_index[nr].missing_cash2);   fputs(",", f);
    json_escape(f, shop_index[nr].message_buy);     fputs(",", f);
    json_escape(f, shop_index[nr].message_sell);
    fputs("]", f);

    fprintf(f, ",\"temper\":%d,\"flags\":", SHOP_BROKE_TEMPER(nr));
    json_flags(f, SHOP_BITVECTOR(nr));
    fprintf(f, ",\"flag_bits\":%lu", (unsigned long) SHOP_BITVECTOR(nr));
    fprintf(f, ",\"keeper\":%d,\"trade_with\":%d,\"rooms\":[",
	    mob_vnum_of(SHOP_KEEPER(nr)), SHOP_TRADE_WITH(nr));
    for (i = 0; SHOP_ROOM(nr, i) != NOWHERE; i++)
      fprintf(f, "%s%d", i ? "," : "", SHOP_ROOM(nr, i));
    fprintf(f, "],\"open1\":%d,\"close1\":%d,\"open2\":%d,\"close2\":%d}",
	    SHOP_OPEN1(nr), SHOP_CLOSE1(nr), SHOP_OPEN2(nr), SHOP_CLOSE2(nr));
  }
  fputs("]", f);
}

/*
 * Dump the whole loaded world. Field order matches dump.go's struct order,
 * because a diff of two differently-ordered JSON files is unreadable even
 * when the content is identical.
 */
void dump_world_json(FILE *f)
{
  fputs("{", f);
  fprintf(f, "\"counts\":{\"rooms\":%d,\"mobiles\":%d,\"objects\":%d,\"zones\":%d,\"shops\":%d},",
	  (int) top_of_world + 1, (int) top_of_mobt + 1, (int) top_of_objt + 1,
	  (int) top_of_zone_table + 1, top_shop + 1);
  dump_zones(f);
  fputs(",", f);
  dump_rooms(f);
  fputs(",", f);
  dump_mobiles(f);
  fputs(",", f);
  dump_objects(f);
  fputs(",", f);
  dump_shops(f);
  fputs("}\n", f);
}
