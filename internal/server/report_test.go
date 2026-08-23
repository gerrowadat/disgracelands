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

// mudlog(buf, CMP, LVL_IMMORT, FALSE) (act.other.c:904-905): bug/idea/typo
// is the one call site wired through obs.WithWizVisEcho, and this is its
// end-to-end proof — the plumbing from a log call, through the wrapped
// handler, into a live session's own socket, not just obs's own unit-level
// guarantee that the callback fires.
func TestBugEchoesInGameToAnImmortalWithSyslogOn(t *testing.T) {
	srv, store := newTestServerWithReports(t)
	addr := listening(t, srv)

	// The first character on the roster is an implementor (twoInARoom's own
	// doc comment, visibility_test.go), so LVL_IMMORT is already met; the
	// default syslog is off (game.ApplyNewCharacterDefaults sets no
	// PrefLog bits), so it has to be turned up before the echo can reach it.
	god := dialClient(t, addr)
	god.create("Warden", "password123", "m", "w")
	god.send("syslog complete")
	god.expect("Your syslog is now complete.")

	reporter := dialClient(t, addr)
	reporter.create("Zod", "password123", "m", "w")

	reporter.send("bug the gate is stuck")
	reporter.expectCount("Okay.  Thanks!", 1)

	// "%s %s: %s" (act.other.c:903) — the exact text mudlog's buf would
	// have been, wrapped in "[ ... ]" and green (utils.c:241,255-257).
	god.expect("Zod bug: the gate is stuck")

	all, err := store.All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d reports, want 1", len(all))
	}
}

// TestBugDoesNotEchoToAnImmortalWithSyslogOff: online, above LVL_IMMORT,
// and still nothing — mudlog()'s own `if (tp < type) continue`
// (utils.c:252-253) against the default, unset syslog preference.
func TestBugDoesNotEchoToAnImmortalWithSyslogOff(t *testing.T) {
	srv, _ := newTestServerWithReports(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Warden", "password123", "m", "w")

	reporter := dialClient(t, addr)
	reporter.create("Zod", "password123", "m", "w")

	reporter.send("bug the gate is stuck")
	reporter.expectCount("Okay.  Thanks!", 1)

	god.settle()
	if god.seen("Zod bug: the gate is stuck") {
		t.Errorf("an immortal with syslog off saw the echo anyway:\n%s", god.transcript())
	}
}

// TestBugDoesNotEchoToAMortal: LVL_IMMORT is the floor, whatever their
// syslog setting says — a mortal reporting their own bug does not get to
// read it back as if they were a god.
func TestBugDoesNotEchoToAMortal(t *testing.T) {
	srv, store := newTestServerWithReports(t)
	addr := listening(t, srv)

	// A second character is a mortal by default (the first on an empty
	// roster is the only implementor made for free).
	first := dialClient(t, addr)
	first.create("Warden", "password123", "m", "w")

	mortal := dialClient(t, addr)
	mortal.create("Bystander", "password123", "f", "w")

	mortal.send("bug the gate is stuck")
	mortal.expectCount("Okay.  Thanks!", 1)

	// Same connection, already past "Okay.  Thanks!" — no separate wait
	// needed, unlike the immortal-on-a-different-connection case above.
	if mortal.seen("Bystander bug: the gate is stuck") {
		t.Errorf("a mortal saw their own bug report echoed as if they were a god:\n%s", mortal.transcript())
	}
	if all, err := store.All(); err != nil || len(all) != 1 {
		t.Errorf("store.All() = %v, %v, want 1 report written regardless", all, err)
	}
}
