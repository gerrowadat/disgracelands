// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// Light, and whether a room is dark enough to stop you seeing.

// Sector types, from structs.h:106. Only the ones the rules ask about by name
// are here — the two room_is_dark tests, and the one do_simple_move demands a
// boat for; the rest are indices into sectorNames and stay numbers.
const (
	SectorInside      int32 = 0
	SectorCity        int32 = 1
	SectorWaterNoSwim int32 = 7
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
	return o != nil && o.Type == ItemLight && o.Values[2] != 0
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
