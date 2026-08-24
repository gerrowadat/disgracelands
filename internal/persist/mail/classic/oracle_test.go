// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/mail"
)

// buildMailOracle compiles reference/tools/mailoracle.c for ILP32 and
// returns the binary's path, skipping if that is not possible.
//
// -m32 is not optional here the way it is inconvenient elsewhere. The block
// size is 100 under every data model but the split inside a block is not
// (79 characters of header text under ILP32, 59 under LP64), so a 64-bit
// oracle would produce a perfectly well-formed file that this package is
// right to disagree with. Better to check nothing than to check that.
func buildMailOracle(t *testing.T) string {
	t.Helper()
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not found; skipping the mail oracle (ilp32)")
	}
	src, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "reference", "tools", "mailoracle.c"))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "mailoracle")
	build := exec.Command(gcc, "-m32", "-std=gnu89", "-w", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("cannot build 32-bit (install gcc-multilib to enable this check, ilp32): %v\n%s", err, out)
	}
	return bin
}

// TestReadsAMessageTheCWrote is the units check the layout test cannot
// make. A block's fields are where maillayout.c says they are; what goes
// *in* the link joining one block to the next is a byte offset into the
// file (push_free_list's own comment, mail.c:76), and no struct layout
// says so.
//
// This port wrote and read a block *index* there instead. That is
// self-consistent, so every round trip through this package passed while
// the package could not read a single message the C wrote that ran past
// one block: the link failed its bounds check and the chain stopped,
// leaving the message truncated to the header block's 79 characters with
// no error anywhere. An archived mail file is what showed it up; this is
// the same thing on demand.
func TestReadsAMessageTheCWrote(t *testing.T) {
	bin := buildMailOracle(t)

	for _, tc := range []struct {
		name string
		text string
	}{
		// Fits the header block, so no link is followed at all: the case
		// that worked before and has to keep working.
		{"one block", strings.Repeat("a", 40)},
		// Two blocks: one link, from the header's next_block.
		{"two blocks", strings.Repeat("b", 120)},
		// Four blocks: the header's link plus two more from data blocks,
		// which the C stores in the block_type field rather than
		// next_block. Both kinds have to be read in the same units.
		{"several blocks", strings.Repeat("c", 300)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "plrmail")
			out, err := exec.Command(bin, path, "1453", "1531", tc.text).CombinedOutput()
			if err != nil {
				t.Fatalf("running the oracle: %v\n%s", err, out)
			}

			s, err := New(mail.Config{Path: path, ReadOnly: true})
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			msgs := s.All()
			if len(msgs) != 1 {
				t.Fatalf("got %d messages, want 1", len(msgs))
			}
			if msgs[0].Text != tc.text {
				t.Errorf("message text:\n got %q (%d bytes)\nwant %q (%d bytes)",
					msgs[0].Text, len(msgs[0].Text), tc.text, len(tc.text))
			}
			if msgs[0].To != 1453 || msgs[0].From != 1531 {
				t.Errorf("to/from: got %d/%d, want 1453/1531", msgs[0].To, msgs[0].From)
			}
		})
	}
}

// TestWritesLinksTheCWouldFollow is the other direction. --lib-dir is the
// same directory either server reads, so a file this port writes has to be
// one the C can read back: the link it stores must be a byte offset, which
// is to say a multiple of the block size that lands on a real block.
func TestWritesLinksTheCWouldFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plrmail")
	s, err := New(mail.Config{Path: path})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.Send(mail.Message{To: 1453, From: 1531, Text: strings.Repeat("d", 300)}); err != nil {
		t.Fatalf("send: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	blocks := len(raw) / BlockSize
	if blocks < 4 {
		t.Fatalf("got %d blocks, want at least 4 for a 300-character message", blocks)
	}

	// The header's next_block, then each data block's own link.
	link := int32(binary.LittleEndian.Uint32(raw[offNextBlock:])) //nolint:gosec // reinterpretation
	for followed := 0; link != lastBlock; followed++ {
		if followed > blocks {
			t.Fatal("the chain does not end")
		}
		if link%BlockSize != 0 {
			t.Fatalf("link %d is not a whole number of blocks; the C refuses these outright "+
				"(write_to_file, mail.c:162)", link)
		}
		if int(link) >= len(raw) {
			t.Fatalf("link %d is past the end of a %d-byte file", link, len(raw))
		}
		link = int32(binary.LittleEndian.Uint32(raw[int(link)+offBlockType:])) //nolint:gosec // reinterpretation
	}
}

// cStyleMailFile builds a mail file the way the C lays one out: ILP32
// fields, and links that are byte offsets. The blocks are handed out in
// descending order, which is not an odd choice made to be awkward — the C's
// free list is a stack, so a file that has had mail read out of it hands
// blocks back in reverse, and a real archived file has its chains running
// backwards through it exactly like this.
//
// This duplicates by hand what mailoracle.c derives from the C itself, and
// the oracle is the authority: if the two ever disagree, this one is wrong.
// It exists because the oracle needs gcc-multilib and so runs only at
// release time, and a bug this quiet should not be able to come back
// between releases.
func cStyleMailFile(to, from int32, text string) []byte {
	blocks := 1
	if len(text) > HeaderTextSize {
		blocks += (len(text) - HeaderTextSize + DataTextSize - 1) / DataTextSize
	}
	file := make([]byte, blocks*BlockSize)

	// Hand out block indices from the top down, header last, the way a
	// stack-shaped free list does.
	order := make([]int, 0, blocks)
	for i := blocks - 1; i >= 0; i-- {
		order = append(order, i)
	}

	head := order[0]
	block := file[head*BlockSize:]
	putInt32(block[offBlockType:], headerBlock)
	putInt32(block[offNextBlock:], lastBlock)
	putInt32(block[offFrom:], from)
	putInt32(block[offTo:], to)
	putInt32(block[offMailTime:], 0)
	n := copyText(block[offHeaderTxt:], text, HeaderTextSize)
	text = text[n:]

	prev := head
	for i := 1; text != ""; i++ {
		next := order[i]
		linkAt := offBlockType
		if prev == head {
			linkAt = offNextBlock
		}
		putInt32(file[prev*BlockSize+linkAt:], int32(next*BlockSize)) //nolint:gosec // a byte offset

		data := file[next*BlockSize:]
		putInt32(data[offBlockType:], lastBlock)
		n = copyText(data[offDataTxt:], text, DataTextSize)
		text = text[n:]
		prev = next
	}
	return file
}

// TestReadsCStyleByteOffsetLinks is TestReadsAMessageTheCWrote without the
// C, so it runs everywhere. See cStyleMailFile.
func TestReadsCStyleByteOffsetLinks(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"one block", strings.Repeat("a", 40)},
		{"exactly one block", strings.Repeat("b", HeaderTextSize)},
		{"two blocks", strings.Repeat("c", HeaderTextSize+10)},
		{"several blocks", strings.Repeat("d", HeaderTextSize+DataTextSize*3)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "plrmail")
			if err := os.WriteFile(path, cStyleMailFile(1453, 1531, tc.text), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}

			s, err := New(mail.Config{Path: path, ReadOnly: true})
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			msgs := s.All()
			if len(msgs) != 1 {
				t.Fatalf("got %d messages, want 1", len(msgs))
			}
			if msgs[0].Text != tc.text {
				t.Errorf("message text:\n got %q (%d bytes)\nwant %q (%d bytes)",
					msgs[0].Text, len(msgs[0].Text), tc.text, len(tc.text))
			}
		})
	}
}
