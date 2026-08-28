// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

//go:build play

package play

import (
	"strings"
	"testing"
	"time"
)

// The login sequence and everything reached from the main menu: the part of
// the game every player goes through before any of the tour happens, and the
// part with a whole state machine behind it.

// TestTheGreetingNamesTheCreators is a licence test before it is a feature
// test. The CircleMUD licence requires the login sequence -- everything
// between connecting and playing -- to name both sets of creators
// (docs/proposals/go-port-plan.md §12). scripts/license-check.sh checks the
// file says so; this checks a connecting player is actually shown it.
func TestTheGreetingNamesTheCreators(t *testing.T) {
	m := start(t, mini)
	c := m.dial()

	greeting := c.expect("By what name do you wish to be known?")
	contains(t, "the greeting", greeting, "Jeremy Elson",
		"Staerfeldt", "Nyboe", "Madsen", "Seifert", "Hammer")

	m.noServerErrors()
}

// TestCreatingACharacter walks the creation sequence and checks each refusal
// on the way, because every one of them is a state in the login machine that
// a player can reach by typing something ordinary.
func TestCreatingACharacter(t *testing.T) {
	m := start(t, mini)
	c := m.dial()

	c.expect("By what name do you wish to be known?")
	c.send("Tourist")
	c.expect("Did I get that right, Tourist (Y/N)?")

	// Saying no goes back to the name prompt rather than on to a password.
	c.send("n")
	c.expect("By what name do you wish to be known?")

	c.send("Tourist")
	c.expect("Did I get that right")
	c.send("y")
	c.expect("Give me a password for Tourist:")
	c.send("tourpass")
	c.expect("retype password")

	// A mistyped confirmation is refused and asked for again.
	c.send("notthesame")
	c.expect("Passwords don't match.")
	c.expect("Give me a password for Tourist:")

	c.send("tourpass")
	c.expect("retype password")
	c.send("tourpass")
	c.expect("What is your sex (M/F)?")
	c.send("m")
	c.expect("Class:")
	c.send("w")
	c.expect("PRESS RETURN")
	c.send("")
	c.enterGame()

	contains(t, "a new character", c.do("score"),
		"This ranks you as Tourist", "(level 1)")

	m.noServerErrors()
}

// TestTheFirstCharacterIsAnImplementor. db.c's "if this is our first player
// --- he be God": the first character created on an empty roster comes out at
// level 34, and this is the one test that wants the roster genuinely empty.
func TestTheFirstCharacterIsAnImplementor(t *testing.T) {
	m := start(t, mini, startOptions{noFounder: true})
	c := m.dial()
	c.create("First", "firstpass", "m", "w")

	contains(t, "the first character", c.do("score"), "(level 34)")

	// And the second is not.
	c.quit()
	c.close()
	if !eventually(10*time.Second, func() bool { return m.rosterHas("First") }) {
		t.Fatal("the first character never reached the roster index")
	}

	second := m.dial()
	second.create("Second", "secondpass", "f", "c")
	contains(t, "the second character", second.do("score"), "(level 1)")

	m.noServerErrors()
}

