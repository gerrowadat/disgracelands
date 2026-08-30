// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// Since #285 examples/torture carries this case too -- obj #4850 below
// every range and mob #5150 above them, in a two-zone world -- so a plain
// `dlctl import` of that directory fails if either branch of fallbackZone
// goes wrong. This stays because it is finer grained: it builds the world
// in memory and can put a record in the gap *between* two ranges, which a
// corpus with adjacent zones cannot.
//
// outOfRangeWorld is two ordinary zones plus one mobile, one object and one
// shop whose vnums land in the gap between them — a builder putting a
// record in a zone's file without noticing it had run past the zone's
// declared top, which the C neither prevents nor cares about (db.c reads
// every indexed file into one flat table; zone_table's bottom/top only
// bound resets and edit permission).
func outOfRangeWorld() *game.World {
	zoneA := &game.ZoneDef{Vnum: 30, Name: "Lower", Bottom: 3000, Top: 3099, Lifespan: 30}
	zoneB := &game.ZoneDef{Vnum: 40, Name: "Upper", Bottom: 4000, Top: 4099, Lifespan: 30}
	return &game.World{
		Zones: []*game.ZoneDef{zoneA, zoneB},
		Rooms: []*game.RoomDef{
			{Vnum: 3001, Zone: 30, Name: "A room", Description: "A room.\r\n"},
			{Vnum: 4001, Zone: 40, Name: "Another room", Description: "Another room.\r\n"},
		},
		Mobiles: []*game.MobDef{
			{Vnum: 3200, Keywords: "stray mob", ShortDesc: "a stray mobile",
				LongDesc: "A stray mobile stands here.\r\n", Level: 1},
		},
		Objects: []*game.ObjDef{
			{Vnum: 3200, Keywords: "stray obj", ShortDesc: "a stray object",
				Description: "A stray object lies here.\r\n"},
		},
		Shops: []*game.ShopDef{
			{Vnum: 3200, Keeper: 3200, Rooms: []game.RoomVnum{3001}},
		},
	}
}

// TestOutOfRangeVnumsSurviveTheRoundTrip is the regression for records that
// no zone's range claims. The writer works one zone at a time and used to
// match on range alone, so nothing ever wrote these and — because no zone
// had been asked about them — nothing reported that they were gone either.
func TestOutOfRangeVnumsSurviveTheRoundTrip(t *testing.T) {
	w := outOfRangeWorld()
	dir := t.TempDir()
	writeImportedManifest(t, dir, w)

	src, err := New(world.Config{Dir: dir})
	if err != nil {
		t.Fatalf("open yaml source: %v", err)
	}
	defer func() { _ = src.Close() }()

	for _, z := range w.Zones {
		if err := src.WriteZone(context.Background(), z, w); err != nil {
			t.Fatalf("write zone %d: %v", z.Vnum, err)
		}
	}

	back, _, err := src.LoadWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("yaml load: %v", err)
	}

	if got, want := len(back.Mobiles), len(w.Mobiles); got != want {
		t.Errorf("mobiles: got %d, want %d", got, want)
	}
	if got, want := len(back.Objects), len(w.Objects); got != want {
		t.Errorf("objects: got %d, want %d", got, want)
	}
	if got, want := len(back.Shops), len(w.Shops); got != want {
		t.Errorf("shops: got %d, want %d", got, want)
	}
}

// TestOutOfRangeVnumLandsInTheNearestZoneBelow pins which file it goes to,
// since "somewhere" is enough for correctness but not enough to keep the
// output stable across runs. The nearest range beginning at or below the
// vnum is the zone whose file it almost certainly came from.
func TestOutOfRangeVnumLandsInTheNearestZoneBelow(t *testing.T) {
	w := outOfRangeWorld()
	lower, upper := w.Zones[0], w.Zones[1]

	if !writtenUnder(w.Zones, lower, 3200) {
		t.Errorf("vnum 3200 should be written under zone %d", lower.Vnum)
	}
	if writtenUnder(w.Zones, upper, 3200) {
		t.Errorf("vnum 3200 should not also be written under zone %d", upper.Vnum)
	}
	// Below every range: the lowest zone takes it, so it is never dropped.
	if !writtenUnder(w.Zones, lower, 10) {
		t.Errorf("vnum 10 should fall to the lowest zone %d", lower.Vnum)
	}
	// Inside a range: unchanged behaviour, claimed by exactly its own zone.
	if !writtenUnder(w.Zones, upper, 4001) || writtenUnder(w.Zones, lower, 4001) {
		t.Errorf("vnum 4001 should be written under zone %d only", upper.Vnum)
	}
}
