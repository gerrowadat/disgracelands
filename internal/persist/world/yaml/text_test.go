// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"testing"

	"github.com/goccy/go-yaml"
)

type textDoc struct {
	Desc Text `yaml:"desc"`
}

func roundTripText(t *testing.T, s string) string {
	t.Helper()
	out, err := yaml.MarshalWithOptions(textDoc{Desc: Text(s)}, yaml.Indent(2))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back textDoc
	if err := yaml.UnmarshalWithOptions(out, &back, yaml.Strict()); err != nil {
		t.Fatalf("unmarshal %s: %v", out, err)
	}
	if string(back.Desc) != s {
		t.Fatalf("round trip mismatch:\n input:  %q\n output: %q\n yaml:\n%s", s, string(back.Desc), out)
	}
	return string(out)
}

func TestTextRoundTripLeadingIndentFirstLine(t *testing.T) {
	// The exact case §4.6 documents: CircleMUD's three-space paragraph
	// indent on a room description's first line only.
	s := "   You are in the southern end of the temple hall in the Temple of Midgaard.\n" +
		"The temple has been constructed from giant marble blocks, eternal in\n" +
		"appearance, and most of the walls are covered by ancient wall paintings\n" +
		"picturing Gods, Giants and peasants.\n"
	out := roundTripText(t, s)
	if !containsIndicator(out) {
		t.Fatalf("expected an explicit indentation indicator in output:\n%s", out)
	}
}

func TestTextRoundTripFlushLeft(t *testing.T) {
	s := "At the northern end of the temple hall is a statue and a huge altar.\n"
	out := roundTripText(t, s)
	if containsIndicator(out) {
		t.Fatalf("did not expect an indentation indicator for flush-left text:\n%s", out)
	}
}

func TestTextRoundTripTrailingWhitespace(t *testing.T) {
	roundTripText(t, "line one \nline two\t\n")
}

func TestTextRoundTripNoTrailingNewline(t *testing.T) {
	roundTripText(t, "a description with no trailing newline")
}

func TestTextRoundTripBlankLineMidParagraph(t *testing.T) {
	roundTripText(t, "first paragraph\n\nsecond paragraph\n")
}

// "Keep" chomping (2+ trailing newlines) is a documented gap — see
// literalBlock's comment — rather than a case this test asserts round-trips.
// Nothing in the real corpus exercises it.

func TestTextRoundTripBraceLeadingSingleLine(t *testing.T) {
	roundTripText(t, "{{red}}The Temple Of Midgaard{{/}}")
}

func TestTextRoundTripEmptyString(t *testing.T) {
	roundTripText(t, "")
}

func TestTextRoundTripSingleLineNoSpecialChars(t *testing.T) {
	roundTripText(t, "a breast plate")
}

func TestTextRoundTripSwearingSocial(t *testing.T) {
	roundTripText(t, swearingSocial)
}

func containsIndicator(yamlText string) bool {
	for i := 0; i+2 < len(yamlText); i++ {
		if yamlText[i] == '|' && yamlText[i+1] == '2' {
			return true
		}
	}
	return false
}

const swearingSocial = `$n swears: #@*"*&^$$%@*&!!!!!!`

// TestTextRoundTripLeadingTabFirstLine is the tab counterpart of
// TestTextRoundTripLeadingIndentFirstLine, and it is a harder case than
// the space one rather than a variation on it: YAML forbids a tab in
// indentation outright (spec §6.1), so without the indicator the emitter
// produces a document the parser rejects, where the leading-space case
// merely decoded to a different string. Builders hand-laying out an
// inscription or a signpost inside an extra description reach for the tab
// key exactly as readily as the space bar.
func TestTextRoundTripLeadingTabFirstLine(t *testing.T) {
	s := "\t\tHere lies a builder\n\t\tWho pressed Tab\n"
	out := roundTripText(t, s)
	if !containsIndicator(out) {
		t.Fatalf("expected an explicit indentation indicator in output:\n%s", out)
	}
}

// TestTextRoundTripLeadingTabAfterSpaces covers leading whitespace that
// mixes the two: it needs the indicator for the space and survives the tab
// only because the indicator is there.
func TestTextRoundTripLeadingTabAfterSpaces(t *testing.T) {
	s := "  \tmixed leading whitespace\n\t more of it\n"
	out := roundTripText(t, s)
	if !containsIndicator(out) {
		t.Fatalf("expected an explicit indentation indicator in output:\n%s", out)
	}
}

// TestTextRoundTripTabOnLaterLineOnly pins the boundary: only the *first*
// non-empty line's leading whitespace decides whether an indicator is
// needed, so a tab further down is ordinary content and must not drag the
// document into the indicator case.
func TestTextRoundTripTabOnLaterLineOnly(t *testing.T) {
	s := "a flush-left first line\n\tand a tab-led second one\n"
	out := roundTripText(t, s)
	if containsIndicator(out) {
		t.Fatalf("did not expect an indentation indicator:\n%s", out)
	}
}

