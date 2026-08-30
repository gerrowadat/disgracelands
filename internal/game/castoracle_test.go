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

// `cast`'s argument parsing against the C, end to end: the typed line goes
// in, and what comes out is which spell (if any) is cast, at what target,
// or which of do_cast's two refusals is printed.
//
// This exists because the function was read wrong twice, confidently, and
// the reason is worth stating because it is not carelessness — it is the
// shape of the problem.
//
// `argument` arrives with a leading space (command_interpreter does
// `line = any_one_arg(argument, arg)`, which returns a pointer to the
// character *after* the command word), and strtok skips leading
// delimiters. Those two facts only make sense together: with the leading
// space, the first strtok returns that space and the second returns the
// spell name. Read with the wrong idea of the input, the same function
// appears to return the spell name from the first call and the target from
// the second — i.e. to cast the target — which is an obviously false
// prediction and is still easy to argue away.
//
// So the port's own behaviour is derived from the compiled C rather than
// asserted about it, the way the command table's abbreviation behaviour is
// derived from interpreter.c's line numbers.

func buildCastOracle(t *testing.T) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("gcc not found in CI; the cast-argument comparison must run")
		}
		t.Skip("gcc not found; skipping the cast-argument comparison")
	}

	src := filepath.Join(repoRoot(t), "reference", "tools", "castoracle.c")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("%s not found: %v", src, err)
	}

	bin := filepath.Join(t.TempDir(), "castoracle")
	build := exec.Command(gcc, "-std=gnu89", "-w", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compiling the oracle: %v\n%s", err, out)
	}
	return bin
}

// castOutcome is what a typed line does, in the vocabulary the oracle
// prints: which refusal, or which spell at which target.
type castOutcome struct {
	verdict string // "blank", "unenclosed", "unknown", or "cast"
	spell   SpellID
	target  string
}

// castOutcomeOfPort runs the typed line through the port's own halves —
// ParseCastArgument, then SpellNumberByName — and reports the same
// vocabulary. do_cast's own two refusals come from those two functions and
// nothing else (internal/session/cast.go), so this is what a player sees.
func castOutcomeOfPort(typed string) castOutcome {
	spell, target, err := ParseCastArgument(portArgument(typed))
	switch {
	case strings.HasPrefix(err, "Cast what where?"):
		return castOutcome{verdict: "blank"}
	case strings.HasPrefix(err, "Spell names must be enclosed"):
		return castOutcome{verdict: "unenclosed"}
	}

	number, ok := SpellNumberByName(spell)
	if !ok {
		return castOutcome{verdict: "unknown"}
	}
	return castOutcome{verdict: "cast", spell: number, target: target}
}

// castQueries is every arrangement of quotes and spaces worth asking
// about, plus real spell names in real positions.
//
// The interesting ones are the degenerate arrangements, because those are
// where "skip leading delimiters" and "take everything before the first
// quote" stop agreeing — over a well-formed `cast 'magic missile' fido`
// the wrong rule and the right one cannot disagree, which is exactly the
// trap docs/design/data-format.md §11.1 records one level up.
func castQueries() []string {
	// Written out in full, command word included, rather than assembled
	// from a prefix and an argument. The space between `cast` and what
	// follows is *part of what is being tested* — it is what any_one_arg
	// leaves behind and what makes strtok's first call return a blank — so
	// a corpus that generated it automatically would be a corpus that
	// could not express its absence. An earlier draft did exactly that,
	// concatenating `"cast"+arg`, and every case in it silently became the
	// no-space form; the C and this port agreed on all of them, and the
	// sweep proved nothing. Same failure as building a keyword corpus out
	// of letters and spaces (#277), one layer up.
	//
	// No tabs: the oracle's output is tab-separated and echoes the query
	// back, so a tab in one would be indistinguishable from a field break.
	// Spaces cover the same ground — what is tested is "whitespace where a
	// spell name should be", not which whitespace.
	return []string{
		// Well-formed. These are the ones over which the right rule and
		// the wrong one cannot disagree, which is why the rest are here.
		"cast 'magic missile' fido",
		"cast 'mag mis' fido",
		"cast 'magic missile'",
		"cast 'magic missile' fido bat",
		"cast  'magic missile'   fido  ",
		"cast 'armor'",
		"cast 'armor' fido",

		// Nothing, or nothing but whitespace.
		"cast",
		"cast ",
		"cast   ",

		// Quotes and nothing else, in every arrangement up to four.
		"cast '",
		"cast ''",
		"cast '''",
		"cast ''''",

		// Whitespace between the quotes: the one shape the C and this
		// port deliberately disagree about.
		"cast ' '",
		"cast '  '",
		"cast '   '",
		"cast ' ' fido",
		"cast '   ' fido",

		// An empty name with a target after it.
		"cast '' fido",
		"cast ''' fido",

		// Unenclosed, half-enclosed, and enclosed in the wrong place.
		// The unterminated ones are not a curiosity: `cast 'armor` works
		// on the real server, because strtok's second call runs to the end
		// of the string when it finds no closing delimiter. This port
		// refused them.
		"cast fireball",
		"cast fireball'",
		"cast 'fireball",
		"cast 'armor",
		"cast 'magic missile",
		"cast 'mag mis",
		"cast 'mag mis fido",
		"cast magic missile'",
		"cast magic 'missile'",

		// No space after the command word, which the interpreter treats as
		// part of the command word — so these reach do_cast with a
		// different argument from the spaced forms above, and that is the
		// difference this corpus exists to be able to see.
		"cast'magic missile' fido",
		"cast'armor'",
		"cast'   '",

		// Two quoted sections, and a name nothing matches.
		"cast 'armor' 'shield'",
		"cast '' ''",
		"cast 'not a spell' fido",
		"cast 'armor'fido",
	}
}

