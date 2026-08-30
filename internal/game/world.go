// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// RoomDef is a room as the world files describe it: the prototype, before any
// runtime state (who is in it, what is lying on the floor) attaches to it.
type RoomDef struct {
	Vnum        RoomVnum
	Zone        ZoneVnum // the zone whose vnum range contains Vnum
	Name        string
	Description string
	Flags       RoomFlags
	SectorType  int32

	// Exits is indexed by Direction. A nil entry means no exit that way.
	Exits [NumDirections]*ExitDef

	ExtraDescs []ExtraDesc
	// Spec is the name of the special procedure attached to this room, or
	// "". Set at boot from the assignment table, not read from the file.
	Spec string
}

// ExitDef is one direction out of a room.
type ExitDef struct {
	// Description is shown on `look <direction>`.
	Description string
	// Keywords name the door for open/close/lock.
	Keywords string

	// DoorFlag is the raw 0/1/2 from the file: 0 no door, 1 door, 2 pickproof
	// door. The C loader maps this to EX_ISDOOR/EX_PICKPROOF immediately; the
	// raw value is kept so a writer can round-trip it exactly.
	DoorFlag int32

	// Key is the vnum of the key object, or -1 for none.
	Key ObjVnum
	// ToRoom is the destination vnum, or NoRoom.
	ToRoom RoomVnum

	// State is the runtime door state — the C's exit_info. It lives on the
	// definition because the C's world[] *is* the live world and a door
	// closed by a player stays closed until a zone reset opens it. Derived
	// from DoorFlag at load; changed thereafter by play and by 'D' reset
	// commands.
	State ExitFlags
}

// DoorState returns the initial exit_info for a raw DoorFlag, porting the
// mapping the C loader does at load time: 1 is a door, 2 a pickproof one.
func DoorState(doorFlag int32) ExitFlags {
	switch doorFlag {
	case 1:
		return NewSet(ExitIsDoor)
	case 2:
		return NewSet(ExitIsDoor, ExitPickproof)
	}
	return ExitFlags{}
}

// IsDoor reports whether this exit has a door.
func (e *ExitDef) IsDoor() bool { return e.DoorFlag == 1 || e.DoorFlag == 2 }

// IsPickproof reports whether the door resists picking.
func (e *ExitDef) IsPickproof() bool { return e.DoorFlag == 2 }

// MobDef is a mobile prototype.
type MobDef struct {
	Vnum MobVnum

	// Keywords is what a player types to refer to it ("puff dragon fractal").
	Keywords string
	// ShortDesc appears inline in sentences ("Puff").
	ShortDesc string
	// LongDesc is the line shown when it is standing in a room.
	LongDesc string
	// Description is shown on `look at`.
	Description string

	ActionFlags    MobFlags
	AffectionFlags Flags
	Alignment      int32

	// Enhanced is true for the 'E' mob format, which carries a trailing block
	// of `Key: value` specials; false for plain 'S'.
	Enhanced bool

	Level int32
	// Thac0 is the raw file value. The C loader converts it to a hitroll as
	// `20 - thac0` at load time; keeping the file value means a writer can
	// reproduce the file rather than a lossy round-trip of the derived value.
	Thac0 int32
	// ArmorClass is the raw file value; the C loader multiplies it by 10.
	ArmorClass int32

	HitDice    Dice // max hitpoints, as NdS+B
	DamageDice Dice

	Gold int32
	Exp  int32

	Position        int32
	DefaultPosition int32
	Sex             int32

	// Especs are the `Key: value` lines of an enhanced mob, in file order.
	// They are kept raw here: interpreting them needs the ability tables,
	// which belong to a later phase.
	Especs []Espec
	// Spec is the name of the special procedure attached to this prototype,
	// or "". Set at boot from the assignment table.
	Spec string
}

// MobIsNPC is the action flag the loader sets on every mobile, whether or not
// the file lists it. structs.h calls it "(R) Automatically set on all Mobs"
// and parse_mobile() does exactly that. It has to be set here too, or every
// mob whose file omits it would differ from the C server's view of it.
const MobIsNPC MobFlag = 3

