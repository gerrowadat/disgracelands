// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package classic reads and writes the mud mail file, porting mail.c.
//
// The file is a block allocator, and the C's own comment says what it is:
// "This works much like DOS' FAT." Fixed hundred-byte records, each either a
// header block starting a message or a data block continuing one, chained
// through their first field. Freed blocks go on a free list and are reused,
// so the file grows to its high-water mark and stays there.
//
// A link in that chain is a byte offset into the file, not a block number —
// see blockOffset, and the fixture in testdata that pins it.
//
// The block size is the same under every data model. What changes is the
// split *inside* a block — a header block holds 79 characters of text under
// ILP32 and 59 under LP64, because the fields before the text are wider — so
// a file written by one and read by the other lines up perfectly and comes
// out garbled in the middle, with nothing anywhere to catch it. See
// reference/tools/maillayout.c.
package classic

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gerrowadat/disgracelands/internal/persist/mail"
)

// FormatName is the name this format registers under, and the default the
// server runs on.
const FormatName = "classic"

func init() {
	mail.Register(FormatName, func(cfg mail.Config) (mail.Store, error) {
		return New(cfg)
	})
}

var byteOrder = binary.LittleEndian

// BlockSize is BLOCK_SIZE (mail.h:29).
const BlockSize = 100

// The ILP32 layout of a block. Verified against gcc -m32 by layout_test.go.
const (
	offBlockType = 0
	offNextBlock = 4
	offFrom      = 8
	offTo        = 12
	offMailTime  = 16
	offHeaderTxt = 20
	offDataTxt   = 4

	// HeaderTextSize is HEADER_BLOCK_DATASIZE: 100 - 4 - 16 - 1.
	HeaderTextSize = 79
	// DataTextSize is DATA_BLOCK_DATASIZE: 100 - 4 - 1.
	DataTextSize = 95
)

// Block types (mail.h:53). A data block's type is either one of these or a
// link to the next block — and that link is a *byte offset into the file*,
// not a block number. See blockOffset. The same is true of a header block's
// next_block.
const (
	headerBlock  int32 = -1
	lastBlock    int32 = -2
	deletedBlock int32 = -3
)

// Store is the mail file.
//
// The whole file is held in memory and rewritten when it changes. The C seeks
// around the file block by block, which is the right shape for 1993 and the
// wrong shape for a server with one goroutine owning the world: `receive`
// would put a seek and two reads on that goroutine. A mail file of ten
// thousand messages is a megabyte.
type Store struct {
	path     string
	readOnly bool

	mu     sync.Mutex
	blocks [][]byte
}

// New opens the mail file.
func New(cfg mail.Config) (*Store, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("mail: no file configured")
	}
	s := &Store{path: cfg.Path, readOnly: cfg.ReadOnly}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Name implements mail.Store.
func (s *Store) Name() string { return FormatName }

// Close implements mail.Store.
func (s *Store) Close() error { return nil }

// load reads the file, porting scan_file (mail.c:247).
//
// A file whose length is not a whole number of blocks is truncated to the
// last whole one, which is what the C's read loop does.
func (s *Store) load() error {
	b, err := os.ReadFile(s.path) //nolint:gosec // a configured path
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading the mail file: %w", err)
	}

	s.blocks = make([][]byte, 0, len(b)/BlockSize)
	for off := 0; off+BlockSize <= len(b); off += BlockSize {
		block := make([]byte, BlockSize)
		copy(block, b[off:off+BlockSize])
		s.blocks = append(s.blocks, block)
	}
	return nil
}

// HasMail implements mail.Store (has_mail, mail.c:287).
func (s *Store) HasMail(recipient int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.findHeader(recipient) >= 0
}

// findHeader returns the index of the lowest-numbered header block addressed
// to this recipient, or -1.
//
// Which one the C picks takes a moment to work out, and the answer is not
// stable. `index_mail` pushes each new message onto the *front* of a
// per-player list (mail.c:233) and `read_delete` walks to the *end* of that
// list and takes from there (mail.c:436), so within one run of the server
// mail is delivered oldest first. But `scan_file` rebuilds the same list at
// boot by scanning the file from the start and prepending each header it
// finds — so after a reboot the list is in descending block order and the
// tail is the *lowest-numbered block*, whatever the order the messages were
// actually sent in.
//
// Those two agree only while the file is growing. Once a freed block has been
// reused, a message stored in it is delivered before older mail sitting
// further up the file, but only after a reboot. Ascending block order is what
// the C does after every reboot, and it is what this does always — the
// alternative is behaviour that changes depending on how long the server has
// been up. It is in docs/weirdnumbers.md.
func (s *Store) findHeader(recipient int64) int {
	for i, block := range s.blocks {
		if blockType(block) != headerBlock {
			continue
		}
		if byteOrder.Uint32(block[offTo:]) == uint32(int32(recipient)) { //nolint:gosec // ids are 32-bit in this format
			return i
		}
	}
	return -1
}

