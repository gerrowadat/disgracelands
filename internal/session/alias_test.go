// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"reflect"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

func TestExpandAliasNotAnAliasPassesThrough(t *testing.T) {
	aliases := []game.Alias{{Name: "mm", Replacement: "cast 'magic missile'"}}
	commands, matched := ExpandAlias(aliases, "look")
	if matched {
		t.Fatalf("matched = true, commands = %v, want unmatched", commands)
	}
}

func TestExpandAliasNoAliasesPassesThrough(t *testing.T) {
	if _, matched := ExpandAlias(nil, "mm"); matched {
		t.Fatal("matched = true with no aliases defined")
	}
}

func TestExpandAliasSimpleRewritesWholeLine(t *testing.T) {
	aliases := []game.Alias{{Name: "k", Replacement: "kill"}}
	// A simple alias's replacement is the whole new line, verbatim — the
	// rest of what the player typed after "k" is discarded, matching
	// perform_alias's strcpy(orig, a->replacement) (interpreter.c:829),
	// which does not consult first_arg's remainder at all.
	commands, matched := ExpandAlias(aliases, "k rat")
	if !matched {
		t.Fatal("expected a match")
	}
	if want := []string{"kill"}; !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestExpandAliasMatchIsCaseInsensitiveOnTheWord(t *testing.T) {
	aliases := []game.Alias{{Name: "k", Replacement: "kill"}}
	if _, matched := ExpandAlias(aliases, "K rat"); !matched {
		t.Fatal("expected K to match the alias stored as k, same as LOWER() in any_one_arg")
	}
}

func TestExpandAliasNewestDefinitionWins(t *testing.T) {
	// doAlias prepends, so the list order itself carries "newest first";
	// ExpandAlias must return the first match it finds, not the last.
	aliases := []game.Alias{
		{Name: "k", Replacement: "kill"},
		{Name: "k", Replacement: "kick"},
	}
	commands, matched := ExpandAlias(aliases, "k")
	if !matched || !reflect.DeepEqual(commands, []string{"kill"}) {
		t.Fatalf("commands = %v, matched = %v, want [kill] true", commands, matched)
	}
}

func TestExpandAliasComplexSemicolonSplitsCommands(t *testing.T) {
	aliases := []game.Alias{{Name: "prep", Replacement: "wield sword;wear shield"}}
	commands, matched := ExpandAlias(aliases, "prep")
	if !matched {
		t.Fatal("expected a match")
	}
	want := []string{"wield sword", "wear shield"}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestExpandAliasPositionalTokens(t *testing.T) {
	aliases := []game.Alias{{Name: "gv", Replacement: "give $1 $2"}}
	commands, matched := ExpandAlias(aliases, "gv sword bob")
	if !matched {
		t.Fatal("expected a match")
	}
	if want := []string{"give sword bob"}; !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestExpandAliasMissingPositionalTokenDropsTheCode(t *testing.T) {
	// $2 with only one token available: not "1 <= num < num_of_tokens", so
	// perform_complex_alias falls through to the else branch and writes the
	// literal digit '2' instead (interpreter.c:786-789: *temp is '2', which
	// is neither ALIAS_GLOB_CHAR nor '$', so it is copied as-is).
	aliases := []game.Alias{{Name: "gv", Replacement: "give $2"}}
	commands, matched := ExpandAlias(aliases, "gv sword")
	if !matched {
		t.Fatal("expected a match")
	}
	if want := []string{"give 2"}; !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestExpandAliasGlobSubstitutesRawUntrimmedRemainder(t *testing.T) {
	// $* takes the exact remainder any_one_arg returned after extracting the
	// alias's own name -- including the leading space that any_one_arg
	// never trims (it only skip_spaces *before* the word, not after).
	// $1, by contrast, comes from strtok's space-collapsed tokens. Both are
	// exercised here so the difference between them is pinned down, not
	// merely asserted about the C.
	aliases := []game.Alias{{Name: "say2", Replacement: "gecho [$1] {$*}"}}
	commands, matched := ExpandAlias(aliases, "say2   hello   world")
	if !matched {
		t.Fatal("expected a match")
	}
	if want := []string{"gecho [hello] {   hello   world}"}; !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestExpandAliasDoubleDollarStaysDoubled(t *testing.T) {
	aliases := []game.Alias{{Name: "cash", Replacement: "gecho $$$1"}}
	commands, matched := ExpandAlias(aliases, "cash 100")
	if !matched {
		t.Fatal("expected a match")
	}
	// "$$" -> "$$" (redoubled, not collapsed), then "$1" -> "100".
	if want := []string{"gecho $$100"}; !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestExpandAliasTrailingLoneDollarContributesNothing(t *testing.T) {
	aliases := []game.Alias{{Name: "x", Replacement: "gecho hi$"}}
	commands, matched := ExpandAlias(aliases, "x")
	if !matched {
		t.Fatal("expected a match")
	}
	if want := []string{"gecho hi"}; !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestExpandAliasOnlyUpToNineTokens(t *testing.T) {
	aliases := []game.Alias{{Name: "x", Replacement: "gecho $9"}}
	commands, matched := ExpandAlias(aliases,
		"x one two three four five six seven eight nine ten")
	if !matched {
		t.Fatal("expected a match")
	}
	if want := []string{"gecho nine"}; !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %v, want %v", commands, want)
	}
}

func TestAnyOneArgLowercasesTheWordButNotTheRest(t *testing.T) {
	word, rest := anyOneArg("MM  Kobold")
	if word != "mm" {
		t.Fatalf("word = %q, want mm", word)
	}
	if rest != "  Kobold" {
		t.Fatalf("rest = %q, want %q (leading spaces kept, case kept)", rest, "  Kobold")
	}
}

func TestAnyOneArgEmptyInput(t *testing.T) {
	word, rest := anyOneArg("   ")
	if word != "" || rest != "" {
		t.Fatalf("word=%q rest=%q, want both empty", word, rest)
	}
}

func TestStrtokSpaceDropsEmptyTokens(t *testing.T) {
	got := strtokSpace("  a   b  c ", aliasMaxTokens)
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens = %v, want %v", got, want)
	}
}

func TestFindAndRemoveAlias(t *testing.T) {
	aliases := []game.Alias{
		{Name: "a", Replacement: "one"},
		{Name: "b", Replacement: "two"},
		{Name: "c", Replacement: "three"},
	}
	got, ok := findAlias(aliases, "b")
	if !ok || got.Replacement != "two" {
		t.Fatalf("findAlias(b) = %+v, %v", got, ok)
	}
	if _, ok := findAlias(aliases, "nope"); ok {
		t.Fatal("findAlias(nope) should fail")
	}

	after := removeAlias(aliases, "b")
	want := []game.Alias{{Name: "a", Replacement: "one"}, {Name: "c", Replacement: "three"}}
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("after removeAlias(b) = %v, want %v", after, want)
	}
}

func TestIsComplexAlias(t *testing.T) {
	cases := []struct {
		replacement string
		complex     bool
	}{
		{"kill", false},
		{"say hello there", false},
		{"wield sword;wear shield", true},
		{"give $1 bob", true},
		{"gecho hello$", true},
	}
	for _, c := range cases {
		if got := isComplexAlias(c.replacement); got != c.complex {
			t.Errorf("isComplexAlias(%q) = %v, want %v", c.replacement, got, c.complex)
		}
	}
}
