// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/colour"
)

// The pager, porting page_string/show_string/next_page/count_pages/
// paginate_string (modify.c). `credits`, `wizlist`, `immlist`, `news`,
// `info`, `handbook`, `policy`, `motd`, `imotd` and `help` all go through
// it there — every one of do_gen_ps's own SCMD_ branches, plus do_help.
// None of this port's equivalents did until now: every long text was sent
// whole, in one write, which is exactly the gap docs/deviations.md's
// "Nothing paginates" entry names.
//
// The C never changes STATE(d) at all while paging (comm.c:811's
// showstr_count check runs *before* the state switch), so it never has
// to remember what a reader was doing beforehand. This port does have a
// real StatePaging, so sendPaged captures the state it interrupted —
// Session.pagerReturn — and restores it once the last page is shown or
// the reader quits. Every caller gets this for free, `background` (menu
// choice 3, pages from CON_MENU rather than CON_PLAYING) included.

// pageLength and pageWidth are PAGE_LENGTH/PAGE_WIDTH (comm.h:44-45).
const (
	pageLength = 22
	pageWidth  = 80
)

// paginate splits text into terminal-height pages, porting next_page's
// column/line counting exactly: an ANSI escape sequence (ESC...m) does
// not count towards the column width, matching next_page's own spec_code
// skip — colour.Render may have put real ones there now that `set color`
// does something (docs/session/colour.go), where before this text never
// contained one at all.
//
// Returns nil for empty text (page_string's own `if (!str || !*str)`
// early return), and a single-element slice for anything that fits in
// one page — sendPaged sends that whole element directly rather than
// entering StatePaging at all, the same as show_string's own last-page
// branch never being reached for short text.
func paginate(text string) []string {
	if text == "" {
		return nil
	}

	runes := []rune(text)
	var pages []string
	start := 0
	col, line := 1, 1
	specCode := false

	for i := 0; i < len(runes); i++ {
		if line > pageLength {
			pages = append(pages, string(runes[start:i]))
			start = i
			line = 1
		}

		r := runes[i]
		switch {
		case r == '\x1B' && !specCode:
			specCode = true
		case r == 'm' && specCode:
			specCode = false
		case !specCode:
			switch r {
			case '\r':
				col = 1
			case '\n':
				line++
			default:
				// col++ > PAGE_WIDTH in the C: the *old* value decides
				// whether to wrap, and col always advances regardless.
				old := col
				col++
				if old > pageWidth {
					col = 1
					line++
				}
			}
		}
	}
	pages = append(pages, string(runes[start:]))
	return pages
}

// pagingPrompt is make_prompt's own showstr_count branch (comm.c:1067),
// verbatim: "\r[ Return to continue, (q)uit, (r)efresh, (b)ack, or page
// number (%d/%d) ]". pagerIndex is already the C's own d->showstr_page —
// the next page to show, past everything shown so far — by the time this
// is called.
func (s *Session) pagingPrompt() string {
	return fmt.Sprintf("\r[ Return to continue, (q)uit, (r)efresh, (b)ack, or page number (%d/%d) ]",
		s.pagerIndex, len(s.pagerPages))
}

// sendPaged is page_string: render text once, then either send it whole
// (one page or none) or send the first page and enter StatePaging for
// the rest.
func (s *Session) sendPaged(want colour.Level, format string, args ...any) {
	if s.closed.Load() {
		return
	}
	text := format
	if len(args) > 0 {
		text = fmt.Sprintf(format, args...)
	}
	text = colour.Render(text, want, s.colourLevel())
	// next_page's own algorithm only resets column on '\r', matching
	// what the C always has (every in-memory string there is CRLF) —
	// this port's own text/ files are plain LF on disk (§7: "prose
	// stays prose"), so without normalising first, a run of short LF
	// lines would rack up phantom column-overflow line breaks instead
	// of real ones. sendRendered normalises too, at the wire; doing it
	// here as well is what makes count_pages' line counting agree with
	// what actually reaches the screen, not a second, different count.
	text = normalise(text)

	pages := paginate(text)
	if len(pages) <= 1 {
		if len(pages) == 1 {
			s.sendRendered(pages[0])
		}
		return
	}

	s.pagerPages = pages
	s.sendRendered(pages[0])
	s.pagerIndex = 1
	s.pagerReturn = s.state
	s.state = StatePaging
}

// handlePaging is show_string: one line of input while a pager is open.
func (s *Session) handlePaging(line string) error {
	arg := strings.TrimSpace(line)

	switch {
	case strings.EqualFold(arg, "q"):
		s.pagerPages = nil
		s.pagerIndex = 0
		s.state = s.pagerReturn
		s.sendPromptIfPlaying()
		return nil
	case strings.EqualFold(arg, "r"):
		s.pagerIndex = max(0, s.pagerIndex-1)
	case strings.EqualFold(arg, "b"):
		s.pagerIndex = max(0, s.pagerIndex-2)
	case arg != "" && isNumber(arg):
		// isNumber("") is vacuously true (an empty range has no
		// non-digit character to object to) — every other caller in
		// this tree that can see an empty string guards for it
		// explicitly first, and a blank line here has to mean "next
		// page" (the C's own `else` branch), not "page 0".
		s.pagerIndex = max(0, min(int(atoi(arg))-1, len(s.pagerPages)-1))
	case arg != "":
		s.Send("Valid commands while paging are RETURN, Q, R, B, or a numeric value.\r\n")
		return nil
	}

	// The last page: send it and close the pager, same as show_string's
	// own "showstr_page + 1 >= showstr_count" branch.
	if s.pagerIndex+1 >= len(s.pagerPages) {
		s.sendRendered(s.pagerPages[s.pagerIndex])
		s.pagerPages = nil
		s.pagerIndex = 0
		s.state = s.pagerReturn
		s.sendPromptIfPlaying()
		return nil
	}

	s.sendRendered(s.pagerPages[s.pagerIndex])
	s.pagerIndex++
	// Still paging (pagerIndex/pagerPages haven't been cleared), so
	// prompt(s) resolves to pagingPrompt() on its own — the same one
	// Dispatcher.Do's own tail would already show after sendPaged's
	// first page, kept explicit here since handlePaging never goes
	// through that tail at all (login.go's handle() calls it directly).
	s.Send("%s", prompt(s))
	return nil
}

// sendPromptIfPlaying shows the ordinary game prompt once paging ends and
// s.pagerReturn was StatePlaying — the same prompt Dispatcher.Do's own
// tail already shows after any other command. Every other state's own
// handler decides for itself what to print next (StateReadMOTD's on the
// very next line typed, for `background`'s own pager use), the same way
// nanny never prints anything extra outside CON_PLAYING either; sending a
// game-style HP/mana/move prompt there would be nonsense — nobody is
// playing yet.
func (s *Session) sendPromptIfPlaying() {
	if s.state == StatePlaying {
		s.Send("%s", prompt(s))
	}
}
