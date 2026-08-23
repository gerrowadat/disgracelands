// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package colour

import "testing"

// TestRenderIsAThreshold. The level is not a palette: a message says how much
// colour it is, and a reader who asked for less gets none of it.
func TestRenderIsAThreshold(t *testing.T) {
	const msg = "{{cyan}}The Temple{{/}}"
	for _, tc := range []struct {
		want, have Level
		coloured   bool
	}{
		{Normal, Off, false},
		{Normal, Sparse, false},
		{Normal, Normal, true},
		{Normal, Complete, true},
		// The combat messages are C_CMP, so "normal" sees them plain.
		{Complete, Normal, false},
		{Complete, Complete, true},
		// And a C_SPR message reaches anybody with any colour at all.
		{Sparse, Sparse, true},
		{Sparse, Off, false},
	} {
		got := Render(msg, tc.want, tc.have)
		if coloured := got != "The Temple"; coloured != tc.coloured {
			t.Errorf("Render(want=%v, have=%v) = %q, coloured=%v want %v",
				tc.want, tc.have, got, coloured, tc.coloured)
		}
	}
}

// TestRenderLeavesUnknownTokensAlone, because a player typing braces on a
// bulletin board is far likelier than a bad format string, and eating their
// text is worse than showing it.
func TestRenderLeavesUnknownTokensAlone(t *testing.T) {
	for _, s := range []string{
		"{{puce}}hello", "{{unterminated hello", "a plain {{ brace", "}}backwards{{",
	} {
		if got := Render(s, Normal, Complete); got != s {
			t.Errorf("Render(%q) = %q, want it unchanged", s, got)
		}
	}
}

// TestLevelBitsRoundTrip. The two preference bits are read as a two-bit
// number, which is why the C writes the assignment as arithmetic — they are
// not adjacent in the flags word.
func TestLevelBitsRoundTrip(t *testing.T) {
	for _, l := range []Level{Off, Sparse, Normal, Complete} {
		one, two := Bits(l)
		if got := LevelOf(one, two); got != l {
			t.Errorf("%v round-tripped to %v", l, got)
		}
	}
}

// TestParseLevel matches a prefix, because `search_block` is called with an
// exact-match flag of FALSE.
func TestParseLevel(t *testing.T) {
	for word, want := range map[string]Level{
		"off": Off, "o": Off, "s": Sparse, "n": Normal,
		"c": Complete, "comp": Complete, "COMPLETE": Complete,
	} {
		got, ok := ParseLevel(word)
		if !ok || got != want {
			t.Errorf("ParseLevel(%q) = (%v, %v), want (%v, true)", word, got, ok, want)
		}
	}
	for _, word := range []string{"", "purple", "x"} {
		if _, ok := ParseLevel(word); ok {
			t.Errorf("ParseLevel(%q) matched and should not", word)
		}
	}
}

// TestStrip takes the markup out without rendering it, for a log line or a
// saved description.
func TestStrip(t *testing.T) {
	if got := Strip("{{red}}danger{{/}}"); got != "danger" {
		t.Errorf("Strip = %q", got)
	}
}
