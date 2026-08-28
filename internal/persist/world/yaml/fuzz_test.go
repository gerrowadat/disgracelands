// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

// FuzzTextRoundTrip is the first of docs/proposals/yaml-only.md §5.3's
// three fuzz targets, and the one with the strongest claim on existing.
//
// Every text-transform finding in docs/design/data-format.md §12 was found
// by round-tripping a corpus rather than by reasoning about the library —
// and the corpus in question was examples/stock, which §5.1 established
// does not contain the hard cases. Before this, `grep -r 'func Fuzz'` over
// the whole tree returned nothing.
//
// The property is the only one that matters for this type: whatever goes
// in comes back out. Not "the YAML is pretty", not "a literal block was
// used" — those are quality-of-output questions with a human in the loop.
// A string that does not survive is a room description a player sees
// differently, and there is no version of that which is acceptable.
//
// The seeds are the shapes that have already gone wrong: the three
// needsQuoting cases (a trailing blank line, a bare carriage return,
// trailing whitespace with no newline after it), the indentation-indicator
// case, and the colour markup that YAML would read as a flow mapping. The
// fuzzer's job is the ones nobody has thought of.
func FuzzTextRoundTrip(f *testing.F) {
	for _, seed := range textSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		// Not valid UTF-8 is out of scope, and deliberately so rather than
		// by oversight: a YAML document is UTF-8 by definition, this
		// format's own answer to a non-UTF-8 world file is to transcode it
		// on the way in (`dlctl import --encoding`), and the encoder
		// substitutes U+FFFD for anything that reaches it undecoded. That
		// substitution is lossy, it is checked at the importer where it
		// can be prevented, and it is not something this type can fix.
		if !utf8.ValidString(s) {
			t.Skip()
		}
		var doc struct {
			Desc Text `yaml:"desc"`
		}
		doc.Desc = Text(s)
		out, err := yaml.MarshalWithOptions(doc, yaml.Indent(2))
		if err != nil {
			t.Fatalf("marshalling %q: %v", s, err)
		}
		var back struct {
			Desc Text `yaml:"desc"`
		}
		if err := yaml.UnmarshalWithOptions(out, &back, yaml.Strict()); err != nil {
			t.Fatalf("%q encoded to\n%s\nwhich will not parse back: %v", s, out, err)
		}
		if string(back.Desc) != s {
			t.Fatalf("round trip changed the string:\n in:  %q\n out: %q\n yaml:\n%s", s, string(back.Desc), out)
		}
	})
}

// FuzzNestedTextRoundTrip is the same property for the type used at every
// depth below a top-level list item's own fields — an exit's desc, an
// extra description's desc.
//
// Worth its own target rather than folding into the one above: the two
// types differ in exactly the case that is hardest to reason about (when
// an indentation indicator may be emitted), NestedText falls back to a
// quoted scalar where Text does not, and a bug in one would not be a bug
// in the other.
func FuzzNestedTextRoundTrip(f *testing.F) {
	for _, seed := range textSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			t.Skip()
		}
		// Nested one level deeper than the flat case, which is the whole
		// point of the type: the re-print's fixed two-space shift is what
		// makes a hand-built indentation indicator unreliable here.
		var doc struct {
			Exit struct {
				Desc NestedText `yaml:"desc"`
			} `yaml:"exit"`
		}
		doc.Exit.Desc = NestedText(s)
		out, err := yaml.MarshalWithOptions(doc, yaml.Indent(2))
		if err != nil {
			t.Fatalf("marshalling %q: %v", s, err)
		}
		var back struct {
			Exit struct {
				Desc NestedText `yaml:"desc"`
			} `yaml:"exit"`
		}
		if err := yaml.UnmarshalWithOptions(out, &back, yaml.Strict()); err != nil {
			t.Fatalf("%q encoded to\n%s\nwhich will not parse back: %v", s, out, err)
		}
		if string(back.Exit.Desc) != s {
			t.Fatalf("round trip changed the string:\n in:  %q\n out: %q\n yaml:\n%s",
				s, string(back.Exit.Desc), out)
		}
	})
}

// textSeeds are the shapes that have already cost this project time, plus
// the structural bytes a YAML encoder has to decide about.
//
// Committed as Go rather than as testdata/fuzz/ files because they are the
// documented cases, not discovered ones: a crasher the fuzzer finds gets
// written to testdata/fuzz/ by `go test` itself and committed from there,
// which is the normal Go discipline and gives the regression suite for
// free. These are the starting points that discipline needs.
func textSeeds() []string {
	return []string{
		"",
		"a",
		"one line, no newline",
		"one line, trailing newline\n",
		// The three needsQuoting cases.
		"a deliberate blank line before the end\n\n",
		"a bare carriage return\rmid-string\n",
		"trailing whitespace and no newline   ",
		// CircleMUD's own three-space paragraph indent: more leading
		// whitespace on the first line than on any later one, which is
		// what the indentation indicator exists for.
		"   indented first line\nflush left second line\n",
		"\ttab-indented first line\nflush left second line\n",
		// ASCII art: leading whitespace everywhere, of differing widths.
		"        +------+\n        | sign |\n        +------+\n",
		// The colour markup, which YAML would otherwise read as a flow
		// mapping opener.
		"{{red}}danger{{/}}",
		"{{red}}\nmulti-line colour\n{{/}}\n",
		// Structural YAML bytes as content.
		"- not a list item",
		"key: not a mapping",
		"#not a comment",
		"|not a block header",
		">not a folded header",
		"...\n",
		"---\n",
		"\"quoted\" and 'quoted'",
		"a: b\nc: d\n",
		// Whitespace-only, in the shapes that are easy to normalise away.
		" ",
		"\n",
		"\n\n\n",
		"\r\n",
		" \n ",
		strings.Repeat("long line ", 40),
	}
}
