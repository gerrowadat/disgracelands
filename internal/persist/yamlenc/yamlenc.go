// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package yamlenc holds the encoding options every yaml format in this
// tree writes with, so there is one answer to "how does a string get onto
// disk" rather than sixteen.
//
// It exists because of one finding, and the finding generalises further
// than the bug did. `internal/persist/world/yaml`'s Text type had already
// been taught not to trust the library's own quoting decisions for a
// world description — goccy emits a string containing a tab as a *plain*
// scalar, and the parser then eats the tab, so "the\tTester" is written
// and "theTester" is read back. But Text is used for descriptions and
// prose. Every other string in every other document — a character's name
// and title, a board heading, a ban's site, an alias, a help keyword —
// was a plain Go `string` marshalled by the library directly, and lost a
// tab in exactly the same way, silently.
//
// Found by FuzzBinaryRecordRoundTrip (cmd/dlctl), which produced a
// character whose name held a tab: the binary reader accepted it, the
// conversion reported success, and the character came back under a
// different name.
//
// The fix is central rather than per-field. Sixteen document structs with
// a hundred string fields between them cannot be audited once and stay
// audited; an encode option applied at every call site can.
package yamlenc

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

// Options are the encode options every Save in this tree uses.
//
// Indent(2) is the format's own house style (docs/design/data-format.md
// §10.3's canonical writer). The custom string marshaler is the policy
// described in the package comment.
//
// Note it applies to `string` and not to named types with a string
// underlying type: goccy matches a CustomMarshaler by reflect type, so
// world/yaml's own Text and NestedText keep their own rules, which are
// stricter still because a block scalar has more ways to go wrong than a
// single-line one.
func Options() []yaml.EncodeOption {
	return []yaml.EncodeOption{
		yaml.Indent(2),
		yaml.CustomMarshaler(MarshalString),
	}
}

// MarshalString writes one string, quoting it whenever a plain scalar
// cannot be trusted to carry it back.
//
// The bar for "plain" is deliberately high — see PlainlySafe. Quoting is
// always lossless and costs only prettiness; guessing which of the
// library's plain-scalar decisions are safe costs a silently altered
// character name.
func MarshalString(s string) ([]byte, error) {
	// A YAML document is UTF-8 by definition, so a string that is not
	// cannot be stored in one — and the encoder does not say so, it
	// substitutes U+FFFD for each offending byte and carries on. That is
	// how a keyword became unreachable and a password hash became
	// unverifiable, both silently, both found only by comparing a
	// converted directory against its source.
	//
	// Refusing is the only honest answer. There is no encoding of a raw
	// 0xFF in a YAML scalar: a double-quoted "\xff" means U+00FF, which
	// is a different byte sequence. The importer's own --encoding flag is
	// what turns a legacy byte into a character, and this error is what
	// tells an operator they need it.
	if !utf8.ValidString(s) {
		return nil, fmt.Errorf("yamlenc: %q is not valid UTF-8 and cannot be written to a YAML document; "+
			"convert the source with `dlctl import --encoding=...` (docs/design/data-format.md §11.1)", s)
	}
	if PlainlySafe(s) {
		return yaml.Marshal(s)
	}
	// strconv.Quote's escapes are a subset of YAML's double-quoted ones —
	// \\, \", \n, \r, \t, \a, \b, \f, \v and \xNN/\uNNNN for anything
	// else non-printable — so its output is a valid YAML scalar as it
	// stands.
	return []byte(strconv.Quote(s) + "\n"), nil
}

