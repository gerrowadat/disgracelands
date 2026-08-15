/*
 * pfiledump.c - read and print any ascii_pfiles-format player file
 * (WipeMud's genuine post-conversion files, or ones freshly produced by
 * bin2ascii). Pure text parsing, no structs.h dependency, so this is a
 * completely ordinary native build (part of `make utils`) - it doesn't
 * care what compiled the game binary or what wrote the ascii file.
 *
 * This exists to prove the ascii pfile format is actually readable
 * end-to-end (not just "we wrote something that looks plausible") -
 * see docs/pfile-conversion.md.
 *
 * Usage: pfiledump <path-to-ascii-pfile> [<path> ...]
 */

#include <stdio.h>
#include <string.h>
#include <time.h>

static void dump_one(const char *path) {
  FILE *fl;
  char line[1024];
  char name[64] = "?", pass[64] = "(hidden)";
  int sex = -1, clas = -1, level = -1, tagcount = 0;
  long brth = 0, last = 0;

  if (!(fl = fopen(path, "r"))) {
    printf("%-40s  COULD NOT OPEN\n", path);
    return;
  }

  while (fgets(line, sizeof(line), fl)) {
    char tag[8];
    line[strcspn(line, "\r\n")] = '\0';
    if (strlen(line) < 5)
      continue;
    memcpy(tag, line, 4);
    tag[4] = '\0';
    tagcount++;
    if (!strcmp(tag, "Name")) snprintf(name, sizeof(name), "%s", line + 6);
    else if (!strcmp(tag, "Pass")) snprintf(pass, sizeof(pass), "%s", "***** (present, not printed)");
    else if (!strcmp(tag, "Sex ")) sscanf(line + 6, "%d", &sex);
    else if (!strcmp(tag, "Clas")) sscanf(line + 6, "%d", &clas);
    else if (!strcmp(tag, "Levl")) sscanf(line + 6, "%d", &level);
    else if (!strcmp(tag, "Brth")) sscanf(line + 6, "%ld", &brth);
    else if (!strcmp(tag, "Last")) sscanf(line + 6, "%ld", &last);
  }
  fclose(fl);

  {
    char bbuf[32] = "-", lbuf[32] = "-";
    if (brth > 0) { struct tm *t = gmtime(&brth); strftime(bbuf, sizeof(bbuf), "%Y-%m-%d", t); }
    if (last > 0) { struct tm *t = gmtime(&last); strftime(lbuf, sizeof(lbuf), "%Y-%m-%d", t); }
    printf("%-40s  name=%-14s sex=%-2d clas=%-2d lvl=%-3d birth=%-10s last=%-10s tags=%d  pass=%s\n",
           path, name, sex, clas, level, bbuf, lbuf, tagcount, pass);
  }
}

int main(int argc, char **argv) {
  int i;
  if (argc < 2) {
    fprintf(stderr, "usage: %s <ascii-pfile> [<ascii-pfile> ...]\n", argv[0]);
    return 1;
  }
  for (i = 1; i < argc; i++)
    dump_one(argv[i]);
  return 0;
}
