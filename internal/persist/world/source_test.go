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

// resolvableWorld is a world whose reset commands all point at records that
// exist, so nothing is disabled by resolution.
func resolvableWorld(cmds ...game.ResetCommand) *game.World {
	return &game.World{
		Rooms:   []*game.RoomDef{{Vnum: 101}},
		Mobiles: []*game.MobDef{{Vnum: 100}},
		Objects: []*game.ObjDef{{Vnum: 200}},
		Zones:   []*game.ZoneDef{{Vnum: 1, Commands: cmds}},
	}
}

func TestDumpOmitsArg3ForTwoArgumentCommands(t *testing.T) {
	// The C loader leaves arg3 uninitialised for G and R, so dumping a zero
	// there would assert something the source data does not say.
	d := world.BuildDump(resolvableWorld(
		game.ResetCommand{Command: 'M', Arg1: 100, Arg2: 1, Arg3: 101},
		game.ResetCommand{Command: 'G', Arg1: 200, Arg2: 1},
	))

	cmds := d.Zones[0].Commands
	if cmds[0].Arg3 == nil || *cmds[0].Arg3 != 101 {
		t.Errorf("M command Arg3 = %v, want 101", cmds[0].Arg3)
	}
	if cmds[1].Arg3 != nil {
		t.Errorf("G command Arg3 = %v, want nil", *cmds[1].Arg3)
	}
}

func TestDumpDisablesUnresolvableResetCommands(t *testing.T) {
	// renum_zone_table() rewrites the opcode to '*' when a vnum does not
	// resolve, which permanently disables the command. A dump that showed the
	// original opcode would claim the server does something it does not.
	d := world.BuildDump(resolvableWorld(
		game.ResetCommand{Command: 'M', Arg1: 100, Arg2: 1, Arg3: 101}, // fine
		game.ResetCommand{Command: 'M', Arg1: 999, Arg2: 1, Arg3: 101}, // no such mob
	))

	cmds := d.Zones[0].Commands
	if cmds[0].Disabled {
		t.Error("a command with resolvable vnums was disabled")
	}
	if !cmds[1].Disabled {
		t.Fatal("a command loading a mob that does not exist was not disabled")
	}
	if cmds[1].Command != "*" {
		t.Errorf("disabled command opcode = %q, want %q", cmds[1].Command, "*")
	}
	// Its arguments are gone in the C server, so claiming to know them would
	// be a lie that shows up as a spurious diff.
	if cmds[1].Arg1 != nil || cmds[1].Arg2 != nil || cmds[1].Arg3 != nil {
		t.Errorf("disabled command kept its arguments: %+v", cmds[1])
	}
}

func TestDumpResolvesExitsToNowhere(t *testing.T) {
	// An exit whose destination does not exist is NOWHERE in the running
	// server; the file's vnum is not recoverable from it.
	rooms := []*game.RoomDef{{Vnum: 1}, {Vnum: 2}}
	rooms[0].Exits[game.North] = &game.ExitDef{ToRoom: 2}   // exists
	rooms[0].Exits[game.South] = &game.ExitDef{ToRoom: 999} // does not

	d := world.BuildDump(&game.World{Rooms: rooms})
	exits := d.Rooms[0].Exits

	if exits[game.North].ToRoom != 2 {
		t.Errorf("north exit ToRoom = %d, want 2", exits[game.North].ToRoom)
	}
	if exits[game.South].ToRoom != game.NoRoom {
		t.Errorf("south exit ToRoom = %d, want %d (unresolvable)", exits[game.South].ToRoom, game.NoRoom)
	}
}

func TestParityModeOmitsFieldsTheCServerDiscards(t *testing.T) {
	// parse_enhanced_mob() consumes the E block without recording that it saw
	// one, and interpret_espec() folds the key/value lines into ordinary
	// fields and discards them. Comparing either against the C dump could
	// only ever produce noise.
	w := &game.World{Mobiles: []*game.MobDef{{
		Vnum: 1, Enhanced: true,
		Especs: []game.Espec{{Key: "BareHandAttack", Value: "12"}},
	}}}

	full := world.BuildDump(w)
	if full.Mobiles[0].Enhanced == nil || !*full.Mobiles[0].Enhanced {
		t.Error("Enhanced was dropped from a non-parity dump")
	}
	if len(full.Mobiles[0].Especs) != 1 {
		t.Error("Especs were dropped from a non-parity dump")
	}

	parity := world.BuildDumpWithOptions(w, world.Options{Parity: true})
	if parity.Mobiles[0].Enhanced != nil {
		t.Error("Enhanced survived into a parity dump")
	}
	if len(parity.Mobiles[0].Especs) != 0 {
		t.Error("Especs survived into a parity dump")
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
