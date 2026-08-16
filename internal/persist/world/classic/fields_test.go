// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"math"
	"testing"
)

func TestScanInt(t *testing.T) {
	tests := []struct {
		in   string
		want int32
		ok   bool
	}{
		{"0", 0, true},
		{"3001", 3001, true},
		{"-1", -1, true},
		{"+5", 5, true},
		// sscanf's %d stops at the first character that cannot continue the
		// number rather than rejecting the whole token, and real world files
		// depend on that: zone command lines carry trailing comments.
		{"3001)", 3001, true},
		{"12abc", 12, true},
		{"  7", 7, true},
		// Nothing numeric at all is a failed conversion.
		{"", 0, false},
		{"abc", 0, false},
		{"-", 0, false},
	}
	for _, tt := range tests {
		got, ok := scanInt(tt.in)
		if ok != tt.ok {
			t.Errorf("scanInt(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("scanInt(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestScanIntSaturatesRatherThanWrapping(t *testing.T) {
	// Everything in this format is 32-bit, so the scanner returns int32 and
	// does the range check once rather than leaving a narrowing conversion at
	// every call site. The C code wraps silently here; saturating keeps the
	// value obviously wrong instead of quietly plausible.
	for _, in := range []string{"999999999999999999999999", "2147483648"} {
		got, ok := scanInt(in)
		if !ok {
			t.Fatalf("scanInt(%q) failed entirely", in)
		}
		if got != math.MaxInt32 {
			t.Errorf("scanInt(%q) = %d, want it saturated at %d", in, got, int32(math.MaxInt32))
		}
	}
	got, ok := scanInt("-2147483649")
	if !ok || got != math.MinInt32 {
		t.Errorf("scanInt(\"-2147483649\") = %d, %v; want %d saturated", got, ok, int32(math.MinInt32))
	}
}

func TestScanIntsStopsAtTheFirstFailure(t *testing.T) {
	out := make([]int32, 4)
	if got := scanInts("1 2 x 4", out); got != 2 {
		t.Errorf("scanInts = %d, want 2 (the scan ends at the bad field)", got)
	}
}

func TestScanIntsIgnoresExtraFields(t *testing.T) {
	// "M 0 1 1 1 \t(Puff)" is a real line from data/world/zon/0.zon.
	out := make([]int32, 4)
	if got := scanInts("0 1 1 1 \t(Puff)", out); got != 4 {
		t.Errorf("scanInts = %d, want 4", got)
	}
	for i, want := range []int32{0, 1, 1, 1} {
		if out[i] != want {
			t.Errorf("scanInts filled field %d with %d, want %d", i, out[i], want)
		}
	}
}

func TestRequireIntsReportsWhatItGot(t *testing.T) {
	_, err := requireInts("1 2", 3, "a test")
	if err == nil {
		t.Fatal("requireInts with too few numbers succeeded")
	}
	// The message has to name what was being read and show the line, or a
	// world-file error is unfixable without reading the parser.
	for _, want := range []string{"a test", "1 2"} {
		if !contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestLowerLeadingArticle(t *testing.T) {
	for in, want := range map[string]string{
		"A pelican": "a pelican",
		"An apple":  "an apple",
		"The lake":  "the lake",
		"Puff":      "Puff",      // not an article
		"Apple pie": "Apple pie", // "Apple" is not "a" or "an"
		"a pelican": "a pelican",
		"":          "",
	} {
		if got := lowerLeadingArticle(in); got != want {
			t.Errorf("lowerLeadingArticle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCapitalize(t *testing.T) {
	for in, want := range map[string]string{
		"a pair of wings": "A pair of wings",
		"A pair":          "A pair",
		"":                "",
		"1 thing":         "1 thing",
	} {
		if got := capitalize(in); got != want {
			t.Errorf("capitalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScanDiceLine(t *testing.T) {
	got, err := scanDiceLine("26 1 -1 5d10+550 4d6+3")
	if err != nil {
		t.Fatalf("scanDiceLine: %v", err)
	}
	want := []int32{26, 1, -1, 5, 10, 550, 4, 6, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("field %d = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestScanDiceLineRejectsShortLines(t *testing.T) {
	if _, err := scanDiceLine("26 1 -1 5d10+550"); err == nil {
		t.Error("scanDiceLine accepted a line missing the damage dice")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || len(haystack) >= len(needle) &&
		(haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
