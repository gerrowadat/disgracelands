#include <stdio.h>
#include "conf.h"
#include "sysdep.h"
#include "structs.h"
#include "comm.h"
#include "handler.h"
#include "db.h"
#include "interpreter.h"
#include "utils.h"
#include "spells.h"
#include <autoeq_struct.h>

int main(int argc, char **argv)
{
  FILE *fl;
  char buf[MAX_STRING_LENGTH];
  struct obj_file_elem object;
  struct obj_data *obj;
  struct rent_info rent;
  int numread;

  if (!(fl = fopen(argv[1], "rb"))) {
    sprintf(buf, "no rent file.\r\n");
    return 0;
  }

  numread = fread(&rent, sizeof(struct rent_info), 1, fl);

  /* Oops, can't get the data, punt. */
  if (numread == 0) {
    fclose(fl);
    return 0;
  }

  while (!feof(fl)) {
    fread(&object, sizeof(struct obj_file_elem), 1, fl);
    if (ferror(fl)) {
      fclose(fl);
      return 0;
    }
    if (!feof(fl))
      if (real_object(object.item_number) != NOTHING) {
	obj = read_object(object.item_number, VIRTUAL);
	sprintf(buf + strlen(buf), " [%5d] (%5dau) <%2d> %-20s\r\n",
		object.item_number, GET_OBJ_RENT(obj),
/*		 object.location, obj->short_description); */
		1, obj->short_description);
	extract_obj(obj);
	if (strlen(buf) > MAX_STRING_LENGTH - 80) {
	  strcat(buf, "** Excessive rent listing. **\r\n");
	  break;
	}
      }
  }
  printf("%s\n",buf);
  fclose(fl);
  return 1;
}


