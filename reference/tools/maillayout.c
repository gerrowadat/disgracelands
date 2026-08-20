/*
 * maillayout.c -- the on-disk layout of the mud mail file.
 *
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * The mail file is a block allocator: fixed BLOCK_SIZE records, each either a
 * header block (the start of a message) or a data block (a continuation),
 * linked through their first field like a FAT. store_mail and read_delete
 * fwrite and fread the structs whole, so once again the format *is* the
 * memory layout.
 *
 * What makes this one interesting is that the block size does not change with
 * the data model but the *split inside the block* does:
 *
 *     #define HEADER_BLOCK_DATASIZE \
 *       (BLOCK_SIZE - sizeof(long) - sizeof(struct header_data_type) - sizeof(char))
 *
 * On the i386 build the archive came from, that is 100 - 4 - 16 - 1 = 79
 * characters of text in a header block. On a 64-bit rebuild it is
 * 100 - 8 - 32 - 1 = 59. Both produce 100-byte blocks, so a file written by
 * one and read by the other lines up perfectly and every message comes out
 * garbled in the middle. There is no length or magic anywhere to catch it.
 *
 * Prints the offsets and sizes the compiler chose.
 */

#include <stdio.h>
#include <stddef.h>
#include <stdlib.h>
#include <time.h>

#define BLOCK_SIZE 100

struct header_data_type {
   long	next_block;
   long from;
   long to;
   time_t mail_time;
};

#define HEADER_BLOCK_DATASIZE \
	(BLOCK_SIZE - sizeof(long) - sizeof(struct header_data_type) - sizeof(char))
#define DATA_BLOCK_DATASIZE (BLOCK_SIZE - sizeof(long) - sizeof(char))

struct header_block_type_d {
   long	block_type;
   struct header_data_type header_data;
   char	txt[HEADER_BLOCK_DATASIZE+1];
};

struct data_block_type_d {
   long	block_type;
   char	txt[DATA_BLOCK_DATASIZE+1];
};

int main(void)
{
  printf("# long %zu time_t %zu\n", sizeof(long), sizeof(time_t));

  printf("block_size %d\n", BLOCK_SIZE);
  printf("header_size %zu\n", sizeof(struct header_block_type_d));
  printf("data_size %zu\n", sizeof(struct data_block_type_d));

  printf("header_datasize %zu\n", (size_t) HEADER_BLOCK_DATASIZE);
  printf("data_datasize %zu\n", (size_t) DATA_BLOCK_DATASIZE);

  printf("block_type %zu\n", offsetof(struct header_block_type_d, block_type));
  printf("next_block %zu\n",
	 offsetof(struct header_block_type_d, header_data) +
	 offsetof(struct header_data_type, next_block));
  printf("from %zu\n",
	 offsetof(struct header_block_type_d, header_data) +
	 offsetof(struct header_data_type, from));
  printf("to %zu\n",
	 offsetof(struct header_block_type_d, header_data) +
	 offsetof(struct header_data_type, to));
  printf("mail_time %zu\n",
	 offsetof(struct header_block_type_d, header_data) +
	 offsetof(struct header_data_type, mail_time));
  printf("header_txt %zu\n", offsetof(struct header_block_type_d, txt));
  printf("data_txt %zu\n", offsetof(struct data_block_type_d, txt));

  return (EXIT_SUCCESS);
}
