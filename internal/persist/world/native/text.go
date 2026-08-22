// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package native

import (
	"strings"

	"github.com/goccy/go-yaml"
)

// Text and NestedText are world strings in the native format. Both decode
// like a plain string — goccy resolves any scalar style (plain, quoted,
// literal block) into the Go value the same way regardless of the type it
// decodes into — but each writes under manual control, because the
// canonical-writer requirements in §10.3 of docs/proposals/data-format.md
// are not something the library's automatic literal-style heuristic gets
// right in every case, verified empirically rather than assumed:
//
//   - A block whose first non-empty line carries more leading whitespace
//     than a later line — CircleMUD's own room-description convention of
//     indenting a paragraph's first line by three spaces (§4.6) — decodes
//     back to a *different* string than it encoded, or fails to parse at
//     all, because plain YAML infers the block's indentation from the
//     first line. The fix is the indentation indicator (`|2`), and the
//     library never emits one on its own.
//   - Whatever a custom MarshalYAML returns is re-parsed and re-embedded
//     into the surrounding document by re-printing the parsed node, and
//     that re-print adds a *fixed* two-space shift to every line
//     regardless of how deeply the field is actually nested — not a
//     shift proportional to nesting depth, which is what would be needed
//     for an indentation-indicator header to mean the same thing at every
//     depth. So a hand-built `|2` header is only reliable at the one
//     depth it was validated against.
//
// Text is that one depth: a field written directly on a top-level list
// item (a room, mobile, object or shop's own `name`/`desc`/`short`/`long`
// fields — always two YAML-file columns deep, because `rooms:`/`mobiles:`/
// etc. are always top-level lists). Validated against every string in the
// real corpus (text_corpus_test.go) with this exact recipe.
//
// NestedText is everything nested deeper — an exit's `desc`, an
// extra-description's `desc` — where the same recipe was tried and
// produces YAML that fails to parse back at all (verified, not assumed:
// see the room/exit ASCII-art signs in the real corpus, which are exactly
// this case). Rather than chase the correct per-depth constant by hand for
// every place this format nests text, NestedText only ever emits a literal
// block when no indentation indicator is needed — safe at any depth, also
// verified — and falls back to a quoted, escaped scalar (still a correct,
// lossless encoding, just not a pretty one) for the rare content that
// would need one. Real-corpus incidence of that fallback is four strings
// out of over twelve thousand, all hand-drawn ASCII signs on a room's
// extra-description, so the readability cost is negligible in practice.
type Text string

// MarshalYAML implements yaml.BytesMarshaler.
func (t Text) MarshalYAML() ([]byte, error) {
	s := string(t)
	if !strings.Contains(s, "\n") {
		return marshalSingleLine(s)
	}
	return []byte(literalBlock(s, 2, true)), nil
}

// NestedText is Text's counterpart for anything nested deeper than a
// top-level list item's own fields. See the package-level doc comment.
type NestedText string

// MarshalYAML implements yaml.BytesMarshaler.
func (t NestedText) MarshalYAML() ([]byte, error) {
	s := string(t)
	if !strings.Contains(s, "\n") {
		return marshalSingleLine(s)
	}
	body := strings.TrimRight(s, "\n")
	if needsIndentIndicator(strings.Split(body, "\n")) {
		// Falls back to a quoted, escaped scalar rather than a literal
		// block: see the package doc comment for why this depth cannot
		// reliably use an indentation indicator.
		return yaml.Marshal(s)
	}
	// nominalIndent here only needs to clear the deepest column this
	// format's schema actually reaches (validated up to an exit's `desc`,
	// six columns) with room to spare; it is not tied to real depth the
	// way the indicator case would need it to be.
	return []byte(literalBlock(s, 10, false)), nil
}

// marshalSingleLine delegates to the library for its own quoting rules —
// verified to already do the one thing this format needs on top of stock
// YAML: a value beginning "{{" (§5.3's colour markup) is quoted rather
// than misread as a flow-mapping opener.
func marshalSingleLine(s string) ([]byte, error) { return yaml.Marshal(s) }

