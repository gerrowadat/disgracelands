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
// character's constitution adds on every level gained. Read through
// HitPointBonus below.
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

// The rest of the ability-modifier tables from constants.c, and the accessors
// the C reaches them through.
//
// They are transcribed mechanically and apply_test.go re-parses constants.c
// and compares every entry, the same arrangement the title tables use. A
// hand-copied table of 31 rows by 4 columns is 124 chances to make a mistake
// that shows up years later as one weapon doing slightly the wrong damage.

// StrengthApply is str_app[] (constants.c:520).
type StrengthApply struct {
	// ToHit is the THAC0 bonus or penalty.
	ToHit int32
	// ToDamage is added to every hit.
	ToDamage int32
	// CarryWeight is the most that can be carried.
	CarryWeight int32
	// WieldWeight is the heaviest weapon that can be wielded.
	WieldWeight int32
}

// DexterityApply is dex_app[] (constants.c:600).
type DexterityApply struct {
	// Reaction orders the combat round.
	Reaction int32
	// MissileAttack is the bonus to hit with a thrown or fired weapon.
	MissileAttack int32
	// Defensive is applied to armour class — negative is better, as always
	// in this game.
	Defensive int32
}

// DexteritySkillApply is dex_app_skill[] (constants.c:560): the thief skill
// percentages dexterity adjusts.
type DexteritySkillApply struct {
	PickPockets int32
	PickLocks   int32
	Traps       int32
	Sneak       int32
	Hide        int32
}

// strengthIndex ports STRENGTH_APPLY_INDEX (utils.h:337).
//
// The table is not indexed by strength. Rows 0-25 are the plain scores, and
// rows 26-30 are the five bands of exceptional strength that only an
// 18-strength warrior can have — which is why a character with strength 18 and
// a percentile of 100 reads row 30 rather than row 18, and why rows 19-25
// (strength 19 and above, reachable only by magic or by an implementor) sit
// between the two.
func strengthIndex(strength, percentile int32) int {
	if percentile == 0 || strength != 18 {
		return abilityIndex(strength)
	}
	switch {
	case percentile <= 50:
		return 26
	case percentile <= 75:
		return 27
	case percentile <= 90:
		return 28
	case percentile <= 99:
		return 29
	}
	return 30
}

// Strength returns the modifiers for a strength score and its exceptional
// percentile.
func Strength(strength, percentile int32) StrengthApply {
	return strApply[strengthIndex(strength, percentile)]
}

// Dexterity returns the modifiers for a dexterity score.
func Dexterity(dex int32) DexterityApply { return dexApply[abilityIndex(dex)] }

// DexteritySkills returns the thief-skill adjustments for a dexterity score.
func DexteritySkills(dex int32) DexteritySkillApply { return dexApplySkill[abilityIndex(dex)] }

// LearnPercent is int_app[].learn: how much of a skill one practice session
// teaches.
func LearnPercent(intelligence int32) int32 { return intApplyLearn[abilityIndex(intelligence)] }

// Practices is wis_app[].bonus: extra practice sessions per level.
func Practices(wisdom int32) int32 { return wisApplyPractices[abilityIndex(wisdom)] }

// HitPointBonus is con_app[].hitp: hit points constitution adds per level.
func HitPointBonus(constitution int32) int32 { return conApplyHitPoints[abilityIndex(constitution)] }

