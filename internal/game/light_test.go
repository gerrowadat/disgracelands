// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"testing"
	"time"
)

// darkWorld builds two rooms: one indoors, one out in a field, and a torch.
func darkWorld(t *testing.T) *Live {
	t.Helper()
	inside := &RoomDef{Vnum: 3001, Name: "The Temple", SectorType: SectorInside}
	field := &RoomDef{Vnum: 3002, Name: "A Field", SectorType: 2}
	cellar := &RoomDef{Vnum: 3003, Name: "A Cellar", SectorType: SectorInside, Flags: NewSet(RoomDark)}

	return NewLive(&World{
		Rooms: []*RoomDef{inside, field, cellar},
		Objects: []*ObjDef{
			// Value 2 is hours of fuel. 24 is an ordinary torch; -1 is an
			// eternal one; 0 is burnt out.
			{Vnum: 3040, Keywords: "torch", ShortDesc: "a torch", Type: ItemLight,
				WearFlags: ItemWearTake, Values: [NumObjValues]int32{0, 0, 24}},
			{Vnum: 3041, Keywords: "lamp", ShortDesc: "an eternal lamp", Type: ItemLight,
				WearFlags: ItemWearTake, Values: [NumObjValues]int32{0, 0, -1}},
			{Vnum: 3042, Keywords: "stub", ShortDesc: "a burnt-out stub", Type: ItemLight,
				WearFlags: ItemWearTake, Values: [NumObjValues]int32{0, 0, 0}},
			{Vnum: 3043, Keywords: "sword", ShortDesc: "a sword", Type: ItemWeapon,
				WearFlags: ItemWearTake},
		},
		Zones: []*ZoneDef{{Vnum: 30, Name: "Midgaard", Bottom: 3000, Top: 3099}},
	})
}

// inRoom makes a character and puts them in a room.
func inRoom(t *testing.T, l *Live, name string, room RoomVnum) *Character {
	t.Helper()
	ch := newCharacter(name)
	if err := l.Enter(ch, room); err != nil {
		t.Fatalf("putting %s in room %d: %v", name, room, err)
	}
	return ch
}

// atHour winds the world's clock so that MudTime().Hours is the hour given.
// The clock is derived from time since boot, so moving the boot backwards is
// how a test picks a time of day.
func atHour(l *Live, hour int32) {
	l.booted = time.Now().Add(-time.Duration(hour) * SecondsPerMudHour * time.Second)
}

