// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// makeDoor turns the link between the two test rooms into a door, on both
// sides, and returns them.
func makeDoor(t *testing.T, srv *Server, state game.Flags, key game.ObjVnum) (near, far *game.ExitDef) {
	t.Helper()

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		near = w.Room(ImmortStartRoom).Exits[game.South]
		far = w.Room(MortalStartRoom).Exits[game.North]

		for _, e := range []*game.ExitDef{near, far} {
			e.Keywords = "gate"
			e.Key = key
			e.State = game.ExitIsDoor | state
		}
	}); err != nil {
		t.Fatal(err)
	}
	return near, far
}

// TestOpeningAndClosingADoorMovesBothSides. A door is one object seen from
// two rooms, and the C keeps the two exits in step.
func TestOpeningAndClosingADoorMovesBothSides(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	near, far := makeDoor(t, srv, game.ExitClosed, game.NoObject)

	c.send("open gate")
	c.expect("Okay.")
	if near.State.Has(game.ExitClosed) || far.State.Has(game.ExitClosed) {
		t.Error("opening the gate left one side closed")
	}

	c.send("close gate")
	// The second "Okay." — expect returns at once on the first, which is the
	// one the open command already produced.
	c.expectCount("Okay.", 2)
	if !near.State.Has(game.ExitClosed) || !far.State.Has(game.ExitClosed) {
		t.Error("closing the gate left one side open")
	}
}

// TestTheRoomBeyondIsTold when a door opens or closes, but not when it is
// locked — a lock makes no noise through a wall.
func TestTheRoomBeyondIsTold(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	_, watcherClient := place(t, srv, fighterRecord("Welmar", 10, 100), MortalStartRoom)
	makeDoor(t, srv, game.ExitClosed, testKeyVnum)

	c.send("open gate")
	c.expect("Okay.")
	if !watcherClient.said("The gate is opened from the other side.") {
		t.Error("the room beyond was not told the gate opened")
	}

	c.send("close gate")
	c.expectCount("Okay.", 2)
	if !watcherClient.said("The gate is closed from the other side.") {
		t.Error("the room beyond was not told the gate closed")
	}
}

// TestTheDoorPreconditions, which the C expresses as a table and which are
// therefore uniform and easy to get subtly wrong.
func TestTheDoorPreconditions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   game.Flags
		command string
		expect  string
	}{
		{"opening an open door", 0, "open gate", "But it's currently open!"},
		{"closing a closed door", game.ExitClosed, "close gate", "But it's already closed!"},
		{"opening a locked door", game.ExitClosed | game.ExitLocked, "open gate", "It seems to be locked."},
		{"locking an open door", 0, "lock gate", "But it's currently open!"},
		{"unlocking an unlocked door", game.ExitClosed, "unlock gate", "Oh.. it wasn't locked, after all.."},
		// Locking an already-locked door reports it as locked, not as
		// unlocked: `lock` needs UNLOCKED, so it is the unlocked
		// precondition that fails.
		{"locking a locked door", game.ExitClosed | game.ExitLocked, "lock gate", "It seems to be locked."},
		{"picking an unlocked door", game.ExitClosed, "pick gate", "Oh.. it wasn't locked, after all.."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			c := dialClient(t, listening(t, srv))
			c.create("Zod", "swordfish", "m", "w")

			makeDoor(t, srv, tc.state, testKeyVnum)

			c.send(tc.command)
			c.expect(tc.expect)
		})
	}
}

// TestLockingNeedsTheKey, and an immortal needs neither.
func TestLockingNeedsTheKey(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// A mortal, so the immortal bypass does not hide the check. The first
	// character on the roster is always the Implementor.
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "w")

	// A mortal starts in the temple, so put the door on that side.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		for _, e := range []*game.ExitDef{
			w.Room(MortalStartRoom).Exits[game.North],
			w.Room(ImmortStartRoom).Exits[game.South],
		} {
			e.Keywords = "gate"
			e.Key = testKeyVnum
			e.State = game.ExitIsDoor | game.ExitClosed
		}
	}); err != nil {
		t.Fatal(err)
	}

	c.send("lock gate")
	c.expect("You don't seem to have the proper key.")

	// With the key in hand it works.
	drop(t, srv, testKeyVnum, MortalStartRoom)
	c.send("get key")
	c.expect("You get a small key.")
	c.send("lock gate")
	c.expect("*Click*")
}

