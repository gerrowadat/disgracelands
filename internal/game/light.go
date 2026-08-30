// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// Light, and whether a room is dark enough to stop you seeing.

// Sector is a room's terrain, from structs.h:106. The numbers are the world
// file's, and are the indices of sector_types[] in constants.c.
//
// All ten are named now. Three were, before this became a type — the two
// room_is_dark tests and the one do_simple_move demands a boat for — with
// the rest left as bare numbers because nothing asked about them by name.
// A partial enumeration is exactly what
// docs/design/idiomatic-go.md §3.2 objects to: it invites somebody to
// invent the next value rather than look it up, and a named constant costs
// nothing. sectorNames and yamlSectorNames are both ten entries long and
// bitnames_test.go checks the first against constants.c, so the order here
// is derived rather than asserted.
//
// The zero value is SectorInside, a real sector — as Class's is a real
// class, and unlike ItemType's.
type Sector int

// Number is the sector's stored number, for the world files and the
// value-indexed name tables.
func (s Sector) Number() int32 { return int32(s) } //nolint:gosec // ten sectors; the format's width

// Sector types, from structs.h:106, in sector_types[] order.
const (
	SectorInside      Sector = 0
	SectorCity        Sector = 1
	SectorField       Sector = 2
	SectorForest      Sector = 3
	SectorHills       Sector = 4
	SectorMountains   Sector = 5
	SectorWaterSwim   Sector = 6
	SectorWaterNoSwim Sector = 7
	SectorFlying      Sector = 8
	SectorUnderwater  Sector = 9
)

// LitLight reports whether an object is a light source that is currently
// burning, which is the C's `GET_OBJ_TYPE(obj) == ITEM_LIGHT &&
// GET_OBJ_VAL(obj, 2)`.
//
// Value 2 is hours remaining, and the C tests it for being *non-zero* rather
// than positive — so **-1 is an eternal light**, not a burnt-out one, and the
// burnout timer skips it because that is guarded on `> 0`
// (handler.c:823). Both readings are load-bearing and neither is obvious.
func LitLight(o *Object) bool {
	light, ok := o.LightValues()
	return ok && light.Hours != 0
}

// LightsIn counts the light sources in a room.
//
// This is `world[room].light`, and the surprise is what it counts: only lights
// **worn in the light slot by characters who are in the room**. The counter is
// touched in exactly five places (handler.c:381, :403, :539, :573, :832) and
// every one of them is char_to_room, char_from_room, equip_char, unequip_char
// or the burnout timer. `obj_to_room` and `obj_from_room` do not touch it at
// all — so a lit torch dropped on the floor lights nothing, and the room goes
// dark the moment you put it down. See docs/weirdnumbers.md.
//
// The C keeps a counter and adjusts it; this counts. See docs/deviations.md —
// the outcome is the same because the C's five adjustments do balance, and a
// count cannot drift the way a counter can.
func (l *Live) LightsIn(room RoomVnum) int {
	n := 0
	for _, ch := range l.Occupants(room) {
		if LitLight(ch.Equipment[WearLight]) {
			n++
		}
	}
	return n
}

// RoomIsDark ports room_is_dark (utils.c:633).
//
// The order of the tests is the C's and matters: a light source beats the DARK
// flag, and the DARK flag beats being indoors. So a room flagged DARK is lit
// while somebody stands in it holding a torch, and a room that is both DARK
// and SECT_INSIDE is dark.
func (l *Live) RoomIsDark(vnum RoomVnum) bool {
	room := l.Room(vnum)
	if room == nil {
		// The C logs "room_is_dark: Invalid room rnum" and returns FALSE.
		// Answering "not dark" for a room that does not exist is the lenient
		// way round and the one the C picked.
		return false
	}

	if l.LightsIn(vnum) > 0 {
		return false
	}
	if room.Flags.Has(RoomDark) {
		return true
	}
	if room.SectorType == SectorInside || room.SectorType == SectorCity {
		return false
	}

	// Outdoors, so it depends on the time of day. Note that SUN_SET counts as
	// dark: dusk is already too dark to read by, and the light comes back at
	// SUN_RISE rather than at SUN_LIGHT.
	switch SunlightAt(l.MudTime().Hours) {
	case SunSet, SunDark:
		return true
	}
	return false
}

// CanSeeInDark ports CAN_SEE_IN_DARK (utils.h:348).
//
// Note that this is *not* what LIGHT_OK asks — that one takes infravision
// alone and lets holylight in by a separate route entirely (utils.h:426 and
// :435). This macro is what `look` uses to decide whether the room is
// describable, and there holylight counts directly.
func CanSeeInDark(ch *Character) bool {
	if ch == nil {
		return false
	}
	if ch.Record == nil {
		return false
	}
	if ch.Record.AffectFlags.Has(AffectInfravision) {
		return true
	}
	return !ch.IsNPC() && ch.Record.Preferences.Has(PrefHolylight)
}
