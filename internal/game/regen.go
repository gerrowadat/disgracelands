// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "time"

// Regeneration and the passage of time, ported from limits.c.
//
// Everything here runs once per mud hour. A mud hour is 75 real seconds
// (utils.h:109), so a character eats, drinks, sobers up and heals on that
// clock rather than on the pulse.

// Mud time, from utils.h:109. A year is a little over eleven real days, which
// is why a character's age matters at all on a server people played for
// years.
const (
	SecondsPerMudHour  = 75
	SecondsPerMudDay   = 24 * SecondsPerMudHour
	SecondsPerMudMonth = 35 * SecondsPerMudDay
	SecondsPerMudYear  = 17 * SecondsPerMudMonth

	MudHour = SecondsPerMudHour * time.Second
)

// startingAge is what age() adds: everyone begins at seventeen (utils.c).
const startingAge = 17

// Age returns a character's age in mud years, porting age() (utils.c).
func Age(rec *PlayerRecord, now time.Time) int32 {
	if rec == nil || rec.Birth.IsZero() {
		return startingAge
	}
	elapsed := now.Sub(rec.Birth)
	if elapsed < 0 {
		return startingAge
	}
	return startingAge + int32(elapsed.Seconds())/SecondsPerMudYear
}

// graf interpolates a value across the bands of a lifetime, porting graf()
// (limits.c:53).
//
// Six line segments and two flat ends: below 15 it is p0, above 79 it is p6,
// and in between it walks from one point to the next. The C's integer
// division truncates and that is reproduced — the curve has small flat spots
// in it as a result, and smoothing them would change every regeneration
// number in the game.
func graf(age, p0, p1, p2, p3, p4, p5, p6 int32) int32 {
	switch {
	case age < 15:
		return p0
	case age <= 29:
		return p1 + ((age-15)*(p2-p1))/15
	case age <= 44:
		return p2 + ((age-30)*(p3-p2))/15
	case age <= 59:
		return p3 + ((age-45)*(p4-p3))/15
	case age <= 79:
		// Twenty years across this band, not fifteen. It looks like a typo
		// and is not: the band is 60..79.
		return p4 + ((age-60)*(p5-p4))/20
	}
	return p6
}

// Condition indexes into PlayerRecord.Conditions.
type Condition int

// The three conditions, in the order the record stores them.
const (
	CondDrunk Condition = iota
	CondFull
	CondThirst
)

// MaxCondition is the ceiling gain_condition clamps to.
const MaxCondition int32 = 24

// CondNotApplicable is the value a condition holds when it does not apply
// to this character at all -- how an immortal's hunger, thirst and
// drunkenness are stored, so that they never get hungry.
//
// It is the C's bare -1 (limits.c:380's `if (GET_COND(ch, condition) == -1)
// return;`) and it stays -1, because it is written to every player file
// ever saved: docs/proposals/idiomatic-go.md §4.4's third case, a sentinel
// that has to survive to disk. What it gets is a name, so that a reader of
// `Conditions[CondFull] == -1` does not have to guess whether it means
// "starving" -- which is what 0 means, and is the opposite.
const CondNotApplicable int32 = -1

// Regenerator supplies what the regeneration formulas need from outside the
// record: whether the character is a mobile, what they are doing, and where
// they are standing.
//
// It is an interface rather than more arguments because the answers come from
// three different places and the formulas should not have to know which.
type Regenerator interface {
	// IsNPC reports whether this is a mobile. Mobiles regenerate their level
	// per tick and skip every other adjustment.
	IsNPC() bool
	// Position is what they are doing.
	Position() Position
	// Poisoned reports AFF_POISON.
	Poisoned() bool
	// GoodRegen reports the ROOM_GOOD_REGEN flag on the room they are in.
	// This is a local addition; see docs/investigations/non-stock-features.md.
	GoodRegen() bool
}

// HitGain is hit points regained per mud hour, porting hit_gain
// (limits.c:128).
func HitGain(rec *PlayerRecord, ctx Regenerator, now time.Time) int32 {
	var gain int32

	if ctx.IsNPC() {
		gain = rec.Level
	} else {
		gain = graf(Age(rec, now), 8, 12, 20, 32, 16, 10, 4)

		switch ctx.Position() {
		case PosSleeping:
			gain += gain / 2
		case PosResting:
			gain += gain / 4
		case PosSitting:
			gain += gain / 8
		}

		// "Ouch", says the comment in the C, and it is not wrong. Note this
		// uses the remort-aware class test, so a warrior who remorted through
		// cleric heals at a caster's rate for the rest of their life.
		if IsMagicUser(rec) || IsCleric(rec) {
			gain /= 2
		}
		if starving(rec) {
			gain /= 4
		}
	}

	return finishGain(gain, ctx)
}

