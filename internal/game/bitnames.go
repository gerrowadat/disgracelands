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
