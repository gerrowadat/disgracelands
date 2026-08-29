// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"context"
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The general line editor, porting string_write (modify.c) and the improved
// editor's own command set (improved-edit.c).
//
// Collect lines until a lone '@', then hand the text to whoever asked for it.
// The C passes a pointer to the string being filled and a magic number saying
// what to do when it is done; a closure says the same thing without the
// magic.
//
// The buffer is held flat — one string with "\r\n" between the lines and
// after the last one — because that is what the C holds and because half of
// what the commands do is defined in terms of it. /f reflows across line
// boundaries, /r substitutes across the whole thing, and /d, /e, /i, /l and
// /n all count '\n' characters rather than indexing a list. Keeping a
// []string here and converting at each edge produced the same answers for
// the five commands that were already ported and would not have for these
// six.

// editText is the C's `*d->str`: the buffer's contents, and whether there is
// a buffer at all.
//
// The two are not the same thing, and the C's own guards depend on the
// difference. `if (*(d->str))` — the test /f, /i, /l and /n are wrapped in
// (improved-edit.c:55,61,70,76) — is a NULL-*pointer* test. A freshly opened
// editor has no buffer yet (string_add allocates one on the first line
// typed, modify.c:132) and neither does one that /c has just freed, so both
// answer "Current buffer empty." But a buffer that /d has emptied is a live
// pointer to "", so the same /l prints a blank line and "0 lines shown."
type editText struct {
	text    string
	present bool
}

// editorResult is improved_editor_execute's return value, from
// improved-edit.h:31-34. The values are the C's, though nothing here depends
// on them.
type editorResult int

const (
	// editorOK: not an editor command at all — buffer the line as text.
	editorOK editorResult = 0
	// editorSave: /s.
	editorSave editorResult = 1
	// editorAbort: /a.
	editorAbort editorResult = 2
	// editorAction: a command ran; do not append the line to the buffer.
	editorAction editorResult = 4
)

// beginEditor puts the session into the line editor.
func (s *Session) beginEditor(maxLength int, done func(text string, saved bool)) {
	s.beginEditorSeeded(maxLength, "", done)
}

// markWriting is string_write's own first two lines (modify.c:100-101):
//
//	if (d->character && !IS_NPC(d->character))
//	  SET_BIT(PLR_FLAGS(d->character), PLR_WRITING);
//
// Every editor goes through here, exactly as every editor in the C goes
// through string_write — mail, a board post, a note, tedit. `mail` sets
// PLR_MAILING as well, in its own command (mail.c:567, whose comment is
// "string_write() sets writing"), which is why that bit is not set here.
//
// Safe to write the record from this goroutine: beginEditor is only ever
// reached from a command, which Dispatcher.Do already runs on the world
// goroutine. Clearing it is the awkward half — see finishEditing.
func (s *Session) markWriting() {
	c := s.Character()
	if c == nil || c.IsNPC() || c.Record == nil {
		return
	}
	c.Record.PlayerFlags = c.Record.PlayerFlags.Set(game.PlayerWriting)
}

// beginEditorSeeded is beginEditor with existing content already in the
// buffer, porting string_write's own plain-editor behaviour when the
// pointer it is handed already points at something: string_add's
// non-empty branch (RECREATE+strcat) appends each typed line onto what
// was already there rather than starting fresh. tedit is the first
// caller with anything to seed — do_tedit shows the file's current
// content before handing the descriptor to string_write with that same
// buffer — so board `write`/mail/description, which always compose new
// text, keep going through beginEditor and see no change at all.
//
// An empty seed leaves the buffer *absent* rather than empty, which is the
// state a fresh editor is really in: see editText.
func (s *Session) beginEditorSeeded(maxLength int, seed string, done func(text string, saved bool)) {
	s.editorBuf = editText{text: seed, present: seed != ""}
	s.editorMax = maxLength
	s.editorDone = done
	s.markWriting()
	s.setState(StateEditing)
}

