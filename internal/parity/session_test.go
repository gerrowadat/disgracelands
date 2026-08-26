// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package parity

import (
	"strings"
	"testing"
)

// TestNormaliseTheMudClock pins the substitution that a green run of the
// session-parity suite depends on, and that a green run is a bad place to
// find out about.
//
// The two transcripts are taken one server after the other, about fifteen
// seconds apart. A mud hour is 75 real seconds and a mud day 1800
// (utils.h:109-110), so the hour between them differs about a fifth of the
// time and the day about one run in a hundred and twenty — and the second of
// those is what actually happened, once, a day after the suite first went
// green. Both lines have to collapse to the same text regardless of when
// each server was asked, or the suite reports a difference the port does not
// have.
func TestNormaliseTheMudClock(t *testing.T) {
	// Two transcripts of the same `time`, straddling a mud-day rollover:
	// the day moves, and the weekday moves with it because the C derives
	// one from the other (act.informative.c:896).
	c := "It is 11 o'clock pm, on the Day of Freedom\r\n" +
		"The 25th Day of the Month of Winter, Year 1062.\r\n"
	g := "It is 12 o'clock am, on the Day of the Great Gods\r\n" +
		"The 26th Day of the Month of Winter, Year 1062.\r\n"

	if got, want := Normalise(c), Normalise(g); got != want {
		t.Errorf("a mud-day rollover between the two transcripts survived normalisation:\n C: %q\nGo: %q", got, want)
	}
}

// TestNormaliseKeepsTheRestOfTheLine checks the clock patterns are anchored
// to their own lines rather than eating what follows.
//
// Both substitutions end in `.*`, which is what lets them absorb a weekday
// and a month name without listing either — and `.` matches everything but a
// newline, so the risk they carry is swallowing the next thing the server
// said rather than failing to match.
func TestNormaliseKeepsTheRestOfTheLine(t *testing.T) {
	got := Normalise("It is 3 o'clock pm, on the Day of the Sun\r\n" +
		"The 1st Day of the Month of Winter, Year 1062.\r\n" +
		"The Temple Of Midgaard\r\n")

	if !strings.Contains(got, "The Temple Of Midgaard") {
		t.Errorf("the line after the date was eaten:\n%q", got)
	}
	for _, gone := range []string{"Day of the Sun", "1st Day", "Year 1062"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q survived normalisation:\n%q", gone, got)
		}
	}
}
