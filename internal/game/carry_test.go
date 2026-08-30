// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// objectWorld is two rooms and a handful of prototypes.
func objectWorld() *Live {
	rooms := []*RoomDef{
		{Vnum: 3001, Name: "The Temple Of Midgaard"},
		{Vnum: 3002, Name: "The Temple Square"},
	}
	objects := []*ObjDef{
		{
			Vnum: 100, Keywords: "sword long", ShortDesc: "a long sword",
			Type: ItemWeapon, WearFlags: NewSet(ItemWearTake, ItemWearWield), Weight: 10,
		},
		{
			Vnum: 101, Keywords: "bag small", ShortDesc: "a small bag",
			Type: ItemContainer, WearFlags: NewSet(ItemWearTake), Weight: 5,
		},
		{
			Vnum: 102, Keywords: "ring gold", ShortDesc: "a gold ring",
			Type: ItemArmor, WearFlags: NewSet(ItemWearTake, ItemWearFinger), Weight: 1,
		},
		{
			Vnum: 103, Keywords: "fountain", ShortDesc: "a fountain",
			Type: ItemFountain, Weight: 500,
		},
	}
	return NewLive(&World{Rooms: rooms, Objects: objects})
}

func newCharacter(name string) *Character {
	return &Character{
		Name:     name,
		Record:   &PlayerRecord{Name: name, Class: ClassWarrior, Level: 10, Weight: 150},
		Position: PosStanding,
	}
}

// assertOnePlace is the invariant this whole file exists for: an object is in
// exactly one place, and everything that claims to hold it agrees.
func assertOnePlace(t *testing.T, l *Live, o *Object) {
	t.Helper()

	places := 0
	for _, list := range l.roomObjects {
		for _, candidate := range list {
			if candidate == o {
				places++
				if _, ok := o.Placement().(InRoom); !ok {
					t.Errorf("%s is in a room list but says it is %T", o.Name(), o.Placement())
				}
			}
		}
	}
	for _, c := range l.Players() {
		for _, candidate := range c.Carrying {
			if candidate == o {
				places++
				if p, ok := o.Placement().(CarriedBy); !ok || p.Holder != c {
					t.Errorf("%s is in %s's inventory but says otherwise", o.Name(), c.Name)
				}
			}
		}
		for pos, candidate := range c.Equipment {
			if candidate == o {
				places++
				if p, ok := o.Placement().(WornBy); !ok || p.Holder != c || int(p.At) != pos {
					t.Errorf("%s is worn by %s but says otherwise", o.Name(), c.Name)
				}
			}
		}
	}
	for _, container := range l.Objects() {
		for _, candidate := range container.Contents {
			if candidate == o {
				places++
				if p, ok := o.Placement().(InContainer); !ok || p.Container != container {
					t.Errorf("%s is inside %s but says otherwise", o.Name(), container.Name())
				}
			}
		}
	}

	if places != 1 {
		t.Errorf("%s is in %d places, want exactly 1 (it says %T)", o.Name(), places, o.Placement())
	}
}

// TestAnObjectIsOnlyEverInOnePlace walks it through every location in turn.
// This is the leak the C is prone to: eight obj_to_*/obj_from_* functions and
// an invariant maintained by every caller remembering the matching pair.
func TestAnObjectIsOnlyEverInOnePlace(t *testing.T) {
	l := objectWorld()
	welmar := newCharacter("Welmar")
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}

	sword := l.NewObject(100)
	bag := l.NewObject(101)
	if sword == nil || bag == nil {
		t.Fatal("could not instantiate the prototypes")
	}

	// Floor, then inventory, then worn, then a container, then back to the
	// floor — never calling a matching "take it out of where it was" first.
	l.ObjectToRoom(sword, 3001)
	assertOnePlace(t, l, sword)

	l.ObjectToChar(sword, welmar)
	assertOnePlace(t, l, sword)
	if len(l.RoomObjects(3001)) != 0 {
		t.Error("the sword is still on the floor after being picked up")
	}

	if !l.Equip(sword, welmar, WearWield) {
		t.Fatal("could not wield the sword")
	}
	assertOnePlace(t, l, sword)
	if len(welmar.Carrying) != 0 {
		t.Error("the sword is still in the inventory after being wielded")
	}

	l.ObjectToChar(bag, welmar)
	if !l.ObjectToObject(sword, bag) {
		t.Fatal("could not put the sword in the bag")
	}
	assertOnePlace(t, l, sword)
	if welmar.Equipment[WearWield] != nil {
		t.Error("the sword is still wielded after going into the bag")
	}

	l.ObjectToRoom(sword, 3002)
	assertOnePlace(t, l, sword)
	if len(bag.Contents) != 0 {
		t.Error("the sword is still in the bag after being dropped")
	}
}

