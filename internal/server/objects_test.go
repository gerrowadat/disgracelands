// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// drop puts an object into a room for a test.
func drop(t *testing.T, srv *Server, vnum game.ObjVnum, room game.RoomVnum) *game.Object {
	t.Helper()

	var obj *game.Object
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		obj = w.NewObject(vnum)
		if obj == nil {
			t.Errorf("no prototype %d in the test world", vnum)
			return
		}
		w.ObjectToRoom(obj, room)
	}); err != nil {
		t.Fatal(err)
	}
	return obj
}

// TestGetAndDrop, and the room being told about both.
func TestGetAndDrop(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	sword := drop(t, srv, testSwordVnum, ImmortStartRoom)

	c.send("look")
	if got := c.expect("long sword"); !strings.Contains(got, "A long sword is lying here.") {
		t.Errorf("the sword is not shown on the floor:\n%s", got)
	}

	c.send("get sword")
	c.expect("You get a long sword.")

	if sword.Location != game.CarriedBy {
		t.Errorf("the sword is %v, want carried", sword.Location)
	}

	c.send("inventory")
	if got := c.expect("You are carrying:"); !strings.Contains(got, "a long sword") {
		t.Errorf("the sword is not in the inventory:\n%s", got)
	}

	c.send("drop sword")
	c.expect("You drop a long sword.")
	if sword.Location != game.InRoom {
		t.Errorf("the sword is %v, want on the floor", sword.Location)
	}
}

// TestGettingSomethingThatIsNotThere.
func TestGettingSomethingThatIsNotThere(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("get sword")
	c.expect("You don't see a sword here.")

	c.send("get")
	c.expect("Get what?")

	// The article follows the word, as the C's AN macro does.
	c.send("get apple")
	c.expect("You don't see an apple here.")
}

// TestSomeThingsCannotBeTaken: no ITEM_WEAR_TAKE.
func TestSomeThingsCannotBeTaken(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testFountainVnum, ImmortStartRoom)

	c.send("get fountain")
	c.expect("you can't take that!")
}

// TestWearAndRemove, including that a second ring goes on the other hand.
func TestWearAndRemove(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testRingVnum, ImmortStartRoom)
	drop(t, srv, testRingVnum, ImmortStartRoom)

	c.send("get ring")
	c.expect("You get a gold ring.")
	c.send("get ring")
	c.expectCount("You get a gold ring.", 2)

	c.send("wear ring")
	c.expect("You slide a gold ring on to your right ring finger.")

	// The second goes on the left hand, not into an occupied slot.
	c.send("wear ring")
	c.expect("You slide a gold ring on to your left ring finger.")

	c.send("equipment")
	got := c.expect("You are using:")
	if strings.Count(got, "a gold ring") < 2 {
		t.Errorf("both rings are not listed:\n%s", got)
	}

	c.send("remove ring")
	c.expect("You stop using a gold ring.")
}

// TestBothRingFingersFull says so rather than saying the ring is unwearable.
func TestBothRingFingersFull(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	for i := 0; i < 3; i++ {
		drop(t, srv, testRingVnum, ImmortStartRoom)
		c.send("get ring")
		c.expectCount("You get a gold ring.", i+1)
	}

	c.send("wear ring")
	c.expect("right ring finger")
	c.send("wear ring")
	c.expect("left ring finger")

	c.send("wear ring")
	c.expect("You're already wearing something on both of your ring fingers.")
}

// TestWieldingAWeaponAndNotAWeapon.
func TestWieldingAWeaponAndNotAWeapon(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testSwordVnum, ImmortStartRoom)
	drop(t, srv, testRingVnum, ImmortStartRoom)

	c.send("get sword")
	c.expect("You get a long sword.")
	c.send("get ring")
	c.expect("You get a gold ring.")

	c.send("wield sword")
	c.expect("You wield a long sword.")

	c.send("wield ring")
	c.expect("You can't wield that.")

	c.send("remove sword")
	c.expect("You stop using a long sword.")
}

// TestAWieldedWeaponIsUsedInCombat, which is the point of all this.
func TestAWieldedWeaponIsUsedInCombat(t *testing.T) {
	srv, _ := newTestServer(t)

	attacker, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	attacker.Record.Points.HitRoll = 100
	victim, _ := place(t, srv, fighterRecord("a large dog", 5, 100000), MortalStartRoom)
	victim.NPC = true

	// A weapon doing 20d10 is unmistakable against bare hands.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		sword := w.NewObject(testSwordVnum)
		sword.Values[1] = 20
		sword.Values[2] = 10
		w.ObjectToChar(sword, attacker)
		if !w.Equip(sword, attacker, game.WearWield) {
			t.Error("could not wield the sword")
		}
		w.SetFighting(attacker, victim)
	}); err != nil {
		t.Fatal(err)
	}

	before := victim.Record.Points.Hit
	round(t, srv)
	dealt := before - victim.Record.Points.Hit

	// Bare-handed the most this attacker could do is strength todam plus
	// number(0, 2); with the weapon the minimum is 20.
	if dealt < 20 {
		t.Errorf("dealt %d damage with a 20d10 weapon, want at least 20", dealt)
	}
}

// TestCarryLimits: the count from CAN_CARRY_N and the weight from the
// strength table.
func TestCarryLimits(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// An implementor has dexterity 25 and level 34, so the count limit is
	// 5 + 12 + 17 = 34. The weight limit at strength 25 is 1750, and each
	// sword weighs 10 — so the count runs out first.
	for i := 0; i < 40; i++ {
		drop(t, srv, testSwordVnum, ImmortStartRoom)
	}
	for i := 0; i < 34; i++ {
		c.send("get sword")
	}
	c.expectCount("You get a long sword.", 34)

	c.send("get sword")
	c.expect("You can't carry that many items.")
}

// TestMovementAbbreviationsStillWin. The command table matches the first
// entry a typed word is a prefix of, so adding `get`, `drop`, `wear` and
// `wield` could quietly steal `g`, `d`, `w` and `e` from the directions —
// which is twenty years of muscle memory.
func TestMovementAbbreviationsStillWin(t *testing.T) {
	for word, want := range map[string]string{
		"n": "north", "e": "east", "s": "south", "w": "west",
		"u": "up", "d": "down",
		"g": "get", "i": "inventory", "eq": "equipment",
		"wea": "wear", "wie": "wield", "r": "remove",
		"l": "look", "k": "kill", "q": "quit",
	} {
		cmd := session.Lookup(word)
		if cmd == nil {
			t.Errorf("%q matches nothing", word)
			continue
		}
		if cmd.Name != want {
			t.Errorf("%q is %s, want %s", word, cmd.Name, want)
		}
	}
}
