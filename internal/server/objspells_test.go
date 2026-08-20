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

// TestBlessingAndCursingAnObject: mag_alter_objs, both directions.
func TestBlessingAndCursingAnObject(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	sword := drop(t, srv, testSwordVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")

	c.send("cast 'bless' sword")
	c.expect("A long sword glows briefly.")

	inWorld(t, srv, func(_ *game.Live) {
		if !sword.ExtraFlags.Has(game.ItemBless) {
			t.Error("the sword was not blessed")
		}
	})

	// Twice does nothing, and says nothing that distinguishes it from a
	// failure.
	c.send("cast 'bless' sword")
	c.expect("Nothing seems to happen.")

	// Cursing a weapon also files a point off its damage die.
	var before int32
	inWorld(t, srv, func(_ *game.Live) { before = sword.Values[2] })

	c.send("cast 'curse' sword")
	c.expect("A long sword briefly glows red.")

	inWorld(t, srv, func(_ *game.Live) {
		if !sword.ExtraFlags.Has(game.ItemNoDrop) {
			t.Error("the sword was not cursed")
		}
		if sword.Values[2] != before-1 {
			t.Errorf("the damage die is %d, want %d", sword.Values[2], before-1)
		}
	})

	c.send("cast 'remove curse' sword")
	c.expect("A long sword briefly glows blue.")

	inWorld(t, srv, func(_ *game.Live) {
		if sword.ExtraFlags.Has(game.ItemNoDrop) {
			t.Error("the curse stayed")
		}
		if sword.Values[2] != before {
			t.Errorf("the damage die is %d, want it back at %d", sword.Values[2], before)
		}
	})
}

// TestBlessingSomethingTooHeavy. The limit is five pounds per caster level,
// and it is silent about why.
func TestBlessingSomethingTooHeavy(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	plate := drop(t, srv, testPlateVnum, ImmortStartRoom)
	c.send("get plate")
	c.expect("You get a suit of plate mail.")

	// An implementor is level 34, so the limit is 170 and the plate weighs
	// 100 — bless it, then make it heavier and try again.
	c.send("cast 'bless' plate")
	c.expect("A suit of plate mail glows briefly.")

	inWorld(t, srv, func(_ *game.Live) {
		plate.ExtraFlags = plate.ExtraFlags.Clear(game.ItemBless)
		plate.Weight = 500
	})

	c.send("cast 'bless' plate")
	c.expect("Nothing seems to happen.")
}

// TestPoisoningAndCleaningADrink, which is mag_alter_objs on value 3 rather
// than on a flag.
func TestPoisoningAndCleaningADrink(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	bottle := giveDrink(t, srv, "Zod", 1, 12)

	c.send("cast 'poison' bottle")
	c.expect("A bottle steams briefly.")
	inWorld(t, srv, func(_ *game.Live) {
		if bottle.Values[3] == 0 {
			t.Error("the beer is not poisoned")
		}
	})

	c.send("cast 'remove poison' bottle")
	c.expectCount("A bottle steams briefly.", 2)
	inWorld(t, srv, func(_ *game.Live) {
		if bottle.Values[3] != 0 {
			t.Error("the beer is still poisoned")
		}
	})
}

// TestCreateWater fills a container — or turns what is in it to slime.
func TestCreateWater(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	// A bottle of beer: create water slimes it rather than filling it, and
	// says nothing at all about having done so.
	bottle := giveDrink(t, srv, "Zod", 1, 12)
	// It says nothing at all about having done this.
	c.send("cast 'create water' bottle")
	c.settle()

	inWorld(t, srv, func(_ *game.Live) {
		if bottle.Values[2] != game.LiquidSlime {
			t.Errorf("the beer is liquid %d, want slime", bottle.Values[2])
		}
		if !bottle.Matches("juice") {
			t.Errorf("the bottle does not answer to the slime's keyword: %q", bottle.Keywords)
		}
	})

	// An empty one is filled to the brim with water.
	c.send("pour bottle out")
	c.expect("You empty a bottle.")
	c.send("cast 'create water' bottle")
	c.expect("A bottle is filled.")

	inWorld(t, srv, func(_ *game.Live) {
		if bottle.Values[1] != bottle.Values[0] {
			t.Errorf("the bottle holds %d of %d", bottle.Values[1], bottle.Values[0])
		}
		if bottle.Values[2] != game.LiquidWater || !bottle.Matches("water") {
			t.Errorf("the bottle holds liquid %d, keywords %q", bottle.Values[2], bottle.Keywords)
		}
	})
}

// TestFillingFromAFountain, and the things it refuses.
func TestFillingFromAFountain(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	bottle := giveDrink(t, srv, "Zod", 1, 12)
	c.send("pour bottle out")
	c.expect("You empty a bottle.")

	// No fountain yet.
	c.send("fill bottle from fountain")
	c.expect("There doesn't seem to be a fountain here.")

	fountain := drop(t, srv, testFountainVnum, ImmortStartRoom)
	inWorld(t, srv, func(_ *game.Live) {
		fountain.Values[0] = 100
		fountain.Values[1] = 100
		fountain.Values[2] = game.LiquidWater
	})

	c.send("fill bottle from fountain")
	c.expect("You gently fill a bottle from a fountain.")

	inWorld(t, srv, func(_ *game.Live) {
		if bottle.Values[1] != bottle.Values[0] {
			t.Errorf("the bottle holds %d of %d", bottle.Values[1], bottle.Values[0])
		}
	})

	c.send("fill bottle from fountain")
	c.expect("There is no room for more.")
}

// TestPouringOneContainerIntoAnother.
func TestPouringOneContainerIntoAnother(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	full := giveDrink(t, srv, "Zod", 1, 12) // beer
	inWorld(t, srv, func(_ *game.Live) {
		full.Keywords = "flask beer"
		full.ShortDesc = "a flask"
	})
	empty := giveDrink(t, srv, "Zod", 0, 12)
	inWorld(t, srv, func(_ *game.Live) {
		empty.Values[1] = 0
		empty.Values[2] = 0
		game.NameFromDrinkCon(empty)
	})

	// `into` is not one of the C's filler words — only `in` is — so the
	// preposition here has to be the short one.
	c.send("pour flask in bottle")
	c.expect("You pour the beer into the bottle.")

	inWorld(t, srv, func(_ *game.Live) {
		if empty.Values[1] != 12 || empty.Values[2] != 1 {
			t.Errorf("the bottle holds %d units of liquid %d", empty.Values[1], empty.Values[2])
		}
		if full.Values[1] != 0 {
			t.Errorf("the flask still holds %d units", full.Values[1])
		}
		if !empty.Matches("beer") {
			t.Errorf("the bottle does not answer to beer: %q", empty.Keywords)
		}
	})

	// Into itself — and it has to be the full one, because an empty
	// container is refused before the C ever looks at where it is going.
	c.send("pour bottle in bottle")
	c.expect("A most unproductive effort.")
}

// TestEnchantWeapon, and the two things it silently refuses.
func TestEnchantWeapon(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	sword := drop(t, srv, testSwordVnum, ImmortStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")

	// A neutral caster's weapon glows yellow and takes no side.
	c.send("cast 'enchant weapon' sword")
	c.expect("A long sword glows yellow.")

	inWorld(t, srv, func(_ *game.Live) {
		if !sword.ExtraFlags.Has(game.ItemMagic) {
			t.Error("the sword is not magical")
		}
		// An implementor is level 34, so both bonuses are the higher one.
		want := []game.ObjAffect{
			{Location: game.ApplyHitRoll, Modifier: 2},
			{Location: game.ApplyDamRoll, Modifier: 2},
		}
		if len(sword.Affects) != 2 || sword.Affects[0] != want[0] || sword.Affects[1] != want[1] {
			t.Errorf("the sword's affects are %+v, want %+v", sword.Affects, want)
		}
	})

	// Already magical: nothing, and no message.
	c.send("cast 'enchant weapon' sword")
	c.settle()
}

// TestLocateObject finds every copy of a thing, up to level/2 of them.
func TestLocateObject(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	drop(t, srv, testSwordVnum, ImmortStartRoom)
	drop(t, srv, testSwordVnum, MortalStartRoom)
	c.send("get sword")
	c.expect("You get a long sword.")

	// The two lines arrive in whatever order the world's object map is
	// walked in, so wait for the command to finish rather than for either of
	// them.
	c.send("cast 'locate object' sword")
	c.settle()
	got := c.transcript()

	for _, want := range []string{
		"A long sword is being carried by Zod.",
		"A long sword is in The Temple Of Midgaard.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("locate object is missing %q:\n%s", want, got)
		}
	}
}

// TestWordOfRecallAndTeleport move people about.
func TestWordOfRecallAndTeleport(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	// The immortal board room is where a new implementor starts; recall
	// sends everyone to the temple.
	c.send("cast 'word of recall'")
	c.expect("The Temple Of Midgaard")

	inWorld(t, srv, func(w *game.Live) {
		if got := w.Find("Zod").Room; got != MortalStartRoom {
			t.Errorf("recall left them in room %d", got)
		}
	})

	// Teleport goes somewhere at random, so the test asserts that it went
	// *somewhere* rather than guessing which of the test world's rooms.
	var before game.RoomVnum
	inWorld(t, srv, func(w *game.Live) { before = w.Find("Zod").Room })

	c.send("cast 'teleport'")
	c.settle()

	inWorld(t, srv, func(w *game.Live) {
		if w.Room(w.Find("Zod").Room) == nil {
			t.Errorf("teleport left them in room %d, which does not exist",
				w.Find("Zod").Room)
		}
	})
	_ = before
}

// TestEarthquakeHitsTheRoom, and skips the caster.
func TestEarthquakeHitsTheRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	dog := spawnDog(t, srv, ImmortStartRoom)

	var dogBefore, casterBefore int32
	inWorld(t, srv, func(w *game.Live) {
		dogBefore = dog.Record.Points.Hit
		casterBefore = w.Find("Zod").Record.Points.Hit
	})

	c.send("cast 'earthquake'")
	c.expect("You gesture and the earth begins to shake all around you!")

	inWorld(t, srv, func(w *game.Live) {
		if dog.Record.Points.Hit >= dogBefore {
			t.Errorf("the dog is unhurt on %d hit points", dog.Record.Points.Hit)
		}
		if got := w.Find("Zod").Record.Points.Hit; got != casterBefore {
			t.Errorf("the caster took %d damage from their own earthquake",
				casterBefore-got)
		}
	})
}

