// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// Every native_* table must describe exactly the same set of bits or values
// as its display-string counterpart — same length, and a name present in one
// iff it is present in the other. This is §4.1's requirement that a bit
// added to one table and not the other fails a test rather than silently
// becoming unnameable in the file format.
func TestNativeBitTablesMatchDisplayTables(t *testing.T) {
	cases := []struct {
		name           string
		display, ident []string
	}{
		{"room flags", roomBitNames, nativeRoomFlagNames},
		{"mob act flags", actionBitNames, nativeMobActFlagNames},
		{"affect flags", affectBitNames, nativeAffectFlagNames},
		{"item extra flags", extraBitNames, nativeItemExtraFlagNames},
		{"wear flags", wearBitNames, nativeWearFlagNames},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if len(c.display) != len(c.ident) {
				t.Fatalf("%s: display table has %d entries, native table has %d",
					c.name, len(c.display), len(c.ident))
			}
			for i := range c.display {
				displayNamed := c.display[i] != "" && c.display[i] != "*" && c.display[i] != "UNUSED"
				identNamed := c.ident[i] != ""
				if displayNamed != identNamed {
					t.Errorf("%s: bit %d: display=%q (named=%v) but native=%q (named=%v)",
						c.name, i, c.display[i], displayNamed, c.ident[i], identNamed)
				}
			}
		})
	}
}

// Same check for the value-keyed tables (indexed by value, not by bit).
func TestNativeValueTablesMatchDisplayTables(t *testing.T) {
	cases := []struct {
		name           string
		display, ident []string
	}{
		{"sectors", sectorNames, nativeSectorNames},
		{"positions", positionNames, nativePositionNames},
		{"genders", genderNames, nativeSexNames},
		{"apply types", applyTypeNames, nativeApplyTypeNames},
		{"item types", ItemTypeNames, nativeItemTypeNames},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if len(c.display) != len(c.ident) {
				t.Fatalf("%s: display table has %d entries, native table has %d",
					c.name, len(c.display), len(c.ident))
			}
			for i := range c.ident {
				if c.ident[i] == "" {
					t.Errorf("%s: value %d (%q) has no native identifier", c.name, i, c.display[i])
				}
			}
		})
	}
}

func TestNameBitsRoundTrip(t *testing.T) {
	table := []string{"a", "b", "", "d"}
	f := Flags(0b1011) // bits 0, 1, 3 -> "a", "b", "d"
	names, raw := NameBits(f, table)
	if raw != 0 {
		t.Fatalf("raw = %d, want 0", raw)
	}
	got, unknown := ParseBitNames(names, table)
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v, want none", unknown)
	}
	if got != f {
		t.Fatalf("round trip: got %d, want %d", got, f)
	}
}

func TestNameBitsUnnamedBitGoesToRaw(t *testing.T) {
	table := []string{"a", ""} // bit 1 unnamed
	f := Flags(0b11)
	names, raw := NameBits(f, table)
	if len(names) != 1 || names[0] != "a" {
		t.Fatalf("names = %v, want [a]", names)
	}
	if raw != 0b10 {
		t.Fatalf("raw = %d, want 2", raw)
	}
}

func TestNameBitsBeyondTableGoesToRaw(t *testing.T) {
	table := []string{"a"}
	f := Flags(0b101) // bit 2 is past the table
	names, raw := NameBits(f, table)
	if len(names) != 1 || names[0] != "a" {
		t.Fatalf("names = %v, want [a]", names)
	}
	if raw != 0b100 {
		t.Fatalf("raw = %d, want 4", raw)
	}
}

func TestParseBitNamesUnknownReported(t *testing.T) {
	table := []string{"a", "b"}
	_, unknown := ParseBitNames([]string{"a", "nope"}, table)
	if len(unknown) != 1 || unknown[0] != "nope" {
		t.Fatalf("unknown = %v, want [nope]", unknown)
	}
}

func TestNameByValueAndValueByName(t *testing.T) {
	table := []string{"zero", "one", ""}
	if name, ok := NameByValue(1, table); !ok || name != "one" {
		t.Fatalf("NameByValue(1) = %q, %v", name, ok)
	}
	if _, ok := NameByValue(2, table); ok {
		t.Fatal("NameByValue(2) should fail: unnamed slot")
	}
	if _, ok := NameByValue(99, table); ok {
		t.Fatal("NameByValue(99) should fail: out of range")
	}
	if v, ok := ValueByName("one", table); !ok || v != 1 {
		t.Fatalf("ValueByName(one) = %d, %v", v, ok)
	}
	if _, ok := ValueByName("nope", table); ok {
		t.Fatal("ValueByName(nope) should fail")
	}
}
