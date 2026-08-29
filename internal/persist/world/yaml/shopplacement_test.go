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

// handNumberedShopWorld is the shape the archived Disgracelands lib/ turned
// out to have and no checked-in fixture did: two zones, and shop files whose
// shops are numbered by hand rather than out of the zone's own range.
//
// Zone 49's file holds shop #5050, whose number lands inside *zone 50's*
// declared range; zone 50's file holds shop #50, which no zone's range
// claims at all. Both are real: the archive's 190.shp holds shops #190-#194
// with keepers in the #19000s, and its 20.shp holds #1-#5 with keepers in
// the #2000s. Nothing in the C derives a shop's number from anything (db.c
// reads it to print it; only OasisOLC's real_shop looks one up), so a
// builder writing a .shp file by hand had no reason to keep them aligned.
//
// The zone order matters to the test and not to the archive: the classic
// loader takes shops in shp/index order, which is ascending by zone, so the
// world below loads them as [#5050, #50]. Placing each by its own vnum
// files #5050 under zone 50 and #50 under the fallback (zone 49, the lowest)
// — which reverses them, because zone files load in vnum order.
func handNumberedShopWorld() *game.World {
	zoneA := &game.ZoneDef{Vnum: 49, Name: "Lower", Bottom: 4900, Top: 4999, Lifespan: 30}
	zoneB := &game.ZoneDef{Vnum: 50, Name: "Upper", Bottom: 5000, Top: 5099, Lifespan: 30}
	return &game.World{
		Zones: []*game.ZoneDef{zoneA, zoneB},
		Rooms: []*game.RoomDef{
			{Vnum: 4901, Zone: 49, Name: "A room", Description: "A room.\r\n"},
			{Vnum: 5001, Zone: 50, Name: "Another room", Description: "Another room.\r\n"},
		},
		Mobiles: []*game.MobDef{
			{Vnum: 4902, Keywords: "lower keeper", ShortDesc: "the lower shopkeeper",
				LongDesc: "The lower shopkeeper stands here.\r\n", Level: 1},
			{Vnum: 5002, Keywords: "upper keeper", ShortDesc: "the upper shopkeeper",
				LongDesc: "The upper shopkeeper stands here.\r\n", Level: 1},
		},
		Shops: []*game.ShopDef{
			{Vnum: 5050, Keeper: 4902, Rooms: []game.RoomVnum{4901}},
			{Vnum: 50, Keeper: 5002, Rooms: []game.RoomVnum{5001}},
		},
	}
}

// TestShopsAreWrittenUnderTheirKeepersZone is the regression for the
// placement itself: which file each shop ends up in, which is what the
// ordering below is downstream of.
func TestShopsAreWrittenUnderTheirKeepersZone(t *testing.T) {
	w := handNumberedShopWorld()
	lower, upper := w.Zones[0], w.Zones[1]

	for _, tc := range []struct {
		shop *game.ShopDef
		want *game.ZoneDef
	}{
		{w.Shops[0], lower}, // #5050, keeper #4902: zone 49 despite the vnum
		{w.Shops[1], upper}, // #50, keeper #5002: zone 50 despite no zone claiming 50
	} {
		home := shopHomeVnum(w.Zones, tc.shop)
		for _, z := range w.Zones {
			got := writtenUnder(w.Zones, z, home)
			if want := z == tc.want; got != want {
				t.Errorf("shop #%d under zone %d: got %v, want %v", tc.shop.Vnum, z.Vnum, got, want)
			}
		}
	}
}

// TestShopOrderSurvivesTheRoundTrip is the reason the placement matters.
//
// Shop order is shop_index[]'s order in the C, and it is not inert: it
// decides which shop shop_keeper (shop.c) finds first when one mobile keeps
// two, and which row `show shops <n>` numbers as n. Zone files load in vnum
// order, so a shop written under the wrong zone comes back in the wrong
// place — and because the records all survived, nothing was missing to
// report. `dlctl import --verify` on the archive saw it only as 200 lines
// of field-by-field mismatch between two shop lists that were merely out of
// step with each other.
func TestShopOrderSurvivesTheRoundTrip(t *testing.T) {
	w := handNumberedShopWorld()
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

	var got, want []game.ShopVnum
	for _, sh := range back.Shops {
		got = append(got, sh.Vnum)
	}
	for _, sh := range w.Shops {
		want = append(want, sh.Vnum)
	}
	if len(got) != len(want) {
		t.Fatalf("shops: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("shop order: got %v, want %v", got, want)
		}
	}
}
