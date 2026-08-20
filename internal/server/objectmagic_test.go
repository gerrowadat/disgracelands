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

// carry drops an object and picks it up, which is how most of these start.
func carry(t *testing.T, srv *Server, c *client, vnum game.ObjVnum, word, name string) *game.Object {
	t.Helper()

	obj := drop(t, srv, vnum, ImmortStartRoom)
	c.send("get " + word)
	c.expect("You get " + name + ".")
	return obj
}

// TestQuaffingAPotion, which casts on the drinker and destroys the potion.
func TestQuaffingAPotion(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	potion := carry(t, srv, c, testPotionVnum, "potion", "a potion of healing")
	inWorld(t, srv, func(w *game.Live) { w.Find("Zod").Record.Points.Hit = 100 })

	c.send("quaff potion")
	c.expect("You quaff a potion of healing.")

	inWorld(t, srv, func(w *game.Live) {
		if got := w.Find("Zod").Record.Points.Hit; got <= 100 {
			t.Errorf("still on %d hit points after a healing potion", got)
		}
		if potion.Location != game.InNowhere {
			t.Errorf("the potion is %v, want gone", potion.Location)
		}
	})

	c.send("quaff potion")
	c.expect("You don't seem to have a potion.")
}

// TestRecitingAScroll runs every spell on it, and the scroll dissolves.
func TestRecitingAScroll(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	scroll := carry(t, srv, c, testScrollVnum, "scroll", "a scroll of protection")

	c.send("recite scroll")
	c.expect("You recite a scroll of protection which dissolves.")

	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Zod").Record
		// Both spells on it landed: armor and bless.
		if !game.AffectedBySpell(rec, game.SpellArmor) {
			t.Error("the scroll's first spell did not land")
		}
		if !game.AffectedBySpell(rec, game.SpellBless) {
			t.Error("the scroll's second spell did not land")
		}
		if scroll.Location != game.InNowhere {
			t.Error("the scroll survived being read")
		}
	})
}

// TestAWandMustBeHeldAndPointed.
func TestAWandMustBeHeldAndPointed(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	wand := carry(t, srv, c, testWandVnum, "wand", "a wand of missiles")
	dog := spawnDog(t, srv, ImmortStartRoom)

	// `use` looks only at what is held — a wand in the pack is not usable.
	c.send("use wand")
	c.expect("You don't seem to be holding a wand.")

	c.send("hold wand")
	c.expect("You grab a wand of missiles.")

	c.send("use wand")
	c.expect("At what should a wand of missiles be pointed?")

	var before int32
	inWorld(t, srv, func(_ *game.Live) { before = dog.Record.Points.Hit })

	c.send("use wand dog")
	c.expect("You point a wand of missiles at a large dog.")

	inWorld(t, srv, func(_ *game.Live) {
		if dog.Record.Points.Hit >= before {
			t.Error("the wand did nothing to the dog")
		}
		if wand.Values[2] != 2 {
			t.Errorf("the wand has %d charges left, want 2", wand.Values[2])
		}
	})
}

// TestAnExhaustedWandSaysSo.
func TestAnExhaustedWandSaysSo(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	wand := carry(t, srv, c, testWandVnum, "wand", "a wand of missiles")
	spawnDog(t, srv, ImmortStartRoom)

	c.send("hold wand")
	c.expect("You grab a wand of missiles.")
	inWorld(t, srv, func(_ *game.Live) { wand.Values[2] = 0 })

	c.send("use wand dog")
	c.expect("It seems powerless.")
}

// TestAStaffHitsEverybodyElse, which is the difference between a staff and a
// wand.
func TestAStaffHitsEverybodyElse(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	carry(t, srv, c, testStaffVnum, "staff", "a staff of sleep")
	first := spawnDog(t, srv, ImmortStartRoom)
	second := spawnDog(t, srv, ImmortStartRoom)

	c.send("hold staff")
	c.expect("You grab a staff of sleep.")

	var beforeSelf int32
	inWorld(t, srv, func(w *game.Live) { beforeSelf = w.Find("Zod").Record.Points.Hit })

	c.send("use staff")
	c.expect("You tap a staff of sleep three times on the ground.")
	c.settle()

	inWorld(t, srv, func(_ *game.Live) {
		if first.Record.Points.Hit >= 500 || second.Record.Points.Hit >= 500 {
			t.Errorf("the staff left them on %d and %d hit points, want both hurt",
				first.Record.Points.Hit, second.Record.Points.Hit)
		}
	})
	inWorld(t, srv, func(w *game.Live) {
		if got := w.Find("Zod").Record.Points.Hit; got != beforeSelf {
			t.Errorf("the staff hit its own user for %d", beforeSelf-got)
		}
	})
}

// TestIdentifyFromAScroll. The spell is number 201, above MAX_SPELLS, so
// `cast` cannot reach it — a scroll is the only way, and now there is one.
func TestIdentifyFromAScroll(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	scroll := carry(t, srv, c, testScrollVnum, "scroll", "a scroll of protection")
	inWorld(t, srv, func(_ *game.Live) {
		scroll.Values[1] = game.SpellIdentify
		scroll.Values[2] = 0
	})
	carry(t, srv, c, testSwordVnum, "sword", "a long sword")

	c.send("recite scroll sword")
	c.expect("Object 'a long sword', Item type: WEAPON")

	// And the command still cannot.
	c.send("cast 'identify' sword")
	c.expect("Cast what?!?")
}

// TestJunkingSomethingPaysALittle.
func TestJunkingSomethingPaysALittle(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	sword := carry(t, srv, c, testSwordVnum, "sword", "a long sword")
	inWorld(t, srv, func(_ *game.Live) { sword.Cost = 320 })

	var before int32
	inWorld(t, srv, func(w *game.Live) { before = w.Find("Zod").Record.Points.Gold })

	c.send("junk sword")
	c.expect("You junk a long sword.  It vanishes in a puff of smoke!")
	c.expect("You have been rewarded by the gods!")

	inWorld(t, srv, func(w *game.Live) {
		// A coin per sixteen of value: 320/16 is 20.
		if got := w.Find("Zod").Record.Points.Gold; got != before+20 {
			t.Errorf("paid %d for junking, want 20", got-before)
		}
		if sword.Location != game.InNowhere {
			t.Error("the junked sword still exists")
		}
	})

	// You cannot junk everything you own in one word.
	c.send("junk all")
	c.expect("Go to the dump if you want to junk EVERYTHING!")
}

// TestDonatingSendsItToTheDonationRoom — one time in three it is junked
// instead, so this tries until it lands.
func TestDonatingSendsItToTheDonationRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// The donation room has to exist for anything to arrive in it. Checked
	// *outside* the closure: t.Skip calls runtime.Goexit, and doing that on
	// the world goroutine kills the world rather than the test.
	var haveRoom bool
	inWorld(t, srv, func(w *game.Live) { haveRoom = w.Room(3063) != nil })
	if !haveRoom {
		t.Skip("the test world has no donation room")
	}

	for i := 0; i < 20; i++ {
		sword := carry(t, srv, c, testSwordVnum, "sword", "a long sword")
		c.send("donate sword")
		c.settle()

		var arrived bool
		inWorld(t, srv, func(_ *game.Live) { arrived = sword.Room == 3063 })
		if arrived {
			return
		}
	}
	t.Error("twenty donations and nothing reached the donation room")
}