// handleEditing collects one line of an edited text, porting string_add
// (modify.c:117): the '@' terminator, then the improved-editor commands
// editorCommand answers, then plain buffering.
//
// Every line that leaves the session still editing gets a `] ` back.
// make_prompt's second branch is `else if (d->str) strcpy(prompt, "] ")`
// (comm.c:1008) — before the CON_PLAYING test and after the pager's, so any
// descriptor with a string being written to prompts that way whatever state
// it is in. The C writes it once per pass of the game loop, which for a
// player typing is once per line.
//
// Without it the editor is completely silent: a player types a line and gets
// nothing back, with no sign the server took it. That is issue #192.
func (s *Session) handleEditing(ctx context.Context, deps Deps, line string) error {
	err := s.editLine(ctx, deps, line)
	if err == nil && s.State() == StateEditing {
		s.Send("%s", prompt(s))
	}
	return err
}

// editLine is string_add itself, without the prompt.
func (s *Session) editLine(ctx context.Context, deps Deps, line string) error {
	if strings.TrimSpace(line) == descriptionTerminator {
		return s.finishEditing(ctx, deps, true)
	}
	if handled, err := s.editorCommand(ctx, deps, line); handled {
		return err
	}
	s.editorBuf.text += line + "\r\n"
	s.editorBuf.present = true
	return nil
}

// editorCommand runs one improved-editor command against the session's
// buffer, or reports that the line was not one.
//
// The work is in improvedEditorExecute, which is a plain function over a
// buffer so that editoracle_test.go can drive it against the C directly.
// All this does is move the answer into and out of the session.
func (s *Session) editorCommand(ctx context.Context, deps Deps, line string) (bool, error) {
	result, buf, sent := improvedEditorExecute(s.editorBuf, s.editorMax, line)
	if result == editorOK {
		return false, nil
	}
	s.editorBuf = buf
	if sent != "" {
		s.Send("%s", sent)
	}
	switch result {
	case editorAbort:
		return true, s.finishEditing(ctx, deps, false)
	case editorSave:
		return true, s.finishEditing(ctx, deps, true)
	}
	return true, nil
}

// improvedEditorExecute runs one line through the improved editor's command
// switch, porting improved_editor_execute (improved-edit.c:27). It returns
// what the C returns, the buffer as the command left it, and the text the C
// would have written to the descriptor.
//
// CONFIG_IMPROVED_EDITOR is hardcoded to 1 in the archived server's
// improved-edit.h:6, so all eleven commands were always on — this was never
// a stock/optional feature. docs/deviations.md has the handful of places
// this port cannot follow the C, all of them memory-safety ones.
//
// Note that a bare "/" is a command: the C reads str[1] after clearing
// str[0], finds the terminator, and falls to "Invalid option." Only a line
// that does not start with '/' at all is text.
func improvedEditorExecute(buf editText, max int, line string) (editorResult, editText, string) {
	if !strings.HasPrefix(line, "/") {
		return editorOK, buf, ""
	}
	// `strncpy(actions, str + 2, ...)` — everything past the letter, taken
	// before `*str = '\0'` blanks the slash.
	var letter byte
	if len(line) > 1 {
		letter = line[1]
	}
	args := ""
	if len(line) > 2 {
		args = line[2:]
	}

	switch letter {
	case 'a':
		return editorAbort, buf, ""
	case 's':
		return editorSave, buf, ""
	case 'c':
		if buf.present {
			// free(*d->str); *(d->str) = NULL — not just emptied.
			return editorAction, editText{}, "Current buffer cleared.\r\n"
		}
		return editorAction, buf, "Current buffer empty.\r\n"
	case 'h':
		return editorAction, buf, editorHelpText
	case 'd':
		buf, sent := editorDelete(buf, args)
		return editorAction, buf, sent
	case 'e':
		buf, sent := editorEditLine(buf, args, max)
		return editorAction, buf, sent
	case 'r':
		buf, sent := editorReplace(buf, args, max)
		return editorAction, buf, sent
	// /f, /i, /l and /n are the four the C guards with `if (*(d->str))`
	// before dispatching. /d, /e and /r are not, and each does its own
	// check — or, in /r's case, does not; see editorReplace.
	case 'f', 'i', 'l', 'n':
		if !buf.present {
			return editorAction, buf, "Current buffer empty.\r\n"
		}
		var sent string
		switch letter {
		case 'f':
			buf, sent = editorFormat(buf, args, max)
		case 'i':
			buf, sent = editorInsert(buf, args, max)
		case 'l':
			sent = editorList(buf, args)
		case 'n':
			sent = editorListNumbered(buf, args)
		}
		return editorAction, buf, sent
	}
	return editorAction, buf, "Invalid option.\r\n"
}