// TestTheMenu. Every option on it, in the order it lists them.
func TestTheMenu(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")

	// Back out to the menu the only way a player can: quit, then reconnect.
	c.quit()
	c.close()

	c = m.dial()
	c.expect("By what name do you wish to be known?")
	c.send("Tourist")
	c.expect("Password:")
	c.send("tourpass")
	c.expect("PRESS RETURN")
	c.send("")

	menu := c.expect("Make your choice:")
	contains(t, "the menu", menu,
		"0) Exit from CircleMUD.", "1) Enter the game.", "2) Enter description.",
		"3) Read the background story.", "4) Change password.", "5) Delete this character.")

	// 3, the background story. The menu does not come straight back: the C
	// leaves the connection in CON_RMOTD when background's paging finishes
	// (interpreter.c:1712-1714), so the *next line typed* is what returns
	// to the menu. A port that "fixed" that would fail here.
	story := c.doUntil("3", "The Testing Grounds")
	contains(t, "the background story", story, "This is not a place with a history.")
	c.doUntil("", "Make your choice:")

	// 4, changing the password, wants the old one first.
	c.doUntil("4", "Enter your old password")
	c.send("wrongpass")
	c.expect("Incorrect password.")
	c.expect("Make your choice:")

	c.doUntil("4", "Enter your old password")
	c.send("tourpass")
	c.expect("Enter a new password:")
	c.send("newerpass")
	c.expect("retype password")
	c.send("newerpass")
	c.expect("Done.")
	c.expect("Make your choice:")

	// 2, the description editor.
	c.doUntil("2", "Enter the new text you'd like others to see")
	c.send("A weary tourist stands here, map in hand.")
	c.doUntil("@", "Make your choice:")

	c.doUntil("1", promptMarker)
	contains(t, "the new description", c.do("look Tourist"),
		"A weary tourist stands here, map in hand.")

	m.noServerErrors()
}

// TestTheNewPasswordIsTheOneThatWorks is the other half of the menu's option
// 4: a password change nobody logs in with again is not a password change.
func TestTheNewPasswordIsTheOneThatWorks(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")
	c.quit()
	c.close()

	c = m.dial()
	c.expect("By what name")
	c.send("Tourist")
	c.expect("Password:")
	c.send("tourpass")
	c.expect("PRESS RETURN")
	c.send("")
	c.expect("Make your choice:")
	c.doUntil("4", "Enter your old password")
	c.send("tourpass")
	c.expect("Enter a new password:")
	c.send("newerpass")
	c.expect("retype password")
	c.send("newerpass")
	c.expect("Done.")
	c.expect("Make your choice:")
	c.doUntil("0", "Goodbye")
	c.close()

	// The old one is refused...
	stale := m.dial()
	stale.expect("By what name")
	stale.send("Tourist")
	stale.expect("Password:")
	stale.send("tourpass")
	stale.expect("Wrong password.")
	stale.close()

	// ...and the new one is not.
	fresh := m.dial()
	fresh.login("Tourist", "newerpass")
	contains(t, "logging in with the new password", fresh.do("score"), "Tourist")

	m.noServerErrors()
}

// TestWhatYouWereCarryingSurvivesAQuit.
//
// This is the rent file, and it is the single most important thing a player
// expects the server to get right. free_rent is on, so `quit` alone is
// supposed to be enough: the belongings are written to a rent file on the way
// out and handed back on the way in.
func TestWhatYouWereCarryingSurvivesAQuit(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")

	c.toRoom(roomArmory)
	c.do("get all")
	c.do("wear tunic")
	c.do("wield sword")
	contains(t, "before quitting", c.do("inventory"), "a small brass lantern")

	c.quit()
	c.close()

	back := m.dial()
	back.login("Tourist", "tourpass")

	// Everything comes back carried, including what was being worn
	// and wielded. That is Crash_crashsave's own doing and not an
	// omission: the C's rent files record no wear position at all,
	// so Crash_load hands the lot back as inventory and the player
	// dresses again. A port that restored the equipment worn would
	// be a nicer game and a wrong one.
	contains(t, "what came back", back.do("inventory"),
		"a rusty key", "a small brass lantern", "a leather tunic", "a training sword")
	contains(t, "nothing came back worn", back.do("equipment"), "Nothing.")

	m.noServerErrors()
}

