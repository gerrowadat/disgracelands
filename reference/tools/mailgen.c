/*
 * mailgen.c -- writes a mud mail file the way the C server writes one.
 *
 * Part of Disgracelands. See reference/tools/README.md.
 *
 * The Go codec in internal/persist/mail/classic is symmetric: it wrote the
 * block links one way and read them back the same way, so every round-trip
 * test it had passed while the meaning of the link was wrong. The link is a
 * *byte offset into the file*, not a block number -- store_mail assigns
 * `header.header_data.next_block = target_address` straight out of
 * pop_free_list (mail.c:349), and pop_free_list deals in the offsets
 * push_free_list was handed by scan_file, which multiplies by BLOCK_SIZE
 * (mail.c:263). Nothing in a self-consistent round trip can tell the two
 * apart, so the test needs bytes a C server actually wrote. That is this.
 *
 * Bodies below are store_mail, write_to_file, read_from_file and the free
 * list from ../WipeMud-src/src/mail.c, changed in three ways and no others:
 * MAIL_FILE is a global set from argv, store_mail takes the mail_time it
 * would have got from time(0) so the output is reproducible, and the
 * block-freeing half of read_delete is split out as delete_message() (the
 * rest of read_delete is the in-memory index and the text formatting, which
 * leave no trace in the file).
 *
 * Must be built 32-bit. HEADER_BLOCK_DATASIZE is
 * BLOCK_SIZE - sizeof(long) - sizeof(struct header_data_type) - sizeof(char),
 * which is 79 under ILP32 and 59 under LP64 -- the block size is 100 either
 * way, so a 64-bit build produces a file of exactly the right length whose
 * every message is garbled in the middle. See maillayout.c.
 *
 *   gcc -m32 -std=gnu89 -w -o mailgen mailgen.c
 *   ./mailgen <full-file> <simple-file>
 *
 * <full-file> exercises reuse: a message is deleted and the next one is
 * threaded back through its blocks, so the chain runs 5 -> 4 -> 6 -> 7 and a
 * reader that assumes chains are contiguous or ascending fails. <simple-file>
 * is append-only, which is the case the Go writer allocates identically to
 * the C, so it can be compared byte for byte.
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#define BLOCK_SIZE 100

#define HEADER_BLOCK  (-1)
#define LAST_BLOCK    (-2)
#define DELETED_BLOCK (-3)

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

/* local globals, as in mail.c */
static position_list_type *free_list = NULL;
static long file_end_pos = 0;
static const char *MAIL_FILE = NULL;

static void push_free_list(long pos)
{
  position_list_type *new_pos;

  new_pos = calloc(1, sizeof(position_list_type));
  new_pos->position = pos;
  new_pos->next = free_list;
  free_list = new_pos;
}

static long pop_free_list(void)
{
  position_list_type *old_pos;
  long return_value;

  /*
   * If we don't have any free blocks, we append to the file.
   */
  if ((old_pos = free_list) == NULL)
    return (file_end_pos);

  return_value = free_list->position;
  free_list = old_pos->next;
  free(old_pos);
  return (return_value);
}

static void write_to_file(void *buf, int size, long filepos)
{
  FILE *mail_file;

  if (filepos % BLOCK_SIZE) {
    fprintf(stderr, "SYSERR: Mail system -- fatal error #2!!! (invalid file position %ld)\n", filepos);
    exit(1);
  }
  if (!(mail_file = fopen(MAIL_FILE, "r+b"))) {
    fprintf(stderr, "SYSERR: Unable to open mail file '%s'.\n", MAIL_FILE);
    exit(1);
  }
  fseek(mail_file, filepos, SEEK_SET);
  fwrite(buf, size, 1, mail_file);

  /* find end of file */
  fseek(mail_file, 0L, SEEK_END);
  file_end_pos = ftell(mail_file);
  fclose(mail_file);
  return;
}

static void read_from_file(void *buf, int size, long filepos)
{
  FILE *mail_file;

  if (filepos % BLOCK_SIZE) {
    fprintf(stderr, "SYSERR: Mail system -- fatal error #3!!! (invalid filepos read %ld)\n", filepos);
    exit(1);
  }
  if (!(mail_file = fopen(MAIL_FILE, "r+b"))) {
    fprintf(stderr, "SYSERR: Unable to open mail file '%s'.\n", MAIL_FILE);
    exit(1);
  }
  fseek(mail_file, filepos, SEEK_SET);
  fread(buf, size, 1, mail_file);
  fclose(mail_file);
  return;
}

/*
 * store_mail (mail.c:307), with mail_time passed in rather than read from
 * time(0), and index_mail dropped -- it only touches the in-memory index.
 */
