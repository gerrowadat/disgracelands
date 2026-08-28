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

// writeReloadableObject writes a minimal yaml world directory holding one
// zone, one room and one object at testSwordVnum — the object counterpart
// to writeReloadableWorld's mobile (reloadmob_test.go). testSwordVnum has
// to fall inside the zone's own range, the same reason writeReloadableWorld
// picks its zone the way it does.
func writeReloadableObject(t *testing.T, dir string, obj *game.ObjDef) {
	t.Helper()
	zone := &game.ZoneDef{Vnum: 1, Name: "Test Zone", Bottom: 0, Top: 199, Lifespan: 15}
	w := &game.World{
		Zones:   []*game.ZoneDef{zone},
		Rooms:   []*game.RoomDef{{Vnum: 1, Name: "A Room"}},
		Objects: []*game.ObjDef{obj},
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

// TestReloadObjCommandEndToEnd: the on-disk definition's changes reach the
// running prototype through the live `reloadobj` command — proving the
// wiring (Context.ObjectReload, Server.ReloadObject, the world re-open),
// not just game.Live.ReloadObject's own unit-level guarantees. An
// already-spawned instance keeps its own copy, the same as
// game.Live.ReloadObject's own doc comment promises.
func TestReloadObjCommandEndToEnd(t *testing.T) {
	srv, _ := newTestServer(t)

	dir := t.TempDir()
	writeReloadableObject(t, dir, &game.ObjDef{
		Vnum: testSwordVnum, Keywords: "sword long", ShortDesc: "the reloaded sword",
		Description: "A reloaded sword is lying here.",
		Type:        game.ItemWeapon,
		WearFlags:   game.ItemWearTake | game.ItemWearWield,
		Weight:      10,
		Cost:        200,
		Values:      [game.NumObjValues]int32{0, 3, 8, 3},
	})
	srv.worldDir = dir

	addr := listening(t, srv)
	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelGreaterGod)

	var existing *game.Object
	inWorld(t, srv, func(w *game.Live) {
		existing = w.NewObject(testSwordVnum)
	})
	if existing == nil {
		t.Fatal("NewObject returned nil")
	}

	god.send("reloadobj 100")
	god.expect("Reloaded object #100")

	var def *game.ObjDef
	inWorld(t, srv, func(w *game.Live) { def = w.ObjectDef(testSwordVnum) })
	if def.ShortDesc != "the reloaded sword" {
		t.Errorf("the object prototype's ShortDesc = %q, want the reloaded text", def.ShortDesc)
	}
	if def.Cost != 200 {
		t.Errorf("the object prototype's Cost = %d, want 200", def.Cost)
	}

	// The already-spawned instance keeps what it had — reloading a
	// prototype must not silently rewrite something a player might
	// already be carrying enchanted, renamed or otherwise changed.
	if existing.ShortDesc != "a long sword" {
		t.Errorf("an existing instance's ShortDesc = %q, want the original — reload must not touch live instances", existing.ShortDesc)
	}
}

// TestReloadObjCommandUnknownVnumIsRefused.
func TestReloadObjCommandUnknownVnumIsRefused(t *testing.T) {
	srv, _ := newTestServer(t)
	dir := t.TempDir()
	writeReloadableObject(t, dir, &game.ObjDef{Vnum: testSwordVnum, ShortDesc: "a long sword"})
	srv.worldDir = dir

	addr := listening(t, srv)
	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelGreaterGod)

	god.send("reloadobj 987654")
	god.expect("Could not reload object #987654")
}
