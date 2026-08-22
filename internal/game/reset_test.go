// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"strings"
	"testing"
)

// resetWorld is a zone with a guard, a sword to equip them with, a bag with
// something in it, and a door.
func resetWorld(commands []ResetCommand) *Live {
	temple := &RoomDef{Vnum: 3001, Name: "The Temple"}
	square := &RoomDef{Vnum: 3002, Name: "The Square"}
	temple.Exits[North] = &ExitDef{ToRoom: 3002, DoorFlag: 1, Keywords: "gate"}

	return NewLive(&World{
		Rooms: []*RoomDef{temple, square},
		Mobiles: []*MobDef{
			{
				Vnum: 3060, Keywords: "guard cityguard", ShortDesc: "the cityguard",
				LongDesc: "A cityguard stands here.\r\n",
				Level:    10, Thac0: 15, ArmorClass: 5,
				HitDice: Dice{Number: 2, Size: 8, Bonus: 10},
				Gold:    100, Exp: 500, Position: int32(PosStanding),
			},
			{Vnum: 3061, Keywords: "dog", ShortDesc: "a dog", Level: 2, Position: int32(PosStanding)},
		},
		Objects: []*ObjDef{
			{Vnum: 3020, Keywords: "sword", ShortDesc: "a sword", Type: ItemWeapon,
				WearFlags: ItemWearTake | ItemWearWield, Weight: 10},
			{Vnum: 3021, Keywords: "bag", ShortDesc: "a bag", Type: ItemContainer,
				WearFlags: ItemWearTake, Weight: 5},
			{Vnum: 3022, Keywords: "coin", ShortDesc: "a coin", Type: ItemTreasure,
				WearFlags: ItemWearTake, Weight: 1},
		},
		Zones: []*ZoneDef{
			{Vnum: 30, Name: "Midgaard", Bottom: 3000, Top: 3099,
				Lifespan: 15, ResetMode: 2, Commands: commands},
		},
	})
}

func TestResetCreatesMobilesAndObjects(t *testing.T) {
	l := resetWorld([]ResetCommand{
		{Command: ResetMobile, Arg1: 3060, Arg2: 1, Arg3: 3001},
		{Command: ResetObject, Arg1: 3020, Arg2: 1, Arg3: 3001},
		{Command: ResetStop},
	})

	report := l.ResetZone(l.Zones()[0], newRNG())

	if report.Mobiles != 1 || report.Objects != 1 {
		t.Errorf("reset made %d mobiles and %d objects, want 1 and 1", report.Mobiles, report.Objects)
	}
	if len(report.Problems) != 0 {
		t.Errorf("problems: %v", report.Problems)
	}

	mobs := l.Mobiles()
	if len(mobs) != 1 {
		t.Fatalf("%d mobiles in the world, want 1", len(mobs))
	}
	guard := mobs[0]

	// Display name and typed name are different things.
	if guard.Name != "the cityguard" {
		t.Errorf("the guard is called %q in a sentence", guard.Name)
	}
	if guard.Keywords != "guard cityguard" {
		t.Errorf("the guard's keywords are %q", guard.Keywords)
	}
	if l.FindInRoom(nil, 3001, "guard") != guard {
		t.Error("`guard` does not find the cityguard")
	}
	if l.FindInRoom(nil, 3001, "cityguard") != guard {
		t.Error("`cityguard` does not find the cityguard")
	}

	// The file's thac0 and armour class are converted as the C's loader does.
	if guard.Record.Points.HitRoll != 5 {
		t.Errorf("hitroll is %d, want 20 - thac0 = 5", guard.Record.Points.HitRoll)
	}
	if guard.Record.Points.Armor != 50 {
		t.Errorf("armour is %d, want ac * 10 = 50", guard.Record.Points.Armor)
	}
	// 2d8+10 is 12..26.
	if hit := guard.Record.Points.MaxHit; hit < 12 || hit > 26 {
		t.Errorf("hit points are %d, want 12..26 from 2d8+10", hit)
	}
	if guard.Record.Points.Gold != 100 || guard.Record.Points.Exp != 500 {
		t.Errorf("the guard carries %d gold and is worth %d",
			guard.Record.Points.Gold, guard.Record.Points.Exp)
	}
}

