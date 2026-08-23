// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// realWorldDir is the world that ships in this repo: stock CircleMUD 3.0
// bpl20's lib/world, the data the C tree in reference/ was built against.
// Parsing it is the test that matters: hand-written fixtures only prove the
// parser handles what its author thought of.
const realWorldDir = "../../../../examples/stock/binary/world"

// Record counts for the shipped world.
//
// These are what the loader produces, not what a boot log says: the C
// server's count_hash_records() counts every line beginning with '#' to size
// a malloc, including '#' lines inside descriptions, so it can allocate more
// slots than it fills. That the two loaders agree on the records themselves
// is scripts/world-parity.sh's job, and it runs in CI.
const (
	wantRooms   = 1878
	wantMobiles = 569
	wantObjects = 679
	wantZones   = 30
)

func loadRealWorld(t *testing.T) (*game.World, []Warning) {
	t.Helper()
	src, err := New(world.Config{Dir: realWorldDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = src.Close() })

	w, warnings, err := src.LoadWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("loading %s: %v", realWorldDir, err)
	}
	return w, warnings
}

func TestLoadRealWorldRecordCounts(t *testing.T) {
	w, _ := loadRealWorld(t)

	for _, tc := range []struct {
		what      string
		got, want int
	}{
		{"rooms", len(w.Rooms), wantRooms},
		{"mobiles", len(w.Mobiles), wantMobiles},
		{"objects", len(w.Objects), wantObjects},
		{"zones", len(w.Zones), wantZones},
	} {
		if tc.got != tc.want {
			t.Errorf("loaded %d %s, want %d", tc.got, tc.what, tc.want)
		}
	}
}

func TestRealWorldHasNoErrors(t *testing.T) {
	// Warnings are expected — the world has been edited by many hands over
	// seven years — but an Error means the C server would refuse to boot, and
	// then the two loaders cannot be compared at all.
	_, warnings := loadRealWorld(t)
	for _, w := range warnings {
		if w.Severity == Error {
			t.Errorf("error-level finding in the shipped world: %s", w.Message)
		}
	}
}

func TestRealWorldLoadsOnlyWhatTheIndexLists(t *testing.T) {
	// The index is the file list; anything beside it is never opened, which
	// is a real trap for a builder who adds a zone and forgets the index.
	// Stock ships no such file, so what is asserted here is that the shipped
	// world is clean — the loader's reporting of orphans is covered by
	// fixtures in TestOrphanFilesAreReported.
	_, warnings := loadRealWorld(t)
	for _, w := range warnings {
		if strings.Contains(w.Message, "not listed in the index") {
			t.Errorf("the shipped world has an unindexed file: %s", w.Message)
		}
	}
}

func TestRealWorldKnownRoom(t *testing.T) {
	// The Temple of Midgaard, the mortal start room: if this is wrong,
	// everything is.
	w, _ := loadRealWorld(t)

	var temple *game.RoomDef
	for _, r := range w.Rooms {
		if r.Vnum == 3001 {
			temple = r
			break
		}
	}
	if temple == nil {
		t.Fatal("room #3001 (the Temple of Midgaard) was not loaded")
	}
	if !strings.Contains(temple.Name, "Temple Of Midgaard") {
		t.Errorf("room #3001 name = %q", temple.Name)
	}
	if temple.Zone != 30 {
		t.Errorf("room #3001 zone = %d, want 30", temple.Zone)
	}
	exits := 0
	for _, e := range temple.Exits {
		if e != nil {
			exits++
		}
	}
	if exits == 0 {
		t.Error("room #3001 has no exits")
	}
}

func TestRealWorldEveryRoomBelongsToAZone(t *testing.T) {
	// The C loader exits the process if a room falls outside every zone's
	// range, so this must hold for the shipped world.
	w, _ := loadRealWorld(t)
	for _, r := range w.Rooms {
		if r.Zone < 0 {
			t.Errorf("room #%d is in no zone", r.Vnum)
		}
	}
}

