/*
 * bin2ascii.c - convert Disgracelands' classic binary player database
 * (a flat array of `struct char_file_u`, one record per player, as written
 * by the pre-ascii_pfiles `save_char()`/`char_to_store()` in db.c) into
 * the ascii_pfiles-style one-file-per-player text format documented in
 * welmar/pfiles/ascii_pfiles_2.1/doc/using.txt and implemented by
 * welmar/pfiles/ascii_pfiles_2.1/full_src/db.c's load_char()/save_char().
 *
 * IMPORTANT: this MUST be compiled 32-bit (-m32) against Reborn's own
 * src/structs.h. The original binary player database was written by a
 * 32-bit FreeBSD/i386 build. struct char_file_u contains several `long`
 * fields (char_special_data_saved.idnum/act/affected_by,
 * player_special_data_saved.spare17-21) that are 4 bytes on a 32-bit
 * build and 8 bytes on a native 64-bit build - a plain 64-bit fread()
 * of this struct silently misreads the file (wrong offsets past the
 * first `long`, not just wrong byte order). This has nothing to do with
 * SPARC/endianness (see docs/circlemud-archive-report.md S0/S5) - it's
 * an ILP32-vs-LP64 struct layout mismatch that would bite on a 64-bit
 * Linux or 64-bit FreeBSD rebuild just as much as anywhere else.
 *
 * Build (from Reborn/, the repo root):
 *   gcc -m32 -std=gnu89 -fcommon -w -Isrc -o bin/bin2ascii tools/bin2ascii.c
 *
 * Usage:
 *   bin/bin2ascii <path-to-binary-players-db> <output-pfiles-dir>
 *
 * Writes <output-pfiles-dir>/<first-letter>/<lowercased-name> for every
 * record, plus a plr_index file, in the same shape WipeMud's real
 * lib/pfiles/ uses (see welmar/WipeMud/lib/pfiles/).
 */

#include "conf.h"
#include "sysdep.h"
#include "structs.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>
#include <sys/stat.h>

static void die(const char *msg) {
  fprintf(stderr, "bin2ascii: %s\n", msg);
  exit(1);
}

static void lowercase(char *s) {
  for (; *s; s++) *s = tolower((unsigned char)*s);
}

/* Write one <name> ascii pfile in the format load_char()/save_char() in
 * welmar/pfiles/ascii_pfiles_2.1/full_src/db.c expect. */
