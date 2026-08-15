package world_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

func TestTextEscapesBytesExactly(t *testing.T) {
	tests := map[string]string{
		"plain":   `"plain"`,
		"a\"b":    `"a\"b"`,
		`a\b`:     `"a\\b"`,
		"a\nb":    `"a\nb"`,
		"a\rb":    `"a\rb"`,
		"a\tb":    `"a\tb"`,
		"\x00":    `"\u0000"`,
		"\x1f":    `"\u001f"`,
		"It\x92s": `"It\u0092s"`,
		"\xff":    `"\u00ff"`,
		"":        `""`,
	}
	for in, want := range tests {
		got, err := json.Marshal(world.Text(in))
		if err != nil {
			t.Errorf("Marshal(%q): %v", in, err)
			continue
		}
		if string(got) != want {
			t.Errorf("Marshal(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestTextPreservesInvalidUTF8(t *testing.T) {
	// This is the reason Text exists. encoding/json replaces every invalid
	// UTF-8 byte with U+FFFD, so two different corrupt bytes would dump
	// identically — and a parity diff would show no difference where a real
	// one exists. lib/world/wld/90.wld contains 0x92.
	const in = "It\x92s possible"

	standard, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(standard), "\ufffd") {
		t.Skip("encoding/json no longer replaces invalid UTF-8; Text may be unnecessary")
	}

	exact, err := json.Marshal(world.Text(in))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(exact), "\ufffd") {
		t.Errorf("Text lost the invalid byte: %s", exact)
	}
	if !strings.Contains(string(exact), `\u0092`) {
		t.Errorf("Text = %s, want it to carry \\u0092", exact)
	}
}

func TestTextDistinguishesDifferentInvalidBytes(t *testing.T) {
	// Two corrupt bytes must not collapse to the same output, or a diff
	// between two loaders could silently pass.
	a, _ := json.Marshal(world.Text("\x92"))
	b, _ := json.Marshal(world.Text("\x93"))
	if string(a) == string(b) {
		t.Errorf("0x92 and 0x93 both dumped as %s", a)
	}
}

func TestTextIsPureASCII(t *testing.T) {
	// The C dumper produces ASCII and nothing else; if the Go side emitted a
	// multi-byte sequence anywhere the two files would differ on encoding
	// rather than on content.
	out, err := json.Marshal(world.Text("caf\xe9 \x92 \xff"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(out); i++ {
		if out[i] > 0x7e {
			t.Fatalf("non-ASCII byte %#x at %d in %s", out[i], i, out)
		}
	}
}

func TestTextRoundTrips(t *testing.T) {
	for _, in := range []string{"plain", "It\x92s", "\x00\x01\xff", "quote\"and\\slash", "line\r\nbreak"} {
		data, err := json.Marshal(world.Text(in))
		if err != nil {
			t.Fatalf("Marshal(%q): %v", in, err)
		}
		var back world.Text
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatalf("Unmarshal(%s): %v", data, err)
		}
		if string(back) != in {
			t.Errorf("round trip of %q gave %q", in, string(back))
		}
	}
}
