// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// TestCastingADamageSpell end to end: the mobile loses hit points and starts
// fighting back.
func TestCastingADamageSpell(t *testing.T) {
	srv, _ := newTestServer(t)
	// The real archive: magic missile's own registered message
	// (skill_message, via SkillDamage) is what a cast prints now, the
	// same table kick/bash/backstab already draw from — see
	// docs/design/data-format.md §11 6c-ii/6c-iii.
	loadRealFightMessages(t, srv)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	dog := spawnDog(t, srv, ImmortStartRoom)
	var before int32
	inWorld(t, srv, func(w *game.Live) { before = dog.Record.Points.Hit })

	c.send("cast 'magic missile' dog")
	// The real archive's Magic Missile hit line (data/misc/messages) —
	// magic missile always deals damage (game.SpellDamage has no miss
	// chance of its own), so the deterministic test RNG always lands on
	// the hit block, never miss or die, against a dog with this much HP.
	c.expect("You watch with selfpride as your magic missile hits a large dog!")

	inWorld(t, srv, func(w *game.Live) {
		if dog.Record.Points.Hit >= before {
			t.Errorf("the dog is on %d hit points, was %d", dog.Record.Points.Hit, before)
		}
		if dog.Fighting == nil {
			t.Error("a violent spell did not start a fight")
		}
	})
}

// TestCastingCostsMana, and a caster without enough is refused.
func TestCastingCostsMana(t *testing.T) {
	srv, _ := newTestServer(t)
	loadRealFightMessages(t, srv)
	addr := listening(t, srv)

	// A mortal: an immortal casts free (the C exempts them from the mana
	// check entirely).
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "m")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "m")
	spawnDog(t, srv, MortalStartRoom)

	var caster *game.Character
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		caster = w.Find("Welmar")
		caster.Record.Level = 10
		if caster.Record.Skills == nil {
			caster.Record.Skills = map[int32]int32{}
		}
		caster.Record.Skills[game.SpellMagicMissile] = 100
	}); err != nil {
		t.Fatal(err)
	}

	var before int32
	inWorld(t, srv, func(w *game.Live) { before = caster.Record.Points.Mana })

	c.send("cast 'magic missile' dog")
	c.expectAny("magic missile", "lost your concentration")

	inWorld(t, srv, func(w *game.Live) {
		if caster.Record.Points.Mana >= before {
			t.Errorf("mana is %d, was %d — casting cost nothing",
				caster.Record.Points.Mana, before)
		}
		// Drained, they are refused.
		caster.Record.Points.Mana = 0
	})
	c.send("cast 'magic missile' dog")
	c.expect("You haven't the energy to cast that spell!")
}

// TestAWarriorDoesNotKnowSpells.
func TestAWarriorDoesNotKnowSpells(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "w")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "w")

	c.send("cast 'magic missile'")
	c.expect("You do not know that spell!")
}

// TestSilenceStopsCasting, which is the local spell's whole point.
func TestSilenceStopsCasting(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		rec := w.Find("Zod").Record
		rec.AffectFlags = rec.AffectFlags.Set(game.AffectSilence)
	}); err != nil {
		t.Fatal(err)
	}

	c.send("cast 'magic missile'")
	c.expect("You try, but the words simply fail you.")
}
