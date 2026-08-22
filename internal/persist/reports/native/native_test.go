// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package native

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestAppendNeverRefuses(t *testing.T) {
	s, _ := newStore(t)
	for i := 0; i < 5; i++ {
		ok, err := s.Append(reports.Report{Kind: reports.KindBug, Reporter: "Zod", Room: 1, Body: "x"})
		if err != nil || !ok {
			t.Fatalf("Append %d: ok=%v err=%v", i, ok, err)
		}
	}
}

func TestAppendAndAllRoundTrip(t *testing.T) {
	s, _ := newStore(t)
	when := time.Date(2001, 3, 14, 12, 0, 0, 0, time.UTC)
	want := reports.Report{Kind: reports.KindIdea, Reporter: "Zod", Room: 3001, Body: "add a shop here", When: when}

	if _, err := s.Append(want); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := s.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != 1 || got[0] != want {
		t.Errorf("All() = %+v, want [%+v]", got, want)
	}
}

// A report with no When stays that way: Append cannot tell a freshly
// filed report (which should get time.Now()) apart from one imported from
// classic (which genuinely has no recoverable timestamp), so it is not
// Append's decision to make — see native.Store.Append's own doc comment.
func TestAppendLeavesAZeroWhenAlone(t *testing.T) {
	s, _ := newStore(t)
	if _, err := s.Append(reports.Report{Kind: reports.KindBug, Reporter: "Zod", Body: "x"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := s.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != 1 || !got[0].When.IsZero() {
		t.Errorf("All() = %+v, want a zero When", got)
	}
}

func TestReportsSurviveReopening(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Append(reports.Report{Kind: reports.KindTypo, Reporter: "Al", Room: 64, Body: "typo here"}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	again, err := New(reports.Config{Dir: dir})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	got, err := again.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != 1 || got[0].Body != "typo here" {
		t.Errorf("All() after reopening = %+v", got)
	}
}

func TestFileIsCanonical(t *testing.T) {
	s, dir := newStore(t)
	if _, err := s.Append(reports.Report{Kind: reports.KindBug, Reporter: "Zod", Room: 1, Body: "x"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, fileName)); err != nil {
		t.Errorf("expected %s to exist: %v", fileName, err)
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
