// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

import (
	"path/filepath"
)

// The world half of the corpus: one zone, #50, rooms/mobiles/objects/shops
// at 5000-5099.
//
// Every record here exists to be awkward, and the awkwardness is the
// content: each one is described, in its own description, by the case it
// is there to break. That is deliberate — a fixture whose hostile cases
// are only listed in a README drifts away from the README, whereas a room
// that says what it is for is read by anyone who ever dumps this world.
//
// The three "cannot survive a literal block" string shapes
// (internal/persist/world/yaml/text.go's needsQuoting) are rooms 5002,
// 5003 and 5004, and they are the reason this directory exists at all:
// before it, no checked-in fixture anywhere in the tree deliberately
// contained one, and all three were found by round-tripping the real
// private corpus rather than by anything in CI.
//
// Bytes matter here in a way they do not in examples/mini, so this is
// written as Go string literals with explicit escapes rather than as
// hand-edited files: \x92 is a CP1252 curly apostrophe, and a lone \r
// before a newline is a bare carriage return. Neither survives an editor
// that means well.

// allCFlags is every bit a CircleMUD bitvector_t can hold on the i386 the
// archive was written on: 'a'-'z' are bits 0-25 and 'A'-'F' bits 26-31.
// 'G' onwards is `1 << 32` in an int, which is undefined behaviour in the
// C and which game.Flags.ExceedsCRange warns about — so this is the widest
// flag field that is hostile rather than simply invalid.
const allCFlags = "abcdefghijklmnopqrstuvwxyzABCDEF"

// worldFiles returns the classic world tree, keyed by path under world/.
func worldFiles() map[string]string {
	return map[string]string{
		"zone.lst":       zoneList,
		"wld/index":      "50.wld\n$\n",
		"mob/index":      "50.mob\n$\n",
		"obj/index":      "50.obj\n$\n",
		"shp/index":      "50.shp\n$\n",
		"zon/index":      "50.zon\n$\n",
		"wld/50.wld":     torturedRooms,
		"mob/50.mob":     torturedMobiles,
		"obj/50.obj":     torturedObjects,
		"shp/50.shp":     torturedShops,
		"zon/50.zon":     torturedZone,
		"wld/index.mini": "50.wld\n$\n",
		"mob/index.mini": "50.mob\n$\n",
		"obj/index.mini": "50.obj\n$\n",
		"shp/index.mini": "50.shp\n$\n",
		"zon/index.mini": "50.zon\n$\n",
	}
}

func worldPath(base, rel string) string {
	return filepath.Join(base, "world", filepath.FromSlash(rel))
}

const zoneList = `Zone Vnum List
***************

50	- The Torture Chamber
`

