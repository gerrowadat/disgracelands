// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"fmt"

	"github.com/gerrowadat/disgracelands/internal/rng"
)

// Zone resets, ported from reset_zone and zone_update (db.c).
//
// This is what puts anything in the world. Rooms come from the world files
// and never change; every mobile and every object a player ever sees is
// created by a zone's reset list and re-created when the zone resets again.
//
// The list is a small stack machine with one register. `M` reads a mobile and
// remembers it; `G` and `E` give and equip onto *that* mobile. `if_flag` makes
// a command conditional on the previous one having succeeded, which is how
// "put a sword in the guard's hand, but only if the guard was actually
// created" is expressed.

// Reset command opcodes.
const (
	ResetIgnore   byte = '*'
	ResetMobile   byte = 'M'
	ResetObject   byte = 'O'
	ResetGive     byte = 'G'
	ResetEquip    byte = 'E'
	ResetPutInObj byte = 'P'
	ResetDoor     byte = 'D'
	ResetRemove   byte = 'R'
	ResetStop     byte = 'S'
)

// Door states, the values of a 'D' command's third argument.
const (
	DoorOpen   int32 = 0
	DoorClosed int32 = 1
	DoorLocked int32 = 2
)

// Exit flags, from structs.h.
const (
	ExitIsDoor    Flags = 1 << 0
	ExitClosed    Flags = 1 << 1
	ExitLocked    Flags = 1 << 2
	ExitPickproof Flags = 1 << 3
)

// ResetReport is what one zone reset did, for logs and tests.
type ResetReport struct {
	Zone     ZoneVnum
	Mobiles  int
	Objects  int
	Problems []string
}

// ResetZone runs a zone's command list, porting reset_zone.
//
// The population caps are the point of most of it: a command creates its
// mobile or object only while fewer than `Arg2` of them exist in the whole
// world. That is how a zone can be reset every fifteen minutes without the
// world filling up with swords.
func (l *Live) ResetZone(zone *ZoneDef, r *rng.Rand) ResetReport {
	report := ResetReport{Zone: zone.Vnum}

	// The one register: the mobile the last `M` created. `G` and `E` load
	// onto it, and both are errors if there has not been one.
	var mob *Character
	lastSucceeded := false

	for i := range zone.Commands {
		cmd := &zone.Commands[i]

		if cmd.Command == ResetStop {
			break
		}
		// A conditional command whose predecessor failed is skipped without
		// changing lastSucceeded, so a run of them all skip together.
		if cmd.IfFlag != 0 && !lastSucceeded {
			continue
		}

		switch cmd.Command {
		case ResetIgnore:
			lastSucceeded = false

		case ResetMobile:
			if l.mobileCount(MobVnum(cmd.Arg1)) >= cmd.Arg2 {
				lastSucceeded = false
				break
			}
			created := l.SpawnMobile(MobVnum(cmd.Arg1), RoomVnum(cmd.Arg3), r)
			if created == nil {
				report.problem(cmd, "no such mobile")
				lastSucceeded = false
				break
			}
			mob = created
			report.Mobiles++
			lastSucceeded = true

		case ResetObject:
			if l.objectCount(ObjVnum(cmd.Arg1)) >= cmd.Arg2 {
				lastSucceeded = false
				break
			}
			obj := l.NewObject(ObjVnum(cmd.Arg1))
			if obj == nil {
				report.problem(cmd, "no such object")
				lastSucceeded = false
				break
			}
			// A third argument of NOWHERE means the object is created and
			// left nowhere, which the C does deliberately — it exists to be
			// counted against the population cap.
			if RoomVnum(cmd.Arg3) != NoRoom {
				l.ObjectToRoom(obj, RoomVnum(cmd.Arg3))
			} else {
				l.track(obj)
			}
			report.Objects++
			lastSucceeded = true

		case ResetGive:
			if mob == nil {
				report.problem(cmd, "give to a mobile that does not exist")
				cmd.Command = ResetIgnore
				break
			}
			if l.objectCount(ObjVnum(cmd.Arg1)) >= cmd.Arg2 {
				lastSucceeded = false
				break
			}
			obj := l.NewObject(ObjVnum(cmd.Arg1))
			if obj == nil {
				report.problem(cmd, "no such object")
				lastSucceeded = false
				break
			}
			l.ObjectToChar(obj, mob)
			report.Objects++
			lastSucceeded = true

		case ResetEquip:
			if mob == nil {
				report.problem(cmd, "equip a mobile that does not exist")
				cmd.Command = ResetIgnore
				break
			}
			if l.objectCount(ObjVnum(cmd.Arg1)) >= cmd.Arg2 {
				lastSucceeded = false
				break
			}
			pos := WearPosition(cmd.Arg3)
			if pos < 0 || pos >= NumWears {
				report.problem(cmd, "invalid equipment position")
				lastSucceeded = false
				break
			}
			obj := l.NewObject(ObjVnum(cmd.Arg1))
			if obj == nil {
				report.problem(cmd, "no such object")
				lastSucceeded = false
				break
			}
			if !l.Equip(obj, mob, pos) {
				// The slot was already filled by an earlier command. The C
				// logs a SYSERR and drops the object on the floor; putting it
				// in the mobile's inventory loses nothing and keeps the
				// object where the builder meant it to be.
				l.ObjectToChar(obj, mob)
			}
			report.Objects++
			lastSucceeded = true

		case ResetPutInObj:
			if l.objectCount(ObjVnum(cmd.Arg1)) >= cmd.Arg2 {
				lastSucceeded = false
				break
			}
			container := l.findObjectByVnum(ObjVnum(cmd.Arg3))
			if container == nil {
				report.problem(cmd, "target object not found, command disabled")
				cmd.Command = ResetIgnore
				break
			}
			obj := l.NewObject(ObjVnum(cmd.Arg1))
			if obj == nil {
				report.problem(cmd, "no such object")
				lastSucceeded = false
				break
			}
			l.ObjectToObject(obj, container)
			report.Objects++
			lastSucceeded = true

		case ResetRemove:
			if obj := l.findObjectInRoom(RoomVnum(cmd.Arg1), ObjVnum(cmd.Arg2)); obj != nil {
				l.ExtractObject(obj)
			}
			lastSucceeded = true

		case ResetDoor:
			// Bounds-checked before the conversion, not after: Direction is a
			// narrow type and a zone file is data from disk.
			room := l.Room(RoomVnum(cmd.Arg1))
			if room == nil || cmd.Arg2 < 0 || cmd.Arg2 >= NumDirections {
				report.problem(cmd, "door does not exist, command disabled")
				cmd.Command = ResetIgnore
				break
			}
			dir := Direction(cmd.Arg2)
			if room.Exits[dir] == nil {
				report.problem(cmd, "door does not exist, command disabled")
				cmd.Command = ResetIgnore
				break
			}
			exit := room.Exits[dir]
			switch cmd.Arg3 {
			case DoorOpen:
				exit.State = exit.State.Clear(ExitClosed | ExitLocked)
			case DoorClosed:
				exit.State = exit.State.Set(ExitClosed).Clear(ExitLocked)
			case DoorLocked:
				exit.State = exit.State.Set(ExitClosed | ExitLocked)
			}
			lastSucceeded = true
		}
	}

	return report
}

