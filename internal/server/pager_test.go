// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strconv"
	"strings"
	"testing"
)

// longNews is 40 lines — comfortably more than PAGE_LENGTH (22), so
// `news` pages rather than sending it whole.
func longNews() string {
	var lines []string
	for i := 1; i <= 40; i++ {
		lines = append(lines, "News item "+strconv.Itoa(i)+".")
	}
	return strings.Join(lines, "\r\n") + "\r\n"
}

// TestLongCannedTextPaginates: `news`, long enough to need two pages,
// shows the first with the pager's own prompt, advances on a blank line,
// and the second page lands the player back at the ordinary game prompt
// — end to end, not just paginate()'s own unit-level guarantee.
func TestLongCannedTextPaginates(t *testing.T) {
	srv, _ := newTestServer(t)
	// Server.SetTextField (built for tedit) is the seam: it updates the
	// running server's in-memory news, the same way `tedit news` would,
	// without needing a real file on disk for this one test.
	if !srv.SetTextField("news", longNews()) {
		t.Fatal("SetTextField(news) refused")
	}

	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("news")
	c.expect("News item 1.")
	c.expect("Return to continue")
	if c.seen("News item 23.") {
		t.Error("the first page already shows content past PAGE_LENGTH (22)")
	}

	c.send("")
	c.expect("News item 23.")
	// The last page: back to the ordinary game prompt, not another
	// paging prompt.
	c.expect("V > ")
}

// TestPagerQuitStopsEarly: `q` closes the pager without showing the rest.
func TestPagerQuitStopsEarly(t *testing.T) {
	srv, _ := newTestServer(t)
	if !srv.SetTextField("news", longNews()) {
		t.Fatal("SetTextField(news) refused")
	}

	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("news")
	c.expect("Return to continue")

	c.send("q")
	c.expect("V > ")
	if c.seen("News item 23.") {
		t.Error("q still showed the second page")
	}
}

// TestPagerJumpsToAPageNumber: typing a page number while paging jumps
// straight there, show_string's own isdigit(*buf) branch.
func TestPagerJumpsToAPageNumber(t *testing.T) {
	srv, _ := newTestServer(t)
	if !srv.SetTextField("news", longNews()) {
		t.Fatal("SetTextField(news) refused")
	}

	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("news")
	c.expect("Return to continue")

	c.send("2")
	c.expect("News item 23.")
}

// TestPagerRejectsGarbage: anything besides RETURN/Q/R/B/a page number
// gets show_string's own refusal, and the pager stays open.
func TestPagerRejectsGarbage(t *testing.T) {
	srv, _ := newTestServer(t)
	if !srv.SetTextField("news", longNews()) {
		t.Fatal("SetTextField(news) refused")
	}

	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("news")
	c.expect("Return to continue")

	c.send("xyz")
	c.expect("Valid commands while paging are RETURN, Q, R, B, or a numeric value.")

	// Still open: a follow-up RETURN still advances normally.
	c.send("")
	c.expect("News item 23.")
}

// TestHelpEntryPaginatesAgainstTheRealArchive: `help alias`'s own real
// entry (data/text/help/commands.hlp) runs to 49 lines — comfortably
// over PAGE_LENGTH — so it is real archived content that pages, not a
// synthetic fixture standing in for one.
func TestHelpEntryPaginatesAgainstTheRealArchive(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("help alias")
	c.expect("ALIAS ALIASES")
	c.expect("Return to continue")

	c.send("")
	c.expect("V > ")
}

// TestShortCannedTextDoesNotPaginate: the ordinary case — short text,
// sent whole, no pager prompt at all.
func TestShortCannedTextDoesNotPaginate(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("credits")
	c.expect("CREDITS-FILE")
	if c.seen("Return to continue") {
		t.Error("short credits text triggered the pager")
	}
}
