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

// TestSneakingSuppressesTheMovementMessages is the half of AFF_SNEAK that was
// missing until #267: the skill was granted, the flag was set, and nothing
// anywhere read it for the thing the skill exists to do.
//
// do_simple_move wraps both messages in `!AFF_FLAGGED(ch, AFF_SNEAK)`
// (act.movement.c:163-170). The test is on the *mover's* flag alone — there
// is no per-observer roll, because `act(..., TO_ROOM)` is simply not called —
// so sneaking past a watchful god works exactly as well as sneaking past a
// sleeping rat, and the message is suppressed for everyone in the room or for
// nobody.
func TestSneakingSuppressesTheMovementMessages(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	// Walking openly, first, so the test knows the messages are there to be
	// suppressed.
	god.send("north")
	god.expect("The Immortal Board Room")
	mortal.settle()
	if !mortal.seen("Zod leaves north.") {
		t.Errorf("an ordinary departure was not announced; got:\n%s", afterLastLook(mortal))
	}

	god.send("south")
	god.expect("The Temple Of Midgaard")
	mortal.settle()
	if !mortal.seen("Zod has arrived.") {
		t.Errorf("an ordinary arrival was not announced; got:\n%s", afterLastLook(mortal))
	}

	affect(t, srv, "Zod", game.AffectSneak)
	before := strings.Count(mortal.transcript(), "Zod leaves north.")
	arrivals := strings.Count(mortal.transcript(), "Zod has arrived.")

	god.send("north")
	god.expect("The Immortal Board Room")
	mortal.settle()
	if got := strings.Count(mortal.transcript(), "Zod leaves north."); got != before {
		t.Errorf("a sneaking departure was announced %d times, want the earlier %d", got, before)
	}

	god.send("south")
	god.expect("The Temple Of Midgaard")
	mortal.settle()
	if got := strings.Count(mortal.transcript(), "Zod has arrived."); got != arrivals {
		t.Errorf("a sneaking arrival was announced %d times, want the earlier %d", got, arrivals)
	}
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
	//
	// Through `kill` rather than `look`, and the reason is worth keeping:
	// every proper prefix of "dog" — "d", "do", "dog" — is also an
	// abbreviation of "down", and do_look tries directions before targets
	// (act.informative.c:686), so `look do` looks at the floor. That is the
	// C's behaviour too, checked against it rather than assumed. `kill` has
	// no direction branch, so it is where the isname point still lives.
	spawnDog(t, srv, MortalStartRoom)
	mortal.send("kill do")
	mortal.expect("They don't seem to be here.")
	mortal.send("look dog")
	mortal.expect("You see nothing special about it.")

	// And a player, by a partial name.
	mortal.send("look zo")
	// The first occurrence, not the second: the `look do` above became a
	// `kill`, so this is now the only "You do not see that here." in the
	// transcript.
	mortal.expect("You do not see that here.")
	mortal.send("look zod")
	mortal.expect("You see nothing special about him.")
}

// TestYouCannotTargetWhatYouCannotSee — the other half of CAN_SEE, and the
// gap `docs/deviations.md` recorded when the display half landed: you could
// not see an invisible thief and could still `kill` them by name.
func TestYouCannotTargetWhatYouCannotSee(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	mortal.send("look zod")
	mortal.expect("You see nothing special about him.")

	affect(t, srv, "Zod", game.AffectInvisible)
	mortal.send("look zod")
	mortal.expect("You do not see that here.")

	// Detect invisible brings them back within reach.
	affect(t, srv, "Bystander", game.AffectDetectInvis)
	mortal.send("look zod")
	mortal.expectCount("You see nothing special about him.", 2)

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

	// Past the end finds nothing rather than the last one — and the refusal
	// says "a sword", not "a 9.sword". get_number rewrites the caller's
	// buffer (handler.c:596) and do_drop prints that same buffer, so the
	// count is gone by the time the player is told about it.
	c.send("drop 9.sword")
	c.expect("You don't seem to have a sword.")
}

// TestZeroDotMeansAPlayer. get_number returns 0 for a non-numeric prefix and
// for a literal `0.`, and get_char_room_vis reads that as "a player of this
// name" (handler.c:1074) — so `0.zod` finds the player and never a mobile.
//
// Through `hit`, not `look`, and the difference is the point. do_hit calls
// get_char_vis directly (act.offensive.c:108) and reaches that branch;
// look_at_target goes through generic_find, which gives up on a zero count
// before it searches anything at all (handler.c:1345), so `look 0.zod` is
// "Look at what?" no matter who zod is. Both halves checked against the C
// with scripts/session-parity.sh.
func TestZeroDotMeansAPlayer(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)
	spawnDog(t, srv, MortalStartRoom)

	mortal.send("look 0.zod")
	mortal.expect("Look at what?")

	mortal.send("hit 0.zod")
	mortal.expect("Use 'murder' to hit another player.")

	// A mobile is not a player, so 0. never reaches one — and the answer is
	// "Look at what?", not "You do not see that here.": look_at_target
	// re-reads the count after the character search and gives up on a zero
	// (act.informative.c:605-608), before any object or extra description is
	// looked at. Checked against the C with scripts/session-parity.sh.
	mortal.send("look 0.dog")
	mortal.expectCount("Look at what?", 2)

	// And a mobile is not a player, so `hit 0.dog` finds nobody either.
	mortal.send("hit 0.dog")
	mortal.expect("They don't seem to be here.")
}

