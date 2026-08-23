// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The improved editor's eleven commands against the C, because reading them
// across is not enough. reference/tools/editoracle.c holds the original
// bodies of improved_editor_execute, parse_action, format_text and
// replace_str; this drives every case it emits through
// improvedEditorExecute and compares the resulting buffer *and* the text
// sent, character for character.
//
// Five findings this caught that a reading did not, each now asserted on its
// own below so nobody tidies it away:
//
//   - a three-line buffer has a line 4 (lineStart's doc comment), so "/d 4"
//     answers "0 lines deleted." and "/e 4"/"/i 4" append;
//   - a buffer emptied by /d is not the same as one cleared by /c;
//   - /n puts its line number on a line of its own and prints no footer;
//   - a /r pattern longer than the buffer reports "Not enough space left in
//     buffer.", the unsigned subtraction having wrapped;
//   - a /ra that runs out of room leaves the buffer truncated at the match
//     and says the string was not found.

// buildEditOracle compiles reference/tools/editoracle.c.
//
// -O0 is not laziness. PARSE_LIST_NUM accumulates with `sprintf(buf,
// "%s%4d:\r\n", buf, i - 1)` — the destination is also the `%s` argument,
// which is undefined behaviour. gcc resolves it at -O2 into something that
// keeps only the *last* line, and at -O0 into a call to glibc's sprintf,
// where the `%s` copy is a self-copy at offset zero and the accumulation
// works. The second is what the archived server's compiler did, and the
// version that makes /n a useful command rather than a broken one; -O2's
// answer is an artifact of a compiler twenty years newer than the code.
func buildEditOracle(t *testing.T) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("gcc not found in CI; the editor comparison must run")
		}
		t.Skip("gcc not found; skipping the editor comparison")
	}

	src := filepath.Join(repoRoot(t), "reference", "tools", "editoracle.c")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("%s not found: %v", src, err)
	}

	bin := filepath.Join(t.TempDir(), "editoracle")
	build := exec.Command(gcc, "-O0", "-Wall", "-Werror",
		"-Wno-restrict", "-Wno-format-overflow", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compiling the oracle: %v\n%s", err, out)
	}
	return bin
}

// editorCase is one row of the oracle's output.
type editorCase struct {
	in     editText
	max    int
	line   string
	result editorResult
	out    editText
	sent   string
}

// unescapeOracle reverses the oracle's own escaping: \\, \r, \n and \t.
func unescapeOracle(t *testing.T, s string) string {
	t.Helper()
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case '\\':
			b.WriteByte('\\')
		case 'r':
			b.WriteByte('\r')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		default:
			t.Fatalf("unknown escape \\%c in oracle output", s[i])
		}
	}
	return b.String()
}

func editOracleCases(t *testing.T) []editorCase {
	t.Helper()

	bin := buildEditOracle(t)
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("running the oracle: %v", err)
	}

	field := func(s string) editText {
		if s == "NULL" {
			return editText{}
		}
		return editText{text: unescapeOracle(t, s), present: true}
	}

	var cases []editorCase
	for _, row := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		cols := strings.Split(row, "\t")
		if len(cols) != 6 {
			t.Fatalf("oracle row has %d fields, want 6: %q", len(cols), row)
		}
		max, err := strconv.Atoi(cols[1])
		if err != nil {
			t.Fatalf("oracle gave a non-numeric max_str %q", cols[1])
		}
		result, err := strconv.Atoi(cols[3])
		if err != nil {
			t.Fatalf("oracle gave a non-numeric return %q", cols[3])
		}
		cases = append(cases, editorCase{
			in:     field(cols[0]),
			max:    max,
			line:   unescapeOracle(t, cols[2]),
			result: editorResult(result),
			out:    field(cols[4]),
			sent:   unescapeOracle(t, cols[5]),
		})
	}
	if len(cases) == 0 {
		t.Fatal("the oracle produced no rows")
	}
	return cases
}

// TestImprovedEditorAgainstTheC is the whole comparison: every buffer the
// oracle holds crossed with every command line it types.
func TestImprovedEditorAgainstTheC(t *testing.T) {
	cases := editOracleCases(t)

	for _, tc := range cases {
		result, got, sent := improvedEditorExecute(tc.in, tc.max, tc.line)

		if result != tc.result {
			t.Errorf("%q on %q (max %d): result %d, the C says %d",
				tc.line, tc.in.text, tc.max, result, tc.result)
		}
		if sent != tc.sent {
			t.Errorf("%q on %q (max %d): sent\n  %q\nthe C sends\n  %q",
				tc.line, tc.in.text, tc.max, sent, tc.sent)
		}
		// /a and /s return before touching the buffer, and this port's
		// own abort path (finishEditing) is what discards it — so only
		// compare the buffer for the commands that edit one.
		if result == editorAbort || result == editorSave {
			continue
		}
		if got.text != tc.out.text || got.present != tc.out.present {
			t.Errorf("%q on %q (max %d): buffer %q (present %v), the C leaves %q (present %v)",
				tc.line, tc.in.text, tc.max, got.text, got.present, tc.out.text, tc.out.present)
		}
	}

	t.Logf("checked %d editor commands against the C", len(cases))
}

