// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"math"

	"github.com/gerrowadat/disgracelands/internal/rng"
)

// The numbers behind a swing, ported from fight.c's compute_thaco,
// compute_armor_class and the damage half of hit().
//
// THAC0 is "to hit armour class zero", the AD&D mechanic this inherited:
// lower is better for the attacker, lower is better for the defender's
// armour, and a swing lands when the attacker's THAC0 minus a d20 is at most
// the defender's armour class. It reads backwards and it is the game.

// Fighter is what the combat formulas need about a combatant beyond their
// record: what they are wielding, what they are doing, and whether they are a
// mobile.
type Fighter interface {
	IsNPC() bool
	Position() Position
	// Wielded is the weapon in hand, or nil for bare hands.
	Wielded() *Object
	// Sanctuary reports AFF_SANCTUARY, which halves incoming damage.
	Sanctuary() bool
}

// THAC0 returns the class table's value, porting thaco (class.c).
//
// The table covers levels 0 to 34. The C's switch has no break after each
// class's inner switch, so an out-of-range level falls through into the next
// class's table — unreachable, since every level in range is covered, but the
// reason this returns a flat 100 instead is that falling through to a
// different class's numbers is not behaviour worth reproducing.
func THAC0(class, level int32) int32 {
	table, ok := thacoTable[class]
	if !ok || level < 0 || int(level) >= len(table) {
		return 100
	}
	return table[level]
}

// ComputeTHAC0 is the attacker's to-hit number, porting compute_thaco
// (fight.c).
//
// The two ability adjustments are the subtle part. The C writes them as
//
//	calc_thaco -= (GET_INT(ch) - 13) / 1.5;
//
// which is floating-point division assigned back into an int, so *each*
// compound assignment truncates towards zero separately. Doing the arithmetic
// in integers, or combining the two terms before subtracting, gives different
// answers. The order and the two separate truncations are reproduced.
func ComputeTHAC0(rec *PlayerRecord, f Fighter) int32 {
	var calc float64

	if f.IsNPC() {
		// The C's comment: "THAC0 for monsters is set in the HitRoll".
		calc = 20
	} else {
		calc = float64(THAC0(rec.Class, rec.Level))
	}

	calc -= float64(Strength(rec.Abilities.Strength, rec.Abilities.StrengthPercentile).ToHit)
	calc -= float64(rec.Points.HitRoll)

	// The C's `calc_thaco -= (GET_INT(ch) - 13) / 1.5` truncates the *result*
	// of the subtraction, not the adjustment: the right-hand side promotes to
	// double, the subtraction happens in double, and the assignment back into
	// an int throws the fraction away. Truncating the adjustment instead —
	// which is the obvious reading — is wrong by one for most inputs, and the
	// answers still look like plausible to-hit numbers. The oracle found it.
	calc = math.Trunc(calc - float64(rec.Abilities.Intelligence-13)/1.5)
	calc = math.Trunc(calc - float64(rec.Abilities.Wisdom-13)/1.5)

	return int32(calc)
}

// ComputeArmorClass is the defender's armour, porting compute_armor_class
// (fight.c). Lower is better, and -100 is the floor.
func ComputeArmorClass(rec *PlayerRecord, f Fighter) int32 {
	ac := rec.Points.Armor
	if f.Position().Awake() {
		ac += Dexterity(rec.Abilities.Dexterity).Defensive * 10
	}
	return max(-100, ac)
}

// Swing is the outcome of one attack.
type Swing struct {
	// Hit is whether it landed.
	Hit bool
	// Damage is how much, before damage() applies its own adjustments.
	Damage int32
	// Roll is the d20, kept because it decides the automatic hit and miss and
	// is worth having in a log.
	Roll int32
}

