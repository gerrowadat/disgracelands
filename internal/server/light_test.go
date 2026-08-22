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

// look_at_room's gates, through a real socket: darkness, blindness, and the
// three preferences that change what it prints.

// putInCellar moves the logged-in character into the dark room. The cellar has
// no exits on purpose, so it is reached this way rather than by walking.
func putInCellar(t *testing.T, srv *Server, name string) *game.Character {
	t.Helper()
	var ch *game.Character
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		ch = w.Find(name)
		if err := w.Enter(ch, CellarRoom); err != nil {
			t.Errorf("putting %s in the cellar: %v", name, err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	return ch
}

// TestADarkRoomIsPitchBlack.
//
// Note that the test character is an implementor — first on the roster — and
// still cannot see. PRF_HOLYLIGHT is set by `advance_level` (class.c:1920),
// and the first player is given level 34 by `init_char` and therefore never
// runs do_start, so they never advance a level and never get it. A god who
// wants to see in the dark has to type `holylight`.
func TestADarkRoomIsPitchBlack(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	putInCellar(t, srv, "Zod")

	c.send("look")
	c.expect("It is pitch black...")
	if c.seen("A Pitch Dark Cellar") {
		t.Error("the room name was printed in the dark")
	}
}

// TestHolylightSeesInTheDark, which is the immortal's half of CAN_SEE_IN_DARK,
// reached through the toggle this change adds.
func TestHolylightSeesInTheDark(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	putInCellar(t, srv, "Zod")

	c.send("look")
	c.expect("It is pitch black...")

	c.send("holylight")
	c.expect("HolyLight mode on.")

	c.send("look")
	c.expect("A Pitch Dark Cellar")
}

// TestInfravisionSeesInTheDark — the mortal's half.
func TestInfravisionSeesInTheDark(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	ch := putInCellar(t, srv, "Zod")
	if err := srv.engine.DoSync(context.Background(), func(_ *game.Live) {
		ch.Record.AffectFlags = ch.Record.AffectFlags.Set(game.AffectInfravision)
	}); err != nil {
		t.Fatal(err)
	}

	c.send("look")
	c.expect("A Pitch Dark Cellar")
}

// TestAWornTorchLightsTheRoom, and the finding underneath it: only the light
// *slot* counts, so putting the torch down puts the lights out.
func TestAWornTorchLightsTheRoom(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	ch := putInCellar(t, srv, "Zod")
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.ObjectToChar(w.NewObject(testTorchVnum), ch)
	}); err != nil {
		t.Fatal(err)
	}

	// Carrying it is not enough: the count is of what is worn in WEAR_LIGHT.
	c.send("look")
	c.expect("It is pitch black...")

	c.send("hold torch")
	c.expect("You light a torch and hold it.")
	c.send("look")
	c.expect("A Pitch Dark Cellar")

	// Back into the pack, and the lights go out again.
	c.send("remove torch")
	c.expect("You stop using a torch.")
	c.send("look")
	c.expectCount("It is pitch black...", 2)

	// On the floor, still burning, and the room stays dark — obj_to_room does
	// not touch the count, so a lit torch at your feet is no light at all.
	c.send("drop torch")
	c.expect("You drop a torch.")
	c.send("look")
	c.expectCount("It is pitch black...", 3)
}

// TestBlindnessIsReportedAfterDarkness. The C tests IS_DARK first, so a blind
// character standing in a dark room is told about the dark rather than about
// their eyes — and only sees "infinite darkness" somewhere lit.
func TestBlindnessIsReportedAfterDarkness(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	ch := putInCellar(t, srv, "Zod")
	if err := srv.engine.DoSync(context.Background(), func(_ *game.Live) {
		ch.Record.AffectFlags = ch.Record.AffectFlags.Set(game.AffectBlind)
	}); err != nil {
		t.Fatal(err)
	}

	// Dark and blind: darkness wins.
	c.send("look")
	c.expect("It is pitch black...")

	// Lit and blind: now the blindness shows.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		if err := w.Enter(ch, ImmortStartRoom); err != nil {
			t.Errorf("moving out of the cellar: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	c.send("look")
	c.expect("You see nothing but infinite darkness...")
}

// TestBriefModeDropsTheDescription, and `look` typed by hand ignores it —
// which is what look_at_room's ignore_brief argument is for, and the only
// caller in the whole C tree that passes 1.
func TestBriefModeDropsTheDescription(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("brief")
	c.expect("Brief mode on.")

	// Walking somewhere is the automatic look, which respects brief mode.
	c.send("south")
	c.expect("The Temple Of Midgaard")
	if c.seen("A temple.") {
		t.Error("brief mode still printed the room description on arrival")
	}

	// `look` typed on purpose shows it anyway.
	c.send("look")
	c.expect("A temple.")
}

// TestAutoexitCanBeSwitchedOff. The exits line is not unconditional in the C —
// it is `if (!IS_NPC(ch) && PRF_FLAGGED(ch, PRF_AUTOEXIT))`, and init_char
// turns the preference on, which is why it looks unconditional.
func TestAutoexitCanBeSwitchedOff(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("look")
	c.expect("[ Exits: s ]")

	c.send("autoexit")
	c.expect("Autoexits disabled.")

	// Count rather than expect: `expect` returns as soon as its marker is
	// anywhere in the transcript, and "[ Exits:" is already in it twice — once
	// from the look on entering the world and once from the look above. What
	// is being asserted is that no *third* one arrives.
	before := strings.Count(c.transcript(), "[ Exits:")
	c.send("look")
	c.settle()
	if after := strings.Count(c.transcript(), "[ Exits:"); after != before {
		t.Errorf("the exits line was printed %d more times with autoexit off", after-before)
	}
	if !c.seen("A board room.") {
		t.Error("the room description went missing along with the exits")
	}
}

// TestARoomWithNoExitsSaysNone. The C prints "None! " rather than an empty
// list, which is easy to miss because almost every room has a way out.
func TestARoomWithNoExitsSaysNone(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// Holylight first: the cellar is dark, and look_at_room never reaches the
	// exits line in the dark.
	c.send("holylight")
	c.expect("HolyLight mode on.")
	putInCellar(t, srv, "Zod")

	c.send("look")
	c.expect("[ Exits: None! ]")
}

// TestRoomflagsShowsTheVnum, the immortal toggle that this port had a
// preference bit for and no command to set.
func TestRoomflagsShowsTheVnum(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("roomflags")
	c.expect("You will now see the room flags.")

	c.send("look")
	c.expect("[ 1204] The Immortal Board Room [ ")
}

// TestRoomflagsIsImmortalOnly: the level is part of matching, so a mortal
// typing it gets "Huh?!?" rather than being told the command exists.
func TestRoomflagsIsImmortalOnly(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// The first character on the roster is the implementor; the second is an
	// ordinary mortal.
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")

	c := dialClient(t, addr)
	c.create("Mortal", "swordfish", "m", "w")

	c.send("roomflags")
	c.expect("Huh?!?")
}
