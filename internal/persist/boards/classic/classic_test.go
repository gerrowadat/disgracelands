// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/boards"
)

func sample() []boards.Message {
	return []boards.Message{
		{Heading: "Aug 20 2026 (Zod)         :: the first post", Level: 34, Body: "Hello.\r\n"},
		{Heading: "Aug 20 2026 (Welmar)      :: a reply", Level: 10, Body: "Hello yourself.\r\n"},
		// A message with no body at all: the C stores message_len 0 and
		// reads it back as a NULL, which prints as "That message seems to be
		// empty."
		{Heading: "Aug 20 2026 (Nobody)      :: silence", Level: 1},
	}
}

func TestABoardSurvivesBeingWrittenAndReadBack(t *testing.T) {
	want := sample()
	got, err := decode(encode(want))
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("read %d messages, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("message %d round-tripped as %+v, want %+v", i+1, got[i], want[i])
		}
	}
}

// The size of a message on disk is the struct plus both strings with their
// terminators, and a body of zero length contributes nothing at all.
func TestTheEncodedSizeIsWhatTheCWouldWrite(t *testing.T) {
	msgs := sample()
	want := countSize
	for _, m := range msgs {
		want += msgInfoSize + len(m.Heading) + 1
		if m.Body != "" {
			want += len(m.Body) + 1
		}
	}
	if got := len(encode(msgs)); got != want {
		t.Errorf("encoded to %d bytes, want %d", got, want)
	}
}

func TestAShortOrLyingFileIsAnError(t *testing.T) {
	full := encode(sample())

	for _, tc := range []struct {
		name string
		in   []byte
	}{
		{"nothing at all", nil},
		{"a count with no messages", full[:countSize]},
		{"a truncated index", full[:countSize+msgInfoSize/2]},
		{"a body that runs off the end", full[:len(full)-4]},
	} {
		if _, err := decode(tc.in); err == nil {
			t.Errorf("decoding %s succeeded, want an error", tc.name)
		}
	}
}

// A heading length of zero is what the C calls corrupt, because it always
// writes the terminator with it.
func TestAZeroHeadingLengthIsCorruption(t *testing.T) {
	b := encode(sample()[:1])
	byteOrder.PutUint32(b[countSize+offHeadingLen:], 0)
	if _, err := decode(b); err == nil {
		t.Error("a zero heading length decoded cleanly, want an error")
	}
}

func TestTheStoreWritesAndReadsAndRemoves(t *testing.T) {
	dir := t.TempDir()
	s, err := New(boards.Config{Dir: dir})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}

	if _, err := s.Load("board.mort"); !errors.Is(err, boards.ErrNotFound) {
		t.Errorf("loading a board with no file gave %v, want ErrNotFound", err)
	}

	if err := s.Save("board.mort", sample()); err != nil {
		t.Fatalf("saving: %v", err)
	}
	got, err := s.Load("board.mort")
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("read %d messages, want 3", len(got))
	}

	// An emptied board leaves no file, which is Board_save_board's first
	// branch: `if (!num_of_msgs) { remove(...); return; }`.
	if err := s.Save("board.mort", nil); err != nil {
		t.Fatalf("saving an empty board: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "board.mort")); !os.IsNotExist(err) {
		t.Error("emptying a board left the file behind")
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	s, err := New(boards.Config{Dir: t.TempDir(), ReadOnly: true})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := s.Save("board.mort", sample()); err == nil {
		t.Error("a read-only store wrote a board")
	}
}