// torturedRooms is wld/50.wld.
//
// Room by room:
//
//	5000  every flag bit the C can represent, set at once
//	5001  CP1252 bytes in the description, name and an exit
//	5002  a deliberate blank line before the closing tilde
//	5003  a bare carriage return mid-description
//	5004  trailing whitespace on an unterminated final line
//	5005  a '#' at the start of a line inside a description
//	5006  ASCII art on an extra description and on an exit description
//	5007  a '*' at the start of a line, and a line opening with "{{"
//	5008  the minimum: a name, an empty description, no exits, no extras
const torturedRooms = "#5000\n" +
	"The Antechamber of Every Flag~\n" +
	"   Every room flag a CircleMUD bitvector_t can hold is set on this room\n" +
	"at once. The field below is 'abcdefghijklmnopqrstuvwxyzABCDEF' - bits 0\n" +
	"through 31, which is as wide as an i386 `long` ever was. Anything from\n" +
	"'G' on would be `1 << 32` in an `int`, which is undefined behaviour in\n" +
	"the C loader rather than a bigger number, so this is the widest field\n" +
	"that is hostile instead of simply broken.\n" +
	"   Bits nothing has a name for come back through `flags_raw`, which is\n" +
	"the escape hatch the yaml format has for exactly this.\n" +
	"~\n" +
	"50 " + allCFlags + " 0\n" +
	"D0\n" +
	"North, to the room whose description is not ASCII.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5001\n" +
	"S\n" +

	"#5001\n" +
	"Caf\x92 de l\x92Encodage\x85~\n" +
	"   This description is CP1252, not UTF-8, and every byte in it that is\n" +
	"not ASCII is one an archived world file really contains: \x92 is the\n" +
	"curly apostrophe in data/world/wld/90.wld, \x93 and \x94 the curly\n" +
	"quotes, \x96 and \x97 the en and em dashes, \x85 an ellipsis, and \xe9\n" +
	"\xe8 \xfc \xf1 the accented letters a builder typed on a machine that\n" +
	"was not this one.\n" +
	"   \xa9 2001. \xbd a loaf. 30\xb0 to starboard.\n" +
	"~\n" +
	"50 0 1\n" +
	"E\n" +
	"caf\x92 sign~\n" +
	"   The sign says \x93Caf\xe9\x94, in the encoding it was painted in.\n" +
	"~\n" +
	"D0\n" +
	"North, past the caf\xe9, to the room that ends in silence.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5002\n" +
	"D2\n" +
	"Back to the Antechamber.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5000\n" +
	"S\n" +

	"#5002\n" +
	"The Room That Ends In Silence~\n" +
	"   This description ends with a deliberately blank line before its\n" +
	"closing tilde. fread_string appends CRLF for every line that does not\n" +
	"contain a tilde, so the loaded string ends with two of them, and a\n" +
	"YAML\n" +
	"literal block cannot carry that back: goccy's re-print right-trims\n" +
	"every trailing newline off the node regardless of the chomping\n" +
	"indicator. The yaml writer quotes and escapes it instead.\n" +
	"\n" +
	"~\n" +
	"50 0 2\n" +
	"D0\n" +
	"North, to the room with a carriage return in it.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5003\n" +
	"D2\n" +
	"Back to the caf\xe9.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5001\n" +
	"S\n" +

	"#5003\n" +
	"The Room With A Bare Carriage Return~\n" +
	"   The line after this one ends with a literal CR before its newline.\n" +
	"rawLine strips only the '\\n', so the CR survives into the string and\n" +
	"fread_string then appends its own CRLF after it:\r\n" +
	"   That is a bare carriage return once CRLF pairs are folded to LF, and\n" +
	"a bare CR is unrepresentable in a YAML block scalar at all - the spec\n" +
	"folds CR, CRLF and LF alike on decode. obj/0.obj's bug object in the\n" +
	"real archive has fifteen of them in a row.\n" +
	"~\n" +
	"50 0 0\n" +
	"D0\n" +
	"North, to the room with trailing spaces.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5004\n" +
	"D2\n" +
	"Back to the room that ends in silence.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5002\n" +
	"S\n" +

	"#5004\n" +
	"The Room With Trailing Spaces~\n" +
	"   The final line of this description carries its own tilde, so\n" +
	"fread_string returns without appending CRLF, and it carries trailing\n" +
	"spaces before that tilde. A literal block drops them; the writer\n" +
	"quotes the string instead.   ~\n" +
	"50 0 0\n" +
	"D0\n" +
	"North, to the room with a hash in it.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5005\n" +
	"D2\n" +
	"Back to the room with a carriage return in it.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5003\n" +
	"S\n" +

	"#5005\n" +
	"The Room With A Hash In It~\n" +
	"   The next line begins with a '#'. count_hash_records counts those to\n" +
	"size its arrays, and it counts this one, so the loader allocates room\n" +
	"for a record that is not there:\n" +
	"#5005 is prose, not a record header, whatever the record counter says.\n" +
	"   A lone dollar sign is content too, and so is a hash that is not in\n" +
	"the first column: only a tilde ends a string, and only a hash in\n" +
	"column one starts a record. $\n" +
	"~\n" +
	"50 0 0\n" +
	"D0\n" +
	"North, to the gallery.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5006\n" +
	"D2\n" +
	"Back to the room with trailing spaces.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5004\n" +
	"S\n" +

	"#5006\n" +
	"The Gallery of Indented Art~\n" +
	"   Two hand-drawn signs hang here. One is an extra description on the\n" +
	"room; the other is the description of the exit north. Both begin with\n" +
	"more leading whitespace than a later line carries, which is the shape\n" +
	"that makes a YAML literal block decode back to a different string than\n" +
	"it encoded unless an indentation indicator is emitted - and NestedText\n" +
	"cannot reliably emit one at this depth, so it quotes instead.\n" +
	"~\n" +
	"50 0 0\n" +
	"E\n" +
	"sign painted~\n" +
	"        +----------------+\n" +
	"        |  THIS WAY UP   |\n" +
	"        +----------------+\n" +
	"   and underneath, in a smaller hand, someone has added: 'or not'.\n" +
	"~\n" +
	"D0\n" +
	"        /\\\n" +
	"       /  \\    a gable, drawn in an exit description, six columns deep\n" +
	"      /____\\   in the yaml document that comes out the other side\n" +
	"~\n" +
	"gate grille~\n" +
	"1 5003 5007\n" +
	"D2\n" +
	"Back to the room with a hash in it.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5005\n" +
	"S\n" +

	"#5007\n" +
	"The Room That Looks Like Syntax~\n" +
	"   get_line skips a line whose first byte is '*'; fread_string does\n" +
	"not. So the next line is content, and a loader that used the wrong\n" +
	"primitive would silently lose it:\n" +
	"* this line begins with an asterisk and is part of the description.\n" +
	"   The line after this one opens with the colour markup's own\n" +
	"delimiter, which YAML would otherwise read as a flow mapping:\n" +
	"{{red}}not colour, just two braces{{/}}\n" +
	"~\n" +
	"50 0 0\n" +
	"D0\n" +
	"North, to the last room.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5008\n" +
	"D2\n" +
	"Back to the gallery.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5006\n" +
	"S\n" +

	"#5008\n" +
	"The Empty Room~\n" +
	"~\n" +
	"50 0 0\n" +
	"D2\n" +
	"Back to the room that looks like syntax.\n" +
	"~\n" +
	"~\n" +
	"0 -1 5007\n" +
	"S\n" +
	"$\n"

