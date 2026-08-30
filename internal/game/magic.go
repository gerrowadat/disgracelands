// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "github.com/gerrowadat/disgracelands/internal/rng"

// What spells do, ported from mag_damage and mag_points (magic.c).
//
// Two routines of the ten. The rest — affects, unaffects, summons, creations,
// areas, groups, masses, alter-objects and the manual spells — arrive with
// the affect system they mostly need.

// MaxSpells is MAX_SPELLS (spells.h): the boundary between spell numbers and
// skill numbers, and the upper bound `cast` checks.
const MaxSpells SpellID = 130

// SpellDamage is what a damage spell does before saving throws, porting the
// switch in mag_damage (magic.c:170).
//
// The magic-user/other split runs through most of it: the same spell rolls d8
// for a mage and d6 for anybody else, which is the whole of a mage's
// advantage at the low levels. The test is the remort-aware IS_MAGIC_USER, so
// a cleric who remorted through mage keeps the better dice.
func SpellDamage(spell SpellID, caster *PlayerRecord, victim *PlayerRecord, level int32, r *rng.Rand) int32 {
	mage := IsMagicUser(caster)
	// d8 for a magic-user, d6 for everybody else.
	size := int32(6)
	if mage {
		size = 8
	}

	switch spell {
	case SpellMagicMissile, SpellChillTouch:
		return r.Dice(1, size) + 1
	case SpellBurningHands:
		return r.Dice(3, size) + 3
	case SpellShockingGrasp:
		return r.Dice(5, size) + 5
	case SpellLightningBolt:
		return r.Dice(7, size) + 7
	case SpellColorSpray:
		return r.Dice(9, size) + 9
	case SpellFireball:
		return r.Dice(11, size) + 11

	// The two local spells, both of which ignore the class split entirely —
	// the C writes out both branches identically.
	case SpellOuchie:
		return r.Dice(1, 1) + 399
	case SpellImmolate:
		return r.Dice(1, 1) + 999

	case SpellCallLightning:
		return r.Dice(7, 8) + 7
	case SpellHarm:
		return r.Dice(8, 8) + 8
	case SpellEarthquake:
		return r.Dice(2, 8) + level

	case SpellEnergyDrain:
		// A victim of level 2 or below is simply destroyed.
		if victim != nil && victim.Level <= 2 {
			return 100
		}
		return r.Dice(1, 10)
	}
	return 0
}

// DispelResult is what dispel evil and dispel good decided. They are the two
// spells that can turn on the caster.
type DispelResult struct {
	// Damage is how much, before saving throws.
	Damage int32
	// Backfired is set when the caster is the one taking it — a good caster
	// casting dispel good, or an evil one casting dispel evil.
	Backfired bool
	// Protected is set when the gods intervene and nothing happens.
	Protected bool
}

// Dispel handles dispel evil and dispel good, porting their cases in
// mag_damage.
//
// Both are symmetric and both are traps: casting one that matches your own
// alignment turns it on you and takes all but one hit point. Casting it at
// somebody of the opposite alignment to its target does nothing.
func Dispel(spell SpellID, caster, victim *PlayerRecord, r *rng.Rand) DispelResult {
	damage := r.Dice(6, 8) + 6

	casterMatches := IsEvil(caster)
	victimProtected := IsGood(victim)
	if spell == SpellDispelGood {
		casterMatches = IsGood(caster)
		victimProtected = IsEvil(victim)
	}

	switch {
	case casterMatches:
		// It turns on the caster and leaves them on one hit point.
		return DispelResult{Damage: caster.Points.Hit - 1, Backfired: true}
	case victimProtected:
		return DispelResult{Protected: true}
	}
	return DispelResult{Damage: damage}
}

// Healing is what a healing spell restores, porting mag_points (magic.c).
type Healing struct {
	Amount  int32
	Message string
}

// SpellHealing returns the healing a spell does and what the target is told.
func SpellHealing(spell SpellID, victim *PlayerRecord, level int32, r *rng.Rand) Healing {
	switch spell {
	case SpellCureLight:
		return Healing{r.Dice(1, 8) + 1 + level/4, "You feel better.\r\n"}
	case SpellCureCritic:
		return Healing{r.Dice(3, 8) + 3 + level/4, "You feel a lot better!\r\n"}
	case SpellHeal:
		return Healing{100 + r.Dice(3, 8), "A warm feeling floods your body.\r\n"}
	case SpellFullHeal:
		// A local spell, and the only one that is defined in terms of what is
		// missing rather than a roll.
		if victim == nil {
			return Healing{}
		}
		return Healing{
			victim.Points.MaxHit - victim.Points.Hit,
			"Oooh. That feels just *lovely*.\r\n",
		}
	}
	return Healing{}
}
