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

// The `set` field table, against the C's.
//
// Fifty-two entries, each with a level, a PC/NPC restriction and a value
// kind, and the C dispatches on the *index* — so an entry inserted in the
// wrong place silently gives every field after it somebody else's handler.
// The table is data, so it is checked rather than read.

var setFieldLine = regexp.MustCompile(
	`\{\s*"(\w+)",\s*(LVL_\w+),\s*(PC|NPC|BOTH),\s*(MISC|BINARY|NUMBER)\s*\}`)

func TestTheSetTableMatchesTheCSource(t *testing.T) {
	b, err := os.ReadFile("../../reference/moderncserver/src/act.wizard.c")
	if err != nil {
		t.Fatalf("reading act.wizard.c: %v", err)
	}

	// Only the block after `set_fields[] = {`, so the show table and the
	// command table cannot be mistaken for it.
	src := string(b)
	start := strings.Index(src, "set_fields[] = {")
	if start < 0 {
		t.Fatal("no set_fields table in the C source")
	}
	end := strings.Index(src[start:], "\n  };")
	if end < 0 {
		t.Fatal("the set_fields table is not terminated")
	}

	levels := map[string]int32{
		"LVL_IMMORT": game.LevelImmortal,
		"LVL_GOD":    game.LevelGod,
		"LVL_GRGOD":  game.LevelGreaterGod,
		"LVL_FREEZE": game.LevelGreaterGod, // structs.h:495
		"LVL_IMPL":   game.LevelImplementor,
	}
	whos := map[string]setWho{"PC": setPC, "NPC": setNPC, "BOTH": setBoth}
	kinds := map[string]setKind{"MISC": setMisc, "BINARY": setBinary, "NUMBER": setNumber}

	matches := setFieldLine.FindAllStringSubmatch(src[start:start+end], -1)
	if len(matches) != len(setFields) {
		t.Fatalf("the C has %d set fields, this package has %d", len(matches), len(setFields))
	}

	for i, m := range matches {
		got, name := setFields[i], m[1]
		if got.name != name {
			t.Errorf("field %d is %q, the C has %q", i, got.name, name)
			continue
		}
		if want, ok := levels[m[2]]; !ok {
			t.Errorf("%s: unknown level %q in the C", name, m[2])
		} else if got.level != want {
			t.Errorf("%s is level %d, the C has %s (%d)", name, got.level, m[2], want)
		}
		if want := whos[m[3]]; got.who != want {
			t.Errorf("%s applies to %v, the C has %s", name, got.who, m[3])
		}
		if want := kinds[m[4]]; got.kind != want {
			t.Errorf("%s is kind %v, the C has %s", name, got.kind, m[4])
		}
	}
	t.Logf("checked %d set fields against the C", len(matches))
}

// Every field must have a handler: a nil one is a panic waiting for the first
// god who types it.
func TestEverySetFieldHasAHandler(t *testing.T) {
	for _, field := range setFields {
		if field.apply == nil {
			t.Errorf("%s has no handler", field.name)
		}
	}
}
