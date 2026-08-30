// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// Attaching special procedures to prototypes, porting spec_assign.c's
// ASSIGNMOB/ASSIGNOBJ/ASSIGNROOM.
//
// A special is named here rather than pointed at. The world model has no
// business knowing what a guildmaster *does* — that is a command's business
// and lives with the commands — so what it holds is a string, and the layer
// that runs specials looks it up. It also means the assignment table can be
// compared against the C's, which a table of function pointers could not be.

// AssignSpecials attaches every special in the table to the prototypes that
// exist, and reports how many were attached and how many named a vnum this
// world does not have.
//
// Missing vnums are normal rather than exceptional: the shipped stock data is
// Midgaard and several assignments in this tree point at the archived world.
// The C logs a SYSERR for each unless it is running a mini-mud.
func (l *Live) AssignSpecials() (attached, missing int) {
	for _, a := range MobileSpecials {
		def, ok := l.mobileDefs[MobVnum(a.Vnum)]
		if !ok {
			missing++
			continue
		}
		def.Spec = a.Name
		attached++
	}
	for _, a := range ObjectSpecials {
		def, ok := l.objectDefs[ObjVnum(a.Vnum)]
		if !ok {
			missing++
			continue
		}
		def.Spec = a.Name
		attached++
	}
	for _, a := range RoomSpecials {
		def, ok := l.rooms[RoomVnum(a.Vnum)]
		if !ok {
			missing++
			continue
		}
		def.Spec = a.Name
		attached++
	}
	return attached, missing
}

// MobSpec is the special attached to a character's prototype, or "".
func (c *Character) MobSpec() string {
	if c == nil || c.MobDef == nil {
		return ""
	}
	return c.MobDef.Spec
}

// ObjSpec is the special attached to an object's prototype, or "".
func (o *Object) ObjSpec() string {
	if o == nil || o.Def == nil {
		return ""
	}
	return o.Def.Spec
}

// guildInfo is guild_info[][3] (class.c:196): which class may pass which
// guild door, by room and direction.
//
// The class of -999 means "everybody", which is how the Brass Dragon's guard
// and the one at 14279 let anybody through — they are doors that need a guard
// for some other reason. The table ends at a -1 sentinel in the C.
var guildInfo = []struct {
	Class Class
	Room  RoomVnum
	Dir   Direction
}{
	// Midgaard.
	{ClassMagicUser, 3017, South},
	{ClassCleric, 3004, North},
	{ClassThief, 3027, East},
	{ClassWarrior, 3021, East},
	// Brass Dragon, and one local addition.
	{guildAnyClass, 5065, West},
	{guildAnyClass, 14279, Up},
}

// guildAnyClass is the C's -999.
const guildAnyClass Class = -999

// GuildBars reports whether a guild guard should block this character from
// leaving this room in this direction, porting the loop in
// SPECIAL(guild_guard).
//
// The condition is the local rewrite: it tests the *remort vector* rather
// than the current class, so a warrior who once was a thief may walk into the
// thieves' guild. Stock CircleMUD compares `GET_CLASS(ch)` and would turn them
// away.
func GuildBars(rec *PlayerRecord, room RoomVnum, dir Direction) bool {
	if rec == nil {
		return false
	}
	for _, g := range guildInfo {
		if g.Room != room || g.Dir != dir {
			continue
		}
		if g.Class == guildAnyClass {
			// Nobody's remort vector holds this bit, so the C's test —
			// "vector does not contain the class's mask" — is always true and
			// the guard blocks everyone.
			return true
		}
		mask, ok := classRemortMasks[g.Class]
		if !ok {
			continue
		}
		if !remortFlags(rec.RemortVector).Has(remortFlags(mask)) {
			return true
		}
	}
	return false
}

// mobileAttackSpells is the ladder in SPECIAL(magic_user): which spell a
// mobile caster throws, by level, in bands of two.
//
// Nothing below level four is listed, so a level-three mobile magic user
// falls through the switch and throws nothing at all — the C's `default` is
// fireball and its cases start at 4, which means levels 0 to 3 reach the
// default too. Reproduced: below four it is fireball, which is almost
// certainly not what anyone intended and is what the code does.
func MobileAttackSpell(level int32) int32 {
	switch level {
	case 4, 5:
		return SpellMagicMissile
	case 6, 7:
		return SpellChillTouch
	case 8, 9:
		return SpellBurningHands
	case 10, 11:
		return SpellShockingGrasp
	case 12, 13:
		return SpellLightningBolt
	case 14, 15, 16, 17:
		return SpellColorSpray
	}
	return SpellFireball
}

// Capitalise upper-cases the first letter, which is what act() does to a
// message it is about to send.
func Capitalise(s string) string { return capitaliseFirst(s) }