// ManaGain is mana regained per mud hour, porting mana_gain (limits.c:81).
func ManaGain(rec *PlayerRecord, ctx Regenerator, now time.Time) int32 {
	var gain int32

	if ctx.IsNPC() {
		gain = rec.Level
	} else {
		gain = graf(Age(rec, now), 4, 8, 12, 16, 12, 10, 8)

		// Mana is the one where sleeping doubles rather than adding a half,
		// which is why a mage sleeps to recover and a warrior does not have
		// to bother.
		switch ctx.Position() {
		case PosSleeping:
			gain *= 2
		case PosResting:
			gain += gain / 2
		case PosSitting:
			gain += gain / 4
		}

		if IsMagicUser(rec) || IsCleric(rec) {
			gain *= 2
		}
		if starving(rec) {
			gain /= 4
		}
	}

	return finishGain(gain, ctx)
}

// MoveGain is movement regained per mud hour, porting move_gain
// (limits.c:178).
func MoveGain(rec *PlayerRecord, ctx Regenerator, now time.Time) int32 {
	var gain int32

	if ctx.IsNPC() {
		gain = rec.Level
	} else {
		gain = graf(Age(rec, now), 16, 20, 24, 20, 16, 12, 10)

		switch ctx.Position() {
		case PosSleeping:
			gain += gain / 2
		case PosResting:
			gain += gain / 4
		case PosSitting:
			gain += gain / 8
		}

		// No class adjustment here: everybody walks the same.
		if starving(rec) {
			gain /= 4
		}
	}

	return finishGain(gain, ctx)
}

// finishGain applies the two adjustments every one of the three shares, in
// the order the C applies them. Poison first, then the room — so a poisoned
// character in a good-regeneration room gets half of what they otherwise
// would, not a quarter.
func finishGain(gain int32, ctx Regenerator) int32 {
	if ctx.Poisoned() {
		gain /= 4
	}
	if ctx.GoodRegen() {
		// The C writes `gain += (gain * 1)`, which is doubling with the
		// working left in.
		gain += gain
	}
	return gain
}

// starving reports the condition that quarters every kind of regeneration:
// completely out of food or completely out of drink.
func starving(rec *PlayerRecord) bool {
	return rec.Conditions[CondFull] == 0 || rec.Conditions[CondThirst] == 0
}

// ConditionChange is what GainCondition did, so the caller can say something
// about it.
type ConditionChange struct {
	// Changed is false if the character is immune — a mobile, or an immortal
	// whose conditions are -1.
	Changed bool
	// Message is what the C sends when a condition reaches zero, or "".
	Message string
}

// GainCondition adjusts hunger, thirst or drunkenness, porting
// gain_condition (limits.c:380).
//
// CondNotApplicable is not "empty", it is "does not apply": immortals have
// all three set that way and never get hungry.
func GainCondition(rec *PlayerRecord, cond Condition, delta int32) ConditionChange {
	if rec.Conditions[cond] == CondNotApplicable {
		return ConditionChange{}
	}

	// Whether they were drunk is checked before the change, because "You are
	// now sober" should only be said to somebody who was not.
	intoxicated := rec.Conditions[CondDrunk] > 0

	value := rec.Conditions[cond] + delta
	value = max(0, value)
	value = min(MaxCondition, value)
	rec.Conditions[cond] = value

	// `if (GET_COND(ch, condition) || PLR_FLAGGED(ch, PLR_WRITING)) return;`
	// (limits.c:394). Note what PLR_WRITING suppresses: the *message*, not
	// the change — hunger and thirst go right on advancing while you are
	// in the editor, you just are not told about it until you come out.
	// The bit is real as of #214; before that this branch could not have
	// been written, since nothing set it.
	if value != 0 || rec.PlayerFlags.Has(PlayerWriting) {
		return ConditionChange{Changed: true}
	}

	switch cond {
	case CondFull:
		return ConditionChange{Changed: true, Message: "You are hungry.\r\n"}
	case CondThirst:
		return ConditionChange{Changed: true, Message: "You are thirsty.\r\n"}
	case CondDrunk:
		if intoxicated {
			return ConditionChange{Changed: true, Message: "You are now sober.\r\n"}
		}
	}
	return ConditionChange{Changed: true}
}
