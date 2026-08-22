// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strings"
	"testing"
)

// The alias command, end to end through a real socket: internal/session's
// pure ExpandAlias logic is unit tested directly (alias_test.go there); this
// checks the whole path actually wired together — the command that defines
// an alias, and readLoop's own hook that expands one before it ever reaches
// command dispatch (session.go's expandAliasedLine).

func TestAliasSimpleExpandsToTheStoredCommand(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("alias b brief")
	c.expect("Alias added.")

	// Typing the alias runs brief's own command, not "b" as a command in
	// its own right — there is no such command, so if expansion did not
	// happen this would be "Huh?!?" instead.
	c.send("b")
	c.expect("Brief mode on.")
}

func TestAliasComplexRunsEachSemicolonSeparatedCommandInOrder(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("alias prep compact;autoexit")
	c.expect("Alias added.")

	c.send("prep")
	c.expect("Compact mode on.")
	// Autoexit starts on for a new character (ApplyNewCharacterDefaults,
	// create.go), so toggling it here turns it off.
	c.expect("Autoexits disabled.")
}

func TestAliasListShowsDefinitionsNewestFirst(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("alias b brief")
	c.expect("Alias added.")
	c.send("alias prep compact;autoexit")
	c.expectCount("Alias added.", 2)

	c.send("alias")
	c.expect("Currently defined aliases:")
	// expect only waits for that one marker, not for the listing lines
	// after it -- settle() drains the rest before the transcript is read.
	c.settle()
	transcript := c.transcript()
	prepAt := strings.Index(transcript, "\r\nprep")
	bAt := strings.Index(transcript, "\r\nb ")
	if prepAt < 0 || bAt < 0 || prepAt > bAt {
		t.Fatalf("expected prep (defined second) to list before b: transcript=%q", transcript)
	}
}

func TestAliasWithNoReplacementDeletesIt(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("alias b brief")
	c.expect("Alias added.")
	c.send("alias b")
	c.expect("Alias deleted.")
	c.send("alias b")
	c.expect("No such alias.")
}

func TestAliasCannotAliasAlias(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("alias alias brief")
	c.expect("You can't alias 'alias'.")
}

// TestAliasSurvivesAQuitAndRelogin exercises the ascii codec's tagAlis
// section (ascii/codec.go) — added alongside this command, since ascii is
// the server's own default and an alias that vanished on every relogin
// would not really be the feature. newTestServer runs on ascii/binary; the
// native-format equivalent is
// internal/persist/player/native's own round-trip test, since folding
// aliases into the one file needs no separate codec to prove there.
func TestAliasSurvivesAQuitAndRelogin(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "w")

	c.send("alias b brief")
	c.expect("Alias added.")

	c.send("quit")
	c.expect("Goodbye")
	c.close()
	waitForLogout(t, srv, "Zod")

	back := dialClient(t, addr)
	back.login("Zod", "swordfish")
	back.send("b")
	back.expect("Brief mode on.")
}
