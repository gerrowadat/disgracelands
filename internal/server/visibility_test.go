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

// CAN_SEE at the display sites, through real sockets.

// affect sets an AFF_* bit on a logged-in character.
func affect(t *testing.T, srv *Server, name string, flag game.Flags) {
	t.Helper()
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		ch := w.Find(name)
		if ch == nil || ch.Record == nil {
			t.Errorf("no character called %s", name)
			return
		}
		ch.Record.AffectFlags = ch.Record.AffectFlags.Set(flag)
	}); err != nil {
		t.Fatal(err)
	}
}

// twoInARoom logs in an implementor and a mortal and puts them together.
//
// They do not start together: the first character on the roster is an
// implementor and wakes in the immortal room, everybody else in the temple.
func twoInARoom(t *testing.T, srv *Server, addr string) (god, mortal *client) {
	t.Helper()
	god = dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	mortal = dialClient(t, addr)
	mortal.create("Bystander", "swordfish", "f", "w")

	// The god walks down to the temple rather than being teleported, so that
	// both clients have drained the arrival messages before a test starts
	// counting lines.
	god.send("south")
	god.expect("The Temple Of Midgaard")
	mortal.settle()
	return god, mortal
}

// TestAHiddenCharacterIsNotListed — the gap `docs/deviations.md` recorded for
// four phases: hiding set a flag that nothing read.
func TestAHiddenCharacterIsNotListed(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	mortal.send("look")
	mortal.expect("Zod ")

	affect(t, srv, "Zod", game.AffectHide)
	mortal.send("look")
	mortal.settle()
	before := strings.Count(mortal.transcript(), "Zod ")

	mortal.send("look")
	mortal.settle()
	if after := strings.Count(mortal.transcript(), "Zod "); after != before {
		t.Error("a hidden character was still listed in the room")
	}
}

// TestSenseLifeFindsAHiddenCharacter, and the marker that says why: the C
// appends " (hidden)" to the line, so somebody who can see them knows they
// are not meant to be seen.
func TestSenseLifeFindsAHiddenCharacter(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	affect(t, srv, "Zod", game.AffectHide)
	affect(t, srv, "Bystander", game.AffectSenseLife)

	mortal.send("look")
	mortal.expect("(hidden)")
}

// TestAnInvisibleCharacterIsNotListed, and detect invisible reveals them with
// the "(invisible)" marker.
func TestAnInvisibleCharacterIsNotListed(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	affect(t, srv, "Zod", game.AffectInvisible)
	mortal.send("look")
	mortal.settle()
	if strings.Contains(afterLastLook(mortal), "Zod") {
		t.Error("an invisible character was listed in the room")
	}

	affect(t, srv, "Bystander", game.AffectDetectInvis)
	mortal.send("look")
	mortal.expect("(invisible)")
}

// afterLastLook is the transcript since the second-to-last prompt, which is
// roughly "what the last command printed". Good enough to ask whether a name
// appeared in a particular room listing.
func afterLastLook(c *client) string {
	text := c.transcript()
	if i := strings.LastIndex(text, "V > "); i >= 0 {
		return text[i:]
	}
	return text
}

// TestSneakingDoesNotHideYouStandingStill. AFF_SNEAK is not in INVIS_OK — it
// conceals movement, not the person — and the three are granted close enough
// together that assuming otherwise is easy.
func TestSneakingDoesNotHideYouStandingStill(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	affect(t, srv, "Zod", game.AffectSneak)
	mortal.send("look")
	mortal.expect("Zod ")
}

// TestAnInvisibleCharacterIsSomeoneInAct. PERS resolves per audience, so the
// same social produces a name for one bystander and "someone" for another.
func TestAnInvisibleCharacterIsSomeoneInAct(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	affect(t, srv, "Zod", game.AffectInvisible)

	god.send("smile")
	god.expect("You smile happily.")
	mortal.settle()
	if !mortal.seen("Someone smiles happily.") {
		t.Errorf("an invisible smiler was not anonymous; got:\n%s", afterLastLook(mortal))
	}
	if mortal.seen("Zod smiles happily.") {
		t.Error("an invisible character was named by a social")
	}
}

