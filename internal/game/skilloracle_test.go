// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// find_skill_num() against the C, because it has two matching rules and the
// second one is easy to miss.
//
// This port had only the first — `is_abbrev(name, spell_info[index].name)`,
// the whole typed string against the whole spell name — and refused 1,145 of
// the 1,549 per-word abbreviations of the table below, `cast 'mag mis'`
// among them (#355). The second rule walks both a word at a time and
// requires each typed word to abbreviate the name-word in the same position.
//
// The oracle takes its name table on **stdin** rather than compiling one in,
// so what it is asked about is this package's own spellTable. A duplicated
// copy of 71 names in C would drift from the Go the first time a name
// changed, and would then agree with itself while both were wrong — which is
// the failure mode docs/investigations/partial-matching.md is about.

func buildSkillOracle(t *testing.T) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("gcc not found in CI; the skill comparison must run")
		}
		t.Skip("gcc not found; skipping the skill comparison")
	}

	src := filepath.Join(repoRoot(t), "reference", "tools", "skilloracle.c")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("%s not found: %v", src, err)
	}

	bin := filepath.Join(t.TempDir(), "skilloracle")
	// -std=gnu89 for the same reason the C server is built that way: these
	// are original bodies with declarations at the top of a block.
	build := exec.Command(gcc, "-std=gnu89", "-O2", "-Wall", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compiling the oracle: %v\n%s", err, out)
	}
	return bin
}

// askSkillOracle feeds the port's own spell table and a list of queries to
// the oracle, and returns what the C answers for each, aligned with
// queries. -1 is no match.
//
// Answers are matched to queries **by position, not by the echoed query
// text**. The oracle prints "<query>\t<answer>" and a query may itself
// contain a tab — `\t` is whitespace to any_one_arg's isspace() and so is a
// case worth sweeping — which makes the echo ambiguous to split on. Position
// is unambiguous: one line out per line in, and a query cannot contain a
// newline because the input is line-delimited.
func askSkillOracle(t *testing.T, bin string, queries []string) []SpellID {
	t.Helper()

	var in strings.Builder
	for _, id := range spellIDsInOrder {
		fmt.Fprintf(&in, "%d\t%s\n", id, spellTable[id].Name)
	}
	in.WriteString("\n") // the blank line that ends the table
	for _, q := range queries {
		if strings.ContainsAny(q, "\n\r") {
			t.Fatalf("query %q contains a newline; the oracle's input is line-delimited", q)
		}
		in.WriteString(q)
		in.WriteString("\n")
	}

	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in.String())
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running the oracle: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != len(queries) {
		t.Fatalf("asked %d queries, got %d answers", len(queries), len(lines))
	}

	answers := make([]SpellID, len(queries))
	for i, line := range lines {
		tab := strings.LastIndex(line, "\t")
		if tab < 0 {
			t.Fatalf("unparsable oracle line %q", line)
		}
		n, convErr := strconv.Atoi(line[tab+1:])
		if convErr != nil {
			t.Fatalf("unparsable oracle answer %q: %v", line, convErr)
		}
		answers[i] = SpellID(n)
	}
	return answers
}

