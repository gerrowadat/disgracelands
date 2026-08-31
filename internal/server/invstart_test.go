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

// PLR_INVSTART: arriving unseen (#378).
//
// The C reads the flag at menu choice '1', before the character is put in a
// room and therefore before anybody is told they arrived
// (interpreter.c:1646-1648). Here it was written by `set`, saved, loaded and
// read by nothing.
//
// Both halves are needed for it to mean anything, and both are tested: the
// invis level has to be raised, and the arrival line has to respect it. The
// C gets the second from act()'s hide_invisible argument; this port sends
// that line itself and was sending it to everybody.

// invisLevelOf reads a character's invis level.
func invisLevelOf(t *testing.T, srv *Server, name string) int32 {
	t.Helper()
	var level int32
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find(name); who != nil && who.Record != nil {
			level = who.Record.InvisLevel
		}
	})
	return level
}

// arriveWithInvstart logs a god in, sets their level and the flag, sends them
// away and brings them back — the flag only does anything on the way *in*, so
// a test of it has to be a second login.
//
// The watcher is a level-31 immortal rather than a mortal because the god
// starts in the immortal room, and it is a *lower* level on purpose: CAN_SEE
// asks `GET_INVIS_LEV(obj) > GET_LEVEL(sub)`, so an equal-level god sees
// through it and would make this test pass for the wrong reason.
func arriveWithInvstart(t *testing.T, srv *Server, addr string, flag bool) (watcher *client, godLevel int32) {
	t.Helper()

	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelImplementor)

	watcher = dialClient(t, addr)
	watcher.create("Watcher", "seeall", "m", "w")
	setLevel(t, srv, "Watcher", game.LevelImmortal)
	// Into the room the god will arrive in.
	watcher.send("goto 1204")
	watcher.expect("> ")

	if flag {
		inWorld(t, srv, func(w *game.Live) {
			if who := w.Find("Zod"); who != nil && who.Record != nil {
				who.Record.PlayerFlags = who.Record.PlayerFlags.With(game.PlayerInvisStart)
			}
		})
	}

	god.send("quit")
	god.expectCount("Make your choice:", 2)
	god.close()
	// The flag and the level are on the live record and only reach disk on
	// the quit, which happens off the world goroutine. #373.
	waitForLogout(t, srv, "Zod")

	watcher.settle()

	back := dialClient(t, addr)
	back.login("Zod", "swordfish")
	watcher.settle()

	return watcher, game.LevelImplementor
}

// TestInvstartArrivesUnseen is #378.
func TestInvstartArrivesUnseen(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	watcher, level := arriveWithInvstart(t, srv, addr, true)

	if got := invisLevelOf(t, srv, "Zod"); got != level {
		t.Errorf("an invstart god came in at invis level %d, want their own level %d", got, level)
	}
	if watcher.seen("Zod has entered the game.") {
		t.Errorf("an invstart god's arrival was announced to somebody who cannot see them:\n%s",
			watcher.transcript())
	}
}

// TestWithoutInvstartTheArrivalIsSeen, at the same levels and in the same
// room, so the flag is the only difference between the two.
func TestWithoutInvstartTheArrivalIsSeen(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	watcher, _ := arriveWithInvstart(t, srv, addr, false)

	if got := invisLevelOf(t, srv, "Zod"); got != 0 {
		t.Errorf("a god with no invstart flag came in at invis level %d, want 0", got)
	}
	if !watcher.seen("Zod has entered the game.") {
		t.Errorf("an ordinary arrival was not announced:\n%s", watcher.transcript())
	}
}

// TestAnInvisibleArrivalIsHiddenWithoutTheFlagToo.
//
// The invis level survives a logout in the pfile, so a god who typed `invis`
// and quit comes back invisible with no flag involved — and their arrival was
// announced to everybody all the same, because this port sent the line
// without act()'s hide_invisible filter. That was a bug in its own right and
// this is the half of the fix that is not about #378's flag at all.
func TestAnInvisibleArrivalIsHiddenWithoutTheFlagToo(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelImplementor)

	watcher := dialClient(t, addr)
	watcher.create("Watcher", "seeall", "m", "w")
	setLevel(t, srv, "Watcher", game.LevelImmortal)
	watcher.send("goto 1204")
	watcher.expect("> ")

	// Typed, not flagged: `invis` with no argument goes to your own level.
	god.send("invis")
	god.expect("> ")
	god.send("quit")
	god.expectCount("Make your choice:", 2)
	god.close()
	waitForLogout(t, srv, "Zod")

	watcher.settle()
	back := dialClient(t, addr)
	back.login("Zod", "swordfish")
	watcher.settle()

	if got := invisLevelOf(t, srv, "Zod"); got == 0 {
		t.Fatal("the invis level did not survive the logout, so this tests nothing")
	}
	if watcher.seen("Zod has entered the game.") {
		t.Errorf("an already-invisible god's arrival was announced:\n%s", watcher.transcript())
	}
}