// TestSaveWritesTheAliasesAndSaysSo is do_save's duplication guard
// (act.other.c:173-186) on a server booted the way one really boots.
// auto_save defaults on (config.c:150) and nothing in examples/mini turns it
// off, so what a player actually gets for typing `save` is "Saving aliases."
// and no character write at all — two clients with coordinated saves, or one
// client and a crash, are how items get duplicated, and the periodic sweep
// has the job instead. internal/server can prove the guard; only this can
// prove the shipped configuration still lands inside it.
//
// The alias surviving the round trip is the other half of the C's reasoning:
// write_aliases has exactly two call sites and both are in do_save, so a
// guarded save that wrote nothing at all would lose an alias defined this
// session.
func TestSaveWritesTheAliasesAndSaysSo(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")

	c.do("alias gl get all corpse")
	contains(t, "a mortal's save", c.do("save"), "Saving aliases.")

	c.quit()
	c.close()

	back := m.dial()
	back.login("Tourist", "tourpass")
	contains(t, "the alias after a round trip", back.do("alias"), "gl", "get all corpse")

	m.noServerErrors()
}

// TestReconnectingToALinkdeadCharacter. A connection that drops without
// quitting leaves the character standing in the world; logging in again picks
// the same body back up rather than making a second one.
func TestReconnectingToALinkdeadCharacter(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")
	c.toRoom(roomArmory)
	c.do("get sword")

	// Somebody to watch it happen, standing in the same room.
	watcher := m.dial()
	watcher.create("Watcher", "watchpass", "f", "c")
	watcher.toRoom(roomArmory)

	// Pull the plug rather than quitting.
	c.close()

	// Wait for the link loss to have actually been processed before logging
	// back in. The room being told is the barrier: Leave announces it in the
	// same step that releases the body (Server.Leave), so once the watcher
	// has seen this the character really is linkdead rather than merely
	// unreachable. Reconnecting before that point is a different case
	// entirely -- the connection is still in the world as far as the server
	// knows, and perform_dupe_check would treat it as a usurp.
	watcher.expect("Tourist has lost their link.")

	back := m.dial()
	back.expect("By what name")
	back.send("Tourist")
	back.expect("Password:")
	back.send("tourpass")
	back.expect("Reconnecting.")
	back.expect(promptMarker)

	// The same body: still in the Armory, still holding the sword.
	contains(t, "where the reconnected character is", back.do("look"), "The Armory")
	contains(t, "what it is still carrying", back.do("inventory"), "a training sword")

	// And the room is told, which the C does (interpreter.c:1284) and this
	// port did not until perform_dupe_check was ported properly: a
	// reconnection used to be invisible to everybody but the person doing
	// it.
	contains(t, "what the room saw", watcher.do("look"), "Tourist has reconnected.")

	m.noServerErrors()
}

func TestLoggingInTwiceTakesOverTheBody(t *testing.T) {
	m := start(t, mini)
	first := m.dial()
	first.create("Tourist", "tourpass", "m", "w")

	// A bystander, so that what the *room* is told can be checked too.
	bystander := m.dial()
	bystander.create("Watcher", "watchpass", "f", "c")
	first.expect("Watcher has entered the game.")

	second := m.dial()
	second.expect("By what name")
	second.send("Tourist")
	second.expect("Password:")
	second.send("tourpass")

	// No menu and no motd: perform_dupe_check runs the moment the password
	// is accepted (interpreter.c:1500) and puts the new connection straight
	// into the body.
	second.expect("You take over your own body, already in use!")
	second.expect("The Testing Grounds")
	// Read through to the prompt before typing anything: do() slices the
	// transcript at the next prompt, and starting it mid-stream would hand
	// back the tail of this `look` instead of the next command's reply.
	second.expect(promptMarker)

	// The older connection is told what happened, in the C's own two
	// messages, and then closed.
	first.expect("This body has been usurped!")
	first.expect("Multiple login detected -- disconnecting.")

	contains(t, "what the room saw", bystander.do("look"),
		"Tourist suddenly keels over in pain, surrounded by a white aura...",
		"Tourist's body has been taken over by a new spirit!")

	// One character, one body: the whole point of the check.
	contains(t, "who", second.do("who"), "Tourist", "Watcher", "2 characters displayed.")

	// And the taken-over body is the live one, still where it was left and
	// still holding what it was holding -- not a second copy loaded from
	// disk.
	second.do("north")
	contains(t, "the body moves", second.do("look"), "Hall of Movement")

	m.noServerErrors()
}