// skillQueries is what the oracle is swept over, and the corpus is the part
// worth reviewing rather than the code — nameoracle.c's own README entry
// says so, having been wrong for a year over a corpus that could not
// disagree with it.
//
// The four cases TestSpellNumberByName had before #355 were "magic missile",
// "magic mis", "armor" and "heal": one full two-word name, one abbreviation
// of the *last* word, and two single-word spells. The missing rule only ever
// fires when the **first** word is abbreviated, so all four agreed with a C
// they were not testing. Hence the first block below.
func skillQueries(t *testing.T) []string {
	t.Helper()

	seen := map[string]bool{}
	var queries []string
	add := func(q string) {
		if !seen[q] {
			seen[q] = true
			queries = append(queries, q)
		}
	}

	// Every per-word abbreviation of every name: each word cut to every
	// length from one character to all of it, in every combination. This is
	// the block that fails without rule 2, and it is 1,549 queries.
	for _, id := range spellIDsInOrder {
		words := strings.Fields(spellTable[id].Name)
		combos := []string{""}
		for _, w := range words {
			var next []string
			for _, prefix := range combos {
				for k := 1; k <= len(w); k++ {
					next = append(next, strings.TrimSpace(prefix+" "+w[:k]))
				}
			}
			combos = next
		}
		for _, c := range combos {
			add(c)
		}
	}

	// Shapes the block above cannot produce, each of which is a rule of its
	// own rather than a variation:
	for _, q := range []string{
		"",               // no words at all: matches the *first* spell
		" ", "   ", "\t", // ... and so does whitespace, once tokenised
		"magic  missile",      // a run of spaces between words
		" magic missile",      // leading, which rule 1 fails and rule 2 skips
		"magic missile ",      // trailing
		"MAGIC MISSILE",       // case, on both rules
		"Magic MiS",           // mixed
		"magic missile extra", // more words than the name: no match
		"cure light extra",
		"armor extra",
		"frobnicate",                      // nothing like any of them
		"!",                               // is_abbrev's own first character, and search_block's
		"'",                               // what do_cast delimits with
		"3",                               // a digit; a name has none
		"magic-mis",                       // a hyphen is not whitespace, so this is one word
		"magicmissile",                    // no separator at all
		"z",                               // no name begins with it
		"a", "b", "c", "d", "e", "s", "w", // one letter: whichever comes first
	} {
		add(q)
	}

	// Every full name, which is the property SpellNameOrNumber's round trip
	// rests on: a name must resolve to its own spell and not to an earlier
	// one that it happens to abbreviate.
	for _, id := range spellIDsInOrder {
		add(spellTable[id].Name)
	}

	return queries
}

// TestSpellNumberByNameMatchesTheC is the comparison.
func TestSpellNumberByNameMatchesTheC(t *testing.T) {
	bin := buildSkillOracle(t)
	queries := skillQueries(t)
	want := askSkillOracle(t, bin, queries)

	mismatches := 0
	for i, q := range queries {
		got, ok := SpellNumberByName(q)
		if !ok {
			got = -1
		}
		if got != want[i] {
			mismatches++
			if mismatches <= 20 {
				t.Errorf("SpellNumberByName(%q) = %d (%s); the C says %d (%s)",
					q, got, SpellName(got), want[i], SpellName(want[i]))
			}
		}
	}
	if mismatches > 20 {
		t.Errorf("... and %d more", mismatches-20)
	}
	if t.Failed() {
		return
	}
	t.Logf("agreed with the C on %d queries over %d spells", len(queries), len(spellIDsInOrder))
}

// TestSkillQueriesCoverBothRules guards the corpus rather than the code.
//
// The bug this file exists for survived because every query anybody had
// tried was one over which the two rules cannot disagree. A sweep that
// drifted back to that state would pass while testing nothing, so the
// property is asserted directly: the corpus must contain queries that only
// rule 2 can answer.
func TestSkillQueriesCoverBothRules(t *testing.T) {
	onlyRuleTwo := 0
	for _, q := range skillQueries(t) {
		words := strings.Fields(q)
		if len(words) < 2 {
			continue
		}
		// Rule 1 is a prefix of the whole name; if no name has this query as
		// a prefix, only rule 2 can match it.
		ruleOnePossible := false
		for _, id := range spellIDsInOrder {
			if isAbbrevOf(q, spellTable[id].Name) {
				ruleOnePossible = true
				break
			}
		}
		if ruleOnePossible {
			continue
		}
		if _, ok := SpellNumberByName(q); ok {
			onlyRuleTwo++
		}
	}
	// The real number is in the hundreds; the floor is deliberately loose so
	// that adding a spell does not fail this, while emptying the corpus does.
	if onlyRuleTwo < 100 {
		t.Errorf("only %d queries in the sweep need rule 2; the corpus has stopped testing it", onlyRuleTwo)
	}
}
