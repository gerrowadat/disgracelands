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

// writeReloadableZone writes a yaml world directory holding testWorld()'s
// own zone 30 (Midgaard, 3000-3099) with room 3001 (MortalStartRoom)
// changed — the room-level counterpart to writeReloadableWorld's mobile.
// Zone 30 and room 3001 both already exist in testWorld(), which is what
// ReloadZone needs: it updates what exists, it does not create what is new.
func writeReloadableZone(t *testing.T, dir, roomName string) {
	t.Helper()
	zone := &game.ZoneDef{Vnum: 30, Name: "Midgaard Reloaded", Bottom: 3000, Top: 3099, Lifespan: 20}
	w := &game.World{
		Zones: []*game.ZoneDef{zone},
		// Zone has to be set explicitly: yaml.WriteZone files a room
		// under the zone this field names, not by re-deriving it from
		// the vnum range (RoomDef.Zone's own doc comment says the C
		// sets it at load time the same way; a fresh definition has to
		// carry it too).
		Rooms: []*game.RoomDef{{Vnum: 3001, Zone: 30, Name: roomName}},
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

// TestReloadZoneCommandEndToEnd: the on-disk zone's changed room reaches
// the running world through the live `reloadzone` command — proving the
// wiring (Context.ZoneReload, Server.ReloadZone, the world re-open), not
// just game.Live.ReloadZone's own unit-level guarantees.
func TestReloadZoneCommandEndToEnd(t *testing.T) {
	srv, _ := newTestServer(t)

	dir := t.TempDir()
	writeReloadableZone(t, dir, "The Reloaded Temple")
	srv.worldDir = dir

	addr := listening(t, srv)
	// The first character on the roster is an implementor and wakes in
	// the immortal room (visibility_test.go's twoInARoom's own doc
	// comment) — nobody is in Midgaard yet.
	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelGreaterGod)

	god.send("reloadzone 30")
	god.expect("Reloaded zone #30")

	inWorld(t, srv, func(w *game.Live) {
		if got := w.Room(MortalStartRoom).Name; got != "The Reloaded Temple" {
			t.Errorf("room %d's name = %q, want the reloaded text", MortalStartRoom, got)
		}
	})
}

// TestReloadZoneCommandRefusesWithAPlayerPresent: a second, mortal
// character wakes in the temple by default (every character but the
// first does) — reloadzone refuses while they are there, and applies
// nothing.
func TestReloadZoneCommandRefusesWithAPlayerPresent(t *testing.T) {
	srv, _ := newTestServer(t)

	dir := t.TempDir()
	writeReloadableZone(t, dir, "The Reloaded Temple")
	srv.worldDir = dir

	addr := listening(t, srv)
	god := dialClient(t, addr)
	god.create("Zod", "swordfish", "m", "w")
	setLevel(t, srv, "Zod", game.LevelGreaterGod)

	mortal := dialClient(t, addr)
	mortal.create("Bystander", "swordfish", "f", "w")

	god.send("reloadzone 30")
	god.expect("has a player in it")

	inWorld(t, srv, func(w *game.Live) {
		if got := w.Room(MortalStartRoom).Name; got == "The Reloaded Temple" {
			t.Error("reloadzone applied despite a player standing in the zone")
		}
	})
}
