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

// giveFood puts a piece of food in a character's hands.
func giveFood(t *testing.T, srv *Server, who string, filling, poison int32) *game.Object {
	t.Helper()

	var food *game.Object
	inWorld(t, srv, func(w *game.Live) {
		food = w.NewBareObject()
		food.Keywords = "bread"
		food.ShortDesc = "a loaf of bread"
		food.Type = game.ItemFood
		food.WearFlags = game.NewSet(game.ItemWearTake)
		food.Values[0] = filling
		food.Values[3] = poison
		w.ObjectToChar(food, w.Find(who))
	})
	return food
}

// giveDrink puts a filled container in their hands.
func giveDrink(t *testing.T, srv *Server, who string, liquid game.Liquid, units int32) *game.Object {
	t.Helper()

	var vessel *game.Object
	inWorld(t, srv, func(w *game.Live) {
		vessel = w.NewBareObject()
		vessel.Keywords = "bottle"
		vessel.ShortDesc = "a bottle"
		vessel.Type = game.ItemDrinkCon
		vessel.WearFlags = game.NewSet(game.ItemWearTake)
		vessel.Weight = units
		vessel.Values[0] = units
		vessel.Values[1] = units
		vessel.Values[2] = liquid.Number()
		// A full container carries the liquid's keyword, as the loader and
		// every pour give it: `drink beer` has to find the bottle.
		game.NameToDrinkCon(vessel, liquid)
		w.ObjectToChar(vessel, w.Find(who))
	})
	return vessel
}

// hungryCharacter drops a character's conditions so they can eat and drink.
func hungryCharacter(t *testing.T, srv *Server, who string) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find(who).Record
		rec.Conditions = [3]int32{0, 0, 0}
	})
}

// TestEatingFillsYouUp, and the food is gone afterwards.
func TestEatingFillsYouUp(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")
	hungryCharacter(t, srv, "Zod")

	food := giveFood(t, srv, "Zod", 8, 0)

	c.send("eat bread")
	c.expect("You eat a loaf of bread.")

	inWorld(t, srv, func(w *game.Live) {
		if got := w.Find("Zod").Record.Conditions[game.CondFull]; got != 8 {
			t.Errorf("fullness is %d, want 8", got)
		}
		if food.Placement() != nil {
			t.Errorf("the bread is still %T", food.Placement())
		}
	})
}

// TestYouCannotEatASword.
func TestYouCannotEatASword(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// A mortal: the C lets a god eat anything.
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "w")
	hungryCharacter(t, srv, "Welmar")

	drop(t, srv, testSwordVnum, MortalStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")

	c.send("eat sword")
	c.expect("You can't eat THAT!")

	c.send("eat bread")
	c.expect("You don't seem to have a bread.")

	c.send("eat")
	c.expect("Eat what?")
}

// TestAFullStomachRefusesMore.
func TestAFullStomachRefusesMore(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// An Implementor's conditions are -1 — they never eat — so give this one
	// an ordinary stomach, nearly full.
	inWorld(t, srv, func(w *game.Live) {
		w.Find("Zod").Record.Conditions = [3]int32{0, 24, 24}
	})
	giveFood(t, srv, "Zod", 8, 0)

	c.send("eat bread")
	c.expect("You are too full to eat more!")
}

// TestPoisonedFood, which is what value 3 is for.
func TestPoisonedFood(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "w")
	hungryCharacter(t, srv, "Welmar")
	giveFood(t, srv, "Welmar", 4, 1)

	c.send("eat bread")
	c.expect("Oops, that tasted rather strange!")

	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Welmar").Record
		if !rec.AffectFlags.Has(game.AffectPoison) {
			t.Error("poisoned food did not poison them")
		}
	})
}

// TestTastingTakesOneBiteAndLeavesTheRest.
func TestTastingTakesOneBiteAndLeavesTheRest(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")
	hungryCharacter(t, srv, "Zod")

	food := giveFood(t, srv, "Zod", 3, 0)

	c.send("taste bread")
	c.expect("You nibble a little bit of a loaf of bread.")

	inWorld(t, srv, func(w *game.Live) {
		if food.Values[0] != 2 {
			t.Errorf("the bread has %d bites left, want 2", food.Values[0])
		}
		if w.Find("Zod").Record.Conditions[game.CondFull] != 1 {
			t.Error("a taste should be worth one point of fullness")
		}
	})

	c.send("taste bread")
	c.expectCount("You nibble", 2)
	c.send("taste bread")
	c.expect("There's nothing left now.")
}

