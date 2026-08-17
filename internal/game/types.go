// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

// Package game holds the world model: rooms, mobiles, objects, zones and the
// types that describe them. It has no I/O and knows nothing about file
// formats — those live in internal/persist, which converts to and from these
// types.
//
// Every field that is ever serialized has an explicit width. The C code this
// is ported from uses `int`, `long`, `sh_int` and `byte` interchangeably, and
// the widths of those changed underneath it when the world moved from 32-bit
// to 64-bit; see docs/proposals/go-port-plan.md §4. `int` appears here only
// for values that exist purely in memory.
package game

// Virtual numbers are the identifiers builders use and the world files
// contain. They are stable across reboots and are what a `goto 3001` refers
// to.
//
// Real numbers are array indices into the loaded world, assigned at load time
// in file order. They are not stable and must never be written to a file.
//
// The C code keeps both as bare ints and relies on naming discipline
// (room_vnum vs room_rnum) to tell them apart. These are distinct types so
// the compiler enforces what the C code only documents.
type (
	RoomVnum int32
	MobVnum  int32
	ObjVnum  int32
	ZoneVnum int32

	RoomRnum int32
	MobRnum  int32
	ObjRnum  int32
	ZoneRnum int32
)

// NoRoom is the "nowhere" sentinel, matching the C NOWHERE. Exits pointing at
// it are unlinked.
const NoRoom RoomVnum = -1

// NoObject is the same sentinel for an object that has no prototype — a
// corpse, or a pile of coins built at runtime.
const NoObject ObjVnum = -1

// NothingRnum marks an unresolved real number, matching the C NOTHING.
const NothingRnum RoomRnum = -1

// Direction indexes a room's exits. The order is the C NUM_OF_DIRS order and
// is load-bearing: it is the order exits appear in the world files, as `D0`
// through `D5`.
type Direction int8

// The six directions, in file order.
const (
	North Direction = iota
	East
	South
	West
	Up
	Down
	NumDirections = 6
)

// String returns the direction's lowercase name.
func (d Direction) String() string {
	if d < 0 || int(d) >= NumDirections {
		return "?"
	}
	return [...]string{"north", "east", "south", "west", "up", "down"}[d]
}

// Valid reports whether d is a real direction.
func (d Direction) Valid() bool { return d >= 0 && int(d) < NumDirections }

// DirectionFromInt converts a scanned number to a Direction, reporting
// whether it is in range. Direction is an int8, so converting an arbitrary
// scanned value directly would wrap: a "D256" in a world file would become
// north rather than an error. The C loader does exactly that conversion
// unchecked and then indexes an array with it.
func DirectionFromInt(v int32) (Direction, bool) {
	if v < 0 || v >= NumDirections {
		return 0, false
	}
	return Direction(v), true
}

// Dice is the CircleMUD `NdS+B` damage/hitpoint notation, as it appears in
// mob files ("5d10+550").
type Dice struct {
	Number int32
	Size   int32
	Bonus  int32
}

// ExtraDesc is a keyword-triggered description, attached to rooms and objects
// and shown when a player looks at something that is not an exit or an item.
type ExtraDesc struct {
	Keywords    string
	Description string
}