// TestColourIsEmittedAtTheReadersLevel. The C interleaves escapes as it builds
// each message; this port writes the markup once and renders it at the socket.
// Either way what reaches the terminal depends on the reader.
func TestColourIsEmittedAtTheReadersLevel(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	// A new character has both PRF_COLOR bits, which is Complete.
	c.send("color")
	c.expect("Your current color level is Complete.")

	c.send("look")
	c.settle()
	if !strings.Contains(string(c.wire()), "\x1b[36m") {
		t.Error("the room title was not cyan for a reader on full colour")
	}

	c.send("color off")
	c.expect("is now Off.")

	before := len(c.wire())
	c.send("look")
	c.settle()
	if strings.Contains(string(c.wire()[before:]), "\x1b[") {
		t.Error("an escape sequence reached a reader who asked for no colour")
	}

	// And the markup does not leak either.
	if strings.Contains(string(c.wire()[before:]), "{{") {
		t.Error("the colour markup reached the player")
	}
}

// TestColourUsage, and that the level is matched on a prefix.
func TestColourUsage(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("color purple")
	c.expect("Usage: color { Off | Sparse | Normal | Complete }")

	c.send("color s")
	c.expect("is now Sparse.")
	c.send("color")
	c.expect("Your current color level is Sparse.")
}

// TestCompactDropsTheBlankLine before the prompt (comm.c:1436) — a preference
// that was settable, listed and saved, and read by nothing.
func TestCompactDropsTheBlankLine(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("compact")
	c.expect("Compact mode on.")
	before := len(c.wire())
	c.send("look")
	c.settle()
	if strings.Contains(string(c.wire()[before:]), "\r\n\r\n500H") {
		t.Error("compact mode still printed the blank line before the prompt")
	}

	c.send("compact")
	c.expect("Compact mode off.")
	before = len(c.wire())
	c.send("look")
	c.settle()
	if !strings.Contains(string(c.wire()[before:]), "\r\n500H") {
		t.Error("the blank line before the prompt went missing")
	}
}

// generic_find shares one count across every list it walks (issue #194).
//
// The C threads one `int *number` down the whole chain (handler.c:1387), so
// `2.bag` in `get sword from 2.bag` means the second bag across the *search
// order* — inventory, then the floor — and not the second bag in whichever
// list happens to hold one. This port searched each list with a counter of
// its own, so `2.bag` with one bag carried and one on the floor found
// nothing at all: neither list had two.
func TestNThingCountIsSharedAcrossTheListsGenericFindWalks(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		ch := w.Find("Zod")
		// One bag carried, with the sword in it; one bag on the floor,
		// empty. `2.bag` is the one on the floor.
		carried := w.NewObject(testBagVnum)
		w.ObjectToChar(carried, ch)
		w.ObjectToObject(w.NewObject(testSwordVnum), carried)

		w.ObjectToRoom(w.NewObject(testBagVnum), ch.Room)
	})

	// The floor's bag is the second one, and it is empty.
	c.send("get sword from 2.bag")
	c.expect("There doesn't seem to be a sword in a bag.")

	// The carried one is the first, and has the sword.
	c.send("get sword from 1.bag")
	c.expect("You get a long sword from a bag.")
}

// The same count, across equipment and inventory, through `look`.
//
// generic_find's order is fixed at worn, carried, on the floor (handler.c:1400
// onwards) regardless of which order the caller names the bits in, so with one
// sword worn and one carried `1.sword` is the worn one and `2.sword` the
// carried one.
func TestNThingCountRunsFromEquipmentIntoInventory(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	inWorld(t, srv, func(w *game.Live) {
		ch := w.Find("Zod")
		w.ObjectToChar(w.NewObject(testSwordVnum), ch)
		w.ObjectToChar(w.NewObject(testSwordVnum), ch)
	})
	c.send("wield sword")
	c.expect("You wield a long sword.")

	// One worn, one carried. Dropping 2.sword can only reach the carried
	// one — do_drop searches the inventory alone with a count of its own,
	// which is the C's, so this is the *other* half of the same behaviour
	// and it must not change.
	c.send("drop 2.sword")
	c.expect("You don't seem to have a sword.")

	// Through look, which is a generic_find caller: 1 is the worn one and 2
	// is the carried one, so both answer and 3 does not.
	c.send("look 1.sword")
	c.expectCount("You see nothing special..", 1)
	c.send("look 2.sword")
	c.expectCount("You see nothing special..", 2)
	c.send("look 3.sword")
	c.expect("You do not see that here.")
}