// TestAContainerCannotHoldItself, directly or through a chain. The C does not
// check, and the result is a cycle that hangs the next thing to walk the
// object graph.
func TestAContainerCannotHoldItself(t *testing.T) {
	l := objectWorld()

	outer := l.NewObject(101)
	middle := l.NewObject(101)
	inner := l.NewObject(101)

	if l.ObjectToObject(outer, outer) {
		t.Error("a bag was put inside itself")
	}

	if !l.ObjectToObject(middle, outer) || !l.ObjectToObject(inner, middle) {
		t.Fatal("could not nest the bags")
	}
	if l.ObjectToObject(outer, inner) {
		t.Error("a bag was put inside its own contents, making a cycle")
	}
	if l.ObjectToObject(outer, middle) {
		t.Error("a bag was put inside its own contents, making a cycle")
	}
}

// TestAnOccupiedSlotRefusesASecondObject. The C logs a SYSERR and drops the
// object on the floor, which is a way of losing equipment.
func TestAnOccupiedSlotRefusesASecondObject(t *testing.T) {
	l := objectWorld()
	welmar := newCharacter("Welmar")
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}

	first := l.NewObject(100)
	second := l.NewObject(100)
	l.ObjectToChar(first, welmar)
	l.ObjectToChar(second, welmar)

	if !l.Equip(first, welmar, WearWield) {
		t.Fatal("could not wield the first sword")
	}
	if l.Equip(second, welmar, WearWield) {
		t.Error("wielded a second sword in an occupied hand")
	}
	// And the refusal left the second where it was rather than losing it.
	assertOnePlace(t, l, second)
	if _, ok := second.Placement().(CarriedBy); !ok {
		t.Errorf("the refused sword is %T, want still carried", second.Placement())
	}
}

// TestDestroyingAContainerSpillsIt rather than destroying the contents, which
// is what stops a bag being extracted from taking everything with it.
func TestDestroyingAContainerSpillsIt(t *testing.T) {
	l := objectWorld()

	bag := l.NewObject(101)
	sword := l.NewObject(100)
	ring := l.NewObject(102)

	l.ObjectToRoom(bag, 3001)
	l.ObjectToObject(sword, bag)
	l.ObjectToObject(ring, bag)

	l.ExtractObject(bag)

	floor := l.RoomObjects(3001)
	if len(floor) != 2 {
		t.Fatalf("%d objects on the floor, want the bag's 2 contents", len(floor))
	}
	for _, o := range floor {
		assertOnePlace(t, l, o)
	}

	// And the bag itself is gone.
	for _, o := range l.Objects() {
		if o == bag {
			t.Error("the extracted bag is still in the world")
		}
	}
}

// TestExtractingFromNowhereDestroysTheContents, because there is nowhere for
// them to spill to.
func TestExtractingFromNowhereDestroysTheContents(t *testing.T) {
	l := objectWorld()

	bag := l.NewObject(101)
	sword := l.NewObject(100)
	l.track(bag)
	l.ObjectToObject(sword, bag)

	l.ExtractObject(bag)

	if len(l.Objects()) != 0 {
		t.Errorf("%d objects survive, want none", len(l.Objects()))
	}
}

func TestCanWearAt(t *testing.T) {
	l := objectWorld()
	sword := l.ObjectDef(100)
	ring := l.ObjectDef(102)
	fountain := l.ObjectDef(103)

	for _, tc := range []struct {
		def  *ObjDef
		pos  WearPosition
		want bool
	}{
		{sword, WearWield, true},
		{sword, WearFingerRight, false},
		{ring, WearFingerRight, true},
		{ring, WearFingerLeft, true},
		{ring, WearWield, false},
		{fountain, WearBody, false},
		// The light slot goes by type, not by a wear flag.
		{fountain, WearLight, false},
		{nil, WearBody, false},
		{sword, -1, false},
		{sword, NumWears, false},
	} {
		if got := CanWearAt(tc.def, tc.pos); got != tc.want {
			name := "nil"
			if tc.def != nil {
				name = tc.def.ShortDesc
			}
			t.Errorf("CanWearAt(%s, %d) = %v, want %v", name, tc.pos, got, tc.want)
		}
	}
}

// TestKeywordMatching. isname is a **whole word** match, case-insensitively —
// see TestMatchesKeywordsAgainstTheC, which checks 168 pairings against the C.
//
// This test asserted that "swo" and "s" matched a long sword until that oracle
// was written. They never did on the real server.
func TestKeywordMatching(t *testing.T) {
	l := objectWorld()
	sword := l.NewObject(100) // "sword long"

	for _, word := range []string{"sword", "long", "LONG", "Sword"} {
		if !sword.Matches(word) {
			t.Errorf("%q does not match %q", word, sword.Keywords)
		}
	}
	for _, word := range []string{"", "swo", "s", "swords", "bag", "ong", "lon"} {
		if sword.Matches(word) {
			t.Errorf("%q matches %q and should not", word, sword.Keywords)
		}
	}
}