var strApply = [31]StrengthApply{
	{ToHit: -5, ToDamage: -4, CarryWeight: 0, WieldWeight: 0},    // 0
	{ToHit: -5, ToDamage: -4, CarryWeight: 3, WieldWeight: 1},    // 1
	{ToHit: -3, ToDamage: -2, CarryWeight: 3, WieldWeight: 2},    // 2
	{ToHit: -3, ToDamage: -1, CarryWeight: 10, WieldWeight: 3},   // 3
	{ToHit: -2, ToDamage: -1, CarryWeight: 25, WieldWeight: 4},   // 4
	{ToHit: -2, ToDamage: -1, CarryWeight: 55, WieldWeight: 5},   // 5
	{ToHit: -1, ToDamage: 0, CarryWeight: 80, WieldWeight: 6},    // 6
	{ToHit: -1, ToDamage: 0, CarryWeight: 90, WieldWeight: 7},    // 7
	{ToHit: 0, ToDamage: 0, CarryWeight: 100, WieldWeight: 8},    // 8
	{ToHit: 0, ToDamage: 0, CarryWeight: 100, WieldWeight: 9},    // 9
	{ToHit: 0, ToDamage: 0, CarryWeight: 115, WieldWeight: 10},   // 10
	{ToHit: 0, ToDamage: 0, CarryWeight: 115, WieldWeight: 11},   // 11
	{ToHit: 0, ToDamage: 0, CarryWeight: 140, WieldWeight: 12},   // 12
	{ToHit: 0, ToDamage: 0, CarryWeight: 140, WieldWeight: 13},   // 13
	{ToHit: 0, ToDamage: 0, CarryWeight: 170, WieldWeight: 14},   // 14
	{ToHit: 0, ToDamage: 0, CarryWeight: 170, WieldWeight: 15},   // 15
	{ToHit: 0, ToDamage: 1, CarryWeight: 195, WieldWeight: 16},   // 16
	{ToHit: 1, ToDamage: 1, CarryWeight: 220, WieldWeight: 18},   // 17
	{ToHit: 1, ToDamage: 2, CarryWeight: 255, WieldWeight: 20},   // 18
	{ToHit: 3, ToDamage: 7, CarryWeight: 640, WieldWeight: 40},   // 19
	{ToHit: 3, ToDamage: 8, CarryWeight: 700, WieldWeight: 40},   // 20
	{ToHit: 4, ToDamage: 9, CarryWeight: 810, WieldWeight: 40},   // 21
	{ToHit: 4, ToDamage: 10, CarryWeight: 970, WieldWeight: 40},  // 22
	{ToHit: 5, ToDamage: 11, CarryWeight: 1130, WieldWeight: 40}, // 23
	{ToHit: 6, ToDamage: 12, CarryWeight: 1440, WieldWeight: 40}, // 24
	{ToHit: 7, ToDamage: 14, CarryWeight: 1750, WieldWeight: 40}, // 25
	{ToHit: 1, ToDamage: 3, CarryWeight: 280, WieldWeight: 22},   // 26
	{ToHit: 2, ToDamage: 3, CarryWeight: 305, WieldWeight: 24},   // 27
	{ToHit: 2, ToDamage: 4, CarryWeight: 330, WieldWeight: 26},   // 28
	{ToHit: 2, ToDamage: 5, CarryWeight: 380, WieldWeight: 28},   // 29
	{ToHit: 3, ToDamage: 6, CarryWeight: 480, WieldWeight: 30},   // 30
}

var dexApply = [26]DexterityApply{
	{Reaction: -7, MissileAttack: -7, Defensive: 6}, // 0
	{Reaction: -6, MissileAttack: -6, Defensive: 5}, // 1
	{Reaction: -4, MissileAttack: -4, Defensive: 5}, // 2
	{Reaction: -3, MissileAttack: -3, Defensive: 4}, // 3
	{Reaction: -2, MissileAttack: -2, Defensive: 3}, // 4
	{Reaction: -1, MissileAttack: -1, Defensive: 2}, // 5
	{Reaction: 0, MissileAttack: 0, Defensive: 1},   // 6
	{Reaction: 0, MissileAttack: 0, Defensive: 0},   // 7
	{Reaction: 0, MissileAttack: 0, Defensive: 0},   // 8
	{Reaction: 0, MissileAttack: 0, Defensive: 0},   // 9
	{Reaction: 0, MissileAttack: 0, Defensive: 0},   // 10
	{Reaction: 0, MissileAttack: 0, Defensive: 0},   // 11
	{Reaction: 0, MissileAttack: 0, Defensive: 0},   // 12
	{Reaction: 0, MissileAttack: 0, Defensive: 0},   // 13
	{Reaction: 0, MissileAttack: 0, Defensive: 0},   // 14
	{Reaction: 0, MissileAttack: 0, Defensive: -1},  // 15
	{Reaction: 1, MissileAttack: 1, Defensive: -2},  // 16
	{Reaction: 2, MissileAttack: 2, Defensive: -3},  // 17
	{Reaction: 2, MissileAttack: 2, Defensive: -4},  // 18
	{Reaction: 3, MissileAttack: 3, Defensive: -4},  // 19
	{Reaction: 3, MissileAttack: 3, Defensive: -4},  // 20
	{Reaction: 4, MissileAttack: 4, Defensive: -5},  // 21
	{Reaction: 4, MissileAttack: 4, Defensive: -5},  // 22
	{Reaction: 4, MissileAttack: 4, Defensive: -5},  // 23
	{Reaction: 5, MissileAttack: 5, Defensive: -6},  // 24
	{Reaction: 5, MissileAttack: 5, Defensive: -6},  // 25
}

