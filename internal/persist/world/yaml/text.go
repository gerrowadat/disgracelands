// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// Text and NestedText are world strings in the yaml format. Both decode
// like a plain string — goccy resolves any scalar style (plain, quoted,
// literal block) into the Go value the same way regardless of the type it
// decodes into — but each writes under manual control, because the
// canonical-writer requirements in §10.3 of docs/design/data-format.md
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
//   - A block containing a carriage return that is not part of a CRLF
//     pair cannot be a block scalar at all. YAML folds CR, CRLF and LF
//     alike into a single '\n' when it parses (spec §5.4), so a bare CR
//     is not merely awkward to emit — it is unrepresentable in that
//     style, and the parser reads it as a line break with no indentation
//     and rejects the block. Both types escape such a string instead.
//     See ToStored for where bare CRs come from, which is not a
//     hypothetical.
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
	if needsQuoting(s) {
		return quotedScalar(s), nil
	}
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
	if needsQuoting(s) {
		return quotedScalar(s), nil
	}
	if !strings.Contains(s, "\n") {
		return marshalSingleLine(s)
	}
	body := strings.TrimRight(s, "\n")
	if needsIndentIndicator(strings.Split(body, "\n")) {
		// Falls back to a quoted, escaped scalar rather than a literal
		// block: see the package doc comment for why this depth cannot
		// reliably use an indentation indicator.
		return quotedScalar(s), nil
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

// quotedScalar renders s as a YAML double-quoted scalar, rather than asking
// the library to pick a style for it.
//
// NestedText's fallback used to be a plain yaml.Marshal of the string, on
// the understanding that the library quotes anything it cannot safely emit
// as a literal block. It does not: goccy picks double-quoted for content
// whose first line begins with a *space*, and a literal block for content
// whose first line begins with a *tab* — which is the one input the
// fallback exists to handle, and which then fails to parse back. Choosing
// the style here rather than describing the input and hoping is what makes
// the fallback actually a fallback.
//
// strconv.Quote's escapes are a subset of YAML's double-quoted ones —
// \\, \", \n, \r, \t, \a, \b, \f, \v, and \xNN/\uNNNN for anything else
// non-printable — so its output is a valid YAML scalar as it stands.
func quotedScalar(s string) []byte { return []byte(strconv.Quote(s) + "\n") }

// needsQuoting reports whether s has to be written as an escaped scalar
// because a literal block cannot carry it back unchanged. Each case below
// is a string the block form loses or rejects, established by round-tripping
// it rather than by reading the spec and hoping:
//
//   - A bare carriage return. Unrepresentable in the style at all; see the
//     package doc comment.
//   - Trailing blank lines. goccy re-parses and re-prints whatever a custom
//     MarshalYAML returns, and its re-print of a literal node
//     (ast.LiteralNode.String(), which unconditionally right-trims every
//     trailing newline off the node's content) collapses any number of them
//     to one, regardless of the "+"/keep chomping indicator asking it not
//     to. Nothing about how the returned bytes are built changes that.
//   - Trailing whitespace on a last line that has no newline after it. The
//     same re-print drops it. A trailing *tab* there happens to survive
//     where a trailing *space* does not, which is exactly the kind of
//     distinction not worth depending on: both are quoted.
//
// This used to be TrimsTrailingBlankLines, which named the second case and
// reported it as an accepted lossy transform for a linter to warn about.
// The transform is not accepted any more, because it did not have to be —
// quoting costs nothing but prettiness, and a trailing blank line is a
// deliberate blank line in a room description on the way to a player's
// screen. A real world file wrote one, which is what made the difference
// between "documented normalisation" and "silently altered text" concrete.
func needsQuoting(s string) bool {
	if strings.ContainsRune(s, '\r') {
		return true
	}
	if strings.HasSuffix(s, "\n\n") {
		return true
	}
	if n := len(s); n > 0 && (s[n-1] == ' ' || s[n-1] == '\t') {
		return true
	}
	return false
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
//
// "Leading whitespace" means a leading *tab* as well as a leading space,
// and the tab case is not a refinement of the space one — it is the
// difference between a document that parses and one that does not. YAML
// forbids a tab anywhere it would be read as indentation (spec §6.1), so a
// literal block whose first content line opens with one has no indentation
// for the parser to infer and the emitter's output is rejected outright,
// where the leading-space case merely round-trips to a different string.
// An explicit indicator settles it: with `|2` present the parser stops
// inferring, the two-space prefix literalBlock writes is the indentation,
// and every tab after it is content like any other character. NestedText,
// which cannot use an indicator at all, falls back to a quoted scalar for
// the same input and is equally safe.
func needsIndentIndicator(lines []string) bool {
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		return leadingWhitespace(l) > 0
	}
	return false
}

// ToStored strips the CRLF line endings classic.readString bakes into every
// multi-line text field it reads (reader.go's readString: every line that
// does not carry the terminating '~' gets "\r\n" appended, reproducing
// fread_string's overwrite of the trailing '\n'). YAML cannot represent CRLF
// distinctly from LF at all — block scalars normalise every line-break
// style to '\n' on decode, verified empirically rather than assumed — so
// the yaml format's stored form is always LF-only, and this is the
// conversion applied once, on the way into a YAML document.
//
// The replacement is lossless in both directions for every '\r' readString
// itself inserts, because it always inserts the pair together. What it is
// not is a guarantee that a stored string holds no '\r' at all: readString
// appends CRLF to the line it read, and says nothing about what was *in*
// that line. A world file whose text already carries carriage returns —
// an editor or a paste that left them behind, which happens and is not
// rare enough to treat as corrupt — survives ToStored as a string with
// bare CRs in it, since only the pairs matched.
//
// That is why Text and NestedText check for a bare '\r' before choosing a
// block scalar: the pairs are converted here, and whatever is left is real
// content that has to be escaped rather than folded away. Preserving it is
// the fidelity answer as well as the mechanical one — the C sends those
// bytes to the client exactly as they sit in the file.
func ToStored(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// FromStored is ToStored's inverse, applied on the way out of a YAML
// document into a game.*Def field: it re-derives the CRLF convention every
// other loader (classic, and the C server itself) uses in memory, so a
// world loaded via yaml looks identical to one loaded via classic
// regardless of which file format produced it.
func FromStored(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

func leadingWhitespace(s string) int {
	n := 0
	for n < len(s) && (s[n] == ' ' || s[n] == '\t') {
		n++
	}
	return n
}