func TestTotalWeightIncludesContents(t *testing.T) {
	l := objectWorld()

	bag := l.NewObject(101)   // 5
	sword := l.NewObject(100) // 10
	ring := l.NewObject(102)  // 1

	if got := bag.TotalWeight(); got != 5 {
		t.Errorf("an empty bag weighs %d, want 5", got)
	}

	l.ObjectToObject(sword, bag)
	l.ObjectToObject(ring, bag)
	if got := bag.TotalWeight(); got != 16 {
		t.Errorf("a full bag weighs %d, want 16", got)
	}

	// And a character's carried weight counts what they are wearing too.
	welmar := newCharacter("Welmar")
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}
	l.ObjectToChar(bag, welmar)
	worn := l.NewObject(102)
	l.Equip(worn, welmar, WearFingerRight)

	if got := welmar.CarriedWeight(); got != 17 {
		t.Errorf("carried weight is %d, want 17", got)
	}
}

// TestAPlacementCarriesExactlyItsOwnFields is what the union bought, as a
// test rather than as a claim.
//
// Before step 5 an Object had five fields — a Location enum plus Room,
// Holder, WornAt and Container — and "exactly one of these is meaningful,
// and which one the enum says" was an invariant maintained by convention.
// The states that convention could not rule out were reachable: a Location
// of InRoom with a stale Holder still set, a WornBy with WornAt of -1, a
// CarriedBy whose Room was some room the object was dropped in three moves
// ago. Nothing tested for them because there was nothing to test — every
// combination compiled.
//
// Now there is nothing to go stale: the placement *is* the fields, and
// asking for the wrong ones does not compile. What is left to check is that
// each shape answers only its own questions.
func TestAPlacementCarriesExactlyItsOwnFields(t *testing.T) {
	l := objectWorld()
	c := newCharacter("Welmar")
	if err := l.Enter(c, 3001); err != nil {
		t.Fatal(err)
	}
	sword := l.NewObject(100)
	bag := l.NewObject(101)
	if sword == nil || bag == nil {
		t.Fatal("could not instantiate the prototypes")
	}

	l.ObjectToRoom(sword, 3001)
	if _, ok := sword.RoomOf(); !ok {
		t.Error("a sword on the floor is in no room")
	}
	if sword.HolderOf() != nil || sword.ContainerOf() != nil {
		t.Error("a sword on the floor has a holder or a container")
	}
	if _, ok := sword.WornAt(); ok {
		t.Error("a sword on the floor is worn somewhere")
	}

	l.ObjectToChar(sword, c)
	if sword.HolderOf() != c {
		t.Error("a carried sword has no holder")
	}
	if room, ok := sword.RoomOf(); ok {
		t.Errorf("a carried sword is also in room %d; carrying is not lying on the floor", room)
	}
	if _, ok := sword.WornAt(); ok {
		t.Error("a carried sword is worn; carrying is not wearing")
	}

	if !l.Equip(sword, c, WearWield) {
		t.Fatal("could not wield the sword")
	}
	if at, ok := sword.WornAt(); !ok || at != WearWield {
		t.Errorf("a wielded sword is worn at %v (%v), want the wield slot", at, ok)
	}
	if sword.HolderOf() != c {
		t.Error("a wielded sword has no holder; WornBy carries one too")
	}

	l.ObjectToRoom(bag, 3001)
	if !l.ObjectToObject(sword, bag) {
		t.Fatal("could not put the sword in the bag")
	}
	if sword.ContainerOf() != bag {
		t.Error("a sword in a bag is in no container")
	}
	if _, ok := sword.RoomOf(); ok {
		t.Error("a sword in a bag in a room is also in the room; the C keeps two lists and so does this")
	}
	if sword.HolderOf() != nil {
		t.Error("a sword in a bag has a holder")
	}

	l.detach(sword)
	if sword.Placement() != nil {
		t.Errorf("a detached sword is %T, want nowhere", sword.Placement())
	}

	// And nowhere answers nothing rather than a stale anything.
	if sword.HolderOf() != nil || sword.ContainerOf() != nil {
		t.Error("a detached sword still points at a holder or container")
	}
	if room, ok := sword.RoomOf(); ok || room != NoRoom {
		t.Errorf("a detached sword is in room %d (%v), want NoRoom and false", room, ok)
	}
}
