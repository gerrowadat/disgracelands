// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

func sample() []houses.House {
	return []houses.House{
		{
			Vnum: 3210, Atrium: 3211, ExitNum: 0,
			BuiltOn: time.Unix(1_100_000_000, 0).UTC(),
			Mode:    houses.ModePrivate, Owner: 9,
		},
		{
			Vnum: 3200, Atrium: 3201, ExitNum: 2,
			BuiltOn:     time.Unix(1_000_000_000, 0).UTC(),
			LastPayment: time.Unix(1_700_000_000, 0).UTC(),
			Mode:        houses.ModePrivate, Owner: 7,
			Guests: []int64{11, 12, 13},
		},
	}
}

func TestHousesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := New(houses.Config{ObjectDir: dir})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	want := sample()
	if err := s.Save(want); err != nil {
		t.Fatalf("saving: %v", err)
	}

	again, err := New(houses.Config{ObjectDir: dir})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	got, err := again.Load()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("read %d houses, want %d", len(got), len(want))
	}
	// Sorted by vnum, unlike the input.
	if got[0].Vnum != 3200 || got[1].Vnum != 3210 {
		t.Errorf("houses are not sorted by vnum: %+v", got)
	}
	for _, w := range want {
		var g houses.House
		for _, c := range got {
			if c.Vnum == w.Vnum {
				g = c
			}
		}
		if g.Atrium != w.Atrium || g.ExitNum != w.ExitNum || g.Mode != w.Mode || g.Owner != w.Owner {
			t.Errorf("house #%d round-tripped as %+v, want %+v", w.Vnum, g, w)
		}
		if !g.BuiltOn.Equal(w.BuiltOn) || !g.LastPayment.Equal(w.LastPayment) {
			t.Errorf("house #%d dates round-tripped as %s/%s, want %s/%s",
				w.Vnum, g.BuiltOn, g.LastPayment, w.BuiltOn, w.LastPayment)
		}
		if len(g.Guests) != len(w.Guests) {
			t.Errorf("house #%d has %d guests, want %d", w.Vnum, len(g.Guests), len(w.Guests))
		}
	}
}

func TestHouseObjectsRoundTripFlat(t *testing.T) {
	dir := t.TempDir()
	s, err := New(houses.Config{ObjectDir: dir})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := s.Save(sample()); err != nil {
		t.Fatalf("saving control records: %v", err)
	}

	if objs, err := s.LoadObjects(3200); err != nil || objs != nil {
		t.Errorf("loading an empty house gave %d objects, %v", len(objs), err)
	}

	// A nested bag: still flat on the wire, since houses do not get the
	// containment deviation player rent files got (see the package
	// comment).
	contents := []player.StoredObject{
		{Vnum: 3032, Weight: 5, Contains: []player.StoredObject{{Vnum: 3009, Weight: 1}}},
		{Vnum: 3022, Weight: 3},
	}
	if err := s.SaveObjects(3200, contents); err != nil {
		t.Fatalf("saving objects: %v", err)
	}

	again, err := New(houses.Config{ObjectDir: dir})
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	got, err := again.LoadObjects(3200)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if len(got) != len(contents) {
		t.Fatalf("read %d objects, want %d", len(got), len(contents))
	}
	if got[0].Vnum != 3032 || len(got[0].Contains) != 1 || got[0].Contains[0].Vnum != 3009 {
		t.Errorf("object 0 round-tripped as %+v", got[0])
	}

	// The control record survives the object save untouched.
	houses2, err := again.Load()
	if err != nil || len(houses2) != 2 {
		t.Fatalf("control records after SaveObjects: %v, %v", houses2, err)
	}
}

// SaveControl replacing the roster preserves each surviving house's
// contents, and a house dropped from the list loses its own (the
// documented divergence from classic's orphan-file behaviour).
func TestSaveReplacesTheRosterButKeepsSurvivingContents(t *testing.T) {
	s, err := New(houses.Config{ObjectDir: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := s.Save([]houses.House{{Vnum: 3200, Owner: 7}, {Vnum: 3210, Owner: 9}}); err != nil {
		t.Fatalf("saving: %v", err)
	}
	if err := s.SaveObjects(3200, []player.StoredObject{{Vnum: 1}}); err != nil {
		t.Fatalf("saving objects: %v", err)
	}
	if err := s.SaveObjects(3210, []player.StoredObject{{Vnum: 2}}); err != nil {
		t.Fatalf("saving objects: %v", err)
	}

	// Re-save the roster with only 3200 in it.
	if err := s.Save([]houses.House{{Vnum: 3200, Owner: 7}}); err != nil {
		t.Fatalf("re-saving: %v", err)
	}

	if got, err := s.LoadObjects(3200); err != nil || len(got) != 1 {
		t.Errorf("house 3200's contents did not survive: %v, %v", got, err)
	}
	if got, err := s.LoadObjects(3210); err != nil || got != nil {
		t.Errorf("house 3210's contents should have gone with its dropped record: %v, %v", got, err)
	}
}

func TestDeleteObjectsClearsButKeepsTheControlRecord(t *testing.T) {
	s, err := New(houses.Config{ObjectDir: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := s.Save([]houses.House{{Vnum: 3200, Owner: 7}}); err != nil {
		t.Fatalf("saving: %v", err)
	}
	if err := s.SaveObjects(3200, []player.StoredObject{{Vnum: 1}}); err != nil {
		t.Fatalf("saving objects: %v", err)
	}
	if err := s.DeleteObjects(3200); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if got, err := s.LoadObjects(3200); err != nil || got != nil {
		t.Errorf("objects survived DeleteObjects: %v, %v", got, err)
	}
	if got, err := s.Load(); err != nil || len(got) != 1 {
		t.Errorf("control record did not survive DeleteObjects: %v, %v", got, err)
	}
}

func TestAMissingFileIsNoHouses(t *testing.T) {
	s, err := New(houses.Config{ObjectDir: t.TempDir()})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if got, err := s.Load(); err != nil || len(got) != 0 {
		t.Errorf("a missing file produced %v, %v", got, err)
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	s, err := New(houses.Config{ObjectDir: t.TempDir(), ReadOnly: true})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := s.Save(sample()); err == nil {
		t.Error("a read-only store wrote the control records")
	}
	if err := s.SaveObjects(1, []player.StoredObject{{Vnum: 1}}); err == nil {
		t.Error("a read-only store wrote objects")
	}
}
