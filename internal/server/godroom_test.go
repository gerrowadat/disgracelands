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

// do_simple_move's godroom check (act.movement.c:147-151), unported until
// #266.
//
// ROOM_GODROOM was parsed, named and listed by `show godrooms`, and `goto`
// and `teleport` both honoured it — so the flag looked fully wired up right
// up until you noticed that walking, the one way most characters would ever
// reach such a room, did not test it at all.

// flagGodRoom marks the room north of the temple ROOM_GODROOM.
func flagGodRoom(t *testing.T, srv *Server) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(ImmortStartRoom)
		if room == nil {
			t.Error("no room north of the temple to flag")
			return
		}
		room.Flags = room.Flags.With(game.RoomGodRoom)
	})
}

// TestAMortalCannotWalkIntoAGodRoom — the bug the issue was filed for.
func TestAMortalCannotWalkIntoAGodRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	flagGodRoom(t, srv)

	mortal.send("north")
	mortal.expect("You aren't godly enough to use that room!")
	if got := roomOf(t, srv, "Bystander"); got != MortalStartRoom {
		t.Errorf("a mortal walked into a god room and is now in %d", got)
	}
}

// TestAPlainImmortalIsRefusedAGodRoomToo. The level is LVL_GRGOD, not
// LVL_IMMORT (act.movement.c:148), so the flag shuts out the two ranks below
// it as flatly as it shuts out a mortal — which is easy to miss, because
// every other god-shaped gate nearby turns on LVL_IMMORT.
func TestAPlainImmortalIsRefusedAGodRoomToo(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	flagGodRoom(t, srv)

	for _, level := range []int32{game.LevelImmortal, game.LevelGod} {
		mortalLevel(t, srv, "Bystander", level)
		mortal.send("north")
		mortal.expect("You aren't godly enough to use that room!")
		if got := roomOf(t, srv, "Bystander"); got != MortalStartRoom {
			t.Errorf("a level-%d god walked into a god room and is now in %d", level, got)
		}
	}

	// LVL_GRGOD itself is the floor, and `<` is the test, so 33 walks in.
	mortalLevel(t, srv, "Bystander", game.LevelGreaterGod)
	mortal.send("north")
	mortal.expect("The Immortal Board Room")
	if got := roomOf(t, srv, "Bystander"); got != ImmortStartRoom {
		t.Errorf("a greater god was refused their own room; they are in %d", got)
	}
}

// TestTheMovementGodRoomRefusalKeepsItsContraction. Three call sites read this
// flag and they do not all say the same thing: `goto` and `teleport` say "You
// are not godly enough to use that room!" (act.wizard.c), and do_simple_move
// says "You aren't" (act.movement.c:150). The difference is the C's, so the
// string here is deliberately not shared with the other two.
func TestTheMovementGodRoomRefusalKeepsItsContraction(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)
	flagGodRoom(t, srv)

	mortal.send("north")
	mortal.expect("You aren't godly enough to use that room!")
	if mortal.seen("You are not godly enough") {
		t.Error("the movement refusal used the teleport wording")
	}

	// And the other wording still belongs to the other path. An implementor
	// is above LVL_GRGOD, so demote them onto the wrong side of it first.
	mortalLevel(t, srv, "Zod", game.LevelImmortal)
	god.send("goto 1204")
	god.expect("You are not godly enough to use that room!")
}