func TestNonUTF8BytesSurvive(t *testing.T) {
	// An archived world is not UTF-8, and the parser must not transcode or
	// validate anything, or a byte like this in a live file is silently
	// corrupted. `dlctl convert` is what moves a directory to UTF-8, once.
	dir := writeWorldFixture(t, map[string]string{
		"wld/index": "1.wld\n$\n",
		// Byte 0x92 is a CP1252 curly apostrophe: not valid UTF-8, and the
		// sort of thing an archived world is full of.
		"wld/1.wld": "#1\nThe Smith\x92s Forge~\n   It\x92s warm in here.\n~\n0 0 1\nS\n$\n",
	})

	l := &loader{dir: dir}
	w := &game.World{}
	if err := l.loadFile(w, "wld", "1.wld"); err != nil {
		t.Fatalf("loading the fixture: %v", err)
	}

	// Search by byte, not by rune. strings.ContainsRune(s, 0x92) looks for
	// U+0092, which encodes as the two bytes 0xC2 0x92 in UTF-8, and so can
	// never match the single raw byte actually in the file. That distinction
	// is the whole point of this test.
	found := 0
	for _, r := range w.Rooms {
		for _, s := range roomStrings(r) {
			if strings.IndexByte(s, 0x92) >= 0 {
				found++
			}
		}
	}
	if found != 2 {
		t.Errorf("found byte 0x92 in %d strings, want 2; the parser is altering non-UTF-8 text", found)
	}
}

// roomStrings returns every piece of text a room carries, so a test can check
// all of them without knowing the struct's shape.
func roomStrings(r *game.RoomDef) []string {
	out := []string{r.Name, r.Description}
	for _, e := range r.ExtraDescs {
		out = append(out, e.Keywords, e.Description)
	}
	for _, e := range r.Exits {
		if e != nil {
			out = append(out, e.Keywords, e.Description)
		}
	}
	return out
}

func TestDumpIsDeterministic(t *testing.T) {
	// The dump exists to be diffed, so two runs must be byte-identical.
	w, _ := loadRealWorld(t)

	var a, b strings.Builder
	if err := world.WriteDump(&a, world.BuildDump(w)); err != nil {
		t.Fatalf("WriteDump: %v", err)
	}
	if err := world.WriteDump(&b, world.BuildDump(w)); err != nil {
		t.Fatalf("WriteDump: %v", err)
	}
	if a.String() != b.String() {
		t.Error("two dumps of the same world differ")
	}
	if a.Len() == 0 {
		t.Error("dump is empty")
	}
}

func TestDumpCountsMatchTheWorld(t *testing.T) {
	w, _ := loadRealWorld(t)
	d := world.BuildDump(w)
	if d.Counts.Rooms != len(w.Rooms) || len(d.Rooms) != len(w.Rooms) {
		t.Errorf("dump has %d rooms in counts and %d in the array, world has %d",
			d.Counts.Rooms, len(d.Rooms), len(w.Rooms))
	}
	if d.Counts.Zones != wantZones {
		t.Errorf("dump zone count = %d, want %d", d.Counts.Zones, wantZones)
	}
}

// writeWorldFixture builds a minimal world directory from a map of relative
// path to contents, filling in an empty index for every subdirectory the
// caller did not name. The loader reads all five kinds on every load, so a
// fixture that omits one fails on the missing index rather than on its point.
func writeWorldFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	for _, sub := range []string{"wld", "mob", "obj", "zon", "shp"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, named := files[sub+"/index"]; !named {
			files[sub+"/index"] = "$\n"
		}
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestOrphanFilesAreReported covers what the shipped world no longer has an
// example of: a data file sitting in a world directory that no index lists.
// The C server never opens one, so a builder who adds a zone and forgets the
// index gets no error and a world quietly missing their work.
func TestOrphanFilesAreReported(t *testing.T) {
	dir := writeWorldFixture(t, map[string]string{
		"wld/index": "1.wld\n$\n",
		"wld/1.wld": "#1\nA Room~\n   A room.\n~\n0 0 1\nS\n$\n",
		"wld/2.wld": "#2\nAn Orphan~\n   Never loaded.\n~\n0 0 1\nS\n$\n",
	})

	src, err := New(world.Config{Dir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w, warnings, err := src.LoadWithWarnings(context.Background())
	if err != nil {
		t.Fatalf("loading the fixture: %v", err)
	}

	found := false
	for _, warn := range warnings {
		if strings.Contains(warn.Message, "not listed in the index") {
			found = true
		}
	}
	if !found {
		t.Error("the unindexed file was not reported")
	}
	for _, r := range w.Rooms {
		if r.Vnum == 2 {
			t.Error("room #2 was loaded, but 2.wld is not in the index")
		}
	}
}
