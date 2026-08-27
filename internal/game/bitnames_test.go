// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"regexp"
	"testing"
)

// The name tables, against the C source they were transcribed from.
//
// A table is data, and data gets checked rather than read — the same argument
// as the command table and the special-procedure assignments. These are the
// only place several of these bits are described anywhere, and `stat` prints
// nearly all of them, so a name shifted by one position renames every flag
// after it and nothing else notices.

// cTableSource matches `const char *name[] = { "A", "B", ... };`.
var cTableSource = regexp.MustCompile(`(?s)const char \*(\w+)\[\]\s*=?\s*\{(.*?)\n\};`)

var cStringLiteral = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

var (
	cBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cLineComment  = regexp.MustCompile(`//[^\n]*`)
)

// parseCTables reads every `const char *x[]` table out of a C source file.
//
// The `"\n"` sentinel that ends each one is dropped: it is the C's
// terminator, not a name.
func parseCTables(t *testing.T, path string) map[string][]string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	out := map[string][]string{}
	for _, m := range cTableSource.FindAllStringSubmatch(string(b), -1) {
		body := cLineComment.ReplaceAllString(cBlockComment.ReplaceAllString(m[2], ""), "")
		var entries []string
		for _, lit := range cStringLiteral.FindAllStringSubmatch(body, -1) {
			if lit[1] == `\n` {
				continue
			}
			entries = append(entries, lit[1])
		}
		out[m[1]] = entries
	}
	return out
}

func TestTheNameTablesMatchTheCSource(t *testing.T) {
	constants := parseCTables(t, "../../reference/moderncserver/src/constants.c")
	classes := parseCTables(t, "../../reference/moderncserver/src/class.c")

	for _, tc := range []struct {
		cName string
		from  map[string][]string
		got   []string
	}{
		{"affected_bits", constants, AffectBitNames()},
		{"extra_bits", constants, ExtraBitNames()},
		{"apply_types", constants, ApplyTypeNames()},
		{"room_bits", constants, RoomBitNames()},
		{"exit_bits", constants, ExitBitNames()},
		{"sector_types", constants, SectorNames()},
		{"genders", constants, GenderNames()},
		{"position_types", constants, PositionNames()},
		{"player_bits", constants, PlayerBitNames()},
		{"action_bits", constants, ActionBitNames()},
		{"preference_bits", constants, PreferenceBitNames()},
		{"connected_types", constants, ConnectedNames()},
		{"item_types", constants, ItemTypeNames},
		{"wear_bits", constants, WearBitNames()},
		{"container_bits", constants, ContainerBitNames()},
		{"npc_class_types", constants, NpcClassNames()},
		{"pc_class_types", classes, PcClassNames()},
	} {
		want, ok := tc.from[tc.cName]
		if !ok {
			t.Errorf("%s: no such table in the C source", tc.cName)
			continue
		}
		if len(tc.got) != len(want) {
			t.Errorf("%s has %d entries, the C has %d\n got: %q\nwant: %q",
				tc.cName, len(tc.got), len(want), tc.got, want)
			continue
		}
		for i := range want {
			if tc.got[i] != want[i] {
				t.Errorf("%s[%d] is %q, the C has %q", tc.cName, i, tc.got[i], want[i])
			}
		}
	}
}

// The drink names are a table too, and they are indexed by liquid number
// everywhere from `stat` to a fountain's description.
func TestTheDrinkNamesMatchTheCSource(t *testing.T) {
	constants := parseCTables(t, "../../reference/moderncserver/src/constants.c")

	want, ok := constants["drinks"]
	if !ok {
		t.Fatal("no drinks table in the C source")
	}
	if len(drinkNames) != len(want) {
		t.Fatalf("drinks has %d entries, the C has %d", len(drinkNames), len(want))
	}
	for i := range want {
		if drinkNames[i] != want[i] {
			t.Errorf("drinks[%d] is %q, the C has %q", i, drinkNames[i], want[i])
		}
	}
}

// The two tables look_in_obj describes a drink container with are re-parsed
// out of the C the same way every other name table is.
//
// fullness[] is the one table in constants.c with no "\n" sentinel — the C
// carries a comment saying so, "Not used in sprinttype() so no \n." — and its
// last entry is an empty string literal rather than a word. That is worth a
// test rather than a glance: an off-by-one here would describe a full
// container as "more than half full" and an empty one is never asked about,
// so nothing else in the game would notice.
func TestTheLiquidTablesMatchTheCSource(t *testing.T) {
	constants := parseCTables(t, "../../reference/moderncserver/src/constants.c")

	for _, tc := range []struct {
		cName string
		got   []string
	}{
		{"color_liquid", liquidColours[:]},
		{"fullness", fullnessNames[:]},
	} {
		want, ok := constants[tc.cName]
		if !ok {
			t.Errorf("%s: no such table in the C source", tc.cName)
			continue
		}
		if len(tc.got) != len(want) {
			t.Errorf("%s has %d entries, the C has %d\n got: %q\nwant: %q",
				tc.cName, len(tc.got), len(want), tc.got, want)
			continue
		}
		for i := range want {
			if tc.got[i] != want[i] {
				t.Errorf("%s[%d] is %q, the C has %q", tc.cName, i, tc.got[i], want[i])
			}
		}
	}
}
