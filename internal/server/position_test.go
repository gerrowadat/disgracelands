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
)

// TestPositionCommands walks the whole state machine, checking both the
// resulting position and the words.
func TestPositionCommands(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	for _, step := range []struct {
		command string
		expect  string
	}{
		{"stand", "You are already standing."},
		{"sit", "You sit down."},
		{"sit", "You're sitting already."},
		{"rest", "You rest your tired bones."},
		{"rest", "You are already resting."},
		{"stand", "You stop resting, and stand up."},
		{"rest", "You sit down and rest your tired bones."},
		{"sit", "You stop resting, and sit up."},
		{"sleep", "You go to sleep."},
		{"sleep", "You are already sound asleep."},
		// `stand`, `sit` and `rest` are all POS_RESTING in the C's table
		// (interpreter.c:490, :468, :426), so a sleeping character is stopped
		// by the interpreter and never reaches the command. Each of the three
		// has a POS_SLEEPING branch saying "You have to wake up first" that no
		// player has ever seen. This test expected those three messages until
		// minimum position was enforced. See docs/weirdnumbers.md.
		{"stand", "In your dreams, or what?"},
		{"sit", "In your dreams, or what?"},
		{"rest", "In your dreams, or what?"},
		{"wake", "You awaken, and sit up."},
		{"wake", "You are already awake..."},
		{"stand", "You stand up."},
	} {
		c.send(step.command)
		c.expect(step.expect)
	}
}

// TestYouCannotRestWhileFighting, in three differently worded refusals —
// two "Are you MAD?" and one rhetorical question, all the C's.
func TestYouCannotRestWhileFighting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")
	spawnDog(t, srv, ImmortStartRoom)

	// `hit`, not `kill`: for an implementor `kill` is the instant slay, and
	// these tests want a fight rather than a corpse.
	c.send("hit dog")
	c.expect("a large dog") // present in every damage tier's text, hit or miss

	for _, tc := range []struct{ command, expect string }{
		{"sit", "Sit down while fighting? Are you MAD?"},
		{"rest", "Rest while fighting?  Are you MAD?"},
		{"sleep", "Sleep while fighting?  Are you MAD?"},
		{"stand", "Do you not consider fighting as standing?"},
	} {
		c.send(tc.command)
		c.expect(tc.expect)
	}
}

// TestFleeingLeavesTheRoomAndTheFight.
func TestFleeingLeavesTheRoomAndTheFight(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	// A mobile to be fighting, in the room the character starts in.
	var mob *game.Character
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		mob = &game.Character{
			Name: "a large dog", Keywords: "dog", NPC: true,
			Position: game.PosStanding,
			MobDef:   &game.MobDef{Vnum: 999, ShortDesc: "a large dog", Keywords: "dog"},
			Record: &game.PlayerRecord{
				Name: "a large dog", Level: 5,
				Points: game.Points{Hit: 50, MaxHit: 100},
			},
		}
		if err := w.Enter(mob, ImmortStartRoom); err != nil {
			t.Errorf("placing the dog: %v", err)
		}
		w.Track(mob)
	}); err != nil {
		t.Fatal(err)
	}

	// `hit`, not `kill`: for an implementor `kill` is the instant slay, and
	// these tests want a fight rather than a corpse.
	c.send("hit dog")
	c.expect("a large dog") // present in every damage tier's text, hit or miss

	fleeUntilItWorks(t, c)

	// They ended up somewhere else, and look ran on arrival — waited for
	// separately, since the flee message arrives first.
	got := c.expect("The Temple Of Midgaard")
	if !strings.Contains(got, "The Temple Of Midgaard") {
		t.Errorf("fleeing did not move them:\n%s", got)
	}

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if mob.Fighting != nil {
			t.Error("the dog is still fighting somebody who ran away")
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// TestFleeingCostsExperience, and the figure is the opponent's *missing* hit
// points times their level — so running from a fight you were winning costs
// more than running from one you were losing.
func TestFleeingCostsExperience(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// Not the first character, so this is an ordinary mortal who can lose
	// experience: an implementor is above the level where gain_exp does
	// anything.
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "w")

	dog := spawnDog(t, srv, MortalStartRoom)
	dog.Record.Level = 10
	dog.Record.Points.MaxHit = 100
	dog.Record.Points.Hit = 40 // 60 missing

	var runner *game.Character
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		runner = w.Find("Welmar")
		runner.Record.Points.Exp = 5_000_000
		runner.Record.Level = 20
	}); err != nil {
		t.Fatal(err)
	}

	// `hit`, not `kill`: for an implementor `kill` is the instant slay, and
	// these tests want a fight rather than a corpse.
	c.send("hit dog")
	c.expect("a large dog") // present in every damage tier's text, hit or miss

	// The cost is the opponent's *missing* hit points times their level, read
	// at the moment of fleeing — `hit` has already landed one blow of its
	// own, so it is not the 60 the dog was set up with.
	var before, missing int32
	inWorld(t, srv, func(_ *game.Live) {
		before = runner.Record.Points.Exp
		missing = dog.Record.Points.MaxHit - dog.Record.Points.Hit
	})
	fleeUntilItWorks(t, c)

	// Read on the world goroutine: the combat pulse writes this field, and
	// reading it from the test goroutine is a genuine race that the detector
	// catches about one run in ten.
	var after int32
	inWorld(t, srv, func(_ *game.Live) { after = runner.Record.Points.Exp })

	lost := before - after
	if want := missing * 10; lost != want {
		t.Errorf("fleeing cost %d experience, want %d (%d missing hit points at level 10)",
			lost, want, missing)
	}
}

