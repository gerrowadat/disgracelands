package classic

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
)

// realWorldDir is the actual Disgracelands world, which ships in this repo.
// Parsing it is the test that matters: hand-written fixtures only prove the
// parser handles what its author thought of.
const realWorldDir = "../../../../data/world"

// Record counts the C server agrees with.
//
// The C server's boot log reports different numbers — 2988 rooms and 1200
// objects — but those come from count_hash_records(), which counts every line
// beginning with '#' in order to size a malloc, including '#' lines inside
// descriptions. data/world contains seven such lines in room files (ASCII-art
// signs in wld/54.wld and wld/64.wld) and one in an object file
// (obj/142.obj), so the C server allocates 2988 slots and fills 2981, and
// allocates 1200 and fills 1199. Both figures below therefore match what the
// C server actually loads.
const (
	wantRooms   = 2981
	wantMobiles = 944
	wantObjects = 1199
	wantZones   = 47
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

func TestRealWorldOnlyIndexedFilesAreLoaded(t *testing.T) {
	// Six zone files and four each of wld/mob/obj sit in data/world without
	// being listed in any index, so the C server never opens them. Loading
	// them would silently add content the real game does not have.
	_, warnings := loadRealWorld(t)

	found := false
	for _, w := range warnings {
		if strings.Contains(w.Message, "not listed in the index") {
			found = true
		}
	}
	if !found {
		t.Error("no finding about unindexed world files; data/world has several")
	}

	w, _ := loadRealWorld(t)
	for _, z := range w.Zones {
		// 90 and 92 are among the unindexed zones.
		if z.Vnum == 90 || z.Vnum == 92 {
			t.Errorf("zone #%d was loaded, but it is not in zon/index", z.Vnum)
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

func TestRealWorldNonUTF8BytesSurvive(t *testing.T) {
	// data/world is not UTF-8 — wld/90.wld holds byte 0x92, a CP1252 curly
	// apostrophe. That file is unindexed, but the parser must not be
	// transcoding or validating anything regardless, or a future edit that
	// puts such a byte in a live file would silently corrupt it.
	l := &loader{dir: filepath.Clean(realWorldDir)}
	w := &game.World{}
	if err := l.loadFile(w, "wld", "90.wld"); err != nil {
		t.Fatalf("loading the unindexed 90.wld directly: %v", err)
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
	if found != 3 {
		t.Errorf("found byte 0x92 in %d strings, want 3; the parser is altering non-UTF-8 text", found)
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