func (r *ResetReport) problem(cmd *ResetCommand, why string) {
	r.Problems = append(r.Problems,
		fmt.Sprintf("line %d: %c %d %d %d: %s", cmd.Line, cmd.Command, cmd.Arg1, cmd.Arg2, cmd.Arg3, why))
}

// SpawnMobile instantiates a mobile prototype into a room, porting
// read_mobile plus char_to_room.
func (l *Live) SpawnMobile(vnum MobVnum, room RoomVnum, r *rng.Rand) *Character {
	def := l.mobileDefs[vnum]
	if def == nil || l.Room(room) == nil {
		return nil
	}

	c := &Character{
		// Name is what appears in a sentence; Keywords is what a player
		// types. The C keeps these as short_descr and name respectively, and
		// conflating them is how you end up with "puff dragon fractal hits
		// you".
		Name:     def.ShortDesc,
		Keywords: def.Keywords,
		Record:   mobileRecord(def, r),
		NPC:      true,
		Position: Position(def.Position),
		MobDef:   def,
	}

	if err := l.Enter(c, room); err != nil {
		return nil
	}
	l.mobiles[c] = true
	return c
}

// mobileRecord builds a fresh *PlayerRecord from a mobile prototype,
// porting read_mobile's stat derivation: rolling hit dice and converting
// thac0/armour class to hitroll/armor, exactly as SpawnMobile always
// has. Factored out so ReloadMobile can give an *existing* instance the
// same fresh derivation without duplicating it.
func mobileRecord(def *MobDef, r *rng.Rand) *PlayerRecord {
	rec := &PlayerRecord{
		Name:        def.ShortDesc,
		Description: def.Description,
		Level:       def.Level,
		Alignment:   def.Alignment,
		Sex:         def.Sex,
		AffectFlags: def.AffectionFlags,
		DamageDice:  def.DamageDice.Number,
		DamageSize:  def.DamageDice.Size,
	}

	// The C converts these at load time; the loader here keeps the file
	// values so a writer can reproduce the file, which means the conversion
	// happens now instead.
	rec.Points.HitRoll = 20 - def.Thac0
	rec.Points.Armor = def.ArmorClass * 10
	rec.Points.DamRoll = def.DamageDice.Bonus

	hit := def.HitDice.Bonus
	if def.HitDice.Number > 0 && def.HitDice.Size > 0 {
		hit += r.Dice(def.HitDice.Number, def.HitDice.Size)
	}
	rec.Points.MaxHit, rec.Points.Hit = hit, hit
	rec.Points.MaxMana, rec.Points.Mana = 100, 100
	rec.Points.MaxMove, rec.Points.Move = 100, 100
	rec.Points.Gold = def.Gold
	rec.Points.Exp = def.Exp

	// Every ability is 11 for a mobile unless an espec says otherwise, which
	// is what read_mobile does.
	rec.Abilities = Abilities{
		Strength: 11, Intelligence: 11, Wisdom: 11,
		Dexterity: 11, Constitution: 11, Charisma: 11,
	}

	// A mobile's prototype flags are its unaffected state, so a spell that
	// wears off leaves it as its file describes rather than as nothing.
	SnapshotReal(rec)
	RecomputeAffects(rec)
	return rec
}

