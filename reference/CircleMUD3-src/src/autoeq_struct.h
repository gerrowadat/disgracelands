
struct obj_file_elem_autoeq {
   obj_vnum item_number;

   sh_int location;
   int	value[4];
   int /*bitvector_t*/	extra_flags;
   int	weight;
   int	timer;
   long /*bitvector_t*/	bitvector;
   struct obj_affected_type affected[MAX_OBJ_AFFECT];
};
