// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"fmt"
	"strconv"
	"strings"
)

// Colour markup for the yaml data format, docs/design/data-format.md
// §5. A raw ESC byte cannot appear anywhere in a YAML stream (§5.2), so
// colour in the data is symbolic: {{red}}, {{bright-red}}, {{/}} for reset,
// {{sgr:...}} for an arbitrary SGR sequence the named table doesn't cover,
// and {{lbrace}} for a literal "{{". Nothing in the Go tree renders colour
// today (§5.1's survey found none), so this is new machinery rather than a
// port of anything the C does at runtime — the C's colour is compiled into
// call-site macros in screen.h, not parsed from data at all.

// ColourLevel is the two-bit level PrefColour1/PrefColour2 encode.
// Engine-chrome colour distinguishes all four C levels (C_OFF/C_SPR/C_NRM/
// C_CMP); data-borne colour does not, per §5.4 — every {{...}} code renders
// from ColourNormal up, which is why this type has only two values.
type ColourLevel int

const (
	// ColourOff strips every code, leaving plain text.
	ColourOff ColourLevel = iota
	// ColourNormal and above renders codes to ANSI.
	ColourNormal
)

// LevelFor derives the two-bit colour level from a player's preference
// flags, matching PRF_COLOR_1/PRF_COLOR_2 (playerflags.go). Any combination
// with at least one bit set renders; neither bit set is C_OFF.
func LevelFor(prefs Preferences) ColourLevel {
	if prefs.HasAny(PrefColour1, PrefColour2) {
		return ColourNormal
	}
	return ColourOff
}

// namedCodes is §5.3's table: the doubled-brace name to screen.h's SGR
// parameter. Order does not matter; this is a lookup, not a bit table.
var namedCodes = map[string]string{
	"/":              "0",
	"red":            "31",
	"green":          "32",
	"yellow":         "33",
	"blue":           "34",
	"magenta":        "35",
	"cyan":           "36",
	"white":          "37",
	"bright-red":     "1;31",
	"bright-green":   "1;32",
	"bright-yellow":  "1;33",
	"bright-blue":    "1;34",
	"bright-magenta": "1;35",
	"bright-cyan":    "1;36",
	"bright-white":   "1;37",
}

// sgrToName is namedCodes' inverse, for demoting an ANSI escape read from a
// classic file (§5.5's import direction). Built once from namedCodes so the
// two tables cannot drift apart — a name added to one appears in the other
// automatically.
var sgrToName = func() map[string]string {
	m := make(map[string]string, len(namedCodes))
	for name, sgr := range namedCodes {
		m[normaliseSGR(sgr)] = name
	}
	return m
}()

// Token is one piece of a parsed colour string: either literal text (Code
// == "") or a colour code.
type Token struct {
	Text string // literal text, when Code == ""
	Code string // "red", "bright-red", "/", "sgr:31;1", or "" for text
}

// ParseColour splits s into text and colour tokens, recognising §5.3's
// grammar: {{name}}, {{sgr:...}}, {{lbrace}} for a literal "{{". A "{{" that
// does not close before the string ends, or that names nothing in the
// table and isn't "sgr:" or "lbrace", is returned as literal text — the
// writer's job (§5.3) is to quote strings that would be misread, not the
// parser's to guess.
func ParseColour(s string) []Token {
	var out []Token
	var text strings.Builder

	flushText := func() {
		if text.Len() > 0 {
			out = append(out, Token{Text: text.String()})
			text.Reset()
		}
	}

	for i := 0; i < len(s); {
		if !strings.HasPrefix(s[i:], "{{") {
			text.WriteByte(s[i])
			i++
			continue
		}
		end := strings.Index(s[i+2:], "}}")
		if end < 0 {
			text.WriteByte(s[i])
			i++
			continue
		}
		inner := s[i+2 : i+2+end]
		next := i + 2 + end + 2

		switch {
		case inner == "lbrace":
			text.WriteString("{{")
		case inner == "/":
			flushText()
			out = append(out, Token{Code: "/"})
		case strings.HasPrefix(inner, "sgr:"):
			flushText()
			out = append(out, Token{Code: inner})
		case isNamedCode(inner):
			flushText()
			out = append(out, Token{Code: inner})
		default:
			// Not a code this table knows: keep the braces as literal text
			// rather than silently eating an author's stray "{{typo}}".
			text.WriteString(s[i:next])
		}
		i = next
	}
	flushText()
	return out
}

