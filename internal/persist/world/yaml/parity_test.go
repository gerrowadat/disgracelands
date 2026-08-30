// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/classic"
)

// TestClassicYamlParity is §11 step 2/3's acceptance bar: import the real
// data/world through classic, write it out as yaml, load that back, and
// check the two loaders produced the same world — via the same parity dump
// scripts/world-parity.sh uses to compare the Go and C loaders, applied
// here to compare classic and yaml within the Go side.
func TestClassicYamlParity(t *testing.T) {
	src, err := classic.New(world.Config{Dir: "../../../../examples/stock/binary/world"})
	if err != nil {
		t.Fatalf("open classic source: %v", err)
	}
	defer func() { _ = src.Close() }()

	cw, classicFindings, err := src.LoadWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("classic load: %v", err)
	}
	// What the *source* directory already says about itself, so that the
	// assertion below is about the conversion rather than about the world.
	// examples/stock has a room whose north exit is locked by an object
	// that does not exist (#12038, key #12104); since #286 both loaders
	// report it, because it is an observation about a loaded world rather
	// than about the file it came from. A yaml load repeating what the
	// classic load already said is the two loaders agreeing, which is what
	// this test is for.
	said := map[string]bool{}
	for _, wr := range classicFindings {
		said[wr.Message] = true
	}

	dir := t.TempDir()
	writeImportedManifest(t, dir, cw)

	nsrc, err := New(world.Config{Dir: dir})
	if err != nil {
		t.Fatalf("open yaml source: %v", err)
	}
	defer func() { _ = nsrc.Close() }()

	for _, z := range cw.Zones {
		if err := nsrc.WriteZone(context.Background(), z, cw); err != nil {
			t.Fatalf("write zone %d: %v", z.Vnum, err)
		}
	}

	nw, warnings, err := nsrc.LoadWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("yaml load: %v", err)
	}
	for _, wr := range warnings {
		if wr.Severity >= world.Warn && !said[wr.Message] {
			t.Errorf("yaml load finding the classic load did not have: %s", wr)
		}
	}

	// Nothing is normalised before comparing any more. This test used to
	// pre-trim cw's trailing blank lines to match what the format could
	// represent; the format represents them now (text.go's needsQuoting),
	// so the comparison is between the two worlds exactly as each loader
	// produced them.

	cDump := world.BuildDumpWithOptions(cw, world.Options{Parity: true})
	nDump := world.BuildDumpWithOptions(nw, world.Options{Parity: true})

	if len(cDump.Rooms) != len(nDump.Rooms) {
		t.Fatalf("room count: classic %d, yaml %d", len(cDump.Rooms), len(nDump.Rooms))
	}
	for i := range cDump.Rooms {
		compareJSON(t, fmt.Sprintf("room #%d", cDump.Rooms[i].Vnum), cDump.Rooms[i], nDump.Rooms[i])
	}
	if len(cDump.Mobiles) != len(nDump.Mobiles) {
		t.Fatalf("mobile count: classic %d, yaml %d", len(cDump.Mobiles), len(nDump.Mobiles))
	}
	for i := range cDump.Mobiles {
		compareJSON(t, fmt.Sprintf("mobile #%d", cDump.Mobiles[i].Vnum), cDump.Mobiles[i], nDump.Mobiles[i])
	}
	if len(cDump.Objects) != len(nDump.Objects) {
		t.Fatalf("object count: classic %d, yaml %d", len(cDump.Objects), len(nDump.Objects))
	}
	for i := range cDump.Objects {
		compareJSON(t, fmt.Sprintf("object #%d", cDump.Objects[i].Vnum), cDump.Objects[i], nDump.Objects[i])
	}
	if len(cDump.Shops) != len(nDump.Shops) {
		t.Fatalf("shop count: classic %d, yaml %d", len(cDump.Shops), len(nDump.Shops))
	}
	for i := range cDump.Shops {
		compareJSON(t, fmt.Sprintf("shop #%d", cDump.Shops[i].Vnum), cDump.Shops[i], nDump.Shops[i])
	}
	if len(cDump.Zones) != len(nDump.Zones) {
		t.Fatalf("zone count: classic %d, yaml %d", len(cDump.Zones), len(nDump.Zones))
	}
	for i := range cDump.Zones {
		compareJSON(t, fmt.Sprintf("zone #%d", cDump.Zones[i].Vnum), cDump.Zones[i], nDump.Zones[i])
	}
}

// compareJSON marshals both values (each field on its own line, so a
// mismatch names the exact field) and reports every line that differs.
func compareJSON(t *testing.T, label string, want, got any) {
	t.Helper()
	wj, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("%s: marshal classic: %v", label, err)
	}
	gj, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("%s: marshal yaml: %v", label, err)
	}
	if bytes.Equal(wj, gj) {
		return
	}
	wl, gl := splitLines(string(wj)), splitLines(string(gj))
	max := len(wl)
	if len(gl) > max {
		max = len(gl)
	}
	for i := 0; i < max; i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			t.Errorf("%s, line %d:\n classic: %s\n yaml:  %s", label, i+1, w, g)
		}
	}
}

// writeImportedManifest builds zones.yaml the way `dlctl import --type=world`
// will: every zone the source loaded, enabled.
func writeImportedManifest(t *testing.T, dir string, w *game.World) {
	t.Helper()
	doc := manifestDoc{Schema: ManifestSchema}
	for _, z := range w.Zones {
		doc.Zones = append(doc.Zones, ManifestEntry{Vnum: int32(z.Vnum), Enabled: true})
	}
	if err := writeManifest(dir, doc); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