// editorHelpText is parse_action's PARSE_HELP text
// (improved-edit.c:104-121), verbatim. All eleven commands it lists work.
const editorHelpText = "Editor command formats: /<letter>\r\n\r\n" +
	"/a         -  aborts editor\r\n" +
	"/c         -  clears buffer\r\n" +
	"/d#        -  deletes a line #\r\n" +
	"/e# <text> -  changes the line at # with <text>\r\n" +
	"/f         -  formats text\r\n" +
	"/fi        -  indented formatting of text\r\n" +
	"/h         -  list text editor commands\r\n" +
	"/i# <text> -  inserts <text> before line #\r\n" +
	"/l         -  lists buffer\r\n" +
	"/n         -  lists buffer with line numbers\r\n" +
	"/r 'a' 'b' -  replace 1st occurance of text <a> in buffer with text <b>\r\n" +
	"/ra 'a' 'b'-  replace all occurances of text <a> within buffer with text <b>\r\n" +
	"              usage: /r[a] 'pattern' 'replacement'\r\n" +
	"/s         -  saves text\r\n"

// maxEditorLine stands in for the C's 999999 (improved-edit.c:225,232), its
// own way of saying "to the end" in an sscanf that always wants two numbers.
// /l's range header is printed or not by comparing against it directly, so
// it is a value and not just a large number.
const maxEditorLine = 999999

// lineStart returns the offset of line n (1-based) in a buffer whose lines
// end "\r\n", porting the walk every one of these commands opens with:
//
//	i = 1; s = *d->str;
//	while (s && i < line_low)
//	  if ((s = strchr(s, '\n')) != NULL) { i++; s++; }
//
// The subtlety is where it stops. `s++` past the buffer's final '\n' lands
// on the terminator, which is a perfectly good pointer to an empty string —
// so a three-line buffer *has* a line 4, and only line 5 is out of range.
// That is why /d 4 on three lines answers "0 lines deleted." rather than
// "Line(s) out of range", and why /e 4 and /i 4 append instead of failing.
func lineStart(text string, n int) (int, bool) {
	p := 0
	for i := 1; i < n; i++ {
		j := strings.IndexByte(text[p:], '\n')
		if j < 0 {
			return 0, false
		}
		p += j + 1
	}
	return p, true
}

// linePlural is the C's own `(n != 1 ? "s " : " ")`, which carries the
// space after the word rather than before it: "1 line deleted." and
// "2 lines deleted." from one format string.
func linePlural(n int) string {
	if n != 1 {
		return "s "
	}
	return " "
}

// scanLineRange ports `sscanf(string, " %d - %d ", &line_low, &line_high)`
// (improved-edit.c:163,222,284), returning the number of conversions it made
// as well as the values, because each caller switches on that count.
//
// Two conversions means a range, one means a single line with the rest of
// the argument ignored (so "/l 3x" lists line 3, the '-' having failed to
// match against 'x'), and none means the argument held no number at all.
//
// The deviation: sscanf returns EOF, not 0, when the argument is empty or
// all whitespace, and every switch reading it has cases 0, 1 and 2 and no
// default — leaving line_low and line_high uninitialised. This returns 0 for
// that case, which is what the code plainly meant. See docs/deviations.md.
func scanLineRange(s string) (n, low, high int) {
	i := 0
	// %d: leading whitespace, an optional sign, then a run of digits.
	readInt := func() (int, bool) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n' || s[i] == '\f' || s[i] == '\v') {
			i++
		}
		j := i
		if j < len(s) && (s[j] == '-' || s[j] == '+') {
			j++
		}
		digits := j
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == digits {
			return 0, false
		}
		// The C's %d is an int, and overflows it; this saturates instead.
		// Anything past a few thousand is out of range on any buffer the
		// editor can hold, so the two only ever differ on numbers no
		// caller can do anything with.
		const intMax = 1<<31 - 1
		v, neg := 0, s[i] == '-'
		for k := digits; k < j; k++ {
			if v < intMax {
				v = v*10 + int(s[k]-'0')
				if v > intMax {
					v = intMax
				}
			}
		}
		i = j
		if neg {
			v = -v
		}
		return v, true
	}

	v, ok := readInt()
	if !ok {
		return 0, 0, 0
	}
	low = v

	// The literal " - " between the two conversions: a whitespace directive
	// matches nothing as happily as something, but the '-' has to be there.
	// A matching failure here is what stops "/l 3x" at one conversion.
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i >= len(s) || s[i] != '-' {
		return 1, low, 0
	}
	i++
	v, ok = readInt()
	if !ok {
		return 1, low, 0
	}
	return 2, low, v
}

