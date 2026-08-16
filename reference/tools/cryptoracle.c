/*
 * Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
 * of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
 * Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
 * (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
 * and DikuMUD licenses; see LICENSE. Non-commercial use only.
 */

/*
 * cryptoracle.c - hash passwords with the system crypt(3), for checking the
 * Go implementation against.
 *
 * internal/auth/descrypt reimplements traditional DES crypt(3) in Go, because
 * the standard library has none and cgo is ruled out by the static container
 * build. An implementation of a cipher that is "probably right" is worth very
 * little, so this exists to settle it: the Go tests feed thousands of
 * password/salt pairs through both and require identical output.
 *
 * Build:
 *   gcc -O2 -o /tmp/cryptoracle reference/tools/cryptoracle.c -lcrypt
 *
 * Usage: reads "<salt><TAB><password>" lines on stdin, writes the hash of
 * each on stdout, in order. The salt comes first because it is fixed-width
 * and a password may contain anything at all except a tab or a newline.
 *
 * This is a test oracle. It is not part of the server and nothing links it.
 */

#define _GNU_SOURCE

#include <crypt.h>
#include <stdio.h>
#include <string.h>

/*
 * <crypt.h> is not optional. Without it C assumes crypt() returns int, which
 * on a 64-bit target truncates the returned pointer and segfaults the moment
 * it is dereferenced - which is exactly what happened when this file was
 * first written. The Go test compiles it with
 * -Werror=implicit-function-declaration so that cannot recur quietly.
 */

int main(void)
{
  char line[4096];

  while (fgets(line, sizeof(line), stdin)) {
    char *tab, *salt, *password, *hash;

    line[strcspn(line, "\n")] = '\0';

    if (!(tab = strchr(line, '\t'))) {
      fprintf(stderr, "cryptoracle: no tab in input line\n");
      return (1);
    }
    *tab = '\0';
    salt = line;
    password = tab + 1;

    /*
     * A NULL return means this build of libcrypt rejected the salt - modern
     * glibc can be built to refuse the traditional DES form entirely. Say so
     * rather than printing nothing, so the Go side skips instead of silently
     * comparing against an empty string.
     */
    if (!(hash = crypt(password, salt))) {
      printf("UNSUPPORTED\n");
      continue;
    }
    printf("%s\n", hash);
  }

  return (0);
}
