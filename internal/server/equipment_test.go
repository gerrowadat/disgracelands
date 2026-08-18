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

// TestWornArmourShowsInTheScore, which is the point of all of it: until now a
// suit of plate mail was a place to put an object and nothing more.
func TestWornArmourShowsInTheScore(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	var before int32
	inWorld(t, srv, func(w *game.Live) {
		before = game.ComputeArmorClass(w.Find("Zod").Record, nil)
	})

	// Armour worth 5, on the body, is worth three times that.
	drop(t, srv, testPlateVnum, ImmortStartRoom)
	c.send("get plate")
	c.expect("You get a suit of plate mail.")
	c.send("wear plate")
	c.expect("You wear a suit of plate mail on your body.")

	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Zod").Record
		if got := game.ComputeArmorClass(rec, nil); got != before-15 {
			t.Errorf("armour class is %d, want %d", got, before-15)
		}
		// The applies arrived too.
		if rec.Points.HitRoll != 2 {
			t.Errorf("hitroll is %d, want the plate's +2", rec.Points.HitRoll)
		}
	})

	c.send("remove plate")
	c.expect("You stop using a suit of plate mail.")

	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Zod").Record
		if got := game.ComputeArmorClass(rec, nil); got != before {
			t.Errorf("armour class is %d after taking it off, want %d", got, before)
		}
		if rec.Points.HitRoll != 0 {
			t.Errorf("hitroll is %d after taking it off, want 0", rec.Points.HitRoll)
		}
	})
}

// TestAnObjectYouAreNotExperiencedEnoughFor. The level check is local to this
// tree, and it is worded two different ways depending on how you got there.
func TestAnObjectYouAreNotExperiencedEnoughFor(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	plate := drop(t, srv, testPlateVnum, ImmortStartRoom)
	c.send("get plate")
	c.expect("You get a suit of plate mail.")

	// Above an implementor's level, so nobody can wear it.
	inWorld(t, srv, func(w *game.Live) {
		_ = w
		plate.Def.MinLevel = 100
	})
	t.Cleanup(func() { plate.Def.MinLevel = 0 })

	c.send("wear plate")
	c.expect("You are not experienced enough to use that.")

	// By way of `all` it is perform_wear's wording instead.
	c.send("wear all")
	c.expect("You aren't experienced enough to use a suit of plate mail.")
}

// TestAnObjectThatZapsYou: worn, announced, and thrown straight back.
func TestAnObjectThatZapsYou(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	plate := drop(t, srv, testPlateVnum, ImmortStartRoom)
	c.send("get plate")
	c.expect("You get a suit of plate mail.")

	// A new character is neutral, so anti-neutral kit rejects them.
	inWorld(t, srv, func(_ *game.Live) {
		plate.ExtraFlags = plate.ExtraFlags.Set(game.ItemAntiNeutral)
	})

	c.send("wear plate")
	// The C says it went on and then says it did not, in that order.
	got := c.expect("You are zapped by a suit of plate mail and instantly let go of it.")
	if !strings.Contains(got, "You wear a suit of plate mail on your body.") {
		t.Errorf("the wear message did not come first:\n%s", got)
	}

	inWorld(t, srv, func(w *game.Live) {
		zod := w.Find("Zod")
		if zod.Equipment[game.WearBody] != nil {
			t.Error("the plate stayed on")
		}
		if plate.Location != game.CarriedBy {
			t.Errorf("the plate is %v, want back in the inventory", plate.Location)
		}
	})
}

// TestWearingEverything, and the two ways of naming a place.
func TestWearingEverything(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testPlateVnum, ImmortStartRoom)
	drop(t, srv, testRingVnum, ImmortStartRoom)
	drop(t, srv, testSwordVnum, ImmortStartRoom)
	for _, word := range []string{"plate", "ring", "sword"} {
		c.send("get " + word)
	}
	c.expect("You get a long sword.")

	c.send("wear all")
	got := c.expect("You slide a gold ring on to your right ring finger.")
	if !strings.Contains(got, "You wear a suit of plate mail on your body.") {
		t.Errorf("`wear all` missed the plate:\n%s", got)
	}
	// A sword is not wearable, so `wear all` passes over it in silence.
	if strings.Contains(got, "wield") {
		t.Errorf("`wear all` wielded the sword:\n%s", got)
	}

	c.send("wear all")
	c.expect("You don't seem to have anything wearable.")

	// A named place, and a place that is not one.
	c.send("remove ring")
	c.expect("You stop using a gold ring.")
	c.send("wear ring finger")
	c.expectCount("You slide a gold ring on to your right ring finger.", 2)

	c.send("remove ring")
	c.expectCount("You stop using a gold ring.", 2)

	c.send("wear ring elbow")
	c.expect("'elbow'?  What part of your body is THAT?")

	// And a place the object does not fit.
	c.send("wear ring head")
	c.expect("You can't wear a gold ring there.")
}

// TestHoldingThings. `hold` and `grab` are one command, and a light goes in
// the light slot rather than the hand.
func TestHoldingThings(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testTorchVnum, ImmortStartRoom)
	drop(t, srv, testSwordVnum, ImmortStartRoom)
	c.send("get torch")
	c.expect("You get a torch.")
	c.send("get sword")
	c.expect("You get a long sword.")

	c.send("hold torch")
	c.expect("You light a torch and hold it.")

	inWorld(t, srv, func(w *game.Live) {
		if w.Find("Zod").Equipment[game.WearLight] == nil {
			t.Error("the torch is not in the light slot")
		}
	})

	// A sword can be wielded but not held.
	c.send("grab sword")
	c.expect("You can't hold that.")

	// `h` is help, not hold — the C's table order.
	c.send("h")
	c.expect("Commands")
}

// TestWearingSomethingUnwearable.
func TestWearingSomethingUnwearable(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	drop(t, srv, testTorchVnum, ImmortStartRoom)
	c.send("get torch")
	c.expect("You get a torch.")

	// find_eq_pos has no entry for the light slot, so a torch has to be
	// held rather than worn.
	c.send("wear torch")
	c.expect("You can't wear a torch.")

	c.send("wear")
	c.expect("Wear what?")
}
