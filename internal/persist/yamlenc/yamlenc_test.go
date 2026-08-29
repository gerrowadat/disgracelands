// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yamlenc

import (
	"testing"

	"github.com/goccy/go-yaml"
)

// roundTrip writes s the way every plain string field in every yaml
// document here is written, and reads it back into a string field.
func roundTrip(t *testing.T, s string) string {
	t.Helper()

	out, err := MarshalString(s)
	if err != nil {
		t.Fatalf("marshalling %q: %v", s, err)
	}
	var back struct {
		V string `yaml:"v"`
	}
	if err := yaml.UnmarshalWithOptions([]byte("v: "+string(out)), &back, yaml.Strict()); err != nil {
		t.Fatalf("%q was written as %s, which will not parse back: %v", s, out, err)
	}
	return back.V
}

// The rule PlainlySafe exists for, stated as the property rather than as
// a list of clauses: whatever goes in comes back.
func TestEveryStringSurvivesAPlainField(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"ordinary text", "a long sword"},
		{"an act() message", "$n gives $p to $N."},
		{"a tab", "the\tTester"},
		{"a line break", "A\n"},
		{"a document marker", "---"},
		{"the other document marker", "..."},
		{"a marker prefix", "...x"},
		{"a leading indicator", "? a"},
		{"leading whitespace", " a"},
		{"trailing whitespace", "a "},

		// The float specials: plain, these resolve to a float rather than
		// to text, so ".NAN" used to come back "NaN". Found by the first
		// real `make fuzz` budget (2026-08-29); the seed corpus had never
		// produced one, which is the whole argument for having a budget.
		{"nan, lower", ".nan"},
		{"nan, mixed", ".NaN"},
		{"nan, upper", ".NAN"},
		{"inf, lower", ".inf"},
		{"inf, mixed", ".Inf"},
		{"inf, upper", ".INF"},

		// Signed, which goccy happens *not* to resolve today. Quoted
		// anyway, because the spec says they are floats and the rule is
		// taken from the spec rather than from what one library version
		// does. These are the cases that would otherwise break silently
		// on a dependency upgrade.
		{"positive infinity", "+.inf"},
		{"negative infinity", "-.inf"},
		{"signed nan", "+.nan"},

		// Not float specials, and not to be quoted for looking like one:
		// the rest of §10.2.1.4's production needs a digit.
		{"a word ending in inf", "sinf"},
		{"a word starting with inf", "infantry"},
		{"a sentence about it", "The .inf sign."},

		// Other core-schema scalars, which reach a string field as their
		// own text and are here so that a future widening of the rule has
		// to notice it is widening.
		{"a bool", "true"},
		{"a null", "null"},
		{"a tilde", "~"},
		{"an integer", "12"},
		{"a float", "1.5"},
		{"hex", "0x1F"},
		{"empty", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := roundTrip(t, c.in); got != c.in {
				t.Errorf("round trip changed the string: in %q, out %q", c.in, got)
			}
		})
	}
}

// PlainlySafe's answer, separately from the round trip, so that a change
// which quotes *everything* still fails: quoting is correct but it is not
// free, and the readable-output half of the format's design depends on the
// ordinary case staying plain.
func TestOrdinaryStringsStayPlain(t *testing.T) {
	plain := []string{"a long sword", "$n gives $p to $N.", "true", "12", "Zod", "infantry"}
	for _, s := range plain {
		if !PlainlySafe(s) {
			t.Errorf("PlainlySafe(%q) = false, want true: this one has no reason to be quoted", s)
		}
	}
	quoted := []string{".nan", ".INF", "+.inf", "-.nan", "\ta", "a\n", "---", " a", "a "}
	for _, s := range quoted {
		if PlainlySafe(s) {
			t.Errorf("PlainlySafe(%q) = true, want false", s)
		}
	}
}
