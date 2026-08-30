// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// do_who's annotations (act.informative.c:1169-1201), through a real socket.
//
// Every one of these is read off the character's *own* who-line, which is the
// cheapest way to sidestep CAN_SEE: SELF is the first test in it, so somebody
// carrying AFF_INVISIBLE or an invis level is still on their own list. That
// matters for two of the cases here and is free for the rest.

// whoLineFor returns the who-list line naming the given character, from a
// `who` this function runs itself.
//
// It slices the transcript from where it stood *before* the command, because
// expect() matches anything already in the buffer — an annotation left over
// from an earlier case in a table would satisfy a later one, and the test
// would pass without the code doing anything.
func whoLineFor(t *testing.T, c *client, name string) string {
	t.Helper()

	before := len(c.transcript())
	c.send("who")
	c.settle()

	for _, line := range strings.Split(c.transcript()[before:], "\n") {
		if strings.Contains(line, "] "+name) {
			return strings.TrimRight(line, "\r")
		}
	}
	t.Fatalf("no who-list line for %s in:\n%s", name, c.transcript()[before:])
	return ""
}

// setRecord edits a logged-in character's record on the world goroutine.
func setRecord(t *testing.T, srv *Server, name string, f func(rec *game.PlayerRecord)) {
	t.Helper()
	inWorld(t, srv, func(w *game.Live) {
		ch := w.Find(name)
		if ch == nil || ch.Record == nil {
			t.Errorf("no character called %s", name)
			return
		}
		f(ch.Record)
	})
}

// TestWhoPrintsItsAnnotations, one flag at a time. Each case sets exactly the
// bits it names and clears them again, so the line under test is the flag's
// own doing.
func TestWhoPrintsItsAnnotations(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	for _, tc := range []struct {
		name string
		set  func(rec *game.PlayerRecord)
		want string
	}{
		{"deaf", func(r *game.PlayerRecord) {
			r.Preferences = r.Preferences.Set(game.PrefDeaf)
		}, " (deaf)"},
		{"notell", func(r *game.PlayerRecord) {
			r.Preferences = r.Preferences.Set(game.PrefNoTell)
		}, " (notell)"},
		{"nogossip", func(r *game.PlayerRecord) {
			r.Preferences = r.Preferences.Set(game.PrefNoGoss)
		}, " (nogossip)"},
		{"quest", func(r *game.PlayerRecord) {
			r.Preferences = r.Preferences.Set(game.PrefQuest)
		}, " (quest)"},
		{"thief", func(r *game.PlayerRecord) {
			r.PlayerFlags = r.PlayerFlags.Set(game.PlayerThief)
		}, " (THIEF)"},
		{"killer", func(r *game.PlayerRecord) {
			r.PlayerFlags = r.PlayerFlags.Set(game.PlayerKiller)
		}, " (KILLER)"},
		{"writing", func(r *game.PlayerRecord) {
			r.PlayerFlags = r.PlayerFlags.Set(game.PlayerWriting)
		}, " (writing)"},
		{"mailing", func(r *game.PlayerRecord) {
			r.PlayerFlags = r.PlayerFlags.Set(game.PlayerMailing)
		}, " (mailing)"},
		{"invis", func(r *game.PlayerRecord) {
			r.AffectFlags = r.AffectFlags.Set(game.AffectInvisible)
		}, " (invis)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setRecord(t, srv, "Bystander", tc.set)
			line := whoLineFor(t, mortal, "Bystander")
			if !strings.Contains(line, tc.want) {
				t.Errorf("who-list line is %q, want it to carry %q", line, tc.want)
			}

			// Back to a clean character for the next case.
			setRecord(t, srv, "Bystander", func(r *game.PlayerRecord) {
				r.Preferences = 0
				r.PlayerFlags = 0
				r.AffectFlags = 0
			})
			if line := whoLineFor(t, mortal, "Bystander"); strings.Contains(line, tc.want) {
				t.Errorf("who-list line still carries %q with the flag cleared: %q", tc.want, line)
			}
		})
	}
}

// TestWhoSaysMailingRatherThanWriting pins the dead arm rather than fixing it.
//
// do_mail sets PLR_WRITING and PLR_MAILING both, and do_who tests MAILING
// first with WRITING on the `else` (act.informative.c:1174-1177) — so the
// who-list never says "(writing)" for somebody composing a letter. `wiznet @`
// tests the same two bits the other way round (act.wizard.c:1907-1911) and so
// never says "(Writing mail)". Reproduced on purpose; see
// docs/weirdnumbers.md.
func TestWhoSaysMailingRatherThanWriting(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	setRecord(t, srv, "Bystander", func(r *game.PlayerRecord) {
		r.PlayerFlags = r.PlayerFlags.Set(game.PlayerWriting).Set(game.PlayerMailing)
	})

	line := whoLineFor(t, mortal, "Bystander")
	if !strings.Contains(line, " (mailing)") {
		t.Errorf("who-list line is %q, want it to say (mailing)", line)
	}
	if strings.Contains(line, " (writing)") {
		t.Errorf("who-list line says (writing) as well as (mailing): %q", line)
	}
}

// TestWhoPrefersTheInvisLevelToTheInvisibilityFlag. The C's `else if`
// (act.informative.c:1169-1172): a god who is both invis-levelled and
// magically invisible reads "(i34)", not "(invis)".
func TestWhoPrefersTheInvisLevelToTheInvisibilityFlag(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	god, _ := twoInARoom(t, srv, addr)

	god.send("invis")
	god.expect("Your invisibility level is 34.")
	affect(t, srv, "Zod", game.AffectInvisible)

	line := whoLineFor(t, god, "Zod")
	if !strings.Contains(line, " (i34)") {
		t.Errorf("who-list line is %q, want it to say (i34)", line)
	}
	if strings.Contains(line, " (invis)") {
		t.Errorf("who-list line says (invis) as well as the level: %q", line)
	}
}

// TestWhoShowsPaladinStandingOnlyToPaladins. The `<DoC>` block is guarded by
// GET_CLASS == CLASS_PALADIN (act.informative.c:1193), so the same bits on a
// character of any other class print nothing — which is what stops a remort
// carrying somebody else's disgrace around.
func TestWhoShowsPaladinStandingOnlyToPaladins(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)
	_, mortal := twoInARoom(t, srv, addr)

	setRecord(t, srv, "Bystander", func(r *game.PlayerRecord) {
		game.SetSpecFlags(r, game.NewSet(game.PaladinUnworthy, game.PaladinFallen))
	})

	// Created as a warrior by twoInARoom.
	if line := whoLineFor(t, mortal, "Bystander"); strings.Contains(line, "WORTHY") ||
		strings.Contains(line, "FALLEN") {
		t.Errorf("a warrior carrying the paladin bits was annotated: %q", line)
	}

	setRecord(t, srv, "Bystander", func(r *game.PlayerRecord) { r.Class = game.ClassPaladin })

	line := whoLineFor(t, mortal, "Bystander")
	// The C's order within the block: UNWORTHY then FALLEN.
	if !strings.Contains(line, " (UNWORTHY) (FALLEN)") {
		t.Errorf("who-list line is %q, want it to say (UNWORTHY) (FALLEN)", line)
	}
}
