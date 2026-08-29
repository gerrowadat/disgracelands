// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// isname() and get_number() against the C, because both read as something
// they are not.
//
// isname looks like a prefix match — the inner loop walks a keyword character
// by character and breaks when the search string runs out — and it is not one.
// This port had it as a prefix match, with a comment saying the C matched
// prefixes, until this oracle said otherwise.
//
// get_number rewrites its argument before deciding whether the prefix was a
// number, so what it leaves behind matters as much as what it returns.

func buildNameOracle(t *testing.T) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("gcc not found in CI; the name comparison must run")
		}
		t.Skip("gcc not found; skipping the name comparison")
	}

	src := filepath.Join(repoRoot(t), "reference", "tools", "nameoracle.c")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("%s not found: %v", src, err)
	}

	bin := filepath.Join(t.TempDir(), "nameoracle")
	build := exec.Command(gcc, "-O2", "-Wall", "-Werror", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compiling the oracle: %v\n%s", err, out)
	}
	return bin
}

// nameOracleLines runs the oracle and returns its tab-separated rows.
func nameOracleLines(t *testing.T, bin string) [][]string {
	t.Helper()

	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("running the oracle: %v", err)
	}

	var rows [][]string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		fields := strings.Split(line, "\t")
		for i := range fields {
			fields[i] = unescape(fields[i])
		}
		rows = append(rows, fields)
	}
	return rows
}

// unescape reverses the oracle's putesc(). The escaping exists so that a
// namelist may contain a tab or a newline without breaking a tab-separated,
// line-per-row format — and it exists at all because a keyword list wrapped
// across lines by fread_string is one of the shapes the original sweep could
// not express, and one of the shapes that was wrong.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// TestMatchesKeywordsAgainstTheC compares every pairing the oracle emits.
//
// It used to be seven keyword lists against twenty-five words, and every one
// of those lists was made only of letters and spaces. That is why 168
// pairings could pass while the port had isname's keyword terminator as
// "whitespace" instead of the C's "not a letter": over an alphabetic
// namelist the two rules never disagree. Issue #277, and the same lesson as
// docs/proposals/yaml-only.md §5.1 one level up — the corpus did not contain
// the hard case, so the oracle could not either.
//
// The sweep now includes digits, punctuation, an apostrophe, a hyphen,
// doubled and trailing spaces and a namelist wrapped across lines, which is
// what a real world file holds.
func TestMatchesKeywordsAgainstTheC(t *testing.T) {
	bin := buildNameOracle(t)

	checked := 0
	// Guards against the sweep quietly narrowing back to the shape that hid
	// #277: a namelist with a digit in it, and one with a byte that is
	// neither a letter, a digit nor a space.
	var sawDigit, sawOtherNonAlpha bool
	for _, row := range nameOracleLines(t, bin) {
		if row[0] != "isname" {
			continue
		}
		word, keywords, want := row[1], row[2], row[3] == "1"

		// The one deliberate difference, and it is unreachable: the C's
		// isname matches an empty string against anything, because `!*curstr`
		// is true on the first pass. Every caller rejects an empty argument
		// before it gets here.
		if word == "" {
			continue
		}

		if got := matchesKeywords(keywords, word); got != want {
			t.Errorf("matchesKeywords(%q, %q) = %v, the C says %v", keywords, word, got, want)
		}
		checked++

		for i := 0; i < len(keywords); i++ {
			switch c := keywords[i]; {
			case c >= '0' && c <= '9':
				sawDigit = true
			case c == ' ' || (c|0x20 >= 'a' && c|0x20 <= 'z'):
			default:
				sawOtherNonAlpha = true
			}
		}
	}

	if checked == 0 {
		t.Fatal("the oracle produced no isname rows")
	}
	if !sawDigit {
		t.Error("no namelist in the sweep contains a digit; that is the gap #277 hid in")
	}
	if !sawOtherNonAlpha {
		t.Error("no namelist in the sweep contains punctuation or a line break; isname ends a keyword at any non-letter, so the sweep has to contain some")
	}
	t.Logf("checked %d isname pairings against the C", checked)
}

// TestAnAbbreviationIsNotAName is the finding, asserted on its own so that
// nobody "fixes" it back. `get swo` does not pick up a sword and never did.
func TestAnAbbreviationIsNotAName(t *testing.T) {
	for _, word := range []string{"swo", "s", "swordd", "ord"} {
		if matchesKeywords("sword long", word) {
			t.Errorf("%q matched the keyword list; isname is a whole-word match", word)
		}
	}
	for _, word := range []string{"sword", "SWORD", "Long", "long"} {
		if !matchesKeywords("sword long", word) {
			t.Errorf("%q did not match; isname is case-insensitive on whole words", word)
		}
	}
}