func TestCastArgumentAgainstC(t *testing.T) {
	bin := buildCastOracle(t)

	numbers := skillNameTable()

	var input strings.Builder
	for _, number := range numbers {
		input.WriteString(strconv.Itoa(int(number)))
		input.WriteByte('\t')
		input.WriteString(spellTable[number].Name)
		input.WriteByte('\n')
	}
	input.WriteByte('\n')

	queries := castQueries()
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
		t.Fatalf("the oracle answered %d lines, asked %d queries", len(lines), len(queries))
	}

	// Known and deliberate: the C casts the lowest-numbered spell in the
	// table for a name made only of whitespace, because find_skill_num's
	// word loop never runs and its verdict is `ok && !*first2`. This port
	// refuses. See docs/deviations.md.
	deviations := map[string]bool{}
	trimmingDifferences := map[string]bool{}

	for i, line := range lines {
		fields := strings.Split(line, "\t")
		if fields[0] != queries[i] {
			t.Fatalf("the oracle answered %q where %q was asked", fields[0], queries[i])
		}

		var want castOutcome
		switch fields[1] {
		case "blank", "unenclosed", "unknown":
			want.verdict = fields[1]
		default:
			n, convErr := strconv.Atoi(fields[1])
			if convErr != nil || len(fields) < 3 {
				t.Fatalf("query %q: unparseable oracle line %q", queries[i], line)
			}
			want = castOutcome{verdict: "cast", spell: SpellID(n), target: strings.TrimSpace(fields[2])}
		}

		got := castOutcomeOfPort(queries[i])
		got.target = strings.TrimSpace(got.target)

		if got == want {
			continue
		}
		if isWhitespaceOnlySpellName(queries[i]) {
			deviations[queries[i]] = true
			// The deviation is specific: the C casts *something*, this
			// refuses. It must not be that the two cast different spells.
			if got.verdict == "cast" {
				t.Errorf("%q: the C casts %d and this casts %d; the documented deviation is that this refuses",
					queries[i], want.spell, got.spell)
			}
			continue
		}
		if portArgument(queries[i]) == "" && want.verdict == "unenclosed" {
			// The other documented difference, and it is the
			// interpreter's rather than this function's: session.split
			// trims the argument, so `cast` and `cast   ` are the same
			// input here and the C tells them apart. Both are refusals;
			// only the sentence differs. See docs/deviations.md.
			trimmingDifferences[queries[i]] = true
			if got.verdict != "blank" {
				t.Errorf("%q: this gives %q, want the blank refusal", queries[i], got.verdict)
			}
			continue
		}
		t.Errorf("%q: this gives %+v, the C gives %+v", queries[i], got, want)
	}

	// A sweep whose hard cases all turned out to be deviations is a sweep
	// that proved nothing, so require that the corpus contains the
	// degenerate shapes *and* that most of it agreed outright.
	if len(deviations) == 0 {
		t.Error("no whitespace-only spell name in the corpus; the deviation is untested")
	}
	if len(deviations) > len(queries)/4 {
		t.Errorf("%d of %d queries were excused as deviations; the corpus is mostly exceptions",
			len(deviations), len(queries))
	}
	if len(trimmingDifferences) == 0 {
		t.Error("no whitespace-only argument in the corpus; the trimming difference is untested")
	}
	t.Logf("%d typed lines: %d agreed outright, %d the whitespace-name deviation, %d the argument-trimming one",
		len(queries), len(queries)-len(deviations)-len(trimmingDifferences),
		len(deviations), len(trimmingDifferences))
}

// portArgument is what this port's interpreter hands a command, for a line
// beginning with a letter: session.split cuts at the first space and
// **trims** what follows.
//
// Reproduced here rather than imported because internal/session imports
// this package and not the other way round. The trimming is the thing to
// notice: the C's own argument keeps every space, and that difference is
// one of the two the sweep below has to account for.
func portArgument(typed string) string {
	line := strings.TrimSpace(typed)
	_, arg, _ := strings.Cut(line, " ")
	return strings.TrimSpace(arg)
}

// isWhitespaceOnlySpellName reports whether the typed line's quoted
// section contains nothing but whitespace, which is one of the two shapes
// the C and this port disagree about.
func isWhitespaceOnlySpellName(typed string) bool {
	_, rest, ok := strtokQuote(" " + portArgument(typed))
	if !ok {
		return false
	}
	name, _, ok := strtokQuote(rest)
	if !ok {
		return false
	}
	return name != "" && strings.TrimSpace(name) == ""
}
