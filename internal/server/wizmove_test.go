// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The wizard commands for getting about.

// setLevel makes somebody a particular level.
func setLevel(t *testing.T, srv *Server, name string, level int32) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find(name)
		if who == nil || who.Record == nil {
			t.Error("the character is not in the world")
			return
		}
		who.Record.Level = level
	})
}

// A mortal is not told there is such a command as `goto`; they are told
// "Huh?!?", the same as for a word that means nothing.
func TestMortalsCannotSeeTheWizardCommands(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// The second character on the roster is an ordinary mortal.
	first := dialClient(t, addr)
	first.create("Deity", "iamagod", "m", "w")

	c := dialClient(t, addr)
	c.create("Mortal", "justaperson", "m", "w")
	setLevel(t, srv, "Mortal", 10)

	for _, command := range []string{"goto 3001", "transfer deity", "teleport deity 3001", "invis"} {
		c.send(command)
		c.expectAny("Huh?!?")
	}

	// And `at` is not `at` either — but `attack` might be, so this only
	// asserts that whatever it is, it is not doing anything godly.
	if got := roomOf(t, srv, "Mortal"); got != MortalStartRoom {
		t.Errorf("a mortal moved themselves to %d", got)
	}
}

func TestGotoAndBack(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Wanderer", "everywhere", "m", "w")

	c.sendExpectNew("goto 3001", "The Temple Of Midgaard")
	if got := roomOf(t, srv, "Wanderer"); got != MortalStartRoom {
		t.Errorf("goto put them in %d, want %d", got, MortalStartRoom)
	}

	c.send("goto 99999")
	c.expect("No room exists with that number.")

	c.send("goto nothinglikethis")
	c.expect("Nothing exists by that name.")

	c.send("goto")
	c.expect("You must supply a room number or a name.")
}

// `goto <somebody>` goes to whoever that is.
func TestGotoAPerson(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Seeker", "findthemall", "m", "w")

	quarry := dialClient(t, addr)
	quarry.create("Quarry", "overherenow", "m", "w")
	moveTo(t, srv, "Quarry", MageGuildRoom)

	god.send("goto quarry")
	god.expect("The Mage Guild")
	if got := roomOf(t, srv, "Seeker"); got != MageGuildRoom {
		t.Errorf("goto <person> put them in %d, want the guild at %d", got, MageGuildRoom)
	}
}

// The room sees you go and sees you arrive, and a poofset changes what it
// sees.
func TestPoofing(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Poofer", "smokeandbang", "m", "w")

	_, watcher := place(t, srv, fighterRecord("Watcher", 10, 100), ImmortStartRoom)

	god.sendExpectNew("goto 3001", "The Temple Of Midgaard")
	god.settle()
	if !watcher.said("Poofer disappears in a puff of smoke.") {
		t.Error("the room did not see the default poofout")
	}

	god.send("poofout vanishes in a shower of sparks.")
	god.expect("Okay.")
	god.send("poofin materialises out of nowhere.")
	god.expect("Okay.")

	// Back to the watcher, then away again so both messages are seen.
	god.sendExpectNew("goto 1204", "The Immortal Board Room")
	god.settle()
	if !watcher.said("Poofer materialises out of nowhere.") {
		t.Error("the room did not see the poofin")
	}

	god.send("goto 3001")
	god.settle()
	if !watcher.said("Poofer vanishes in a shower of sparks.") {
		t.Error("the room did not see the poofout")
	}
}

func TestAtRunsACommandSomewhereElse(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Remote", "actionatadistance", "m", "w")

	_, watcher := place(t, srv, fighterRecord("Watcher", 10, 100), MortalStartRoom)

	god.send("at")
	god.expect("You must supply a room number or a name.")

	god.send("at 3001")
	god.expect("What do you want to do there?")

	god.send("at 3001 smile")
	god.settle()
	if !watcher.said("smiles happily") {
		t.Errorf("the smile did not happen in the temple:\n%s", god.transcript())
	}

	// And they came back.
	if got := roomOf(t, srv, "Remote"); got != ImmortStartRoom {
		t.Errorf("after `at` they are in %d, want back at %d", got, ImmortStartRoom)
	}
}

