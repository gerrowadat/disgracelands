// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// TestPuttingThingsInABagAndTakingThemOut, which is the whole point.
func TestPuttingThingsInABagAndTakingThemOut(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bag := drop(t, srv, testBagVnum, ImmortStartRoom)
	sword := drop(t, srv, testSwordVnum, ImmortStartRoom)

	c.send("get bag")
	c.expect("You get a bag.")
	c.send("get sword")
	c.expect("You get a long sword.")

	// The preposition is a fill word, so this is the same command as
	// `put sword bag`.
	c.send("put sword in bag")
	c.expect("You put a long sword in a bag.")

	inWorld(t, srv, func(_ *game.Live) {
		if sword.ContainerOf() != bag {
			t.Errorf("the sword is not in the bag: %T", sword.Placement())
		}
	})

	// It is no longer carried directly, so the inventory shows only the bag.
	c.send("inventory")
	got := c.expect("You are carrying:")
	if strings.Contains(got[strings.LastIndex(got, "You are carrying:"):], "long sword") {
		t.Errorf("the sword is still listed in the inventory:\n%s", got)
	}

	c.send("examine bag")
	if got := c.expect("When you look inside"); !strings.Contains(got, "a long sword") {
		t.Errorf("the bag does not show its contents:\n%s", got)
	}

	c.send("get sword from bag")
	c.expect("You get a long sword from a bag.")

	inWorld(t, srv, func(_ *game.Live) {
		if _, ok := sword.Placement().(game.CarriedBy); !ok {
			t.Errorf("the sword is %T, want carried", sword.Placement())
		}
	})
}

// TestABagWillNotHoldEverything: the capacity check counts the container's own
// weight, which is the C's arithmetic and not anybody's intent.
func TestABagWillNotHoldEverything(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testBagVnum, ImmortStartRoom)
	c.send("get bag")
	c.expect("You get a bag.")

	// The bag weighs 2 and holds 100, so it takes nine ten-pound swords and
	// refuses the tenth: 2 + 90 + 10 > 100.
	for i := 0; i < 10; i++ {
		drop(t, srv, testSwordVnum, ImmortStartRoom)
		c.send("get sword")
		c.expectCount("You get a long sword.", i+1)
		c.send("put sword bag")
	}
	c.expectCount("You put a long sword in a bag.", 9)
	c.expect("A long sword won't fit in a bag.")
}

// TestGettingEverythingOutOfAContainer, in both the `all` and `all.thing`
// forms.
func TestGettingEverythingOutOfAContainer(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		bag := w.NewObject(testBagVnum)
		w.ObjectToRoom(bag, ImmortStartRoom)
		for i := 0; i < 3; i++ {
			w.ObjectToObject(w.NewObject(testRingVnum), bag)
		}
		w.ObjectToObject(w.NewObject(testSwordVnum), bag)
	})

	c.send("get all.ring bag")
	c.expectCount("You get a gold ring from a bag.", 3)

	// The sword is still there, and `all` takes it.
	c.send("get all bag")
	c.expect("You get a long sword from a bag.")

	c.send("get all bag")
	c.expect("A bag seems to be empty.")

	c.send("get all.ring bag")
	c.expect("You can't seem to find any rings in a bag.")
}

// TestGettingACountedNumberOfThings: `get 2 ring bag` takes two and leaves the
// third.
func TestGettingACountedNumberOfThings(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	var bag *game.Object
	inWorld(t, srv, func(w *game.Live) {
		bag = w.NewObject(testBagVnum)
		w.ObjectToRoom(bag, ImmortStartRoom)
		for i := 0; i < 3; i++ {
			w.ObjectToObject(w.NewObject(testRingVnum), bag)
		}
	})

	c.send("get 2 ring bag")
	c.expectCount("You get a gold ring from a bag.", 2)

	inWorld(t, srv, func(_ *game.Live) {
		if len(bag.Contents) != 1 {
			t.Errorf("the bag holds %d rings, want 1", len(bag.Contents))
		}
	})
}