// The fight is coloured at the C's own call sites (issue #190): yellow to
// whoever swung, red to whoever was hit, and nothing at all to the room.
//
// Both are CCYEL/CCRED at **C_CMP** (fight.c:687-698, :732-764), the highest
// threshold there is, so a player on "normal" colour watches their own fight
// in plain text. That is the C's and it is why this test sets the level
// explicitly rather than trusting the default.
func TestTheFightIsColouredForTheTwoCombatantsOnly(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	attacker := dialClient(t, addr)
	attacker.create("Zod", "swordfish", "m", "w")
	victim := dialClient(t, addr)
	victim.create("Welmar", "swordfish", "m", "w")
	bystander := dialClient(t, addr)
	bystander.create("Watcher", "swordfish", "m", "w")

	for _, c := range []*client{attacker, victim, bystander} {
		c.send("color complete")
		c.expect("is now Complete.")
	}
	inWorld(t, srv, func(w *game.Live) {
		for _, name := range []string{"Zod", "Welmar", "Watcher"} {
			if who := w.Find(name); who != nil {
				if err := w.Enter(who, MortalStartRoom); err != nil {
					t.Errorf("moving %s: %v", name, err)
				}
			}
		}
	})

	attackerMark, victimMark, bystanderMark := len(attacker.wire()), len(victim.wire()), len(bystander.wire())

	// murder, not hit: player-killing is off in the test server and do_hit
	// refuses one player swinging at another without it (act.offensive.c).
	attacker.send("murder Welmar")
	attacker.settle()
	victim.settle()
	bystander.settle()

	if !strings.Contains(string(attacker.wire()[attackerMark:]), "\x1b[33m") {
		t.Error("the attacker's damage message was not yellow")
	}
	if !strings.Contains(string(victim.wire()[victimMark:]), "\x1b[31m") {
		t.Error("the victim's damage message was not red")
	}
	// TO_NOTVICT is wrapped in nothing: a bystander sees the fight in plain
	// text however much colour they have asked for.
	if strings.Contains(string(bystander.wire()[bystanderMark:]), "\x1b[3") {
		t.Errorf("a bystander's view of the fight was coloured:\n%q",
			string(bystander.wire()[bystanderMark:]))
	}
}

// A tell is red to whoever hears it at C_NRM and red to whoever sends it at
// C_CMP (act.comm.c:156, :164) — the same colour at two different
// thresholds, so a player on "normal" sees the tells they receive in red and
// their own echo in plain text.
func TestATellIsRedAtTwoDifferentThresholds(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	teller := dialClient(t, addr)
	teller.create("Zod", "swordfish", "m", "w")
	hearer := dialClient(t, addr)
	hearer.create("Welmar", "swordfish", "m", "w")

	teller.send("color normal")
	teller.expect("is now Normal.")

	tellerMark, hearerMark := len(teller.wire()), len(hearer.wire())
	teller.send("tell Welmar hello")
	teller.settle()
	hearer.settle()

	if !strings.Contains(string(hearer.wire()[hearerMark:]), "\x1b[31m") {
		t.Error("the tell was not red for the person who heard it")
	}
	if strings.Contains(string(teller.wire()[tellerMark:]), "\x1b[31m") {
		t.Error("the teller's own echo was red at Normal; it is C_CMP")
	}
}

// A channel carries its own colour from com_msgs' fourth column
// (act.comm.c:442-466): holler bright green, shout bright red, gossip yellow,
// auction magenta, congrat green.
func TestEachChannelHasItsOwnColour(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	speaker := dialClient(t, addr)
	speaker.create("Zod", "swordfish", "m", "w")
	hearer := dialClient(t, addr)
	hearer.create("Welmar", "swordfish", "m", "w")

	for _, tc := range []struct{ command, escape string }{
		{"gossip hello", "\x1b[33m"},
		{"auction hello", "\x1b[35m"},
		{"grats hello", "\x1b[32m"},
		{"holler hello", "\x1b[1;32m"},
	} {
		mark := len(hearer.wire())
		speaker.send(tc.command)
		speaker.settle()
		hearer.settle()
		if !strings.Contains(string(hearer.wire()[mark:]), tc.escape) {
			t.Errorf("%q did not arrive in %q:\n%q",
				tc.command, tc.escape, string(hearer.wire()[mark:]))
		}
	}
}
