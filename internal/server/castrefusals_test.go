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

// cast_spell's three refusals (spell_parser.c:506-522), which were declared
// in the spell table and read by nothing (#301). Eleven spells carry
// TAR_SELF_ONLY and two carry TAR_NOT_SELF, so this was not a corner case:
// every self-only detection spell could be put on somebody else.

// TestASelfOnlySpellIsRefusedOnSomebodyElse.
func TestASelfOnlySpellIsRefusedOnSomebodyElse(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")
	dog := spawnDog(t, srv, ImmortStartRoom)

	c.send("cast 'detect magic' dog")
	c.expect("You can only cast this spell upon yourself!")

	// And it really was refused, rather than refused-and-cast-anyway.
	inWorld(t, srv, func(w *game.Live) {
		if dog.Record.AffectFlags.Has(game.AffectDetectMagic) {
			t.Error("the dog was given detect magic by a spell that cannot leave the caster")
		}
	})

	// The same spell on yourself is the case the flag exists to allow.
	c.send("cast 'detect magic'")
	c.expect("Your eyes tingle.")
}

// TestASpellThatCannotBeSelfCastIsRefused. Blindness is TAR_NOT_SELF and is
// *not* flagged violent, so the "could be bad for your health" check above
// it does not catch it -- before this the spell ran and rolled a save.
func TestASpellThatCannotBeSelfCastIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dialClient(t, listening(t, srv))
	c.create("Zod", "swordfish", "m", "m")

	c.send("cast 'blindness' zod")
	c.expect("You cannot cast this spell upon yourself!")

	inWorld(t, srv, func(w *game.Live) {
		if rec := w.Find("Zod").Record; rec.AffectFlags.Has(game.AffectBlind) {
			t.Error("the caster blinded themselves with a TAR_NOT_SELF spell")
		}
	})
}

// TestAGroupSpellOutsideAGroupIsRefusedAndCostsNothing is the half of #301
// that bites. spellGroup returns early when the caster is not grouped, but
// castSpell had already set did = true, so doCast charged the full sixty
// mana for a spell that did nothing and said nothing at all.
//
// The C never reaches that: cast_spell refuses first and returns 0, and
// do_cast subtracts the full mana only when cast_spell returns 1
// (spell_parser.c:519-522, 735-739).
func TestAGroupSpellOutsideAGroupIsRefusedAndCostsNothing(t *testing.T) {
	srv, _ := newTestServer(t)
	addr := listening(t, srv)

	// A mortal, because an immortal is exempt from the mana check and so
	// cannot show the mana half of this at all.
	first := dialClient(t, addr)
	first.create("Zod", "swordfish", "m", "m")
	first.send("quit")
	first.expect("Goodbye")
	first.close()

	c := dialClient(t, addr)
	c.create("Welmar", "hunter2!", "m", "c")

	var caster *game.Character
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		caster = w.Find("Welmar")
		caster.Record.Level = 30
		if caster.Record.Skills == nil {
			caster.Record.Skills = map[game.SpellID]int32{}
		}
		caster.Record.Skills[game.SpellGroupHeal] = 100
		caster.Record.Points.MaxMana, caster.Record.RealMaxMana = 200, 200
		caster.Record.Points.Mana = 200
	}); err != nil {
		t.Fatal(err)
	}

	c.send("cast 'group heal'")
	c.expect("You can't cast this spell if you're not in a group!")

	inWorld(t, srv, func(w *game.Live) {
		if caster.Record.Points.Mana != 200 {
			t.Errorf("mana is %d, want 200: a refused spell costs nothing",
				caster.Record.Points.Mana)
		}
	})
}