// TestADuplicateLoginAtTheMenuIsDisconnected: the other half of
// perform_dupe_check's first pass. A connection that has authenticated but is
// sitting at the menu holds a loaded character and no body in the world, so
// there is nothing to take over -- but it is still a duplicate, and leaving
// it there is how a player ends up with two connections that both save.
func TestADuplicateLoginAtTheMenuIsDisconnected(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")
	c.quit()
	c.close()

	if !eventually(10*time.Second, func() bool { return m.rosterHas("Tourist") }) {
		t.Fatal("the character never reached the roster index")
	}

	// One connection parked at the menu.
	atMenu := m.dial()
	atMenu.expect("By what name")
	atMenu.send("Tourist")
	atMenu.expect("Password:")
	atMenu.send("tourpass")
	atMenu.expect("PRESS RETURN")
	atMenu.send("")
	atMenu.expect("Make your choice:")

	// A second login as the same character.
	second := m.dial()
	second.expect("By what name")
	second.send("Tourist")
	second.expect("Password:")
	second.send("tourpass")

	atMenu.expect("Multiple login detected -- disconnecting.")

	// Nothing was in the world to take over, so this is an ordinary login:
	// motd, menu, and in.
	second.expect("PRESS RETURN")
	second.send("")
	second.enterGame()
	contains(t, "the surviving connection", second.do("score"), "This ranks you as Tourist")

	m.noServerErrors()
}

// TestADisplacedConnectionDoesNotSaveOverTheBody.
//
// The sharp edge of the dupe check, and the reason it clears the old
// descriptor's character pointer rather than just closing the socket
// (interpreter.c:1211). A duplicate sitting at the menu is carrying nothing.
// If its teardown were allowed to run normally it would crash-save that
// nothing over the rent file, and the player would come back to an empty
// pack.
func TestADisplacedConnectionDoesNotSaveOverTheBody(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")
	c.toRoom(roomArmory)
	c.do("get all")
	contains(t, "before any of this", c.do("inventory"), "a training sword")
	c.quit()
	c.close()

	if !eventually(10*time.Second, func() bool { return m.rosterHas("Tourist") }) {
		t.Fatal("the character never reached the roster index")
	}

	// A connection at the menu, holding a loaded record and nothing else.
	atMenu := m.dial()
	atMenu.expect("By what name")
	atMenu.send("Tourist")
	atMenu.expect("Password:")
	atMenu.send("tourpass")
	atMenu.expect("PRESS RETURN")
	atMenu.send("")
	atMenu.expect("Make your choice:")

	// Displaced by a real login, which then plays.
	real := m.dial()
	real.login("Tourist", "tourpass")
	atMenu.expect("Multiple login detected -- disconnecting.")
	atMenu.close()

	contains(t, "what the real connection has", real.do("inventory"),
		"a training sword", "a leather tunic", "a small brass lantern")

	// And it is still there after that connection has gone too, which is
	// the assertion that would fail if the displaced one had crash-saved.
	real.quit()
	real.close()

	back := m.dial()
	back.login("Tourist", "tourpass")
	contains(t, "what survived", back.do("inventory"),
		"a training sword", "a leather tunic", "a small brass lantern")

	m.noServerErrors()
}

