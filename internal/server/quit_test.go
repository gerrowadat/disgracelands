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

// do_quit's own guards. `quit` is POS_DEAD in the command table, so the
// interpreter refuses nobody and every check is the command's own.

// mortalClient logs in an implementor first so the second character is an
// ordinary mortal — the guards mostly exempt immortals.
func mortalClient(t *testing.T, srv *Server, addr string) *client {
	t.Helper()
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	c := dialClient(t, addr)
	c.create("Mortal", "swordfish", "m", "w")
	return c
}

// TestQuiIsNotEnough. `qui` is a command of its own one line above `quit`
// (interpreter.c:421) — the C's way of making an abbreviation of a dangerous
// command refuse rather than act.
//
// Shorter than that does not even reach it: `quaff` is at :418 and `quest` at
// :420, so `q` and `qu` are quaff. Nothing between `q` and `quit` leaves the
// game, which is the point.
func TestQuiIsNotEnough(t *testing.T) {
	srv, _ := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))

	c.send("qui")
	c.expect("You have to type quit--no less, to quit!")

	c.send("q")
	c.expect("What do you want to quaff?")
}

// TestYouCannotQuitWhileFighting.
func TestYouCannotQuitWhileFighting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	c := mortalClient(t, srv, addr)
	spawnDog(t, srv, MortalStartRoom)

	c.send("hit dog")
	c.expect("a large dog") // present in every damage tier's text, hit or miss

	c.send("quit")
	c.expect("No way!  You're fighting for your life!")
}

// TestQuittingWhileDyingKillsYou, which is the C being literal: below stunned
// it does not refuse, it calls die().
func TestQuittingWhileDyingKillsYou(t *testing.T) {
	srv, _ := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		w.Find("Mortal").Position = game.PosMortallyWounded
	}); err != nil {
		t.Fatal(err)
	}

	c.send("quit")
	c.expect("You die before your time...")
}

// TestTooManyItemsRefusesTheQuit — a `<DoC>` local addition, and the first
// thing do_quit does. The count is of everything carried and worn, with
// containers counted through.
func TestTooManyItemsRefusesTheQuit(t *testing.T) {
	srv, _ := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))

	// Comfortably under first: the count is always reported.
	c.send("quit")
	c.expect("Saving 0 items.")

	// A fresh connection, and this time over the limit.
	c2 := dialClient(t, listening(t, srv))
	c2.login("Mortal", "swordfish")
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		ch := w.Find("Mortal")
		for i := int32(0); i <= game.MaxRent; i++ {
			w.ObjectToChar(w.NewObject(testSwordVnum), ch)
		}
	}); err != nil {
		t.Fatal(err)
	}

	c2.send("quit")
	c2.expect("You currently have too many items (29 items in total).")
	c2.expect("You must have 28 items or less before leaving.")
}

// TestAContainerCountsItsContents, which is what makes the limit bite: a bag
// of twenty things is twenty-one items, not one.
func TestAContainerCountsItsContents(t *testing.T) {
	srv, _ := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		ch := w.Find("Mortal")
		bag := w.NewObject(testBagVnum)
		w.ObjectToChar(bag, ch)
		for i := 0; i < 3; i++ {
			w.ObjectToObject(w.NewObject(testSwordVnum), bag)
		}
	}); err != nil {
		t.Fatal(err)
	}

	// One bag plus three swords.
	c.send("quit")
	c.expect("Saving 4 items.")
}

// TestAnImmortalIsExemptFromBoth: the item limit and the `qui` refusal are
// both guarded on being a mortal.
func TestAnImmortalIsExemptFromBoth(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("qui")
	c.expect("Goodbye, friend.. Come back soon!")
}

// TestShutdowIsNotEnough — the same guard, on the other command in the game
// that cannot be taken back.
func TestShutdowIsNotEnough(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("shutdow")
	c.expect("If you want to shut something down, say so!")
}

// `quit` returns to the main menu instead of disconnecting (issue #187).
//
// do_quit ends in extract_char (act.other.c:180) and extract_char_final puts
// the descriptor into CON_MENU and writes the menu to it (handler.c:931). A
// player who quits on the real server is back at "Make your choice:" and can
// enter the game again without dialling in; this port closed the connection.
func TestQuittingReturnsToTheMenu(t *testing.T) {
	srv, _ := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))

	c.send("quit")
	c.expect("Goodbye, friend.. Come back soon!")
	c.expectCount("Make your choice:", 2)

	// Out of the world, though: `quit` is a real departure, not a step back
	// into the lobby with the body left standing.
	var gone bool
	inWorld(t, srv, func(w *game.Live) { gone = w.Find("Mortal") == nil })
	if !gone {
		t.Error("the character is still in the world after quitting")
	}

	// And the connection is still open, which is the whole point.
	if c.seen("Goodbye.\r\n") {
		t.Error("quitting closed the connection")
	}
}

// And the same connection can go straight back in, which is what the menu is
// for. The C only frees the character when there is no descriptor
// (handler.c:988), so the record stays attached across the round trip.
func TestQuittingAndEnteringAgainOnTheSameConnection(t *testing.T) {
	srv, _ := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))

	c.send("quit")
	c.expectCount("Make your choice:", 2)

	c.send("1")
	c.expectCount("The Temple Of Midgaard", 2)

	var back bool
	inWorld(t, srv, func(w *game.Live) { back = w.Find("Mortal") != nil })
	if !back {
		t.Error("the character did not come back into the world")
	}

	c.send("look")
	c.expectCount("The Temple Of Midgaard", 3)
}

// The menu prints no H/M/V prompt after it. make_prompt's last two branches
// are both `STATE(d) == CON_PLAYING` and its else writes an empty prompt
// (comm.c:1010, :1048, :1051) — which only became reachable when quit stopped
// disconnecting.
func TestNoPlayingPromptAfterQuittingToTheMenu(t *testing.T) {
	srv, _ := newTestServer(t)
	c := mortalClient(t, srv, listening(t, srv))

	c.send("quit")
	c.expectCount("Make your choice:", 2)
	// No settle() here: `time` is not a menu choice, and the menu's answer to
	// one it does not know is to print itself again. Pressing Return is,
	// which redraws the menu and is a barrier for everything before it.
	c.send("")
	c.expectCount("Make your choice:", 3)

	tail := c.transcript()[strings.Index(c.transcript(), "Goodbye, friend"):]
	if strings.Contains(tail, "V > ") {
		t.Errorf("the playing prompt was printed at the menu:\n%s", tail)
	}
}
