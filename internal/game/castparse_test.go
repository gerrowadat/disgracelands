// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// do_cast's argument parse against the C, because strtok is not what a
// reader expects and the reading is what went wrong.
//
// `ParseCastArgument` found the first quote and then the second. strtok
// skips a *run* of delimiters, and only the quote is one — a space is not —
// which between them decide four answers that parser got wrong (#358), one
// of which is why the empty-spell-name behaviour could not be fixed alone
// (#365).

// TestParseCastArgumentAgainstC compares this port's parse to the C's over
// every shape of quoting a player can type.
func TestParseCastArgumentAgainstC(t *testing.T) {
	gcc, err := exec.LookPath("gcc")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("gcc not found in CI; the cast-parse comparison must run")
		}
		t.Skip("gcc not found; skipping the cast-parse comparison")
	}

	src := filepath.Join(repoRoot(t), "reference", "tools", "castparse.c")
	if _, statErr := os.Stat(src); statErr != nil {
		t.Fatalf("%s not found: %v", src, statErr)
	}
	bin := filepath.Join(t.TempDir(), "castparse")
	if out, buildErr := exec.Command(gcc, "-std=gnu89", "-O2", "-Wall", "-o", bin, src).CombinedOutput(); buildErr != nil {
		t.Fatalf("compiling the oracle: %v\n%s", buildErr, out)
	}

	// What a player types, in full, because the oracle removes the command
	// word itself the way the interpreter does.
	typed := []string{
		"cast 'magic missile'",
		"cast 'magic missile' orc",
		"cast 'mag mis' fido",
		"cast 'cure light'  welmar",
		"cast 'armor'",
		"cast magic missile", // no quotes at all
		"cast",               // nothing

		// The four the old parser got wrong.
		"cast ''",
		"cast '''",
		"cast '  '",
		"cast ' '",
		"cast '' fido",
		"cast 'magic missile", // no closing quote
		"cast 'mag mis fido",

		// And some shapes nobody would type on purpose.
		"cast ''''",
		"cast 'a'b'c'",
		"cast 'a' 'b'",
		"cast '''x'",
	}

	var in strings.Builder
	for _, line := range typed {
		in.WriteString(line)
		in.WriteString("\n")
	}
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in.String())
	out, runErr := cmd.Output()
	if runErr != nil {
		t.Fatalf("running the oracle: %v", runErr)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != len(typed) {
		t.Fatalf("the oracle answered %d lines, asked %d", len(lines), len(typed))
	}

	for i, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			t.Fatalf("%q: unparseable oracle line %q", typed[i], line)
		}
		wantVerdict, wantSpell, wantTarget := fields[1], fields[2], fields[3]

		// The port is handed the argument with the command word removed and
		// trimmed, which is what session.split produces.
		arg := strings.TrimSpace(strings.TrimPrefix(typed[i], "cast"))
		spell, target, errMsg := ParseCastArgument(arg)

		gotVerdict := "ok"
		switch {
		case strings.HasPrefix(errMsg, "Cast what where?"):
			gotVerdict = "what-where"
		case strings.HasPrefix(errMsg, "Spell names must be enclosed"):
			gotVerdict = "unenclosed"
		}

		if gotVerdict != wantVerdict {
			t.Errorf("%q: verdict %q, the C says %q", typed[i], gotVerdict, wantVerdict)
			continue
		}
		if wantVerdict != "ok" {
			continue
		}
		if spell != wantSpell {
			t.Errorf("%q: spell %q, the C says %q", typed[i], spell, wantSpell)
		}
		// The target is compared trimmed: the C trims it a few lines later
		// with one_argument and skip_spaces (spell_parser.c), and this port
		// does it here, so the two agree by the time anything reads it.
		if target != strings.TrimSpace(wantTarget) {
			t.Errorf("%q: target %q, the C says %q", typed[i], target, strings.TrimSpace(wantTarget))
		}
	}
}
