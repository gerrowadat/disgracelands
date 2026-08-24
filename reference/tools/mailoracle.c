/*
 * mailoracle.c -- write a mud mail file the way the C actually writes one.
 *
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * maillayout.c next door prints the offsets inside a block. That is enough
 * to place the fields and not enough to say what goes *in* one of them: the
 * link that joins a message's blocks together holds a **byte offset into the
 * file**, not a block number, and nothing about a struct layout reveals
 * which. push_free_list's own comment is the statement of it ("#1 - What
 * byte offset into the file the block resides", mail.c:76); every value that
 * reaches a link comes from pop_free_list or from block_num * BLOCK_SIZE.
 *
 * A port that writes a block *index* there is self-consistent and passes its
 * own round-trip tests forever, while being unable to read a single message
 * the C wrote that ran past one block. So this is the oracle for the units:
 * store_mail's body, unedited apart from the globals it needs, run to
 * produce a real file for the Go to read.
 *
 * Usage: mailoracle <outfile> <to> <from> <message>
 *
 * Build 32-bit (-m32) to get the archive's own layout: HEADER_BLOCK_DATASIZE
 * is 79 there and 59 on a 64-bit build, and both give 100-byte blocks. See
 * maillayout.c's comment for why that particular mismatch is so quiet.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#define BLOCK_SIZE	100
#define HEADER_BLOCK	(-1)
#define LAST_BLOCK	(-2)
#define DELETED_BLOCK	(-3)

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

typedef struct header_block_type_d header_block_type;
typedef struct data_block_type_d data_block_type;

typedef struct position_list_type_d {
   long position;
   struct position_list_type_d *next;
} position_list_type;

/* The globals mail.c keeps, and the file it works on. */
static position_list_type *free_list = NULL;
static long file_end_pos = 0;
static const char *MAIL_FILE;

/* push_free_list/pop_free_list (mail.c:82, mail.c:100), verbatim. */
static long pop_free_list(void)
{
  position_list_type *old_pos;
  long return_value;

  if ((old_pos = free_list) == NULL)
    return (file_end_pos);

  return_value = free_list->position;
  free_list = old_pos->next;
  free(old_pos);
  return (return_value);
}

/*
 * write_to_file (mail.c:158), with the log()/no_mail reporting replaced by
 * an abort -- the caller here is a test, and a mail system that has decided
 * to stop accepting mail is not a result worth returning quietly.
 */
static void write_to_file(void *buf, int size, long filepos)
{
  FILE *mail_file;

  if (filepos % BLOCK_SIZE) {
    fprintf(stderr, "fatal error #2!!! (invalid file position %ld)\n", filepos);
    exit(1);
  }
  if (!(mail_file = fopen(MAIL_FILE, "r+b"))) {
    fprintf(stderr, "unable to open mail file '%s'\n", MAIL_FILE);
    exit(1);
  }
  fseek(mail_file, filepos, SEEK_SET);
  fwrite(buf, size, 1, mail_file);

  fseek(mail_file, 0L, SEEK_END);
  file_end_pos = ftell(mail_file);
  fclose(mail_file);
  return;
}

/*
 * store_mail (mail.c:303). The body is unchanged; index_mail is dropped
 * because it only builds the in-memory index, and the sizeof/negative-id
 * guards are dropped because this tool's caller controls both.
 */
static void store_mail(long to, long from, char *message_pointer)
{
  header_block_type header;
  data_block_type data;
  long last_address, target_address;
  char *msg_txt = message_pointer;
  int bytes_written, total_length = strlen(message_pointer);

  memset((char *) &header, 0, sizeof(header));
  header.block_type = HEADER_BLOCK;
  header.header_data.next_block = LAST_BLOCK;
  header.header_data.from = from;
  header.header_data.to = to;
  header.header_data.mail_time = 0;	/* time(0), pinned so the file is reproducible */
  strncpy(header.txt, msg_txt, HEADER_BLOCK_DATASIZE);
  header.txt[HEADER_BLOCK_DATASIZE] = '\0';

  target_address = pop_free_list();
  write_to_file(&header, BLOCK_SIZE, target_address);

  if (strlen(msg_txt) <= HEADER_BLOCK_DATASIZE)
    return;

  bytes_written = HEADER_BLOCK_DATASIZE;
  msg_txt += HEADER_BLOCK_DATASIZE;

  last_address = target_address;
  target_address = pop_free_list();
  header.header_data.next_block = target_address;
  write_to_file(&header, BLOCK_SIZE, last_address);

  memset((char *) &data, 0, sizeof(data));
  data.block_type = LAST_BLOCK;
  strncpy(data.txt, msg_txt, DATA_BLOCK_DATASIZE);
  data.txt[DATA_BLOCK_DATASIZE] = '\0';
  write_to_file(&data, BLOCK_SIZE, target_address);
  bytes_written += strlen(data.txt);
  msg_txt += strlen(data.txt);

  while (bytes_written < total_length) {
    last_address = target_address;
    target_address = pop_free_list();

    data.block_type = target_address;
    write_to_file(&data, BLOCK_SIZE, last_address);

    data.block_type = LAST_BLOCK;
    strncpy(data.txt, msg_txt, DATA_BLOCK_DATASIZE);
    data.txt[DATA_BLOCK_DATASIZE] = '\0';
    write_to_file(&data, BLOCK_SIZE, target_address);

    bytes_written += strlen(data.txt);
    msg_txt += strlen(data.txt);
  }
}

int main(int argc, char **argv)
{
  FILE *f;

  if (argc != 5) {
    fprintf(stderr, "usage: %s <outfile> <to> <from> <message>\n", argv[0]);
    return (2);
  }
  MAIL_FILE = argv[1];

  /* Start from an empty file, the way the mud does on a fresh install. */
  if (!(f = fopen(MAIL_FILE, "wb"))) {
    fprintf(stderr, "cannot create '%s'\n", MAIL_FILE);
    return (1);
  }
  fclose(f);
  file_end_pos = 0;

  store_mail(atol(argv[2]), atol(argv[3]), argv[4]);

  printf("header_datasize %zu\n", HEADER_BLOCK_DATASIZE);
  printf("data_datasize %zu\n", DATA_BLOCK_DATASIZE);
  printf("blocks %ld\n", file_end_pos / BLOCK_SIZE);
  return (0);
}
