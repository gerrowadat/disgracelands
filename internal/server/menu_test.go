// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

// newMenuClient makes a character and leaves it sitting at the main menu.
func newMenuClient(t *testing.T, srv *Server, name string) *client {
	t.Helper()
	c := dialClient(t, listening(t, srv))
	c.expect("By what name")
	c.send(name)
	c.expect("Did I get that right")
	c.send("y")
	c.expect("Give me a password")
	c.send("swordfish")
	c.expect("retype password")
	c.send("swordfish")
	c.expect("What is your sex")
	c.send("m")
	c.expect("Class:")
	c.send("w")
	c.expect("PRESS RETURN")
	c.send("")
	c.expect("Make your choice:")
	return c
}

// TestTheMenuIsShownAndIsTheCs. A player does not walk into the world off the
// message of the day; the C stops at CON_MENU (interpreter.c:1637) and so
// does this.
func TestTheMenuIsShownAndIsTheCs(t *testing.T) {
	srv, _ := newTestServer(t)
	c := newMenuClient(t, srv, "Zod")

	if !strings.Contains(c.transcript(), MainMenu) {
		t.Errorf("the menu is not the C's, verbatim:\n%q", c.transcript())
	}
	// And nothing has entered the world yet.
	if c.seen("The Immortal Board Room") {
		t.Errorf("the character was put into the world without choosing to:\n%s", c.transcript())
	}
}

func TestAnUnknownMenuChoiceIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	c := newMenuClient(t, srv, "Zod")

	c.send("9")
	c.expect("That's not a menu choice!")
	c.expectCount("Make your choice:", 2)
	if c.seen("The Immortal Board Room") {
		t.Error("a bad menu choice entered the world anyway")
	}
}

// TestMenuChoiceZeroLeaves without entering the world at all.
func TestMenuChoiceZeroLeaves(t *testing.T) {
	srv, _ := newTestServer(t)
	c := newMenuClient(t, srv, "Zod")

	c.send("0")
	c.expect("Goodbye.")
	if c.seen("The Immortal Board Room") {
		t.Error("choosing to leave entered the world first")
	}
}

// TestMenuChoiceThreeShowsTheBackground, and comes back to the menu on the
// next line typed — which is what the C does by dropping into CON_RMOTD.
func TestMenuChoiceThreeShowsTheBackground(t *testing.T) {
	srv, _ := newTestServer(t)
	c := newMenuClient(t, srv, "Zod")

	c.send("3")
	c.expect("BACKGROUND-FILE")

	c.send("")
	c.expectCount("Make your choice:", 2)
}

// TestMenuChoiceThreeWithLongBackgroundPaginates: page_string
// (interpreter.c:1713) pages `background` too, from CON_MENU rather than
// CON_PLAYING — the one caller docs/deviations.md's pager entry used to
// name as "not ported". Confirms the pager's own return-state tracking
// (Session.pagerReturn) rather than only the short-text path
// TestMenuChoiceThreeShowsTheBackground already covers: the menu comes
// back once paging finishes, not the ordinary game prompt.
func TestMenuChoiceThreeWithLongBackgroundPaginates(t *testing.T) {
	srv, _ := newTestServer(t)
	if !srv.SetTextField("background", longNews()) {
		t.Fatal("SetTextField(background) refused")
	}

	c := newMenuClient(t, srv, "Zod")

	c.send("3")
	c.expect("News item 1.")
	c.expect("Return to continue")
	if c.seen("News item 23.") {
		t.Error("the first page already shows content past PAGE_LENGTH (22)")
	}

	c.send("")
	c.expect("News item 23.")

	// StateReadMOTD, restored once the pager closes, shows the menu on
	// the *next* line typed rather than automatically — CON_RMOTD's own
	// behaviour, unchanged by paging having been in the way.
	c.send("")
	// The menu, not the ordinary game prompt: the C leaves the
	// connection in CON_RMOTD once background's own paging finishes, and
	// so does this — the same thing the short-text path already proves,
	// now proven through a pager that actually opened.
	c.expectCount("Make your choice:", 2)
	if c.seen("V > ") {
		t.Error("the ordinary game prompt appeared after background's own pager closed")
	}
}

// TestTheDescriptionEditor covers menu choice 2 end to end, including that
// what was typed is what `look` will show.
func TestTheDescriptionEditor(t *testing.T) {
	srv, store := newTestServer(t)
	c := newMenuClient(t, srv, "Zod")

	c.send("2")
	c.expect("Terminate with a '@' on a new line.")
	c.send("A short, angry man.")
	c.send("He is holding a clipboard.")
	c.send("@")
	c.expectCount("Make your choice:", 2)

	rec, err := store.Load(context.Background(), "Zod")
	if err != nil {
		t.Fatal(err)
	}
	// The ascii format stores multi-line text with bare newlines and no
	// trailing one; Session.Send puts the carriage returns back on the way
	// out. What matters is that both lines survived, in order.
	want := "A short, angry man.\nHe is holding a clipboard."
	if rec.Description != want {
		t.Errorf("description saved as %q, want %q", rec.Description, want)
	}

	// Re-opening the editor shows what is already there.
	c.send("2")
	c.expect("Old description:")
	if !strings.Contains(c.transcript(), "A short, angry man.") {
		t.Errorf("the old description was not shown:\n%s", c.transcript())
	}
	c.send("@")
	c.expectCount("Make your choice:", 3)
}

