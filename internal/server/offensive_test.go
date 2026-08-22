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

// TestHitSwingsImmediately, rather than joining the fight and waiting for the
// round. That is the difference between `hit` and everything else that starts
// a fight, and it is why opening with `hit` costs lag.
func TestHitSwingsImmediately(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	dog := spawnDog(t, srv, ImmortStartRoom)
	var before int32
	inWorld(t, srv, func(_ *game.Live) { before = dog.Record.Points.Hit })

	c.send("hit dog")
	got := c.expectAny("You hit a large dog", "You miss a large dog")

	inWorld(t, srv, func(w *game.Live) {
		if w.Find("Zod").Fighting != dog {
			t.Error("the swing did not start a fight")
		}
		// A hit landed means hit points came off there and then; a miss
		// means they did not.
		hurt := dog.Record.Points.Hit < before
		if hit := len(got) > 0 && c.seen("You hit a large dog"); hit != hurt {
			t.Errorf("said it hit = %v, but hit points came off = %v", hit, hurt)
		}
	})
}

// TestHittingYourself, and hitting your own pet.
func TestHittingYourself(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("hit")
	c.expect("Hit who?")

	c.send("hit nobody")
	c.expect("They don't seem to be here.")

	c.send("hit zod")
	c.expect("You hit yourself...OUCH!.")
}

// TestHittingAnotherPlayerNeedsMurder, which is the only thing the murder
// subcommand does differently.
func TestHittingAnotherPlayerNeedsMurder(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "w")

	grimm, _ := place(t, srv, fighterRecord("Grimm", 20, 100), MortalStartRoom)
	grimm.Record.Sex = game.SexMale

	c.send("hit grimm")
	c.expect("Use 'murder' to hit another player.")

	c.send("murder grimm")
	c.expectAny("You hit Grimm", "You miss Grimm")
}

// TestAnImplementorsKillIsSomethingElse. For everybody else `kill` is `hit`;
// for an implementor it is instant and unanswerable.
func TestAnImplementorsKillIsSomethingElse(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	dog := spawnDog(t, srv, ImmortStartRoom)

	var expBefore int32
	inWorld(t, srv, func(w *game.Live) { expBefore = w.Find("Zod").Record.Points.Exp })

	c.send("kill dog")
	c.expect("You chop it to pieces!  Ah!  The blood!")

	inWorld(t, srv, func(w *game.Live) {
		if dog.Record.Points.Hit > 0 {
			t.Errorf("the dog is on %d hit points", dog.Record.Points.Hit)
		}
		// A corpse, and no experience: raw_kill credits nobody.
		var corpse bool
		for _, obj := range w.RoomObjects(ImmortStartRoom) {
			if game.IsCorpse(obj) {
				corpse = true
			}
		}
		if !corpse {
			t.Error("no corpse was left")
		}
		if got := w.Find("Zod").Record.Points.Exp; got != expBefore {
			t.Errorf("the implementor was paid %d experience for a slaying", got-expBefore)
		}
	})

	c.send("kill zod")
	c.expect("Your mother would be so sad.. :(")
}

// TestAssistJoinsSomebodyElsesFight.
func TestAssistJoinsSomebodyElsesFight(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	friend, friendClient := place(t, srv, fighterRecord("Bob", 30, 500), ImmortStartRoom)
	friend.Record.Sex = game.SexMale
	dog := spawnDog(t, srv, ImmortStartRoom)

	c.send("assist")
	c.expect("Whom do you wish to assist?")

	c.send("assist zod")
	c.expect("You can't help yourself any more than this!")

	c.send("assist bob")
	c.expect("But nobody is fighting him!")

	inWorld(t, srv, func(w *game.Live) { w.SetFighting(friend, dog) })

	c.send("assist bob")
	c.expect("You join the fight!")
	c.settle()

	if !friendClient.said("Zod assists you!") {
		t.Error("Bob was not told")
	}
	inWorld(t, srv, func(w *game.Live) {
		if w.Find("Zod").Fighting != dog {
			t.Error("the assister did not join the fight")
		}
	})

	// Already fighting: you cannot assist anybody.
	c.send("assist bob")
	c.expect("You're already fighting!  How can you assist someone else?")
}

