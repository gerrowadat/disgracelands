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

// withSpec attaches a special to a mobile prototype and spawns one, the way
// spec_assign.c and a zone reset would between them.
func withSpec(t *testing.T, srv *Server, vnum game.MobVnum, spec string, room game.RoomVnum) *game.Character {
	t.Helper()

	// Errors are collected rather than fatal: t.Fatal inside this closure
	// would Goexit the world goroutine. See inWorld.
	var mob *game.Character
	var problem string
	inWorld(t, srv, func(w *game.Live) {
		def := w.MobileDef(vnum)
		if def == nil {
			problem = "no mobile prototype in the test world"
			return
		}
		def.Spec = spec
		def.ActionFlags = def.ActionFlags.With(game.MobSpec)

		if mob = w.SpawnMobile(vnum, room, srv.rng); mob == nil {
			problem = "could not spawn it"
		}
	})
	if problem != "" {
		t.Fatalf("mobile %d: %s", vnum, problem)
	}
	return mob
}

// TestPracticeNeedsAGuildmaster, which is the deviation this slice closes:
// `practice` taught anywhere until specprocs existed to take it back.
func TestPracticeNeedsAGuildmaster(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	// With nobody to teach, the command only lists. A fresh implementor
	// (the first character on an empty roster, see newTestServer) knows
	// every class's spells, which list_skills's own single page_string
	// call (spec_procs.c:193) is long enough to page — so the pager has
	// to be closed before the next command, or "practice armor" would be
	// read as pager input instead of a new command.
	c.send("practice")
	c.expect("You know of the following")
	c.expect("Return to continue")
	c.send("q")

	c.send("practice armor")
	c.expect("You can only practice skills in your guild.")

	// A guildmaster in the room takes the command instead.
	withSpec(t, srv, testGuildmasterVnum, "guild", ImmortStartRoom)

	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Zod").Record
		rec.SpellsToLearn = 3
		rec.Skills[game.SpellArmor] = 0
	})

	c.send("practice armor")
	c.expect("You practice for a while...")

	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Zod").Record
		if rec.Skills[game.SpellArmor] == 0 {
			t.Error("the guildmaster taught nothing")
		}
		if rec.SpellsToLearn != 2 {
			t.Errorf("%d sessions left, want 2", rec.SpellsToLearn)
		}
	})
}

// TestAGuildmasterOnlyTakesPractice — every other command goes past it.
func TestAGuildmasterOnlyTakesPractice(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	withSpec(t, srv, testGuildmasterVnum, "guild", ImmortStartRoom)

	c.send("look")
	c.expect("The Immortal Board Room")
	c.send("score")
	c.expect("You are")
}

// TestPuffSaysThings. The roll is number(0, 60) against four lines, so most
// pulses are silent; the test runs enough of them.
func TestPuffSaysThings(t *testing.T) {
	srv, _ := newTestServer(t)

	puff := withSpec(t, srv, testDogVnum, "puff", ImmortStartRoom)
	// Sentinel, or she wanders out of earshot within a few pulses and the
	// test is watching an empty room.
	inWorld(t, srv, func(_ *game.Live) {
		puff.MobDef.ActionFlags = puff.MobDef.ActionFlags.With(game.MobSentinel)
	})
	_, listener := place(t, srv, fighterRecord("Bob", 10, 100), ImmortStartRoom)

	for i := 0; i < 300; i++ {
		inWorld(t, srv, srv.mobileActivity)
		if listener.said("very female dragon") || listener.said("full of stars") ||
			listener.said("all those fish") || listener.said("Hail to the King") {
			return
		}
	}
	t.Error("Puff said nothing in three hundred pulses")
}

// TestAJanitorPicksUpTrash, and leaves anything valuable.
func TestAJanitorPicksUpTrash(t *testing.T) {
	srv, _ := newTestServer(t)

	janitor := withSpec(t, srv, testDogVnum, "janitor", ImmortStartRoom)

	// The sword is worth more than fifteen coins; the ring is worth nothing.
	sword := drop(t, srv, testSwordVnum, ImmortStartRoom)
	ring := drop(t, srv, testRingVnum, ImmortStartRoom)
	inWorld(t, srv, func(_ *game.Live) {
		sword.Cost = 100
		ring.Cost = 5
	})

	for i := 0; i < 5; i++ {
		inWorld(t, srv, srv.mobileActivity)
	}

	inWorld(t, srv, func(_ *game.Live) {
		if ring.Holder != janitor {
			t.Error("the janitor left the worthless ring on the floor")
		}
		if sword.Location != game.InRoom {
			t.Error("the janitor took a hundred-coin sword")
		}
	})
}

