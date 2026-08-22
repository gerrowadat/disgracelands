// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gerrowadat/disgracelands/internal/persist/houses"
	"github.com/gerrowadat/disgracelands/internal/persist/player"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := New(houses.Config{ControlPath: filepath.Join(dir, "hcontrol"), ObjectDir: filepath.Join(dir, "house")})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	return s, dir
}

func sample() []houses.House {
	return []houses.House{
		{
			Vnum: 3200, Atrium: 3201, ExitNum: 2,
			BuiltOn:     time.Unix(1_000_000_000, 0).UTC(),
			LastPayment: time.Unix(1_700_000_000, 0).UTC(),
			Mode:        houses.ModePrivate,
			Owner:       7,
			Guests:      []int64{11, 12, 13},
		},
		{
			Vnum: 3210, Atrium: 3211, ExitNum: 0,
			BuiltOn: time.Unix(1_100_000_000, 0).UTC(),
			Owner:   9,
		},
	}
}

func TestTheControlFileRoundTrips(t *testing.T) {
	s, _ := newStore(t)
	want := sample()
	if err := s.Save(want); err != nil {
		t.Fatalf("saving: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("read %d houses, want %d", len(got), len(want))
	}
	for i := range want {
		w, g := want[i], got[i]
		if g.Vnum != w.Vnum || g.Atrium != w.Atrium || g.ExitNum != w.ExitNum ||
			g.Mode != w.Mode || g.Owner != w.Owner {
			t.Errorf("house %d round-tripped as %+v, want %+v", i, g, w)
		}
		if !g.BuiltOn.Equal(w.BuiltOn) || !g.LastPayment.Equal(w.LastPayment) {
			t.Errorf("house %d dates round-tripped as %s/%s, want %s/%s",
				i, g.BuiltOn, g.LastPayment, w.BuiltOn, w.LastPayment)
		}
		if len(g.Guests) != len(w.Guests) {
			t.Errorf("house %d has %d guests, want %d", i, len(g.Guests), len(w.Guests))
			continue
		}
		for j := range w.Guests {
			if g.Guests[j] != w.Guests[j] {
				t.Errorf("house %d guest %d is %d, want %d", i, j, g.Guests[j], w.Guests[j])
			}
		}
	}
}

// One record per house, exactly, with no header — the C fwrites the array.
func TestTheControlFileIsAnArrayOfRecords(t *testing.T) {
	s, dir := newStore(t)
	if err := s.Save(sample()); err != nil {
		t.Fatalf("saving: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "hcontrol"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := int64(2 * recordSize); info.Size() != want {
		t.Errorf("two houses take %d bytes, want %d", info.Size(), want)
	}
}

// A guest list longer than the C's array is truncated rather than written
// past the end of the record.
func TestTooManyGuestsAreTruncated(t *testing.T) {
	s, _ := newStore(t)
	h := houses.House{Vnum: 1, Owner: 1}
	for i := 0; i < houses.MaxGuests+5; i++ {
		h.Guests = append(h.Guests, int64(100+i))
	}
	if err := s.Save([]houses.House{h}); err != nil {
		t.Fatalf("saving: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if len(got[0].Guests) != houses.MaxGuests {
		t.Errorf("read %d guests, want %d", len(got[0].Guests), houses.MaxGuests)
	}
}

func TestAMissingControlFileIsNoHouses(t *testing.T) {
	s, _ := newStore(t)
	got, err := s.Load()
	if err != nil {
		t.Errorf("loading a missing control file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a missing control file produced %d houses", len(got))
	}
}

func TestHouseObjectFiles(t *testing.T) {
	s, _ := newStore(t)

	if objs, err := s.LoadObjects(3200); err != nil || objs != nil {
		t.Errorf("loading an empty house gave %d objects, %v", len(objs), err)
	}

	contents := []player.StoredObject{
		{Vnum: 3009, Weight: 1},
		{Vnum: 3010, Weight: 2, ExtraFlags: 1},
	}
	if err := s.SaveObjects(3200, contents); err != nil {
		t.Fatalf("saving: %v", err)
	}
	got, err := s.LoadObjects(3200)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if len(got) != len(contents) {
		t.Fatalf("read %d objects, want %d", len(got), len(contents))
	}
	for i := range contents {
		if got[i].Vnum != contents[i].Vnum || got[i].Weight != contents[i].Weight {
			t.Errorf("object %d round-tripped as %+v, want %+v", i, got[i], contents[i])
		}
	}

	// An emptied house leaves no file, which is what House_crashsave of an
	// empty room amounts to.
	if err := s.SaveObjects(3200, nil); err != nil {
		t.Fatalf("emptying: %v", err)
	}
	if _, err := os.Stat(s.objectFile(3200)); !os.IsNotExist(err) {
		t.Error("emptying a house left the file behind")
	}

	if err := s.DeleteObjects(3200); err != nil {
		t.Errorf("deleting an already-absent house file: %v", err)
	}
}

func TestAReadOnlyStoreRefusesToWrite(t *testing.T) {
	dir := t.TempDir()
	s, err := New(houses.Config{ControlPath: filepath.Join(dir, "hcontrol"), ObjectDir: filepath.Join(dir, "house"), ReadOnly: true})
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if err := s.Save(sample()); err == nil {
		t.Error("a read-only store wrote the control file")
	}
	if err := s.SaveObjects(1, []player.StoredObject{{Vnum: 1}}); err == nil {
		t.Error("a read-only store wrote a house file")
	}
}
