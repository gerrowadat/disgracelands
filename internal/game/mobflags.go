// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// The MOB_* bits, from structs.h:199. Listed in full for the reason the
// player flags are: a partial list invites inventing a value for the next one
// needed, and an invented one is a different mobile.
//
// MobFlag is one of them and MobFlags is a mobile's set. Bit indices, not
// masks: docs/design/idiomatic-go.md §4.1, §4.1.1 and §4.1.2 for the
// three ways that bites. action_bits[] in constants.c is the name table.
type MobFlag int

// MobFlags is a set of MobFlag.
type MobFlags = Set[MobFlag]

const (
	// MobSpec: the mobile has a special procedure. The flag is in the mob
	// file and the *function* comes from spec_assign.c's table — a mobile
	// needs both, and one without the other is a SYSERR in the C.
	//
	// Old comment, kept because it dates the port: "Those arrive with the
	// scripting seam (plan §8); the flag is read so a mobile carrying it can
	// be told apart from one that is not.
	MobSpec MobFlag = 0
	// MobSentinel: stays put.
	MobSentinel MobFlag = 1
	// MobScavenger: picks up the most valuable thing on the floor.
	MobScavenger MobFlag = 2
	// Bit 3 is MOB_ISNPC, which the world loader force-sets on every mobile.
	// It is declared in world.go, beside the loader that sets it.
	// MobAware: cannot be backstabbed.
	MobAware MobFlag = 4
	// MobAggressive: attacks anybody.
	MobAggressive MobFlag = 5
	// MobStayZone: will not wander out of its own zone.
	MobStayZone MobFlag = 6
	// MobWimpy: flees when badly hurt, and only attacks the sleeping.
	MobWimpy MobFlag = 7
	// The three alignment-selective aggressions.
	MobAggrEvil    MobFlag = 8
	MobAggrGood    MobFlag = 9
	MobAggrNeutral MobFlag = 10
	// MobMemory: remembers who attacked it.
	MobMemory MobFlag = 11
	// MobHelper: joins in when another mobile is fighting a player.
	MobHelper   MobFlag = 12
	MobNoCharm  MobFlag = 13
	MobNoSummon MobFlag = 14
	MobNoSleep  MobFlag = 15
	MobNoBash   MobFlag = 16
	MobNoBlind  MobFlag = 17
	// MobNotDeadYet marks a mobile being extracted.
	MobNotDeadYet MobFlag = 18
)

// MobAggrToAlign is the C's MOB_AGGR_TO_ALIGN: any of the three selective
// aggressions. A var rather than a const because a set is a struct — the
// value is still fixed, and nothing writes to it.
var MobAggrToAlign = NewSet(MobAggrEvil, MobAggrNeutral, MobAggrGood)

// Alignment thresholds, from utils.h:351.
const (
	AlignmentGood int32 = 350
	AlignmentEvil int32 = -350
)

// IsGood, IsEvil and IsNeutral port the alignment macros. Neutral is defined
// as neither of the other two rather than as a range, so the boundaries
// cannot disagree.
func IsGood(rec *PlayerRecord) bool { return rec != nil && rec.Alignment >= AlignmentGood }

// IsEvil reports whether a character's alignment is evil.
func IsEvil(rec *PlayerRecord) bool { return rec != nil && rec.Alignment <= AlignmentEvil }

// IsNeutral reports whether a character is neither good nor evil.
func IsNeutral(rec *PlayerRecord) bool { return !IsGood(rec) && !IsEvil(rec) }

// MobFlags returns a mobile's action flags, or none for a player.
func (c *Character) MobFlags() MobFlags {
	if c == nil || c.MobDef == nil {
		return MobFlags{}
	}
	return c.MobDef.ActionFlags
}

// HasMobFlag reports whether a mobile carries any of the given flags.
func (c *Character) HasMobFlag(f ...MobFlag) bool { return c.MobFlags().HasAny(f...) }

// HasAnyMobFlag is HasMobFlag against a set rather than a list, for the
// handful of callers holding a computed mask such as MobAggrToAlign.
func (c *Character) HasAnyMobFlag(f MobFlags) bool { return c.MobFlags().Overlaps(f) }