// editorDelete is parse_action's PARSE_DELETE (improved-edit.c:162-213).
//
// Note the order the checks come in: the argument is parsed first, so
// "/d x" on an empty buffer complains about the argument, and the
// higher-than-zero test comes *after* the buffer test, so "/d 0" on an
// empty buffer complains about the buffer.
func editorDelete(buf editText, args string) (editText, string) {
	n, low, high := scanLineRange(args)
	switch n {
	case 0:
		return buf, "You must specify a line number or range to delete.\r\n"
	case 1:
		high = low
	default:
		if high < low {
			return buf, "That range is invalid.\r\n"
		}
	}

	if !buf.present {
		return buf, "Buffer is empty.\r\n"
	}
	if low <= 0 {
		return buf, "Invalid, line numbers to delete must be higher than 0.\r\n"
	}
	start, ok := lineStart(buf.text, low)
	if !ok {
		return buf, "Line(s) out of range; not deleting.\r\n"
	}

	// total_len starts at 1 and counts up with the walk to line_high, then
	// comes back down by one if the buffer ran out before the last line's
	// '\n' was found. That is the whole of the C's counting, and it is what
	// makes "/d 4" on a three-line buffer report zero.
	total := 1
	p, i := start, low
	ranOut := false
	for i < high {
		j := strings.IndexByte(buf.text[p:], '\n')
		if j < 0 {
			ranOut = true
			break
		}
		p += j + 1
		i++
		total++
	}
	end := -1
	if !ranOut {
		if j := strings.IndexByte(buf.text[p:], '\n'); j >= 0 {
			end = p + j + 1
		}
	}
	if end >= 0 {
		buf.text = buf.text[:start] + buf.text[end:]
	} else {
		total--
		buf.text = buf.text[:start]
	}
	// Still present, even when it is now empty: the C truncates in place
	// and never frees, which is the whole point of editText.
	return buf, fmt.Sprintf("%d line%sdeleted.\r\n", total, linePlural(total))
}

// editorList is parse_action's PARSE_LIST_NORM (improved-edit.c:215-275),
// sent directly rather than through the pager: paging mid-edit would need
// StatePaging to remember what state to return to, which it does not —
// session/pager.go's own doc comment names the identical gap for
// `background` — and a buffer within any caller's own length limit rarely
// runs past a screen anyway.
func editorList(buf editText, args string) string {
	low, high, errMsg := listRange(args)
	if errMsg != "" {
		return errMsg
	}

	out := ""
	// The header decision is made from the *requested* range, against the
	// sentinel and not against the buffer: "/l 1-500" on a five-line buffer
	// prints one, because 500 is not 999999.
	if high < maxEditorLine || low > 1 {
		out = fmt.Sprintf("Current buffer range [%d - %d]:\r\n", low, high)
	}
	start, ok := lineStart(buf.text, low)
	if !ok {
		return "Line(s) out of range; no buffer listing.\r\n"
	}

	total, p, i := 0, start, low
	end := len(buf.text)
	for i <= high {
		j := strings.IndexByte(buf.text[p:], '\n')
		if j < 0 {
			end = len(buf.text)
			break
		}
		p += j + 1
		i++
		total++
		end = p
	}
	out += buf.text[start:end]
	// "This is kind of annoying...but some people like it." — the C's own
	// comment on the footer, improved-edit.c:271-273.
	return out + fmt.Sprintf("\r\n%d line%sshown.\r\n", total, linePlural(total))
}

