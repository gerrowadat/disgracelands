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

// `enter`, `leave` and `order`, end to end.

// setIndoors flags a room as indoors, or clears it.
func setIndoors(t *testing.T, srv *Server, vnum game.RoomVnum, indoors bool) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(vnum)
		if room == nil {
			t.Errorf("no room %d", vnum)
			return
		}
		if indoors {
			room.Flags = room.Flags.With(game.RoomIndoors)
		} else {
			room.Flags = room.Flags.Without(game.RoomIndoors)
		}
	})
}

func roomOf(t *testing.T, srv *Server, name string) game.RoomVnum {
	t.Helper()
	room := game.NoRoom
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find(name); who != nil {
			room = who.Room
		}
	})
	return room
}

// The house is indoors and the atrium is not, which is exactly the shape
// `enter` and `leave` were written for.
func TestEnteringAndLeaving(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Rambler", "inandout", "m", "w")

	setIndoors(t, srv, HouseRoom, true)
	setIndoors(t, srv, AtriumRoom, false)
	moveTo(t, srv, "Rambler", AtriumRoom)

	c.send("leave")
	c.expect("You are outside.. where do you want to go?")

	c.send("enter")
	c.expect("A Small House")
	if got := roomOf(t, srv, "Rambler"); got != HouseRoom {
		t.Errorf("after entering they are in %d, want the house at %d", got, HouseRoom)
	}

	c.send("enter")
	c.expect("You are already indoors.")

	c.send("leave")
	c.expect("An Atrium")
	if got := roomOf(t, srv, "Rambler"); got != AtriumRoom {
		t.Errorf("after leaving they are in %d, want the atrium at %d", got, AtriumRoom)
	}
}

func TestEnteringSomethingThatIsNotThere(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Lost", "nowaythrough", "m", "w")

	moveTo(t, srv, "Lost", AtriumRoom)
	setIndoors(t, srv, AtriumRoom, false)
	setIndoors(t, srv, HouseRoom, false)

	c.send("enter portal")
	c.expect("There is no portal here.")

	// Nothing indoors next door, so nothing to enter.
	c.send("enter")
	c.expect("You can't seem to find anything to enter.")
}

func TestLeavingWithNoWayOut(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Shutin", "noexithere", "m", "w")

	// Both rooms indoors, so from the house there is nowhere outside to go.
	setIndoors(t, srv, HouseRoom, true)
	setIndoors(t, srv, AtriumRoom, true)
	moveTo(t, srv, "Shutin", HouseRoom)

	c.send("leave")
	c.expect("I see no obvious exits to the outside.")
}

// `enter <keyword>` walks through a named door.
func TestEnteringANamedDoor(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Knocker", "openthedoor", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(AtriumRoom)
		if room == nil || room.Exits[game.South] == nil {
			t.Error("the atrium has no south exit")
			return
		}
		room.Exits[game.South].Keywords = "gate"
	})
	moveTo(t, srv, "Knocker", AtriumRoom)

	c.send("enter gate")
	c.expect("A Small House")
}

func TestOrderingACharmedFollower(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Master", "doasisay", "m", "w")

	dog := aMobile(t, srv, "Master")

	c.send("order")
	c.expect("Order who to do what?")

	c.send("order dog")
	c.expectCount("Order who to do what?", 2)

	c.send("order nobody smile")
	c.expect("That person isn't here.")

	c.send("order master smile")
	c.expect("You obviously suffer from skitzofrenia.")

	// Not a follower: it hears the order and looks blank.
	c.send("order dog smile")
	c.expect("has an indifferent look.")

	// Charmed and following: it obeys.
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Master")
		if who == nil || dog == nil || dog.Record == nil {
			t.Error("could not charm the dog")
			return
		}
		w.AddFollower(dog, who)
		dog.Record.AffectFlags = dog.Record.AffectFlags.With(game.AffectCharm)
	})

	c.send("order dog smile")
	c.expect("Okay.")
	c.settle()
	if !c.seen("dog smiles happily") {
		t.Errorf("the dog did not obey:\n%s", c.transcript())
	}
}

func TestOrderingFollowers(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("General", "formupmen", "m", "w")

	dog := aMobile(t, srv, "General")

	c.send("order followers smile")
	c.expect("Nobody here is a loyal subject of yours!")

	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("General")
		if who == nil || dog == nil || dog.Record == nil {
			t.Error("could not charm the dog")
			return
		}
		w.AddFollower(dog, who)
		dog.Record.AffectFlags = dog.Record.AffectFlags.With(game.AffectCharm)
	})

	c.send("order followers smile")
	c.expect("Okay.")
	c.settle()
	if !c.seen("dog smiles happily") {
		t.Errorf("the follower did not obey:\n%s", c.transcript())
	}
}

