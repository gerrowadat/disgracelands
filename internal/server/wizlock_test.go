// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"log/slog"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Issue #211: nanny's two `circle_restrict` refusals had nothing calling
// them. `Server.AllowedIn` existed and was reached only by its own test, so
// `wizlock 32` closed the game to new *names* and let every mortal already on
// the roster walk straight in — which is exactly what the settings above 1
// exist to stop.

// wizlockSetup makes an implementor with a wizlock command to type, and an
// ordinary mortal who has been created and then disconnected, so there is a
// real record on the roster to try to log back in with.
func wizlockSetup(t *testing.T, srv *Server, addr string) (god *client, mortalName, mortalPassword string) {
	t.Helper()
	god = dialClient(t, addr)
	god.create("Gatekeeper", "whomayenter", "m", "w")

	mortal := dialClient(t, addr)
	mortal.create("Wanderer", "letmeback", "m", "w")
	// Out of the world before the lock goes on, so logging back in is a real
	// login and not a dupe-check reconnection.
	mortal.send("quit")
	mortal.expectCount("Make your choice:", 2)
	mortal.close()
	god.settle()

	return god, "Wanderer", "letmeback"
}

// The whole of #211: `GET_LEVEL(d->character) < circle_restrict`
// (interpreter.c:1491). A level-1 mortal against a wizlock of 20 is turned
// away after their password is accepted, not before — the C checks it at
// CON_PASSWORD, so a wrong password is still a wrong password first.
func TestWizlockKeepsAnExistingMortalOut(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, name, password := wizlockSetup(t, srv, addr)

	god.send("wizlock 20")
	god.expect("Only level 20 and above may enter the game now.")

	back := dialClient(t, addr)
	back.expect("By what name")
	back.send(name)
	back.expect("Password:")
	back.send(password)
	back.expect("The game is temporarily restricted.. try again later.")

	// STATE(d) = CON_CLOSE (interpreter.c:1493): refused and disconnected,
	// not left sitting at a prompt.
	back.expectEOF()

	var inWorldNow bool
	inWorld(t, srv, func(w *game.Live) { inWorldNow = w.Find(name) != nil })
	if inWorldNow {
		t.Error("a wizlocked-out mortal got into the world")
	}
}

// The other side of the same test: the threshold is a threshold, so somebody
// at or above it walks in. Level 20 exactly, because `<` is the C's operator
// and off-by-one here would be invisible at any other level.
func TestWizlockLetsInAnybodyAtTheLevel(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god := dialClient(t, addr)
	god.create("Gatekeeper", "whomayenter", "m", "w")

	senior := dialClient(t, addr)
	senior.create("Senior", "highenough", "m", "w")
	setLevel(t, srv, "Senior", 20)
	senior.send("quit")
	senior.expectCount("Make your choice:", 2)
	senior.close()
	god.settle()

	god.send("wizlock 20")
	god.expect("Only level 20 and above may enter the game now.")

	back := dialClient(t, addr)
	back.login("Senior", "highenough")
	back.send("look")
	back.expect("The Temple Of Midgaard")
}

// `if (circle_restrict)` at CON_NAME_CNFRM (interpreter.c:1421): *any*
// wizlock stops a character being made, which is what `wizlock 1` is for.
//
// And note the message. Before #211 the only thing stopping a new character
// was Create returning an error, which the session turned into "Something
// went wrong creating your character." — a message about the server, for
// something that is a policy.
func TestWizlockRefusesANewCharacter(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god := dialClient(t, addr)
	god.create("Gatekeeper", "whomayenter", "m", "w")

	god.send("wizlock 1")
	god.expect("The game is now closed to new players.")

	newcomer := dialClient(t, addr)
	newcomer.expect("By what name")
	newcomer.send("Hopeful")
	// The refusal is after the confirmation, which is where the C puts it: the
	// name it logs is one the player has already stood over.
	newcomer.expect("Did I get that right")
	newcomer.send("y")
	newcomer.expect("Sorry, new players can't be created at the moment.")
	newcomer.expectEOF()

	if newcomer.seen("Something went wrong creating your character") {
		t.Errorf("the wizlock refusal came out as a creation error:\n%s", newcomer.transcript())
	}
}

// Both refusals are mudlog(buf, NRM, LVL_GOD, TRUE) (interpreter.c:1423,
// :1494) — LVL_GOD, so a level-31 immortal never sees either of them however
// high their syslog is turned up. The implementor here is 34.
func TestWizlockRefusalsAreLogged(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, name, password := wizlockSetup(t, srv, addr)

	god.send("syslog complete")
	god.expect("Your syslog is now complete.")
	god.send("wizlock 20")
	god.expect("Only level 20 and above may enter the game now.")

	newcomer := dialClient(t, addr)
	newcomer.expect("By what name")
	newcomer.send("Hopeful")
	newcomer.expect("Did I get that right")
	newcomer.send("y")
	newcomer.expectEOF()

	back := dialClient(t, addr)
	back.expect("By what name")
	back.send(name)
	back.expect("Password:")
	back.send(password)
	back.expectEOF()

	god.settle()
	// "Request for new char %s denied from [%s] (wizlock)" and "Request for
	// login denied for %s [%s] (wizlock)" — different shapes, which is the
	// C's own inconsistency: `from [%s]` for one and `for %s [%s]` for the
	// other.
	for _, want := range []string{
		"Request for new char Hopeful denied from [",
		"Request for login denied for Wanderer [",
	} {
		if !god.seen(want) {
			t.Errorf("no syslog line matching %q:\n%s", want, god.transcript())
		}
	}
}

// `-r` on the command line is `circle_restrict = 1` in the C and nothing
// else (comm.c:328-330), so it is the same field `wizlock` sets — which
// means `wizlock 0` reopens a server that was started restricted. It could
// not, while `-r` was a separate bool here.
func TestRestrictIsJustAWizlockOfOne(t *testing.T) {
	srv := New(Options{Restrict: true, Logger: slog.New(slog.DiscardHandler)})

	if got := srv.Restrict(); got != 1 {
		t.Errorf("a server started with -r reports wizlock %d, want 1", got)
	}
	if srv.NewCharactersAllowed() {
		t.Error("a server started with -r is accepting new characters")
	}
	// An existing mortal is still let in: `1 < 1` is false, and do_start puts
	// everybody who has ever played at level 1 or above (class.c:1836).
	if !srv.AllowedIn(1) {
		t.Error("-r turned away a level 1 character")
	}
	// But a name created and never played is level 0 until Enter runs
	// do_start, so it is as new as a new one.
	if srv.AllowedIn(0) {
		t.Error("-r let in a character who has never entered the world")
	}

	srv.SetRestrict(0)
	if !srv.NewCharactersAllowed() {
		t.Error("wizlock 0 did not reopen a server started with -r")
	}
}
