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

// writeReloadableWorld writes a minimal yaml world directory holding one
// zone, one room and one mobile at testDogVnum — self-contained scaffolding
// a reloadmob test can point Server.worldDir at, independent of testWorld()'s
// own in-memory fixture (which has no on-disk form at all). The mobile's own
// fields are the caller's to set, so a test can write a "changed" version
// straight off rather than writing once and editing.
func writeReloadableWorld(t *testing.T, dir string, mob *game.MobDef) {
	t.Helper()
	// testDogVnum (999) has to fall inside the zone's own range: yaml
	// writes "every room, mobile, object and shop whose vnum falls in
	// [Bottom, Top]" into that zone's file (data-format.md §3) — a range
	// that does not cover it silently drops the mob from the write.
	zone := &game.ZoneDef{Vnum: 9, Name: "Test Zone", Bottom: 900, Top: 999, Lifespan: 15}
	w := &game.World{
		Zones:   []*game.ZoneDef{zone},
		Rooms:   []*game.RoomDef{{Vnum: 901, Name: "A Room"}},
		Mobiles: []*game.MobDef{mob},
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

// TestReloadMobCommandEndToEnd: the on-disk definition's changes reach an
// already-spawned, unengaged instance through the live `reloadmob` command
// — proving the wiring (Context.MobReload, Server.ReloadMobile, the world
// re-open), not just game.Live.ReloadMobile's own unit-level guarantees.
func TestReloadMobCommandEndToEnd(t *testing.T) {
	srv, _ := newTestServer(t)

	dir := t.TempDir()
	writeReloadableWorld(t, dir, &game.MobDef{
		Vnum: testDogVnum, Keywords: "dog", ShortDesc: "the reloaded dog",
		LongDesc: "A reloaded dog sits here.\r\n", Level: 5,
		HitDice:  game.Dice{Number: 1, Size: 1, Bonus: 200},
		Position: int32(game.PosStanding), DefaultPosition: int32(game.PosStanding),
	})
	srv.worldFormat, srv.worldDir = "yaml", dir

	addr := listening(t, srv)
	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelGreaterGod)

	var dog *game.Character
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		dog = w.SpawnMobile(testDogVnum, MortalStartRoom, testRNG())
	}); err != nil {
		t.Fatal(err)
	}
	if dog == nil {
		t.Fatal("SpawnMobile returned nil")
	}

	god.send("reloadmob 999")
	god.expect("Reloaded mob #999")

	var longDesc string
	var maxHit int32
	inWorld(t, srv, func(_ *game.Live) {
		longDesc = dog.MobDef.LongDesc
		maxHit = dog.Record.Points.MaxHit
	})
	if longDesc != "A reloaded dog sits here.\r\n" {
		t.Errorf("dog.MobDef.LongDesc = %q, want the reloaded text", longDesc)
	}
	if maxHit < 200+1 {
		t.Errorf("dog.Record.Points.MaxHit = %d, want at least 201 (the new hit dice's floor)", maxHit)
	}
}

// TestReloadMobCommandRefusesWhileFighting: the same command, against an
// engaged instance, changes nothing and says so.
func TestReloadMobCommandRefusesWhileFighting(t *testing.T) {
	srv, _ := newTestServer(t)

	dir := t.TempDir()
	writeReloadableWorld(t, dir, &game.MobDef{
		Vnum: testDogVnum, Keywords: "dog", ShortDesc: "the reloaded dog",
		LongDesc: "A reloaded dog sits here.\r\n", Level: 5,
		HitDice:  game.Dice{Number: 1, Size: 1, Bonus: 200},
		Position: int32(game.PosStanding), DefaultPosition: int32(game.PosStanding),
	})
	srv.worldFormat, srv.worldDir = "yaml", dir

	addr := listening(t, srv)
	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelGreaterGod)

	var dog *game.Character
	if err := srv.engine.DoSync(context.Background(), func(w *game.Live) {
		dog = w.SpawnMobile(testDogVnum, MortalStartRoom, testRNG())
		zod := w.Find("Zod")
		dog.Fighting = zod
	}); err != nil {
		t.Fatal(err)
	}

	god.send("reloadmob 999")
	god.expect("is in combat")

	inWorld(t, srv, func(_ *game.Live) {
		if dog.MobDef.LongDesc == "A reloaded dog sits here.\r\n" {
			t.Error("reloadmob applied despite the instance fighting")
		}
	})
}
