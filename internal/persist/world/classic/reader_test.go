package classic

import (
	"strings"
	"testing"
)

func TestGetLineSkipsCommentsAndBlanks(t *testing.T) {
	r := newReader(strings.NewReader("#1\n\n* a comment\n\nThe Void~\n"), "t")

	for _, want := range []string{"#1", "The Void~"} {
		got, ok := r.getLine()
		if !ok {
			t.Fatalf("getLine() ended early, wanted %q", want)
		}
		if got != want {
			t.Errorf("getLine() = %q, want %q", got, want)
		}
	}
	if _, ok := r.getLine(); ok {
		t.Error("getLine() returned a line past end of file")
	}
}

func TestGetLineTracksLineNumbersThroughSkippedLines(t *testing.T) {
	// Error messages point at file:line, so the count has to include the
	// comment and blank lines get_line silently consumes.
	r := newReader(strings.NewReader("* one\n\n* three\nfour\n"), "t")
	if _, ok := r.getLine(); !ok {
		t.Fatal("getLine() ended early")
	}
	if r.lineNo != 4 {
		t.Errorf("lineNo = %d after reading the 4th line, want 4", r.lineNo)
	}
}

func TestReadString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			// A tilde on the same line as the text yields no trailing CRLF.
			name:  "terminator on the text line",
			input: "The Void~\n",
			want:  "The Void",
		},
		{
			// A tilde on its own line leaves the preceding line's CRLF in
			// place. This asymmetry is player-visible and must be preserved.
			name:  "terminator on its own line",
			input: "Line one.\n~\n",
			want:  "Line one.\r\n",
		},
		{
			name:  "multiple lines",
			input: "First.\nSecond.\n~\n",
			want:  "First.\r\nSecond.\r\n",
		},
		{
			// fread_string truncates at the first tilde and discards the rest
			// of that line.
			name:  "text after the terminator is dropped",
			input: "Keep this~drop this\n",
			want:  "Keep this",
		},
		{
			name:  "empty string",
			input: "~\n",
			want:  "",
		},
		{
			// Unlike get_line, fread_string does not skip comment lines, so a
			// '*' inside a description is content.
			name:  "asterisk lines are content",
			input: "*not a comment\n~\n",
			want:  "*not a comment\r\n",
		},
		{
			// A blank line inside a description is a real blank line.
			name:  "blank lines are preserved",
			input: "One.\n\nTwo.\n~\n",
			want:  "One.\r\n\r\nTwo.\r\n",
		},
		{
			// '#' has no special meaning inside a string; the world contains
			// ASCII-art signs that rely on this.
			name:  "hash lines are content",
			input: "####\n# hi #\n####\n~\n",
			want:  "####\r\n# hi #\r\n####\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReader(strings.NewReader(tt.input), "t")
			got, err := r.readString("test")
			if err != nil {
				t.Fatalf("readString() = error %v", err)
			}
			if got != tt.want {
				t.Errorf("readString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadStringUnterminatedIsAnError(t *testing.T) {
	// The C code exits the process here. An error that names the file and
	// line is more useful and equally impossible to ignore.
	r := newReader(strings.NewReader("no terminator ever comes\n"), "world.wld")
	_, err := r.readString("a description")
	if err == nil {
		t.Fatal("readString() on an unterminated string succeeded, want an error")
	}
	for _, want := range []string{"world.wld", "~"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestUnreadLine(t *testing.T) {
	// Object records end only when the next record's '#' line is read, so the
	// parser has to be able to hand that line back.
	r := newReader(strings.NewReader("#1\n#2\n"), "t")
	first, _ := r.getLine()
	r.unreadLine(first)

	again, ok := r.getLine()
	if !ok || again != first {
		t.Fatalf("getLine() after unreadLine = %q, %v; want %q, true", again, ok, first)
	}
	next, _ := r.getLine()
	if next != "#2" {
		t.Errorf("getLine() = %q, want the following line %q", next, "#2")
	}
}
