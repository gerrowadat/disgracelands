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

// freshGuardDef is 3060's own definition (resetWorld) with everything
// changed a builder might change: description, level/stats, and the
// action flags a live AI check reads continuously (mobflags.go).
func freshGuardDef() *MobDef {
	return &MobDef{
		Vnum: 3060, Keywords: "guard cityguard", ShortDesc: "the reloaded cityguard",
		LongDesc: "A reloaded cityguard stands here.\r\n",
		Level:    20, Thac0: 5, ArmorClass: 1,
		HitDice: Dice{Number: 4, Size: 8, Bonus: 20},
		Gold:    999, Exp: 5000, Position: int32(PosStanding),
		ActionFlags: MobIsNPC | 1<<10, // an arbitrary extra bit, not just the always-set one
	}
}

func TestReloadMobileUpdatesTheSharedPrototype(t *testing.T) {
	l := resetWorld(nil)
	c := l.SpawnMobile(3060, 3001, newRNG())
	if c == nil {
		t.Fatal("SpawnMobile returned nil")
	}

	refreshed, ok := l.ReloadMobile(freshGuardDef(), newRNG())
	if !ok {
		t.Fatal("ReloadMobile refused an unengaged instance")
	}
	if refreshed != 1 {
		t.Errorf("refreshed = %d, want 1", refreshed)
	}

	// Behavioural/descriptive fields are read live from the shared
	// *MobDef every time, so the existing instance sees them at once —
	// the whole point of mutating in place rather than swapping the map
	// entry.
	if c.MobDef.LongDesc != "A reloaded cityguard stands here.\r\n" {
		t.Errorf("MobDef.LongDesc = %q, want the reloaded text", c.MobDef.LongDesc)
	}
	if c.MobDef.ActionFlags&(1<<10) == 0 {
		t.Error("MobDef.ActionFlags did not pick up the new flag")
	}

	// Derived stats were recomputed as if freshly spawned.
	if c.Record.Points.MaxHit < 20+4 { // HitDice{4,8,20}'s floor
		t.Errorf("Record.Points.MaxHit = %d, want at least 24 (the new hit dice's floor)", c.Record.Points.MaxHit)
	}
	if c.Record.Points.Gold != 999 {
		t.Errorf("Record.Points.Gold = %d, want 999", c.Record.Points.Gold)
	}
	if c.Name != "the reloaded cityguard" {
		t.Errorf("Name = %q, want the reloaded short description", c.Name)
	}

	// Identity, room and possessions are untouched.
	if c.Room != 3001 {
		t.Errorf("Room = %d, want unchanged at 3001", c.Room)
	}
}

func TestReloadMobileRefusesWhileFighting(t *testing.T) {
	l := resetWorld(nil)
	c := l.SpawnMobile(3060, 3001, newRNG())
	if c == nil {
		t.Fatal("SpawnMobile returned nil")
	}
	victim := newCharacter("Welmar")
	if err := l.Enter(victim, 3001); err != nil {
		t.Fatal(err)
	}
	c.Fighting = victim
	before := c.MobDef.LongDesc

	refreshed, ok := l.ReloadMobile(freshGuardDef(), newRNG())
	if ok {
		t.Fatal("ReloadMobile succeeded against a fighting instance")
	}
	if refreshed != 0 {
		t.Errorf("refreshed = %d, want 0 on refusal", refreshed)
	}
	if c.MobDef.LongDesc != before {
		t.Error("ReloadMobile mutated the shared MobDef despite refusing — partial application")
	}
}

func TestReloadMobilePreservesSpecAssignment(t *testing.T) {
	l := resetWorld(nil)
	l.mobileDefs[3060].Spec = "cityguard"
	c := l.SpawnMobile(3060, 3001, newRNG())
	if c == nil {
		t.Fatal("SpawnMobile returned nil")
	}

	// The freshly-parsed def, exactly as a world-file parse would
	// produce: Spec is never in the file, so it comes back empty.
	if _, ok := l.ReloadMobile(freshGuardDef(), newRNG()); !ok {
		t.Fatal("ReloadMobile refused")
	}
	if c.MobDef.Spec != "cityguard" {
		t.Errorf("MobDef.Spec = %q, want the boot-time assignment preserved", c.MobDef.Spec)
	}
}