// torturedMobiles is mob/50.mob.
//
//	5000  every action and affection bit, dice at their extremes, S format
//	5001  E format with an espec for every ability the C names
//	5002  the minimum a mobile can be, and a shopkeeper for shp/50.shp
const torturedMobiles = "#5000\n" +
	"everything mobile flags~\n" +
	"the mobile with every flag~\n" +
	"A mobile carrying every action and affection bit at once stands here.\n" +
	"~\n" +
	"   Both bit fields on this mobile are the full 32 the C can represent.\n" +
	"Its hit dice and damage dice are at the extremes sscanf will read: the\n" +
	"format embeds the 'd' and the '+' as literals, so a negative bonus\n" +
	"cannot be written at all, and these are the largest that fit.\n" +
	"~\n" +
	allCFlags + " " + allCFlags + " -1000 S\n" +
	"34 -10 -100 30d30+30000 20d20+2000\n" +
	"2000000000 2000000000\n" +
	"8 8 2\n" +

	"#5001\n" +
	"enhanced especs mobile~\n" +
	"the enhanced mobile~\n" +
	"A mobile with an espec for every ability is here, being enhanced.\n" +
	"~\n" +
	"   parse_enhanced_mob reads 'Key: Value' lines until an 'E' on its own.\n" +
	"interpret_espec splits on the first colon and treats a line with no\n" +
	"colon as a keyword with no value, so both shapes are here.\n" +
	"~\n" +
	"ab 0 0 E\n" +
	"1 20 10 1d1+1 1d1+0\n" +
	"0 1\n" +
	"8 8 0\n" +
	"BareHandAttack: 12\n" +
	"Str: 18\n" +
	"StrAdd: 100\n" +
	"Int: 18\n" +
	"Wis: 18\n" +
	"Dex: 18\n" +
	"Con: 18\n" +
	"Cha: 18\n" +
	"NoColonKeyword\n" +
	"E\n" +

	"#5002\n" +
	"shopkeeper torture~\n" +
	"the torture chamber's shopkeeper~\n" +
	"A shopkeeper stands here, selling awkward things.\n" +
	"~\n" +
	"~\n" +
	"ab 0 0 S\n" +
	"20 10 0 10d10+100 2d4+2\n" +
	"500 1000\n" +
	"8 8 1\n" +
	"$\n"

