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

// TestFollowingSomebodyMovesYouWithThem, which is the whole point of it.
func TestFollowingSomebodyMovesYouWithThem(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, bobClient := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)

	// Bob follows Zod, then Zod walks south.
	inWorld(t, srv, func(w *game.Live) { w.AddFollower(bob, w.Find("Zod")) })

	c.send("south")
	c.expect("The Temple Of Midgaard")

	inWorld(t, srv, func(_ *game.Live) {
		if bob.Room != MortalStartRoom {
			t.Errorf("Bob is in room %d, want the temple", bob.Room)
		}
	})
	if !bobClient.said("You follow Zod.") {
		t.Error("Bob was not told he was following")
	}
	// And he was shown the room he arrived in, not the one Zod is looking at.
	if !bobClient.said("The Temple Of Midgaard") {
		t.Error("Bob did not see the room he arrived in")
	}
}

// TestAFollowerWhoIsNotStandingStaysBehind.
func TestAFollowerWhoIsNotStandingStaysBehind(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, _ := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)
	inWorld(t, srv, func(w *game.Live) {
		w.AddFollower(bob, w.Find("Zod"))
		bob.Position = game.PosSleeping
	})

	c.send("south")
	c.expect("The Temple Of Midgaard")

	inWorld(t, srv, func(_ *game.Live) {
		if bob.Room != ImmortStartRoom {
			t.Error("a sleeping follower was dragged along")
		}
	})
}

// TestFollowAndStopFollowing, through the commands.
func TestFollowAndStopFollowing(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, bobClient := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)
	bob.Record.Sex = game.SexMale

	c.send("follow")
	c.expect("Whom do you wish to follow?")

	c.send("follow nobody")
	c.expect("No-one by that name here.")

	c.send("follow bob")
	c.expect("You now follow Bob.")
	c.settle()
	if !bobClient.said("Zod starts following you.") {
		t.Error("Bob was not told")
	}

	inWorld(t, srv, func(w *game.Live) {
		zod := w.Find("Zod")
		if zod.Master != bob {
			t.Error("Zod is not following Bob")
		}
		if len(bob.Followers) != 1 || bob.Followers[0] != zod {
			t.Errorf("Bob's followers are %v", bob.Followers)
		}
	})

	c.send("follow bob")
	c.expect("You are already following him.")

	// `follow self` is how you stop.
	c.send("follow zod")
	c.expect("You stop following Bob.")

	inWorld(t, srv, func(w *game.Live) {
		if w.Find("Zod").Master != nil {
			t.Error("Zod is still following somebody")
		}
		if len(bob.Followers) != 0 {
			t.Errorf("Bob still has followers: %v", bob.Followers)
		}
	})

	c.send("follow zod")
	c.expect("You are already following yourself.")
}

// TestFollowingInLoopsIsRefused. Without this check the two of them make a
// cycle, and the next step either takes hangs the server walking it.
func TestFollowingInLoopsIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, _ := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)
	inWorld(t, srv, func(w *game.Live) { w.AddFollower(bob, w.Find("Zod")) })

	c.send("follow bob")
	c.expect("Sorry, but following in loops is not allowed.")
}

// TestGroupingAndUngrouping.
func TestGroupingAndUngrouping(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, bobClient := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)

	c.send("group")
	c.expect("But you are not the member of a group!")

	// Somebody who is not following you cannot be enrolled.
	c.send("group bob")
	c.expect("Bob must follow you to enter your group.")

	inWorld(t, srv, func(w *game.Live) { w.AddFollower(bob, w.Find("Zod")) })

	c.send("group bob")
	c.expect("Bob is now a member of your group.")
	c.settle()
	if !bobClient.said("You are now a member of Zod's group.") {
		t.Error("Bob was not told he had been enrolled")
	}

	// The leader is not in their own group until they say so, which is why
	// `group` still refuses to list it.
	c.send("group")
	c.expect("But you are not the member of a group!")

	c.send("group zod")
	c.expect("You are now a member of Zod's group.")

	// The listing is several lines and the header arrives first, so wait for
	// the last member rather than for the header.
	c.send("group")
	got := c.expect("] Bob")
	for _, want := range []string{"Your group consists of:", "Zod (Head of group)"} {
		if !strings.Contains(got, want) {
			t.Errorf("the group listing is missing %q:\n%s", want, got)
		}
	}

	// Naming somebody already in the group throws them out.
	c.send("group bob")
	c.expect("Bob is no longer a member of your group.")

	inWorld(t, srv, func(_ *game.Live) {
		if bob.Grouped() {
			t.Error("Bob is still grouped")
		}
		if bob.Master == nil {
			t.Error("being thrown out of the group also stopped the following")
		}
	})
}

// TestUngroupDisbands, and stops everyone following.
func TestUngroupDisbands(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	bob, bobClient := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)
	inWorld(t, srv, func(w *game.Live) { w.AddFollower(bob, w.Find("Zod")) })

	c.send("group all")
	c.expect("Bob is now a member of your group.")

	c.send("ungroup")
	c.expect("You disband the group.")
	c.settle()

	if !bobClient.said("Zod has disbanded the group.") {
		t.Error("Bob was not told the group had gone")
	}
	inWorld(t, srv, func(w *game.Live) {
		if bob.Grouped() || w.Find("Zod").Grouped() {
			t.Error("somebody is still grouped")
		}
		if bob.Master != nil {
			t.Error("disbanding did not stop the following")
		}
	})
}

