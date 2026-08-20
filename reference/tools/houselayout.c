/*
 * houselayout.c -- the on-disk layout of the house control file.
 *
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * House_save_control fwrites the whole house_control array in one call
 * (house.c:237, and the C's comment on that line is "Pretty nifty, eh?"), so
 * the control file is an array of `struct house_control_rec` and once again
 * the format is the struct's memory layout.
 *
 * This one has a padding hole in it. The first three members are sh_int and
 * the fourth is a time_t, so under ILP32 there are two bytes of nothing at
 * offset 6 that the compiler inserted and hcontrol_build_house never writes:
 * temp_house is a stack local, filled field by field, and the hole holds
 * whatever the stack held. So do the eight spare longs, and the unused tail
 * of the guests array. None of it is read back, and all of it is in the file.
 *
 * Prints "name offset size" per member, then the total.
 */

#include <stdio.h>
#include <stddef.h>
#include <stdlib.h>
#include <time.h>

typedef short int sh_int;
typedef sh_int room_vnum;

#define MAX_GUESTS 10

struct house_control_rec {
   room_vnum vnum;
   room_vnum atrium;
   sh_int exit_num;
   time_t built_on;
   int mode;
   long owner;
   int num_of_guests;
   long guests[MAX_GUESTS];
   time_t last_payment;
   long spare0;
   long spare1;
   long spare2;
   long spare3;
   long spare4;
   long spare5;
   long spare6;
   long spare7;
};

#define SHOW(f) printf("%s %zu %zu\n", #f, offsetof(struct house_control_rec, f), \
		       sizeof(((struct house_control_rec *) 0)->f))

int main(void)
{
  printf("# long %zu time_t %zu sh_int %zu\n",
	 sizeof(long), sizeof(time_t), sizeof(sh_int));

  SHOW(vnum);
  SHOW(atrium);
  SHOW(exit_num);
  SHOW(built_on);
  SHOW(mode);
  SHOW(owner);
  SHOW(num_of_guests);
  SHOW(guests);
  SHOW(last_payment);
  SHOW(spare0);
  printf("sizeof %zu\n", sizeof(struct house_control_rec));

  return (EXIT_SUCCESS);
}
