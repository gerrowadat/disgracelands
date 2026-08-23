// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package server

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/yaml"
)

// writeReloadableShop writes a minimal yaml world directory holding one
// zone, one room and one shop at testShopVnum — the shop counterpart to
// writeReloadableWorld's mobile (reloadmob_test.go). testShopVnum has to
// fall inside the zone's own range, the same reason writeReloadableWorld
// picks its zone the way it does.
func writeReloadableShop(t *testing.T, dir string, shop *game.ShopDef) {
	t.Helper()
	zone := &game.ZoneDef{Vnum: 90, Name: "Test Zone", Bottom: 9000, Top: 9099, Lifespan: 15}
	w := &game.World{
		Zones: []*game.ZoneDef{zone},
		Rooms: []*game.RoomDef{{Vnum: 9000, Name: "A Room"}},
		Shops: []*game.ShopDef{shop},
	}
	if err := yaml.WriteManifest(dir, []yaml.ManifestEntry{{Vnum: int32(zone.Vnum), Enabled: true}}); err != nil {
		t.Fatalf("writing zones.yaml: %v", err)
	}
	src, err := yaml.New(world.Config{Dir: dir})
	if err != nil {
		t.Fatalf("yaml.New: %v", err)
	}
	defer func() { _ = src.Close() }()
	if err := src.WriteZone(context.Background(), zone, w); err != nil {
		t.Fatalf("WriteZone: %v", err)
	}
}

// TestReloadShopCommandEndToEnd: the on-disk configuration's changes reach
// the running shop through the live `reloadshop` command — proving the
// wiring (Context.ShopReload, Server.ReloadShop, the world re-open), not
// just game.Live.ReloadShop's own unit-level guarantees.
func TestReloadShopCommandEndToEnd(t *testing.T) {
	srv, _ := newTestServer(t)

	dir := t.TempDir()
	writeReloadableShop(t, dir, &game.ShopDef{
		Vnum:       testShopVnum,
		Keeper:     testShopkeeperVnum,
		Producing:  []game.ObjVnum{testSwordVnum},
		ProfitBuy:  2.0,
		ProfitSell: 0.5,
		Rooms:      []game.RoomVnum{ShopRoom},
	})
	srv.worldFormat, srv.worldDir = "yaml", dir

	addr := listening(t, srv)
	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelGreaterGod)

	god.send("reloadshop 9001")
	god.expect("Reloaded shop #9001")

	var shop *game.ShopDef
	inWorld(t, srv, func(w *game.Live) { shop = w.Shop(testShopVnum) })
	if shop.ProfitBuy != 2.0 {
		t.Errorf("the shop's ProfitBuy = %v, want 2.0", shop.ProfitBuy)
	}
	if shop.ProfitSell != 0.5 {
		t.Errorf("the shop's ProfitSell = %v, want 0.5", shop.ProfitSell)
	}
}

// TestReloadShopCommandUnknownVnumIsRefused.
func TestReloadShopCommandUnknownVnumIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	dir := t.TempDir()
	writeReloadableShop(t, dir, &game.ShopDef{Vnum: testShopVnum, Keeper: testShopkeeperVnum})
	srv.worldFormat, srv.worldDir = "yaml", dir

	addr := listening(t, srv)
	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelGreaterGod)

	god.send("reloadshop 987654")
	god.expect("Could not reload shop #987654")
}
