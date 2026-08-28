// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"fmt"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// do_simple_move's ROOM_DEATH branch (act.movement.c:171-176), which was
// never ported: the flag was read, stored, listed by `show death` and avoided
// by wandering mobiles, and walking into one of the rooms was survivable.
// Issue #209.

// deathTrapRoom flags the room north of the temple as a death trap, so a
// mortal standing in the temple is one `north` away from it.
//
// ImmortStartRoom rather than a room of its own: the test world is rebuilt
// for every server (server_test.go), so flagging an existing room changes
// nothing for any other test, and this one is already joined to the temple in
// both directions — which is what the death cry's CAN_GO loop needs.
func deathTrapRoom(t *testing.T, srv *Server) game.RoomVnum {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(ImmortStartRoom)
		if room == nil {
			t.Errorf("no room %d", ImmortStartRoom)
			return
		}
		room.Flags = room.Flags.Set(game.RoomDeathTrap)
	})
	return ImmortStartRoom
}

// The whole of it: a mortal walks in, sees the room, and is out of the game
// with their belongings on the floor behind them.
func TestWalkingIntoADeathTrap(t *testing.T) {
	srv, _ := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))
	trap := deathTrapRoom(t, srv)

	inWorld(t, srv, func(w *game.Live) {
		w.ObjectToChar(w.NewObject(testSwordVnum), w.Find("Mortal"))
	})

	c.send("north")
	// The room description first: the death-trap branch is the last thing
	// do_simple_move does, *after* look_at_room (act.movement.c:169), so the
	// last thing a player sees is the room that killed them.
	c.expect("The Immortal Board Room")
	// extract_char_final's CON_MENU (handler.c:931), the same ending `quit`
	// has. Nothing announces a death: this is not die(), and the C prints no
	// "You are dead!" of any kind here.
	c.expectCount("Make your choice:", 2)

	var (
		gone    bool
		onFloor int
	)
	inWorld(t, srv, func(w *game.Live) {
		gone = w.Find("Mortal") == nil
		for _, o := range w.RoomObjects(trap) {
			if o.Vnum() == testSwordVnum {
				onFloor++
			}
		}
	})
	if !gone {
		t.Error("the character survived a death trap")
	}
	// extract_char_final drops the inventory into IN_ROOM (handler.c:906) —
	// loose in the room, not into a corpse. There is no corpse: nothing here
	// calls make_corpse.
	if onFloor != 1 {
		t.Errorf("the sword is in the death trap %d times, want 1", onFloor)
	}
	inWorld(t, srv, func(w *game.Live) {
		for _, o := range w.RoomObjects(trap) {
			if game.IsCorpse(o) {
				t.Error("a death trap left a corpse behind")
			}
		}
	})
}

// `GET_LEVEL(ch) < LVL_IMMORT` (act.movement.c:171). A god can stand in one,
// which is what makes the flag usable at all: somebody has to be able to go
// and look at the room.
func TestADeathTrapSparesAnImmortal(t *testing.T) {
	srv, _ := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))
	trap := deathTrapRoom(t, srv)

	setLevel(t, srv, "Mortal", game.LevelImmortal)

	c.send("north")
	c.expect("The Immortal Board Room")
	c.settle()

	where := game.NoRoom
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Mortal"); who != nil {
			where = who.Room
		}
	})
	if where != trap {
		t.Errorf("the immortal is in room %d, want the death trap at %d", where, trap)
	}
}

// death_cry (fight.c:367), through the one caller that is not a death: the
// room hears whose it was and every room one step away hears that it was
// somebody. Here the victim is alone in the trap, so the only cry that can be
// heard is the neighbours' one, back in the temple.
func TestADeathTrapIsHeardNextDoor(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)
	deathTrapRoom(t, srv)

	mortal.send("north")
	mortal.expectCount("Make your choice:", 2)

	// settle() first: `expect` waits for a write to *this* client's socket,
	// and the god's copy is a separate write on the world goroutine.
	god.settle()
	if !god.seen("Your blood freezes as you hear someone's death cry.") {
		t.Errorf("the temple did not hear the death cry:\n%s", god.transcript())
	}
}

// log_death_trap's `<DoC>` half (utils.c:172-173): a send_to_all_color, so
// the whole game is told and not just the gods watching syslog. The reader
// here is in another room entirely.
func TestADeathTrapIsAnnouncedToTheWholeGame(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)
	deathTrapRoom(t, srv)

	moveTo(t, srv, "Zod", ShopRoom)
	god.settle()

	mortal.send("north")
	mortal.expectCount("Make your choice:", 2)

	god.settle()
	want := "A voice whispers in your ear, 'Bystander has met their demise " +
		"in the fatal death trap, 'The Immortal Board Room'.'"
	if !god.seen(want) {
		t.Errorf("the game was not told about the death trap:\n%s", god.transcript())
	}
}

// mudlog(buf, BRF, LVL_IMMORT, TRUE) (utils.c:170). BRF is the C's own floor,
// so `syslog brief` is enough to see it.
func TestADeathTrapIsLoggedToTheSyslog(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)
	trap := deathTrapRoom(t, srv)

	god.send("syslog brief")
	god.expect("Your syslog is now brief.")

	mortal.send("north")
	mortal.expectCount("Make your choice:", 2)

	god.settle()
	// "%s hit death trap #%d (%s)" — the name, GET_ROOM_VNUM and the room's
	// name (utils.c:167-168).
	want := fmt.Sprintf("Bystander hit death trap #%d (The Immortal Board Room)", trap)
	if !god.seen(want) {
		t.Errorf("the death trap was not logged:\n%s", god.transcript())
	}
}

// do_simple_move returns 0 from the death trap, and perform_move takes that
// as a failure and never reaches its follower loop (act.movement.c:202). So a
// leader who walks into one dies alone: the group stays where it was.
func TestADeathTrapDoesNotTakeTheFollowers(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)
	deathTrapRoom(t, srv)

	god.send("follow Bystander")
	god.expect("You now follow Bystander.")
	mortal.settle()

	mortal.send("north")
	mortal.expectCount("Make your choice:", 2)

	god.settle()
	where := game.NoRoom
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Zod"); who != nil {
			where = who.Room
		}
	})
	if where != MortalStartRoom {
		t.Errorf("the follower is in room %d, want the temple at %d", where, MortalStartRoom)
	}
}
