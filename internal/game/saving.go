// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "github.com/gerrowadat/disgracelands/internal/rng"

// Saving throws, ported from saving_throws (class.c:225) and mag_savingthrow
// (magic.c).
//
// The table is 1,125 numbers: five classes by five save types by forty-odd
// levels. It is checked against the C by re-parsing class.c, because a table
// that large transcribed by hand is a table with a mistake in it.
//
// The numbers read backwards, and the C says so: "Negative apply_saving_throw
// values make saving throws better! Then, so do negative modifiers." The
// table value is a target the roll must beat, so *lower is better* — a
// level-1 mage needs 70 and a level-30 one needs 28.

// SaveType indexes the five saving throws.
type SaveType int

// The five, in the order the record stores them.
const (
	SaveParalyse SaveType = iota
	SaveRod
	SavePetrify
	SaveBreath
	SaveSpell
	// NumSaveTypes is how many there are.
	NumSaveTypes
)

// SavingThrow returns the table value for a class, type and level.
//
// A level past the end of the table returns the last entry rather than the
// C's SYSERR-and-zero: zero means "always saves", and handing a free save to
// anyone off the end of a table is not a failure mode worth reproducing.
func SavingThrow(class Class, save SaveType, level int32) int32 {
	if save < 0 || save >= NumSaveTypes {
		return 0
	}
	table, ok := savingThrowTable[class]
	if !ok {
		// Mobiles and anything unrecognised use the warrior tables, "according
		// to some book" as mag_savingthrow's comment has it.
		table = savingThrowTable[ClassWarrior]
	}
	values := table[save]
	if len(values) == 0 {
		return 0
	}
	if level < 0 {
		level = 0
	}
	if int64(level) >= int64(len(values)) {
		return values[len(values)-1]
	}
	return values[level]
}

// MakesSavingThrow rolls one, porting mag_savingthrow (magic.c).
//
// The character's own saving-throw bonuses are added to the table value, and
// so is the caller's modifier — and because lower is better, a *negative*
// bonus is an improvement. That is the C's arrangement and its comment
// apologises for it.
//
// The floor is worth keeping: `MAX(1, save)` means a target of zero or below
// still needs the roll to come in under 1, so a perfect saving throw is not
// quite automatic.
func MakesSavingThrow(rec *PlayerRecord, isNPC bool, save SaveType, modifier int32, r *rng.Rand) bool {
	class := ClassWarrior
	if !isNPC && rec != nil {
		class = rec.Class
	}

	target := SavingThrow(class, save, rec.Level)
	if save >= 0 && save < NumSaveTypes {
		target += rec.SavingThrows[save]
	}
	target += modifier

	return max(1, target) < r.Number(0, 99)
}

