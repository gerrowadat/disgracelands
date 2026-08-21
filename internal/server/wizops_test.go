// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Running the place: the descriptor surgery and the server controls.

func TestSnooping(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Watcher", "overyourshoulder", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Watched", "beingobserved", "m", "w")
	setLevel(t, srv, "Watched", 10)

	god.send("snoop")
	god.expect("You aren't snooping anyone.")

	god.send("snoop nobodyatall")
	god.expect("No such person around.")

	god.send("snoop watched")
	god.expect("Okay.")

	// Anything the victim is told reaches the snooper too.
	victim.send("look")
	victim.expect("The Temple Of Midgaard")
	god.expect("The Temple Of Midgaard")

	god.send("snoop")
	god.expect("You stop snooping.")

	// And now it does not.
	victim.send("gold")
	victim.expect("You're broke!")
	god.settle()
	if god.seen("You're broke!") {
		t.Error("the snooper still saw output after stopping")
	}
}

// You cannot snoop somebody at or above your own level.
func TestYouCannotSnoopYourBetters(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Lesser", "notthebiggest", "m", "w")

	second := dialClient(t, addr)
	second.create("Greater", "aboveyourank", "m", "w")
	setLevel(t, srv, "Greater", game.LevelImplementor)
	setLevel(t, srv, "Lesser", game.LevelGod)

	first.send("snoop greater")
	first.expect("You can't.")
}

func TestSwitchAndReturn(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Bodysnatcher", "intoyourskin", "m", "w")

	dog := aMobile(t, srv, "Bodysnatcher")
	if dog == nil {
		t.Fatal("no mobile to switch into")
	}

	c.send("switch")
	c.expect("Switch with who?")

	c.send("switch bodysnatcher")
	c.expect("Hee hee... we are jolly funny today, eh?")

	c.send("switch nobodyatall")
	c.expect("No such character.")

	c.send("switch dog")
	c.expect("Okay.")

	// Now the session is the dog: `look` is the dog looking, and what the
	// room says about them is the dog's name.
	c.send("emote wags.")
	c.settle()
	if !c.seen("dog wags.") {
		t.Errorf("the switched session did not act as the dog:\n%s", c.transcript())
	}

	// And here is something worth knowing: `switch` is a LVL_GOD command,
	// the level used for matching is the *body's*, and the dog is level one
	// — so a switched god cannot even see the command. "Huh?!?", not
	// "You're already switched.". The C behaves the same way for the same
	// reason (interpreter.c:623); its "You can't use immortal commands while
	// switched" message only fires when the body is itself high enough level
	// to match the command.
	c.send("switch dog")
	c.expect("Huh?!?")

	c.send("return")
	c.expect("You return to your original body.")

	c.send("emote waves.")
	c.settle()
	if !c.seen("Bodysnatcher waves.") {
		t.Error("returning did not put them back in their own body")
	}
}

// A body with somebody in it cannot be switched into.
func TestYouCannotSwitchIntoAnOccupiedBody(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Intruder", "letmeinthere", "m", "w")

	other := dialClient(t, addr)
	other.create("Occupied", "alreadyinuse", "m", "w")
	setLevel(t, srv, "Occupied", 10)

	god.send("switch occupied")
	god.expect("You can't do that, the body is already in use!")
}

// `return` for somebody who is not switched does nothing at all — the whole
// of do_return is inside the `if`.
func TestReturnDoesNothingWhenNotSwitched(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Athome", "neverleft", "m", "w")

	c.send("return")
	c.send("gold")
	c.expect("You're broke!")
	if c.seen("You return to your original body.") {
		t.Error("return said something for somebody who was not switched")
	}
}

func TestWizlock(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Gatekeeper", "whomayenter", "m", "w")

	c.send("wizlock")
	c.expect("The game is currently completely open.")

	c.send("wizlock 1")
	c.expect("The game is now closed to new players.")

	c.send("wizlock 20")
	c.expect("Only level 20 and above may enter the game now.")

	c.send("wizlock 99")
	c.expect("Invalid wizlock value.")

	if !srv.AllowedIn(20) {
		t.Error("a level 20 character is refused at wizlock 20")
	}
	if srv.AllowedIn(19) {
		t.Error("a level 19 character is allowed in at wizlock 20")
	}

	c.send("wizlock 0")
	c.expect("The game is now completely open.")
}

func TestDisconnect(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Bouncer", "outyougo", "m", "w")

	victim := dialClient(t, addr)
	victim.create("Bounced", "onmywayout", "m", "w")
	setLevel(t, srv, "Bounced", 10)

	god.send("dc")
	god.expect("Usage: DC <user number>")

	god.send("dc 99")
	god.expect("No such connection.")

	// The god is connection 1 and cannot disconnect themselves.
	god.send("dc 1")
	god.expect("Umm.. maybe that's not such a good idea...")

	god.send("dc 2")
	god.expect("Connection #2 closed.")

	// `dc` closes the connection; it does not remove the character, who is
	// left standing as linkdead exactly as any dropped link leaves them.
	// What must be true is that nothing is driving them any more.
	if !eventually(5*time.Second, func() bool {
		var linkdead bool
		inWorld(t, srv, func(w *game.Live) {
			who := w.Find("Bounced")
			linkdead = who != nil && who.Client == nil
		})
		return linkdead
	}) {
		t.Error("the disconnected character still has a connection")
	}
}

func TestDateAndUptime(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Timekeeper", "whatstheclock", "m", "w")

	c.send("date")
	c.expect("Current machine time:")

	c.send("uptime")
	c.expect("Up since ")
	c.expect(" day")
}

func TestLast(t *testing.T) {
	srv, store := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Archivist", "wholastcame", "m", "w")

	// Somebody who has been and gone, so `last` is reading the roster rather
	// than the world.
	other := dialClient(t, addr)
	other.create("Departed", "camethenwent", "m", "w")
	setLevel(t, srv, "Departed", 10)
	other.send("quit")
	other.expect("Goodbye")
	other.close()
	waitForLogout(t, srv, "Departed")
	srv.WaitForWrites()

	if _, err := store.Load(t.Context(), "Departed"); err != nil {
		t.Fatalf("the character was not saved: %v", err)
	}

	god.send("last")
	god.expect("For whom do you wish to search?")

	god.send("last nobodyatall")
	god.expect("There is no such player.")

	god.send("last departed")
	god.expect("Departed")
}

// The shutdown switch is a channel main() waits on, so the test can watch it
// without stopping anything.
func TestShutdown(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Closer", "timetostop", "m", "w")

	c.send("shutdown nonsense")
	c.expect("Unknown shutdown option.")

	select {
	case <-srv.ShutdownRequested():
		t.Fatal("a bad option asked the server to stop")
	default:
	}

	c.send("shutdown reboot")
	c.expect("Rebooting.. come back in a minute or two.")

	select {
	case <-srv.ShutdownRequested():
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not ask the server to stop")
	}
	if !srv.RebootWanted() {
		t.Error("`shutdown reboot` did not ask to come back")
	}
}

func TestMortalsCannotRunTheServer(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Owner", "runstheplace", "m", "w")

	c := dialClient(t, addr)
	c.create("Guest", "justvisiting", "m", "w")
	setLevel(t, srv, "Guest", 10)

	for _, command := range []string{
		"snoop owner", "switch owner", "dc 1", "wizlock 1", "shutdown", "last owner",
	} {
		c.send(command)
		c.expect("Huh?!?")
	}
}
