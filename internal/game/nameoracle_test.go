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
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows
}

// TestMatchesKeywordsAgainstTheC compares every pairing the oracle emits —
// seven keyword lists against twenty-five words, including the abbreviations
// that look like they should work.
func TestMatchesKeywordsAgainstTheC(t *testing.T) {
	bin := buildNameOracle(t)

	checked := 0
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
	}

	if checked == 0 {
		t.Fatal("the oracle produced no isname rows")
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