var savingThrowTable = map[Class][NumSaveTypes][]int32{
	ClassMagicUser: {
		SaveParalyse: {
			90, 70, 69, 68, 67, 66, 65, 63, 61, 60, // 0-9
			59, 57, 55, 54, 53, 53, 52, 51, 50, 48, // 10-19
			46, 45, 44, 42, 40, 38, 36, 34, 32, 30, // 20-29
			28, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SaveRod: {
			90, 55, 53, 51, 49, 47, 45, 43, 41, 40, // 0-9
			39, 37, 35, 33, 31, 30, 29, 27, 25, 23, // 10-19
			21, 20, 19, 17, 15, 14, 13, 12, 11, 10, // 20-29
			9, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SavePetrify: {
			90, 65, 63, 61, 59, 57, 55, 53, 51, 50, // 0-9
			49, 47, 45, 43, 41, 40, 39, 37, 35, 33, // 10-19
			31, 30, 29, 27, 25, 23, 21, 19, 17, 15, // 20-29
			13, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SaveBreath: {
			90, 75, 73, 71, 69, 67, 65, 63, 61, 60, // 0-9
			59, 57, 55, 53, 51, 50, 49, 47, 45, 43, // 10-19
			41, 40, 39, 37, 35, 33, 31, 29, 27, 25, // 20-29
			23, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SaveSpell: {
			90, 60, 58, 56, 54, 52, 50, 48, 46, 45, // 0-9
			44, 42, 40, 38, 36, 35, 34, 32, 30, 28, // 10-19
			26, 25, 24, 22, 20, 18, 16, 14, 12, 10, // 20-29
			8, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
	},
	ClassCleric: {
		SaveParalyse: {
			90, 60, 59, 48, 46, 45, 43, 40, 37, 35, // 0-9
			34, 33, 31, 30, 29, 27, 26, 25, 24, 23, // 10-19
			22, 21, 20, 18, 15, 14, 12, 10, 9, 8, // 20-29
			7, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SaveRod: {
			90, 70, 69, 68, 66, 65, 63, 60, 57, 55, // 0-9
			54, 53, 51, 50, 49, 47, 46, 45, 44, 43, // 10-19
			42, 41, 40, 38, 35, 34, 32, 30, 29, 28, // 20-29
			27, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SavePetrify: {
			90, 65, 64, 63, 61, 60, 58, 55, 53, 50, // 0-9
			49, 48, 46, 45, 44, 43, 41, 40, 39, 38, // 10-19
			37, 36, 35, 33, 31, 29, 27, 25, 24, 23, // 20-29
			22, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SaveBreath: {
			90, 80, 79, 78, 76, 75, 73, 70, 67, 65, // 0-9
			64, 63, 61, 60, 59, 57, 56, 55, 54, 53, // 10-19
			52, 51, 50, 48, 45, 44, 42, 40, 39, 38, // 20-29
			37, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SaveSpell: {
			90, 75, 74, 73, 71, 70, 68, 65, 63, 60, // 0-9
			59, 58, 56, 55, 54, 53, 51, 50, 49, 48, // 10-19
			47, 46, 45, 43, 41, 39, 37, 35, 34, 33, // 20-29
			32, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
	},
	ClassThief: {
		SaveParalyse: {
			90, 65, 64, 63, 62, 61, 60, 59, 58, 57, // 0-9
			56, 55, 54, 53, 52, 51, 50, 49, 48, 47, // 10-19
			46, 45, 44, 43, 42, 41, 40, 39, 38, 37, // 20-29
			36, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SaveRod: {
			90, 70, 68, 66, 64, 62, 60, 58, 56, 54, // 0-9
			52, 50, 48, 46, 44, 42, 40, 38, 36, 34, // 10-19
			32, 30, 28, 26, 24, 22, 20, 18, 16, 14, // 20-29
			13, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SavePetrify: {
			90, 60, 59, 58, 58, 56, 55, 54, 53, 52, // 0-9
			51, 50, 49, 48, 47, 46, 45, 44, 43, 42, // 10-19
			41, 40, 39, 38, 37, 36, 35, 34, 33, 32, // 20-29
			31, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SaveBreath: {
			90, 80, 79, 78, 77, 76, 75, 74, 73, 72, // 0-9
			71, 70, 69, 68, 67, 66, 65, 64, 63, 62, // 10-19
			61, 60, 59, 58, 57, 56, 55, 54, 53, 52, // 20-29
			51, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
		SaveSpell: {
			90, 75, 73, 71, 69, 67, 65, 63, 61, 59, // 0-9
			57, 55, 53, 51, 49, 47, 45, 43, 41, 39, // 10-19
			37, 35, 33, 31, 29, 27, 25, 23, 21, 19, // 20-29
			17, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, // 40-40
		},
	},
	ClassWarrior: {
		SaveParalyse: {
			90, 70, 68, 67, 65, 62, 58, 55, 53, 52, // 0-9
			50, 47, 43, 40, 38, 37, 35, 32, 28, 25, // 10-19
			24, 23, 22, 20, 19, 17, 16, 15, 14, 13, // 20-29
			12, 11, 10, 9, 8, 7, 6, 5, 4, 3, // 30-39
			2, 1, 0, 0, 0, 0, 0, 0, 0, 0, // 40-49
			0, // 50-50
		},
		SaveRod: {
			90, 80, 78, 77, 75, 72, 68, 65, 63, 62, // 0-9
			60, 57, 53, 50, 48, 47, 45, 42, 38, 35, // 10-19
			34, 33, 32, 30, 29, 27, 26, 25, 24, 23, // 20-29
			22, 20, 18, 16, 14, 12, 10, 8, 6, 5, // 30-39
			4, 3, 2, 1, 0, 0, 0, 0, 0, 0, // 40-49
			0, // 50-50
		},
		SavePetrify: {
			90, 75, 73, 72, 70, 67, 63, 60, 58, 57, // 0-9
			55, 52, 48, 45, 43, 42, 40, 37, 33, 30, // 10-19
			29, 28, 26, 25, 24, 23, 21, 20, 19, 18, // 20-29
			17, 16, 15, 14, 13, 12, 11, 10, 9, 8, // 30-39
			7, 6, 5, 4, 3, 2, 1, 0, 0, 0, // 40-49
			0, // 50-50
		},
		SaveBreath: {
			90, 85, 83, 82, 80, 75, 70, 65, 63, 62, // 0-9
			60, 55, 50, 45, 43, 42, 40, 37, 33, 30, // 10-19
			29, 28, 26, 25, 24, 23, 21, 20, 19, 18, // 20-29
			17, 16, 15, 14, 13, 12, 11, 10, 9, 8, // 30-39
			7, 6, 5, 4, 3, 2, 1, 0, 0, 0, // 40-49
			0, // 50-50
		},
		SaveSpell: {
			90, 85, 83, 82, 80, 77, 73, 70, 68, 67, // 0-9
			65, 62, 58, 55, 53, 52, 50, 47, 43, 40, // 10-19
			39, 38, 36, 35, 34, 33, 31, 30, 29, 28, // 20-29
			27, 25, 23, 21, 19, 17, 15, 13, 11, 9, // 30-39
			7, 6, 5, 4, 3, 2, 1, 0, 0, 0, // 40-49
			0, // 50-50
		},
	},
	ClassPaladin: {
		SaveParalyse: {
			70, 50, 48, 47, 45, 42, 38, 35, 33, 32, // 0-9
			30, 27, 23, 20, 18, 17, 15, 12, 8, 5, // 10-19
			4, 3, 2, 0, 0, 0, 0, 0, 0, 0, // 20-29
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 40-49
			0, // 50-50
		},
		SaveRod: {
			70, 60, 58, 57, 55, 52, 48, 45, 43, 42, // 0-9
			40, 37, 33, 30, 28, 27, 25, 22, 18, 15, // 10-19
			14, 13, 12, 10, 9, 7, 6, 5, 4, 3, // 20-29
			2, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 40-49
			0, // 50-50
		},
		SavePetrify: {
			70, 55, 53, 52, 50, 47, 43, 40, 38, 37, // 0-9
			35, 32, 28, 25, 23, 22, 20, 17, 13, 10, // 10-19
			9, 8, 6, 5, 4, 3, 1, 0, 0, 0, // 20-29
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 40-49
			0, // 50-50
		},
		SaveBreath: {
			70, 65, 63, 62, 60, 55, 50, 45, 43, 42, // 0-9
			40, 35, 30, 25, 23, 22, 20, 17, 13, 10, // 10-19
			9, 8, 6, 5, 4, 3, 1, 0, 0, 0, // 20-29
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 30-39
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 40-49
			0, // 50-50
		},
		SaveSpell: {
			70, 65, 63, 62, 60, 57, 53, 50, 48, 47, // 0-9
			45, 42, 38, 35, 33, 32, 30, 27, 23, 20, // 10-19
			19, 18, 16, 15, 14, 13, 11, 10, 9, 8, // 20-29
			7, 5, 3, 1, 0, 0, 0, 0, 0, 0, // 30-39
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 40-49
			0, // 50-50
		},
	},
}
