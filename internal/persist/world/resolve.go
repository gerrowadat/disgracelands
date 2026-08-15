package world

import "github.com/gerrowadat/disgracelands/internal/game"

// After loading, the C server runs renum_world() and renum_zone_table(),
// which convert every vnum a record points at into a real number — an index
// into the loaded arrays — and, crucially, do two things that change what the
// server actually holds:
//
//   - An exit whose destination does not exist becomes NOWHERE. The file's
//     original vnum is gone.
//   - A zone command any of whose vnums do not resolve has its opcode
//     rewritten to '*', which is the comment opcode. The command is still in
//     the array and is never executed again.
//
// The game model keeps the file's values, because a writer has to be able to
// reproduce the file. The dump has the opposite job — it is comparing what
// the two servers ended up holding — so resolution happens here, on the way
// into the dump, and the dump shows the post-resolution view.
//
// Real numbers themselves are never dumped. They are array indices, they
// depend on load order, and the plan is explicit that they must not reach a
// file. Resolvable references dump as their vnum; unresolvable ones as -1.

// resolver answers "does this vnum exist" for each record type.
type resolver struct {
	rooms map[game.RoomVnum]bool
	mobs  map[game.MobVnum]bool
	objs  map[game.ObjVnum]bool
}

func newResolver(w *game.World) *resolver {
	r := &resolver{
		rooms: make(map[game.RoomVnum]bool, len(w.Rooms)),
		mobs:  make(map[game.MobVnum]bool, len(w.Mobiles)),
		objs:  make(map[game.ObjVnum]bool, len(w.Objects)),
	}
	for _, x := range w.Rooms {
		r.rooms[x.Vnum] = true
	}
	for _, x := range w.Mobiles {
		r.mobs[x.Vnum] = true
	}
	for _, x := range w.Objects {
		r.objs[x.Vnum] = true
	}
	return r
}

// room returns v if the room exists, or NoRoom if it does not.
func (r *resolver) room(v game.RoomVnum) game.RoomVnum {
	if v != game.NoRoom && r.rooms[v] {
		return v
	}
	return game.NoRoom
}

func (r *resolver) roomOK(v game.RoomVnum) bool { return r.rooms[v] }
func (r *resolver) mobOK(v game.MobVnum) bool   { return r.mobs[v] }
func (r *resolver) objOK(v game.ObjVnum) bool   { return r.objs[v] }

// resetCommand reports how a reset command survives renumbering: the opcode
// the server ends up with, and the arguments after resolution.
//
// renum_zone_table() checks only the arguments its switch actually converts,
// which is why a 'G' command with a bad object is disabled but an 'M'
// command's max-in-world count is never examined. Reproducing that exactly
// matters: being stricter here would disable commands the C server runs.
func (r *resolver) resetCommand(c game.ResetCommand) (opcode byte, a1, a2, a3 int32, disabled bool) {
	a1, a2, a3 = c.Arg1, c.Arg2, c.Arg3

	var ok bool
	switch c.Command {
	case 'M': // load mobile arg1 into room arg3
		ok = r.mobOK(game.MobVnum(a1)) && r.roomOK(game.RoomVnum(a3))
	case 'O': // load object arg1 into room arg3
		ok = r.objOK(game.ObjVnum(a1))
		// The C code skips the room lookup entirely when arg3 is NOWHERE,
		// which is how "load into the last container" is expressed.
		if a3 != int32(game.NoRoom) {
			ok = ok && r.roomOK(game.RoomVnum(a3))
		}
	case 'G', 'E': // give/equip object arg1 to the last-loaded mobile
		ok = r.objOK(game.ObjVnum(a1))
	case 'P': // put object arg1 into object arg3
		ok = r.objOK(game.ObjVnum(a1)) && r.objOK(game.ObjVnum(a3))
	case 'D': // set the door state of room arg1, direction arg2
		ok = r.roomOK(game.RoomVnum(a1))
	case 'R': // remove object arg2 from room arg1
		ok = r.roomOK(game.RoomVnum(a1)) && r.objOK(game.ObjVnum(a2))
	default:
		// An opcode the server does not know is left alone; reset_zone
		// ignores it at run time.
		return c.Command, a1, a2, a3, false
	}

	if !ok {
		// The C loader rewrites the opcode to the comment character, which
		// disables the command permanently without removing it.
		return '*', a1, a2, a3, true
	}
	return c.Command, a1, a2, a3, false
}
