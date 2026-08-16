// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "math/rand/v2"

// Classes, matching structs.h:122. The numbers are stored in every player
// record ever written, so they are the format as much as they are an enum.
const (
	ClassUndefined int32 = -1
	ClassMagicUser int32 = 0
	ClassCleric    int32 = 1
	ClassThief     int32 = 2
	ClassWarrior   int32 = 3
	// ClassPaladin is Disgracelands' own fifth class, not stock CircleMUD.
	// See docs/investigations/non-stock-features.md.
	ClassPaladin int32 = 4
)

// ClassNames are the display names from class.c's pc_class_types.
var ClassNames = map[int32]string{
	ClassMagicUser: "Magic User",
	ClassCleric:    "Cleric",
	ClassThief:     "Thief",
	ClassWarrior:   "Warrior",
	ClassPaladin:   "Paladin",
}

// ClassAbbrevs are class.c's class_abbrevs, used by the who-list.
var ClassAbbrevs = map[int32]string{
	ClassMagicUser: "Mu",
	ClassCleric:    "Cl",
	ClassThief:     "Th",
	ClassWarrior:   "Wa",
	ClassPaladin:   "Pa",
}

// ClassName returns a class's display name.
func ClassName(c int32) string {
	if n, ok := ClassNames[c]; ok {
		return n
	}
	return "Undefined"
}

// CreationMenu is the class menu shown at character creation, verbatim from
// class.c:93.
//
// Paladin is deliberately absent: it is reached by remorting, not by
// choosing it. See ParseCreationClass for the discrepancy this exposed in
// the C server.
const CreationMenu = "\r\nSelect a class:\r\n" +
	"  [C]leric\r\n" +
	"  [T]hief\r\n" +
	"  [W]arrior\r\n" +
	"  [M]agic-user\r\n"

// ParseCreationClass interprets a class letter typed at character creation.
//
// **This is a deliberate deviation from the C, recorded in
// docs/proposals/go-port-plan.md.** The C's parse_class (class.c:117) accepts
// 'p' and returns CLASS_PALADIN, and character creation calls that same
// function — so typing the unadvertised letter at the creation prompt made a
// Paladin without remorting. The menu never offered it and the class exists
// to reward remorting, so the C's intent and its behaviour disagree.
//
// Creation follows the intent and rejects 'p'. ParseClass below keeps the C's
// behaviour for the places where it is correct: an implementor's `set class`,
// and remorting.
func ParseCreationClass(arg byte) int32 {
	switch lower(arg) {
	case 'm':
		return ClassMagicUser
	case 'c':
		return ClassCleric
	case 'w':
		return ClassWarrior
	case 't':
		return ClassThief
	}
	return ClassUndefined
}

// ParseClass interprets a class letter anywhere it is not character creation:
// `set class`, and remorting. Matches class.c's parse_class exactly,
// including Paladin.
func ParseClass(arg byte) int32 {
	if c := ParseCreationClass(arg); c != ClassUndefined {
		return c
	}
	if lower(arg) == 'p' {
		return ClassPaladin
	}
	return ClassUndefined
}

func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b - 'A' + 'a'
	}
	return b
}

// RollAbilities rolls a character's statistics, porting roll_real_abils
// (class.c:1726).
//
// Six scores are rolled as 4d6-drop-lowest, sorted descending, and then dealt
// out in a per-class order that gives each class its best roll where it needs
// it. The sort in the C is an insertion into a table using a three-way XOR
// swap; the effect is a descending sort and that is what is reproduced here.
//
// Only a warrior with an 18 strength rolls the exceptional-strength
// percentile, which is why that field is meaningless for everyone else.
func RollAbilities(class int32, rng *rand.Rand) Abilities {
	roll := func() int32 {
		var dice [4]int32
		lowest := int32(7)
		total := int32(0)
		for i := range dice {
			dice[i] = randN(rng, 6) + 1
			total += dice[i]
			if dice[i] < lowest {
				lowest = dice[i]
			}
		}
		return total - lowest
	}

	var table [6]int32
	for i := range table {
		table[i] = roll()
	}
	// Descending, matching what the C's swap loop achieves.
	for i := 0; i < len(table); i++ {
		for j := i + 1; j < len(table); j++ {
			if table[j] > table[i] {
				table[i], table[j] = table[j], table[i]
			}
		}
	}

	var a Abilities
	switch class {
	case ClassMagicUser:
		a.Intelligence, a.Wisdom, a.Dexterity = table[0], table[1], table[2]
		a.Strength, a.Constitution, a.Charisma = table[3], table[4], table[5]
	case ClassCleric:
		a.Wisdom, a.Intelligence, a.Strength = table[0], table[1], table[2]
		a.Dexterity, a.Constitution, a.Charisma = table[3], table[4], table[5]
	case ClassThief:
		a.Dexterity, a.Strength, a.Constitution = table[0], table[1], table[2]
		a.Intelligence, a.Wisdom, a.Charisma = table[3], table[4], table[5]
	case ClassWarrior:
		a.Strength, a.Dexterity, a.Constitution = table[0], table[1], table[2]
		a.Wisdom, a.Intelligence, a.Charisma = table[3], table[4], table[5]
		if a.Strength == 18 {
			a.StrengthPercentile = randN(rng, 101)
		}
	case ClassPaladin:
		// Unreachable at creation — Paladin is remort-only — but the ordering
		// is ported because remorting needs it.
		a.Charisma, a.Wisdom, a.Strength = table[0], table[1], table[2]
		a.Constitution, a.Dexterity, a.Intelligence = table[3], table[4], table[5]
	}
	return a
}

// randN returns a random value in [0, n), as an int32.
//
// The conversion is safe for any n this package uses — a die has six sides
// and a percentile has 101 values — but it is done in one named place rather
// than at each call site, which is the same reason the player-file codec has
// its narrowing helpers.
func randN(rng *rand.Rand, n int32) int32 {
	return int32(rng.IntN(int(n))) //nolint:gosec // bounded by n, which is a small constant at every call site
}

// Sexes, matching structs.h.
const (
	SexNeutral int32 = 0
	SexMale    int32 = 1
	SexFemale  int32 = 2
)

// ParseSex interprets the letter typed at the sex prompt, as
// interpreter.c's nanny does.
func ParseSex(arg byte) int32 {
	switch lower(arg) {
	case 'm':
		return SexMale
	case 'f':
		return SexFemale
	}
	return -1
}