// ReloadMobile applies a freshly-parsed definition to the matching
// prototype already in the world, in place — mutating the shared
// *MobDef object rather than replacing the map entry, so every live
// instance's behavioural/descriptive reads (ActionFlags, Spec, LongDesc,
// Position, Keywords — see mobflags.go, spec.go, commands.go's `look`)
// see the change immediately, the same way they already see it change
// mid-tick from an affect. There is no way to update a shared object for
// some readers and not others, which is why this is all-or-nothing:
// ok is false, and nothing is touched at all, if any current instance of
// the vnum is fighting.
//
// This is new capability, not a C port — reference/moderncserver has
// nothing like it. See docs/deviations.md.
//
// On success, every current instance's derived stats (hit/mana/move,
// thac0, armour, damage dice, gold, exp, abilities) are recomputed via
// mobileRecord, as if freshly spawned — but Room, Carrying, Equipment,
// Followers and Position are left exactly as they are, and so is Spec
// (set once at boot from the assignment table, AssignSpecials — never
// part of the world file, so a fresh parse's own empty Spec must not
// overwrite it). refreshed is how many instances that applied to.
func (l *Live) ReloadMobile(fresh *MobDef, r *rng.Rand) (refreshed int, ok bool) {
	if fresh == nil {
		return 0, false
	}
	existing := l.mobileDefs[fresh.Vnum]
	if existing == nil {
		return 0, false
	}

	var instances []*Character
	for c := range l.mobiles {
		if c.MobDef != existing {
			continue
		}
		if c.Fighting != nil {
			return 0, false
		}
		instances = append(instances, c)
	}

	spec := existing.Spec
	*existing = *fresh
	existing.Spec = spec

	for _, c := range instances {
		c.Record = mobileRecord(existing, r)
		c.Name = existing.ShortDesc
		c.Keywords = existing.Keywords
	}
	return len(instances), true
}

// ReloadZoneResult reports what a zone reload actually changed.
type ReloadZoneResult struct {
	// Rooms is how many room prototypes were updated.
	Rooms int
	// Mobiles is how many live mobile instances had their derived stats
	// refreshed — the same count ReloadMobile reports, summed across
	// every mobile vnum in the zone's range.
	Mobiles int
}