// Espec is one `Key: value` line from an enhanced mob's E section.
type Espec struct {
	Key   string
	Value string
}

// HitRoll returns the derived hitroll the C loader computes from Thac0.
func (m *MobDef) HitRoll() int32 { return 20 - m.Thac0 }

// ArmorClassScaled returns the internal armor class, which the C loader
// stores as ten times the file value.
func (m *MobDef) ArmorClassScaled() int32 { return 10 * m.ArmorClass }

// Start rooms, from config.c:171. Compile-time constants in the C, and they
// belong in the config file that the plan's §9.1 describes; until that exists
// they live here with their provenance attached. Word of recall sends people
// to the mortal one.
const (
	MortalStartRoom RoomVnum = 3001
	ImmortStartRoom RoomVnum = 1204
	FrozenStartRoom RoomVnum = 1202
)

// CircleMUDVersion is circlemud_version (constants.c:17), which this tree
// never changed: it still names the base it was patched from.
const CircleMUDVersion = "CircleMUD, version 3.00 beta patchlevel 19"

// NumObjValues is the number of value slots an object carries. The C
// NUM_OBJ_VAL_POSITIONS is 4 and the file format writes exactly four.
const NumObjValues = 4

// MaxObjAffects is the number of `A` affect slots an object may carry,
// matching the C MAX_OBJ_AFFECT.
const MaxObjAffects = 6

// ObjDef is an object prototype.
type ObjDef struct {
	Vnum ObjVnum

	Keywords    string
	ShortDesc   string
	Description string
	// ActionDesc is shown when the object is used; often empty.
	ActionDesc string

	Type       int32
	ExtraFlags ExtraFlagSet
	WearFlags  WearFlagSet
	// PermAffect is the bitvector of affects worn objects confer. Read as a
	// plain integer by the C loader, not as letter flags.
	PermAffect int32

	Values [NumObjValues]int32

	Weight int32
	Cost   int32
	// RentPerDay is the daily cost of leaving this in a rent room.
	RentPerDay int32
	MinLevel   int32

	Affects    []ObjAffect
	ExtraDescs []ExtraDesc
	// Spec is the name of the special procedure attached to this prototype,
	// or "". Set at boot from the assignment table.
	Spec string
}

// ObjAffect is one `A` line: an apply location and its modifier.
type ObjAffect struct {
	Location int32
	Modifier int32
}

// ZoneDef is a zone: a vnum range, a reset schedule, and the list of commands
// that repopulate it.
type ZoneDef struct {
	Vnum ZoneVnum
	Name string

	// Bottom and Top bound the vnum range this zone owns, inclusive.
	Bottom RoomVnum
	Top    RoomVnum

	// Lifespan is how many minutes between reset attempts.
	Lifespan int32
	// ResetMode: 0 never, 1 only when empty of players, 2 always.
	ResetMode int32

	Commands []ResetCommand
}

// ResetCommand is one line of a zone's reset script.
type ResetCommand struct {
	// Command is the single-letter opcode: M, O, G, E, P, D, R.
	Command byte

	// IfFlag makes execution conditional on the previous command having
	// succeeded.
	IfFlag int32

	Arg1 int32
	Arg2 int32
	// Arg3 is only meaningful for the four-argument commands; see NumArgs.
	Arg3 int32

	// Line is the 1-based line number in the source file, for error messages.
	Line int
}

// fourArgCommands are the reset opcodes that take a third argument. The C
// loader tests `strchr("MOEPD", cmd)`; G and R take two.
const fourArgCommands = "MOEPD"

// NumArgs returns how many arguments this command's opcode takes: 3 or 2,
// excluding the if-flag.
func (c ResetCommand) NumArgs() int {
	for i := 0; i < len(fourArgCommands); i++ {
		if fourArgCommands[i] == c.Command {
			return 3
		}
	}
	return 2
}

// World is a fully loaded world.
type World struct {
	Rooms   []*RoomDef
	Mobiles []*MobDef
	Objects []*ObjDef
	Zones   []*ZoneDef
	Shops   []*ShopDef
}