// TestAFidoEatsCorpsesAndLeavesTheContents.
func TestAFidoEatsCorpsesAndLeavesTheContents(t *testing.T) {
	srv, _ := newTestServer(t)

	withSpec(t, srv, testDogVnum, "fido", ImmortStartRoom)

	var sword *game.Object
	inWorld(t, srv, func(w *game.Live) {
		victim := &game.Character{
			Name: "Welmar", Position: game.PosStanding,
			Record: &game.PlayerRecord{Name: "Welmar", Points: game.Points{MaxHit: 10}},
		}
		if err := w.Enter(victim, ImmortStartRoom); err != nil {
			t.Error(err)
		}
		sword = w.NewObject(testSwordVnum)
		w.ObjectToChar(sword, victim)
		w.MakeCorpse(victim)
	})

	inWorld(t, srv, srv.mobileActivity)

	inWorld(t, srv, func(w *game.Live) {
		for _, obj := range w.RoomObjects(ImmortStartRoom) {
			if game.IsCorpse(obj) {
				t.Error("the corpse survived the fido")
			}
		}
		if sword.Location != game.InRoom {
			t.Errorf("the sword is %v, want left on the floor", sword.Location)
		}
	})
}

// TestASnakeBitesWhatItIsFighting. The special runs *before* mobile_activity's
// fighting check, which is the whole reason a snake works at all.
func TestASnakeBitesWhatItIsFighting(t *testing.T) {
	srv, _ := newTestServer(t)

	snake := withSpec(t, srv, testDogVnum, "snake", MortalStartRoom)
	victim, _ := place(t, srv, fighterRecord("Bob", 20, 500), MortalStartRoom)

	inWorld(t, srv, func(w *game.Live) {
		snake.Record.Level = 1 // number(0, 1): bites about half the time
		w.SetFighting(snake, victim)
	})

	var bitten bool
	for i := 0; i < 40 && !bitten; i++ {
		inWorld(t, srv, srv.mobileActivity)
		inWorld(t, srv, func(_ *game.Live) {
			bitten = victim.Record.AffectFlags.Has(game.AffectPoison)
		})
	}
	if !bitten {
		t.Error("a snake fought for forty pulses without poisoning anybody")
	}
}

// TestTheDumpEatsWhatYouDropAndPaysYou.
func TestTheDumpEatsWhatYouDropAndPaysYou(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		w.Room(ImmortStartRoom).Spec = "dump"
	})

	// Handed over directly rather than picked up: the dump destroys anything
	// lying in the room on *every* call, including the `get` that would have
	// taken it, so there is no way to pick something up in there.
	inWorld(t, srv, func(w *game.Live) {
		sword := w.NewObject(testSwordVnum)
		sword.Cost = 100
		w.ObjectToChar(sword, w.Find("Zod"))
	})

	var before int32
	inWorld(t, srv, func(w *game.Live) { before = w.Find("Zod").Record.Points.Gold })

	c.send("drop sword")
	c.expect("You are awarded for outstanding performance.")

	inWorld(t, srv, func(w *game.Live) {
		if got := len(w.RoomObjects(ImmortStartRoom)); got != 0 {
			t.Errorf("%d objects left in the dump", got)
		}
		// A hundred-coin sword is worth ten, under the cap of fifty.
		if got := w.Find("Zod").Record.Points.Gold; got != before+10 {
			t.Errorf("paid %d, want 10", got-before)
		}
	})
}

// TestGuildGuardsBlockTheWrongClass, and let a remorted character through —
// which is the local rewrite, and the reason this special is worth a test of
// its own.
func TestGuildGuardsBlockTheWrongClass(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// The mage guild's door: room 3017, south. Move the start room's exit to
	// stand in for it, and put a guard on it.
	inWorld(t, srv, func(w *game.Live) {
		zod := w.Find("Zod")
		zod.Record.Level = 10 // a mortal; an immortal walks past guards
		zod.Record.Class = game.ClassWarrior
		zod.Record.RemortVector = 0
		if err := w.Enter(zod, MageGuildRoom); err != nil {
			t.Error(err)
		}
	})

	withSpec(t, srv, testDogVnum, "guild_guard", MageGuildRoom)

	c.send("south")
	c.expect("The guard humiliates you, and blocks your way.")

	// A character who has been a magic-user is let through.
	inWorld(t, srv, func(w *game.Live) {
		w.Find("Zod").Record.RemortVector = int32(game.RemortMagicUser)
	})

	c.send("south")
	c.expect("The Temple Of Midgaard")

	inWorld(t, srv, func(w *game.Live) {
		if got := w.Find("Zod").Room; got != MortalStartRoom {
			t.Errorf("a former magic-user is in room %d, want the temple", got)
		}
	})
}

// TestSpecialsCanBeTurnedOff, which is the C's -s.
func TestSpecialsCanBeTurnedOff(t *testing.T) {
	srv, _ := newTestServer(t)
	srv.noSpecials = true

	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	withSpec(t, srv, testGuildmasterVnum, "guild", ImmortStartRoom)
	inWorld(t, srv, func(w *game.Live) { w.Find("Zod").Record.SpellsToLearn = 3 })

	// The guildmaster is inert, so the command reaches do_practice.
	c.send("practice armor")
	c.expect("You can only practice skills in your guild.")
}
