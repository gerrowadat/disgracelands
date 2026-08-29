// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"strings"
	"testing"
)

// The differ's keyword exception, which is the one field where the yaml
// format does not promise the bytes back — see compareKeywords for why,
// and docs/deviations.md's "The yaml format re-spaces a keyword list".
//
// Found by scripts/fuzz.sh: comparing these as bytes made `import
// --verify` refuse a conversion that had lost nothing, and every fixture
// in this repo has single-spaced keywords, so nothing here had ever shown
// it.
func TestKeywordsCompareAsListsNotBytes(t *testing.T) {
	type extraDesc struct {
		Keywords    string
		Description string
	}

	same := []struct {
		name  string
		a, b  string
		about string
	}{
		{"a doubled space", "cape  wool", "cape wool", "two spaces between two words"},
		{"a trailing space", "cape wool ", "cape wool", "what four real records have"},
		{"a leading space", " cape wool", "cape wool", "the same at the other end"},
		{"a newline", "staircase stair 606\r\nrs", "staircase stair 606 rs", "the fuzz finding"},
		{"a tab", "cape\twool", "cape wool", "any whitespace, not just a space"},
		{"identical", "cape wool", "cape wool", "the ordinary case still passes"},
	}
	for _, c := range same {
		t.Run(c.name, func(t *testing.T) {
			diffs := diffValues(extraDesc{Keywords: c.a}, extraDesc{Keywords: c.b})
			if len(diffs) != 0 {
				t.Errorf("%s (%s) reported a difference, want none:\n%s",
					c.name, c.about, strings.Join(diffs, "\n"))
			}
		})
	}

	// The exception is about whitespace and nothing else. A keyword that is
	// actually gained, lost or misspelled is still a difference, because
	// that one a player can observe: it is what `look staircase` matches.
	differ := []struct {
		name string
		a, b string
	}{
		{"a keyword lost", "cape wool woolen", "cape wool"},
		{"a keyword gained", "cape wool", "cape wool woolen"},
		{"a keyword changed", "cape wool", "cape silk"},
		{"order changed", "cape wool", "wool cape"},
		{"joined into one", "606 rs", "606rs"},
	}
	for _, c := range differ {
		t.Run(c.name, func(t *testing.T) {
			diffs := diffValues(extraDesc{Keywords: c.a}, extraDesc{Keywords: c.b})
			if len(diffs) != 1 {
				t.Errorf("%s reported %d difference(s), want 1: %v", c.name, len(diffs), diffs)
			}
		})
	}

	// The exception is keyed on the field, not on the string: every other
	// string in the world data is compared byte for byte, which is the
	// whole point of the corpus (a trailing blank line is a blank line on
	// a player's screen).
	t.Run("only the Keywords field", func(t *testing.T) {
		diffs := diffValues(
			extraDesc{Keywords: "cape", Description: "A cape.  "},
			extraDesc{Keywords: "cape", Description: "A cape."},
		)
		if len(diffs) != 1 {
			t.Errorf("a description differing only in trailing space reported %d difference(s), want 1: %v",
				len(diffs), diffs)
		}
	})
}

// Two lists of the same vnum-keyed records in a different order are the
// one shape the positional walk describes worst, and the shape a per-zone
// yaml directory can actually produce: it loads zone files in vnum order,
// so a record written under the wrong zone comes back in the wrong place
// with nothing missing. Importing the archived Disgracelands lib/ hit it
// with 77 shops and reported 200 field mismatches for it.
func TestReorderedRecordsAreReportedAsAReordering(t *testing.T) {
	type shop struct {
		Vnum   int32
		Keeper int32
	}
	type world struct{ Shops []*shop }

	a, b, c := &shop{Vnum: 190, Keeper: 19004}, &shop{Vnum: 2505, Keeper: 2505}, &shop{Vnum: 3000, Keeper: 3000}

	t.Run("a pure reordering is one line", func(t *testing.T) {
		diffs := diffValues(world{Shops: []*shop{b, c, a}}, world{Shops: []*shop{a, b, c}})
		if len(diffs) != 1 {
			t.Fatalf("got %d difference(s), want 1:\n%s", len(diffs), strings.Join(diffs, "\n"))
		}
		for _, want := range []string{"the same 3 record(s) in a different order", "#2505 is 1st on the left and 2nd on the right"} {
			if !strings.Contains(diffs[0], want) {
				t.Errorf("report does not mention %q:\n%s", want, diffs[0])
			}
		}
	})

	// The reason this is safe rather than merely quieter: a record that has
	// both moved and changed still reports the change, against its own
	// counterpart and under a path that names the vnum rather than a
	// position that now means nothing.
	t.Run("a record that also changed still reports", func(t *testing.T) {
		moved := &shop{Vnum: 2505, Keeper: 9999}
		diffs := diffValues(world{Shops: []*shop{b, c, a}}, world{Shops: []*shop{a, moved, c}})
		if len(diffs) != 2 {
			t.Fatalf("got %d difference(s), want 2:\n%s", len(diffs), strings.Join(diffs, "\n"))
		}
		if want := "Shops[#2505].Keeper: 2505 vs 9999"; !strings.Contains(diffs[1], want) {
			t.Errorf("got %q, want it to contain %q", diffs[1], want)
		}
	})

	// A different *set* of records is not a reordering, and pairing them up
	// by vnum would hide the fact that one is not there at all.
	t.Run("a different set falls back to positions", func(t *testing.T) {
		other := &shop{Vnum: 4201, Keeper: 4203}
		diffs := diffValues(world{Shops: []*shop{a, b, c}}, world{Shops: []*shop{a, b, other}})
		if len(diffs) == 0 {
			t.Fatal("got no differences, want the third element to disagree")
		}
		if !strings.Contains(diffs[0], "Shops[2]") {
			t.Errorf("got %q, want a positional path", diffs[0])
		}
	})

	// Same order, same records: still nothing to say.
	t.Run("identical lists are identical", func(t *testing.T) {
		diffs := diffValues(world{Shops: []*shop{a, b, c}}, world{Shops: []*shop{a, b, c}})
		if len(diffs) != 0 {
			t.Errorf("got %d difference(s), want none:\n%s", len(diffs), strings.Join(diffs, "\n"))
		}
	})
}