func TestReloadMobileUnknownVnumIsRefused(t *testing.T) {
	l := resetWorld(nil)
	if _, ok := l.ReloadMobile(&MobDef{Vnum: 99999}, newRNG()); ok {
		t.Error("ReloadMobile succeeded for a vnum nothing has")
	}
}

// freshTemple is resetWorld's own zone 30 (Bottom 3000, Top 3099), room
// 3001 and mob 3060, all changed the way a builder editing the world
// file would change them.
func freshTemple() (*ZoneDef, []*RoomDef, []*MobDef) {
	zone := &ZoneDef{Vnum: 30, Name: "Midgaard Reloaded", Bottom: 3000, Top: 3099, Lifespan: 20, ResetMode: 1}
	room := &RoomDef{Vnum: 3001, Name: "The Reloaded Temple"}
	mob := freshGuardDef()
	return zone, []*RoomDef{room}, []*MobDef{mob}
}

func TestReloadZoneUpdatesRoomsAndMobiles(t *testing.T) {
	l := resetWorld(nil)
	l.SpawnMobile(3060, 3001, newRNG())

	zone, rooms, mobs := freshTemple()
	result, ok := l.ReloadZone(zone, rooms, mobs, newRNG())
	if !ok {
		t.Fatal("ReloadZone refused an empty, unengaged zone")
	}
	if result.Rooms != 1 {
		t.Errorf("Rooms = %d, want 1", result.Rooms)
	}
	if result.Mobiles != 1 {
		t.Errorf("Mobiles = %d, want 1", result.Mobiles)
	}

	if got := l.Room(3001).Name; got != "The Reloaded Temple" {
		t.Errorf("room 3001's name = %q, want the reloaded text", got)
	}
	if got := l.Zones()[0].Name; got != "Midgaard Reloaded" {
		t.Errorf("zone name = %q, want the reloaded text", got)
	}
	if got := l.Zones()[0].Lifespan; got != 20 {
		t.Errorf("zone lifespan = %d, want 20", got)
	}
}

func TestReloadZoneRefusesWithAPlayerPresent(t *testing.T) {
	l := resetWorld(nil)
	player := newCharacter("Welmar")
	if err := l.Enter(player, 3001); err != nil {
		t.Fatal(err)
	}

	zone, rooms, mobs := freshTemple()
	_, ok := l.ReloadZone(zone, rooms, mobs, newRNG())
	if ok {
		t.Fatal("ReloadZone succeeded with a player standing in the zone")
	}
	if l.Room(3001).Name == "The Reloaded Temple" {
		t.Error("ReloadZone applied despite a player present — partial application")
	}
}

func TestReloadZoneRefusesWithAFightingMobile(t *testing.T) {
	l := resetWorld(nil)
	c := l.SpawnMobile(3060, 3001, newRNG())
	victim := newCharacter("Welmar")
	if err := l.Enter(victim, 3001); err != nil {
		t.Fatal(err)
	}
	c.Fighting = victim

	zone, rooms, mobs := freshTemple()
	_, ok := l.ReloadZone(zone, rooms, mobs, newRNG())
	if ok {
		t.Fatal("ReloadZone succeeded with a mobile fighting in the zone")
	}
}

func TestReloadZoneSkipsVnumsTheWorldDoesNotAlreadyHave(t *testing.T) {
	l := resetWorld(nil)
	zone, rooms, mobs := freshTemple()
	// A room and a mob the running world has never heard of — reload
	// updates what exists, it does not import what is new.
	rooms = append(rooms, &RoomDef{Vnum: 3005, Name: "A brand new room"})
	mobs = append(mobs, &MobDef{Vnum: 3062, ShortDesc: "a brand new mob"})

	result, ok := l.ReloadZone(zone, rooms, mobs, newRNG())
	if !ok {
		t.Fatal("ReloadZone refused")
	}
	if result.Rooms != 1 || result.Mobiles != 0 {
		t.Errorf("Rooms=%d Mobiles=%d, want 1 and 0 (only the pre-existing room updated, the new room and mob skipped)",
			result.Rooms, result.Mobiles)
	}
	if l.Room(3005) != nil {
		t.Error("a brand new room vnum was created by reload")
	}
}

func TestReloadZoneUnknownVnumIsRefused(t *testing.T) {
	l := resetWorld(nil)
	if _, ok := l.ReloadZone(&ZoneDef{Vnum: 9999}, nil, nil, newRNG()); ok {
		t.Error("ReloadZone succeeded for a zone vnum nothing has")
	}
}
