// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"strings"
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

// TestASelfOnlySpellIsRefusedOnSomebodyElse is cast_spell's
// `(tch != ch) && IS_SET(SINFO.targets, TAR_SELF_ONLY)` (spell_parser.c:506).
//
// Eleven spells carry TAR_SELF_ONLY and the flag was read by nothing, so
// every self-only detection spell could be put on somebody else — see
// docs/deviations.md.
func TestASelfOnlySpellIsRefusedOnSomebodyElse(t *testing.T) {
	srv, c := spellbookServer(t)
	dog := prey(t, srv, ImmortStartRoom)

	c.send("cast 'detect magic' dog")
	c.expect("You can only cast this spell upon yourself!")

	if rec := record(t, srv, dog); rec.AffectFlags.Has(game.AffectDetectMagic) {
		t.Error("the dog was given detect magic anyway")
	}
	// And on yourself it still works, which is the half that must not break.
	castSelf(c, "detect magic")
	if rec := playerRecord(t, srv, "Zod"); !rec.AffectFlags.Has(game.AffectDetectMagic) {
		t.Error("detect magic no longer works on the caster")
	}
}

// TestASpellThatCannotBeCastOnYourselfIsRefused is the mirror,
// `(tch == ch) && IS_SET(SINFO.targets, TAR_NOT_SELF)` (spell_parser.c:510).
//
// Blindness and charm person are the two. Without the check the spell ran:
// blindness on yourself rolled a saving throw and usually answered
// "You fail.", which reads like the spell failing rather than like the
// server never having been told you cannot do that.
func TestASpellThatCannotBeCastOnYourselfIsRefused(t *testing.T) {
	srv, c := spellbookServer(t)

	c.send("cast 'blindness' zod")
	c.expect("You cannot cast this spell upon yourself!")

	if rec := playerRecord(t, srv, "Zod"); rec.AffectFlags.Has(game.AffectBlind) {
		t.Error("the caster blinded themselves")
	}
}

// TestAGroupSpellNeedsAGroupAndIsFreeWithout is the third,
// `IS_SET(SINFO.routines, MAG_GROUPS) && !AFF_FLAGGED(ch, AFF_GROUP)`
// (spell_parser.c:513).
//
// The mana is the point. mag_groups returns early for an ungrouped caster
// (magic.c:855) — so the spell already did nothing and said nothing — but
// castSpell had counted it as done, and do_cast spends the mana on anything
// cast_spell returns true for. Sixty points for silence.
func TestAGroupSpellNeedsAGroupAndIsFreeWithout(t *testing.T) {
	srv, c := spellbookServer(t)

	// Read the mana on each attempt rather than once: a lost concentration
	// spends half of it whatever happens afterwards, so the reading has to
	// be the one taken immediately before the cast that got through.
	var before, after int32
	var cast bool
	for try := 0; try < 12 && !cast; try++ {
		lost := strings.Count(c.transcript(), lostConcentration)
		before = playerRecord(t, srv, "Zod").Points.Mana
		c.send("cast 'group heal'")
		c.settle()
		if strings.Count(c.transcript(), lostConcentration) == lost {
			after = playerRecord(t, srv, "Zod").Points.Mana
			cast = true
		}
	}
	if !cast {
		t.Fatalf("twelve casts all lost their concentration; the transcript was:\n%s",
			c.transcript())
	}

	if !c.seen("You can't cast this spell if you're not in a group!") {
		t.Errorf("no refusal; the transcript was:\n%s", c.transcript())
	}
	if after != before {
		t.Errorf("a refused group spell cost %d mana (%d to %d)", before-after, before, after)
	}
}

// TestFullHealSaysNothingAboutBlindness covers mag_unaffects' `default`.
//
// full heal is MAG_POINTS | MAG_UNAFFECTS (spell_parser.c:1007-1009) and the
// archived server never gave it a case in mag_unaffects, so it falls to the
// SYSERR-and-return branch and the unaffects half is silent. See
// docs/weirdnumbers.md. What this pins is the silence: the previous code
// reached NOEFFECT there, so a full heal reported "Nothing seems to happen."
// immediately after restoring every hit point.
func TestFullHealSaysNothingAboutBlindness(t *testing.T) {
	srv, c := spellbookServer(t)
	dog := prey(t, srv, ImmortStartRoom)
	inWorld(t, srv, func(_ *game.Live) { dog.Record.Points.Hit = 50 })

	castOn(c, "full heal", "dog")

	rec := record(t, srv, dog)
	if rec.Points.Hit != rec.Points.MaxHit {
		t.Errorf("the dog is on %d of %d hit points", rec.Points.Hit, rec.Points.MaxHit)
	}
	if c.seen(game.NoEffect) {
		t.Errorf("full heal reported nothing happening; the transcript was:\n%s",
			c.transcript())
	}
}

// TestHealAlsoCuresBlindnessAndDoesNotReportFailing is heal's own pair of
// mag_unaffects behaviours: it is the second spell mapped to SPELL_BLINDNESS,
// and it is the one the NOEFFECT is suppressed for (magic.c:911-915, :932).
func TestHealAlsoCuresBlindnessAndDoesNotReportFailing(t *testing.T) {
	srv, c := spellbookServer(t)
	dog := prey(t, srv, ImmortStartRoom)

	// First on a victim who is not blind at all: the healing must not be
	// followed by a report that nothing happened.
	inWorld(t, srv, func(_ *game.Live) { dog.Record.Points.Hit = 50 })
	castOn(c, "heal", "dog")
	if record(t, srv, dog).Points.Hit <= 50 {
		t.Error("heal did not heal")
	}
	if c.seen(game.NoEffect) {
		t.Errorf("heal reported nothing happening; the transcript was:\n%s", c.transcript())
	}

	// And then on one who is: heal cures blindness as well.
	castOn(c, "blindness", "dog")
	if !record(t, srv, dog).AffectFlags.Has(game.AffectBlind) {
		t.Fatal("the dog was not blinded to begin with")
	}
	castOn(c, "heal", "dog")
	if record(t, srv, dog).AffectFlags.Has(game.AffectBlind) {
		t.Error("heal left the dog blind")
	}
}
