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
