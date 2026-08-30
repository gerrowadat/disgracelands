// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

func TestParseFlagsLetterForm(t *testing.T) {
	tests := map[string]uint64{
		"a":  1 << 0,
		"b":  1 << 1,
		"z":  1 << 25,
		"A":  1 << 26,
		"F":  1 << 31,
		"ae": 1<<0 | 1<<4,
		// Order does not matter and repeats are harmless.
		"ea":      1<<0 | 1<<4,
		"aa":      1 << 0,
		"adnopqr": 1<<0 | 1<<3 | 1<<13 | 1<<14 | 1<<15 | 1<<16 | 1<<17,
	}
	for in, want := range tests {
		got, unknown := ParseFlagLetters(in)
		if got != want {
			t.Errorf("ParseFlagLetters(%q) = %#b, want %#b", in, got, want)
		}
		if len(unknown) != 0 {
			t.Errorf("ParseFlagLetters(%q) reported unknown runes %q", in, string(unknown))
		}
	}
}

func TestParseFlagsDecimalForm(t *testing.T) {
	// A field of nothing but digits is a plain number, not letters. This is
	// the branch that makes "128" mean bit 7 rather than nothing at all, and
	// it is checked over the whole string.
	for in, want := range map[string]uint64{
		"0":       0,
		"128":     128,
		"2253040": 2253040,
	} {
		if got, _ := ParseFlagLetters(in); got != want {
			t.Errorf("ParseFlagLetters(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseFlagsMixedIsNotDecimal(t *testing.T) {
	// One letter is enough to take the letter branch, and the digits then
	// contribute nothing — they are neither letters nor an all-digit string.
	got, unknown := ParseFlagLetters("a1")
	if got != 1 {
		t.Errorf("ParseFlagLetters(%q) = %d, want 1 (only the letter counts)", "a1", got)
	}
	if string(unknown) != "1" {
		t.Errorf("ParseFlagLetters(%q) unknown = %q, want %q", "a1", string(unknown), "1")
	}
}

func TestParseFlagsEmpty(t *testing.T) {
	if got, _ := ParseFlagLetters(""); got != 0 {
		t.Errorf("ParseFlagLetters(\"\") = %d, want 0", got)
	}
}

func TestFlagsStringRoundTrip(t *testing.T) {
	// The writer emits the letter form; the reader must return the same bits.
	for _, want := range []uint64{0, 1, 1 << 25, 1 << 26, 1<<0 | 1<<31, 0xDEADBEEF} {
		s := FlagLetters(want)
		got, unknown := ParseFlagLetters(s)
		if len(unknown) != 0 {
			t.Errorf("FlagLetters(%d) = %q, which does not parse cleanly (%q)", want, s, string(unknown))
		}
		if got != want {
			t.Errorf("round trip of %d via %q gave %d", want, s, got)
		}
	}
}

func TestZeroFlagsRenderAsDigitZero(t *testing.T) {
	// An empty field would break the reader, so the writer emits "0".
	if got := FlagLetters(0); got != "0" {
		t.Errorf("FlagLetters(0) = %q, want %q", got, "0")
	}
}
