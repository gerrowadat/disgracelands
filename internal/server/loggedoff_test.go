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

// The half of char_to_store that writes back to the character (db.c:2337-
// 2342), and #389.
//
// store_to_char's side of this is already covered by storetochar_test.go —
// an hour away heals, five minutes does not, poison holds it off — and every
// one of those tests puts `last_logon` on disk by hand. That is what let the
// field be stamped at the wrong moment for as long as it was: what the value
// *means* was never asserted, only what store_to_char does with a value
// somebody else chose.
//
// These are about where the value comes from.

// leaveAfter creates a character, backdates the start of their session by
// `session`, hurts them, and sends them away — returning what reached disk.
//
// Backdating rather than waiting: LastLogon is the start of the accounting
// window, so moving it back is indistinguishable from having played that
// long.
func leaveAfter(t *testing.T, srv *Server, addr string, session time.Duration) *game.PlayerRecord {
	t.Helper()

	c := dialClient(t, addr)
	c.create("Zod", "swordfish", "m", "m")
	inWorld(t, srv, func(w *game.Live) {
		who := w.Find("Zod")
		if who == nil || who.Record == nil {
			t.Error("the character is not in the world")
			return
		}
		who.Record.Points.Hit = 1
		who.Record.LastLogon = time.Now().UTC().Add(-session)
	})
	c.send("quit")
	c.expect("Goodbye")
	c.close()
	waitForLogout(t, srv, "Zod")

	rec, err := srv.players.Load(t.Context(), "Zod")
	if err != nil {
		t.Fatalf("loading Zod: %v", err)
	}
	return rec
}

// TestQuittingStampsTheMomentTheyLeft is the field the issue turns on.
//
// `st->last_logon = time(0)` is in char_to_store, so a record on disk says
// when it was *written*. This port wrote it only in Enter, so it said when
// the session began — and the away-an-hour heal, which asks "how long have
// they been gone", was reading how long they had been *here*.
func TestQuittingStampsTheMomentTheyLeft(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := leaveAfter(t, srv, listening(t, srv), 3*time.Hour)

	if age := time.Since(rec.LastLogon); age > time.Minute {
		t.Errorf("the saved last-logon is %v old after a three-hour session; it is stamping "+
			"the login rather than the save", age)
	}
}

// TestALongSessionDoesNotHealOnRelogging is that mistake with its clothes
// off, and the reason it is worth fixing rather than merely tidying.
//
// Measured from the login, a three-hour session *is* three hours "away" the
// instant it ends — so anybody willing to type `quit` and log straight back
// in got full hit points for it. storetochar_test.go's own straight-back
// test cannot catch this: it writes last_logon on disk by hand, so it never
// exercises what the server put there.
func TestALongSessionDoesNotHealOnRelogging(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	leaveAfter(t, srv, addr, 3*time.Hour)

	back := dialClient(t, addr)
	back.login("Zod", "swordfish")

	got := figuresOf(t, srv, "Zod")
	if got.hit != 1 {
		t.Errorf("a three-hour session healed on relogging: %d of %d hit points. The interval "+
			"is being measured from the login rather than from the save", got.hit, got.maxHit)
	}
}

// TestPlayedTimeSurvivesTheSession.
//
// `st->played += time(0) - ch->player.time.logon` (db.c:2337) had no
// counterpart here. Nothing in the tree wrote Played — the only assignment
// was the ascii codec reading one back — so a character's total stayed at
// whatever their file was created with and every session was lost when it
// ended. `score` hid it by adding the current session on the fly, which is
// right about the session you are in and wrong about every one before it.
func TestPlayedTimeSurvivesTheSession(t *testing.T) {
	srv, _ := newTestServer(t)
	rec := leaveAfter(t, srv, listening(t, srv), 30*time.Minute)

	if rec.Played < 29*time.Minute {
		t.Errorf("played time on disk is %v after a half-hour session; the session was lost",
			rec.Played)
	}
}

// TestPlayedTimeAccumulatesAcrossSessions: the point of storing it at all.
func TestPlayedTimeAccumulatesAcrossSessions(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := leaveAfter(t, srv, addr, 30*time.Minute)

	// A second session of the same length, on the same character.
	back := dialClient(t, addr)
	back.login("Zod", "swordfish")
	inWorld(t, srv, func(w *game.Live) {
		if who := w.Find("Zod"); who != nil {
			who.Record.LastLogon = time.Now().UTC().Add(-30 * time.Minute)
		}
	})
	back.send("quit")
	back.expect("Goodbye")
	back.close()
	waitForLogout(t, srv, "Zod")

	rec, err := srv.players.Load(t.Context(), "Zod")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Played <= first.Played {
		t.Errorf("played time is %v after two half-hour sessions, and was %v after one: "+
			"the second was not added", rec.Played, first.Played)
	}
	if rec.Played < 59*time.Minute {
		t.Errorf("played time is %v after two half-hour sessions, want about an hour", rec.Played)
	}
}