// TestWhoHidesAnInvisGod, which is what `invis` is for.
func TestWhoHidesAnInvisGod(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	mortal.send("who")
	mortal.expect("Zod")

	god.send("invis")
	god.expect("Your invisibility level is 34.")

	mortal.send("who")
	mortal.settle()
	if strings.Contains(afterLastLook(mortal), "Zod") {
		t.Error("an invis god was on the who-list")
	}
	// And they can still see themselves: SELF is the first test in CAN_SEE.
	god.send("who")
	god.expect("Zod")
}

// TestAnInvisibleObjectIsNotOnTheFloor.
func TestAnInvisibleObjectIsNotOnTheFloor(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	var sword *game.Object
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		sword = w.NewObject(testSwordVnum)
		w.ObjectToRoom(sword, ImmortStartRoom)
	}); err != nil {
		t.Fatal(err)
	}

	c.send("look")
	c.expect("A long sword is lying here.")

	if err := srv.engine.DoSync(context.Background(), func(_ *game.Live) {
		sword.ExtraFlags = sword.ExtraFlags.Set(game.ItemInvisible)
	}); err != nil {
		t.Fatal(err)
	}
	c.send("look")
	c.settle()
	if strings.Contains(afterLastLook(c), "A long sword is lying here.") {
		t.Error("an invisible sword was listed on the floor")
	}
}

// TestGlowingRedEyes is list_char_to_char's `else if`, and the one place
// somebody you cannot see is not silent.
//
// Reachable only through `do_look`'s own darkness branch, which prints "It is
// pitch black..." and then calls list_char_to_char anyway — the C's comment on
// that line is just "glowing red eyes". `look_at_room` returns before its
// listing, so *arriving* in a dark room shows nothing.
//
// Note whose infravision it is. Theirs, not yours: it is the creature's own
// night vision that gives it away.
func TestGlowingRedEyes(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	// Both into the dark cellar; give the *other* character infravision.
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		for _, name := range []string{"Zod", "Bystander"} {
			if err := w.Enter(w.Find(name), CellarRoom); err != nil {
				t.Errorf("moving %s: %v", name, err)
			}
		}
		zod := w.Find("Zod")
		zod.Record.AffectFlags = zod.Record.AffectFlags.Set(game.AffectInfravision)
	}); err != nil {
		t.Fatal(err)
	}

	mortal.send("look")
	mortal.expect("You see a pair of glowing red eyes looking your way.")
}