var dexApplySkill = [26]DexteritySkillApply{
	{PickPockets: -99, PickLocks: -99, Traps: -90, Sneak: -99, Hide: -60}, // 0
	{PickPockets: -90, PickLocks: -90, Traps: -60, Sneak: -90, Hide: -50}, // 1
	{PickPockets: -80, PickLocks: -80, Traps: -40, Sneak: -80, Hide: -45}, // 2
	{PickPockets: -70, PickLocks: -70, Traps: -30, Sneak: -70, Hide: -40}, // 3
	{PickPockets: -60, PickLocks: -60, Traps: -30, Sneak: -60, Hide: -35}, // 4
	{PickPockets: -50, PickLocks: -50, Traps: -20, Sneak: -50, Hide: -30}, // 5
	{PickPockets: -40, PickLocks: -40, Traps: -20, Sneak: -40, Hide: -25}, // 6
	{PickPockets: -30, PickLocks: -30, Traps: -15, Sneak: -30, Hide: -20}, // 7
	{PickPockets: -20, PickLocks: -20, Traps: -15, Sneak: -20, Hide: -15}, // 8
	{PickPockets: -15, PickLocks: -10, Traps: -10, Sneak: -20, Hide: -10}, // 9
	{PickPockets: -10, PickLocks: -5, Traps: -10, Sneak: -15, Hide: -5},   // 10
	{PickPockets: -5, PickLocks: 0, Traps: -5, Sneak: -10, Hide: 0},       // 11
	{PickPockets: 0, PickLocks: 0, Traps: 0, Sneak: -5, Hide: 0},          // 12
	{PickPockets: 0, PickLocks: 0, Traps: 0, Sneak: 0, Hide: 0},           // 13
	{PickPockets: 0, PickLocks: 0, Traps: 0, Sneak: 0, Hide: 0},           // 14
	{PickPockets: 0, PickLocks: 0, Traps: 0, Sneak: 0, Hide: 0},           // 15
	{PickPockets: 0, PickLocks: 5, Traps: 0, Sneak: 0, Hide: 0},           // 16
	{PickPockets: 5, PickLocks: 10, Traps: 0, Sneak: 5, Hide: 5},          // 17
	{PickPockets: 10, PickLocks: 15, Traps: 5, Sneak: 10, Hide: 10},       // 18
	{PickPockets: 15, PickLocks: 20, Traps: 10, Sneak: 15, Hide: 15},      // 19
	{PickPockets: 15, PickLocks: 20, Traps: 10, Sneak: 15, Hide: 15},      // 20
	{PickPockets: 20, PickLocks: 25, Traps: 10, Sneak: 15, Hide: 20},      // 21
	{PickPockets: 20, PickLocks: 25, Traps: 15, Sneak: 20, Hide: 20},      // 22
	{PickPockets: 25, PickLocks: 25, Traps: 15, Sneak: 20, Hide: 20},      // 23
	{PickPockets: 25, PickLocks: 30, Traps: 15, Sneak: 25, Hide: 25},      // 24
	{PickPockets: 25, PickLocks: 30, Traps: 15, Sneak: 25, Hide: 25},      // 25
}

var intApplyLearn = [26]int32{
	3, 5, 7, 8, 9, 10, // 0-5
	11, 12, 13, 15, 17, 19, // 6-11
	22, 25, 30, 35, 40, 45, // 12-17
	50, 53, 55, 56, 57, 58, // 18-23
	59, 60, // 24-25
}

// RemortMask is the bit a class occupies in the remort vector, from class.c's
// `pc_class_remort_masks`, or 0 for a class with none.
//
// Paladin has a mask in the C's table (16) like every other class, and no
// `IS_PALADIN` macro reading it — the four macros in utils.h:508 cover mage,
// cleric, thief and warrior only. So the bit is set, saved and listed by
// `remort` as normal, and what it does *not* do is gate abilities the way the
// other four do. That is a fact about how paladin powers are checked, not
// about whether the vector remembers somebody was one: it does.
func RemortMask(class int32) Flags {
	if mask, ok := classRemortMasks[class]; ok {
		return remortFlags(mask)
	}
	return 0
}

// RemortFlagsOf and SetRemortFlags read and write the record's remort vector
// as a bitfield.
//
// The record stores it as an int32 because that is its width in the player
// file — one of the `spare` longs `char_file_u` reserves — so the conversion
// belongs here rather than at every caller.
func RemortFlagsOf(rec *PlayerRecord) Flags {
	if rec == nil {
		return 0
	}
	return remortFlags(rec.RemortVector)
}

// SetRemortFlags writes the vector back.
func SetRemortFlags(rec *PlayerRecord, f Flags) {
	if rec != nil {
		rec.RemortVector = int32(uint32(f)) //nolint:gosec // the same reinterpretation as remortFlags, reversed
	}
}
