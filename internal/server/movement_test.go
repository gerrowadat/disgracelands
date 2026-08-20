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
			room.Flags = room.Flags.Set(game.RoomIndoors)
		} else {
			room.Flags = room.Flags.Clear(game.RoomIndoors)
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
		dog.Record.AffectFlags = dog.Record.AffectFlags.Set(game.AffectCharm)
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
		dog.Record.AffectFlags = dog.Record.AffectFlags.Set(game.AffectCharm)
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
			who.Record.AffectFlags = who.Record.AffectFlags.Set(game.AffectCharm)
		}
	})

	c.send("order dog smile")
	c.expect("Your superior would not aprove of you giving orders.")
}