// editorListNumbered is parse_action's PARSE_LIST_NUM
// (improved-edit.c:277-340), which is /l with three differences, all of them
// the C's: the line number goes on a line of its own ("%4d:\r\n", not
// "%4d: "), there is no range header, and there is no "N lines shown."
// footer even though the count is still tallied up.
//
// The C accumulates into its scratch buffer with `sprintf(buf, "%s%4d:\r\n",
// buf, i - 1)` — source and destination the same buffer, which is undefined
// and which modern gcc compiles into something that keeps only the last
// line. reference/tools/editoracle.c is built -O0 for that reason; what it
// then prints, and what this reproduces, is what the archived server's
// compiler and libc did with it.
func editorListNumbered(buf editText, args string) string {
	low, high, errMsg := listRange(args)
	if errMsg != "" {
		return errMsg
	}
	start, ok := lineStart(buf.text, low)
	if !ok {
		return "Line(s) out of range; no buffer listing.\r\n"
	}

	var out strings.Builder
	p, i, from := start, low, start
	for i <= high {
		j := strings.IndexByte(buf.text[p:], '\n')
		if j < 0 {
			// s is NULL and t is not: whatever is left after the last
			// '\n' goes out with no number of its own. On a buffer that
			// ends in one — which is every buffer the editor builds —
			// that is the empty string, and page_string sends nothing
			// at all for it (modify.c:443-446).
			out.WriteString(buf.text[from:])
			return out.String()
		}
		p += j + 1
		i++
		fmt.Fprintf(&out, "%4d:\r\n", i-1)
		out.WriteString(buf.text[from:p])
		from = p
	}
	// Ended on the range rather than the buffer: t has caught up with s, so
	// the C's tail appends nothing.
	return out.String()
}

// listRange is the argument handling /l and /n share
// (improved-edit.c:220-242 and 282-304), including the pair of bounds
// checks. /n's own copy tests them in two `if`s rather than an if/else,
// which changes nothing.
func listRange(args string) (low, high int, errMsg string) {
	n, lo, hi := scanLineRange(args)
	switch n {
	case 0:
		lo, hi = 1, maxEditorLine
	case 1:
		hi = lo
	}
	if lo < 1 {
		return 0, 0, "Line numbers must be greater than 0.\r\n"
	}
	if hi < lo {
		return 0, 0, "That range is invalid.\r\n"
	}
	return lo, hi, ""
}

// editorInsert is parse_action's PARSE_INSERT (improved-edit.c:342-388):
// the typed text becomes a new line *before* the numbered one.
func editorInsert(buf editText, args string, max int) (editText, string) {
	num, text := halfChop(args)
	if num == "" {
		return buf, "You must specify a line number before which to insert text.\r\n"
	}
	low := int(atoiC(num))
	text += "\r\n"

	if !buf.present {
		// Unreachable: improvedEditorExecute's own `if (*(d->str))` guard
		// has already answered "Current buffer empty." Ported because the
		// C carries it too.
		return buf, "Buffer is empty, nowhere to insert.\r\n"
	}
	if low <= 0 {
		return buf, "Line number must be higher than 0.\r\n"
	}
	start, ok := lineStart(buf.text, low)
	if !ok {
		return buf, "Line number out of range; insert aborted.\r\n"
	}

	// The C's own size check, kept exactly: the prefix, the new line, and
	// `strlen(s + 1)` — one character short of the remainder, because it
	// measures from *past* the character the insertion point sits on. Plus
	// 3 for "\r\n\0".
	rest := buf.text[start:]
	restLen := len(rest)
	if restLen > 0 {
		restLen--
	}
	if start+len(text)+restLen+3 > max {
		return buf, "Insert text pushes buffer over maximum size, insert aborted.\r\n"
	}
	buf.text = buf.text[:start] + text + rest
	return buf, "Line inserted.\r\n"
}

// editorEditLine is parse_action's PARSE_EDIT (improved-edit.c:390-467):
// the typed text replaces the numbered line.
//
// Because line N+1 of an N-line buffer exists (see lineStart), "/e 4" on
// three lines appends a fourth rather than refusing — there is simply no
// '\n' after the insertion point for the tail to start from.
func editorEditLine(buf editText, args string, max int) (editText, string) {
	num, text := halfChop(args)
	if num == "" {
		return buf, "You must specify a line number at which to change text.\r\n"
	}
	low := int(atoiC(num))
	text += "\r\n"

	if !buf.present {
		return buf, "Buffer is empty, nothing to change.\r\n"
	}
	if low <= 0 {
		return buf, "Line number must be higher than 0.\r\n"
	}
	start, ok := lineStart(buf.text, low)
	if !ok {
		return buf, "Line number out of range; change aborted.\r\n"
	}

	next := len(buf.text)
	if j := strings.IndexByte(buf.text[start:], '\n'); j >= 0 {
		next = start + j + 1
	}
	updated := buf.text[:start] + text + buf.text[next:]
	// The C measures the finished string against max_str and only then
	// commits it, so an over-long change leaves the buffer alone.
	if len(updated) > max {
		return buf, "Change causes new length to exceed buffer maximum size, aborted.\r\n"
	}
	buf.text = updated
	return buf, "Line changed.\r\n"
}