// TestAClosedContainerKeepsItsContents.
func TestAClosedContainerKeepsItsContents(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		chest := w.NewObject(testChestVnum)
		w.ObjectToRoom(chest, ImmortStartRoom)
		w.ObjectToObject(w.NewObject(testRingVnum), chest)
	})
	drop(t, srv, testSwordVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")

	c.send("get ring chest")
	c.expect("A wooden chest is closed.")

	c.send("put sword chest")
	c.expect("You'd better open it first!")

	// Locked, so it will not open until it is unlocked. The first character
	// on the roster is an Implementor and needs no key.
	c.send("open chest")
	c.expect("It seems to be locked.")
	c.send("unlock chest")
	c.expect("*Click*")
	c.send("open chest")
	c.expect("Okay.")

	c.send("get ring chest")
	c.expect("You get a gold ring from a wooden chest.")
	c.send("put sword chest")
	c.expect("You put a long sword in a wooden chest.")

	c.send("close chest")
	c.expectCount("Okay.", 2)
	c.send("close chest")
	c.expect("But it's already closed!")
}

// TestSomethingThatIsNotAContainer.
func TestSomethingThatIsNotAContainer(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testSwordVnum, ImmortStartRoom)
	drop(t, srv, testRingVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")

	c.send("put sword ring")
	c.expect("A gold ring is not a container.")

	c.send("get ring sword")
	c.expect("A long sword is not a container.")

	c.send("put sword chalice")
	c.expect("You don't see a chalice here.")

	c.send("put")
	c.expect("Put what in what?")

	c.send("put sword")
	c.expect("What do you want to put it in?")

	c.send("put all")
	c.expect("What do you want to put them in?")
}

// TestPuttingABagIntoItself, which the C catches with a message of its own.
func TestPuttingABagIntoItself(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testBagVnum, ImmortStartRoom)
	c.send("get bag")
	c.expect("You get a bag.")

	c.send("put bag bag")
	c.expect("You attempt to fold it into itself, but fail.")

	// And `put all bag` must not put the bag into itself either — the C skips
	// it explicitly, and without that it is an object graph with a cycle.
	drop(t, srv, testSwordVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")
	c.send("put all bag")
	c.expect("You put a long sword in a bag.")
}

// TestPickingUpCoinsTurnsThemIntoGold, and says how many there were.
func TestPickingUpCoinsTurnsThemIntoGold(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		w.ObjectToRoom(w.MakeMoney(150), ImmortStartRoom)
	})

	var before int32
	inWorld(t, srv, func(w *game.Live) { before = w.Find("Zod").Record.Points.Gold })

	// The pile is described, never counted, until it is picked up.
	c.send("look")
	c.expect("A small pile of gold coins is lying here.")

	c.send("get coins")
	c.expect("There were 150 coins.")

	inWorld(t, srv, func(w *game.Live) {
		if got := w.Find("Zod").Record.Points.Gold; got != before+150 {
			t.Errorf("gold is %d, want %d", got, before+150)
		}
	})

	// And the coins are gone rather than carried: get_check_money extracts
	// the object, which is why gold never appears in an inventory.
	inWorld(t, srv, func(w *game.Live) {
		if carrying := w.Find("Zod").Carrying; len(carrying) != 0 {
			t.Errorf("carrying %d objects after picking up coins, want none", len(carrying))
		}
	})
}

// TestDroppingGold, which costs a wait state to stop coin-bombing.
func TestDroppingGold(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	inWorld(t, srv, func(w *game.Live) { w.Find("Zod").Record.Points.Gold = 500 })

	c.send("drop 1000 coins")
	c.expect("You don't have that many coins!")

	c.send("drop 0 coins")
	c.expect("Heh heh heh.. we are jolly funny today, eh?")

	c.send("drop 200 coins")
	c.expect("You drop some gold.")

	inWorld(t, srv, func(w *game.Live) {
		if got := w.Find("Zod").Record.Points.Gold; got != 300 {
			t.Errorf("gold is %d, want 300", got)
		}
	})
}

