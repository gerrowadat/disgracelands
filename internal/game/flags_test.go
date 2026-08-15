package game

import "testing"

func TestParseFlagsLetterForm(t *testing.T) {
	tests := map[string]Flags{
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
		got, unknown := ParseFlags(in)
		if got != want {
			t.Errorf("ParseFlags(%q) = %#b, want %#b", in, got, want)
		}
		if len(unknown) != 0 {
			t.Errorf("ParseFlags(%q) reported unknown runes %q", in, string(unknown))
		}
	}
}

func TestParseFlagsDecimalForm(t *testing.T) {
	// A field of nothing but digits is a plain number, not letters. This is
	// the branch that makes "128" mean bit 7 rather than nothing at all, and
	// it is checked over the whole string.
	for in, want := range map[string]Flags{
		"0":       0,
		"128":     128,
		"2253040": 2253040,
	} {
		if got, _ := ParseFlags(in); got != want {
			t.Errorf("ParseFlags(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseFlagsMixedIsNotDecimal(t *testing.T) {
	// One letter is enough to take the letter branch, and the digits then
	// contribute nothing — they are neither letters nor an all-digit string.
	got, unknown := ParseFlags("a1")
	if got != 1 {
		t.Errorf("ParseFlags(%q) = %d, want 1 (only the letter counts)", "a1", got)
	}
	if string(unknown) != "1" {
		t.Errorf("ParseFlags(%q) unknown = %q, want %q", "a1", string(unknown), "1")
	}
}

func TestParseFlagsEmpty(t *testing.T) {
	if got, _ := ParseFlags(""); got != 0 {
		t.Errorf("ParseFlags(\"\") = %d, want 0", got)
	}
}

func TestFlagsStringRoundTrip(t *testing.T) {
	// The writer emits the letter form; the reader must return the same bits.
	for _, want := range []Flags{0, 1, 1 << 25, 1 << 26, 1<<0 | 1<<31, 0xDEADBEEF} {
		s := want.String()
		got, unknown := ParseFlags(s)
		if len(unknown) != 0 {
			t.Errorf("Flags(%d).String() = %q, which does not parse cleanly (%q)", want, s, string(unknown))
		}
		if got != want {
			t.Errorf("round trip of %d via %q gave %d", want, s, got)
		}
	}
}

func TestZeroFlagsRenderAsDigitZero(t *testing.T) {
	// An empty field would break the reader, so the writer emits "0".
	if got := Flags(0).String(); got != "0" {
		t.Errorf("Flags(0).String() = %q, want %q", got, "0")
	}
}

func TestExceedsCRange(t *testing.T) {
	// asciiflag_conv computes `1 << (26 + c - 'A')` into an int, so bit 31
	// ('F') is the last one the C server can represent.
	if Flags(1 << 31).ExceedsCRange() {
		t.Error("bit 31 reported as beyond the C range; 'F' maps to it")
	}
	if !Flags(1 << 32).ExceedsCRange() {
		t.Error("bit 32 not reported as beyond the C range; the C shift is undefined there")
	}
}

func TestFlagsHelpers(t *testing.T) {
	f := Flags(0b1010)
	if !f.Has(0b1000) {
		t.Error("Has(0b1000) = false")
	}
	if f.Has(0b1100) {
		t.Error("Has(0b1100) = true, but bit 2 is clear")
	}
	if !f.HasAny(0b1100) {
		t.Error("HasAny(0b1100) = false, but bit 3 is set")
	}
	if got := f.Set(0b0001); got != 0b1011 {
		t.Errorf("Set(0b0001) = %#b, want 0b1011", got)
	}
	if got := f.Clear(0b0010); got != 0b1000 {
		t.Errorf("Clear(0b0010) = %#b, want 0b1000", got)
	}
}