// editorFormat is parse_action's PARSE_FORMAT (improved-edit.c:123-132).
//
// The option scan is the C's, and it is why "/fi" indents and "/f i" does
// not: it reads at most two characters from `str + 2` and stops at the first
// that is not a letter, so a space ends it before the 'i' is seen.
func editorFormat(buf editText, args string, max int) (editText, string) {
	indent := false
	for j := 0; j < len(args) && j < 2 && isAlphaByte(args[j]); {
		c := args[j]
		j++
		if c == 'i' && !indent {
			indent = true
		}
	}
	buf.text = formatText(buf.text, indent, max)
	if indent {
		return buf, "Text formatted with indent.\r\n"
	}
	return buf, "Text formatted without indent.\r\n"
}

// formatText ports format_text (improved-edit.c:483-571): the word-wrap
// behind /f.
//
// Words are taken between runs of "\n\r\f\t\v ", wrapped at 79 columns,
// capitalised at the start of the text and after every sentence, and
// separated by two spaces after a '.', '!' or '?' rather than one. The
// output always ends "\r\n", and is cut to maxlen if it grew past it.
//
// The one deliberate difference is a read past the end of the buffer. The C
// runs `while (strchr(".!?", *flow))` with no guard on *flow, and
// strchr(s, '\0') finds the terminator and returns non-NULL — so a buffer
// whose last word is not followed by whitespace walks off the end of the
// allocation. Every buffer the editor builds ends "\r\n", so the C never
// reaches it; a Go string simply ends. docs/deviations.md.
func formatText(text string, indent bool, maxlen int) string {
	// "Fix memory overrun" (improved-edit.c:491-494): a max_str larger than
	// the C's own scratch buffer formats nothing at all. No caller in this
	// port passes one — 8192 is both MAX_STRING_LENGTH and the largest
	// tedit field — but the branch is the C's.
	if maxlen > maxStringLength {
		return text
	}

	var out strings.Builder
	lineChars := 0
	if indent {
		out.WriteString("   ")
		lineChars = 3
	}
	capNext, capNextNext := true, false

	isFlowSpace := func(c byte) bool { return strings.IndexByte("\n\r\f\t\v ", c) >= 0 }

	p := 0
	for p < len(text) {
		for p < len(text) && isFlowSpace(text[p]) {
			p++
		}
		if p < len(text) {
			start := p
			p++
			for p < len(text) && strings.IndexByte("\n\r\f\t\v .?!", text[p]) < 0 {
				p++
			}
			if capNextNext {
				capNextNext = false
				capNext = true
			}
			// Move off the sentence delimiter, taking it with the word.
			for p < len(text) && strings.IndexByte(".!?", text[p]) >= 0 {
				capNextNext = true
				p++
			}
			word := text[start:p]

			if lineChars+len(word)+1 > 79 {
				out.WriteString("\r\n")
				lineChars = 0
			}
			if !capNext {
				if lineChars > 0 {
					out.WriteString(" ")
					lineChars++
				}
			} else {
				capNext = false
				word = upperFirst(word)
			}
			lineChars += len(word)
			out.WriteString(word)
		}

		if capNextNext {
			if lineChars+3 > 79 {
				out.WriteString("\r\n")
				lineChars = 0
			} else {
				out.WriteString("  ")
				lineChars += 2
			}
		}
	}
	formatted := out.String() + "\r\n"

	if maxlen > 0 && len(formatted)+1 > maxlen {
		formatted = formatted[:maxlen-1]
	}
	return formatted
}

// maxStringLength is the C's MAX_STRING_LENGTH (structs.h:530), which
// format_text refuses to format past.
const maxStringLength = 8192

// upperFirst is the C's `*start = UPPER(*start)` — one byte, ASCII only,
// and applied to whatever character the word starts with, punctuation
// included.
func upperFirst(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-('a'-'A')) + s[1:]
}