func TestLitLight(t *testing.T) {
	l := darkWorld(t)
	for _, tc := range []struct {
		vnum ObjVnum
		name string
		want bool
	}{
		{3040, "a torch with fuel", true},
		// -1 is the C's eternal light: GET_OBJ_VAL(obj, 2) is tested for being
		// non-zero, and the burnout timer is guarded on > 0.
		{3041, "an eternal lamp", true},
		{3042, "a burnt-out stub", false},
		{3043, "a sword", false},
	} {
		if got := LitLight(l.NewObject(tc.vnum)); got != tc.want {
			t.Errorf("LitLight(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
	if LitLight(nil) {
		t.Error("LitLight(nil) is true")
	}
}

// TestAnIndoorRoomIsNeverDark. SECT_INSIDE and SECT_CITY are lit at every hour
// — which is why Midgaard is walkable at three in the morning.
func TestAnIndoorRoomIsNeverDark(t *testing.T) {
	l := darkWorld(t)
	for _, hour := range []int32{0, 3, 12, 20, 23} {
		atHour(l, hour)
		if l.RoomIsDark(3001) {
			t.Errorf("the indoor room was dark at hour %d", hour)
		}
	}
}

// TestAnOutdoorRoomFollowsTheSun, including the boundaries that are easy to
// get wrong: SUN_SET counts as dark, the light comes back at SUN_RISE rather
// than at SUN_LIGHT, and **sunset is at twenty-one, not twenty**.
//
// That last one was wrong here, and it was an hour of darkness across every
// outdoor room in the world. Two functions in the C set the sunlight and they
// agree: another_hour's switch (weather.c:41) fires at 5, 6, 21 and 22, and
// reset_time's chain (db.c) reads `< 5` dark, `== 5` rise, `<= 20` light,
// `== 21` set, else dark.
func TestAnOutdoorRoomFollowsTheSun(t *testing.T) {
	for _, tc := range []struct {
		hour int32
		dark bool
	}{
		{4, true},   // SUN_DARK
		{5, false},  // SUN_RISE — already light enough
		{6, false},  // SUN_LIGHT
		{12, false}, // SUN_LIGHT
		{19, false}, // still SUN_LIGHT
		{20, false}, // SUN_LIGHT — the C's last lit hour
		{21, true},  // SUN_SET — dusk is dark
		{22, true},  // SUN_DARK
		{23, true},  // SUN_DARK
	} {
		l := darkWorld(t)
		atHour(l, tc.hour)
		if got := l.RoomIsDark(3002); got != tc.dark {
			t.Errorf("at hour %d the field was dark=%v, want %v", tc.hour, got, tc.dark)
		}
	}
}

// TestADarkFlagBeatsBeingIndoors. The order of the tests in room_is_dark is
// the whole of its behaviour: ROOM_DARK is asked before the sector is.
func TestADarkFlagBeatsBeingIndoors(t *testing.T) {
	l := darkWorld(t)
	atHour(l, 12)
	if !l.RoomIsDark(3003) {
		t.Error("a DARK cellar was lit at noon")
	}
}

// TestALightBeatsTheDarkFlag, which is the other half of that order: the light
// count is asked before ROOM_DARK, so a torch lights a cellar.
func TestALightBeatsTheDarkFlag(t *testing.T) {
	l := darkWorld(t)
	atHour(l, 12)

	ch := inRoom(t, l, "Zod", 3003)
	ch.Equipment[WearLight] = l.NewObject(3040)

	if l.RoomIsDark(3003) {
		t.Error("a cellar was dark with a lit torch in it")
	}
	if got := l.LightsIn(3003); got != 1 {
		t.Errorf("LightsIn = %d, want 1", got)
	}
}

// TestOnlyAWornLightCounts is the finding this whole file exists to pin down.
//
// `world[room].light` is adjusted in five places, all of them char_to_room,
// char_from_room, equip_char, unequip_char or the burnout timer.
// `obj_to_room` does not touch it — so a lit torch lying on the floor lights
// nothing at all, and putting your torch down plunges the room into darkness
// while it goes on burning at your feet.
func TestOnlyAWornLightCounts(t *testing.T) {
	l := darkWorld(t)
	atHour(l, 12)

	ch := inRoom(t, l, "Zod", 3003)
	torch := l.NewObject(3040)

	// In the light slot: the cellar is lit.
	ch.Equipment[WearLight] = torch
	if l.RoomIsDark(3003) {
		t.Fatal("a worn torch did not light the cellar")
	}

	// Carried rather than worn: dark. The C counts the slot, not the object.
	ch.Equipment[WearLight] = nil
	ch.Carrying = append(ch.Carrying, torch)
	if !l.RoomIsDark(3003) {
		t.Error("a torch in the pack lit the room; only the light slot counts")
	}

	// On the floor, still burning: dark.
	ch.Carrying = nil
	l.ObjectToRoom(torch, 3003)
	if !l.RoomIsDark(3003) {
		t.Error("a torch on the floor lit the room; obj_to_room does not touch the count")
	}
}

// TestABurntOutLightDoesNotCount.
func TestABurntOutLightDoesNotCount(t *testing.T) {
	l := darkWorld(t)
	atHour(l, 12)

	ch := inRoom(t, l, "Zod", 3003)
	ch.Equipment[WearLight] = l.NewObject(3042)

	if !l.RoomIsDark(3003) {
		t.Error("a burnt-out stub lit the cellar")
	}
}

// TestLightsAreCountedPerRoom, so somebody else's torch two rooms away is no
// help.
func TestLightsAreCountedPerRoom(t *testing.T) {
	l := darkWorld(t)
	atHour(l, 12)

	other := inRoom(t, l, "Bystander", 3001)
	other.Equipment[WearLight] = l.NewObject(3040)

	if !l.RoomIsDark(3003) {
		t.Error("a torch in another room lit the cellar")
	}
}

// TestRoomIsDarkOnAMissingRoom answers "not dark", which is what the C does
// after logging a SYSERR. Worth asserting because the lenient answer is the
// one that keeps a bug visible rather than blacking out the game.
func TestRoomIsDarkOnAMissingRoom(t *testing.T) {
	l := darkWorld(t)
	if l.RoomIsDark(9999) {
		t.Error("a room that does not exist reported dark")
	}
}

// TestCanSeeInDark covers the two ways through it and the one that is not
// there: holylight counts, infravision counts, and an ordinary mortal in an
// unlit room sees nothing.
func TestCanSeeInDark(t *testing.T) {
	l := darkWorld(t)

	mortal := inRoom(t, l, "Zod", 3003)
	if CanSeeInDark(mortal) {
		t.Error("an ordinary mortal could see in the dark")
	}

	mortal.Record.AffectFlags = mortal.Record.AffectFlags.Set(AffectInfravision)
	if !CanSeeInDark(mortal) {
		t.Error("infravision did not let them see in the dark")
	}

	god := inRoom(t, l, "Odin", 3003)
	god.Record.Preferences = god.Record.Preferences.Set(PrefHolylight)
	if !CanSeeInDark(god) {
		t.Error("holylight did not let a god see in the dark")
	}

	if CanSeeInDark(nil) {
		t.Error("CanSeeInDark(nil) is true")
	}
}
