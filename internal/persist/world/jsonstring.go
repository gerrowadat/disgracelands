package world

import (
	"strconv"
	"strings"
)

// Text is a world string in the parity dump.
//
// It exists because encoding/json is lossy for this data. data/world is not
// UTF-8 — wld/90.wld holds byte 0x92, a CP1252 apostrophe — and the standard
// encoder replaces every invalid byte with U+FFFD. Two different corrupt
// bytes would then dump identically, so a diff of two loaders could show no
// difference where a real one exists. For a format whose entire purpose is
// detecting differences, that is disqualifying.
//
// Text escapes byte by byte instead: every byte outside printable ASCII
// becomes \u00XX of its own value. The output is pure ASCII, unambiguous, and
// byte-exact, and — the point of the exercise — it is trivial for the C
// dumper to produce exactly the same bytes without needing to know anything
// about UTF-8.
type Text string

// MarshalJSON implements json.Marshaler.
func (t Text) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.Grow(len(t) + 2)
	b.WriteByte('"')
	for i := 0; i < len(t); i++ {
		b.WriteString(escapeByte(t[i]))
	}
	b.WriteByte('"')
	return []byte(b.String()), nil
}

// escapeByte renders one byte as JSON. The short escapes are the ones JSON
// defines; everything else outside printable ASCII takes the \u00XX form.
//
// Keep this in step with json_escape() in reference/moderncserver/src/worlddump.c — the two must
// produce identical bytes or every string in the world reads as a difference.
func escapeByte(c byte) string {
	switch c {
	case '"':
		return `\"`
	case '\\':
		return `\\`
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	case '\b':
		return `\b`
	case '\f':
		return `\f`
	}
	if c < 0x20 || c > 0x7e {
		const hex = "0123456789abcdef"
		return `\u00` + string([]byte{hex[c>>4], hex[c&0xf]})
	}
	return string(c)
}

// UnmarshalJSON implements json.Unmarshaler, so a dump can be read back.
func (t *Text) UnmarshalJSON(data []byte) error {
	var s string
	if err := strconvUnquote(data, &s); err != nil {
		return err
	}
	*t = Text(s)
	return nil
}

// strconvUnquote decodes a JSON string literal back to its bytes. The \u00XX
// escapes this package writes are all below U+0100, so they map back to
// single bytes; strconv.Unquote handles the rest.
func strconvUnquote(data []byte, out *string) error {
	s, err := strconv.Unquote(string(data))
	if err != nil {
		return err
	}
	// Runes below 0x100 came from single bytes and must go back to single
	// bytes rather than their two-byte UTF-8 encodings.
	var b strings.Builder
	for _, r := range s {
		if r < 0x100 {
			b.WriteByte(byte(r)) //nolint:gosec // guarded by the range check
			continue
		}
		b.WriteRune(r)
	}
	*out = b.String()
	return nil
}