// TestTheGroupLevelSpread is local to this tree: more than seven levels apart
// and you cannot group at all.
func TestTheGroupLevelSpread(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// An implementor is level 34 and the newcomer is level 1.
	bob, bobClient := place(t, srv, fighterRecord("Bob", 1, 100), ImmortStartRoom)
	inWorld(t, srv, func(w *game.Live) { w.AddFollower(bob, w.Find("Zod")) })

	c.send("group bob")
	c.expect("Bob cannot hope to keep up with you!")
	c.settle()
	if !bobClient.said("You simply cannot keep up with Zod's group.") {
		t.Error("Bob was not told why")
	}

	inWorld(t, srv, func(_ *game.Live) {
		if bob.Grouped() {
			t.Error("Bob was grouped despite the level spread")
		}
	})

	// And the other way round.
	inWorld(t, srv, func(w *game.Live) { w.Find("Zod").Record.Level = 1 })
	bob.Record.Level = 30

	c.send("group bob")
	c.expect("Bob is far too tough to group with little ol' you.")
}

// TestAGroupSharesExperience, and everybody present gets a cut whether or not
// they swung at anything.
func TestAGroupSharesExperience(t *testing.T) {
	srv, _ := newTestServer(t)

	// Enough hitroll to land and enough damroll to take the dog past -11,
	// which is where UpdatePosition calls it dead.
	killer, _ := place(t, srv, fighterRecord("Zod", 10, 500), MortalStartRoom)
	killer.Record.Points.HitRoll = 100
	killer.Record.Points.DamRoll = 100
	idler, idlerClient := place(t, srv, fighterRecord("Bob", 10, 500), MortalStartRoom)

	victim, _ := place(t, srv, fighterRecord("a large dog", 5, 1), MortalStartRoom)
	victim.NPC = true
	victim.Record.Points.Exp = 3000

	inWorld(t, srv, func(w *game.Live) {
		w.AddFollower(idler, killer)
		killer.SetGrouped(true)
		idler.SetGrouped(true)
		w.SetFighting(killer, victim)
	})

	round(t, srv)

	inWorld(t, srv, func(_ *game.Live) {
		if idler.Record.Points.Exp == 0 {
			t.Error("the idler got no share of the kill")
		}
		if killer.Record.Points.Exp == 0 {
			t.Error("the killer got no experience")
		}
		// (3000/3 + 2 - 1) / 2 = 500 each, rounded up.
		if idler.Record.Points.Exp != 500 || killer.Record.Points.Exp != 500 {
			t.Errorf("shares are %d and %d, want 500 each",
				killer.Record.Points.Exp, idler.Record.Points.Exp)
		}
	})
	if !idlerClient.said("You receive your share of experience -- 500 points.") {
		t.Error("the idler was not told about their share")
	}
}

// TestDeathDissolvesTheFollowing.
func TestDeathDissolvesTheFollowing(t *testing.T) {
	srv, _ := newTestServer(t)

	leader, _ := place(t, srv, fighterRecord("Zod", 30, 500), MortalStartRoom)
	leader.Record.Points.HitRoll = 100
	leader.Record.Points.DamRoll = 100
	pet, _ := place(t, srv, fighterRecord("a large dog", 5, 1), MortalStartRoom)
	pet.NPC = true

	inWorld(t, srv, func(w *game.Live) {
		w.AddFollower(pet, leader)
		w.SetFighting(leader, pet)
	})

	round(t, srv)

	inWorld(t, srv, func(_ *game.Live) {
		if pet.Record.Points.Hit > 0 {
			t.Error("the dog survived a hundred damroll")
		}
		if len(leader.Followers) != 0 {
			t.Errorf("the dead follower is still on the list: %v", leader.Followers)
		}
		if pet.Master != nil {
			t.Error("the dead follower still has a master")
		}
	})
}

// TestCharmingAMobile, and what it does to the following.
func TestCharmingAMobile(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	dog := spawnDog(t, srv, ImmortStartRoom)

	c.send("cast 'charm' dog")
	c.settle()

	inWorld(t, srv, func(w *game.Live) {
		zod := w.Find("Zod")
		if dog.Master != zod {
			t.Error("the dog is not following the caster")
		}
		if !dog.Charmed() {
			t.Error("the dog is not charmed")
		}
	})

	// Charm is flagged violent and TAR_NOT_SELF, so `cast 'charm' me` never
	// reaches spell_charm's "You like yourself even better!" — do_cast turns
	// it away first. That is the C's arrangement too.
	c.send("cast 'charm' zod")
	c.expect("You shouldn't cast that on yourself")

	// Charming something already charmed fails, with no explanation.
	c.send("cast 'charm' dog")
	c.expect("You fail.")
}

// TestAGroupSpellReachesTheWholeGroup.
func TestAGroupSpellReachesTheWholeGroup(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "c")

	bob, bobClient := place(t, srv, fighterRecord("Bob", 30, 100), ImmortStartRoom)
	bob.Record.Points.Hit = 10

	inWorld(t, srv, func(w *game.Live) {
		w.AddFollower(bob, w.Find("Zod"))
		bob.SetGrouped(true)
		w.Find("Zod").SetGrouped(true)
	})

	c.send("cast 'group heal'")
	c.settle()

	inWorld(t, srv, func(_ *game.Live) {
		if bob.Record.Points.Hit <= 10 {
			t.Errorf("Bob is on %d hit points and was not healed", bob.Record.Points.Hit)
		}
	})
	if !bobClient.said("A warm feeling floods your body.") {
		t.Error("Bob was not told he was healed")
	}
}

// TestSummoningIsRefusedForMobiles, which is local: the C's summon can move
// monsters and this tree's cannot.
func TestSummoningIsRefusedForMobiles(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	spawnDog(t, srv, MortalStartRoom)

	c.send("cast 'summon' dog")
	c.expect("Only players may be summoned.")
}
