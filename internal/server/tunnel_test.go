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

// do_simple_move's tunnel cap (act.movement.c:139-146), which was the last
// thing in that function's run of gates still unported (#136).
//
// The test world has no TUNNEL room, so these flag one: ImmortStartRoom, which
// is one step north of the temple everybody starts in.

// tunnelWorld flags the room north of the temple TUNNEL and sets the cap,
// restoring the tuning afterwards. The room is this server's own — the world
// is rebuilt per newTestServer — so the flag needs no undoing.
func tunnelWorld(t *testing.T, srv *Server, size int32) {
	t.Helper()

	orig := game.Tuning()
	t.Cleanup(func() { game.SetTuning(orig) })
	tuning := orig
	tuning.TunnelSize = size
	game.SetTuning(tuning)

	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(ImmortStartRoom)
		if room == nil {
			t.Error("no room north of the temple to flag")
			return
		}
		room.Flags = room.Flags.Set(game.RoomTunnel)
	})
}

// TestATunnelFillsUp: the cap is how many may be in the room, not how many may
// already be there — the C's test is `>=` (act.movement.c:140).
func TestATunnelFillsUp(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)
	tunnelWorld(t, srv, 2)

	// Two fit, on the default setting.
	god.send("north")
	god.expect("The Immortal Board Room")
	mortal.send("north")
	mortal.expect("The Immortal Board Room")

	// The third does not.
	third := dialClient(t, addr)
	third.create("Cerberus", "swordfish", "f", "w")
	third.send("north")
	third.expect("There isn't enough room for you to go there!")
	if third.seen("The Immortal Board Room") {
		t.Error("a third player got into a tunnel with tunnel_size 2")
	}
}

// TestATunnelOfOneHasItsOwnMessage. `tunnel_size > 1` picks between the two
// wordings (act.movement.c:141-144), and it is the *setting* that decides,
// not the room's occupancy — so a server on the default 2 never prints this
// one however full the room is.
func TestATunnelOfOneHasItsOwnMessage(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)
	tunnelWorld(t, srv, 1)

	god.send("north")
	god.expect("The Immortal Board Room")

	mortal.send("north")
	mortal.expect("There isn't enough room there for more than one person!")
	if mortal.seen("The Immortal Board Room") {
		t.Error("a second player got into a tunnel with tunnel_size 1")
	}
}

// TestMobilesDoNotFillATunnel. num_pc_in_room counts `!IS_NPC` only
// (utils.c:575-585), so a tunnel packed with creatures is empty as far as the
// cap is concerned. Read quickly, "num_pc_in_room" looks like a count of
// bodies.
func TestMobilesDoNotFillATunnel(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	tunnelWorld(t, srv, 1)

	spawnDog(t, srv, ImmortStartRoom)
	spawnDog(t, srv, ImmortStartRoom)

	mortal.send("north")
	mortal.expect("The Immortal Board Room")
}

// TestATunnelStopsAnImmortalToo. There is no level test and no IS_NPC test on
// the mover (act.movement.c:139) — this is the one gate in do_simple_move an
// implementor cannot walk through, and `goto` is how they get past it, since
// that does not come through here at all.
func TestATunnelStopsAnImmortalToo(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)
	tunnelWorld(t, srv, 1)

	mortal.send("north")
	mortal.expect("The Immortal Board Room")

	god.send("north")
	god.expect("There isn't enough room there for more than one person!")

	// And `goto` still works, which is what makes the refusal survivable.
	god.send("goto 1200")
	god.expect("The Immortal Board Room")
}

// TestARefusedTunnelStepCostsNoMovement. The check sits before the charge
// (act.movement.c:139 against :161), so being turned back is free. Getting
// this order wrong is invisible until somebody walks into a full tunnel
// repeatedly.
func TestARefusedTunnelStepCostsNoMovement(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)
	tunnelWorld(t, srv, 1)

	god.send("north")
	god.expect("The Immortal Board Room")

	var before, after int32
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Bystander"); who != nil && who.Record != nil {
			before = who.Record.Points.Move
		}
	})

	mortal.send("north")
	mortal.expect("There isn't enough room there for more than one person!")

	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Bystander"); who != nil && who.Record != nil {
			after = who.Record.Points.Move
		}
	})
	if after != before {
		t.Errorf("a refused tunnel step cost %d movement, want it to cost nothing", before-after)
	}
}

// TestPlayersInRoomCountsPlayersOnly is num_pc_in_room on its own, since the
// gate above can only ever show it through one message.
func TestPlayersInRoomCountsPlayersOnly(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	twoInARoom(t, srv, addr)
	spawnDog(t, srv, MortalStartRoom)

	var got int32
	inWorld(t, srv, func(w *game.Live) { got = w.PlayersInRoom(MortalStartRoom) })
	if got != 2 {
		t.Errorf("PlayersInRoom with two players and a dog = %d, want 2", got)
	}
}
