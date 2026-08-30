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
	"sort"
	"strconv"
	"strings"
	"testing"
)

// find_skill_num() against the C, because it has two matching rules and
// this port had one.
//
// The first rule — the whole typed string against the whole spell name —
// is the obvious one and returns on its own line. The second walks both a
// word at a time and requires each typed word to abbreviate the spell-name
// word in the same position, and is what makes `cast 'mag mis'` work. It
// was missing for the whole of this port's life and nothing noticed,
// because TestSpellNumberByName's four cases only ever abbreviated the
// *last* word — over which the two rules cannot disagree. That is the same
// failure as isname's letters-and-spaces corpus (#277) in a different
// function, and the answer is the same: sweep the inputs that can tell the
// rules apart, not the ones that read naturally.
//
// The oracle is fed this package's own table, so the two cannot drift.

func buildSkillOracle(t *testing.T) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("gcc not found in CI; the skill-name comparison must run")
		}
		t.Skip("gcc not found; skipping the skill-name comparison")
	}

	src := filepath.Join(repoRoot(t), "reference", "tools", "skilloracle.c")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("%s not found: %v", src, err)
	}

	bin := filepath.Join(t.TempDir(), "skilloracle")
	build := exec.Command(gcc, "-std=gnu89", "-w", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compiling the oracle: %v\n%s", err, out)
	}
	return bin
}

// skillNameTable is this package's own spell table, in ascending number
// order — which is the order the C iterates, because there the index *is*
// the spell number. Feeding the oracle that order is what makes "the first
// entry the C matches" and "the lowest-numbered entry this package matches"
// the same claim.
func skillNameTable() []SpellID {
	numbers := make([]SpellID, 0, len(spellTable))
	for number := range spellTable {
		numbers = append(numbers, number)
	}
	sort.Slice(numbers, func(i, j int) bool { return numbers[i] < numbers[j] })
	return numbers
}

// perWordAbbreviations is every abbreviation of name that the C's second
// rule could conceivably accept: every non-empty prefix of every word, in
// every combination, for every number of leading words from one up to all
// of them.
//
// The whole point is that these are the queries the *first* rule cannot
// express. "magic mis" is a prefix of the whole name and so tests nothing;
// "mag mis" is not, and is what a caster types. A corpus built only from
// prefixes of the full name is a corpus with the hard case designed out of
// it — the mistake docs/design/data-format.md §11.1 records for
// examples/stock, one level up.
func perWordAbbreviations(name string) []string {
	words := strings.Fields(name)
	var out []string

	for count := 1; count <= len(words); count++ {
		combos := []string{""}
		for _, word := range words[:count] {
			var next []string
			for _, prefix := range combos {
				for n := 1; n <= len(word); n++ {
					if prefix == "" {
						next = append(next, word[:n])
						continue
					}
					next = append(next, prefix+" "+word[:n])
				}
			}
			combos = next
		}
		out = append(out, combos...)
	}
	return out
}

func TestFindSkillNumAgainstC(t *testing.T) {
	bin := buildSkillOracle(t)

	numbers := skillNameTable()

	// The table, then a blank line, then one query per line: the oracle's
	// own input format.
	var input strings.Builder
	for _, number := range numbers {
		input.WriteString(strconv.Itoa(int(number)))
		input.WriteByte('\t')
		input.WriteString(spellTable[number].Name)
		input.WriteByte('\n')
	}
	input.WriteByte('\n')

	// Every per-word abbreviation of every name in the table, deduplicated
	// — "b h" is reachable from more than one name and only needs asking
	// once.
	seen := map[string]bool{}
	var queries []string
	for _, number := range numbers {
		for _, q := range perWordAbbreviations(spellTable[number].Name) {
			if seen[q] {
				continue
			}
			seen[q] = true
			queries = append(queries, q)
		}
	}
	// And a handful that should match nothing, so a rule that accepts
	// everything fails too.
	for _, q := range []string{"zzz", "magic zzz", "zzz missile", "magic missile zzz", "q q q q"} {
		if !seen[q] {
			seen[q] = true
			queries = append(queries, q)
		}
	}
	for _, q := range queries {
		input.WriteString(q)
		input.WriteByte('\n')
	}

	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(input.String())
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running the oracle: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != len(queries) {
		t.Fatalf("the oracle answered %d queries, asked %d", len(lines), len(queries))
	}

	var mismatches, matched int
	for i, line := range lines {
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 {
			t.Fatalf("query %q: unparseable oracle line %q", queries[i], line)
		}
		if fields[0] != queries[i] {
			t.Fatalf("the oracle answered %q where %q was asked", fields[0], queries[i])
		}
		want, err := strconv.Atoi(fields[1])
		if err != nil {
			t.Fatalf("query %q: unparseable oracle answer %q", queries[i], fields[1])
		}

		number, ok := SpellNumberByName(queries[i])
		got := -1
		if ok {
			got = int(number)
			matched++
		}
		if got != want {
			mismatches++
			if mismatches <= 20 {
				t.Errorf("SpellNumberByName(%q) = %s, the C finds %s",
					queries[i], describeSkill(got), describeSkill(want))
			}
		}
	}
	if mismatches > 20 {
		t.Errorf("...and %d more mismatches", mismatches-20)
	}

	// A sweep that agreed with the C because it asked nothing hard is the
	// failure mode this whole file exists to avoid, so say how much was
	// actually asked.
	t.Logf("%d queries over %d names, %d of them matched a spell",
		len(queries), len(numbers), matched)
	if len(queries) < 1000 {
		t.Errorf("only %d queries; the corpus is not sweeping what it should", len(queries))
	}
}

func describeSkill(n int) string {
	if n < 0 {
		return "nothing"
	}
	return strconv.Itoa(n) + " (" + SpellName(SpellID(n)) + ")"
}

// TestSkillNamesAreNotReachableFromEachOther checks the property
// SpellNumberByName's exact-match preference relies on: no spell's whole
// name matches a *different* spell by either of find_skill_num's rules.
//
// Where one did, the C would answer with whichever sits lower in the table
// and this package would answer with the exact match, and they would
// disagree about a name a player typed in full. Asserted here rather than
// in a comment because it is a property of the name table, and the table is
// data that can change.
func TestSkillNamesAreNotReachableFromEachOther(t *testing.T) {
	for number, info := range spellTable {
		for other, otherInfo := range spellTable {
			if other == number || otherInfo.Name == info.Name {
				continue
			}
			if matchesSkillName(info.Name, otherInfo.Name) {
				t.Errorf("%q (%d) matches %q (%d) by find_skill_num's rules; "+
					"SpellNumberByName's exact-match preference now disagrees with the C",
					info.Name, number, otherInfo.Name, other)
			}
		}
	}
}