// Send implements mail.Store (store_mail, mail.c:303).
func (s *Store) Send(m mail.Message) error {
	if m.From < 0 || m.To < 0 || m.Text == "" {
		// "SYSERR: Mail system -- non-fatal error #5."
		return fmt.Errorf("mail: refusing to store a message from %d to %d", m.From, m.To)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	text := m.Text
	head := s.alloc()

	block := make([]byte, BlockSize)
	putInt32(block[offBlockType:], headerBlock)
	putInt32(block[offNextBlock:], lastBlock)
	putInt32(block[offFrom:], int32(m.From))            //nolint:gosec // ids are 32-bit in this format
	putInt32(block[offTo:], int32(m.To))                //nolint:gosec // ditto
	putInt32(block[offMailTime:], int32(m.Sent.Unix())) //nolint:gosec // 2038, and the format's fault
	written := copyText(block[offHeaderTxt:], text, HeaderTextSize)
	s.blocks[head] = block
	text = text[written:]

	prev := head
	for text != "" {
		next := s.alloc()
		// Link the previous block to this one. In a header block the link is
		// next_block; in a data block it is the block type itself. Either
		// way it is a byte offset, not an index.
		if prev == head {
			putInt32(s.blocks[prev][offNextBlock:], blockOffset(next))
		} else {
			putInt32(s.blocks[prev][offBlockType:], blockOffset(next))
		}

		data := make([]byte, BlockSize)
		putInt32(data[offBlockType:], lastBlock)
		written = copyText(data[offDataTxt:], text, DataTextSize)
		s.blocks[next] = data
		text = text[written:]
		prev = next
	}

	return s.flushLocked()
}

// Receive implements mail.Store (read_delete, mail.c:398).
func (s *Store) Receive(recipient int64) (mail.Message, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	head := s.findHeader(recipient)
	if head < 0 {
		return mail.Message{}, false, nil
	}

	m, span := s.readChainLocked(head)
	for _, i := range span {
		s.blocks[i] = make([]byte, BlockSize)
		putInt32(s.blocks[i][offBlockType:], deletedBlock)
	}
	return m, true, s.flushLocked()
}

// All is a non-destructive read of every message in the file, for tooling
// that needs to see everything at once (`dlctl state import`, chiefly) —
// unlike Receive, nothing here is freed or removed.
func (s *Store) All() []mail.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []mail.Message
	for i, block := range s.blocks {
		if blockType(block) != headerBlock {
			continue
		}
		m, _ := s.readChainLocked(i)
		out = append(out, m)
	}
	return out
}

// readChainLocked assembles the message rooted at header block head,
// returning it and the indices of every block it spans (the header plus
// any data blocks in its chain), in header-then-chain order. The caller
// holds s.mu.
//
// A malformed chain stops rather than looping: the C would follow it off
// the end of the file, and a message that points at itself would hang the
// server.
func (s *Store) readChainLocked(head int) (mail.Message, []int) {
	block := s.blocks[head]
	m := mail.Message{
		To:   int64(int32(byteOrder.Uint32(block[offTo:]))),   //nolint:gosec // reinterpretation
		From: int64(int32(byteOrder.Uint32(block[offFrom:]))), //nolint:gosec // reinterpretation
		Text: readText(block[offHeaderTxt:], HeaderTextSize),
	}
	if secs := int32(byteOrder.Uint32(block[offMailTime:])); secs != 0 { //nolint:gosec // reinterpretation
		m.Sent = time.Unix(int64(secs), 0).UTC()
	}

	span := []int{head}
	seen := map[int]bool{head: true}
	next := s.blockAt(int32(byteOrder.Uint32(block[offNextBlock:]))) //nolint:gosec // reinterpretation
	for next >= 0 && !seen[next] {
		seen[next] = true
		data := s.blocks[next]
		m.Text += readText(data[offDataTxt:], DataTextSize)
		span = append(span, next)
		next = s.blockAt(blockType(data))
	}
	return m, span
}