// isAlphaByte is the C locale's isalpha, which /f and /r scan their options
// with.
func isAlphaByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// editorReplace is parse_action's PARSE_REPLACE (improved-edit.c:133-161).
//
// The argument is picked apart by four successive strtok(..., "'") calls, so
// the pattern is whatever sits between the first pair of apostrophes and the
// replacement is whatever sits between the second — and, because strtok
// never returns an empty token, `/r ” 'x'` is not a way to say "the empty
// pattern"; it reads as a missing quote instead.
//
// Two things here are worth not tidying:
//
// The space check `(strlen(t) - strlen(s)) + strlen(*d->str)` is unsigned,
// so a pattern longer than the whole buffer wraps it to something near
// UINT_MAX and the answer is "Not enough space left in buffer." rather than
// "String ... not found." docs/weirdnumbers.md.
//
// And the C dereferences the buffer here without checking it exists — /r is
// the one command improved_editor_execute does not guard — so `/r 'a' 'b'`
// as the first thing typed into a fresh editor is a NULL dereference. This
// treats an absent buffer as an empty one, which is the answer the same code
// gives one line later. docs/deviations.md.
func editorReplace(buf editText, args string, max int) (editText, string) {
	repAll := false
	for j := 0; j < len(args) && j < 2 && isAlphaByte(args[j]); {
		c := args[j]
		j++
		if c == 'a' {
			repAll = true
		}
	}

	fields := strtokFields(args, '\'')
	switch len(fields) {
	case 0:
		return buf, "Invalid format.\r\n"
	case 1:
		return buf, "Target string must be enclosed in single quotes.\r\n"
	case 2:
		return buf, "No replacement string.\r\n"
	case 3:
		return buf, "Replacement string must be enclosed in single quotes.\r\n"
	}
	pattern, replacement := fields[1], fields[3]

	// unsigned int, deliberately: see the doc comment. The wraparound is
	// the behaviour, so the conversions are not a mistake to be checked
	// for.
	totalLen := u32(len(replacement)) - u32(len(pattern)) + u32(len(buf.text))
	if max < 0 || totalLen > u32(max) {
		return buf, "Not enough space left in buffer.\r\n"
	}

	text, replaced := replaceStr(buf.text, pattern, replacement, repAll, max)
	buf.text = text
	buf.present = true
	switch {
	case replaced > 0:
		return buf, fmt.Sprintf("Replaced %d occurance%sof '%s' with '%s'.\r\n",
			replaced, linePlural(replaced), pattern, replacement)
	case replaced == 0:
		return buf, fmt.Sprintf("String '%s' not found.\r\n", pattern)
	}
	// Unreachable, and the C's own arithmetic is why: replace_str's guard
	// is the same expression as the one above, so anything that would
	// return -1 has already been answered "Not enough space left in
	// buffer."
	return buf, "ERROR: Replacement string causes buffer overflow, aborted replace.\r\n"
}

// replaceStr ports replace_str (improved-edit.c:573-620): the substitution
// itself, and the count PARSE_REPLACE reports.
//
// The /ra path has a second size check inside its loop, and what it does
// when it trips is the surprise. It sets the count to -1 and breaks, and the
// tail then reads that as `i <= 0` and returns 0 — so the player is told
// "String ... not found." about a string that was found four times. Worse,
// the loop measures each segment by writing a '\0' into the *caller's*
// buffer and restoring it afterwards, and the break happens in between: the
// buffer is left truncated at the match that overflowed. Both are
// reproduced. docs/weirdnumbers.md.
func replaceStr(text, pattern, replacement string, repAll bool, maxSize int) (string, int) {
	if u32(len(text))-u32(len(pattern))+u32(len(replacement)) > u32(maxSize) {
		return text, -1
	}

	var out strings.Builder
	found := 0
	if repAll {
		flow, jetsam := 0, 0
		for {
			k := strings.Index(text[flow:], pattern)
			if k < 0 {
				break
			}
			flow += k
			found++
			if u32(out.Len())+u32(flow-jetsam)+u32(len(replacement)) > u32(maxSize) {
				// i = -1, break — and `*flow = '\0'` never put back.
				return text[:flow], 0
			}
			out.WriteString(text[jetsam:flow])
			out.WriteString(replacement)
			flow += len(pattern)
			jetsam = flow
		}
		out.WriteString(text[jetsam:])
	} else if k := strings.Index(text, pattern); k >= 0 {
		found++
		out.WriteString(text[:k])
		out.WriteString(replacement)
		out.WriteString(text[k+len(pattern):])
	}

	if found <= 0 {
		return text, 0
	}
	return out.String(), found
}

