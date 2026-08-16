// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// Position is what a character is doing with themselves, from structs.h:164.
//
// The numbers are ordered and the order is load-bearing: the C compares
// positions with < and > all over the place — `GET_POS(ch) > POS_STUNNED` is
// "awake", `>= POS_STUNNED` is "not dying" — so these are a scale, not a set
// of labels.
type Position int32

// The positions, in the C's order.
const (
	PosDead Position = iota
	PosMortallyWounded
	PosIncapacitated
	PosStunned
	PosSleeping
	PosResting
	PosSitting
	PosFighting
	PosStanding
)

// String names the position, in the words the C uses for it.
func (p Position) String() string {
	switch p {
	case PosDead:
		return "dead"
	case PosMortallyWounded:
		return "mortally wounded"
	case PosIncapacitated:
		return "incapacitated"
	case PosStunned:
		return "stunned"
	case PosSleeping:
		return "sleeping"
	case PosResting:
		return "resting"
	case PosSitting:
		return "sitting"
	case PosFighting:
		return "fighting"
	case PosStanding:
		return "standing"
	}
	return "in an unknown position"
}

// Awake ports AWAKE(ch) (utils.h:347).
func (p Position) Awake() bool { return p > PosSleeping }

// Conscious reports whether the character is above the dying positions, which
// is the test point_update makes before regenerating anything.
func (p Position) Conscious() bool { return p >= PosStunned }

// Dying reports whether they are bleeding out — losing hit points every tick
// with nothing they can do about it.
func (p Position) Dying() bool { return p == PosIncapacitated || p == PosMortallyWounded }

// Hit-point thresholds from update_pos (fight.c). A character does not simply
// die at zero: there are eleven points of dying below it, which is what makes
// rescuing somebody possible.
const (
	// HitPointsDead is the level at or below which a character is dead.
	HitPointsDead int32 = -11
	// HitPointsMortallyWounded: losing two hit points a tick.
	HitPointsMortallyWounded int32 = -6
	// HitPointsIncapacitated: losing one a tick.
	HitPointsIncapacitated int32 = -3
)

// UpdatePosition ports update_pos (fight.c).
//
// Note the first clause: a character who is already below stunned and has
// positive hit points is stood up, but one who is awake and merely hurt keeps
// whatever position they were in. That is what stops this function from
// yanking a resting character to their feet every tick.
func UpdatePosition(rec *PlayerRecord, pos Position) Position {
	hit := rec.Points.Hit

	switch {
	case hit > 0 && pos > PosStunned:
		return pos
	case hit > 0:
		return PosStanding
	case hit <= HitPointsDead:
		return PosDead
	case hit <= HitPointsMortallyWounded:
		return PosMortallyWounded
	case hit <= HitPointsIncapacitated:
		return PosIncapacitated
	}
	return PosStunned
}