// Attack rolls one swing, porting the body of hit() up to the call to
// damage().
//
// A natural 20 always hits and a natural 1 always misses, and a victim who is
// not awake is hit regardless of either.
func Attack(attacker, victim *PlayerRecord, af, vf Fighter, r *rng.Rand) Swing {
	thaco := ComputeTHAC0(attacker, af)
	// Integer division by ten: armour class is stored in tenths.
	victimAC := ComputeArmorClass(victim, vf) / 10

	roll := r.Number(1, 20)

	var landed bool
	switch {
	case roll == 20 || !vf.Position().Awake():
		landed = true
	case roll == 1:
		landed = false
	default:
		landed = thaco-roll <= victimAC
	}

	if !landed {
		return Swing{Roll: roll}
	}

	dam := Strength(attacker.Abilities.Strength, attacker.Abilities.StrengthPercentile).ToDamage
	dam += attacker.Points.DamRoll

	if wielded := af.Wielded(); wielded != nil && wielded.Type == ItemWeapon {
		// Values 1 and 2 are the number and size of the damage dice.
		dam += r.Dice(wielded.Values[1], wielded.Values[2])
	} else if af.IsNPC() {
		dam += r.Dice(attacker.DamageDice, attacker.DamageSize)
	} else {
		// "Max 2 bare hand damage for players", says the C.
		dam += r.Number(0, 2)
	}

	// The multiplier for a victim who cannot defend themselves. The C's
	// comment lists 1.33x for sitting through 3.00x for mortally wounded,
	// but the arithmetic is integer division — so the multipliers are
	// actually 1, 1, 2, 2, 2 and 3. The comment has been wrong since 1993
	// and the code is what players experienced.
	if pos := vf.Position(); pos < PosFighting {
		dam *= 1 + (int32(PosFighting)-int32(pos))/3
	}

	return Swing{Hit: true, Damage: max(1, dam), Roll: roll}
}

// maxDamagePerBlow is the ceiling damage() clamps to.
const maxDamagePerBlow int32 = 1000

// ApplyDamage adjusts a raw damage figure the way damage() does before
// subtracting it, porting the arithmetic between damage()'s guards and its
// `GET_HIT(victim) -= dam`.
//
// **The immortal case is a local deviation and it is not a small one.** Stock
// CircleMUD sets `dam = 0` here, under a comment reading "You can't damage an
// immortal!". This tree doubles it instead — the comment was left in place
// when the line was changed. See docs/deviations.md; it is reproduced because
// it is what players fought against.
func ApplyDamage(dam int32, victim *PlayerRecord, vf Fighter) int32 {
	if !vf.IsNPC() && victim.Level >= LevelImmortal {
		dam *= 2
	}
	if vf.Sanctuary() && dam >= 2 {
		dam /= 2
	}
	return max(0, min(dam, maxDamagePerBlow))
}

var thacoTable = map[int32][]int32{
	ClassMagicUser: {
		100, 20, 20, 20, 19, // 0-4
		19, 19, 18, 18, 18, // 5-9
		17, 17, 17, 16, 16, // 10-14
		16, 15, 15, 15, 14, // 15-19
		14, 14, 13, 13, 13, // 20-24
		12, 12, 12, 11, 11, // 25-29
		11, 10, 10, 10, 9, // 30-34
	},
	ClassCleric: {
		100, 20, 20, 20, 18, // 0-4
		18, 18, 16, 16, 16, // 5-9
		14, 14, 14, 12, 12, // 10-14
		12, 10, 10, 10, 8, // 15-19
		8, 8, 6, 6, 6, // 20-24
		4, 4, 4, 2, 2, // 25-29
		2, 1, 1, 1, 1, // 30-34
	},
	ClassThief: {
		100, 20, 20, 19, 19, // 0-4
		18, 18, 17, 17, 16, // 5-9
		16, 15, 15, 14, 14, // 10-14
		13, 13, 12, 12, 11, // 15-19
		11, 10, 10, 9, 9, // 20-24
		8, 8, 7, 7, 6, // 25-29
		6, 5, 5, 4, 4, // 30-34
	},
	ClassWarrior: {
		100, 20, 19, 18, 17, // 0-4
		16, 15, 14, 14, 13, // 5-9
		12, 11, 10, 9, 8, // 10-14
		7, 6, 5, 4, 3, // 15-19
		2, 1, 1, 1, 1, // 20-24
		1, 1, 1, 1, 1, // 25-29
		1, 1, 1, 1, 1, // 30-34
	},
	ClassPaladin: {
		100, 20, 19, 18, 17, // 0-4
		16, 15, 14, 14, 13, // 5-9
		12, 11, 10, 9, 8, // 10-14
		7, 6, 5, 4, 3, // 15-19
		2, 1, 1, 1, 1, // 20-24
		1, 1, 1, 1, 1, // 25-29
		1, 1, 1, 1, 1, // 30-34
	},
}
