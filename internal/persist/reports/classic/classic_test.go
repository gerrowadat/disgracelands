// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/reports"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := New(reports.Config{Dir: dir})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	return s, dir
}

// Append matches do_gen_write's exact fprintf format
// (act.other.c:920-921): "%-8s (%6.6s) [%5d] %s\n".
func TestAppendMatchesTheCsExactFormat(t *testing.T) {
	s, dir := newStore(t)
	when := time.Date(2001, time.August, 9, 12, 0, 0, 0, time.UTC)

	ok, err := s.Append(reports.Report{Kind: reports.KindBug, Reporter: "Zod", Room: 3001, Body: "the gate is stuck", When: when})
	if err != nil || !ok {
		t.Fatalf("Append: ok=%v err=%v", ok, err)
	}

	b, err := os.ReadFile(filepath.Join(dir, "bugs"))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	want := "Zod      (Aug  9) [ 3001] the gate is stuck\n"
	if string(b) != want {
		t.Errorf("wrote %q, want %q", string(b), want)
	}
}

// A name at maxNameLength (20, internal/session/login.go) is not
// truncated, the way %-8s never truncates in the C.
func TestAppendDoesNotTruncateALongName(t *testing.T) {
	s, dir := newStore(t)
	longName := "Abcdefghijklmnopqrst" // 20 characters
	when := time.Date(2001, time.August, 9, 12, 0, 0, 0, time.UTC)

	if _, err := s.Append(reports.Report{Kind: reports.KindIdea, Reporter: longName, Room: 1, Body: "x", When: when}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "ideas"))
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	want := longName + " (Aug  9) [    1] x\n"
	if string(b) != want {
		t.Errorf("wrote %q, want %q", string(b), want)
	}
}

func TestAppendGoesToTheRightFile(t *testing.T) {
	s, dir := newStore(t)
	for _, kind := range []reports.Kind{reports.KindBug, reports.KindIdea, reports.KindTypo} {
		if _, err := s.Append(reports.Report{Kind: kind, Reporter: "Zod", Room: 1, Body: "x"}); err != nil {
			t.Fatalf("Append(%s): %v", kind, err)
		}
	}
	for kind, name := range fileNames {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}
}

// max_filesize (config.c:233) gates further appends once a file is full —
// a refusal, not an error.
func TestAppendRefusesOnceTheFileIsFull(t *testing.T) {
	dir := t.TempDir()
	s, err := New(reports.Config{Dir: dir, MaxFileSize: 10})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}

	ok, err := s.Append(reports.Report{Kind: reports.KindBug, Reporter: "Zod", Room: 1, Body: "the first one is already over ten bytes"})
	if err != nil || !ok {
		t.Fatalf("first Append: ok=%v err=%v", ok, err)
	}

	ok, err = s.Append(reports.Report{Kind: reports.KindBug, Reporter: "Zod", Room: 1, Body: "second"})
	if err != nil {
		t.Fatalf("second Append: %v", err)
	}
	if ok {
		t.Error("Append succeeded on a file already over MaxFileSize, want a refusal")
	}
}

// TestAppendUsesLiveGameTuningWhenNotPinned covers the SIGHUP-reload path:
// a Store opened with Config.MaxFileSize left at zero (every real server
// boot; classic_test.go's other tests pin a size instead) reads
// game.Tuning().MaxFileSize fresh on every Append, so changing the live
// tuning takes effect without reopening the Store.
func TestAppendUsesLiveGameTuningWhenNotPinned(t *testing.T) {
	orig := game.Tuning()
	t.Cleanup(func() { game.SetTuning(orig) })

	tuning := game.DefaultGameTuning()
	tuning.MaxFileSize = 10
	game.SetTuning(tuning)

	s, _ := newStore(t) // Config.MaxFileSize is the zero value here.

	ok, err := s.Append(reports.Report{Kind: reports.KindBug, Reporter: "Zod", Room: 1, Body: "the first one is already over ten bytes"})
	if err != nil || !ok {
		t.Fatalf("first Append: ok=%v err=%v", ok, err)
	}
	ok, err = s.Append(reports.Report{Kind: reports.KindBug, Reporter: "Zod", Room: 1, Body: "second"})
	if err != nil {
		t.Fatalf("second Append: %v", err)
	}
	if ok {
		t.Error("Append succeeded past the live game.Tuning().MaxFileSize, want a refusal")
	}

	// Raising it live, as a SIGHUP reload would, must unblock the very same
	// Store without reopening it.
	tuning.MaxFileSize = 1_000_000
	game.SetTuning(tuning)
	ok, err = s.Append(reports.Report{Kind: reports.KindBug, Reporter: "Zod", Room: 1, Body: "third"})
	if err != nil || !ok {
		t.Fatalf("third Append after raising the live limit: ok=%v err=%v", ok, err)
	}
}

func TestAllParsesBackWhatAppendWrote(t *testing.T) {
	s, _ := newStore(t)
	want := []reports.Report{
		{Kind: reports.KindBug, Reporter: "Zod", Room: 3001, Body: "the gate is stuck"},
		{Kind: reports.KindIdea, Reporter: "Abcdefghijklmnopqrst", Room: 1, Body: "add a shop here"},
		{Kind: reports.KindTypo, Reporter: "Al", Room: 64, Body: "\"recieve\" should be \"receive\""},
	}
	for _, r := range want {
		if _, err := s.Append(r); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := s.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("All returned %d reports, want %d", len(got), len(want))
	}
	for i, g := range got {
		w := want[i]
		if g.Kind != w.Kind || g.Reporter != w.Reporter || g.Room != w.Room || g.Body != w.Body {
			t.Errorf("report %d = %+v, want %+v", i, g, w)
		}
		if !g.When.IsZero() {
			t.Errorf("report %d: When = %v, want zero (classic cannot recover a year)", i, g.When)
		}
	}
}

func TestAllOnAnEmptyDirectory(t *testing.T) {
	s, _ := newStore(t)
	got, err := s.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("All on an empty directory returned %d reports, want 0", len(got))
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	s, err := New(reports.Config{Dir: t.TempDir(), ReadOnly: true})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if _, err := s.Append(reports.Report{Kind: reports.KindBug, Reporter: "Zod", Body: "x"}); err == nil {
		t.Error("a read-only store wrote a report")
	}
}
