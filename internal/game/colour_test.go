// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// The swearing social from data/misc/socials:133, §5.3's named round-trip
// test case: it contains every character that would need escaping under a
// single-character sigil scheme, and no "{{", so under this scheme it needs
// none at all.
const swearingSocial = `$n swears: #@*"*&^$$%@*&!!!!!!`

func TestSwearingSocialSurvivesParseRenderStrip(t *testing.T) {
	tokens := ParseColour(swearingSocial)
	if got := RenderANSI(tokens, ColourOff); got != swearingSocial {
		t.Fatalf("RenderANSI(ColourOff) = %q, want %q", got, swearingSocial)
	}
	if got := RenderANSI(tokens, ColourNormal); got != swearingSocial {
		t.Fatalf("RenderANSI(ColourNormal) = %q, want %q", got, swearingSocial)
	}
	if got := Strip(swearingSocial); got != swearingSocial {
		t.Fatalf("Strip = %q, want %q", got, swearingSocial)
	}
	if got := DisplayWidth(swearingSocial); got != len([]rune(swearingSocial)) {
		t.Fatalf("DisplayWidth = %d, want %d", got, len([]rune(swearingSocial)))
	}
}

func TestParseColourNamedCode(t *testing.T) {
	tokens := ParseColour("{{red}}blood{{/}} everywhere")
	want := []Token{
		{Code: "red"},
		{Text: "blood"},
		{Code: "/"},
		{Text: " everywhere"},
	}
	if len(tokens) != len(want) {
		t.Fatalf("got %d tokens, want %d: %+v", len(tokens), len(want), tokens)
	}
	for i := range want {
		if tokens[i] != want[i] {
			t.Errorf("token %d: got %+v, want %+v", i, tokens[i], want[i])
		}
	}
}

func TestRenderANSIStripsAtColourOff(t *testing.T) {
	tokens := ParseColour("{{red}}blood{{/}}")
	if got := RenderANSI(tokens, ColourOff); got != "blood" {
		t.Fatalf("got %q, want %q", got, "blood")
	}
}

func TestRenderANSIRendersAtColourNormal(t *testing.T) {
	tokens := ParseColour("{{red}}blood{{/}}")
	want := "\x1B[31mblood\x1B[0m"
	if got := RenderANSI(tokens, ColourNormal); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSGRPassthrough(t *testing.T) {
	s := "{{sgr:38;5;208}}orange{{/}}"
	tokens := ParseColour(s)
	want := "\x1B[38;5;208morange\x1B[0m"
	if got := RenderANSI(tokens, ColourNormal); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := Strip(s); got != "orange" {
		t.Fatalf("Strip = %q, want %q", got, "orange")
	}
}

func TestLbraceLiteral(t *testing.T) {
	tokens := ParseColour("{{lbrace}}not a code}}")
	if len(tokens) != 1 || tokens[0].Code != "" || tokens[0].Text != "{{not a code}}" {
		t.Fatalf("got %+v", tokens)
	}
}

func TestUnknownCodeSurvivesAsText(t *testing.T) {
	s := "{{nonsense}}"
	tokens := ParseColour(s)
	if len(tokens) != 1 || tokens[0].Code != "" || tokens[0].Text != s {
		t.Fatalf("got %+v, want literal %q", tokens, s)
	}
}

func TestUnbalanced(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"{{red}}open{{/}}closed", false},
		{"{{red}}never closes", true},
		{"plain text", false},
		{"{{red}}{{/}}{{blue}}", true},
	}
	for _, c := range cases {
		if got := Unbalanced(ParseColour(c.s)); got != c.want {
			t.Errorf("Unbalanced(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestDisplayWidthTreatsCodesAsZeroWidth(t *testing.T) {
	s := "{{red}}hello{{/}}"
	if got := DisplayWidth(s); got != 5 {
		t.Fatalf("DisplayWidth(%q) = %d, want 5", s, got)
	}
}

func TestDemoteANSINamedColour(t *testing.T) {
	s := "\x1B[31mblood\x1B[0m"
	want := "{{red}}blood{{/}}"
	if got := DemoteANSI(s); got != want {
		t.Fatalf("DemoteANSI(%q) = %q, want %q", s, got, want)
	}
}

func TestDemoteANSINormalisesBoldOrder(t *testing.T) {
	a := DemoteANSI("\x1B[1;31mx")
	b := DemoteANSI("\x1B[31;1mx")
	if a != b {
		t.Fatalf("normalisation mismatch: %q vs %q", a, b)
	}
	if a != "{{bright-red}}x" {
		t.Fatalf("got %q, want {{bright-red}}x", a)
	}
}

func TestDemoteANSIUnknownSequenceSurvivesAsSGR(t *testing.T) {
	s := "\x1B[38;5;208morange"
	want := "{{sgr:38;5;208}}orange"
	if got := DemoteANSI(s); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestClassicRoundTripSyntheticColour(t *testing.T) {
	original := "\x1B[31mblood\x1B[0m and \x1B[36mwater\x1B[0m"
	yaml := DemoteANSI(original)
	back := PromoteANSI(yaml)
	if back != original {
		t.Fatalf("round trip: got %q, want %q", back, original)
	}
}

func TestLevelFor(t *testing.T) {
	if LevelFor(0) != ColourOff {
		t.Fatal("no colour prefs should be ColourOff")
	}
	if LevelFor(PrefColour1) != ColourNormal {
		t.Fatal("PrefColour1 alone should render")
	}
	if LevelFor(PrefColour1|PrefColour2) != ColourNormal {
		t.Fatal("both colour bits should render")
	}
}