// TestTheWrongPassword: max_bad_pws (config.c:236) end to end, and that the
// server says nothing useful about which half was wrong.
//
// Three things at once, because they are one sequence a player types: the
// first two wrong passwords ask again rather than hanging up
// (interpreter.c:1470-1472), the third disconnects (:1467-1469), and the next
// successful login is told how many there were (:1511-1518). The count that
// survives the disconnect is written to the *pfile*, which is why this is
// worth having here as well as in internal/server: this is the only suite
// that boots a real server on a real lib directory and so the only one where
// that write is the real one.
func TestTheWrongPassword(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")
	c.quit()
	c.close()

	back := m.dial()
	back.expect("By what name")
	back.send("Tourist")
	back.expect("Password:")

	// Two strikes, each answered with another prompt. The prompts are
	// counted rather than matched: there is already a "Password:" in the
	// transcript, from before any of this.
	for i := 1; i <= 2; i++ {
		back.send("nottherightone")
		back.expectCount("Wrong password.", i)
		back.expectCount("Password:", i+1)
	}

	// The third is the door, and it says something different on the way out.
	back.send("stillnottherightone")
	contains(t, "the third wrong password", back.expectEOF(),
		"Wrong password... disconnecting.")
	back.close()

	// A fresh connection starts from zero strikes -- the counter max_bad_pws
	// is measured against belongs to the socket (structs.h:1019), not to the
	// character -- and is told what happened while nobody was looking.
	fresh := m.dial()
	fresh.expect("By what name")
	fresh.send("Tourist")
	fresh.expect("Password:")
	fresh.send("tourpass")
	contains(t, "the login after three failures", fresh.expect("PRESS RETURN"),
		"3 LOGIN FAILURES SINCE LAST SUCCESSFUL LOGIN.")
	fresh.send("")
	fresh.enterGame()
	fresh.quit()
	fresh.close()

	// Reporting it clears it: the next login is a quiet one.
	quiet := m.dial()
	quiet.login("Tourist", "tourpass")
	if strings.Contains(quiet.transcript(), "LOGIN FAILURE") {
		t.Errorf("a clean login was told about failures anyway:\n%s", quiet.transcript())
	}

	m.noServerErrors()
}

// TestDeletingACharacter, menu option 5 -- the one menu entry that cannot be
// undone, and which therefore asks twice.
func TestDeletingACharacter(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Doomed", "doompass", "m", "w")
	c.quit()
	c.close()

	if !eventually(10*time.Second, func() bool { return m.rosterHas("Doomed") }) {
		t.Fatal("the character never reached the roster index")
	}

	c = m.dial()
	c.expect("By what name")
	c.send("Doomed")
	c.expect("Password:")
	c.send("doompass")
	c.expect("PRESS RETURN")
	c.send("")
	c.expect("Make your choice:")

	c.doUntil("5", "Enter your password for verification:")
	c.send("doompass")
	c.expect("YOU ARE ABOUT TO DELETE THIS CHARACTER PERMANENTLY.")
	c.expect("Please type \"yes\" to confirm:")

	// Anything but the exact word is a refusal -- the C compares against
	// "yes" and "YES" only, so even "Yes" does not delete a character, and
	// the confirmation being awkward to type is the point.
	c.send("Yes")
	c.expect("Character not deleted.")
	c.expect("Make your choice:")

	c.doUntil("5", "Enter your password for verification:")
	c.send("doompass")
	c.expect("Please type \"yes\" to confirm:")
	c.send("yes")
	c.expect("Character 'Doomed' deleted!")
	c.close()

	m.noServerErrors()
}

// TestQuittingFromTheMenu, option 0, which is the other way out and the one
// that never enters the world at all.
func TestQuittingFromTheMenu(t *testing.T) {
	m := start(t, mini)
	c := m.dial()
	c.create("Tourist", "tourpass", "m", "w")
	c.quit()
	c.close()

	c = m.dial()
	c.expect("By what name")
	c.send("Tourist")
	c.expect("Password:")
	c.send("tourpass")
	c.expect("PRESS RETURN")
	c.send("")
	c.expect("Make your choice:")
	contains(t, "leaving from the menu", c.doUntil("0", "Goodbye"), "Goodbye")

	m.noServerErrors()
}