// nestedDoc mirrors the real schema's deepest text: a top-level list of
// rooms, each carrying a list of extra descriptions. An extra's desc is
// four columns further in than a room's own, which is the depth
// NestedText exists for.
type nestedDoc struct {
	Rooms []struct {
		VNum   int `yaml:"vnum"`
		Extras []struct {
			Keyword string     `yaml:"keyword"`
			Desc    NestedText `yaml:"desc"`
		} `yaml:"extras"`
	} `yaml:"rooms"`
}

func roundTripNestedText(t *testing.T, s string) string {
	t.Helper()
	var doc nestedDoc
	doc.Rooms = make([]struct {
		VNum   int `yaml:"vnum"`
		Extras []struct {
			Keyword string     `yaml:"keyword"`
			Desc    NestedText `yaml:"desc"`
		} `yaml:"extras"`
	}, 1)
	doc.Rooms[0].VNum = 3001
	doc.Rooms[0].Extras = make([]struct {
		Keyword string     `yaml:"keyword"`
		Desc    NestedText `yaml:"desc"`
	}, 1)
	doc.Rooms[0].Extras[0].Keyword = "sign"
	doc.Rooms[0].Extras[0].Desc = NestedText(s)

	out, err := yaml.MarshalWithOptions(doc, yaml.Indent(2))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back nestedDoc
	if err := yaml.UnmarshalWithOptions(out, &back, yaml.Strict()); err != nil {
		t.Fatalf("unmarshal %s: %v", out, err)
	}
	if got := string(back.Rooms[0].Extras[0].Desc); got != s {
		t.Fatalf("round trip mismatch:\n input:  %q\n output: %q\n yaml:\n%s", s, got, out)
	}
	return string(out)
}

func TestNestedTextRoundTripFlushLeft(t *testing.T) {
	roundTripNestedText(t, "The sign is painted in a neat hand.\nIt has seen better years.\n")
}

// The quoted fallback: NestedText cannot use an indentation indicator (see
// its doc comment), so anything that would need one is escaped instead.
func TestNestedTextRoundTripLeadingSpacesFirstLine(t *testing.T) {
	roundTripNestedText(t, "   an indented first line\nand a flush-left second\n")
}

// The same fallback, reached by a tab rather than a space. This is the
// input the fallback was built for and the one it did not actually
// handle: asking the library to pick a style for it got a literal block
// back, tab and all, which then failed to parse.
func TestNestedTextRoundTripLeadingTabFirstLine(t *testing.T) {
	roundTripNestedText(t, "\tDirections:\n\t\tUp - the tower\n\t\tDown - the cellar\n")
}

// A hand-drawn sign, the shape that made the fallback necessary in the
// first place: leading whitespace on the first line and backslashes
// throughout, both of which the escaped form has to survive.
func TestNestedTextRoundTripAsciiArt(t *testing.T) {
	roundTripNestedText(t, "   /\\_/\\\n  /     \\\n |  ___  |\n  \\_____/\n")
}

// TestTextRoundTripTrailingBlankLine is what a builder means by leaving a
// blank line at the end of a sign or an inscription: a blank line on the
// player's screen. The literal-block form cannot carry it (goccy's
// re-print right-trims the node's trailing newlines whatever the chomping
// indicator asks for), so needsQuoting sends it to the escaped form
// instead. This used to be a documented, accepted normalisation.
func TestTextRoundTripTrailingBlankLine(t *testing.T) {
	roundTripText(t, "The sign reads:\n\nBeware.\n\n")
}

func TestTextRoundTripSeveralTrailingBlankLines(t *testing.T) {
	roundTripText(t, "line one\nline two\n\n\n")
}

// TestTextRoundTripTrailingSpaceOnLastLine covers the other half of the
// same re-print: trailing whitespace on a final line with no newline after
// it. A trailing tab there survives where a trailing space does not, which
// is not a distinction worth resting on, so both are quoted.
func TestTextRoundTripTrailingSpaceOnLastLine(t *testing.T) {
	roundTripText(t, "the description ends with a space\nand no newline ")
}

func TestTextRoundTripTrailingTabOnLastLine(t *testing.T) {
	roundTripText(t, "the description ends with a tab\nand no newline\t")
}

// TestTextRoundTripBareCarriageReturn is the shape a text editor leaves
// behind. A bare CR is not representable in a literal block at all: YAML
// folds CR, CRLF and LF alike to a single '\n' when it parses, so the
// escaped form is the only one that can carry it back.
func TestTextRoundTripBareCarriageReturn(t *testing.T) {
	roundTripText(t, "a line with stray returns\r\r\r\nand another\r\r\n")
}

func TestNestedTextRoundTripTrailingBlankLine(t *testing.T) {
	roundTripNestedText(t, "An inscription reads:\n\nRest here.\n\n")
}

func TestNestedTextRoundTripBareCarriageReturn(t *testing.T) {
	roundTripNestedText(t, "a nested line with stray returns\r\r\nand another\r\r\n")
}
