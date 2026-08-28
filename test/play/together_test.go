// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build play

package play

import (
	"testing"
)

// Everything that needs a second person in the room. Graduation Hall's own
// closing note says these were the parts of the game the tutorial could not
// give a room to, because they need somebody to do them to.

// twoInARoom logs two ordinary mortals in, both standing in the start room.
//
// The order matters: the second is created after the first, so the first sees
// the arrival and the second does not. Tests that assert on what somebody was
// told rely on that being settled before they type anything.
func twoInARoom(t *testing.T, m *mud) (*client, *client) {
	t.Helper()

	first := m.dial()
	first.create("Alfie", "alfiepass", "m", "w")

	second := m.dial()
	second.create("Bertha", "berthapass", "f", "c")

	// Both are now in the room; drain the arrival notice so a later
	// assertion about what Alfie saw is about what the test caused.
	first.expect("Bertha has entered the game.")
	return first, second
}

// TestSayingThings: `say`, its one-character form, and that the room hears it.
//
// The apostrophe form is not a nicety -- split() gives the one-character
// commands their own path in the interpreter, and it is the form people
// actually type.
func TestSayingThings(t *testing.T) {
	m := start(t, miniClassic)
	alfie, bertha := twoInARoom(t, m)

	contains(t, "say", alfie.do("say hello there"), "You say, 'hello there'")
	contains(t, "what the room heard", bertha.do("look"), "Alfie says, 'hello there'")

	contains(t, "the ' form", alfie.do("'good morning"), "You say, 'good morning'")
	contains(t, "what the room heard", bertha.do("look"), "Alfie says, 'good morning'")

	m.noServerErrors()
}

// TestTellingAndWhispering: the directed forms, which go to one person and
// not to the room.
func TestTellingAndWhispering(t *testing.T) {
	m := start(t, miniClassic)
	alfie, bertha := twoInARoom(t, m)

	contains(t, "tell", alfie.do("tell Bertha psst"), "You tell Bertha, 'psst'")
	contains(t, "what Bertha heard", bertha.do("look"), "Alfie tells you, 'psst'")

	contains(t, "whisper", alfie.do("whisper Bertha over here"),
		"You whisper to Bertha, 'over here'")
	contains(t, "what Bertha heard", bertha.do("look"), "Alfie whispers to you, 'over here'")

	contains(t, "ask", alfie.do("ask Bertha are you there"),
		"You ask Bertha, 'are you there'")

	// Telling somebody who is not there.
	contains(t, "telling a stranger", alfie.do("tell Nobody hello"), "No-one by that name here.")

	m.noServerErrors()
}

// TestSocials: with a target, without one, and the abbreviation. The social
// table is a third of the command table (examples/mini/binary/misc/socials is
// the archive's own file), so this is also a check that it loaded at all.
func TestSocials(t *testing.T) {
	m := start(t, miniClassic)
	alfie, bertha := twoInARoom(t, m)

	// The exact wording is the socials file's, not the port's: these four
	// strings are lines in examples/mini/binary/misc/socials, which is the
	// archive's own copy. A social table that failed to load, or loaded and
	// lost its targeted forms, fails here rather than silently degrading to
	// "Huh?!?".
	contains(t, "an untargeted social", alfie.do("smile"), "You smile happily.")
	contains(t, "what the room saw", bertha.do("look"), "Alfie smiles happily.")

	contains(t, "a targeted social", alfie.do("wave Bertha"), "You wave goodbye to Bertha.")
	contains(t, "what the target saw", bertha.do("look"),
		"Alfie waves goodbye to you.  Have a good journey.")

	// A social aimed at somebody who is not here uses the social's own
	// no-target line rather than a generic refusal.
	contains(t, "a social at a stranger", alfie.do("wave Nobody"),
		"They didn't wait for you to wave goodbye.")

	m.noServerErrors()
}

