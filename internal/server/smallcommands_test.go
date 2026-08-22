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

// since returns what a client has been sent after a recorded mark, which is
// how a test reads one command's output rather than the whole transcript.
// afterLastLook is no use here: a listing is printed *before* the prompt it
// ends with, so slicing at the last prompt drops the listing.
func since(c *client, mark int) string {
	text := c.transcript()
	if mark > len(text) {
		return text
	}
	return text[mark:]
}

// The tail of small commands: two aliases, insult, page, qecho and wizhelp.

// TestColonIsEmote. `:` is emote with no space, the way `'` is say — the
// one-character command path in split() is what makes it work.
func TestColonIsEmote(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	god.send(":waves.")
	god.expect("Zod waves.")
	mortal.settle()
	if !mortal.seen("Zod waves.") {
		t.Error("the room did not see the emote")
	}
}

// TestTakeIsGet, and the abbreviation that follows from it: `take` is two
// lines above `taste` in the C's table, so `ta` is take.
func TestTakeIsGet(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("take sword")
	c.expect("You don't see a sword here.")
}

// TestInsultSaysSomething. Three messages at random and the first branches on
// both sexes; the test only asserts that one of them arrives, since which is
// a roll.
func TestInsultSaysSomething(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	god.send("insult Bystander")
	god.expect("You insult Bystander.")
	mortal.settle()

	got := mortal.transcript()
	any := false
	for _, line := range []string{
		"fighting like a woman", "women can't fight",
		"smallest... (brain?)", "beauty contest against a troll",
		"calls your mother a bitch", "tells you to get lost",
	} {
		if strings.Contains(got, line) {
			any = true
		}
	}
	if !any {
		t.Error("the victim was not insulted with any of the six lines")
	}
}

// TestInsultYourself and the empty form, both of which have their own answer.
func TestInsultEdges(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	c.send("insult")
	c.expect("I'm sure you don't want to insult *everybody*...")
	c.send("insult Zod")
	c.expect("You feel insulted.")
	c.send("insult nobody")
	c.expect("Can't hear you!")
}

// TestPage rings the bell at one person.
func TestPage(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	god.send("page Bystander come here")
	god.expect("*Zod* come here")
	mortal.settle()
	if !mortal.seen("*Zod* come here") {
		t.Error("the page did not arrive")
	}
}

// TestPageAllNeedsMoreThanAGod — the only place in the game that asks for
// *above* LVL_GOD rather than at it.
func TestPageAllNeedsMoreThanAGod(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	// The implementor is above LVL_GOD, so this one works.
	god.send("page all everybody listen")
	god.settle()
	if god.seen("You will never be godly enough to do that!") {
		t.Error("an implementor was refused `page all`")
	}
}

// TestQechoIsUnattributed, where `qsay` wraps what you type and `qecho` does
// not. Both refuse anybody not on the quest.
func TestQechoIsUnattributed(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, mortal := twoInARoom(t, srv, addr)

	god.send("qecho anything")
	god.expect("You aren't even part of the quest!")

	god.send("quest")
	god.expect("Okay, you are part of the Quest!")
	mortal.send("quest")
	mortal.expect("Okay, you are part of the Quest!")

	god.send("qecho the sky darkens")
	god.expect("the sky darkens")
	mortal.settle()
	if !mortal.seen("the sky darkens") {
		t.Error("the quest channel did not carry the echo")
	}
	if mortal.seen("Zod quest-says") {
		t.Error("qecho attributed the line to its sender")
	}

	// The empty form spells the command's own name back at you.
	god.send("qecho")
	god.expect("Qecho?  Yes, fine, qecho we must, but WHAT??")
}

// TestWizhelpAndCommandsAreDisjoint. The C's filter is
// `(minimum_level >= LVL_IMMORT) != wizhelp`, so `commands` shows the mortal
// ones and `wizhelp` the immortal ones — a god typing `commands` does not see
// their own. Not what either name suggests.
func TestWizhelpAndCommandsAreDisjoint(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	mark := len(c.transcript())
	c.send("wizhelp")
	c.expect("The following privileged commands are available to you:")
	c.settle()
	wiz := since(c, mark)
	if !strings.Contains(wiz, "goto") {
		t.Error("wizhelp did not list goto")
	}
	if strings.Contains(wiz, "look") {
		t.Error("wizhelp listed a mortal command")
	}

	mark = len(c.transcript())
	c.send("commands")
	c.expect("The following commands are available to you:")
	c.settle()
	mortal := since(c, mark)
	if !strings.Contains(mortal, "look") {
		t.Error("commands did not list look")
	}
	if strings.Contains(mortal, "goto") {
		t.Error("commands listed an immortal command to a god")
	}
}

// TestSocialsListsInsult, because `insult` is a social that happens to be
// written in C rather than in the socials file — the C's filter says so.
func TestSocialsListsInsult(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "w")

	mark := len(c.transcript())
	c.send("socials")
	c.expect("The following socials are available to you:")
	c.settle()
	if !strings.Contains(since(c, mark), "insult") {
		t.Error("the socials list is missing insult")
	}
}