// TrimsTrailingBlankLines reports whether encoding s through Text or
// NestedText will normalise away trailing blank lines, so a caller can
// raise a lint finding for it — the same "reported rather than refused"
// posture §5.5 sets for colour normalisation.
//
// The reason, verified rather than guessed at: goccy re-parses and
// re-prints whatever a custom MarshalYAML returns while splicing it into
// the surrounding document, and its re-print of a literal node
// (ast.LiteralNode.String(), which unconditionally right-trims every
// trailing newline off the node's content before re-emitting it) collapses
// any number of trailing blank lines to at most one, regardless of the
// "+"/keep chomping indicator asking it to preserve them. This reproduces
// identically no matter how the returned bytes are built, so it is not a
// bug this function can route around — it is normalised on purpose here,
// to a single trailing newline, rather than emitting a "+2" header the
// library cannot actually honour.
func TrimsTrailingBlankLines(s string) bool {
	return strings.HasSuffix(s, "\n\n")
}

// literalBlock renders s as a YAML literal block scalar: a chomping
// indicator chosen from s's trailing newlines, and — only when
// withIndicator is true and the content needs one — an indentation
// indicator of 2. nominalIndent is the content indent used inside the
// returned bytes; see Text and NestedText's doc comments for why the two
// callers need different values and different tolerance for the indicator
// case.
func literalBlock(s string, nominalIndent int, withIndicator bool) string {
	header := "|"
	if !strings.HasSuffix(s, "\n") {
		header += "-"
	}
	// See TrimsTrailingBlankLines: a run of trailing newlines beyond the one
	// clip chomping keeps cannot survive this path, so it is collapsed here
	// rather than left to fail unpredictably later.
	body := strings.TrimRight(s, "\n")

	lines := strings.Split(body, "\n")
	if withIndicator && needsIndentIndicator(lines) {
		header = "|2" + header[1:]
	}
	prefix := strings.Repeat(" ", nominalIndent)

	var b strings.Builder
	b.WriteString(header)
	b.WriteByte('\n')
	for _, line := range lines {
		if line != "" {
			b.WriteString(prefix)
			b.WriteString(line)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// needsIndentIndicator reports whether relying on the default "first
// non-empty line sets the indentation" rule is safe. It is not safe (an
// indicator is required) whenever the first non-empty line has any leading
// whitespace at all — not only when a later line has less, which is what
// the YAML spec itself requires an indicator for.
//
// A stricter check (require one only when a *later* line has fewer leading
// spaces than the first) looks more precise and was tried first, but a
// single-content-line block — the common case here, e.g. a one-line room
// description with the three-space paragraph indent and nothing after it —
// has no later line to compare against, and the reparse-and-reprint Text
// and NestedText go through (see literalBlock and TrimsTrailingBlankLines)
// collapses that leading whitespace to nothing without an indicator
// present, verified against the real corpus rather than assumed.
func needsIndentIndicator(lines []string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		return leadingSpaces(l) > 0
	}
	return false
}

// ToStored strips the CRLF line endings classic.readString bakes into every
// multi-line text field it reads (reader.go's readString: every line that
// does not carry the terminating '~' gets "\r\n" appended, reproducing
// fread_string's overwrite of the trailing '\n'). YAML cannot represent CRLF
// distinctly from LF at all — block scalars normalise every line-break
// style to '\n' on decode, verified empirically rather than assumed — so
// the native format's stored form is always LF-only, and this is the
// conversion applied once, on the way into a YAML document.
//
// The replacement is lossless in both directions because readString never
// produces a bare '\r': it always inserts the pair together, so every '\r'
// in a loaded description is immediately followed by '\n'.
func ToStored(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// FromStored is ToStored's inverse, applied on the way out of a YAML
// document into a game.*Def field: it re-derives the CRLF convention every
// other loader (classic, and the C server itself) uses in memory, so a
// world loaded via native looks identical to one loaded via classic
// regardless of which file format produced it.
func FromStored(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

func leadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}
