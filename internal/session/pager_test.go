// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strconv"
	"strings"
	"testing"
)

func TestPaginateEmptyTextIsNoPages(t *testing.T) {
	if got := paginate(""); got != nil {
		t.Errorf("paginate(\"\") = %v, want nil", got)
	}
}

func TestPaginateShortTextIsOnePage(t *testing.T) {
	text := "Line one.\r\nLine two.\r\n"
	got := paginate(text)
	if len(got) != 1 {
		t.Fatalf("got %d pages, want 1", len(got))
	}
	if got[0] != text {
		t.Errorf("page 0 = %q, want the whole text unchanged", got[0])
	}
}

// TestPaginateRejoinsToTheOriginal: whatever the split, concatenating the
// pages must always reproduce the input exactly — next_page's own pointer
// walk never drops or duplicates a byte, and this is the invariant that
// would catch it if this port's version did.
func TestPaginateRejoinsToTheOriginal(t *testing.T) {
	var lines []string
	for i := 1; i <= 60; i++ {
		lines = append(lines, "Line "+strconv.Itoa(i)+".")
	}
	text := strings.Join(lines, "\r\n") + "\r\n"

	pages := paginate(text)
	if len(pages) < 2 {
		t.Fatalf("60 lines produced %d page(s), want at least 2", len(pages))
	}
	if got := strings.Join(pages, ""); got != text {
		t.Errorf("pages do not rejoin to the original text")
	}
}

// TestPaginateBreaksAtPageLength: PAGE_LENGTH is 22 (comm.h:44) — a page
// holds lines while line <= PAGE_LENGTH, breaking only once line exceeds
// it, so 22 lines fit on one page and the 23rd starts a new one.
func TestPaginateBreaksAtPageLength(t *testing.T) {
	line := func(n int) string {
		var lines []string
		for i := 1; i <= n; i++ {
			lines = append(lines, "L")
		}
		return strings.Join(lines, "\r\n") + "\r\n"
	}

	if got := paginate(line(pageLength)); len(got) != 1 {
		t.Errorf("%d lines: got %d page(s), want 1", pageLength, len(got))
	}
	if got := paginate(line(pageLength + 1)); len(got) != 2 {
		t.Errorf("%d lines: got %d page(s), want 2", pageLength+1, len(got))
	}
}

// TestPaginateWrapsOnColumnWidth: a single line with no \r\n at all still
// wraps once it passes PAGE_WIDTH (80) columns, next_page's own col++ >
// PAGE_WIDTH check.
func TestPaginateWrapsOnColumnWidth(t *testing.T) {
	long := strings.Repeat("x", pageWidth*(pageLength+1))
	pages := paginate(long)
	if len(pages) < 2 {
		t.Fatalf("a %d-character line with no newlines produced %d page(s), want at least 2",
			len(long), len(pages))
	}
	if got := strings.Join(pages, ""); got != long {
		t.Error("pages do not rejoin to the original text")
	}
}

// TestPaginateSkipsAnsiCodesWhenCountingColumns: an ANSI escape sequence
// (ESC...m) does not count towards the column width, next_page's own
// spec_code skip — real now that colour.Render can put one there.
func TestPaginateSkipsAnsiCodesWhenCountingColumns(t *testing.T) {
	// 79 visible characters plus a colour code that would push a naive
	// count over PAGE_WIDTH (80) if it were counted; it must not be.
	text := "\x1B[31m" + strings.Repeat("x", 79) + "\x1B[0m\r\n"
	pages := paginate(text)
	if len(pages) != 1 {
		t.Errorf("got %d pages, want 1 — the ANSI codes must not count as columns", len(pages))
	}
}

// TestSessionConnectedNameDuringPaging: users.go calls Session.ConnectedName
// rather than the pure State.ConnectedName precisely so that a reader mid-page
// shows what paging actually interrupted (pagerReturn) rather than the
// fallback "Playing" every other caller of State.ConnectedName has to live
// with — real once `background` could page from the menu rather than only
// from CON_PLAYING. A single Session value, set up directly rather than
// through a live connection: the field this depends on (pagerReturn) is
// Session-private, and nothing about the check needs a socket.
func TestSessionConnectedNameDuringPaging(t *testing.T) {
	// Ordinary case: every paginated command but `background` runs from
	// StatePlaying, so a reader mid-page still shows "Playing".
	ordinary := &Session{state: StatePaging, pagerReturn: StatePlaying}
	if got := ordinary.ConnectedName(); got != "Playing" {
		t.Errorf("ConnectedName() = %q, want %q for an ordinary paginated command", got, "Playing")
	}

	// background's own case: handleMenu sets state to StateReadMOTD before
	// calling SendPaged (menu.go), the same state the C leaves the
	// connection in once background's own paging finishes (CON_RMOTD) —
	// so that is what pagerReturn captures, and what a reader mid-page
	// should show, not "Playing".
	fromBackground := &Session{state: StatePaging, pagerReturn: StateReadMOTD}
	if got := fromBackground.ConnectedName(); got != "Reading MOTD" {
		t.Errorf("ConnectedName() = %q, want %q while background's own page is open", got, "Reading MOTD")
	}

	// Not paging at all: ConnectedName is just State.ConnectedName, same
	// as it always was.
	playing := &Session{state: StatePlaying}
	if got := playing.ConnectedName(); got != "Playing" {
		t.Errorf("ConnectedName() = %q, want %q outside the pager entirely", got, "Playing")
	}
}
