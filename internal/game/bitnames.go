// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strings"

// The names the game prints for its bitfields, from constants.c, and the two
// functions that print them.
//
// These are what `identify` shows and what the builders' tools will show. They
// are not decoration: they are the only place several of these bits are
// described anywhere, and a couple of the names are wrong in ways worth
// keeping — AFF_NO_TRACK's slot is followed by an "UNUSED" that is not unused
// at all in this tree.

// affectBitNames are affected_bits[] (constants.c:640). The last five are
// local: no-track, the unused slot, holy shield, sneak, hide, silence and
// charm run past where stock CircleMUD stops.
var affectBitNames = []string{
	"BLIND", "INVIS", "DET-ALIGN", "DET-INVIS", "DET-MAGIC", "SENSE-LIFE",
	"WATWALK", "SANCT", "GROUP", "CURSE", "INFRA", "POISON", "PROT-EVIL",
	"PROT-GOOD", "SLEEP", "NO_TRACK", "UNUSED", "HOLY-SHIELD", "SNEAK",
	"HIDE", "SILENCE", "CHARM",
}

// extraBitNames are extra_bits[] (constants.c:665). NO_LOCATE is local, and
// carries a `/*humbug*/` marker on either side of it in the C.
var extraBitNames = []string{
	"GLOW", "HUM", "NO_RENT", "NO_DONATE", "NO_INVIS", "INVISIBLE", "MAGIC",
	"NO_DROP", "BLESS", "ANTI_GOOD", "ANTI_EVIL", "ANTI_NEUTRAL", "ANTI_MAGE",
	"ANTI_CLERIC", "ANTI_THIEF", "ANTI_WARRIOR", "NO_SELL", "NO_LOCATE",
}

// applyTypeNames are apply_types[] (constants.c:690), indexed by apply
// location rather than by bit.
var applyTypeNames = []string{
	"NONE", "STR", "DEX", "INT", "WIS", "CON", "CHA", "CLASS", "LEVEL", "AGE",
	"CHAR_WEIGHT", "CHAR_HEIGHT", "MAXMANA", "MAXHIT", "MAXMOVE", "GOLD",
	"EXP", "ARMOR", "HITROLL", "DAMROLL", "SAVING_PARA", "SAVING_ROD",
	"SAVING_PETRI", "SAVING_BREATH", "SAVING_SPELL",
}

// SprintBit lists the names of the set bits, porting sprintbit (utils.c).
//
// Every name is followed by a space, including the last, and an empty
// bitfield prints "NOBITS " — both of those are the C's and both are visible
// in `identify`'s output. A bit past the end of the table prints "UNDEFINED",
// and the C's index deliberately stops advancing at the terminator so *every*
// bit above the table prints it.
func SprintBit(f Flags, names []string) string {
	var out strings.Builder

	nr := 0
	for bits := f; bits != 0; bits >>= 1 {
		if bits&1 != 0 {
			if nr < len(names) {
				out.WriteString(names[nr])
			} else {
				out.WriteString("UNDEFINED")
			}
			out.WriteString(" ")
		}
		if nr < len(names) {
			nr++
		}
	}

	if out.Len() == 0 {
		return "NOBITS "
	}
	return out.String()
}

// SprintType names one value from a table, porting sprinttype (utils.c).
func SprintType(value int32, names []string) string {
	if value < 0 || int(value) >= len(names) {
		return "UNDEFINED"
	}
	return names[value]
}

// AffectBitNames, ExtraBitNames and ApplyTypeNames are the tables above, for
// callers outside this package.
func AffectBitNames() []string { return affectBitNames }

// ExtraBitNames are the ITEM_* names.
func ExtraBitNames() []string { return extraBitNames }

// ApplyTypeNames are the APPLY_* names.
func ApplyTypeNames() []string { return applyTypeNames }

// --- the rest of the name tables ---------------------------------------
//
// Added for `stat`, which prints nearly every bitfield the game has. They
// are transcribed from the C rather than invented, and bitnames_test.go
// re-parses constants.c and class.c and compares every entry — the same
// treatment the command table and the special-procedure assignments get, and
// for the same reason: a table is data, and data is checked rather than read.

// roomBitNames are room_bits[] (constants.c:40): the ROOM_* flags.
var roomBitNames = []string{
	"DARK", "DEATH", "NO_MOB", "INDOORS", "PEACEFUL", "SOUNDPROOF", "NO_TRACK",
	"NO_MAGIC", "TUNNEL", "PRIVATE", "GODROOM", "HOUSE", "HCRSH", "ATRIUM",
	"OLC", "*", "GOOD_REGEN", "CAN_QUIT", "PKILL",
}