// TestFleeingAnotherPlayerCostsNothing — the local
// no-experience-between-players rule applies here too.
func TestFleeingAnotherPlayerCostsNothing(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "w")

	other, _ := place(t, srv, fighterRecord("Grimm", 25, 100), MortalStartRoom)
	other.Record.Points.Hit = 10

	var runner *game.Character
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		runner = w.Find("Welmar")
		runner.Record.Points.Exp = 5_000_000
		runner.Record.Level = 20
	}); err != nil {
		t.Fatal(err)
	}

	// Another player, so it has to be `murder` — `hit` refuses.
	c.send("murder grimm")
	c.expect("Grimm") // present in every damage tier's text, hit or miss

	var before, after int32
	inWorld(t, srv, func(_ *game.Live) { before = runner.Record.Points.Exp })
	fleeUntilItWorks(t, c)
	inWorld(t, srv, func(_ *game.Live) { after = runner.Record.Points.Exp })

	if after != before {
		t.Errorf("fleeing another player cost %d experience", before-after)
	}
}

// TestYouCannotFleeWhenDying.
func TestYouCannotFleeWhenDying(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.Find("Zod").Position = game.PosStunned
	}); err != nil {
		t.Fatal(err)
	}

	// `flee` is POS_FIGHTING in the C's table (interpreter.c:297) and
	// `do_flee` opens with `if (GET_POS(ch) < POS_FIGHTING)` — the same test
	// the interpreter has already made. So its "You are in pretty bad shape,
	// unable to flee!" is unreachable, and what a stunned character actually
	// gets is the interpreter's message for being stunned. See
	// docs/weirdnumbers.md.
	c.send("flee")
	c.expect("All you can do right now is think about the stars!")
}

// TestScore shows the numbers, including the "/10" the C says out loud about
// armour class.
func TestScore(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("score")
	// Wait for the *last* line score prints, not the first: expect returns as
	// soon as its marker appears and the rest may not have arrived yet.
	got := c.expect("You are standing.")

	for _, want := range []string{
		"You are 17 years old.",
		"You have 500(500) hit, 100(100) mana and 82(82) movement points.",
		// Base armour 100, plus dexterity 25's -6 defensive times ten.
		"armor class is 40/10",
		"This ranks you as Zod the Implementor (level 34).",
		"You are standing.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("score is missing %q:\n%s", want, got)
		}
	}

	// An implementor is past the last mortal level, so there is no "you need
	// N exp" line.
	if strings.Contains(got, "to reach your next level") {
		t.Errorf("an implementor was told how much experience they need:\n%s", got)
	}
}

// TestScoreReportsThePosition, in the C's words.
func TestScoreReportsThePosition(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	for _, tc := range []struct {
		command string
		expect  string
	}{
		{"sit", "You are sitting."},
		{"rest", "You are resting."},
		{"sleep", "You are sleeping."},
	} {
		c.send(tc.command)
		c.expect("> ")
		c.send("score")
		c.expect(tc.expect)
	}
}

func TestExits(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("exits")
	got := c.expect("The Temple Of Midgaard")
	if !strings.Contains(got, "Obvious exits:") {
		t.Errorf("the exit's destination is not named:\n%s", got)
	}

	// A closed door is named rather than pointed through, which is how a
	// player knows there is something to open.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		exit := w.Room(ImmortStartRoom).Exits[game.South]
		exit.Keywords = "gate"
		exit.State = exit.State.Set(game.ExitIsDoor | game.ExitClosed)
	}); err != nil {
		t.Fatal(err)
	}

	c.send("exits")
	if got := c.expect("The gate is closed."); !strings.Contains(got, "The gate is closed.") {
		t.Errorf("a closed door was not reported:\n%s", got)
	}
}

// fleeUntilItWorks types `flee` until the character gets away.
//
// The C picks six random directions and gives up if none of them is a way
// out — "PANIC!  You couldn't escape!" — and in a two-room test world that
// happens about a third of the time. Retrying is the test accommodating
// faithful behaviour rather than papering over a bug.
func fleeUntilItWorks(t *testing.T, c *client) string {
	t.Helper()

	for attempt := 1; attempt <= 20; attempt++ {
		c.send("flee")
		c.expectAny("head over heels", "couldn't escape")
		if c.seen("head over heels") {
			return c.transcript()
		}
	}
	t.Fatalf("could not flee in twenty attempts:\n%s", c.transcript())
	return ""
}

// spawnDog puts a plain mobile in a room for a test.
func spawnDog(t *testing.T, srv *Server, room game.RoomVnum) *game.Character {
	t.Helper()

	dog := &game.Character{
		Name: "a large dog", Keywords: "dog", NPC: true,
		Position: game.PosStanding,
		MobDef:   &game.MobDef{Vnum: 999, ShortDesc: "a large dog", Keywords: "dog"},
		Record: &game.PlayerRecord{
			Name: "a large dog", Level: 5,
			Points: game.Points{Hit: 500, MaxHit: 500},
		},
	}
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if err := w.Enter(dog, room); err != nil {
			t.Errorf("placing the dog: %v", err)
		}
		w.Track(dog)
	}); err != nil {
		t.Fatal(err)
	}
	return dog
}