// TestEmote, and the one-character form of it, which is `:` for the same
// reason `'` is say.
func TestEmote(t *testing.T) {
	m := start(t, miniClassic)
	alfie, bertha := twoInARoom(t, m)

	alfie.do("emote looks around slowly.")
	contains(t, "what the room saw", bertha.do("look"), "Alfie looks around slowly.")

	alfie.do(":taps a foot.")
	contains(t, "what the room saw", bertha.do("look"), "Alfie taps a foot.")

	m.noServerErrors()
}

// TestWho lists both of them, and `who` is what a player uses to find out
// whether anybody else is on at all.
func TestWho(t *testing.T) {
	m := start(t, miniClassic)
	alfie, _ := twoInARoom(t, m)

	contains(t, "who", alfie.do("who"), "Players", "Alfie", "Bertha", "2 characters playing.")

	m.noServerErrors()
}

// TestFollowingAndGrouping: `follow`, moving as a pair, `group` and
// `ungroup`.
func TestFollowingAndGrouping(t *testing.T) {
	m := start(t, miniClassic)
	alfie, bertha := twoInARoom(t, m)

	contains(t, "follow", bertha.do("follow Alfie"), "You now follow Alfie.")
	contains(t, "what the leader was told", alfie.do("look"), "Bertha starts following you.")

	// The whole point of following: the follower comes along.
	alfie.do("north")
	contains(t, "the follower came too", bertha.do("look"), "Hall of Movement")

	contains(t, "grouping", alfie.do("group Bertha"), "Bertha is now a member of your group.")

	// The leader is not in their own group until they say so: do_group's
	// no-argument branch is print_group, which refuses outright unless the
	// caller is AFF_GROUP (act.other.c:515). `group all` is what puts the
	// leader in, and that is the C's behaviour rather than an oversight
	// worth smoothing over.
	contains(t, "the leader before grouping themselves", alfie.do("group"),
		"But you are not the member of a group!")
	alfie.do("group all")
	contains(t, "the group listing", alfie.do("group"),
		"Your group consists of:", "Alfie", "Bertha")

	// Following yourself by name is how you stop (act.movement.c:721).
	contains(t, "stopping following", bertha.do("follow Bertha"),
		"You stop following Alfie.")

	m.noServerErrors()
}

// TestGivingSomethingToSomebody, which is the other half of `get`: an object
// changing hands, and the two messages that go with it.
func TestGivingSomethingToSomebody(t *testing.T) {
	m := start(t, miniClassic)
	alfie, bertha := twoInARoom(t, m)

	alfie.toRoom(roomArmory)
	alfie.do("get lantern")
	bertha.do("follow Alfie")
	// Walked separately rather than following, so that the test does not
	// depend on the door state the leader left behind.
	bertha.toRoom(roomArmory)

	contains(t, "giving", alfie.do("give lantern Bertha"),
		"You give a small brass lantern to Bertha.")
	contains(t, "receiving", bertha.do("inventory"), "a small brass lantern")
	missing(t, "the giver no longer has it", alfie.do("inventory"), "a small brass lantern")

	m.noServerErrors()
}

// TestQuittingIsSeenByTheRoom. Somebody leaving is an event other people
// notice, and it is also the point at which the server saves them.
func TestQuittingIsSeenByTheRoom(t *testing.T) {
	m := start(t, miniClassic)
	alfie, bertha := twoInARoom(t, m)

	bertha.quit()

	contains(t, "what the room saw", alfie.do("look"), "Bertha has left the game.")

	// `quit` takes the character out of the world in the command itself now
	// (issue #187: do_quit ends in extract_char and the connection goes to
	// the menu, it does not disconnect), so the room seeing the goodbye is
	// enough of a wait -- both happen on the world goroutine, in that order.
	// This used to need its own poll because the removal was deferred to the
	// connection's teardown.
	contains(t, "who after quitting", alfie.do("who"), "One lonely character displayed.")

	// And the quitter is at the menu, on the same connection, able to go
	// back in.
	contains(t, "the menu after quitting", bertha.expectCount("Make your choice:", 2),
		"Make your choice:")
	bertha.close()

	m.noServerErrors()
}