// exitBitNames are exit_bits[] (constants.c:67): the EX_* flags on a door.
var exitBitNames = []string{
	"DOOR", "CLOSED", "LOCKED", "PICKPROOF",
}

// sectorNames are sector_types[] (constants.c:77), indexed by SECT_* rather than by bit.
var sectorNames = []string{
	"Inside", "City", "Field", "Forest", "Hills", "Mountains", "Water (Swim)",
	"Water (No Swim)", "In Flight", "Underwater",
}

// genderNames are genders[] (constants.c:96).
var genderNames = []string{
	"neutral", "male", "female",
}

// positionNames are position_types[] (constants.c:106), indexed by POS_*.
var positionNames = []string{
	"Dead", "Mortally wounded", "Incapacitated", "Stunned", "Sleeping",
	"Resting", "Sitting", "Fighting", "Standing",
}

// playerBitNames are player_bits[] (constants.c:121): the PLR_* flags.
var playerBitNames = []string{
	"KILLER", "THIEF", "FROZEN", "DONTSET", "WRITING", "MAILING", "CSH",
	"SITEOK", "NOSHOUT", "NOTITLE", "DELETED", "LOADRM", "NO_WIZL", "NO_DEL",
	"INVST", "CRYO", "DEAD",
}

// actionBitNames are action_bits[] (constants.c:144): the MOB_* flags.
var actionBitNames = []string{
	"SPEC", "SENTINEL", "SCAVENGER", "ISNPC", "AWARE", "AGGR", "STAY-ZONE",
	"WIMPY", "AGGR_EVIL", "AGGR_GOOD", "AGGR_NEUTRAL", "MEMORY", "HELPER",
	"NO_CHARM", "NO_SUMMN", "NO_SLEEP", "NO_BASH", "NO_BLIND", "DEAD",
}

// preferenceBitNames are preference_bits[] (constants.c:169): the PRF_* flags.
var preferenceBitNames = []string{
	"BRIEF", "COMPACT", "DEAF", "NO_TELL", "D_HP", "D_MANA", "D_MOVE", "AUTOEX",
	"NO_HASS", "QUEST", "SUMN", "NO_REP", "LIGHT", "C1", "C2", "NO_WIZ", "L1",
	"L2", "NO_AUC", "NO_GOS", "NO_GTZ", "RMFLG",
}

// connectedNames are connected_types[] (constants.c:226): the CON_* login states.
var connectedNames = []string{
	"Playing", "Disconnecting", "Get name", "Confirm name", "Get password",
	"Get new PW", "Confirm new PW", "Select sex", "Select class",
	"Reading MOTD", "Main Menu", "Get descript.", "Changing PW 1",
	"Changing PW 2", "Changing PW 3", "Self-Delete 1", "Self-Delete 2",
	"Disconnecting", "Object edit", "Room edit", "Zone edit", "Mobile edit",
	"Shop edit", "Text edit",
}

// wearBitNames are wear_bits[] (constants.c:336): the ITEM_WEAR_* flags.
var wearBitNames = []string{
	"TAKE", "FINGER", "NECK", "BODY", "HEAD", "LEGS", "FEET", "HANDS", "ARMS",
	"SHIELD", "ABOUT", "WAIST", "WRIST", "WIELD", "HOLD",
}

// containerBitNames are container_bits[] (constants.c:414): the CONT_* flags.
var containerBitNames = []string{
	"CLOSEABLE", "PICKPROOF", "CLOSED", "LOCKED",
}

// npcClassNames are npc_class_types[] (constants.c:727).
var npcClassNames = []string{
	"Normal", "Undead",
}

// pcClassNames are pc_class_types[] (class.c:58).
var pcClassNames = []string{
	"Magic User", "Cleric", "Thief", "Warrior", "Paladin",
}

// Accessors, so the session package can print them without the tables
// becoming part of its vocabulary.
func RoomBitNames() []string       { return roomBitNames }
func ExitBitNames() []string       { return exitBitNames }
func SectorNames() []string        { return sectorNames }
func GenderNames() []string        { return genderNames }
func PositionNames() []string      { return positionNames }
func PlayerBitNames() []string     { return playerBitNames }
func ActionBitNames() []string     { return actionBitNames }
func PreferenceBitNames() []string { return preferenceBitNames }
func ConnectedNames() []string     { return connectedNames }
func WearBitNames() []string       { return wearBitNames }
func ContainerBitNames() []string  { return containerBitNames }
func NpcClassNames() []string      { return npcClassNames }
func PcClassNames() []string       { return pcClassNames }
