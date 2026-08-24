// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build play

package play

import "testing"

// The immortal side of the game, played rather than unit-tested.
//
// These commands are the ones an operator uses on a live server, and most of
// them only mean anything against a real loaded world: `goto` needs rooms
// that exist, `load` needs prototypes, `zreset` needs a zone with reset
// commands in it, `stat` needs something to stat. None of that exists in
// internal/server's synthetic world, which is why they are here.

// god logs the implementor the harness created back in.
func (m *mud) god() *client {
	m.t.Helper()

	c := m.dial()
	c.login(founderName, founderPassword)
	return c
}

// TestTheImplementorCanMoveAround: goto by vnum, and back.
func TestGoto(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	contains(t, "goto a vnum", c.do("goto 3011"), "The General Store")
	contains(t, "goto another", c.do("goto 3001"), "The Testing Grounds")
	contains(t, "goto a room that is not there", c.do("goto 9999"), "No room exists with that number.")

	m.noServerErrors()
}

// TestStat, on a room, a player, a mobile and an object -- four different
// branches of do_stat, all of which need real loaded data to say anything.
func TestStat(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	contains(t, "stat room", c.do("stat room"), "Room name:", "The Testing Grounds", "Zone:")
	contains(t, "stat a player", c.do("stat Founder"),
		"PC 'Founder'", "Title: the Implementor", "Class: Warrior", "Lev: [34]")

	c.do("goto 3008")
	contains(t, "stat a mobile", c.do("stat dummy"), "a training dummy", "Alias: dummy training")
	contains(t, "stat an object", c.do("stat wand"), "a wand")

	m.noServerErrors()
}

// TestLoadAndPurge: making something out of nothing and then unmaking it,
// which is how an operator fixes a world that has gone wrong.
func TestLoadAndPurge(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	contains(t, "load a mobile", c.do("load mob 130"), "a training dummy")
	contains(t, "it is in the room", c.do("look"), "A straw-stuffed training dummy stands here")

	// A loaded object goes into the loader's hands, not onto the floor:
	// do_load's obj_to_char for anything takeable.
	contains(t, "load an object", c.do("load obj 140"), "a training sword")
	contains(t, "it is in hand", c.do("inventory"), "a training sword")

	c.do("purge dummy")
	missing(t, "the mobile is gone", c.do("look"), "straw-stuffed training dummy stands here")

	contains(t, "loading a vnum that does not exist", c.do("load mob 9999"),
		"There is no monster with that number.")

	m.noServerErrors()
}

// TestSet changes a field on a character who is logged in, and the change is
// visible where a player would see it.
func TestSet(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	contains(t, "set gold", c.do("set Founder gold 500"), "Founder's gold set to 500.")
	contains(t, "the new gold in score", c.do("score"), "500 gold coins")

	contains(t, "an unknown field", c.do("set Founder nosuchfield 1"), "Can't set that!")

	m.noServerErrors()
}

// TestUsersAndWho. `users` is the immortal's view: the connections, not the
// characters, which is the difference that matters when somebody is stuck at
// a login prompt.
func TestUsersAndWho(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	mortal := m.dial()
	mortal.create("Tourist", "tourpass", "m", "w")

	contains(t, "who", c.do("who"), "Founder", "Tourist")
	contains(t, "users", c.do("users"), "Founder", "Tourist", "Playing")

	m.noServerErrors()
}

// TestForceAndEcho: making somebody else act, and speaking as the world
// rather than as a character.
func TestForceAndEcho(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	mortal := m.dial()
	mortal.create("Tourist", "tourpass", "m", "w")

	c.do("force Tourist smile")
	contains(t, "what the forced character did", mortal.do("look"), "You smile happily.")

	c.do("echo A cold wind blows through the room.")
	contains(t, "what the room heard", mortal.do("look"), "A cold wind blows through the room.")

	m.noServerErrors()
}