// Somebody else's puppet does not get to give orders of their own.
func TestACharmedCharacterCannotGiveOrders(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Puppet", "notmyownman", "m", "w")

	aMobile(t, srv, "Puppet")
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Puppet"); who != nil && who.Record != nil {
			who.Record.AffectFlags = who.Record.AffectFlags.With(game.AffectCharm)
		}
	})

	c.send("order dog smile")
	c.expect("Your superior would not aprove of you giving orders.")
}

// Walking costs movement points (issue #189). do_simple_move charges
// need_movement — the truncated average of the two rooms' movement loss
// (act.movement.c:127) — and refuses when there are not enough. The port used
// to charge nothing, so the number on the prompt never moved.
//
// Every one of these demotes the character first. The first character on an
// empty roster is promoted to Implementor, and the charge is guarded on
// GET_LEVEL(ch) < LVL_IMMORT (act.movement.c:160) — an immortal walks for
// nothing, so an undemoted fixture would prove nothing at all.
func TestWalkingCostsMovementPoints(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Walker", "onefoot", "m", "w")
	demote(t, srv, "Walker", 10)

	// Both test rooms are SECT_INSIDE, so a step is (1+1)/2 = 1.
	before := moveOf(t, srv, "Walker")
	c.send("south")
	c.expect("The Temple Of Midgaard")
	if got := moveOf(t, srv, "Walker"); got != before-1 {
		t.Errorf("after one step the walker has %d movement, want %d", got, before-1)
	}

	c.send("north")
	// The board room is already in the transcript — it is where the character
	// logged in — so `expect` would match that and read the movement points
	// back before the step had happened. expectCount is the barrier.
	c.expectCount("The Immortal Board Room", 2)
	if got := moveOf(t, srv, "Walker"); got != before-2 {
		t.Errorf("after two steps the walker has %d movement, want %d", got, before-2)
	}
}

// An immortal is charged nothing, which is the other half of that guard.
func TestAnImmortalWalksForNothing(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Deity", "onefoot", "m", "w")

	before := moveOf(t, srv, "Deity")
	c.send("south")
	c.expect("The Temple Of Midgaard")
	if got := moveOf(t, srv, "Deity"); got != before {
		t.Errorf("an immortal walked and has %d movement, want %d", got, before)
	}
}

// And a mortal runs out. The refusal is on `!IS_NPC(ch)` alone
// (act.movement.c:130), so what immortals are exempt from is the charge and
// not this.
func TestWalkingWithNoMovementLeftIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Weary", "nolegs", "m", "w")
	demote(t, srv, "Weary", 10)
	setMove(t, srv, "Weary", 0)

	c.send("south")
	c.expect("You are too exhausted.")

	if got := roomOf(t, srv, "Weary"); got != ImmortStartRoom {
		t.Errorf("the exhausted walker is in room %d, want to have stayed put", got)
	}
}

// A follower dragged along by a leader gets the other wording, because
// perform_move passes need_specials_check = 1 for followers
// (act.movement.c:233) and do_simple_move picks its message on that and on
// having a master.
func TestAnExhaustedFollowerIsToldTheyCannotFollow(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, bobClient := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)
	inWorld(t, srv, func(w *game.Live) {
		bob.Record.Points.Move = 0
		w.AddFollower(bob, w.Find("Zod"))
	})

	c.send("south")
	c.expect("The Temple Of Midgaard")
	// The follower's lines are written on the world goroutine, and seeing
	// Zod's own reply says nothing about them. inWorld is the barrier.
	var stayed bool
	inWorld(t, srv, func(_ *game.Live) { stayed = bob.Room == ImmortStartRoom })

	if !bobClient.said("You are too exhausted to follow.") {
		t.Error("the exhausted follower was not told they could not follow")
	}
	if !stayed {
		t.Error("the exhausted follower was dragged along anyway")
	}
}

// demote takes a character back down to a mortal level, which the first
// character on an empty roster is not.
func demote(t *testing.T, srv *Server, name string, level int32) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil || who.Record == nil {
			t.Errorf("no character called %s", name)
			return
		}
		who.Record.Level = level
	})
}

// moveOf reads a character's movement points.
func moveOf(t *testing.T, srv *Server, name string) int32 {
	t.Helper()
	var n int32
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find(name); who != nil && who.Record != nil {
			n = who.Record.Points.Move
		}
	})
	return n
}

// setMove sets them.
func setMove(t *testing.T, srv *Server, name string, move int32) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil || who.Record == nil {
			t.Errorf("no character called %s", name)
			return
		}
		who.Record.Points.Move = move
	})
}
