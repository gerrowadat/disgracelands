// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// armourFor builds a piece of armour worth ac in value 0.
func armourFor(ac int32, affects ...ObjAffect) *Object {
	return &Object{
		Keywords: "armour", ShortDesc: "a suit of armour",
		Type:      ItemArmor,
		WearFlags: NewSet(ItemWearTake, ItemWearBody, ItemWearHead, ItemWearWrist),
		Values:    [NumObjValues]int32{ac},
		Affects:   affects,
	}
}

// TestArmourIsWorthMoreOnTheBody. The multiplier is the slot's, not the
// object's, so the same suit counts three times on the body and once on a
// wrist.
func TestArmourIsWorthMoreOnTheBody(t *testing.T) {
	armour := armourFor(5)
	for pos, want := range map[WearPosition]int32{
		WearBody:        15,
		WearHead:        10,
		WearLegs:        10,
		WearWristRight:  5,
		WearFingerRight: 5,
	} {
		if got := ArmorClassOf(armour, pos); got != want {
			t.Errorf("%v is worth %d, want %d", pos, got, want)
		}
	}

	// Anything that is not armour is worth nothing wherever it goes, even
	// with a value 0 that looks like an armour class.
	sword := &Object{Type: ItemWeapon, Values: [NumObjValues]int32{9}}
	if got := ArmorClassOf(sword, WearBody); got != 0 {
		t.Errorf("a weapon is worth %d armour class, want 0", got)
	}
}

// TestWornArmourChangesArmourClassAndTakingItOffPutsItBack.
func TestWornArmourChangesArmourClassAndTakingItOffPutsItBack(t *testing.T) {
	l := objectWorld()
	welmar := newCharacter("Welmar")
	welmar.Record.Points.Armor = 100
	SnapshotReal(welmar.Record)
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}

	armour := armourFor(5)
	l.track(armour)
	l.ObjectToChar(armour, welmar)

	if !l.Equip(armour, welmar, WearBody) {
		t.Fatal("could not wear the armour")
	}
	if got := welmar.Record.Points.Armor; got != 85 {
		t.Errorf("armour class is %d, want 100 - 15", got)
	}

	l.Unequip(welmar, WearBody)
	if got := welmar.Record.Points.Armor; got != 100 {
		t.Errorf("armour class is %d after taking it off, want 100", got)
	}
}

// TestAnObjectsAppliesAreRecomputedRatherThanAccumulated.
//
// This is the bug the recompute exists to prevent: put a ring on, cast a
// spell, take the ring off, and the C's paired add/subtract can leave the
// character permanently richer or poorer. Here the totals are rebuilt from
// the real values every time, so the order cannot matter.
func TestAnObjectsAppliesAreRecomputedRatherThanAccumulated(t *testing.T) {
	l := objectWorld()
	welmar := newCharacter("Welmar")
	welmar.Record.Points.HitRoll = 0
	welmar.Record.Points.DamRoll = 0
	SnapshotReal(welmar.Record)
	if err := l.Enter(welmar, 3001); err != nil {
		t.Fatal(err)
	}

	ring := &Object{
		Keywords: "ring", ShortDesc: "a gold ring",
		Type: ItemArmor, WearFlags: NewSet(ItemWearTake, ItemWearFinger),
		Affects: []ObjAffect{
			{Location: ApplyHitRoll, Modifier: 3},
			{Location: ApplyDamRoll, Modifier: 2},
		},
		PermAffect: NewSet(AffectDetectInvis),
	}
	l.track(ring)
	l.ObjectToChar(ring, welmar)

	if !l.Equip(ring, welmar, WearFingerRight) {
		t.Fatal("could not wear the ring")
	}
	if welmar.Record.Points.HitRoll != 3 || welmar.Record.Points.DamRoll != 2 {
		t.Errorf("wearing the ring gave %+d hit and %+d dam, want +3 and +2",
			welmar.Record.Points.HitRoll, welmar.Record.Points.DamRoll)
	}
	if !welmar.Record.AffectFlags.Has(AffectDetectInvis) {
		t.Error("the ring's permanent affect did not arrive")
	}

	// A spell lands and wears off while the ring is on.
	AddAffect(welmar.Record, Affect{
		Type: SpellBless, Location: ApplyHitRoll, Modifier: 2, Duration: 6,
	})
	if got := welmar.Record.Points.HitRoll; got != 5 {
		t.Errorf("blessed and ringed is %+d hit, want +5", got)
	}
	RemoveAffectsOf(welmar.Record, SpellBless)
	if got := welmar.Record.Points.HitRoll; got != 3 {
		t.Errorf("after the blessing wore off, %+d hit, want the ring's +3", got)
	}

	l.Unequip(welmar, WearFingerRight)
	if welmar.Record.Points.HitRoll != 0 || welmar.Record.Points.DamRoll != 0 {
		t.Errorf("taking the ring off left %+d hit and %+d dam, want nothing",
			welmar.Record.Points.HitRoll, welmar.Record.Points.DamRoll)
	}
	if welmar.Record.AffectFlags.Has(AffectDetectInvis) {
		t.Error("the ring's permanent affect outlived the ring")
	}
}

// TestZappingObjects, by alignment and by class.
func TestZappingObjects(t *testing.T) {
	good := &PlayerRecord{Alignment: 500, Class: ClassCleric}
	evil := &PlayerRecord{Alignment: -500, Class: ClassThief}
	neutral := &PlayerRecord{Alignment: 0, Class: ClassWarrior}

	antiGood := &Object{ExtraFlags: NewSet(ItemAntiGood)}
	antiEvil := &Object{ExtraFlags: NewSet(ItemAntiEvil)}
	antiNeutral := &Object{ExtraFlags: NewSet(ItemAntiNeutral)}

	for _, tc := range []struct {
		name string
		rec  *PlayerRecord
		obj  *Object
		want bool
	}{
		{"a good cleric and an anti-good sword", good, antiGood, true},
		{"a good cleric and an anti-evil sword", good, antiEvil, false},
		{"an evil thief and an anti-evil sword", evil, antiEvil, true},
		{"a neutral warrior and an anti-neutral sword", neutral, antiNeutral, true},
		{"a neutral warrior and an anti-good sword", neutral, antiGood, false},
		{"a cleric and an anti-cleric sword", good, &Object{ExtraFlags: NewSet(ItemAntiCleric)}, true},
		{"a cleric and an anti-thief sword", good, &Object{ExtraFlags: NewSet(ItemAntiThief)}, false},
	} {
		if got := Zaps(tc.rec, tc.obj); got != tc.want {
			t.Errorf("%s: zaps = %v, want %v", tc.name, got, tc.want)
		}
	}

	// The class test is remort-aware, because it uses the IS_<CLASS> macros:
	// a warrior who was once a thief is still zapped by anti-thief kit.
	exThief := &PlayerRecord{Class: ClassWarrior, RemortVector: NewSet(ClassThief)}
	if !Zaps(exThief, &Object{ExtraFlags: NewSet(ItemAntiThief)}) {
		t.Error("an ex-thief was not zapped by an anti-thief object")
	}
}