// The buffer these use is the oracle's own three-line one.
const threeLines = "First line.\r\nSecond line.\r\nThird line.\r\n"

func runEditorLine(t *testing.T, buf editText, line string) (editText, string) {
	t.Helper()
	_, got, sent := improvedEditorExecute(buf, maxStringLength, line)
	return got, sent
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestEditorLineAfterTheLastOneExists is the off-by-one that reads like a
// bug and is the C's: the walk to line N stops on the terminator after the
// buffer's final '\n', which is a valid empty line. Three-line buffers have
// a fourth line; the fifth is what is out of range.
func TestEditorLineAfterTheLastOneExists(t *testing.T) {
	full := editText{text: threeLines, present: true}

	for _, tc := range []struct {
		line string
		want string
		buf  string
	}{
		{"/d 4", "0 lines deleted.\r\n", threeLines},
		{"/d 5", "Line(s) out of range; not deleting.\r\n", threeLines},
		{"/e 4 fourth", "Line changed.\r\n", threeLines + "fourth\r\n"},
		{"/e 5 fifth", "Line number out of range; change aborted.\r\n", threeLines},
		{"/i 4 fourth", "Line inserted.\r\n", threeLines + "fourth\r\n"},
		{"/i 5 fifth", "Line number out of range; insert aborted.\r\n", threeLines},
	} {
		got, sent := runEditorLine(t, full, tc.line)
		if sent != tc.want {
			t.Errorf("%q sent %q, want %q", tc.line, sent, tc.want)
		}
		if got.text != tc.buf {
			t.Errorf("%q left %q, want %q", tc.line, got.text, tc.buf)
		}
	}
}

// TestEditorEmptyIsNotAbsent: /c frees the buffer and /d only empties it,
// and the four commands guarded by `if (*(d->str))` can tell the difference.
func TestEditorEmptyIsNotAbsent(t *testing.T) {
	full := editText{text: threeLines, present: true}

	emptied, sent := runEditorLine(t, full, "/d 1-3")
	if sent != "3 lines deleted.\r\n" {
		t.Fatalf("/d 1-3 sent %q", sent)
	}
	if emptied.text != "" || !emptied.present {
		t.Fatalf("/d 1-3 left %q (present %v), want an empty but live buffer", emptied.text, emptied.present)
	}

	cleared, sent := runEditorLine(t, full, "/c")
	if sent != "Current buffer cleared.\r\n" {
		t.Fatalf("/c sent %q", sent)
	}
	if cleared.present {
		t.Fatal("/c left a live buffer; the C frees it")
	}

	if _, sent := runEditorLine(t, emptied, "/l"); sent != "\r\n0 lines shown.\r\n" {
		t.Errorf("/l on an emptied buffer sent %q, want a blank line and a count", sent)
	}
	if _, sent := runEditorLine(t, cleared, "/l"); sent != "Current buffer empty.\r\n" {
		t.Errorf("/l on a cleared buffer sent %q", sent)
	}
}

// TestEditorNumberedListing: /n's number goes on its own line, and there is
// no "N lines shown." footer even though /l has one.
func TestEditorNumberedListing(t *testing.T) {
	full := editText{text: threeLines, present: true}

	_, sent := runEditorLine(t, full, "/n")
	want := "   1:\r\nFirst line.\r\n   2:\r\nSecond line.\r\n   3:\r\nThird line.\r\n"
	if sent != want {
		t.Errorf("/n sent\n  %q\nwant\n  %q", sent, want)
	}
	if strings.Contains(sent, "shown") {
		t.Error("/n printed a lines-shown footer; only /l has one")
	}
	if _, sent := runEditorLine(t, full, "/n 4"); sent != "" {
		t.Errorf("/n 4 sent %q, want nothing at all", sent)
	}
}

// TestEditorReplaceUnsignedSpaceCheck: a pattern longer than the buffer
// makes `(strlen(t) - strlen(s)) + strlen(*d->str)` wrap, and the player is
// told the buffer is full rather than that the string was not found.
func TestEditorReplaceUnsignedSpaceCheck(t *testing.T) {
	short := editText{text: "hi\r\n", present: true}

	if _, sent := runEditorLine(t, short, "/r 'a-much-longer-pattern' 'x'"); sent != "Not enough space left in buffer.\r\n" {
		t.Errorf("/r with an over-long pattern sent %q", sent)
	}
	// One character shorter than the buffer, so the subtraction stays
	// positive and the ordinary answer comes back.
	if _, sent := runEditorLine(t, short, "/r 'abc' 'x'"); sent != "String 'abc' not found.\r\n" {
		t.Errorf("/r with a short pattern sent %q", sent)
	}
}

// TestEditorReplaceAllOverflowTruncates is the worst of them: /ra that runs
// out of room reports the string as not found *and* leaves the buffer cut
// off at the match it gave up on, because the '\0' it wrote to measure the
// segment is never put back.
func TestEditorReplaceAllOverflowTruncates(t *testing.T) {
	const text = "repeat repeat repeat\r\nrepeat again\r\n"
	buf := editText{text: text, present: true}

	_, got, sent := improvedEditorExecute(buf, 39, "/ra 'e' 'EEEE'")
	if sent != "String 'e' not found.\r\n" {
		t.Errorf("overflowing /ra sent %q, want the not-found message", sent)
	}
	if got.text != "repeat repeat repeat\r\nr" {
		t.Errorf("overflowing /ra left %q, want the buffer truncated at the match", got.text)
	}
}

// TestEditorFormatOptionScan: "/fi" indents and "/f i" does not, because
// the scan stops at the first character that is not a letter.
func TestEditorFormatOptionScan(t *testing.T) {
	buf := editText{text: "one two three\r\n", present: true}

	for _, tc := range []struct {
		line   string
		indent bool
	}{
		{"/f", false},
		{"/fi", true},
		{"/fii", true},
		{"/f i", false},
		{"/fx", false},
		{"/f 1", false},
	} {
		got, sent := runEditorLine(t, buf, tc.line)
		wantMsg := "Text formatted without indent.\r\n"
		if tc.indent {
			wantMsg = "Text formatted with indent.\r\n"
		}
		if sent != wantMsg {
			t.Errorf("%q sent %q, want %q", tc.line, sent, wantMsg)
		}
		if indented := strings.HasPrefix(got.text, "   "); indented != tc.indent {
			t.Errorf("%q produced %q; indented = %v, want %v", tc.line, got.text, indented, tc.indent)
		}
	}
}

// TestScanLineRange, porting `sscanf(string, " %d - %d ", &line_low,
// &line_high)` (improved-edit.c:222). The count matters as much as the
// values: /d's own switch turns 0 into an error where /l's turns it into
// "the whole buffer".
func TestScanLineRange(t *testing.T) {
	for _, tc := range []struct {
		args           string
		n, low, high   int
		why            string
		checkHighOnTwo bool
	}{
		// An empty or all-whitespace argument is the C's EOF, which this
		// deliberately reads as zero conversions: docs/deviations.md.
		{args: "", n: 0, why: "no argument at all"},
		{args: "   ", n: 0, why: "whitespace only"},
		{args: "abc", n: 0, why: "no digits"},
		{args: " - 5", n: 0, why: "a sign with no digits behind it"},
		{args: "3", n: 1, low: 3, why: "one number"},
		{args: " 3 ", n: 1, low: 3, why: "leading and trailing space"},
		{args: "0", n: 1, low: 0, why: "zero is a conversion"},
		{args: "-5", n: 1, low: -5, why: "%d takes the sign"},
		{args: "3x", n: 1, low: 3, why: "the '-' fails to match, one conversion stands"},
		{args: "3-", n: 1, low: 3, why: "a dash with nothing behind it"},
		{args: "3-abc", n: 1, low: 3, why: "the second %d fails"},
		{args: "3-5", n: 2, low: 3, high: 5, checkHighOnTwo: true},
		{args: "3 - 5", n: 2, low: 3, high: 5, checkHighOnTwo: true},
	} {
		n, low, high := scanLineRange(tc.args)
		if n != tc.n {
			t.Errorf("scanLineRange(%q) matched %d, want %d — %s", tc.args, n, tc.n, tc.why)
			continue
		}
		if n >= 1 && low != tc.low {
			t.Errorf("scanLineRange(%q) low = %d, want %d", tc.args, low, tc.low)
		}
		if tc.checkHighOnTwo && high != tc.high {
			t.Errorf("scanLineRange(%q) high = %d, want %d", tc.args, high, tc.high)
		}
	}
}
