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
