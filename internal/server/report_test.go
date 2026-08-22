// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/reports"
	reportsclassic "github.com/gerrowadat/disgracelands/internal/persist/reports/classic"
)

func newTestServerWithReports(t *testing.T) (*Server, *reportsclassic.Store) {
	t.Helper()
	srv, _ := newTestServer(t)
	store, err := reportsclassic.New(reports.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("opening the report store: %v", err)
	}
	srv.reports = store
	return srv, store
}

// `bug`, `idea` and `typo` all end to end: do_gen_write (act.other.c:867-
// 924) for each of its three subcommands.
func TestBugIdeaAndTypoAppendReports(t *testing.T) {
	srv, store := newTestServerWithReports(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "password123", "m", "w")

	// "Okay.  Thanks!" is identical for all three, so each has to ask for
	// its own occurrence — expect alone would match the first reply again
	// and return immediately, per CLAUDE.md's testing-traps note.
	c.send("bug the gate is stuck")
	c.expectCount("Okay.  Thanks!", 1)

	c.send("idea add a shop here")
	c.expectCount("Okay.  Thanks!", 2)

	c.send("typo \"recieve\" should be \"receive\"")
	c.expectCount("Okay.  Thanks!", 3)

	all, err := store.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("got %d reports, want 3: %+v", len(all), all)
	}
	for _, r := range all {
		if r.Reporter != "Zod" {
			t.Errorf("report %+v: Reporter = %q, want Zod", r, r.Reporter)
		}
	}
	if all[0].Kind != reports.KindBug || all[0].Body != "the gate is stuck" {
		t.Errorf("report 0 = %+v", all[0])
	}
	if all[1].Kind != reports.KindIdea || all[1].Body != "add a shop here" {
		t.Errorf("report 1 = %+v", all[1])
	}
	if all[2].Kind != reports.KindTypo || all[2].Body != `"recieve" should be "receive"` {
		t.Errorf("report 2 = %+v", all[2])
	}
}

// "That must be a mistake..." (act.other.c:900-903): an empty argument is
// refused, and nothing is written.
func TestBugRefusesAnEmptyArgument(t *testing.T) {
	srv, store := newTestServerWithReports(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "password123", "m", "w")

	c.send("bug")
	c.expect("That must be a mistake...")

	all, err := store.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("got %d reports after an empty argument, want 0", len(all))
	}
}

// max_filesize (config.c:233): once the destination file is full,
// do_gen_write refuses rather than appending past it.
func TestBugRefusesOnceTheFileIsFull(t *testing.T) {
	srv, _ := newTestServer(t)
	dir := t.TempDir()
	store, err := reportsclassic.New(reports.Config{Dir: dir, MaxFileSize: 10})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	srv.reports = store

	c := dialClient(t, listening(t, srv))
	c.create("Zod", "password123", "m", "w")

	c.send("bug this line alone is already past ten bytes")
	c.expect("Okay.  Thanks!")

	c.send("bug a second report")
	c.expect("Sorry, the file is full right now.. try again later.")
}

// A server with no report store configured (the default a test world
// gets) refuses gracefully rather than panicking.
func TestBugWithNoReportsConfigured(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "password123", "m", "w")

	c.send("bug the gate is stuck")
	c.expect("Could not open the file.  Sorry.")
}