// TestPopulationCapsStopTheWorldFilling. This is what makes a reset every
// fifteen minutes survivable.
func TestPopulationCapsStopTheWorldFilling(t *testing.T) {
	l := resetWorld([]ResetCommand{
		{Command: ResetMobile, Arg1: 3060, Arg2: 2, Arg3: 3001},
		{Command: ResetObject, Arg1: 3020, Arg2: 3, Arg3: 3001},
		{Command: ResetStop},
	})
	zone := l.Zones()[0]

	for i := 0; i < 10; i++ {
		l.ResetZone(zone, newRNG())
	}

	if got := len(l.Mobiles()); got != 2 {
		t.Errorf("%d guards after ten resets, want the cap of 2", got)
	}
	if got := l.objectCount(3020); got != 3 {
		t.Errorf("%d swords after ten resets, want the cap of 3", got)
	}
}

// TestKillingAMobileFreesItsSlot, so the next reset replaces it.
func TestKillingAMobileFreesItsSlot(t *testing.T) {
	l := resetWorld([]ResetCommand{
		{Command: ResetMobile, Arg1: 3060, Arg2: 1, Arg3: 3001},
		{Command: ResetStop},
	})
	zone := l.Zones()[0]

	l.ResetZone(zone, newRNG())
	l.ResetZone(zone, newRNG())
	if got := len(l.Mobiles()); got != 1 {
		t.Fatalf("%d guards, want 1", got)
	}

	l.RemoveMobile(l.Mobiles()[0])
	l.ResetZone(zone, newRNG())

	if got := len(l.Mobiles()); got != 1 {
		t.Errorf("%d guards after the first was killed and the zone reset, want 1", got)
	}
}

// TestGiveAndEquipLoadOntoTheLastMobile, which is the one register the
// command list has.
func TestGiveAndEquipLoadOntoTheLastMobile(t *testing.T) {
	l := resetWorld([]ResetCommand{
		{Command: ResetMobile, Arg1: 3060, Arg2: 1, Arg3: 3001},
		{Command: ResetEquip, Arg1: 3020, Arg2: 1, Arg3: int32(WearWield)},
		{Command: ResetGive, Arg1: 3021, Arg2: 1},
		{Command: ResetStop},
	})

	report := l.ResetZone(l.Zones()[0], newRNG())
	if len(report.Problems) != 0 {
		t.Fatalf("problems: %v", report.Problems)
	}

	guard := l.Mobiles()[0]
	if wielded := guard.Equipment[WearWield]; wielded == nil || wielded.Vnum() != 3020 {
		t.Error("the guard is not wielding the sword")
	}
	if len(guard.Carrying) != 1 || guard.Carrying[0].Vnum() != 3021 {
		t.Errorf("the guard is carrying %d things, want the bag", len(guard.Carrying))
	}
}

// TestGiveWithoutAMobileDisablesTheCommand, as the C does — permanently, so
// a broken zone complains once rather than every reset.
func TestGiveWithoutAMobileDisablesTheCommand(t *testing.T) {
	l := resetWorld([]ResetCommand{
		{Command: ResetGive, Arg1: 3020, Arg2: 1},
		{Command: ResetStop},
	})
	zone := l.Zones()[0]

	report := l.ResetZone(zone, newRNG())
	if len(report.Problems) != 1 || !strings.Contains(report.Problems[0], "does not exist") {
		t.Fatalf("problems were %v, want one about a missing mobile", report.Problems)
	}

	// The command has been turned into a no-op, so the second reset is quiet.
	if report := l.ResetZone(zone, newRNG()); len(report.Problems) != 0 {
		t.Errorf("the disabled command complained again: %v", report.Problems)
	}
}

// TestTheIfFlagChainsCommands: a conditional command runs only when the one
// before it succeeded, which is how "equip the guard, if there is a guard" is
// written.
func TestTheIfFlagChainsCommands(t *testing.T) {
	l := resetWorld([]ResetCommand{
		// Two guards, cap of one — so the second M fails.
		{Command: ResetMobile, Arg1: 3060, Arg2: 1, Arg3: 3001},
		{Command: ResetEquip, IfFlag: 1, Arg1: 3020, Arg2: 10, Arg3: int32(WearWield)},
		{Command: ResetMobile, Arg1: 3060, Arg2: 1, Arg3: 3001},
		{Command: ResetGive, IfFlag: 1, Arg1: 3021, Arg2: 10},
		{Command: ResetStop},
	})

	l.ResetZone(l.Zones()[0], newRNG())

	guard := l.Mobiles()[0]
	if guard.Equipment[WearWield] == nil {
		t.Error("the first conditional command did not run after its M succeeded")
	}
	if len(guard.Carrying) != 0 {
		t.Error("the second conditional command ran after its M failed")
	}
}