// TestMagicDoesNotWorkEverywhere: the two room checks at the top of
// call_magic.
func TestMagicDoesNotWorkEverywhere(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	spawnDog(t, srv, ImmortStartRoom)

	inWorld(t, srv, func(w *game.Live) {
		w.Room(ImmortStartRoom).Flags = w.Room(ImmortStartRoom).Flags.Set(game.RoomNoMagic)
	})
	c.send("cast 'magic missile' dog")
	c.expect("Your magic fizzles out and dies.")

	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(ImmortStartRoom)
		room.Flags = room.Flags.Clear(game.RoomNoMagic).Set(game.RoomPeaceful)
	})
	c.send("cast 'magic missile' dog")
	c.expect("A flash of white light fills the room, dispelling your violent magic!")

	// A harmless spell still works in a peaceful room.
	c.send("cast 'armor'")
	c.expect("You feel someone protecting you.")
}

// TestDispelMagicStripsEverything, which is local to this tree and does not
// discriminate.
func TestDispelMagicStripsEverything(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	c.send("cast 'armor'")
	c.expect("You feel someone protecting you.")
	c.send("cast 'bless'")
	c.expect("You feel righteous.")

	inWorld(t, srv, func(w *game.Live) {
		if got := len(w.Find("Zod").Record.Affects); got < 2 {
			t.Errorf("expected at least two affects, got %d", got)
		}
	})

	c.send("cast 'dispel magic'")
	c.settle()

	inWorld(t, srv, func(w *game.Live) {
		if got := len(w.Find("Zod").Record.Affects); got != 0 {
			t.Errorf("%d affects survived dispel magic", got)
		}
	})
}
