// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/session"
)

// `steal` and `track`, end to end.

// asMortal drops a character out of implementor-hood.
//
// The first character created on an empty roster is an Implementor, and
// init_char gives an implementor 100% in *every* skill (db.c:2750) — so a
// test for "you have no idea how" has to make them somebody ordinary first.
func asMortal(t *testing.T, srv *Server, name string) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil || who.Record == nil {
			t.Error("the character is not in the world")
			return
		}
		who.Record.Level = 20
		for i := range who.Record.Skills {
			who.Record.Skills[i] = 0
		}
	})
}

// withSkill gives a character a skill at a percentage.
func withSkill(t *testing.T, srv *Server, name string, skill game.SpellID, percent int32) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil || who.Record == nil {
			t.Error("the character is not in the world")
			return
		}
		// A mortal thief, not the implementor the first character becomes —
		// and with only the skill under test, so nothing else interferes.
		who.Record.Level = 20
		who.Record.Class = game.ClassThief
		for i := range who.Record.Skills {
			who.Record.Skills[i] = 0
		}
		who.Record.Skills[skill] = percent
	})
}

// aMobile spawns the test dog in the character's room and returns it.
func aMobile(t *testing.T, srv *Server, near string) *game.Character {
	t.Helper()
	var mob *game.Character
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(near)
		if who == nil {
			t.Error("the character is not in the world")
			return
		}
		if mob = w.SpawnMobile(testDogVnum, who.Room, srv.rng); mob == nil {
			t.Error("could not spawn a mobile")
		}
	})
	return mob
}

func TestStealingNeedsTheSkill(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Fumble", "allthumbs", "m", "w")
	asMortal(t, srv, "Fumble")
	aMobile(t, srv, "Fumble")

	c.send("steal purse dog")
	c.expect("You have no idea how to do that.")
}

func TestStealingCoinsFromAMobile(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Nimble", "lightfingers", "m", "t")
	withSkill(t, srv, "Nimble", game.SkillSteal, 100)

	dog := aMobile(t, srv, "Nimble")
	inWorld(t, srv, func(_ *game.Live) {
		if dog != nil && dog.Record != nil {
			dog.Record.Points.Gold = 1000
			// Asleep, so the roll is skipped entirely and this is
			// deterministic.
			dog.Position = game.PosSleeping
		}
	})

	c.send("steal coins dog")
	c.expectAny("Bingo!  You got", "You manage to swipe a solitary gold coin.",
		"You couldn't get any gold...")

	var left int32
	inWorld(t, srv, func(_ *game.Live) {
		if dog != nil && dog.Record != nil {
			left = dog.Record.Points.Gold
		}
	})
	if left > 1000 {
		t.Errorf("the dog gained gold: %d", left)
	}
	if got := goldOf(t, srv, "Nimble"); got != 1000-left {
		t.Errorf("the thief has %d gold and the dog lost %d", got, 1000-left)
	}
}

// pt_allowed is NO, so a steal from another *player* is a flat failure
// whatever the skill.
func TestYouCannotStealFromAnotherPlayer(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	thief := dialClient(t, addr)
	thief.create("Cutpurse", "sleightofhand", "m", "t")

	mark := dialClient(t, addr)
	mark.create("Mark", "unsuspecting", "m", "w")
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Mark"); who != nil && who.Record != nil {
			who.Record.Points.Gold = 5000
			who.Record.Level = 10
		}
	})

	withSkill(t, srv, "Cutpurse", game.SkillSteal, 100)
	// The first character starts in the immortal room and the second in the
	// temple; stealing needs them in the same one.
	moveTo(t, srv, "Cutpurse", MortalStartRoom)

	if !session.PlayerThievingAllowed {
		thief.send("steal coins mark")
		thief.expect("Oops..")
		mark.settle()
		if !mark.seen("has his hands in your wallet") {
			t.Error("the mark was not told about the attempt")
		}
	}

	if got := goldOf(t, srv, "Mark"); got != 5000 {
		t.Errorf("a player was robbed of %d gold with pt_allowed off", 5000-got)
	}
}

func TestStealingWhatSomebodyHasNotGot(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Grabby", "quickhands", "m", "t")
	withSkill(t, srv, "Grabby", game.SkillSteal, 100)
	aMobile(t, srv, "Grabby")

	c.send("steal crown dog")
	c.expect("hasn't got that item.")

	c.send("steal purse nobody")
	c.expect("Steal what from who?")

	c.send("steal purse grabby")
	c.expect("Come on now, that's rather stupid!")
}

// Equipment can only be taken from somebody out cold.
func TestStealingEquipment(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Pincher", "offyourback", "m", "t")
	withSkill(t, srv, "Pincher", game.SkillSteal, 100)

	dog := aMobile(t, srv, "Pincher")
	inWorld(t, srv, func(w *game.Live) {
		ring := w.NewObject(testRingVnum)
		if dog == nil || ring == nil {
			t.Error("could not equip the mobile")
			return
		}
		w.Equip(ring, dog, game.WearFingerRight)
	})

	c.send("steal ring dog")
	c.expect("Steal the equipment now?  Impossible!")

	inWorld(t, srv, func(_ *game.Live) {
		if dog != nil {
			dog.Position = game.PosStunned
		}
	})

	c.send("steal ring dog")
	c.expect("You unequip a gold ring and steal it.")

	var carrying int
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Pincher"); who != nil {
			carrying = len(who.Carrying)
		}
	})
	if carrying != 1 {
		t.Errorf("the thief is carrying %d things, want the ring", carrying)
	}
}

func TestTrackingNeedsTheSkill(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Blind", "cannotsee", "m", "w")
	asMortal(t, srv, "Blind")

	c.send("track somebody")
	c.expect("You have no idea how.")
}

func TestTracking(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	tracker := dialClient(t, addr)
	tracker.create("Hunter", "onthetrail", "m", "t")
	withSkill(t, srv, "Hunter", game.SkillTrack, 100)

	quarry := dialClient(t, addr)
	quarry.create("Quarry", "runningaway", "m", "w")

	tracker.send("track")
	tracker.expect("Whom are you trying to track?")

	tracker.send("track nobodyatall")
	tracker.expect("No one is around by that name.")

	// Both start in the same room.
	tracker.send("track quarry")
	tracker.expectAny("You're already in the same room!!", "You sense a trail")

	// The temple is north of the board room, so from the temple the trail
	// runs north. A skill of 100 still fails one roll in 102, so either
	// answer is acceptable — what must not happen is an error.
	moveTo(t, srv, "Hunter", MortalStartRoom)
	moveTo(t, srv, "Quarry", ImmortStartRoom)
	tracker.send("track quarry")
	tracker.expect("You sense a trail")

	if tracker.seen("something seems to be wrong") {
		t.Error("tracking reported an error")
	}
}

// A NOTRACK affect hides somebody entirely.
func TestSomebodyWithNoTrackCannotBeFound(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	tracker := dialClient(t, addr)
	tracker.create("Seeker", "wheredidgo", "m", "t")
	withSkill(t, srv, "Seeker", game.SkillTrack, 100)

	hidden := dialClient(t, addr)
	hidden.create("Ghost", "notfoundhere", "m", "w")
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Ghost"); who != nil && who.Record != nil {
			who.Record.AffectFlags = who.Record.AffectFlags.With(game.AffectNoTrack)
		}
	})

	tracker.send("track ghost")
	tracker.expect("You sense no trail.")
}
