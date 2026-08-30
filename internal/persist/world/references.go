// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package world

import (
	"fmt"
	"sort"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// CheckReferences reports every vnum a loaded world names that the world
// does not have: exits leading nowhere, exits locked by an object that was
// deleted, reset commands loading a mobile or an object that is gone, shops
// operating in a room that is not there, and records defined twice.
//
// These are *observations about a world*, not about the file it was read
// from, which is why they live here and are run by both loaders rather than
// being written into one of them.
//
// That distinction was the whole of #286. These checks were in the classic
// reader, one warnf per parse site, and the yaml loader produced none of
// them -- so `dlctl lint --type=world` reported problems while a world was
// still in the format we are migrating *off*, and reported nothing at all
// once it was in the format we actually run on. On the archived
// Disgracelands lib/ that was twenty findings against the classic
// directory and zero against the converted one, four of which are still
// true of the converted data:
//
//	warn: shop #3008 operates in room #3056, which does not exist
//	warn: shop #5433 operates in room #6563, which does not exist
//	warn: room #12038's north exit is locked by object #12104, which does not exist
//	warn: room #14258's up exit is locked by object #14260, which does not exist
//
// `dlctl lint` exists to replace src/util/scheck and `dlmud -c`, and on a
// yaml world it replaced neither.
//
// What stays behind in the classic reader is the other kind of finding, and
// the line between them is worth stating because it is not obvious: a
// finding belongs *there* when the loader **changed** the data as it read
// it, so the world handed back no longer has the problem to observe. "shop
// #N produces object #M, which does not exist; the C loader drops it
// silently" is one of those -- real_object() discards the entry, so the
// yaml written from that load has no such entry and cannot report one. The
// same goes for a keeper that does not exist, which is read as no keeper at
// all. Those are facts about a conversion, and they correctly disappear
// once the conversion has happened.
//
// The order is fixed and vnum-ascending within each kind, so two runs over
// the same world produce the same report and a diff of two reports is
// about the worlds.
func CheckReferences(w *game.World) []Warning {
	var out []Warning
	warnf := func(format string, args ...any) {
		out = append(out, Warning{Severity: Warn, Message: fmt.Sprintf(format, args...)})
	}

	rooms := make(map[game.RoomVnum]bool, len(w.Rooms))
	for _, r := range w.Rooms {
		if rooms[r.Vnum] {
			warnf("room #%d is defined more than once; the later definition is a separate room with the same vnum", r.Vnum)
		}
		rooms[r.Vnum] = true
	}
	mobs := make(map[game.MobVnum]bool, len(w.Mobiles))
	for _, m := range w.Mobiles {
		if mobs[m.Vnum] {
			warnf("mob #%d is defined more than once", m.Vnum)
		}
		mobs[m.Vnum] = true
	}
	objs := make(map[game.ObjVnum]bool, len(w.Objects))
	for _, o := range w.Objects {
		if objs[o.Vnum] {
			warnf("object #%d is defined more than once", o.Vnum)
		}
		objs[o.Vnum] = true
	}

	for _, room := range w.Rooms {
		for dir, exit := range room.Exits {
			if exit == nil {
				continue
			}
			if exit.ToRoom != game.NoRoom && !rooms[exit.ToRoom] {
				warnf("room #%d's %s exit leads to room #%d, which does not exist; the exit will be unusable",
					room.Vnum, game.Direction(dir), exit.ToRoom)
			}
			if exit.Key > 0 && !objs[exit.Key] {
				warnf("room #%d's %s exit is locked by object #%d, which does not exist",
					room.Vnum, game.Direction(dir), exit.Key)
			}
		}
	}

	// A shop's rooms are the one shop reference that survives a load
	// unchanged: nothing is dropped and nothing is rewritten, the dangling
	// vnum is still in the file, and a player walking into that room finds
	// no shopkeeper. The other two shop checks stay in the classic reader
	// with the rest of the load-time rewrites.
	for _, shop := range w.Shops {
		for _, rv := range shop.Rooms {
			if !rooms[rv] {
				warnf("shop #%d operates in room #%d, which does not exist", shop.Vnum, rv)
			}
		}
	}

	out = append(out, checkZoneRanges(w)...)

	for _, zone := range w.Zones {
		for _, cmd := range zone.Commands {
			out = append(out, checkResetCommand(zone, cmd, rooms, mobs, objs)...)
		}
	}
	return out
}

// checkZoneRanges reports zones whose vnum ranges overlap, which nothing
// checked in either loader until #286 went looking for what else was
// unchecked in both.
//
// It is worth more than it looks. A room belongs to exactly one zone, and
// the C works out which with a cursor that only ever moves *forward*
// through the zone table: parse_room advances `zone` while the room's vnum
// is above zone_table[zone].top, and aborts outright if the vnum is below
// the current zone's bottom (db.c:916-924, "Room #%d is below zone %d",
// exit(1)). So with overlapping ranges the owner of a room in the overlap
// depends on the order the rooms are read in -- and a room that arrives
// after the cursor has moved past its own zone does not produce a wrong
// answer, it stops the server booting. This port is more forgiving (it
// sorts zones by bottom and takes the first range that contains the room),
// which means an overlap can sit in a directory that boots here and
// refuses to boot on the C, with nothing said either way.
func checkZoneRanges(w *game.World) []Warning {
	zones := make([]*game.ZoneDef, len(w.Zones))
	copy(zones, w.Zones)
	sort.Slice(zones, func(i, j int) bool {
		if zones[i].Bottom != zones[j].Bottom {
			return zones[i].Bottom < zones[j].Bottom
		}
		return zones[i].Vnum < zones[j].Vnum
	})

	var out []Warning
	for i := 1; i < len(zones); i++ {
		prev, this := zones[i-1], zones[i]
		if this.Bottom > prev.Top {
			continue
		}
		out = append(out, Warning{Severity: Warn, Message: fmt.Sprintf(
			"zone #%d (rooms #%d-#%d) and zone #%d (rooms #%d-#%d) have overlapping vnum ranges; "+
				"which zone owns a room in the overlap depends on the order the rooms are read, "+
				"and the C aborts the boot for a room that arrives after its zone (db.c:916-924)",
			prev.Vnum, prev.Bottom, prev.Top, this.Vnum, this.Bottom, this.Top)})
	}
	return out
}

// checkResetCommand validates the vnums one reset command refers to. Which
// argument means what depends on the opcode; see reset_zone() in db.c.
func checkResetCommand(zone *game.ZoneDef, cmd game.ResetCommand,
	rooms map[game.RoomVnum]bool, mobs map[game.MobVnum]bool, objs map[game.ObjVnum]bool) []Warning {

	var out []Warning
	where := fmt.Sprintf("zone #%d line %d: command %q", zone.Vnum, cmd.Line, string(cmd.Command))
	warnf := func(format string, args ...any) {
		out = append(out, Warning{Severity: Warn, Message: fmt.Sprintf(format, args...)})
	}

	switch cmd.Command {
	case 'M': // load mobile Arg1 into room Arg3, max Arg2 in the world
		if !mobs[game.MobVnum(cmd.Arg1)] {
			warnf("%s loads mob #%d, which does not exist", where, cmd.Arg1)
		}
		if !rooms[game.RoomVnum(cmd.Arg3)] {
			warnf("%s loads into room #%d, which does not exist", where, cmd.Arg3)
		}
	case 'O': // load object Arg1 into room Arg3
		if !objs[game.ObjVnum(cmd.Arg1)] {
			warnf("%s loads object #%d, which does not exist", where, cmd.Arg1)
		}
		if !rooms[game.RoomVnum(cmd.Arg3)] {
			warnf("%s loads into room #%d, which does not exist", where, cmd.Arg3)
		}
	case 'G', 'E': // give/equip object Arg1 to the last-loaded mob
		if !objs[game.ObjVnum(cmd.Arg1)] {
			warnf("%s gives object #%d, which does not exist", where, cmd.Arg1)
		}
	case 'P': // put object Arg1 into object Arg3
		if !objs[game.ObjVnum(cmd.Arg1)] {
			warnf("%s puts object #%d, which does not exist", where, cmd.Arg1)
		}
		if !objs[game.ObjVnum(cmd.Arg3)] {
			warnf("%s puts into container #%d, which does not exist", where, cmd.Arg3)
		}
	case 'D': // set door state in room Arg1, direction Arg2
		if !rooms[game.RoomVnum(cmd.Arg1)] {
			warnf("%s sets a door in room #%d, which does not exist", where, cmd.Arg1)
		}
		if _, ok := game.DirectionFromInt(cmd.Arg2); !ok {
			warnf("%s sets a door in direction %d, which is not a direction", where, cmd.Arg2)
		}
	case 'R': // remove object Arg2 from room Arg1
		if !rooms[game.RoomVnum(cmd.Arg1)] {
			warnf("%s removes from room #%d, which does not exist", where, cmd.Arg1)
		}
		if !objs[game.ObjVnum(cmd.Arg2)] {
			warnf("%s removes object #%d, which does not exist", where, cmd.Arg2)
		}
	default:
		warnf("%s is not a reset command the server understands", where)
	}
	return out
}
