// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/persist/mail"
)

// The mail file's block links, against files a C server wrote.
//
// This package's own round-trip tests cannot see the bug these catch. It read
// a link as a block index and wrote it as one, and the two agree with each
// other perfectly — so the store round-tripped, `dlctl state import`
// round-tripped, and every message in the archived plrmail from the real
// server truncated silently at its header block's 79 characters. The link is
// a byte offset (mail.c:349, :476); see blockOffset.
//
// testdata/plrmail and testdata/plrmail-append-only are the output of
// reference/tools/mailgen.c, which is store_mail lifted out of mail.c. The
// last test here rebuilds them and requires the bytes to match, so the
// fixtures cannot drift away from what the C does; the other two run
// everywhere and are what actually stops the regression.

const (
	fixtureFull       = "testdata/plrmail"
	fixtureAppendOnly = "testdata/plrmail-append-only"
)

// The three messages both fixtures start with. Each run of characters is one
// block's worth, so a chain followed wrongly shows up as the wrong letter and
// not merely as a length.
var (
	fixtureHello = mail.Message{
		To: 7, From: 3, Sent: time.Unix(1_700_000_000, 0).UTC(),
		Text: "hello",
	}
	fixtureThreeBlock = mail.Message{
		To: 1, From: 2, Sent: time.Unix(1_700_000_060, 0).UTC(),
		Text: strings.Repeat("a", HeaderTextSize) + strings.Repeat("b", DataTextSize) + strings.Repeat("c", 19),
	}
	fixtureTwoBlock = mail.Message{
		To: 9, From: 4, Sent: time.Unix(1_700_000_120, 0).UTC(),
		Text: strings.Repeat("d", HeaderTextSize) + strings.Repeat("e", 5),
	}
	// Stored into the gap left by reading the two-block message, so its
	// chain runs block 5 -> 4 -> 6 -> 7: pop_free_list is LIFO and only
	// grows the file once the free list is empty.
	fixtureReused = mail.Message{
		To: 11, From: 5, Sent: time.Unix(1_700_000_180, 0).UTC(),
		Text: strings.Repeat("1", HeaderTextSize) + strings.Repeat("2", DataTextSize) +
			strings.Repeat("3", DataTextSize) + strings.Repeat("4", 20),
	}
)

// openFixture copies a fixture somewhere writable, since Receive rewrites the
// file, and opens it.
func openFixture(t *testing.T, name string) *Store {
	t.Helper()

	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "plrmail")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("copying the fixture: %v", err)
	}
	s, err := New(mail.Config{Path: path})
	if err != nil {
		t.Fatalf("opening the fixture: %v", err)
	}
	return s
}

func checkMessage(t *testing.T, got, want mail.Message) {
	t.Helper()

	if got.To != want.To || got.From != want.From {
		t.Errorf("message is from %d to %d, want from %d to %d", got.From, got.To, want.From, want.To)
	}
	if !got.Sent.Equal(want.Sent) {
		t.Errorf("message to %d is dated %s, want %s", want.To, got.Sent, want.Sent)
	}
	if got.Text != want.Text {
		t.Errorf("message to %d came back %d characters, want %d:\n got %q\nwant %q",
			want.To, len(got.Text), len(want.Text), got.Text, want.Text)
	}
}

// Every message in a file the C wrote reads back whole, chains and all.
func TestAFileTheCWroteReadsBack(t *testing.T) {
	s := openFixture(t, fixtureFull)

	all := s.All()
	if len(all) != 3 {
		t.Fatalf("the fixture holds %d messages, want 3", len(all))
	}
	// All returns headers in ascending block order: blocks 0, 1 and 5. The
	// message in block 5 is the reused one, whose chain descends first.
	checkMessage(t, all[0], fixtureHello)
	checkMessage(t, all[1], fixtureThreeBlock)
	checkMessage(t, all[2], fixtureReused)

	for _, want := range []mail.Message{fixtureHello, fixtureThreeBlock, fixtureReused} {
		got, ok, err := s.Receive(want.To)
		if err != nil || !ok {
			t.Fatalf("receiving for %d: ok=%v err=%v", want.To, ok, err)
		}
		checkMessage(t, got, want)
	}
}