// A command that moves you leaves you where it put you: `at` only comes back
// if you are still where it sent you.
func TestAtDoesNotDragYouBackFromSomewhereElse(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Rover", "keepsmoving", "m", "w")

	// From the temple, north is the board room.
	c.send("at 3001 north")
	c.settle()

	if got := roomOf(t, srv, "Rover"); got != ImmortStartRoom {
		t.Errorf("after `at 3001 north` they are in %d, want the board room at %d",
			got, ImmortStartRoom)
	}
}

func TestTransfer(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Summoner", "comehereyou", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Summoned", "goingnowhere", "m", "w")
	setLevel(t, srv, "Summoned", 10)

	god.send("transfer")
	god.expect("Whom do you wish to transfer?")

	god.send("transfer nobodyatall")
	god.expect("No-one by that name here.")

	god.send("transfer summoner")
	god.expect("That doesn't make much sense, does it?")

	god.send("transfer summoned")
	victim.expect("has transferred you!")

	if got := roomOf(t, srv, "Summoned"); got != ImmortStartRoom {
		t.Errorf("the transferred character is in %d, want the summoner's room %d",
			got, ImmortStartRoom)
	}
}

func TestTeleport(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Sender", "offyougo", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Sent", "wherenow", "m", "w")
	setLevel(t, srv, "Sent", 10)

	god.send("teleport")
	god.expect("Whom do you wish to teleport?")

	god.send("teleport sender 3001")
	god.expect("Use 'goto' to teleport yourself.")

	god.send("teleport sent")
	god.expect("Where do you wish to send this person?")

	god.send("teleport sent 3017")
	god.expect("Okay.")
	victim.expect("has teleported you!")

	if got := roomOf(t, srv, "Sent"); got != MageGuildRoom {
		t.Errorf("the teleported character is in %d, want the guild at %d", got, MageGuildRoom)
	}
}

// You cannot teleport somebody at or above your own level.
func TestYouCannotTeleportYourBetters(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Lesser", "notthebiggest", "m", "w")

	other := dialClient(t, addr)
	other.create("Greater", "biggerthanyou", "m", "w")
	setLevel(t, srv, "Greater", game.LevelImplementor)
	setLevel(t, srv, "Lesser", game.LevelGod)

	god.send("teleport greater 3001")
	god.expect("Maybe you shouldn't do that.")
}

func TestInvisibility(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Ghostly", "nowyoudont", "m", "w")

	c.send("invis")
	c.expect("Your invisibility level is 34.")

	c.send("invis")
	c.expect("You are now fully visible.")

	c.send("invis 20")
	c.expect("Your invisibility level is 20.")

	c.send("invis 0")
	c.expectCount("You are now fully visible.", 2)

	c.send("invis 99")
	c.expect("You can't go invisible above your own level.")
}

// The two messages go to the people whose view of you actually changes.
func TestGoingInvisibleTellsThePeopleItAffects(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Fader", "watchmego", "m", "w")

	_, mortal := place(t, srv, fighterRecord("Mortal", 10, 100), ImmortStartRoom)

	c.send("invis")
	c.settle()
	if !mortal.said("You blink and suddenly realize that Fader is gone.") {
		t.Error("the mortal was not told the god vanished")
	}

	// Coming back is appear()'s message, not perform_immort_invis's. The
	// two are easy to confuse and this test had them confused: `invis 0`
	// goes to perform_immort_vis (act.wizard.c:1562), which calls appear
	// (fight.c:91), which for anybody at or above LVL_IMMORT says this.
	// "You suddenly realize that $n is standing beside you." is what
	// perform_immort_invis says when a god *lowers* their invis level far
	// enough for somebody to start seeing them — a different event, and
	// still tested by the `invis 20`-shaped cases.
	c.send("invis 0")
	c.settle()
	if !mortal.said("You feel a strange presence as Fader appears, seemingly from nowhere.") {
		t.Error("the mortal was not told the god came back")
	}
}
