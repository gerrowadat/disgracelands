// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// The MOB_* bits, from structs.h:199. Listed in full for the reason the
// player flags are: a partial list invites inventing a value for the next one
// needed, and an invented one is a different mobile.
const (
	// MobSpec: the mobile has a special procedure. The flag is in the mob
	// file and the *function* comes from spec_assign.c's table — a mobile
	// needs both, and one without the other is a SYSERR in the C.
	//
	// Old comment, kept because it dates the port: "Those arrive with the
	// scripting seam (plan §8); the flag is read so a mobile carrying it can
	// be told apart from one that is not.
	MobSpec Flags = 1 << 0
	// MobSentinel: stays put.
	MobSentinel Flags = 1 << 1
	// MobScavenger: picks up the most valuable thing on the floor.
	MobScavenger Flags = 1 << 2
	// Bit 3 is MOB_ISNPC, which the world loader force-sets on every mobile.
	// It is declared in world.go, beside the loader that sets it.
	// MobAware: cannot be backstabbed.
	MobAware Flags = 1 << 4
	// MobAggressive: attacks anybody.
	MobAggressive Flags = 1 << 5
	// MobStayZone: will not wander out of its own zone.
	MobStayZone Flags = 1 << 6
	// MobWimpy: flees when badly hurt, and only attacks the sleeping.
	MobWimpy Flags = 1 << 7
	// The three alignment-selective aggressions.
	MobAggrEvil    Flags = 1 << 8
	MobAggrGood    Flags = 1 << 9
	MobAggrNeutral Flags = 1 << 10
	// MobMemory: remembers who attacked it.
	MobMemory Flags = 1 << 11
	// MobHelper: joins in when another mobile is fighting a player.
	MobHelper   Flags = 1 << 12
	MobNoCharm  Flags = 1 << 13
	MobNoSummon Flags = 1 << 14
	MobNoSleep  Flags = 1 << 15
	MobNoBash   Flags = 1 << 16
	MobNoBlind  Flags = 1 << 17
	// MobNotDeadYet marks a mobile being extracted.
	MobNotDeadYet Flags = 1 << 18
)

// MobAggrToAlign is the C's MOB_AGGR_TO_ALIGN: any of the three selective
// aggressions.
const MobAggrToAlign = MobAggrEvil | MobAggrNeutral | MobAggrGood

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
func (c *Character) MobFlags() Flags {
	if c == nil || c.MobDef == nil {
		return 0
	}
	return c.MobDef.ActionFlags
}

// HasMobFlag reports whether a mobile carries a flag.
func (c *Character) HasMobFlag(f Flags) bool { return c.MobFlags().HasAny(f) }