static void write_ascii_pfile(const char *dir, struct char_file_u *st) {
  char lname[MAX_NAME_LENGTH + 1];
  char subdir[512], path[600];
  FILE *fl;
  int i;

  strncpy(lname, st->name, MAX_NAME_LENGTH);
  lname[MAX_NAME_LENGTH] = '\0';
  lowercase(lname);

  snprintf(subdir, sizeof(subdir), "%s/%c", dir, lname[0]);
  mkdir(subdir, 0755); /* ignore EEXIST */
  snprintf(path, sizeof(path), "%s/%s", subdir, lname);

  if (!(fl = fopen(path, "w"))) {
    fprintf(stderr, "bin2ascii: couldn't open %s for writing\n", path);
    return;
  }

  fprintf(fl, "Name: %s\n", st->name);
  fprintf(fl, "Pass: %s\n", st->pwd);
  if (st->title[0])
    fprintf(fl, "Titl: %s\n", st->title);
  if (st->description[0]) {
    char *p, *desc = strdup(st->description);
    fprintf(fl, "Desc:\n");
    for (p = strtok(desc, "\n"); p; p = strtok(NULL, "\n"))
      fprintf(fl, "%s\n", p);
    fprintf(fl, "~\n");
    free(desc);
  }
  fprintf(fl, "Sex : %d\n", st->sex);
  fprintf(fl, "Clas: %d\n", st->chclass);
  fprintf(fl, "Race: 0\n"); /* CircleMUD3 has no race system (unlike WipeMud) */
  fprintf(fl, "Levl: %d\n", st->level);
  fprintf(fl, "Home: %d\n", st->hometown);
  fprintf(fl, "Brth: %ld\n", (long)st->birth);
  fprintf(fl, "Plyd: %d\n", st->played);
  fprintf(fl, "Last: %ld\n", (long)st->last_logon);
  fprintf(fl, "Host: %s\n", st->host);
  fprintf(fl, "Hite: %d\n", st->height);
  fprintf(fl, "Wate: %d\n", st->weight);

  fprintf(fl, "Str : %d/%d\n", st->abilities.str, st->abilities.str_add);
  fprintf(fl, "Int : %d\n", st->abilities.intel);
  fprintf(fl, "Wis : %d\n", st->abilities.wis);
  fprintf(fl, "Dex : %d\n", st->abilities.dex);
  fprintf(fl, "Con : %d\n", st->abilities.con);
  fprintf(fl, "Cha : %d\n", st->abilities.cha);

  fprintf(fl, "Hit : %d/%d\n", st->points.hit, st->points.max_hit);
  fprintf(fl, "Mana: %d/%d\n", st->points.mana, st->points.max_mana);
  fprintf(fl, "Move: %d/%d\n", st->points.move, st->points.max_move);
  fprintf(fl, "Ac  : %d\n", st->points.armor);
  fprintf(fl, "Gold: %d\n", st->points.gold);
  fprintf(fl, "Bank: %d\n", st->points.bank_gold);
  fprintf(fl, "Exp : %d\n", st->points.exp);
  fprintf(fl, "Hrol: %d\n", st->points.hitroll);
  fprintf(fl, "Drol: %d\n", st->points.damroll);

  fprintf(fl, "Alin: %d\n", st->char_specials_saved.alignment);
  fprintf(fl, "Id  : %ld\n", st->char_specials_saved.idnum);
  fprintf(fl, "Act : %ld\n", st->char_specials_saved.act);
  fprintf(fl, "Aff : %ld\n", st->char_specials_saved.affected_by);
  for (i = 0; i < 5; i++)
    if (st->char_specials_saved.apply_saving_throw[i])
      fprintf(fl, "Thr%d: %d\n", i + 1, st->char_specials_saved.apply_saving_throw[i]);

  fprintf(fl, "Wimp: %d\n", st->player_specials_saved.wimp_level);
  fprintf(fl, "Frez: %d\n", st->player_specials_saved.freeze_level);
  fprintf(fl, "Invs: %d\n", st->player_specials_saved.invis_level);
  fprintf(fl, "Room: %d\n", st->player_specials_saved.load_room);
  fprintf(fl, "Pref: %ld\n", st->player_specials_saved.pref);
  fprintf(fl, "Badp: %d\n", st->player_specials_saved.bad_pws);
  fprintf(fl, "Drnk: %d\n", st->player_specials_saved.conditions[DRUNK]);
  fprintf(fl, "Hung: %d\n", st->player_specials_saved.conditions[FULL]);
  fprintf(fl, "Thir: %d\n", st->player_specials_saved.conditions[THIRST]);
  fprintf(fl, "Lern: %d\n", st->player_specials_saved.spells_to_learn);
  if (st->player_specials_saved.remort_vector)
    fprintf(fl, "Remv: %d\n", st->player_specials_saved.remort_vector);

  fprintf(fl, "Skil:\n");
  for (i = 1; i <= MAX_SKILLS; i++)
    if (st->player_specials_saved.skills[i])
      fprintf(fl, "%d %d\n", i, st->player_specials_saved.skills[i]);
  fprintf(fl, "0 0\n");

  fprintf(fl, "Affs:\n");
  for (i = 0; i < MAX_AFFECT; i++)
    if (st->affected[i].type)
      fprintf(fl, "%d %d %d %d %ld\n", st->affected[i].type,
              st->affected[i].duration, st->affected[i].modifier,
              st->affected[i].location, st->affected[i].bitvector);
  fprintf(fl, "0 0 0 0 0\n");

  fprintf(fl, "End\n");
  fclose(fl);
}

int main(int argc, char **argv) {
  FILE *in;
  struct char_file_u rec;
  int count = 0, ok = 0;
  char indexpath[600];
  FILE *idx;

  if (sizeof(long) != 4 || sizeof(int) != 4)
    die("this must be compiled 32-bit (-m32) - see the comment at the "
        "top of this file. sizeof(long) or sizeof(int) != 4 here.");

  if (argc != 3) {
    fprintf(stderr, "usage: %s <binary-players-db> <output-pfiles-dir>\n", argv[0]);
    return 1;
  }

  if (!(in = fopen(argv[1], "rb")))
    die("couldn't open input binary player database");

  mkdir(argv[2], 0755);
  snprintf(indexpath, sizeof(indexpath), "%s/plr_index", argv[2]);
  if (!(idx = fopen(indexpath, "w")))
    die("couldn't open plr_index for writing");

  while (fread(&rec, sizeof(struct char_file_u), 1, in) == 1) {
    count++;
    if (!rec.name[0])
      continue; /* deleted/empty slot */
    write_ascii_pfile(argv[2], &rec);
    fprintf(idx, "%d %s %d %d %ld\n", count,
            rec.name, rec.level, 0, (long)rec.last_logon);
    ok++;
  }
  fclose(idx);
  fclose(in);

  printf("bin2ascii: %d records read, %d non-empty players converted into %s\n",
         count, ok, argv[2]);
  printf("sizeof(struct char_file_u) here = %zu (sanity check against the\n"
         "source file size: it should divide evenly, same as the analysis\n"
         "in docs/circlemud-archive-report.md S5)\n", sizeof(struct char_file_u));
  return 0;
}
