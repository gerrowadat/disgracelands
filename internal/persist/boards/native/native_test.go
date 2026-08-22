// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package native

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
		{Heading: "Aug 20 2026 (Nobody)      :: silence", Level: 1},
	}
}

func TestBoardsRoundTrip(t *testing.T) {
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
	if err := s.Save("board.immort", sample()[:1]); err != nil {
		t.Fatalf("saving: %v", err)
	}

	again, err := New(boards.Config{Dir: dir})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	got, err := again.Load("board.mort")
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	want := sample()
	if len(got) != len(want) {
		t.Fatalf("read %d messages, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("message %d round-tripped as %+v, want %+v", i+1, got[i], want[i])
		}
	}
	if got, err := again.Load("board.immort"); err != nil || len(got) != 1 {
		t.Errorf("board.immort: got %v, %v, want 1 message", got, err)
	}
}

// An emptied board leaves no trace in the file, matching classic's own
// "Board_save_board removes rather than writes empty" rule.
func TestEmptyingABoardRemovesItsEntry(t *testing.T) {
	dir := t.TempDir()
	s, err := New(boards.Config{Dir: dir})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := s.Save("board.mort", sample()); err != nil {
		t.Fatalf("saving: %v", err)
	}
	if err := s.Save("board.mort", nil); err != nil {
		t.Fatalf("emptying: %v", err)
	}
	if _, err := s.Load("board.mort"); !errors.Is(err, boards.ErrNotFound) {
		t.Errorf("an emptied board loaded as %v, want ErrNotFound", err)
	}

	b, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if string(b) != "schema: dl/boards@1\n" {
		t.Errorf("the file still mentions the emptied board: %q", b)
	}
}

func TestAMissingFileIsNoBoards(t *testing.T) {
	s, err := New(boards.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if _, err := s.Load("board.mort"); !errors.Is(err, boards.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
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

// The written file's board keys come out sorted, verified rather than
// assumed: goccy/go-yaml's own map-marshalling behaviour, not a guarantee
// this package makes itself (nothing here sorts by hand). This is what
// makes the file's diffs meaningful and dlctl state fmt idempotent.
func TestWrittenFileHasSortedBoardKeys(t *testing.T) {
	dir := t.TempDir()
	s, err := New(boards.Config{Dir: dir})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	// Saved out of alphabetical order on purpose.
	for _, name := range []string{"board.social", "board.freeze", "board.immort"} {
		if err := s.Save(name, sample()[:1]); err != nil {
			t.Fatalf("saving %s: %v", name, err)
		}
	}

	b, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	freeze := indexOf(t, string(b), "board.freeze:")
	immort := indexOf(t, string(b), "board.immort:")
	social := indexOf(t, string(b), "board.social:")
	if freeze >= immort || immort >= social {
		t.Errorf("board keys are not sorted in the written file:\n%s", b)
	}
}

func indexOf(t *testing.T, haystack, needle string) int {
	t.Helper()
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	t.Fatalf("%q not found in:\n%s", needle, haystack)
	return -1
}
