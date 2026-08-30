// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package world

import (
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// danglingWorld is a world in which every reference that can dangle does.
func danglingWorld() *game.World {
	room := &game.RoomDef{Vnum: 100, Name: "A room"}
	room.Exits[game.North] = &game.ExitDef{ToRoom: 999}
	room.Exits[game.South] = &game.ExitDef{ToRoom: 100, Key: 888}
	return &game.World{
		Rooms:   []*game.RoomDef{room},
		Mobiles: []*game.MobDef{{Vnum: 100}},
		Objects: []*game.ObjDef{{Vnum: 100}},
		Shops:   []*game.ShopDef{{Vnum: 1, Keeper: 100, Rooms: []game.RoomVnum{100, 777}}},
		Zones: []*game.ZoneDef{{
			Vnum: 1, Bottom: 100, Top: 199,
			Commands: []game.ResetCommand{
				{Command: 'M', Line: 1, Arg1: 666, Arg3: 100},
				{Command: 'O', Line: 2, Arg1: 100, Arg3: 555},
				{Command: 'D', Line: 3, Arg1: 100, Arg2: 44},
				{Command: 'X', Line: 4},
			},
		}},
	}
}

// TestCheckReferencesFindsEveryDanglingVnum is the pass #286 moved out of
// the classic reader, exercised on a world rather than on a directory --
// which is the whole point of it living here. Anything a loader can put in
// a game.World, either loader can be asked about.
func TestCheckReferencesFindsEveryDanglingVnum(t *testing.T) {
	got := CheckReferences(danglingWorld())
	joined := ""
	for _, w := range got {
		joined += w.String() + "\n"
	}

	for _, want := range []string{
		"room #100's north exit leads to room #999",
		"room #100's south exit is locked by object #888",
		"shop #1 operates in room #777",
		"loads mob #666, which does not exist",
		"loads into room #555, which does not exist",
		"sets a door in direction 44, which is not a direction",
		`command "X" is not a reset command the server understands`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("CheckReferences did not report %q:\n%s", want, joined)
		}
	}
	for _, w := range got {
		if w.Severity != Warn {
			t.Errorf("%s is severity %s, want warn: a dangling vnum is playable-but-broken, "+
				"not a load failure", w.Message, w.Severity)
		}
	}
}

// A world in which nothing dangles produces nothing at all. Worth asserting
// separately: a check that fires on a clean world is worse than no check,
// because it trains a reader to skip the output.
func TestCheckReferencesIsSilentOnAWorldThatIsFine(t *testing.T) {
	w := danglingWorld()
	w.Rooms[0].Exits[game.North] = nil
	w.Rooms[0].Exits[game.South] = &game.ExitDef{ToRoom: 100, Key: 100}
	w.Shops[0].Rooms = []game.RoomVnum{100}
	w.Zones[0].Commands = []game.ResetCommand{
		{Command: 'M', Line: 1, Arg1: 100, Arg3: 100},
		{Command: 'D', Line: 2, Arg1: 100, Arg2: 0},
	}
	if got := CheckReferences(w); len(got) != 0 {
		t.Errorf("CheckReferences on a clean world = %v, want nothing", got)
	}
}

// TestCheckReferencesFindsOverlappingZones covers what nothing in either
// loader checked at all until #286 went looking (the issue's own last
// line). The C assigns a room to a zone with a forward-only cursor and
// exits outright for a room below the current zone (db.c:916-924), so an
// overlap is a directory that can boot here and refuse to boot there.
func TestCheckReferencesFindsOverlappingZones(t *testing.T) {
	w := danglingWorld()
	w.Zones = append(w.Zones, &game.ZoneDef{Vnum: 2, Bottom: 150, Top: 249})
	found := false
	for _, f := range CheckReferences(w) {
		if strings.Contains(f.Message, "overlapping vnum ranges") {
			found = true
			if !strings.Contains(f.Message, "zone #1 (rooms #100-#199)") ||
				!strings.Contains(f.Message, "zone #2 (rooms #150-#249)") {
				t.Errorf("the overlap report does not name both ranges: %s", f.Message)
			}
		}
	}
	if !found {
		t.Error("two zones sharing rooms #150-#199 were not reported")
	}

	// Ranges that merely touch do not overlap: #100-#199 and #200-#299 is
	// how every stock zone meets its neighbour, and reporting those would
	// be thirty findings on every world anyone has.
	w.Zones[1] = &game.ZoneDef{Vnum: 2, Bottom: 200, Top: 299}
	for _, f := range CheckReferences(w) {
		if strings.Contains(f.Message, "overlapping vnum ranges") {
			t.Errorf("adjacent zone ranges were reported as overlapping: %s", f.Message)
		}
	}
}