// TestAnImmortalNeedsNoKey.
func TestAnImmortalNeedsNoKey(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	makeDoor(t, srv, game.ExitClosed, testKeyVnum)

	c.send("lock gate")
	c.expect("*Click*")
}

// TestPickingALock: pickproof doors resist, a door with no keyhole confuses,
// and the skill decides the rest.
func TestPickingALock(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// Pickproof.
	makeDoor(t, srv, game.ExitClosed|game.ExitLocked|game.ExitPickproof, testKeyVnum)
	c.send("pick gate")
	c.expect("It resists your attempts to pick it.")

	// No keyhole.
	makeDoor(t, srv, game.ExitClosed|game.ExitLocked, game.NoObject)
	c.send("pick gate")
	c.expect("Odd - you can't seem to find a keyhole.")

	// No skill. This has to be set deliberately: the first character on the
	// roster is the Implementor, and init_char gives an Implementor every
	// skill at 100%. The roll is 1..101 against zero, so it always fails.
	setSkill(t, srv, "Zod", game.SkillPickLock, 0)
	makeDoor(t, srv, game.ExitClosed|game.ExitLocked, testKeyVnum)
	c.send("pick gate")
	c.expect("You failed to pick the lock.")

	// Full skill: the roll runs to 101, so even a perfect thief fails one
	// time in a hundred and one. Try until it opens.
	setSkill(t, srv, "Zod", game.SkillPickLock, 100)

	var picked bool
	for i := 0; i < 50 && !picked; i++ {
		c.send("pick gate")
		c.expectAny("yields to your skills", "failed to pick")
		picked = c.seen("yields to your skills")
	}
	if !picked {
		t.Error("a thief with 100% pick lock never opened the gate in fifty tries")
	}
}

// setSkill sets one skill on a character already in the world.
func setSkill(t *testing.T, srv *Server, name string, skill, percent int32) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		rec := w.Find(name).Record
		if rec.Skills == nil {
			rec.Skills = map[int32]int32{}
		}
		rec.Skills[skill] = percent
	}); err != nil {
		t.Fatal(err)
	}
}

// TestAClosedDoorStopsAPlayer, which it did not before this slice: the
// mobiles respected doors and the players walked straight through them.
func TestAClosedDoorStopsAPlayer(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	makeDoor(t, srv, game.ExitClosed, game.NoObject)

	c.send("south")
	c.expect("The gate seems to be closed.")

	c.send("open gate")
	c.expect("Okay.")
	c.send("south")
	c.expect("The Temple Of Midgaard")
}

// TestANamelessDoorIsJustADoor.
func TestANamelessDoorIsJustADoor(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		e := w.Room(ImmortStartRoom).Exits[game.South]
		e.Keywords = ""
		e.State = game.ExitIsDoor | game.ExitClosed
	}); err != nil {
		t.Fatal(err)
	}

	c.send("south")
	c.expect("It seems to be closed.")

	// And it can still be opened by direction.
	c.send("open south")
	c.expect("Okay.")
}

// TestOperatingSomethingThatIsNotADoor.
func TestOperatingSomethingThatIsNotADoor(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// The exit exists but is not a door.
	c.send("open south")
	c.expect("You can't open that!")

	// And a direction with no exit at all.
	c.send("open north")
	c.expect("There doesn't seem to be a north here.")

	c.send("open")
	c.expect("Open what?")
}

// TestAOneWayDoorHasOneSide. Two rooms can point at each other one-way, in
// which case the doors are separate things and operating one must not touch
// the other.
func TestAOneWayDoorHasOneSide(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	var far *game.ExitDef
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		near := w.Room(ImmortStartRoom).Exits[game.South]
		near.Keywords = "gate"
		near.State = game.ExitIsDoor | game.ExitClosed

		far = w.Room(MortalStartRoom).Exits[game.North]
		far.Keywords = "gate"
		far.State = game.ExitIsDoor | game.ExitClosed
		// Points somewhere else, so it is not the same door.
		far.ToRoom = MortalStartRoom
	}); err != nil {
		t.Fatal(err)
	}

	c.send("open gate")
	c.expect("Okay.")

	if !far.State.Has(game.ExitClosed) {
		t.Error("opening one side of a one-way pair opened the other")
	}
}