// TestAPeacefulRoomStopsEveryKindOfViolence. The check is in damage(), so one
// test covers the swing, the skill and the spell.
func TestAPeacefulRoomStopsEveryKindOfViolence(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	dog := spawnDog(t, srv, ImmortStartRoom)
	inWorld(t, srv, func(w *game.Live) {
		room := w.Room(ImmortStartRoom)
		room.Flags = room.Flags.Set(game.RoomPeaceful)
	})

	var before int32
	inWorld(t, srv, func(_ *game.Live) { before = dog.Record.Points.Hit })

	// A spell is turned away earlier, by call_magic rather than by damage().
	c.send("cast 'magic missile' dog")
	c.expect("A flash of white light fills the room")

	c.send("hit dog")
	c.expect("This room just has such a peaceful, easy feeling...")

	// Kick last: it costs three rounds of lag whether or not it lands, and
	// the dispatcher waits them out before the next command.
	c.send("kick dog")
	c.expectCount("This room just has such a peaceful, easy feeling...", 2)

	inWorld(t, srv, func(_ *game.Live) {
		if dog.Record.Points.Hit != before {
			t.Errorf("the dog lost %d hit points in a peaceful room",
				before-dog.Record.Points.Hit)
		}
	})
}

// TestASkillThatKillsLeavesACorpse, which it did not before: every command
// that could hurt somebody applied the damage itself and none of them handled
// what happens when the hit points run out.
func TestASkillThatKillsLeavesACorpse(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	dog := spawnDog(t, srv, ImmortStartRoom)
	inWorld(t, srv, func(_ *game.Live) {
		dog.Record.Points.Hit = 1
		dog.Record.Points.Exp = 3000
	})

	// An implementor's kick does level/2 = 17 damage, which is past -11.
	c.send("kick dog")
	c.expect("You kick a large dog.")

	inWorld(t, srv, func(w *game.Live) {
		var corpse bool
		for _, obj := range w.RoomObjects(ImmortStartRoom) {
			if game.IsCorpse(obj) {
				corpse = true
			}
		}
		if !corpse {
			t.Error("a kick killed the dog and left no corpse")
		}
		if got := w.Find("Zod").Record.Points.Exp; got == 0 {
			t.Error("a kick killed the dog and paid nothing")
		}
	})
}

// TestHittingYourOwnPetEndsTheArrangement.
func TestHittingYourOwnPetEndsTheArrangement(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	dog := spawnDog(t, srv, ImmortStartRoom)

	c.send("cast 'charm' dog")
	c.settle()
	inWorld(t, srv, func(w *game.Live) {
		if dog.Master != w.Find("Zod") {
			t.Error("the dog was not charmed into following")
		}
	})

	c.send("hit dog")
	c.expect("a large dog hates your guts!")

	inWorld(t, srv, func(_ *game.Live) {
		if dog.Master != nil {
			t.Error("the pet is still following after being hit")
		}
		if dog.Charmed() {
			t.Error("the charm survived the betrayal")
		}
	})
}

// TestACharacterCanLevel is Phase 4's done-criterion, end to end: a mortal
// walks up to something, kills it, is paid for it, and rises.
//
// Everything in the phase is behind this one test. The kill has to land, the
// death has to be noticed, the experience has to be worth something, the
// tables have to say what the next level costs, and advance_level has to run.
func TestACharacterCanLevel(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// The first character on the roster is the Implementor, who cannot gain
	// experience at all; the second is an ordinary mortal.
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "w")

	var welmar *game.Character
	inWorld(t, srv, func(w *game.Live) {
		welmar = w.Find("Welmar")
		welmar.Record.Points.HitRoll = 100
		welmar.Record.Points.DamRoll = 100
	})

	if welmar.Record.Level != 1 {
		t.Fatalf("a new character starts at level %d", welmar.Record.Level)
	}

	// A warrior needs 2000 experience for level 2, and the local
	// tenth-of-a-band cap means no single kill may award more than 200 — so
	// the boundary is ten kills away from nothing. Start most of the way
	// there: the point of this test is that the last kill levels them, not
	// that the cap works, which experience_test.go covers.
	inWorld(t, srv, func(_ *game.Live) { welmar.Record.Points.Exp = 1800 })

	for i := 0; i < 5 && welmar.Record.Level < 2; i++ {
		dog := spawnDog(t, srv, MortalStartRoom)
		inWorld(t, srv, func(_ *game.Live) {
			dog.Record.Points.Hit = 1
			dog.Record.Points.Exp = 100000
		})

		c.send("hit dog")
		c.expectAny("You hit a large dog", "You miss a large dog", "You receive")
		c.settle()
	}

	if welmar.Record.Level < 2 {
		t.Fatalf("still level %d after five kills, on %d experience",
			welmar.Record.Level, welmar.Record.Points.Exp)
	}
	if !c.seen("You rise a level!") {
		t.Error("the player was never told they had levelled")
	}

	inWorld(t, srv, func(_ *game.Live) {
		// advance_level ran: a warrior's hit points went up, and the title
		// changed with the level.
		if welmar.Record.Points.MaxHit <= 10 {
			t.Errorf("max hit is %d, which is not a level's worth",
				welmar.Record.Points.MaxHit)
		}
		if welmar.Record.Title == "" {
			t.Error("no title after levelling")
		}
	})
}