// TestTransferAndAt: moving somebody else, and acting somewhere you are not.
func TestTransferAndAt(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	mortal := m.dial()
	mortal.create("Tourist", "tourpass", "m", "w")

	c.do("goto 3011")
	c.do("transfer Tourist")
	contains(t, "where the transferred character is", mortal.do("look"), "The General Store")

	// `at` runs one command somewhere else and comes back.
	contains(t, "at", c.do("at 3001 look"), "The Testing Grounds")
	contains(t, "back where it started", c.do("look"), "The General Store")

	m.noServerErrors()
}

// TestWizinvis. An invisible immortal is not in the room as far as a mortal
// is concerned, which is the whole point of it.
func TestWizinvis(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	mortal := m.dial()
	mortal.create("Tourist", "tourpass", "m", "w")
	c.do("goto 3001")

	contains(t, "before going invisible", mortal.do("look"),
		"Founder the Implementor is standing here.")

	// Each check below drains the mortal's socket with a command of its own
	// first. The "you blink and suddenly realize" and "suddenly appears"
	// notices are written when the immortal types, not when the mortal does,
	// and both contain the name -- so asserting on a `look` that still had
	// one of them queued in front of it would match the notice rather than
	// the room.
	c.do("invis")
	mortal.do("score")
	missing(t, "while invisible", mortal.do("look"), "Founder the Implementor is standing here.")

	// `visible` is do_visible, whose immortal branch is perform_immort_vis
	// (act.other.c:404). It used to have no immortal branch at all -- it
	// tested AFF_INVISIBLE, which a wizinvis god does not have, said "You
	// are already visible" and left them invisible, so toggling `invis` a
	// second time was the only way back. This is the test that found it.
	contains(t, "`visible` while wizinvis", c.do("visible"), "You are now fully visible.")
	mortal.do("score")
	contains(t, "after coming back", mortal.do("look"),
		"Founder the Implementor is standing here.")

	// And appear()'s own message is what the room is told (fight.c:98-101),
	// which for an immortal is this and not the "standing beside you" line
	// perform_immort_invis uses when a god merely lowers their level.
	contains(t, "what the room was told", mortal.transcript(),
		"You feel a strange presence as Founder appears, seemingly from nowhere.")

	// A second `visible` has nothing left to do.
	contains(t, "`visible` when already visible", c.do("visible"),
		"You are already fully visible.")

	// `invis` with no argument still toggles (do_invis, act.wizard.c:1589).
	contains(t, "toggling out again", c.do("invis"), "Your invisibility level is 34.")

	m.noServerErrors()
}

// TestRestore puts a character back to full, which is the command an operator
// reaches for after something has gone wrong in a fight.
func TestRestore(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	mortal := m.dial()
	mortal.create("Tourist", "tourpass", "m", "w")

	c.do("set Tourist hit 1")
	contains(t, "after being hurt", mortal.do("score"), "You have 1(")

	c.do("restore Tourist")
	contains(t, "what the restored character was told", mortal.do("look"),
		"You have been fully healed by Founder!")

	m.noServerErrors()
}

// TestWizhelpLists the immortal commands, which is the only discoverable
// index of them there is.
func TestWizhelp(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	contains(t, "wizhelp", c.doUntil("wizhelp", promptMarker), "goto", "stat", "purge")

	m.noServerErrors()
}

// TestAMortalCannotUseImmortalCommands. The level gate is the only thing
// between an ordinary player and `purge`, and it is worth one test of its
// own.
func TestAMortalCannotUseImmortalCommands(t *testing.T) {
	m := start(t, miniClassic)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")

	for _, command := range []string{"goto 3011", "purge", "load mob 130", "set Tourist gold 999", "shutdown"} {
		got := c.do(command)
		if !containsAny(got, "Huh?!?", "You do not have that ability") {
			t.Errorf("a mortal typing %q was not refused; the reply was:\n%s", command, got)
		}
	}
	contains(t, "still poor", c.do("score"), "0 gold coins")

	m.noServerErrors()
}