// TestADescriptionIsCappedAtTheFormatsLimit, so that a description written
// here still round-trips through the binary format the C reads.
func TestADescriptionIsCappedAtTheFormatsLimit(t *testing.T) {
	srv, store := newTestServer(t)
	c := newMenuClient(t, srv, "Zod")

	c.send("2")
	c.expect("Terminate with a '@' on a new line.")
	for i := 0; i < 20; i++ {
		c.send(strings.Repeat("x", 60))
	}
	c.send("@")
	c.expect("truncated")

	rec, err := store.Load(context.Background(), "Zod")
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Description) > 240 {
		t.Errorf("description is %d bytes, want at most 240", len(rec.Description))
	}
}

// TestChangingAPassword covers menu choice 4, including that the old password
// stops working and the new one starts.
func TestChangingAPassword(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	c := newMenuClient(t, srv, "Zod")

	// A wrong old password gets nowhere.
	c.send("4")
	c.expect("Enter your old password:")
	c.send("not-it")
	c.expect("Incorrect password.")
	c.expectCount("Make your choice:", 2)

	// The right one does.
	c.send("4")
	c.expectCount("Enter your old password:", 2)
	c.send("swordfish")
	c.expect("Enter a new password:")

	// Mismatched confirmations start over.
	c.send("hunter2!")
	c.expect("retype password")
	c.send("hunter3!")
	c.expect("Passwords don't match... start over.")

	c.send("hunter2!")
	c.expectCount("retype password", 2)
	c.send("hunter2!")
	c.expect("Done.")
	c.send("0")
	c.expect("Goodbye.")
	c.close()

	// The new password works.
	back := dialClient(t, addr)
	back.expect("By what name")
	back.send("Zod")
	back.expect("Password:")
	back.send("hunter2!")
	back.expect("PRESS RETURN")

	// The old one does not.
	stale := dialClient(t, addr)
	stale.expect("By what name")
	stale.send("Zod")
	stale.expect("Password:")
	stale.send("swordfish")
	stale.expect("Wrong password")
}

// TestDeletingACharacter covers menu choice 5 and both of its confirmations.
func TestDeletingACharacter(t *testing.T) {
	srv, store := newTestServer(t)
	addr := listening(t, srv)

	// The first character on the roster is an Implementor, and an Implementor
	// is above the level at which the C actually deletes. Make one first, so
	// this test is about an ordinary mortal.
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := newMenuClient(t, srv, "Welmar")

	// A wrong password gets nowhere.
	c.send("5")
	c.expect("Enter your password for verification:")
	c.send("not-it")
	c.expect("Incorrect password.")

	// Nor does anything but a literal "yes".
	c.expectCount("Make your choice:", 2)
	c.send("5")
	c.expectCount("Enter your password for verification:", 2)
	c.send("swordfish")
	c.expect("ARE YOU ABSOLUTELY SURE?")
	c.send("y")
	c.expect("Character not deleted.")

	if _, err := store.Load(context.Background(), "Welmar"); err != nil {
		t.Fatalf("the character was deleted by an unconfirmed answer: %v", err)
	}

	// "yes" does it.
	c.expectCount("Make your choice:", 3)
	c.send("5")
	c.expectCount("Enter your password for verification:", 3)
	c.send("swordfish")
	c.expectCount("ARE YOU ABSOLUTELY SURE?", 2)
	c.send("yes")
	c.expect("deleted!")

	if _, err := store.Load(context.Background(), "Welmar"); !errors.Is(err, player.ErrNotFound) {
		t.Errorf("the character is still on the roster: %v", err)
	}

	// And the name is free again.
	again := dialClient(t, addr)
	again.expect("By what name")
	again.send("Welmar")
	if got := again.expect("Did I get that right"); !strings.Contains(got, "Did I get that right, Welmar") {
		t.Errorf("a deleted character's name was not free again:\n%s", got)
	}
}

// TestAGreaterGodCannotSelfDelete. The C only sets PLR_DELETED below
// LVL_GRGOD, so a god who types their way through both confirmations is
// disconnected and still there afterwards. It reads like a safety valve, and
// it is kept as one.
func TestAGreaterGodCannotSelfDelete(t *testing.T) {
	srv, store := newTestServer(t)

	// The first character is an Implementor, which is above LVL_GRGOD.
	c := newMenuClient(t, srv, "Zod")

	c.send("5")
	c.expect("Enter your password for verification:")
	c.send("swordfish")
	c.expect("ARE YOU ABSOLUTELY SURE?")
	c.send("yes")
	c.expect("deleted!")

	rec, err := store.Load(context.Background(), "Zod")
	if err != nil {
		t.Fatalf("an Implementor was removed from the roster: %v", err)
	}
	if rec.Level != game.LevelImplementor {
		t.Errorf("level is %d, want %d", rec.Level, game.LevelImplementor)
	}
	if rec.PlayerFlags.Has(game.PlayerDeleted) {
		t.Error("an Implementor was marked deleted")
	}
}

// TestEchoIsRestoredOnEveryMenuPasswordPath: the menu asks for a password in
// two places and each has a failure path, and a player left with echo off is
// a player typing blind.
func TestEchoIsRestoredOnEveryMenuPasswordPath(t *testing.T) {
	srv, _ := newTestServer(t)

	for _, tc := range []struct {
		name      string
		character string
		choice    string
		prompt    string
	}{
		{"changing a password", "Zod", "4", "Enter your old password:"},
		// A separate character: the first has already been created by the
		// subtest above, and would be asked for a password rather than
		// walked through creation.
		{"deleting a character", "Welmar", "5", "Enter your password for verification:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newMenuClient(t, srv, tc.character)
			c.send(tc.choice)
			c.expect(tc.prompt)
			before := len(c.wire())

			c.send("not-it")
			c.expect("Incorrect password.")

			if !echoRestored(c.wire()[before:]) {
				t.Errorf("echo was not turned back on after a failed password: % x", c.wire()[before:])
			}
		})
	}
}
