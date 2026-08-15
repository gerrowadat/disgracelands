package world_test

import (
	"context"
	"strings"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"

	// Registers the classic format, which is what makes Open("classic") work
	// without the caller naming the package. This blank import is the whole
	// mechanism the registry exists for.
	_ "github.com/gerrowadat/disgracelands/internal/persist/world/classic"
)

func TestClassicFormatIsRegistered(t *testing.T) {
	found := false
	for _, n := range world.Formats() {
		if n == "classic" {
			found = true
		}
	}
	if !found {
		t.Errorf("Formats() = %v, want it to include \"classic\"", world.Formats())
	}
}

func TestOpenUnknownFormatNamesTheAlternatives(t *testing.T) {
	_, err := world.Open("sqlite", world.Config{Dir: "."})
	if err == nil {
		t.Fatal("Open(\"sqlite\") succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "classic") {
		t.Errorf("error = %q, want it to list the formats that do exist", err)
	}
}

func TestOpenRequiresADirectory(t *testing.T) {
	if _, err := world.Open("classic", world.Config{}); err == nil {
		t.Error("Open with no directory succeeded, want an error")
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("registering a duplicate name did not panic")
		}
	}()
	world.Register("classic", nil)
}

func TestLoadReportsAMissingDirectory(t *testing.T) {
	src, err := world.Open("classic", world.Config{Dir: "does/not/exist"})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = src.Close() }()

	if _, err := src.Load(context.Background()); err == nil {
		t.Error("Load from a missing directory succeeded, want an error")
	}
}

func TestDumpOmitsArg3ForTwoArgumentCommands(t *testing.T) {
	// The C loader leaves arg3 uninitialised for G and R, so dumping a zero
	// there would assert something the source data does not say.
	w := &game.World{Zones: []*game.ZoneDef{{
		Vnum: 1,
		Commands: []game.ResetCommand{
			{Command: 'M', Arg1: 100, Arg2: 1, Arg3: 101},
			{Command: 'G', Arg1: 200, Arg2: 1},
		},
	}}}

	d := world.BuildDump(w)
	cmds := d.Zones[0].Commands
	if cmds[0].Arg3 == nil || *cmds[0].Arg3 != 101 {
		t.Errorf("M command Arg3 = %v, want 101", cmds[0].Arg3)
	}
	if cmds[1].Arg3 != nil {
		t.Errorf("G command Arg3 = %v, want nil", *cmds[1].Arg3)
	}
}

func TestDumpDistinguishesMissingExitsFromExitsToNowhere(t *testing.T) {
	// A diff must not confuse "there is no door here" with "the door leads
	// nowhere", so absent exits are null rather than omitted.
	room := &game.RoomDef{Vnum: 1}
	room.Exits[game.North] = &game.ExitDef{ToRoom: game.NoRoom}

	d := world.BuildDump(&game.World{Rooms: []*game.RoomDef{room}})
	exits := d.Rooms[0].Exits

	if len(exits) != game.NumDirections {
		t.Fatalf("dumped %d exit slots, want %d", len(exits), game.NumDirections)
	}
	if exits[game.North] == nil {
		t.Error("the north exit is null, but the room has one")
	} else if exits[game.North].ToRoom != game.NoRoom {
		t.Errorf("north exit ToRoom = %d, want %d", exits[game.North].ToRoom, game.NoRoom)
	}
	if exits[game.South] != nil {
		t.Error("the south exit is not null, but the room has no south exit")
	}
}

func TestDumpEmptyWorld(t *testing.T) {
	// Empty slices rather than nulls, so a diff against a world that has
	// records shows the records rather than a type change.
	var sb strings.Builder
	if err := world.WriteDump(&sb, world.BuildDump(&game.World{})); err != nil {
		t.Fatalf("WriteDump: %v", err)
	}
	out := sb.String()
	for _, want := range []string{`"rooms": 0`, `"zones": []`} {
		if !strings.Contains(out, want) {
			t.Errorf("dump of an empty world is missing %q:\n%s", want, out)
		}
	}
}
