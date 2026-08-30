// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// The population index (#322) replaced two functions that counted by
// walking the world with two map lookups. The scans are kept here as the
// oracle: every test below drives the world through some sequence of
// spawns, extractions and resets and then requires the index to agree with
// them, entry for entry.
//
// That is the same posture the command-table and constants tests take
// towards the C — derive the expected answer rather than assert it — and
// it is the only thing standing between a future call site that writes
// l.mobiles or l.objects directly and a population cap that quietly
// counts the wrong number of mobiles for the rest of the server's life.

// countMobilesByScan is the pre-#322 mobileCount.
func countMobilesByScan(l *Live, vnum MobVnum) int32 {
	var n int32
	for c := range l.mobiles {
		if c.MobDef != nil && c.MobDef.Vnum == vnum {
			n++
		}
	}
	return n
}

// countObjectsByScan is the pre-#322 objectCount.
func countObjectsByScan(l *Live, vnum ObjVnum) int32 {
	var n int32
	for _, o := range l.objects {
		if o.Vnum() == vnum {
			n++
		}
	}
	return n
}

// requireIndexAgrees checks every vnum the world has a prototype for, not
// merely the ones the test touched: an index that has drifted positive on
// a vnum nobody looked at is exactly as broken and much harder to find
// later.
func requireIndexAgrees(t *testing.T, l *Live, when string) {
	t.Helper()

	for vnum := range l.mobileDefs {
		want := countMobilesByScan(l, vnum)
		if got := l.mobileCount(vnum); got != want {
			t.Errorf("%s: mobileCount(%d) = %d, a scan finds %d", when, vnum, got, want)
		}
	}
	for vnum := range l.objectDefs {
		want := countObjectsByScan(l, vnum)
		if got := l.objectCount(vnum); got != want {
			t.Errorf("%s: objectCount(%d) = %d, a scan finds %d", when, vnum, got, want)
		}
	}
	// And nothing may be left counted that no longer exists — a decrement
	// that ran twice shows up here as a negative and nowhere else.
	for vnum, n := range l.mobCounts {
		if n < 0 {
			t.Errorf("%s: mobCounts[%d] = %d, below zero", when, vnum, n)
		}
	}
	for vnum, n := range l.objCounts {
		if n < 0 {
			t.Errorf("%s: objCounts[%d] = %d, below zero", when, vnum, n)
		}
	}
}

func TestPopulationIndexAgreesWithAScan(t *testing.T) {
	l := resetWorld([]ResetCommand{
		{Command: ResetMobile, Arg1: 3060, Arg2: 2, Arg3: 3001},
		{Command: ResetObject, Arg1: 3020, Arg2: 2, Arg3: 3001},
		{Command: ResetMobile, Arg1: 3061, Arg2: 3, Arg3: 3002},
		{Command: ResetObject, Arg1: 3021, Arg2: 1, Arg3: 3002},
		{Command: ResetStop},
	})
	r := newRNG()

	requireIndexAgrees(t, l, "before anything")

	// One reset makes one of each; the cap of two is what lets the second
	// reset make a second cityguard and then stop. Resetting a third time
	// must add nothing, which is the population cap doing its job — and
	// the whole reason mobileCount exists.
	l.ResetZone(l.Zones()[0], r)
	requireIndexAgrees(t, l, "after one reset")
	if got := l.mobileCount(3060); got != 1 {
		t.Errorf("after one reset there are %d cityguards, want 1", got)
	}

	l.ResetZone(l.Zones()[0], r)
	requireIndexAgrees(t, l, "after a second reset")

	l.ResetZone(l.Zones()[0], r)
	requireIndexAgrees(t, l, "after a third reset")
	if got := l.mobileCount(3060); got != 2 {
		t.Errorf("the cap of two left %d cityguards, want 2", got)
	}

	// Kill everything the reset made, which is what a busy zone does all
	// day, and reset again.
	for _, c := range l.Mobiles() {
		l.RemoveMobile(c)
	}
	requireIndexAgrees(t, l, "after removing every mobile")
	if got := l.mobileCount(3060); got != 0 {
		t.Errorf("after removing every mobile there are %d cityguards, want 0", got)
	}

	for _, o := range l.Objects() {
		l.ExtractObject(o)
	}
	requireIndexAgrees(t, l, "after extracting every object")

	l.ResetZone(l.Zones()[0], r)
	requireIndexAgrees(t, l, "after repopulating")
	if got := l.mobileCount(3060); got != 1 {
		t.Errorf("the repopulated zone has %d cityguards, want 1", got)
	}
}

// TestPopulationIndexSurvivesRepeatedRegistration covers the two ways a
// caller can say the same thing twice. Track is exported and an
// implementor's `load` reaches it, and ExtractObject recurses into
// contents; neither contract forbids being handed something already
// registered, so both have to be idempotent rather than merely usually
// called once.
func TestPopulationIndexSurvivesRepeatedRegistration(t *testing.T) {
	l := resetWorld(nil)
	r := newRNG()

	mob := l.SpawnMobile(3060, 3001, r)
	if mob == nil {
		t.Fatal("no cityguard spawned")
	}
	l.Track(mob)
	l.Track(mob)
	if got := l.mobileCount(3060); got != 1 {
		t.Errorf("tracking the same mobile three times counts %d, want 1", got)
	}
	requireIndexAgrees(t, l, "after repeated Track")

	l.RemoveMobile(mob)
	l.RemoveMobile(mob)
	if got := l.mobileCount(3060); got != 0 {
		t.Errorf("removing the same mobile twice counts %d, want 0", got)
	}
	requireIndexAgrees(t, l, "after repeated RemoveMobile")

	obj := l.NewObject(3020)
	l.ObjectToRoom(obj, 3001)
	l.track(obj)
	if got := l.objectCount(3020); got != 1 {
		t.Errorf("tracking the same object twice counts %d, want 1", got)
	}

	l.ExtractObject(obj)
	l.ExtractObject(obj)
	if got := l.objectCount(3020); got != 0 {
		t.Errorf("extracting the same object twice counts %d, want 0", got)
	}
	requireIndexAgrees(t, l, "after repeated ExtractObject")
}

// TestPopulationIndexCountsContainerContents is the case ExtractObject's
// recursion makes easy to get wrong: a bag counts as one object and so
// does each thing inside it, and extracting the bag spills them rather
// than destroying them.
func TestPopulationIndexCountsContainerContents(t *testing.T) {
	l := resetWorld(nil)

	bag := l.NewObject(3021)
	l.ObjectToRoom(bag, 3001)
	coin := l.NewObject(3022)
	l.ObjectToObject(coin, bag)

	if got := l.objectCount(3022); got != 1 {
		t.Errorf("a coin in a bag counts %d, want 1", got)
	}
	requireIndexAgrees(t, l, "with a coin in a bag")

	// The bag goes; the coin is spilled into the room, not destroyed.
	l.ExtractObject(bag)
	if got := l.objectCount(3021); got != 0 {
		t.Errorf("the extracted bag counts %d, want 0", got)
	}
	if got := l.objectCount(3022); got != 1 {
		t.Errorf("the spilled coin counts %d, want 1", got)
	}
	requireIndexAgrees(t, l, "after extracting the bag")
}