// blockOffset is what the C stores in a link: the block's byte offset into
// the file, because that is what it passes to fseek.
//
// pop_free_list hands back an offset (mail.c:115, and file_end_pos when the
// free list is empty), store_mail assigns it straight into next_block or a
// data block's block_type (mail.c:349, :378), and read_delete hands it back
// to read_from_file, which seeks to it (mail.c:476, :209). scan_file builds
// the free list the same way, multiplying its block counter by BLOCK_SIZE
// (mail.c:263).
//
// It reads exactly like an index and is not one, which is how this package
// had it wrong: the store wrote indices and read indices, so every
// round-trip test it had passed while no file a C server wrote could be
// read at all past its first block. docs/weirdnumbers.md has it; the fixture
// in testdata is bytes the C actually wrote.
func blockOffset(i int) int32 {
	return int32(i * BlockSize) //nolint:gosec // a 2GB mail file is not a thing
}

// blockAt is the reverse: the index of the block a link names, or -1 if it
// names no block in this file.
//
// LAST_BLOCK and DELETED_BLOCK are negative and land here as -1, which ends
// a chain. So does an offset past the end of the file or one that is not a
// whole number of blocks — the C would seek there and read rubbish, or log
// its "invalid filepos read" and disable mail; there is nothing to be gained
// from reproducing that in a reader whose whole file is already in memory.
func (s *Store) blockAt(link int32) int {
	if link < 0 || link%BlockSize != 0 {
		return -1
	}
	i := int(link) / BlockSize
	if i >= len(s.blocks) {
		return -1
	}
	return i
}

// alloc returns a block index to write into, porting pop_free_list.
//
// The C keeps an explicit free list built at boot; this scans, which is the
// same answer at a size where it does not matter. A deleted block is reused
// before the file is grown, which is the behaviour that keeps the file from
// growing without bound.
//
// Not quite the same answer, in one respect: pop_free_list is LIFO, so the C
// refills the block freed *last* and this refills the *lowest*. Neither file
// is unreadable to the other and only a byte-for-byte comparison can see it,
// which is why fixture_test.go can only make that comparison against a file
// nothing has been deleted from yet. It is in docs/deviations.md.
func (s *Store) alloc() int {
	for i, block := range s.blocks {
		if blockType(block) == deletedBlock {
			return i
		}
	}
	s.blocks = append(s.blocks, make([]byte, BlockSize))
	return len(s.blocks) - 1
}

// flushLocked rewrites the whole file. The caller holds the lock.
func (s *Store) flushLocked() error {
	if s.readOnly {
		return fmt.Errorf("mail: the data directory is open read-only")
	}

	// A file of nothing but deleted blocks is removed, so an empty mail
	// system leaves no file. The C never shrinks its file; this is a
	// deviation and is documented.
	live := false
	for _, block := range s.blocks {
		if blockType(block) != deletedBlock {
			live = true
			break
		}
	}
	if !live {
		s.blocks = nil
		if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing the empty mail file: %w", err)
		}
		return nil
	}

	out := make([]byte, 0, len(s.blocks)*BlockSize)
	for _, block := range s.blocks {
		out = append(out, block...)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("writing the mail file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing the mail file: %w", err)
	}
	return nil
}

func blockType(block []byte) int32 {
	return int32(byteOrder.Uint32(block[offBlockType:])) //nolint:gosec // reinterpretation
}

func putInt32(b []byte, v int32) { byteOrder.PutUint32(b, uint32(v)) } //nolint:gosec // reinterpretation

// copyText writes up to size bytes of text into a field that is size+1 wide,
// NUL-terminated, and reports how much it took.
//
// strncpy followed by an explicit terminator, which is what the C does — so a
// block holds exactly HEADER_BLOCK_DATASIZE characters and never the NUL's
// worth more.
func copyText(dst []byte, text string, size int) int {
	n := len(text)
	if n > size {
		n = size
	}
	copy(dst[:size+1], make([]byte, size+1))
	copy(dst, text[:n])
	return n
}

// readText reads a NUL-terminated field of at most size bytes.
func readText(src []byte, size int) string {
	b := src[:size+1]
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b[:size])
}