// ReloadZone is ReloadMobile's zone-wide extension: applies a
// freshly-parsed zone, plus every room and mobile whose vnum falls in
// its range, to the running world, in place. New capability, not a C
// port — see docs/deviations.md and docs/proposals/go-port-plan.md.
//
// Refuses outright (ok=false, nothing touched) if a player is anywhere
// in the zone (ZoneIsEmpty) or any live mobile instance whose vnum falls
// in the zone's range is fighting — the same all-or-nothing reasoning
// ReloadMobile already documents, now at a bigger blast radius: a room's
// exits or description changing under a standing player is exactly the
// kind of surprise this whole feature exists to avoid.
//
// Deliberately conservative in what it applies: a room or mobile vnum
// this reload's fresh data introduces that the running world does not
// already have is skipped, not created — reload updates what exists,
// it does not import what is new. A vnum the fresh data no longer has
// (deleted from the file) is left as a stale entry rather than removed.
// Both are real limitations, not oversights, and both need a restart
// still — see the open questions this leaves, noted where this is
// wired into the command.
func (l *Live) ReloadZone(fresh *ZoneDef, freshRooms []*RoomDef, freshMobiles []*MobDef, r *rng.Rand) (ReloadZoneResult, bool) {
	if fresh == nil {
		return ReloadZoneResult{}, false
	}
	var existing *ZoneDef
	for _, z := range l.defs.Zones {
		if z.Vnum == fresh.Vnum {
			existing = z
			break
		}
	}
	if existing == nil {
		return ReloadZoneResult{}, false
	}
	if !l.ZoneIsEmpty(existing) {
		return ReloadZoneResult{}, false
	}
	for c := range l.mobiles {
		if c.MobDef == nil {
			continue
		}
		v := RoomVnum(c.MobDef.Vnum)
		if v >= existing.Bottom && v <= existing.Top && c.Fighting != nil {
			return ReloadZoneResult{}, false
		}
	}

	var result ReloadZoneResult
	for _, fr := range freshRooms {
		if fr.Vnum < existing.Bottom || fr.Vnum > existing.Top {
			continue
		}
		if _, ok := l.rooms[fr.Vnum]; !ok {
			continue
		}
		l.rooms[fr.Vnum] = fr
		result.Rooms++
	}
	for _, fm := range freshMobiles {
		if RoomVnum(fm.Vnum) < existing.Bottom || RoomVnum(fm.Vnum) > existing.Top {
			continue
		}
		if _, ok := l.mobileDefs[fm.Vnum]; !ok {
			continue
		}
		n, ok := l.ReloadMobile(fm, r)
		if !ok {
			// Already confirmed nothing in range is fighting, above —
			// this would mean the world changed under us mid-call,
			// which cannot happen on the single-threaded world
			// goroutine this always runs on. Kept as a guard, not a
			// reachable path.
			continue
		}
		result.Mobiles += n
	}

	existing.Name = fresh.Name
	existing.Bottom = fresh.Bottom
	existing.Top = fresh.Top
	existing.Lifespan = fresh.Lifespan
	existing.ResetMode = fresh.ResetMode
	existing.Commands = fresh.Commands

	return result, true
}

// mobileCount is how many of a prototype exist, which is what the population
// caps are measured against.
func (l *Live) mobileCount(vnum MobVnum) int32 {
	var n int32
	for c := range l.mobiles {
		if c.MobDef != nil && c.MobDef.Vnum == vnum {
			n++
		}
	}
	return n
}

func (l *Live) objectCount(vnum ObjVnum) int32 {
	var n int32
	for _, o := range l.objects {
		if o.Vnum() == vnum {
			n++
		}
	}
	return n
}

func (l *Live) findObjectByVnum(vnum ObjVnum) *Object {
	for _, o := range l.objects {
		if o.Vnum() == vnum {
			return o
		}
	}
	return nil
}

func (l *Live) findObjectInRoom(room RoomVnum, vnum ObjVnum) *Object {
	for _, o := range l.roomObjects[room] {
		if o.Vnum() == vnum {
			return o
		}
	}
	return nil
}

// Track registers a mobile that was created outside a zone reset — an
// implementor's `load`, or a test.
func (l *Live) Track(c *Character) {
	if c != nil && c.IsNPC() {
		l.mobiles[c] = true
	}
}

// Mobiles returns every mobile in the world.
func (l *Live) Mobiles() []*Character {
	out := make([]*Character, 0, len(l.mobiles))
	for c := range l.mobiles {
		out = append(out, c)
	}
	return out
}

// RemoveMobile takes a dead mobile out of the world.
func (l *Live) RemoveMobile(c *Character) {
	delete(l.mobiles, c)
	l.Remove(c)
}

// Zones returns the zone definitions, in file order.
func (l *Live) Zones() []*ZoneDef { return l.defs.Zones }

// ZoneIsEmpty reports whether a zone has no players in it, which is what
// reset mode 1 waits for.
func (l *Live) ZoneIsEmpty(zone *ZoneDef) bool {
	for _, c := range l.Players() {
		if c.IsNPC() {
			continue
		}
		if c.Room >= zone.Bottom && c.Room <= zone.Top {
			return false
		}
	}
	return true
}

// ZoneOf returns the zone a room belongs to, or nil.
//
// The C stores the zone number on the room at load time; here it is a lookup
// over the zones' vnum ranges, which is the same answer — `world[i].zone` is
// set by exactly that comparison in db.c.
func (l *Live) ZoneOf(room RoomVnum) *ZoneDef {
	for _, zone := range l.defs.Zones {
		if room >= zone.Bottom && room <= zone.Top {
			return zone
		}
	}
	return nil
}
