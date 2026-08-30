// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestSyllableTableMatchesTheCSource re-parses syls[] out of spell_parser.c
// and compares it row by row, in order.
//
// In order, because the order *is* the behaviour: ScrambleSpellName takes the
// first row that matches at each offset, so {"ar", "abra"} has to come before
// {"a", "i"} or "ar" is read as two letters and the words change. A
// transcription that got the pairs right and the order wrong would produce
// plausible gibberish, which is the one kind of wrong nobody would notice --
// the output is meant to be unreadable.
//
// The same reasoning as the command table being sorted by interpreter.c line
// number: derive the behaviour from the C rather than assert about it.
func TestSyllableTableMatchesTheCSource(t *testing.T) {
	b, err := os.ReadFile(spellParserSource)
	if err != nil {
		t.Fatalf("reading spell_parser.c: %v", err)
	}
	src := string(b)

	start := strings.Index(src, "struct syllable syls[] = {")
	if start < 0 {
		t.Fatal("no syls[] in spell_parser.c")
	}
	end := strings.Index(src[start:], "\n};")
	if end < 0 {
		t.Fatal("syls[] has no end")
	}

	pair := regexp.MustCompile(`\{"([^"]*)",\s*"([^"]*)"\}`)
	var want []syllable
	for _, m := range pair.FindAllStringSubmatch(src[start:start+end], -1) {
		// The table is terminated by {"", ""}, which is the loop's sentinel
		// and not a row: say_spell stops at `!*syls[j].org`.
		if m[1] == "" {
			break
		}
		want = append(want, syllable{org: m[1], news: m[2]})
	}

	if len(want) != len(syllables) {
		t.Fatalf("the C has %d syllables, this table has %d", len(want), len(syllables))
	}
	for i := range want {
		if syllables[i] != want[i] {
			t.Errorf("row %d is %v, want %v", i, syllables[i], want[i])
		}
	}
}

// TestScramblingASpellName is the substitution loop itself, on names from the
// table. Each expectation is what the C's loop produces for that name,
// derived by hand from the rows above and checked against the ordering rule
// the table's own comment describes.
func TestScramblingASpellName(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		// "ar" beats "a", and then "mor" beats "m": three rows for five
		// letters. Derived by hand as "abrawaf" first, which was wrong --
		// "mor" has a row and it is easy to read past. CLAUDE.md's "do not
		// read the C and transcribe it" applies to a table walk as much as
		// to arithmetic.
		{"armor", "abrazak"},
		// "magi" then "c", and " " has a row of its own.
		{"magic missile", "kariq wugguro"},
		// "blind" is one row, "ness" another: a name can be two syllables
		// and eleven letters.
		{"blindness", "noselacri"},
		// "word of" is the longest row in the table, and it contains a
		// space -- so it has to be tried before the space row that
		// precedes everything else.
		{"word of recall", "inset candusqirr"},
		// Nothing at all is not an error.
		{"", ""},
	} {
		if got := ScrambleSpellName(tc.name); got != tc.want {
			t.Errorf("ScrambleSpellName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A character no row matches is dropped rather than copied, which is what the
// C does: it logs and advances the offset without writing anything.
func TestScramblingDropsWhatItCannotSay(t *testing.T) {
	// Every row is lower case and strncmp is case-sensitive, so an
	// upper-case letter matches nothing either -- all three characters here
	// are dropped and the result is empty. No spell name in the C's table
	// reaches this; a hand-added one with a capital or an apostrophe would,
	// and would come out shorter rather than wrong.
	if got := ScrambleSpellName("A-B"); got != "" {
		t.Errorf("ScrambleSpellName(%q) = %q, want %q — nothing here has a row, "+
			"and the C drops what it cannot substitute", "A-B", got, "")
	}
}