func isNamedCode(name string) bool {
	_, ok := namedCodes[name]
	return ok
}

// RenderANSI renders tokens to a string with ANSI escapes, or strips them to
// plain text when level is ColourOff (§5.4).
func RenderANSI(tokens []Token, level ColourLevel) string {
	var b strings.Builder
	for _, t := range tokens {
		if t.Code == "" {
			b.WriteString(t.Text)
			continue
		}
		if level == ColourOff {
			continue
		}
		if sgr, ok := sgrParam(t.Code); ok {
			fmt.Fprintf(&b, "\x1B[%sm", sgr)
		}
	}
	return b.String()
}

// sgrParam returns the SGR parameter for a code token: the named table for
// "red"/"bright-red"/"/", or the raw parameter for "sgr:...".
func sgrParam(code string) (string, bool) {
	if rest, ok := strings.CutPrefix(code, "sgr:"); ok {
		return rest, true
	}
	sgr, ok := namedCodes[code]
	return sgr, ok
}

// Strip removes every colour code, leaving plain text — the default for
// anything that is not a live player socket (§5.4): logs, the parity dump,
// dlctl output, GMCP.
func Strip(s string) string {
	var b strings.Builder
	for _, t := range ParseColour(s) {
		b.WriteString(t.Text)
	}
	return b.String()
}

// DisplayWidth counts s as it would appear on a terminal: every colour code
// is zero-width, matching next_page()'s hand-rolled ESC-skipping in the C
// (§5.4). This is the function the wrapping layer is meant to be built on
// rather than retrofitted to, per §11 step 1b.
func DisplayWidth(s string) int {
	width := 0
	for _, t := range ParseColour(s) {
		width += len([]rune(t.Text))
	}
	return width
}

// Unbalanced reports whether tokens ends inside an open colour — opens with
// a named or sgr code and never returns to {{/}} — which §5.4 calls the most
// common colour bug in MUD data and asks the linter to catch.
func Unbalanced(tokens []Token) bool {
	open := false
	for _, t := range tokens {
		switch t.Code {
		case "":
			continue
		case "/":
			open = false
		default:
			open = true
		}
	}
	return open
}

// DemoteANSI recognises screen.h ANSI escapes in s (as a classic-format file
// would contain them) and rewrites them to named {{...}} codes, per §5.5's
// import direction. A sequence namedCodes doesn't cover survives as
// {{sgr:...}} rather than being dropped, which is what makes the demotion
// total rather than partial. Bold/normal parameter order normalises to
// screen.h's own spelling (\x1B[1;31m and \x1B[31;1m both become
// {{bright-red}}) — the one accepted lossy case §5.5 documents.
func DemoteANSI(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] != 0x1B || i+1 >= len(s) || s[i+1] != '[' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i:], 'm')
		if end < 0 {
			b.WriteByte(s[i])
			i++
			continue
		}
		params := s[i+2 : i+end]
		if name, ok := sgrToName[normaliseSGR(params)]; ok {
			b.WriteString("{{")
			b.WriteString(name)
			b.WriteString("}}")
		} else {
			b.WriteString("{{sgr:")
			b.WriteString(params)
			b.WriteString("}}")
		}
		i += end + 1
	}
	return b.String()
}

// normaliseSGR sorts a semicolon-separated SGR parameter list numerically,
// so "1;31" and "31;1" compare equal — the bold/normal order §5.5 says the
// round trip normalises rather than preserves byte-for-byte.
func normaliseSGR(params string) string {
	parts := strings.Split(params, ";")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return params // not numeric; leave it for the caller to not-match
		}
		nums = append(nums, n)
	}
	for i := 1; i < len(nums); i++ {
		for j := i; j > 0 && nums[j-1] > nums[j]; j-- {
			nums[j-1], nums[j] = nums[j], nums[j-1]
		}
	}
	out := make([]string, len(nums))
	for i, n := range nums {
		out[i] = strconv.Itoa(n)
	}
	return strings.Join(out, ";")
}

// PromoteANSI renders tokens back to raw screen.h ANSI bytes, per §5.5's
// export direction: {{red}} becomes \x1B[31m in a classic-format file,
// because the C server passes ESC through untouched and its pager already
// skips it.
func PromoteANSI(s string) string {
	return RenderANSI(ParseColour(s), ColourNormal)
}
