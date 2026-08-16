// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// Ability-modifier tables from constants.c.
//
// The C has six of these — str, dex, int, wis, con and the thief-skill one.
// Only the two that advance_level reads are here; the rest arrive with combat
// and spellcasting, which are the only things that use them. They are indexed
// by the ability score directly, so an out-of-range score would be a read off
// the end of the array in the C; abilityIndex clamps instead.

// conApplyHitPoints is con_app[].hitp (constants.c:634): the hit points a
// character's constitution adds on every level gained.
var conApplyHitPoints = [26]int32{
	-4, -3, -2, -2, -1, // con 0-4
	-1, -1, 0, 0, 0, // con 5-9
	0, 0, 0, 0, 0, // con 10-14
	1, 2, 2, 3, 3, // con 15-19
	4, 5, 5, 5, 6, // con 20-24
	6, // con 25
}

// wisApplyPractices is wis_app[].bonus (constants.c:697): extra practice
// sessions per level.
var wisApplyPractices = [26]int32{
	0, 0, 0, 0, 0, // wis 0-4
	0, 0, 0, 0, 0, // wis 5-9
	0, 0, 2, 2, 3, // wis 10-14
	3, 3, 4, 5, 6, // wis 15-19
	6, 6, 6, 7, 7, // wis 20-24
	7, // wis 25
}

// abilityIndex bounds a score to the tables' range.
//
// The C indexes con_app[] and wis_app[] with the score unchecked, so a score
// outside 0–25 reads past the array. Nothing in the game produces one — rolls
// are 3–18 and the spells that raise a statistic cap at 25 — but reading off
// the end of a table is not behaviour worth reproducing faithfully.
func abilityIndex(score int32) int {
	if score < 0 {
		return 0
	}
	if score > 25 {
		return 25
	}
	return int(score)
}

// Remort bits, from utils.h:505. One per class a character has passed
// through; see docs/investigations/non-stock-features.md.
const (
	RemortMagicUser Flags = 1 << 0
	RemortCleric    Flags = 1 << 1
	RemortThief     Flags = 1 << 2
	RemortWarrior   Flags = 1 << 3
)

// IsMagicUser and friends port the IS_<CLASS> macros (utils.h:505).
//
// These are not `Class == ClassMagicUser`. Disgracelands rewrote them to
// consult the remort vector, so a character who remorted out of a class still
// counts as one for every check in the game — that is what makes the local
// multiclassing work at all. The stock definitions are still in the C source,
// commented out above the replacements.
func IsMagicUser(rec *PlayerRecord) bool { return isClass(rec, ClassMagicUser, RemortMagicUser) }

// IsCleric reports whether the character counts as a cleric.
func IsCleric(rec *PlayerRecord) bool { return isClass(rec, ClassCleric, RemortCleric) }

// IsThief reports whether the character counts as a thief.
func IsThief(rec *PlayerRecord) bool { return isClass(rec, ClassThief, RemortThief) }

// IsWarrior reports whether the character counts as a warrior.
func IsWarrior(rec *PlayerRecord) bool { return isClass(rec, ClassWarrior, RemortWarrior) }

// IsPaladin has no macro in the C — paladin is the remort destination and has
// no bit of its own — so it is the plain class check the C would have made.
func IsPaladin(rec *PlayerRecord) bool { return rec != nil && rec.Class == ClassPaladin }

func isClass(rec *PlayerRecord, class int32, bit Flags) bool {
	if rec == nil {
		return false
	}
	return rec.Class == class || remortFlags(rec.RemortVector).Has(bit)
}

// remortFlags reads the remort vector as a bitfield.
//
// The record holds it as an int32 because that is its width in the player
// file, and Flags is 64 bits wide. Going through uint32 makes the
// reinterpretation explicit and total: every bit pattern maps to exactly one
// Flags value, including the sign bit, which the C would treat as bit 31.
func remortFlags(v int32) Flags {
	return Flags(uint32(v)) //nolint:gosec // a deliberate bit-pattern reinterpretation, not an arithmetic conversion
}