// torturedObjects is obj/50.obj.
//
//	5000  every extra and wear bit, all four values at int32 extremes
//	5001  a drink container the loader mutates at load time (the weight fix)
//	5002  the maximum number of 'A' affect lines, and extra descriptions
//	5003  the minimum an object can be
//	5004  a container, for the rent file's nesting
const torturedObjects = "#5000\n" +
	"extremes object every flag~\n" +
	"the object of every extreme~\n" +
	"An object whose every value is at an extreme lies here.~\n" +
	"   Read me. I am an action description, which most objects do not have.\n" +
	"~\n" +
	"5 " + allCFlags + " " + allCFlags + " 2147483647\n" +
	"2147483647 -2147483648 2147483647 -2147483648\n" +
	"2147483647 2147483647 2147483647 34\n" +
	"A\n" +
	"1 -100\n" +
	"A\n" +
	"18 100\n" +

	"#5001\n" +
	"flask heavy liquid~\n" +
	"a suspiciously light flask~\n" +
	"A flask that weighs less than what is in it sits here.~\n" +
	"~\n" +
	"17 0 1 0\n" +
	"100 96 0 0\n" +
	"1 10 1 0\n" +

	"#5002\n" +
	"described thing many~\n" +
	"a much-described thing~\n" +
	"A thing with more descriptions than sense is here.~\n" +
	"~\n" +
	"9 0 3 0\n" +
	"0 0 0 0\n" +
	"1 1 1 0\n" +
	"E\n" +
	"first~\n" +
	"   The first extra description in the file, which the C loader prepends\n" +
	"and therefore sees last.\n" +
	"~\n" +
	"E\n" +
	"second~\n" +
	"        ___\n" +
	"       /   \\   art on an object's extra description, indented deeper on\n" +
	"       \\___/   its first line than on its last\n" +
	"~\n" +
	"E\n" +
	"third~\n" +
	"   The last extra description in the file, which the C loader sees\n" +
	"first. reverseExtras is what keeps that observable.\n" +
	"~\n" +
	"A\n" +
	"1 1\n" +
	"A\n" +
	"2 2\n" +

	"#5003\n" +
	"key minimal~\n" +
	"a minimal key~\n" +
	"A minimal key lies here.~\n" +
	"~\n" +
	"18 0 0\n" +
	"0 0 0 0\n" +
	"0 0 0\n" +

	"#5004\n" +
	"box nesting~\n" +
	"a nesting box~\n" +
	"A box that holds another box is here.~\n" +
	"~\n" +
	"15 0 1 0\n" +
	"100 0 -1 0\n" +
	"5 10 1 0\n" +
	"$\n"

// torturedShops is shp/50.shp: one shop, with both of the fields
// docs/design/data-format.md §4.5 calls "kept awkward on purpose".
const torturedShops = `CircleMUD v3.0 Shop File~
#50~
5000
5002
-1
1.5
0.25
WEAPON
ARMOR
-1
%s I do not have that, and would not sell it if I did.~
%s You do not seem to have that.~
%s I do not buy that here.~
%s I cannot afford that right now!~
%s You cannot afford that!~
%s That will be %d coins, thanks.~
%s I will give you %d coins for it.~
0
0
5002
0
5000
-1
0
28
0
0
$~
`

// torturedZone is zon/50.zon. The header's bot-top range is 5000-5099,
// wide enough for every vnum above; a reset naming something outside its
// own zone's range is what examples/mini's README records losing records
// to, so this one deliberately does not.
const torturedZone = `#50
The Torture Chamber~
5000 5099 30 2
* The mobile with every flag, and everything it is carrying.
M 0 5000 1 5000
G 1 5000 1
E 1 5002 1 16
* A container inside a container inside a container, on the floor.
O 0 5004 3 5006
P 1 5004 3 5004
P 1 5004 3 5004
P 1 5003 1 5004
* The enhanced mobile, and the shopkeeper with its stock.
M 0 5001 1 5001
M 0 5002 1 5002
G 1 5000 5
G 1 5002 5
* Doors: the north exit out of the gallery is closed and locked.
D 0 5006 0 2
S
$
`
