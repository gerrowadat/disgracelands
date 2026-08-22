// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/classic"
)

// TestClassicNativeParity is §11 step 2/3's acceptance bar: import the real
// data/world through classic, write it out as native, load that back, and
// check the two loaders produced the same world — via the same parity dump
// scripts/world-parity.sh uses to compare the Go and C loaders, applied
// here to compare classic and native within the Go side.
func TestClassicNativeParity(t *testing.T) {
	src, err := classic.New(world.Config{Dir: "../../../../data/world"})
	if err != nil {
		t.Fatalf("open classic source: %v", err)
	}
	defer func() { _ = src.Close() }()

	cw, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("classic load: %v", err)
	}

	dir := t.TempDir()
	writeImportedManifest(t, dir, cw)

	nsrc, err := New(world.Config{Dir: dir})
	if err != nil {
		t.Fatalf("open native source: %v", err)
	}
	defer func() { _ = nsrc.Close() }()

	for _, z := range cw.Zones {
		if err := nsrc.WriteZone(context.Background(), z, cw); err != nil {
			t.Fatalf("write zone %d: %v", z.Vnum, err)
		}
	}

	nw, warnings, err := nsrc.LoadWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("native load: %v", err)
	}
	for _, wr := range warnings {
		if wr.Severity >= world.Warn {
			t.Errorf("native load finding: %s", wr)
		}
	}

	// The one documented, accepted lossy transform (TrimsTrailingBlankLines
	// in text.go): a string with 2+ trailing newlines normalises to
	// exactly one. Applied to cw's own fields, after WriteZone already
	// captured the pre-normalisation originals, so the parity comparison
	// below is between two worlds that agree on what the format can
	// actually represent.
	normalizeTrailingBlankLines(cw)

	cDump := world.BuildDumpWithOptions(cw, world.Options{Parity: true})
	nDump := world.BuildDumpWithOptions(nw, world.Options{Parity: true})

	if len(cDump.Rooms) != len(nDump.Rooms) {
		t.Fatalf("room count: classic %d, native %d", len(cDump.Rooms), len(nDump.Rooms))
	}
	for i := range cDump.Rooms {
		compareJSON(t, fmt.Sprintf("room #%d", cDump.Rooms[i].Vnum), cDump.Rooms[i], nDump.Rooms[i])
	}
	if len(cDump.Mobiles) != len(nDump.Mobiles) {
		t.Fatalf("mobile count: classic %d, native %d", len(cDump.Mobiles), len(nDump.Mobiles))
	}
	for i := range cDump.Mobiles {
		compareJSON(t, fmt.Sprintf("mobile #%d", cDump.Mobiles[i].Vnum), cDump.Mobiles[i], nDump.Mobiles[i])
	}
	if len(cDump.Objects) != len(nDump.Objects) {
		t.Fatalf("object count: classic %d, native %d", len(cDump.Objects), len(nDump.Objects))
	}
	for i := range cDump.Objects {
		compareJSON(t, fmt.Sprintf("object #%d", cDump.Objects[i].Vnum), cDump.Objects[i], nDump.Objects[i])
	}
	if len(cDump.Shops) != len(nDump.Shops) {
		t.Fatalf("shop count: classic %d, native %d", len(cDump.Shops), len(nDump.Shops))
	}
	for i := range cDump.Shops {
		compareJSON(t, fmt.Sprintf("shop #%d", cDump.Shops[i].Vnum), cDump.Shops[i], nDump.Shops[i])
	}
	if len(cDump.Zones) != len(nDump.Zones) {
		t.Fatalf("zone count: classic %d, native %d", len(cDump.Zones), len(nDump.Zones))
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
		t.Fatalf("%s: marshal native: %v", label, err)
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
			t.Errorf("%s, line %d:\n classic: %s\n native:  %s", label, i+1, w, g)
		}
	}
}

// writeImportedManifest builds zones.yaml the way `dlctl world import`
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

// normalizeTrailingBlankLines applies the one documented lossy transform
// (text.go's TrimsTrailingBlankLines) to every prose field a zone file
// carries, matching exactly what a value goes through on its way to and
// from a native YAML document.
//
// Text (room/mob/obj top-level fields) always encodes multi-line content
// through literalBlock, so the transform always applies there. NestedText
// (exit and extra-description fields) only goes through literalBlock when
// the content does *not* need an indentation indicator — content that does
// falls back to a quoted scalar instead (see NestedText's doc comment),
// which is lossless and must not be normalised here, or this helper would
// "fix" a field the real round trip never actually touches.
func normalizeTrailingBlankLines(w *game.World) {
	fixAlways := func(s *string) {
		stored := ToStored(*s)
		if TrimsTrailingBlankLines(stored) {
			*s = FromStored(strings.TrimRight(stored, "\n") + "\n")
		}
	}
	fixNested := func(s *string) {
		stored := ToStored(*s)
		if needsIndentIndicator(strings.Split(strings.TrimRight(stored, "\n"), "\n")) {
			return // takes the lossless quoted fallback; nothing to normalise
		}
		fixAlways(s)
	}
	for _, r := range w.Rooms {
		fixAlways(&r.Description)
		for _, e := range r.Exits {
			if e != nil {
				fixNested(&e.Description)
			}
		}
		for i := range r.ExtraDescs {
			fixNested(&r.ExtraDescs[i].Description)
		}
	}
	for _, m := range w.Mobiles {
		fixAlways(&m.LongDesc)
		fixAlways(&m.Description)
	}
	for _, o := range w.Objects {
		fixAlways(&o.Description)
		fixAlways(&o.ActionDesc)
		for i := range o.ExtraDescs {
			fixNested(&o.ExtraDescs[i].Description)
		}
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
