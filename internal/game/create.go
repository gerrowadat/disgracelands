// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"time"

	"github.com/gerrowadat/disgracelands/internal/rng"
)

// Character creation, ported from init_char (db.c:2688) and the local
// additions the C makes at the class prompt (interpreter.c's CON_QCLASS).
//
// Creation happens in two steps and it is worth being clear about which does
// what, because getting them the wrong way round produces a character that
// looks right and is not:
//
//	InitChar  runs when the class is chosen. It fills in everything that does
//	          not depend on the class roll, and leaves the character at level
//	          zero — except the first character on an empty roster, who is
//	          made an Implementor here and skips the rest.
//	Start     runs on first entry into the world, and only while the level is
//	          still zero. It is do_start: the roll, the starting points and
//	          skills, and the first level.

// MaxSkills is MAX_SKILLS (structs.h:540). It is fixed by the binary player
// format and does not change.
const MaxSkills = 200

// baseMaxMana and baseMaxMove are what init_char gives every character,
// regardless of class or level. do_start never touches either, so a level-one
// character walks around with 100 mana and 82-odd movement points — which is
// where the C's familiar new-character prompt comes from.
const (
	baseMaxMana int32 = 100
	baseMaxMove int32 = 82
	baseArmor   int32 = 100
)

// MinMaxMana is store_to_char's floor on a loaded character's maximum mana
// (db.c:2254-2255), applied on every load from disk before the stored
// affects go back on. It is the same flat 100 init_char gives everyone, so
// in practice it only fires for a character something else has taken mana
// away from -- which is exactly why it is easy to leave out (#295).
const MinMaxMana = baseMaxMana

// AwayLongEnoughToHeal is SECS_PER_REAL_HOUR (utils.h:116). A character who
// has been logged out at least this long, and is not poisoned, comes back
// with full hit points, mana and movement (db.c:2276-2287).
const AwayLongEnoughToHeal = time.Hour

// implementorExp is the experience init_char hands the first character.
const implementorExp int32 = 7000000

// InitChar fills in a newly created character, porting init_char (db.c:2688).
//
// first says whether this is the first character on the roster: the C tests
// `top_of_p_table == 0` and makes that character the Implementor, which is
// how a fresh install gets an administrator.
func InitChar(rec *PlayerRecord, r *rng.Rand, first bool) {
	if first {
		// "*** if this is our first player --- he be God ***"
		rec.Points.Exp = implementorExp
		rec.Level = LevelImplementor
		rec.Points.MaxHit = 500
		rec.Points.MaxMana = baseMaxMana
		rec.Points.MaxMove = baseMaxMove
	}

	// At level zero this is "the Man" or "the Woman"; do_start sets the real
	// one when they enter the world. An implementor gets theirs here, since
	// do_start will not run for them.
	rec.Title = Title(rec.Class, rec.Level, rec.Sex)
	rec.Description = ""
	// Hometown one, not the mortal start room: init_char sets it literally
	// and nothing in the C changes it at creation.
	rec.Hometown = 1

	// Weight and height are rolled from the sex, which is the only thing the
	// C uses sex for mechanically.
	if rec.Sex == SexMale {
		rec.Weight = randRange(r, 120, 180)
		rec.Height = randRange(r, 160, 200)
	} else {
		rec.Weight = randRange(r, 100, 160)
		rec.Height = randRange(r, 150, 180)
	}

	// Unconditional, and after the first-player block, so everyone gets
	// these. Note the ordering: hit is set from max_hit, which for anyone but
	// the first character is still zero at this point — do_start fixes it.
	rec.Points.MaxMana = baseMaxMana
	rec.Points.Mana = rec.Points.MaxMana
	rec.Points.Hit = rec.Points.MaxHit
	rec.Points.MaxMove = baseMaxMove
	rec.Points.Move = rec.Points.MaxMove
	rec.Points.Armor = baseArmor

	// An implementor knows everything; everyone else knows nothing. The C
	// writes all 200 slots either way, so the map is only populated for the
	// case where the values are not zero.
	rec.Skills = nil
	if rec.Level >= LevelImplementor {
		rec.Skills = make(map[int32]int32, MaxSkills)
		for i := int32(1); i <= MaxSkills; i++ {
			rec.Skills[i] = 100
		}
	}

	rec.AffectFlags = 0
	rec.SavingThrows = [5]int32{}

	// Every statistic starts at 25 and is overwritten by do_start's roll.
	// They are not left at zero because a character who somehow reaches the
	// world without do_start — the first player does exactly that — would
	// otherwise be indexing the ability tables at zero.
	rec.Abilities = Abilities{
		Strength: 25, StrengthPercentile: 100,
		Intelligence: 25, Wisdom: 25, Dexterity: 25,
		Constitution: 25, Charisma: 25,
	}

	cond := int32(24)
	if rec.Level == LevelImplementor {
		cond = -1
	}
	rec.Conditions = [3]int32{cond, cond, cond}

	rec.LoadRoom = NoRoom

	SnapshotReal(rec)
}

// RemortCount is do_who's own count of how many times somebody has remorted
// (act.informative.c:1116-1132).
//
// It walks the class masks from the top down, subtracting each one it can,
// which counts the bits set — and then takes one off for the class they are
// now, since that bit is in the vector too. The `if (prevclasses > 0)` guard
// on the subtraction is the C's and matters: a character with an empty
// vector stays at zero rather than going to -1, which is the value do_who
// uses for an immortal.
func RemortCount(rec *PlayerRecord) int {
	if rec == nil {
		return 0
	}
	remaining, n := rec.RemortVector, 0
	for class := ClassPaladin; class >= 0; class-- {
		mask, ok := classRemortMasks[class]
		if !ok {
			continue
		}
		if remaining >= mask {
			n++
			remaining -= mask
		}
	}
	if n > 0 {
		n--
	}
	return n
}

// classRemortMasks is pc_class_remort_masks (class.c:82): the bit each class
// occupies in the remort vector.
//
// The values are the record's own int32, not Flags, because that is what they
// are assigned to; a test checks each one against the matching Remort*
// constant so the two cannot drift.
var classRemortMasks = map[int32]int32{
	ClassMagicUser: 1,
	ClassCleric:    2,
	ClassThief:     4,
	ClassWarrior:   8,
	// Paladin's bit is in the C's table but no IS_ macro reads it, since
	// paladin is where remorting ends rather than somewhere it passes
	// through.
	ClassPaladin: 16,
}

// ApplyNewCharacterDefaults sets the things Disgracelands does to a character
// at the class prompt that stock CircleMUD does not: the remort vector, the
// preferences a new player should not have to find, and siteok.
//
// It is separate from InitChar because it is a local addition rather than
// part of the stock function, and keeping the seam visible is how the two
// stay tellable apart. See docs/investigations/non-stock-features.md.
func ApplyNewCharacterDefaults(rec *PlayerRecord) {
	// A new player's remort vector is their own class, so the IS_<CLASS>
	// macros — which consult the vector — answer correctly from the start.
	if mask, ok := classRemortMasks[rec.Class]; ok {
		rec.RemortVector = mask
	}
	rec.Preferences = rec.Preferences.Set(
		PrefColour1 | PrefColour2 |
			PrefDisplayHP | PrefDisplayMana | PrefDisplayMove |
			PrefAutoExit)
	rec.PlayerFlags = rec.PlayerFlags.Set(PlayerSiteOK)
}