// PlainlySafe reports whether s can be emitted as a plain YAML scalar with
// no risk of coming back as something else.
//
// The rule is the YAML spec's own, not a list of the library's bugs:
//
//   - No tab or line break anywhere. A tab cannot appear in a plain
//     scalar at all (spec §5.5, "tabs are not allowed as indentation"),
//     and a plain scalar that spans lines folds its breaks into spaces
//     (§7.3.3) — both silently. goccy emits both anyway: "the\tTester" is
//     written plain and read back as "theTester", and a name of "A\n"
//     comes back as "A".
//
//     Quoting a multi-line string produces an escaped one-liner, which is
//     not pretty. Every field where that would matter — a description, a
//     board body, a mail message — is a Text or NestedText and never
//     reaches here; what is left is names, titles, keywords and hostnames,
//     which have no business spanning lines and are being written escaped
//     precisely because something has gone unusual.
//
//   - No leading or trailing whitespace. A plain scalar's own leading and
//     trailing whitespace is stripped by definition (§7.3.3).
//
//   - The first character is not a c-indicator (§5.3): one of
//     -?:,[]{}#&*!|>'"%@` — every one of which means something structural
//     in that position.
//
//   - The string is not, and does not begin with, a document marker
//     ("---" or "..."). goccy panics with a nil pointer dereference on
//     both, and fails to marshal "...x" at all.
//
//   - The string does not spell one of the core schema's float specials:
//     [-+]?(.inf|.Inf|.INF) or (.nan|.NaN|.NAN), §10.2.1.4. Plain, those
//     resolve to a *float*, not to a string — so a room description of
//     ".NAN" is written `desc: .NAN`, read back as a float, and reaches
//     the string field as "NaN". Three characters in, three characters
//     out, silently different ones.
//
//     This is the clause that most repays being taken from the spec
//     rather than from the failure. goccy resolves only the six unsigned
//     spellings that way — "+.inf" survives, and "true", "null", "12"
//     and "0x1F" all reach a string field as their own text — so an
//     observed-failure rule would have excluded six strings and left
//     "+.inf" to break on a library upgrade. The spec says all eight are
//     floats; all eight are quoted. Nothing in twenty years of world data
//     is a description of "+.inf".
//
// Anchoring on the spec rather than on observed failures is deliberate.
// The observed list grew by one every time the fuzzer was re-run, which
// is a fact about goccy 1.19.2 rather than about the format; and the
// obvious alternative — quote unless the string starts with a letter or a
// digit — turned out to quote every act() message in the corpus, because
// they all start with "$n" or "$N". A thousand escaped lines of the
// most-read files in the tree is a real cost for no safety: "$" has no
// meaning in YAML at all.
//
// Every shape fuzzing has found is excluded by one of the four clauses,
// which is the check that the spec-derived rule is not narrower than the
// evidence:
//
//	"\t0"   tab
//	"0\t0"  tab
//	"A\n"   line break
//	"..."   document marker
//	"---"   document marker
//	"? a"   leading indicator
//	"...x"  document marker
//	".NAN"  float special (found by the first real `make fuzz` budget,
//	        2026-08-29 -- the seed corpus had never produced one)
func PlainlySafe(s string) bool {
	if s == "" {
		return true // the library's own `""`, which round-trips
	}
	if strings.ContainsAny(s, "\t\n\r") {
		return false
	}
	if isSpace(s[0]) || isSpace(s[len(s)-1]) {
		return false
	}
	if strings.ContainsRune(indicators, rune(s[0])) {
		return false
	}
	if strings.HasPrefix(s, "---") || strings.HasPrefix(s, "...") {
		return false
	}
	if isFloatSpecial(s) {
		return false
	}
	return true
}

// isFloatSpecial reports whether s is one of the core schema's float
// spellings that has no digits in it (§10.2.1.4) -- the ones a plain
// scalar resolves to a float rather than to text. The rest of that
// production needs a digit, so it cannot collide with a word.
func isFloatSpecial(s string) bool {
	// One optional sign, not "as many as are there": "+-.inf" is not a
	// float in any schema and has no business being quoted as if it were.
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		s = s[1:]
	}
	switch s {
	case ".inf", ".Inf", ".INF", ".nan", ".NaN", ".NAN":
		return true
	}
	return false
}

// indicators is YAML 1.2 §5.3's c-indicator set: the characters that carry
// structural meaning where a scalar could otherwise begin.
const indicators = "-?:,[]{}#&*!|>'\"%@`"

func isSpace(c byte) bool { return c == ' ' || c == '\t' }
