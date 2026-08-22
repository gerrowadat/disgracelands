// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"strings"
	"testing"
)

func TestParseHelpFileMechanics(t *testing.T) {
	src := "FOO BAR\nFirst line.\nSecond line.\n#\nBAZ\nOne entry, one keyword.\n#\n$\n"
	entries, err := ParseHelpFile(strings.NewReader(src))
	if err != nil {
		t.Fatalf("ParseHelpFile: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}

	if got, want := entries[0].Keywords, []string{"FOO", "BAR"}; !equalStrings(got, want) {
		t.Errorf("entries[0].Keywords = %v, want %v", got, want)
	}
	// The keyword line is the entry's own first line — load_help
	// (db.c:1710) copies it in before appending anything else.
	if want := "FOO BAR\r\nFirst line.\r\nSecond line.\r\n"; entries[0].Body != want {
		t.Errorf("entries[0].Body = %q, want %q", entries[0].Body, want)
	}

	if got, want := entries[1].Keywords, []string{"BAZ"}; !equalStrings(got, want) {
		t.Errorf("entries[1].Keywords = %v, want %v", got, want)
	}
	if want := "BAZ\r\nOne entry, one keyword.\r\n"; entries[1].Body != want {
		t.Errorf("entries[1].Body = %q, want %q", entries[1].Body, want)
	}
}

func TestParseHelpFileTerminatesOnBareDollarFirstChar(t *testing.T) {
	// The C checks only the first character (`while (*key != '$')`), not
	// the whole line — a line that is just "$" with trailing junk would
	// still terminate. Not something real data does, but the algorithm is
	// what it is.
	src := "FOO\nbody\n#\n$ignored trailing text\n"
	entries, err := ParseHelpFile(strings.NewReader(src))
	if err != nil {
		t.Fatalf("ParseHelpFile: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
}

func TestParseHelpFileUnterminatedIsAnError(t *testing.T) {
	if _, err := ParseHelpFile(strings.NewReader("FOO\nbody\n#\nBAR\nbody with no terminator\n")); err == nil {
		t.Error("ParseHelpFile with no $ succeeded, want an error")
	}
	if _, err := ParseHelpFile(strings.NewReader("FOO\nbody with no #\n")); err == nil {
		t.Error("ParseHelpFile with no # succeeded, want an error")
	}
}

func TestParseHelpIndex(t *testing.T) {
	files, err := ParseHelpIndex(strings.NewReader("commands.hlp\ninfo.hlp\n$\n"))
	if err != nil {
		t.Fatalf("ParseHelpIndex: %v", err)
	}
	if got, want := files, []string{"commands.hlp", "info.hlp"}; !equalStrings(got, want) {
		t.Errorf("ParseHelpIndex = %v, want %v", got, want)
	}
}

func TestParseHelpIndexUnterminatedIsAnError(t *testing.T) {
	if _, err := ParseHelpIndex(strings.NewReader("commands.hlp\n")); err == nil {
		t.Error("ParseHelpIndex with no $ succeeded, want an error")
	}
}

func TestHelpIndexLookup(t *testing.T) {
	idx := NewHelpIndex([]HelpEntry{
		{Keywords: []string{"ALIAS", "ALIASES"}, Body: "alias body"},
		{Keywords: []string{"BATTLEAXE"}, Body: "battleaxe body"},
		{Keywords: []string{"BATTLECRY"}, Body: "battlecry body"},
	})

	if _, ok := idx.Lookup(""); ok {
		t.Error("Lookup(\"\") matched something, want no match (do_help's own no-argument branch never reaches this)")
	}
	if e, ok := idx.Lookup("nonsense"); ok {
		t.Errorf("Lookup(nonsense) = %+v, want no match", e)
	}

	e, ok := idx.Lookup("alias")
	if !ok || e.Body != "alias body" {
		t.Errorf("Lookup(alias) = %+v, %v", e, ok)
	}
	e, ok = idx.Lookup("ALIASES")
	if !ok || e.Body != "alias body" {
		t.Errorf("Lookup(ALIASES) (case-insensitive) = %+v, %v", e, ok)
	}

	// "battle" is a prefix of both BATTLEAXE and BATTLECRY. The C's
	// backward-walk lands on whichever sorts first — BATTLEAXE, since
	// 'A' < 'C' — not "the one the player probably meant". Reproduced
	// deliberately.
	e, ok = idx.Lookup("battle")
	if !ok || e.Body != "battleaxe body" {
		t.Errorf("Lookup(battle) = %+v, %v, want the alphabetically-first match (battleaxe body)", e, ok)
	}
}

// ParseHelpFile against the real archive: exact entry counts (from '#'
// delimiters) and one entry's exact text, including
// data/text/help/info.hlp's CIRCLE CIRCLEMUD CREDITS block — the entry
// go-port-plan.md §12 requires `help circlemud` to reach for licence
// compliance.
func TestParseHelpFileAgainstTheRealArchive(t *testing.T) {
	for _, tc := range []struct {
		file string
		want int
	}{
		{"commands.hlp", 99},
		{"info.hlp", 19},
		{"socials.hlp", 4},
		{"spells.hlp", 44},
		{"wizhelp.hlp", 50},
	} {
		f, err := os.Open("../../data/text/help/" + tc.file)
		if err != nil {
			t.Fatalf("%s: %v", tc.file, err)
		}
		entries, err := ParseHelpFile(f)
		_ = f.Close()
		if err != nil {
			t.Fatalf("%s: ParseHelpFile: %v", tc.file, err)
		}
		if len(entries) != tc.want {
			t.Errorf("%s: got %d entries, want %d", tc.file, len(entries), tc.want)
		}
	}
}

func TestCirclemudCreditsEntryIsReachable(t *testing.T) {
	f, err := os.Open("../../data/text/help/info.hlp")
	if err != nil {
		t.Fatalf("opening info.hlp: %v", err)
	}
	defer func() { _ = f.Close() }()
	entries, err := ParseHelpFile(f)
	if err != nil {
		t.Fatalf("ParseHelpFile: %v", err)
	}

	idx := NewHelpIndex(entries)
	for _, query := range []string{"circlemud", "credits", "circle", "CIRCLEMUD"} {
		e, ok := idx.Lookup(query)
		if !ok {
			t.Errorf("Lookup(%q) found nothing", query)
			continue
		}
		if !strings.Contains(e.Body, "CircleMUD was developed from DikuMud") {
			t.Errorf("Lookup(%q) = %q, want the CircleMUD/DikuMud credits text", query, e.Body)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