// TestGivingSomethingToSomebody, and everybody being told.
func TestGivingSomethingToSomebody(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, bobClient := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)

	sword := drop(t, srv, testSwordVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")

	c.send("give sword bob")
	c.expect("You give a long sword to Bob.")

	inWorld(t, srv, func(_ *game.Live) {
		if sword.HolderOf() != bob {
			t.Error("the sword did not reach Bob")
		}
	})
	if !bobClient.said("Zod gives you a long sword.") {
		t.Error("Bob was not told he had been given anything")
	}
}

// TestGivingToNobodyAndToYourself.
func TestGivingToNobodyAndToYourself(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testSwordVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")

	c.send("give")
	c.expect("Give what to who?")

	c.send("give sword")
	c.expect("To who?")

	c.send("give sword nobody")
	c.expect("No-one by that name here.")

	c.send("give sword zod")
	c.expect("What's the point of that?")
}

// TestGivingCoins, which is a different command from giving an object even
// though it is spelled the same way.
func TestGivingCoins(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, bobClient := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)

	inWorld(t, srv, func(w *game.Live) { w.Find("Zod").Record.Points.Gold = 0 })

	c.send("give 25 coins bob")
	c.expect("Okay.")
	c.settle()

	if !bobClient.said("Zod gives you 25 gold coins.") {
		t.Error("Bob was not told about the money")
	}
	inWorld(t, srv, func(w *game.Live) {
		if bob.Record.Points.Gold != 25 {
			t.Errorf("Bob has %d gold, want 25", bob.Record.Points.Gold)
		}
		// An Implementor's gold is not deducted: the C checks the level both
		// when deciding whether they can afford it and when charging them.
		if got := w.Find("Zod").Record.Points.Gold; got != 0 {
			t.Errorf("the Implementor was charged %d gold", -got)
		}
	})
}

// TestCursedThingsStayWithYou. ITEM_NODROP is the curse flag, and it stops
// dropping and giving — but not putting things in a bag, which instead curses
// the bag.
func TestCursedThingsStayWithYou(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)

	sword := drop(t, srv, testSwordVnum, ImmortStartRoom)
	bag := drop(t, srv, testBagVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")
	c.send("get bag")
	c.expect("You get a bag.")
	inWorld(t, srv, func(_ *game.Live) {
		sword.ExtraFlags = sword.ExtraFlags.With(game.ItemNoDrop)
	})

	c.send("drop sword")
	c.expect("You can't drop a long sword, it must be CURSED!")

	c.send("give sword bob")
	c.expect("You can't let go of a long sword!!  Yeech!")

	c.send("put sword bag")
	c.expect("You get a strange feeling as you put a long sword in a bag.")

	inWorld(t, srv, func(_ *game.Live) {
		if !bag.ExtraFlags.Has(game.ItemNoDrop) {
			t.Error("the curse did not spread to the bag")
		}
	})
	c.send("drop bag")
	c.expect("You can't drop a bag, it must be CURSED!")
}

// TestDroppingEverything and its counted form.
func TestDroppingEverything(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	for i := 0; i < 3; i++ {
		drop(t, srv, testRingVnum, ImmortStartRoom)
		c.send("get ring")
		c.expectCount("You get a gold ring.", i+1)
	}

	c.send("drop 2 ring")
	c.expectCount("You drop a gold ring.", 2)

	c.send("drop all")
	c.expectCount("You drop a gold ring.", 3)

	c.send("drop all")
	c.expect("You don't seem to be carrying anything.")

	c.send("drop all.ring")
	c.expect("You don't seem to have any rings.")

	c.send("drop")
	c.expect("What do you want to drop?")
}

// TestPutAndGiveTakeTheirPlacesInTheCommandTable. `p` has meant put since
// 1993 and `g` has meant get; adding two commands beginning with those
// letters is exactly how that gets broken.
func TestPutAndGiveTakeTheirPlacesInTheCommandTable(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testBagVnum, ImmortStartRoom)

	c.send("g bag")
	c.expect("You get a bag.")

	// `p` is put (interpreter.c:396), ahead of pick, pour and practice.
	c.send("p bag bag")
	c.expect("You attempt to fold it into itself, but fail.")

	// `gi` is give, and `g` is not.
	c.send("gi")
	c.expect("Give what to who?")
}