// TestGetNumberAgainstTheC checks both halves: the count it returns and the
// string it leaves behind, which the C rewrites in place.
func TestGetNumberAgainstTheC(t *testing.T) {
	bin := buildNameOracle(t)

	checked := 0
	for _, row := range nameOracleLines(t, bin) {
		if row[0] != "get_number" {
			continue
		}
		arg, wantRest := row[1], ""
		wantN, err := strconv.Atoi(row[2])
		if err != nil {
			t.Fatalf("the oracle gave a non-numeric count %q", row[2])
		}
		// A trailing empty field is dropped by the split when the rewritten
		// string is empty.
		if len(row) > 3 {
			wantRest = row[3]
		}

		gotN, gotRest := GetNumber(arg)
		if gotN != wantN || gotRest != wantRest {
			t.Errorf("GetNumber(%q) = (%d, %q), the C says (%d, %q)",
				arg, gotN, gotRest, wantN, wantRest)
		}
		checked++
	}

	if checked == 0 {
		t.Fatal("the oracle produced no get_number rows")
	}
	t.Logf("checked %d get_number cases against the C", checked)
}

// TestGetNumberQuirks names the three that will otherwise be tidied away by
// somebody who thinks they are bugs. They are, but they are the C's.
func TestGetNumberQuirks(t *testing.T) {
	for _, tc := range []struct {
		arg  string
		n    int
		rest string
		why  string
	}{
		// The prefix is rewritten away before the digits are checked, so a
		// non-numeric prefix still eats it — and 0 means "a player with this
		// name" to a character search.
		{"foo.sword", 0, "sword", "a non-numeric prefix returns 0 and still strips"},
		{".sword", 0, "sword", "a bare dot is the same as a bad prefix"},
		// atoi, with everything that implies.
		{"007.sword", 7, "sword", "leading zeroes are just atoi"},
		{"-1.sword", 0, "sword", "a minus sign is not a digit"},
		// Only the first dot is consumed.
		{"2.3.sword", 2, "3.sword", "the second dot survives for the next caller"},
		{"sword", 1, "sword", "no dot at all is the first match"},
	} {
		n, rest := GetNumber(tc.arg)
		if n != tc.n || rest != tc.rest {
			t.Errorf("GetNumber(%q) = (%d, %q), want (%d, %q) — %s",
				tc.arg, n, rest, tc.n, tc.rest, tc.why)
		}
	}
}

// TestRespacingAKeywordListChangesNoAnswer pins the claim docs/deviations.md
// makes in "The yaml format re-spaces a keyword list": converting a namelist
// to yaml splits it on whitespace and converting back joins it with one
// space, and "nothing can observe the difference".
//
// That is a claim about isname, so it belongs next to isname, and it needs
// re-checking whenever isname changes — which it just did (#277). The
// property is not "the oracle agrees" (the sweep covers that); it is that
// the two *spellings* of one namelist give the same answer for every word.
//
// The CRLF case is the one that matters in practice: a keyword list wrapped
// across lines by fread_string is legal in a .wld file, and comparing the
// two spellings byte for byte is what made `dlctl import --verify` refuse a
// conversion that had lost nothing.
func TestRespacingAKeywordListChangesNoAnswer(t *testing.T) {
	pairs := []struct {
		name       string
		stored     string // as the classic file holds it
		normalised string // as a yaml round trip gives it back
	}{
		{"a trailing space", "cape wool woolen ", "cape wool woolen"},
		{"a leading space", " cape wool", "cape wool"},
		{"a doubled space", "cape  wool", "cape wool"},
		{"a wrapped line", "staircase stair 606\r\nrs", "staircase stair 606 rs"},
		{"a bare newline", "cape\nwool", "cape wool"},
		{"a tab", "cape\twool", "cape wool"},
		{"every kind at once", " cape \r\n wool\twoolen  ", "cape wool woolen"},
	}
	// Every word in either spelling, plus the ones most likely to expose a
	// difference: prefixes, and the digits that started all this.
	words := []string{
		"cape", "wool", "woolen", "staircase", "stair", "rs", "r", "s",
		"606", "6", "60", "0", "woo", "capes", "wo", "olen", "n",
	}
	for _, p := range pairs {
		t.Run(p.name, func(t *testing.T) {
			for _, w := range words {
				stored := matchesKeywords(p.stored, w)
				normalised := matchesKeywords(p.normalised, w)
				if stored != normalised {
					t.Errorf("%q matches %q = %v but %q = %v; the re-spacing is observable after all, and docs/deviations.md says it is not",
						w, p.stored, stored, p.normalised, normalised)
				}
			}
		})
	}
}