// u32 is the narrowing every size comparison in /r goes through, because
// the C's own are `unsigned int` and `size_t` and the answers depend on the
// wraparound. Anything genuinely negative comes out near UINT_MAX, which is
// exactly what makes an over-long pattern read as a full buffer; see
// editorReplace.
func u32(n int) uint32 {
	return uint32(n) //nolint:gosec // the wraparound is the ported behaviour
}

// strtokFields splits on a delimiter the way a run of strtok(s, delim)
// calls does: leading and repeated delimiters are skipped rather than
// producing empty fields, and a trailing one produces nothing.
func strtokFields(s string, delim byte) []string {
	var out []string
	for i := 0; i < len(s); {
		for i < len(s) && s[i] == delim {
			i++
		}
		if i >= len(s) {
			break
		}
		j := i
		for j < len(s) && s[j] != delim {
			j++
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

// finishEditing ends the line editor, porting string_add's own tail
// (modify.c:159-221). saved is false only for /a: the C frees *d->str and
// restores whatever was there before (modify.c:170-172), which this port
// has never captured a "before" of — every caller already treats an empty
// result as "nothing changed" (tedit's file, mail's send, a board post),
// so handing back "" is the observable-equivalent outcome.
//
// The cleanup runs on the **world goroutine**, which is where the C runs
// it: string_add is called from the game loop like everything else. Here
// it is reached from the session's own goroutine — the editor is the one
// thing a playing character drives outside Dispatcher.Do — and what it
// touches is world state. The PLR_WRITING bit is read by `tell`, by the
// room listing and by mudlog's own echo, all of them on the world
// goroutine; the callbacks write a board's message list and a note's
// action description, which are world objects. Clearing or writing those
// from here without the hop is a data race, and -race finds it.
//
// The hop is CommandHandler.InWorld, which waits, so nothing the player
// types next can overtake the cleanup. A callback must therefore not call
// InWorld itself — none does, and none should; it is already inside the
// world goroutine by the time it runs.
//
// setState comes *before* the hop on purpose: the connection is out of
// the editor the moment the terminator is typed, so the next line is a
// command. Its own DoSync queues behind this task either way.
func (s *Session) finishEditing(ctx context.Context, deps Deps, saved bool) error {
	var text string
	if saved {
		text = s.editorBuf.text
		if s.editorMax > 0 && len(text) > s.editorMax {
			text = text[:s.editorMax]
			s.Send("Your message was truncated to %d characters.\r\n", s.editorMax)
		}
	}

	done := s.editorDone
	s.editorBuf, s.editorDone, s.editorMax = editText{}, nil, 0
	s.setState(StatePlaying)

	cleanup := func(*game.Live) {
		// `REMOVE_BIT(PLR_FLAGS(d->character), PLR_MAILING | PLR_WRITING)`
		// (modify.c:218-219), on both the save and the abort path, and
		// guarded by the same `!IS_NPC` the set was.
		if c := s.Character(); c != nil && !c.IsNPC() && c.Record != nil {
			rec := c.Record
			rec.PlayerFlags = rec.PlayerFlags.Clear(game.PlayerMailing | game.PlayerWriting)
		}
		if done != nil {
			done(text, saved)
		}
	}
	// Two ways to end up running it inline, and both are safe for the same
	// reason — there is no world goroutine to race with.
	//
	// A nil Commands is a unit test holding a Session directly. A failed
	// hop is the shutdown case: Engine.DoSync only ever fails on a
	// cancelled context, and by then Run has drained its queue and
	// returned, so a task submitted now would never be run at all. Doing
	// it here instead is what stops a letter typed at the moment the
	// server goes down from being silently dropped — the C, being
	// single-threaded, never had anywhere to lose it.
	if deps.Commands == nil {
		cleanup(nil)
	} else if err := deps.Commands.InWorld(ctx, cleanup); err != nil {
		cleanup(nil)
	}

	s.Send("%s", prompt(s))
	return nil
}