static void store_mail(long to, long from, char *message_pointer, time_t mail_time)
{
  header_block_type header;
  data_block_type data;
  long last_address, target_address;
  char *msg_txt = message_pointer;
  int bytes_written, total_length = strlen(message_pointer);

  if ((sizeof(header_block_type) != sizeof(data_block_type)) ||
      (sizeof(header_block_type) != BLOCK_SIZE)) {
    fprintf(stderr, "SYSERR: Mail system -- block size is wrong; build 32-bit.\n");
    exit(1);
  }

  if (from < 0 || to < 0 || !*message_pointer) {
    fprintf(stderr, "SYSERR: Mail system -- non-fatal error #5. (from == %ld, to == %ld)\n", from, to);
    return;
  }
  memset((char *) &header, 0, sizeof(header));	/* clear the record */
  header.block_type = HEADER_BLOCK;
  header.header_data.next_block = LAST_BLOCK;
  header.header_data.from = from;
  header.header_data.to = to;
  header.header_data.mail_time = mail_time;
  strncpy(header.txt, msg_txt, HEADER_BLOCK_DATASIZE);
  header.txt[HEADER_BLOCK_DATASIZE] = '\0';

  target_address = pop_free_list();	/* find next free block */
  write_to_file(&header, BLOCK_SIZE, target_address);

  if (strlen(msg_txt) <= HEADER_BLOCK_DATASIZE)
    return;			/* that was the whole message */

  bytes_written = HEADER_BLOCK_DATASIZE;
  msg_txt += HEADER_BLOCK_DATASIZE;	/* move pointer to next bit of text */

  /*
   * find the next block address, then rewrite the header to reflect where
   * the next block is.
   */
  last_address = target_address;
  target_address = pop_free_list();
  header.header_data.next_block = target_address;
  write_to_file(&header, BLOCK_SIZE, last_address);

  /* now write the current data block */
  memset((char *) &data, 0, sizeof(data));	/* clear the record */
  data.block_type = LAST_BLOCK;
  strncpy(data.txt, msg_txt, DATA_BLOCK_DATASIZE);
  data.txt[DATA_BLOCK_DATASIZE] = '\0';
  write_to_file(&data, BLOCK_SIZE, target_address);
  bytes_written += strlen(data.txt);
  msg_txt += strlen(data.txt);

  while (bytes_written < total_length) {
    last_address = target_address;
    target_address = pop_free_list();

    /* rewrite the previous block to link it to the next */
    data.block_type = target_address;
    write_to_file(&data, BLOCK_SIZE, last_address);

    /* now write the next block, assuming it's the last.  */
    data.block_type = LAST_BLOCK;
    strncpy(data.txt, msg_txt, DATA_BLOCK_DATASIZE);
    data.txt[DATA_BLOCK_DATASIZE] = '\0';
    write_to_file(&data, BLOCK_SIZE, target_address);

    bytes_written += strlen(data.txt);
    msg_txt += strlen(data.txt);
  }
}				/* store mail */

/*
 * The block-freeing half of read_delete (mail.c:476), with the in-memory
 * index and the text formatting left out. mail_address is the offset of the
 * message's header block.
 */
static void delete_message(long mail_address)
{
  header_block_type header;
  data_block_type data;
  long following_block;

  read_from_file(&header, BLOCK_SIZE, mail_address);
  if (header.block_type != HEADER_BLOCK) {
    fprintf(stderr, "SYSERR: Oh dear. (Header block %ld != %d)\n", header.block_type, HEADER_BLOCK);
    exit(1);
  }
  following_block = header.header_data.next_block;

  /* mark the block as deleted */
  header.block_type = DELETED_BLOCK;
  write_to_file(&header, BLOCK_SIZE, mail_address);
  push_free_list(mail_address);

  while (following_block != LAST_BLOCK) {
    read_from_file(&data, BLOCK_SIZE, following_block);
    mail_address = following_block;
    following_block = data.block_type;
    data.block_type = DELETED_BLOCK;
    write_to_file(&data, BLOCK_SIZE, mail_address);
    push_free_list(mail_address);
  }
}

/* appends n copies of c to buf, NUL terminated. */
static char *pad(char *buf, char c, int n)
{
  int len = strlen(buf);

  memset(buf + len, c, n);
  buf[len + n] = '\0';
  return buf;
}

/* touch(MAIL_FILE), as scan_file does when the file does not exist. */
static void start(const char *path)
{
  FILE *f;

  MAIL_FILE = path;
  free_list = NULL;
  file_end_pos = 0;
  if (!(f = fopen(path, "wb"))) {
    fprintf(stderr, "cannot create '%s'\n", path);
    exit(1);
  }
  fclose(f);
}

#define MAILTIME ((time_t) 1700000000)

/*
 * The three messages both files start with: one block, then three, then two.
 * Each run of characters is one block's worth, so a chain followed wrongly
 * shows up as the wrong letter rather than as a length.
 */
static void store_the_first_three(void)
{
  char buf[1024];

  store_mail(7, 3, "hello", MAILTIME);

  buf[0] = '\0';
  pad(buf, 'a', HEADER_BLOCK_DATASIZE);
  pad(buf, 'b', DATA_BLOCK_DATASIZE);
  pad(buf, 'c', 19);
  store_mail(1, 2, buf, MAILTIME + 60);

  buf[0] = '\0';
  pad(buf, 'd', HEADER_BLOCK_DATASIZE);
  pad(buf, 'e', 5);
  store_mail(9, 4, buf, MAILTIME + 120);
}

int main(int argc, char **argv)
{
  char buf[1024];

  if (argc != 3) {
    fprintf(stderr, "usage: %s <full-file> <simple-file>\n", argv[0]);
    return 1;
  }

  /*
   * The append-only file. Every block the C allocates here is a fresh one off
   * the end of the file, which is the one case the Go store's own allocator
   * agrees with pop_free_list about, so this file can be compared against
   * what the Go writes byte for byte.
   */
  start(argv[2]);
  store_the_first_three();

  /*
   * The same three, and then the third is read -- which frees its two blocks,
   * header first, onto the front of the free list -- and a four-block message
   * is stored into the gap. pop_free_list is LIFO, so it takes block 5 before
   * block 4 and only then grows the file: the chain runs 5 -> 4 -> 6 -> 7,
   * descending and then ascending, so a reader that assumes a chain is
   * contiguous or ascending fails on it.
   */
  start(argv[1]);
  store_the_first_three();
  delete_message(4 * BLOCK_SIZE);

  buf[0] = '\0';
  pad(buf, '1', HEADER_BLOCK_DATASIZE);
  pad(buf, '2', DATA_BLOCK_DATASIZE);
  pad(buf, '3', DATA_BLOCK_DATASIZE);
  pad(buf, '4', 20);
  store_mail(11, 5, buf, MAILTIME + 180);

  return 0;
}