// TestNoGlowingEyesWithoutInfravision: the same dark room, and somebody with
// no night vision of their own is simply not there.
func TestNoGlowingEyesWithoutInfravision(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		for _, name := range []string{"Zod", "Bystander"} {
			if err := w.Enter(w.Find(name), CellarRoom); err != nil {
				t.Errorf("moving %s: %v", name, err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	mortal.send("look")
	mortal.expect("It is pitch black...")
	mortal.settle()
	if mortal.seen("glowing red eyes") {
		t.Error("saw glowing eyes on somebody with no infravision")
	}
}

// TestLookWhileBlindSaysSomethingElse. do_look tests blindness *before*
// darkness and uses its own wording; look_at_room tests darkness first and
// uses different wording again. Both are reachable: one by typing `look`, the
// other by walking into the room.
func TestLookWhileBlindSaysSomethingElse(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	affect(t, srv, "Zod", game.AffectBlind)

	c.send("look")
	c.expect("You can't see a damned thing, you're blind!")

	// Walking into a lit room takes look_at_room's path instead.
	c.send("south")
	c.expect("You see nothing but infinite darkness...")
}

// TestPositionsInTheRoomList. Everybody used to be "standing here" whatever
// they were doing; list_one_char has a line per position.
func TestPositionsInTheRoomList(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	other, mortal := twoInARoom(t, srv, addr)

	for _, tc := range []struct{ command, expect string }{
		{"sit", "is sitting here."},
		{"rest", "is resting here."},
		{"sleep", "is sleeping here."},
	} {
		other.send(tc.command)
		other.settle()
		mortal.send("look")
		mortal.expect(tc.expect)
	}
}

// TestAPlayerIsListedWithTheirTitle, which is what the C prints — "%s %s",
// name and title — where this port had printed the name alone.
func TestAPlayerIsListedWithTheirTitle(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	other, mortal := twoInARoom(t, srv, addr)

	other.send("title the Tester")
	other.expect("Okay, you're now Zod the Tester.")

	mortal.send("look")
	mortal.expect("Zod the Tester is standing here.")
}

// TestALinklessBodyIsMarked. The C says so in the room list, which is how you
// tell a linkdead player from one who is simply quiet.
func TestALinklessBodyIsMarked(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	other, mortal := twoInARoom(t, srv, addr)

	other.close()
	// Wait for the disconnect to have been *handled*, not merely sent.
	// Closing the socket returns immediately; the body is still linked until
	// the connection goroutine's teardown runs, and settle() is no barrier for
	// that — it waits on this client's socket, not on the other one's
	// teardown. The room being told is the signal.
	mortal.expect("Zod has lost their link.")

	mortal.send("look")
	mortal.expect("(linkless)")
}

// TestAnAbbreviationDoesNotNameAnybody, through a socket.
//
// isname() is a whole-word match (handler.c:56), so `kill dra` finds no dragon
// and `kill zo` finds no Zod. This port matched prefixes until an oracle said
// otherwise; the change makes the game stricter, and it is what players typed
// against for seven years.
func TestAnAbbreviationDoesNotNameAnybody(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	// A mobile, by a partial keyword and then a whole one.
	spawnDog(t, srv, MortalStartRoom)
	mortal.send("look do")
	mortal.expect("You do not see that here.")
	mortal.send("look dog")
	mortal.expect("You see nothing special about a large dog.")

	// And a player, by a partial name.
	mortal.send("look zo")
	mortal.expectCount("You do not see that here.", 2)
	mortal.send("look zod")
	mortal.expect("You see nothing special about Zod.")
}

// TestYouCannotTargetWhatYouCannotSee — the other half of CAN_SEE, and the
// gap `docs/deviations.md` recorded when the display half landed: you could
// not see an invisible thief and could still `kill` them by name.
func TestYouCannotTargetWhatYouCannotSee(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	mortal.send("look zod")
	mortal.expect("You see nothing special about Zod.")

	affect(t, srv, "Zod", game.AffectInvisible)
	mortal.send("look zod")
	mortal.expect("You do not see that here.")

	// Detect invisible brings them back within reach.
	affect(t, srv, "Bystander", game.AffectDetectInvis)
	mortal.send("look zod")
	mortal.expectCount("You see nothing special about Zod.", 2)

	_ = god
}

// TestNThingTargeting is get_number, threaded through the search at last:
// `2.sword` is the second sword rather than a keyword nobody has.
func TestNThingTargeting(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		ch := w.Find("Zod")
		for i := 0; i < 3; i++ {
			w.ObjectToChar(w.NewObject(testSwordVnum), ch)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// All three answer to "sword"; the count picks which.
	c.send("drop 2.sword")
	c.expect("You drop a long sword.")
	c.send("inventory")
	c.expect("a long sword")

	// Past the end finds nothing rather than the last one.
	c.send("drop 9.sword")
	c.expect("You don't seem to have a 9.sword.")
}

// TestZeroDotMeansAPlayer. get_number returns 0 for a non-numeric prefix and
// for a literal `0.`, and a character search reads that as "a player of this
// name" (handler.c:1068) — so `0.zod` finds the player and never a mobile.
func TestZeroDotMeansAPlayer(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	spawnDog(t, srv, MortalStartRoom)

	mortal.send("look 0.zod")
	mortal.expect("You see nothing special about Zod.")

	// A mobile is not a player, so 0. never reaches one.
	mortal.send("look 0.dog")
	mortal.expect("You do not see that here.")
}