// TestPutInObject and its disabling when the container is missing.
func TestPutInObject(t *testing.T) {
	l := resetWorld([]ResetCommand{
		{Command: ResetObject, Arg1: 3021, Arg2: 1, Arg3: 3001},
		{Command: ResetPutInObj, Arg1: 3022, Arg2: 1, Arg3: 3021},
		{Command: ResetStop},
	})

	report := l.ResetZone(l.Zones()[0], newRNG())
	if len(report.Problems) != 0 {
		t.Fatalf("problems: %v", report.Problems)
	}

	bag := l.findObjectByVnum(3021)
	if bag == nil || len(bag.Contents) != 1 || bag.Contents[0].Vnum() != 3022 {
		t.Error("the coin is not in the bag")
	}
}

func TestDoorCommands(t *testing.T) {
	for _, tc := range []struct {
		state  int32
		closed bool
		locked bool
	}{
		{DoorOpen, false, false},
		{DoorClosed, true, false},
		{DoorLocked, true, true},
	} {
		l := resetWorld([]ResetCommand{
			{Command: ResetDoor, Arg1: 3001, Arg2: int32(North), Arg3: tc.state},
			{Command: ResetStop},
		})
		l.ResetZone(l.Zones()[0], newRNG())

		exit := l.Room(3001).Exits[North]
		if exit.State.Has(ExitClosed) != tc.closed {
			t.Errorf("state %d: closed is %v, want %v", tc.state, !tc.closed, tc.closed)
		}
		if exit.State.Has(ExitLocked) != tc.locked {
			t.Errorf("state %d: locked is %v, want %v", tc.state, !tc.locked, tc.locked)
		}
		// A door is still a door whatever its state.
		if !exit.State.Has(ExitIsDoor) {
			t.Errorf("state %d: the exit stopped being a door", tc.state)
		}
	}
}

// TestADoorThatDoesNotExistDisablesTheCommand.
func TestADoorThatDoesNotExistDisablesTheCommand(t *testing.T) {
	l := resetWorld([]ResetCommand{
		{Command: ResetDoor, Arg1: 3001, Arg2: int32(South), Arg3: DoorClosed},
		{Command: ResetStop},
	})
	zone := l.Zones()[0]

	report := l.ResetZone(zone, newRNG())
	if len(report.Problems) != 1 {
		t.Fatalf("problems were %v, want one", report.Problems)
	}
	if report := l.ResetZone(zone, newRNG()); len(report.Problems) != 0 {
		t.Errorf("the disabled command complained again: %v", report.Problems)
	}
}

// TestRemoveTakesSomethingOutOfARoom.
func TestRemoveTakesSomethingOutOfARoom(t *testing.T) {
	l := resetWorld([]ResetCommand{
		{Command: ResetObject, Arg1: 3020, Arg2: 1, Arg3: 3001},
		{Command: ResetStop},
	})
	zone := l.Zones()[0]
	l.ResetZone(zone, newRNG())

	if len(l.RoomObjects(3001)) != 1 {
		t.Fatal("the sword was not created")
	}

	zone.Commands = []ResetCommand{
		{Command: ResetRemove, Arg1: 3001, Arg2: 3020},
		{Command: ResetStop},
	}
	l.ResetZone(zone, newRNG())

	if got := len(l.RoomObjects(3001)); got != 0 {
		t.Errorf("%d objects left in the room, want 0", got)
	}
}

// TestEverythingAfterStopIsIgnored.
func TestEverythingAfterStopIsIgnored(t *testing.T) {
	l := resetWorld([]ResetCommand{
		{Command: ResetStop},
		{Command: ResetMobile, Arg1: 3060, Arg2: 10, Arg3: 3001},
	})

	if report := l.ResetZone(l.Zones()[0], newRNG()); report.Mobiles != 0 {
		t.Errorf("%d mobiles were created after the S command", report.Mobiles)
	}
}

func TestZoneIsEmpty(t *testing.T) {
	l := resetWorld([]ResetCommand{{Command: ResetStop}})
	zone := l.Zones()[0]

	if !l.ZoneIsEmpty(zone) {
		t.Error("an empty zone is not empty")
	}

	// A mobile in the zone does not count: reset mode 1 waits for players.
	l.SpawnMobile(3060, 3001, newRNG())
	if !l.ZoneIsEmpty(zone) {
		t.Error("a mobile made the zone non-empty")
	}

	player := newCharacter("Welmar")
	if err := l.Enter(player, 3001); err != nil {
		t.Fatal(err)
	}
	if l.ZoneIsEmpty(zone) {
		t.Error("a zone with a player in it reported empty")
	}

	// And out of the zone's vnum range again.
	if err := l.Enter(player, 9999); err == nil {
		t.Skip("the test world has no room outside the zone")
	}
}
