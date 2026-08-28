// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The `show` field table, against the C's — the same treatment `set`'s table
// gets next door, and for the same reason: it is data, and the C dispatches
// on the *index*, so a row in the wrong place hands every mode after it
// somebody else's handler.
//
// `show houses` was the one entry with no handler behind it until this test
// was written; the table is only worth deriving from the C if every row it
// derives actually does something, so the two landed together.

var showFieldLine = regexp.MustCompile(`\{\s*"(\w+)",\s*(LVL_\w+|0)\s*\}`)

func TestTheShowTableMatchesTheCSource(t *testing.T) {
	b, err := os.ReadFile("../../reference/moderncserver/src/act.wizard.c")
	if err != nil {
		t.Fatalf("reading act.wizard.c: %v", err)
	}

	// Only do_show's own `fields[]`, so the set table cannot be mistaken for
	// it. The declaration is `} fields[] = {` inside do_show.
	src := string(b)
	start := strings.Index(src, "} fields[] = {")
	if start < 0 {
		t.Fatal("no show fields table in the C source")
	}
	end := strings.Index(src[start:], "\n  };")
	if end < 0 {
		t.Fatal("the show fields table is not terminated")
	}

	levels := map[string]int32{
		"LVL_IMMORT": game.LevelImmortal,
		"LVL_GOD":    game.LevelGod,
		"LVL_GRGOD":  game.LevelGreaterGod,
		"LVL_FREEZE": game.LevelGreaterGod, // structs.h:495
		"LVL_IMPL":   game.LevelImplementor,
	}

	matches := showFieldLine.FindAllStringSubmatch(src[start:start+end], -1)
	// Entry 0 is `{ "nothing", 0 }`, which nothing can name: the C's search
	// starts at index 1 and prints the option list from there. The Go table
	// leaves it out and indexes from zero instead.
	if len(matches) == 0 || matches[0][1] != "nothing" {
		t.Fatalf("the C's show table does not start with `nothing`: %v", matches)
	}
	matches = matches[1:]

	if len(matches) != len(showFields) {
		t.Fatalf("the C has %d show fields, this package has %d", len(matches), len(showFields))
	}
	for i, m := range matches {
		got, name := showFields[i], m[1]
		if got.name != name {
			t.Errorf("field %d is %q, the C has %q", i, got.name, name)
			continue
		}
		want, ok := levels[m[2]]
		if !ok {
			t.Errorf("%s: unknown level %q in the C", name, m[2])
			continue
		}
		if got.level != want {
			t.Errorf("%s is level %d, the C has %s (%d)", name, got.level, m[2], want)
		}
	}
	t.Logf("checked %d show fields against the C", len(matches))
}
