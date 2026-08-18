// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strings"
	"testing"
)

// TestFillWordsAreInvisible. `put sword in bag` and `put sword bag` must parse
// identically, because one_argument drops the preposition — and a player who
// types the sentence out is not doing anything different from one who does
// not.
func TestFillWordsAreInvisible(t *testing.T) {
	for _, line := range []string{
		"sword bag",
		"sword in bag",
		"the sword in the bag",
		"  sword   in    bag  ",
	} {
		// The leftover is whatever follows, unskipped — the C leaves the
		// spaces there too, and the next call is what steps over them.
		first, second, rest := twoArguments(line)
		if first != "sword" || second != "bag" || strings.TrimSpace(rest) != "" {
			t.Errorf("%q parsed as (%q, %q) with %q left over, want (sword, bag)",
				line, first, second, rest)
		}
	}
}

// TestArgumentsAreLowerCased, which is why every keyword match in the game can
// assume it.
func TestArgumentsAreLowerCased(t *testing.T) {
	if first, _ := oneArgument("SWORD"); first != "sword" {
		t.Errorf("got %q, want sword", first)
	}
}

// TestAnArgumentOfNothingButFillWords is empty, not "the".
func TestAnArgumentOfNothingButFillWords(t *testing.T) {
	if first, rest := oneArgument("the on at"); first != "" || rest != "" {
		t.Errorf("got (%q, %q), want two empty strings", first, rest)
	}
}

// TestIsNumber, including the C's empty-string case.
func TestIsNumber(t *testing.T) {
	for word, want := range map[string]bool{
		"5": true, "0": true, "12345": true,
		"":   true, // the C's loop never runs; see is_number
		"5a": false, "-1": false, "a": false, "5.0": false,
	} {
		if got := isNumber(word); got != want {
			t.Errorf("isNumber(%q) = %v, want %v", word, got, want)
		}
	}
}

// TestFindAllDots.
func TestFindAllDots(t *testing.T) {
	for word, want := range map[string]struct {
		mode dotMode
		rest string
	}{
		"sword":     {findIndiv, "sword"},
		"all":       {findAll, "all"},
		"all.sword": {findAllDot, "sword"},
		// "all." with nothing after it is FIND_ALLDOT of the empty string,
		// which is what makes "Get all of what?" reachable.
		"all.": {findAllDot, ""},
		// Not a dot-form: the C compares the first four characters exactly.
		"allsword": {findIndiv, "allsword"},
	} {
		mode, rest := findAllDots(word)
		if mode != want.mode || rest != want.rest {
			t.Errorf("findAllDots(%q) = (%v, %q), want (%v, %q)",
				word, mode, rest, want.mode, want.rest)
		}
	}
}