// Reading a message out of a C-written file frees every block of its chain,
// wherever in the file they are — so a four-block message stored into the
// space left behind is four blocks of space again, and the file does not grow.
func TestReadingAFileTheCWroteFreesTheWholeChain(t *testing.T) {
	s := openFixture(t, fixtureFull)

	if _, _, err := s.Receive(11); err != nil {
		t.Fatalf("receiving the reused message: %v", err)
	}

	// Blocks 4, 5, 6 and 7 are free now. Four blocks' worth of new mail
	// should fit in them without the file growing past its eight blocks.
	send(t, s, 12, 13, strings.Repeat("z", HeaderTextSize+3*DataTextSize))
	info, err := os.Stat(s.path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := int64(8 * BlockSize); info.Size() != want {
		t.Errorf("the file is %d bytes, want %d — the freed chain was not all reused", info.Size(), want)
	}

	got, ok, err := s.Receive(12)
	if err != nil || !ok {
		t.Fatalf("receiving what we just sent: ok=%v err=%v", ok, err)
	}
	if want := strings.Repeat("z", HeaderTextSize+3*DataTextSize); got.Text != want {
		t.Errorf("the new message came back %d characters, want %d", len(got.Text), len(want))
	}
}

// The other half: what this package writes is byte for byte what the C wrote.
//
// Only for the append-only fixture. Once a block has been freed the two
// allocators disagree about which one comes back — the C's free list is LIFO
// and this scans for the lowest — which is a documented deviation, so the
// file with reuse in it is not comparable and only its reader is checked.
func TestWhatThisPackageWritesIsWhatTheCWrote(t *testing.T) {
	want, err := os.ReadFile(fixtureAppendOnly)
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}

	s, path := newStore(t)
	for _, m := range []mail.Message{fixtureHello, fixtureThreeBlock, fixtureTwoBlock} {
		if err := s.Send(m); err != nil {
			t.Fatalf("sending to %d: %v", m.To, err)
		}
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading what we wrote: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the file this package wrote is not the file the C wrote:%s", diffBlocks(got, want))
	}
}

// diffBlocks reports the first differing byte of each differing block, which
// is more use than a hexdump of eight hundred bytes.
func diffBlocks(got, want []byte) string {
	var b strings.Builder
	if len(got) != len(want) {
		fmt.Fprintf(&b, "\n  length %d, want %d", len(got), len(want))
	}
	n := min(len(got), len(want))
	for off := 0; off+BlockSize <= n; off += BlockSize {
		g, w := got[off:off+BlockSize], want[off:off+BlockSize]
		if bytes.Equal(g, w) {
			continue
		}
		for i := range g {
			if g[i] != w[i] {
				fmt.Fprintf(&b, "\n  block %d byte %d: %d, want %d", off/BlockSize, i, g[i], w[i])
				break
			}
		}
	}
	return b.String()
}

// The fixtures are regenerated and required to match, so they cannot drift
// away from what the C actually writes.
//
// Needs `gcc -m32` (gcc-multilib on Debian), same as the layout check, and
// for the same reason: HEADER_BLOCK_DATASIZE is 79 under ILP32 and 59 under
// LP64, so a 64-bit build produces a file of exactly the right length with
// every message garbled in the middle. Skips where it cannot build, which is
// why the two tests above read the checked-in bytes rather than these.
func TestTheFixturesAreWhatTheCStillProduces(t *testing.T) {
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("gcc not found; skipping the mail fixture regeneration check (ilp32)")
	}

	src, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "reference", "tools", "mailgen.c"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "mailgen")
	if out, err := exec.Command(gcc, "-m32", "-std=gnu89", "-w", "-o", bin, src).CombinedOutput(); err != nil {
		t.Skipf("cannot build 32-bit (install gcc-multilib to enable this check, ilp32): %v\n%s", err, out)
	}

	full := filepath.Join(dir, "plrmail")
	appendOnly := filepath.Join(dir, "plrmail-append-only")
	if out, err := exec.Command(bin, full, appendOnly).CombinedOutput(); err != nil {
		t.Fatalf("running mailgen: %v\n%s", err, out)
	}

	for _, pair := range [][2]string{{full, fixtureFull}, {appendOnly, fixtureAppendOnly}} {
		fresh, err := os.ReadFile(pair[0])
		if err != nil {
			t.Fatalf("reading the freshly generated file: %v", err)
		}
		stored, err := os.ReadFile(pair[1])
		if err != nil {
			t.Fatalf("reading the fixture: %v", err)
		}
		if !bytes.Equal(fresh, stored) {
			t.Errorf("%s is not what mailgen.c now produces; regenerate it:%s", pair[1], diffBlocks(stored, fresh))
		}
	}
	t.Log("regenerated both mail fixtures with the 32-bit C and they match (ilp32)")
}
