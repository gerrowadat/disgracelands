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

// mag_unaffects' messages (magic.c:910-943), which are the half of #299 that
// the spellbook suite's state assertions cannot see. Each cure has its own
// pair of lines in the C's switch, and the port sent one "You feel better."
// for all three.

// TestCuringBlindnessSaysWhatTheCSays, to the cured and to the room.
func TestCuringBlindnessSaysWhatTheCSays(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	caster := dialClient(t, addr)
	caster.create("Zod", "swordfish", "m", "m")
	watcher := dialClient(t, addr)
	watcher.create("Welmar", "hunter2!", "m", "m")
	// The first character on an empty roster is an implementor and starts
	// in the immortal room; the second is a mortal and does not.
	moveTo(t, srv, "Welmar", ImmortStartRoom)

	// Blinded directly rather than by casting it: blindness rolls a saving
	// throw, and what is being tested here is the cure's messages, not
	// whether a mortal happened to resist.
	inWorld(t, srv, func(w *game.Live) {
		rec := w.Find("Welmar").Record
		rec.Affects = append(rec.Affects, game.Affect{
			Type: game.SpellBlindness, Duration: 20, Bits: game.AffectBlind,
		})
		game.RecomputeAffects(rec)
	})

	caster.send("cast 'cure blind' welmar")
	// act(to_vict, FALSE, victim, 0, ch, TO_CHAR): the cured character.
	watcher.expect("Your vision returns!")
	// act(to_room, TRUE, victim, 0, ch, TO_ROOM), with $n the cured
	// character rather than the caster.
	caster.expect("There's a momentary gleam in Welmar's eyes.")
}

// TestHealDoesNotSayNothingHappensToSomebodyWhoIsNotBlind.
//
// heal is MAG_POINTS | MAG_UNAFFECTS, and its unaffect half is blindness.
// The C skips NOEFFECT for heal alone (`if (spellnum != SPELL_HEAL)`,
// magic.c:932) precisely because healing somebody who is not blind would
// otherwise print "Nothing seems to happen." straight after healing them.
func TestHealDoesNotSayNothingHappensToSomebodyWhoIsNotBlind(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	dog := spawnDog(t, srv, ImmortStartRoom)
	inWorld(t, srv, func(w *game.Live) { dog.Record.Points.Hit = 1 })

	c.send("cast 'heal' dog")
	c.settle()

	if c.seen(game.NoEffect) {
		t.Errorf("heal told the caster nothing happened:\n%s", c.transcript())
	}
	inWorld(t, srv, func(w *game.Live) {
		if dog.Record.Points.Hit <= 1 {
			t.Errorf("the dog is on %d hit points: heal did nothing at all",
				dog.Record.Points.Hit)
		}
	})
}

// A cure with nothing to cure does say so -- for every spell but heal. This
// is the other side of the branch above, and without it "no NOEFFECT" could
// be satisfied by never sending one.
func TestCureBlindOnSomebodyWhoIsNotBlindSaysNothingHappens(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")
	spawnDog(t, srv, ImmortStartRoom)

	c.send("cast 'cure blind' dog")
	c.expect(strings.TrimSuffix(game.NoEffect, "\r\n"))
}
