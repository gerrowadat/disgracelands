// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The refusals command_interpreter makes between finding a command and running
// it (interpreter.c:629-661), through a real socket.

// TestPositionRefusalsAreTheCs walks a character down through every position
// and checks the message each one produces.
//
// The commands are chosen so that the *command* would otherwise work: `kill`
// needs POS_FIGHTING, `get` POS_RESTING, `look` POS_RESTING. What is being
// asserted is the interpreter's refusal, not any command's own.
func TestPositionRefusalsAreTheCs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pos     game.Position
		command string
		want    string
	}{
		// `kill` is POS_FIGHTING, so standing is the only position that clears
		// it, and every position below produces its own message.
		{"sitting", game.PosSitting, "kill dog", "Maybe you should get on your feet first?"},
		{"resting", game.PosResting, "kill dog", "Nah... You feel too relaxed to do that.."},
		{"sleeping", game.PosSleeping, "kill dog", "In your dreams, or what?"},
		{"stunned", game.PosStunned, "kill dog", "All you can do right now is think about the stars!"},
		{"incapacitated", game.PosIncapacitated, "kill dog", "You are in a pretty bad shape, unable to do anything!"},
		{"mortally wounded", game.PosMortallyWounded, "kill dog", "You are in a pretty bad shape, unable to do anything!"},
		{"dead", game.PosDead, "kill dog", "Lie still; you are DEAD!!! :-("},
		// `look` is POS_RESTING, so a resting character may look and a
		// sleeping one may not. This is the boundary the whole mechanism
		// turns on and it is worth having both sides of it.
		{"asleep cannot look", game.PosSleeping, "look", "In your dreams, or what?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newTestServer(t)
			c := dialClient(t, listening(t, srv))
			c.create("Zod", "swordfish", "m", "w")

			if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
				w.Find("Zod").Position = tc.pos
			}); err != nil {
				t.Fatal(err)
			}

			c.send(tc.command)
			c.expect(tc.want)
		})
	}
}

// TestRestingStillLetsYouLook is the other side of the boundary above: the
// refusal is `<`, not `<=`, so a command whose minimum is exactly your
// position runs.
func TestRestingStillLetsYouLook(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("rest")
	c.expect("You sit down and rest your tired bones.")
	c.send("look")
	c.expect("The Immortal Board Room")
}

// TestPositionIsNotPartOfMatching. The level check happens *while* matching, so
// a command above your level is invisible and answers "Huh?!?"; the position
// check happens after, so a command you cannot do right now is still found and
// still says so. Mixing the two up would make `kill` mean something else to a
// sitting character.
func TestPositionIsNotPartOfMatching(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.Find("Zod").Position = game.PosSitting
	}); err != nil {
		t.Fatal(err)
	}

	c.send("kill dog")
	c.expect("Maybe you should get on your feet first?")
	if c.seen("Huh?!?") {
		t.Error("a command refused by position was treated as no command at all")
	}
}

// TestASpecialDoesNotGetFirstRefusalFromTheFloor.
//
// The position check comes before `special()` in the C (interpreter.c:636 and
// :661), which is what stops a shopkeeper trading with somebody who is asleep.
// Getting this order wrong is invisible in every other test, because the
// special would answer correctly — just to somebody who should never have been
// heard.
func TestASpecialDoesNotGetFirstRefusalFromTheFloor(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.Find("Zod").Position = game.PosSleeping
	}); err != nil {
		t.Fatal(err)
	}

	// `buy` is POS_STANDING (interpreter.c:248). With no shopkeeper here it
	// would otherwise fall through to do_not_here's "You can't do that here!",
	// so either answer proves which check ran first.
	c.send("buy bread")
	c.expect("In your dreams, or what?")
	if c.seen("You can't do that here!") {
		t.Error("the command ran despite the position refusal")
	}
}

// TestAFrozenCharacterCannotType. `freeze` is how a god stops somebody without
// disconnecting them, and it is enforced in the interpreter rather than in each
// command — so it catches everything, including the commands that would
// otherwise work from any position.
func TestAFrozenCharacterCannotType(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// Frozen *and* mortal: the C exempts an implementor, so a god can always
	// thaw themselves out. The test character is made an implementor by being
	// first on the roster, hence the demotion.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		rec := w.Find("Zod").Record
		rec.Level = 10
		rec.PlayerFlags = rec.PlayerFlags.With(game.PlayerFrozen)
	}); err != nil {
		t.Fatal(err)
	}

	c.send("look")
	c.expect("You try, but the mind-numbing cold prevents you...")
}

// TestAFrozenImplementorCanStillType is the exemption, and the reason it
// exists: an implementor who freezes themselves by accident is not locked out
// of their own game.
func TestAFrozenImplementorCanStillType(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		rec := w.Find("Zod").Record
		if rec.Level != game.LevelImplementor {
			t.Errorf("the first character on the roster is level %d, expected an implementor", rec.Level)
		}
		rec.PlayerFlags = rec.PlayerFlags.With(game.PlayerFrozen)
	}); err != nil {
		t.Fatal(err)
	}

	c.send("look")
	c.expect("The Immortal Board Room")
	if c.seen("mind-numbing cold") {
		t.Error("an implementor was stopped by their own freeze")
	}
}
