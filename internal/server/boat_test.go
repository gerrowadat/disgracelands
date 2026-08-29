// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// do_simple_move's boat check (act.movement.c:112-119) and has_boat
// (act.movement.c:52), which were unported until #265: deep water was walked
// across as if it were a road.
//
// The test world has no water in it, so these make some: the room north of
// the temple becomes SECT_WATER_NOSWIM.

// floodTheBoardRoom makes the room north of the temple deep water. The world
// is rebuilt per newTestServer, so the sector needs no undoing.
//
// One consequence worth knowing before reading the assertions: deep water is
// neither SECT_INSIDE nor SECT_CITY, so room_is_dark calls the flooded room
// outdoors, and the test world's clock starts at midnight. A character who
// arrives there is told "It is pitch black" and never sees the room's name —
// so these tests assert on `roomOf`, which is what they mean anyway.
func floodTheBoardRoom(t *testing.T, srv *Server) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(ImmortStartRoom)
		if room == nil {
			t.Error("no room north of the temple to flood")
			return
		}
		room.SectorType = game.SectorWaterNoSwim
	})
}

// mortalLevel puts a character at a given level, for the gates that turn on
// one. Read out of the closure, never asserted inside it.
func mortalLevel(t *testing.T, srv *Server, name string, level int32) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		ch := w.Find(name)
		if ch == nil || ch.Record == nil {
			t.Errorf("no character called %s", name)
			return
		}
		ch.Record.Level = level
	})
}

// TestDeepWaterNeedsABoat: the plain case, and the one that made this an
// issue — without it a mortal walks into the sea.
func TestDeepWaterNeedsABoat(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	floodTheBoardRoom(t, srv)

	mortal.send("north")
	mortal.expect("You need a boat to go there.")
	if got := roomOf(t, srv, "Bystander"); got != MortalStartRoom {
		t.Errorf("a mortal with no boat is in room %d, want to be stopped in %d", got, MortalStartRoom)
	}
}

// TestABoatInYourInventoryIsEnough, provided it is one you could not wear:
// has_boat's inventory loop is guarded on `find_eq_pos(ch, obj, NULL) < 0`.
func TestABoatInYourInventoryIsEnough(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	floodTheBoardRoom(t, srv)

	drop(t, srv, testBoatVnum, MortalStartRoom)
	mortal.send("get canoe")
	mortal.expect("You get a small canoe.")

	mortal.send("north")
	mortal.settle()
	if got := roomOf(t, srv, "Bystander"); got != ImmortStartRoom {
		t.Errorf("a character carrying a canoe is in room %d, want %d", got, ImmortStartRoom)
	}
}

// TestAWearableBoatHasToBeWorn is the other half of that guard, and the part
// that reads wrongly: a boat with a wear slot does nothing while it is merely
// carried. The C's own comment says "non-wearable boats in inventory will do
// it", and the loop below it is what picks the thing up once it is on.
func TestAWearableBoatHasToBeWorn(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	floodTheBoardRoom(t, srv)

	drop(t, srv, testBootsVnum, MortalStartRoom)
	mortal.send("get waders")
	mortal.expect("You get a pair of waders.")

	mortal.send("north")
	mortal.expect("You need a boat to go there.")
	if got := roomOf(t, srv, "Bystander"); got != MortalStartRoom {
		t.Errorf("carrying a wearable boat floated a character into room %d", got)
	}

	mortal.send("wear waders")
	mortal.expect("You wear a pair of waders on your feet.")
	mortal.send("north")
	mortal.settle()
	if got := roomOf(t, srv, "Bystander"); got != ImmortStartRoom {
		t.Errorf("a character wearing a boat is in room %d, want %d", got, ImmortStartRoom)
	}
}

// TestWaterwalkIsABoat. has_boat's second test, so the spell works without
// anybody carrying anything.
func TestWaterwalkIsABoat(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	floodTheBoardRoom(t, srv)

	affect(t, srv, "Bystander", game.AffectWaterwalk)
	mortal.send("north")
	mortal.settle()
	if got := roomOf(t, srv, "Bystander"); got != ImmortStartRoom {
		t.Errorf("a waterwalking character is in room %d, want %d", got, ImmortStartRoom)
	}
}

// TestAPlainImmortalStillNeedsABoat, which is the surprise in has_boat. Its
// level test is `GET_LEVEL(ch) > LVL_IMMORT` — strictly greater — so 31 is
// refused and 32 is not. Every neighbouring gate in do_simple_move that lets
// gods past is `< LVL_IMMORT`, so this one is off by exactly one from all of
// them.
func TestAPlainImmortalStillNeedsABoat(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	floodTheBoardRoom(t, srv)

	mortalLevel(t, srv, "Bystander", game.LevelImmortal)
	mortal.send("north")
	mortal.expect("You need a boat to go there.")
	if got := roomOf(t, srv, "Bystander"); got != MortalStartRoom {
		t.Errorf("a level-31 immortal reached room %d; the C's test is > LVL_IMMORT", got)
	}

	mortalLevel(t, srv, "Bystander", game.LevelGod)
	mortal.send("north")
	mortal.settle()
	if got := roomOf(t, srv, "Bystander"); got != ImmortStartRoom {
		t.Errorf("a level-32 god is in room %d, want %d", got, ImmortStartRoom)
	}
}

// TestLeavingDeepWaterNeedsABoatToo. The C tests *either* room
// (act.movement.c:113-114), not just the one being entered — so losing your
// boat mid-crossing strands you rather than letting you wade ashore.
func TestLeavingDeepWaterNeedsABoatToo(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	floodTheBoardRoom(t, srv)

	drop(t, srv, testBoatVnum, MortalStartRoom)
	mortal.send("get canoe")
	mortal.expect("You get a small canoe.")
	mortal.send("north")
	mortal.settle()
	if got := roomOf(t, srv, "Bystander"); got != ImmortStartRoom {
		t.Errorf("a character carrying a canoe is in room %d, want %d", got, ImmortStartRoom)
	}

	// Taken rather than dropped: the flooded room is pitch black, so `drop`
	// would be answered with "You don't seem to have a canoe" — a character
	// cannot see their own inventory in the dark.
	inWorld(t, srv, func(w *game.Live) {
		ch := w.Find("Bystander")
		if ch == nil || len(ch.Carrying) == 0 {
			t.Error("Bystander is not carrying the canoe")
			return
		}
		w.ExtractObject(ch.Carrying[0])
	})

	mortal.send("south")
	mortal.expect("You need a boat to go there.")
	if got := roomOf(t, srv, "Bystander"); got != ImmortStartRoom {
		t.Errorf("a character with no boat waded out of deep water into room %d", got)
	}
}
