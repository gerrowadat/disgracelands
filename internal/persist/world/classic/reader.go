package classic

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// reader wraps a world file with the two primitives the C loader reads
// through: get_line() for structural lines and fread_string() for
// tilde-terminated text blocks. Both are reimplemented here from db.c and
// utils.c rather than approximated, because the real world files depend on
// details neither doc/building.txt nor the format's reputation mentions.
//
// Text is handled as opaque bytes. The files are not UTF-8 — lib/world/wld/
// 90.wld contains 0x92, a CP1252 curly apostrophe — and transcoding at load
// time would change the bytes a writer later emits. What encoding to present
// to a player is a per-connection question for the protocol layer, not a
// question for the parser.
type reader struct {
	br   *bufio.Reader
	name string

	// lineNo is the file line number of the most recently consumed line,
	// 1-based, counting comment and blank lines that get_line skips.
	lineNo int

	// pending holds a line pushed back by unreadLine.
	pending    string
	hasPending bool
}

func newReader(r io.Reader, name string) *reader {
	return &reader{br: bufio.NewReader(r), name: name}
}

// rawLine reads one line, stripping only the trailing newline. It does not
// skip comments and it does not touch carriage returns. ok is false at end of
// file.
//
// Keeping the carriage returns matters. Several world files contain runs of
// literal CR bytes before the newline — obj/0.obj's bug object has fifteen —
// and fread_string only ever overwrites the line's *final* character, so
// those runs survive into the loaded string and are what players see.
// Trimming them here would silently differ from the C server on every such
// line. get_line does strip them, which is why the two callers differ.
func (r *reader) rawLine() (line string, ok bool) {
	s, err := r.br.ReadString('\n')
	if s == "" && err != nil {
		return "", false
	}
	r.lineNo++
	return strings.TrimSuffix(s, "\n"), true
}

// getLine returns the next structural line, skipping comment lines (those
// beginning with '*') and blank lines, exactly as utils.c's get_line does.
//
// Note the asymmetry with readString: get_line skips comments, fread_string
// does not, so a '*' at the start of a line inside a description is content.
func (r *reader) getLine() (line string, ok bool) {
	if r.hasPending {
		r.hasPending = false
		return r.pending, true
	}
	for {
		s, ok := r.rawLine()
		if !ok {
			return "", false
		}
		// The skip test is on the line's *first* byte, before any trimming:
		// get_line loops while *temp is '*', '\n' or '\r'. A line beginning
		// with a carriage return is therefore skipped whatever follows it,
		// which is not the same as "the line is blank once trimmed".
		if s == "" || s[0] == '*' || s[0] == '\r' {
			continue
		}
		// Only then does it strip trailing CRs and LFs — unlike
		// fread_string, which keeps them.
		return strings.TrimRight(s, "\r\n"), true
	}
}

// unreadLine pushes one line back, so a parser that reads a line to discover
// it belongs to the next record can hand it back. The C code achieves the
// same thing for objects by returning the line from parse_object and having
// the caller reuse it; a pushback reads better and behaves identically.
func (r *reader) unreadLine(line string) {
	r.pending = line
	r.hasPending = true
}

// readString reads a tilde-terminated text block, reproducing db.c's
// fread_string:
//
//   - Reading stops at the first '~' anywhere in a line, and everything after
//     it on that line is discarded.
//   - Every line that does not contain a '~' contributes its text followed by
//     CRLF, because the C code overwrites the trailing '\n' with "\r\n". A
//     description whose last text line carries the '~' therefore has no
//     trailing CRLF, while one whose '~' sits on its own line does. That
//     distinction is visible to players and has to survive the port.
//   - Comment lines are *not* skipped; a line starting with '*' inside a
//     description is part of the description.
//
// An empty result is returned as the empty string. The C code returns NULL
// there and several call sites test for it; the callers here check for "".
func (r *reader) readString(what string) (string, error) {
	if r.hasPending {
		// A pushed-back line belongs to the structural stream; consuming it as
		// text would silently misparse. Nothing should do this.
		return "", fmt.Errorf("%s: internal error: readString with a pushed-back line", r.where(what))
	}

	var b strings.Builder
	for {
		line, ok := r.rawLine()
		if !ok {
			return "", fmt.Errorf("%s: file ended inside a string (no terminating '~')", r.where(what))
		}
		if i := strings.IndexByte(line, '~'); i >= 0 {
			b.WriteString(line[:i])
			return b.String(), nil
		}
		b.WriteString(line)
		b.WriteString("\r\n")
	}
}

// where formats a file:line prefix for error messages.
func (r *reader) where(what string) string {
	if what == "" {
		return fmt.Sprintf("%s:%d", r.name, r.lineNo)
	}
	return fmt.Sprintf("%s:%d: %s", r.name, r.lineNo, what)
}