// TestDrinkingQuenchesThirst. Water is ten thirst per four units.
func TestDrinkingQuenchesThirst(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")
	hungryCharacter(t, srv, "Zod")

	vessel := giveDrink(t, srv, "Zod", 0, 20) // water

	c.send("drink bottle")
	c.expect("You drink the water.")

	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Zod").Record
		if rec.Conditions[game.CondThirst] <= 0 {
			t.Error("drinking water did not quench any thirst")
		}
		if vessel.Values[1] >= 20 {
			t.Errorf("the bottle still holds %d units", vessel.Values[1])
		}
	})
}

// TestSaltWaterMakesYouThirstier, which is the trap in the table.
func TestSaltWaterMakesYouThirstier(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		w.Find("Zod").Record.Conditions = [3]int32{0, 0, 20}
	})
	giveDrink(t, srv, "Zod", 14, 40) // salt water

	c.send("drink bottle")
	c.expect("You drink the salt water.")

	inWorld(t, srv, func(w *game.Live) {
		if got := w.Find("Zod").Record.Conditions[game.CondThirst]; got >= 20 {
			t.Errorf("thirst is %d, want less than the 20 they started with", got)
		}
	})
}

// TestAnEmptyBottle and the other refusals.
func TestDrinkRefusals(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")
	hungryCharacter(t, srv, "Zod")

	empty := giveDrink(t, srv, "Zod", 0, 0)
	inWorld(t, srv, func(w *game.Live) { empty.Values[1] = 0 })

	c.send("drink bottle")
	c.expect("It's empty.")

	c.send("drink")
	c.expect("Drink from what?")

	c.send("drink nothing")
	c.expect("You can't find it!")

	// A sword is not a drink.
	drop(t, srv, testSwordVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")
	c.send("drink sword")
	c.expect("You can't drink from that!")
}

// TestABottleOnTheFloorMustBeHeld, but a fountain need not be.
func TestABottleOnTheFloorMustBeHeld(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")
	hungryCharacter(t, srv, "Zod")

	inWorld(t, srv, func(w *game.Live) {
		bottle := w.NewBareObject()
		bottle.Keywords = "bottle"
		bottle.ShortDesc = "a bottle"
		bottle.Type = game.ItemDrinkCon
		bottle.Values[1] = 10
		w.ObjectToRoom(bottle, ImmortStartRoom)

		fountain := w.NewBareObject()
		fountain.Keywords = "fountain"
		fountain.ShortDesc = "a fountain"
		fountain.Type = game.ItemFountain
		fountain.Values[1] = 1000
		w.ObjectToRoom(fountain, ImmortStartRoom)
	})

	c.send("drink bottle")
	c.expect("You have to be holding that to drink from it.")

	c.send("drink fountain")
	c.expect("You drink the water.")
}

// TestPouringEmptiesAContainer.
func TestPouringEmptiesAContainer(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	vessel := giveDrink(t, srv, "Zod", 1, 12) // beer

	// The C wants somewhere to put it: `pour bottle` on its own asks.
	c.send("pour bottle")
	c.expect("Where do you want it?  Out or in what?")

	c.send("pour bottle out")
	c.expect("You empty a bottle.")

	inWorld(t, srv, func(w *game.Live) {
		if vessel.Values[1] != 0 {
			t.Errorf("the bottle still holds %d units", vessel.Values[1])
		}
		// Emptying takes the liquid's keyword off the name, so it no longer
		// answers to `beer`.
		if vessel.Matches("beer") {
			t.Errorf("the empty bottle still answers to beer: %q", vessel.Keywords)
		}
	})

	c.send("pour bottle out")
	c.expect("The a bottle is empty.")
}

// TestDrinkTablesMatchTheC, spot-checked against constants.c's drink_aff.
func TestDrinkTables(t *testing.T) {
	for liquid, want := range map[game.Liquid][3]int32{
		0:  {0, 1, 10}, // water
		5:  {6, 1, 4},  // whisky
		7:  {10, 0, 0}, // firebreather
		9:  {0, 4, -8}, // slime mold juice
		14: {0, 1, -2}, // salt water
		15: {0, 0, 13}, // clear water
	} {
		if got := game.DrinkEffect(liquid); got != want {
			t.Errorf("drink %d (%s) is %v, want %v", liquid, game.DrinkName(liquid), got, want)
		}
	}

	if game.DrinkName(0) != "water" || game.DrinkName(7) != "firebreather" {
		t.Error("the drink names are wrong")
	}
	if game.DrinkName(99) != "something" {
		t.Error("an unknown liquid should be described vaguely")
	}
}
