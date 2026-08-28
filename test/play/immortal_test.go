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

// TestSyslogEchoesMudlogLines: `syslog` turns on the second half of
// mudlog() (utils.c:243), the one that shows a god what the game is doing
// as it happens. Only a booted server with real rooms in it can produce
// the interesting lines -- `zreset` needs a zone that resets, and a real
// login needs the login sequence -- so this is here rather than in
// internal/server.
//
// The green is the C's CCGRN at C_NRM (utils.c:255-257); the brackets are
// mudlog's own `sprintf(buf, "[ %s ]\r\n", str)`.
//
// Every assertion here is a doUntil rather than a do: the echo is queued
// onto the world goroutine (Server.echoWizVis), so it may land after the
// prompt of the command that caused it. Waiting for the line itself is
// the barrier; waiting for a prompt is not.
func TestSyslogEchoesMudlogLines(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	contains(t, "turning it on", c.do("syslog complete"),
		"Your syslog is now complete.")

	// (GC) %s reset zone %d (%s) -- NRM at LVL_GRGOD (act.wizard.c:2008),
	// which an implementor clears. examples/mini has one zone, #1.
	contains(t, "a zone reset is echoed",
		c.doUntil("zreset 1", "[ (GC) Founder reset zone 1 (The Testing Grounds) ]"),
		"[ (GC) Founder reset zone 1 (The Testing Grounds) ]")

	// Three lines from somebody else's login, none of them produced by
	// anything typed on this connection: "%s [%s] new player."
	// (interpreter.c:1629), do_start's "advanced to level 1"
	// (class.c:1837) and Crash_load's "entering game with no equipment."
	// (objsave.c:458).
	other := m.dial()
	other.create("Onlooker", "onlookerpass", "f", "w")
	contains(t, "somebody being created is echoed",
		c.doUntil("look", "[ Onlooker entering game with no equipment. ]"),
		"[ Onlooker [", "] new player. ]",
		"[ Onlooker advanced to level 1 ]",
		"[ Onlooker entering game with no equipment. ]")

	// %s has quit the game. -- NRM at LVL_IMMORT (act.other.c:154).
	other.doUntil("quit", "Make your choice:")
	contains(t, "somebody quitting is echoed",
		c.doUntil("look", "[ Onlooker has quit the game. ]"),
		"[ Onlooker has quit the game. ]")

	// %s [%s] has connected. -- BRF at LVL_IMMORT (interpreter.c:1509).
	// The character exists now, so this is a login rather than a
	// creation, and it is the other of the two lines.
	back := m.dial()
	back.login("Onlooker", "onlookerpass")
	contains(t, "somebody logging back in is echoed",
		c.doUntil("look", "] has connected. ]"),
		"[ Onlooker [", "] has connected. ]")

	m.noServerErrors()
}

// TestSyslogOffShowsNothing is the same server with the preference left
// where a new character finds it: `syslog` defaults to off, and mudlog's
// `if (tp < type) continue` (utils.c:252-253) then discards everything.
func TestSyslogOffShowsNothing(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	contains(t, "it starts off", c.do("syslog"), "Your syslog is currently off.")

	// The second command is the barrier: it is queued onto the world
	// goroutine behind whatever the reset queued, so its prompt is proof
	// that an echo would already have been written if there was one.
	// Both slices are searched, since a queued echo could land in either.
	out := c.do("zreset 1") + c.do("look")
	missing(t, "nothing is echoed", out, "[ (GC) Founder reset zone")

	m.noServerErrors()
}

// TestTeditMarksTheEditorAsWriting: PLR_WRITING (#214), on a real booted
// server. The C sets it in string_write (modify.c:100-101) and clears it
// in string_add's cleanup (:218-219), and every check on it was dead here
// until the setter existed.
//
// `tedit` is the editor worth using for this in the play suite rather
// than in internal/server: it edits a canned text file that was actually
// loaded off disk at boot, which is the one thing this suite has and the
// unit tests fake.
func TestTeditMarksTheEditorAsWriting(t *testing.T) {
	m := start(t, miniClassic)
	god := m.god()

	other := m.dial()
	other.create("Onlooker", "onlookerpass", "f", "w")

	// doUntil, not do: the editor prompts with "] " rather than returning
	// to the game prompt, so waiting for a prompt marker never finishes.
	god.doUntil("tedit motd", "Edit file below:")

	// act.comm.c:184 -- `tell` refuses rather than interrupting. The
	// mortal's own command runs on the world goroutine behind whatever
	// tedit queued there, so its reply is the barrier for the flag.
	contains(t, "tell refuses somebody mid-edit", other.do("tell Founder hello"),
		"writing a message right now; try again later.")

	god.doUntil("/a", "Edit aborted.")

	// And once the editor closes, the tell lands.
	contains(t, "tell works again afterwards", other.do("tell Founder hello again"),
		"You tell Founder, 'hello again'")

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

// TestHouses: hcontrol builds one, both listings show it, and hcontrol
// destroys it again.
//
// `show houses` is do_show's case 9 calling straight into
// hcontrol_list_houses (act.wizard.c:2321), so the two commands print the
// same table. Here rather than in internal/server because the housing store
// is wired up by the boot sequence -- `--lib-dir`'s house directory, opened
// in the state format the flags asked for -- and a server whose houses were
// never opened answers "Houses are not enabled on this server." to both.
func TestHouses(t *testing.T) {
	m := start(t, miniClassic)
	c := m.god()

	contains(t, "nothing built yet", c.do("show houses"), "No houses have been defined.")

	// Graduation Hall is a dead end with one two-way door, which is what a
	// house has to be: the trespassing check guards the room you are
	// *leaving*, so a second way in would not be guarded at all.
	contains(t, "build a house", c.do("hcontrol build 3016 south Founder"),
		"House built.")

	contains(t, "show houses", c.do("show houses"),
		"Address  Atrium  Build Date", "   3016    3015", "Founder")
	contains(t, "hcontrol show", c.do("hcontrol show"), "   3016    3015")

	contains(t, "destroy it", c.do("hcontrol destroy 3016"), "House deleted.")
	contains(t, "and it is gone", c.do("show houses"), "No houses have been defined.")

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
