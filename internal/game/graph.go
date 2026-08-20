// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// find_first_step, ported from graph.c:110.
//
// A breadth-first search over the room graph, returning the first step of a
// shortest path. It is what `track` follows and what a hunting mobile walks,
// and it is the only graph algorithm in the whole server.

// What the search found (graph.h).
const (
	// BFSError is a bad room number.
	BFSError = -1
	// BFSAlreadyThere is the source being the target.
	BFSAlreadyThere = -2
	// BFSNoPath is no route at all.
	BFSNoPath = -3
)

// TrackThroughDoors is `track_through_doors = YES` (config.c:109), so a
// closed door does not break a trail on this server. The stock default is NO.
var TrackThroughDoors = true

// FindFirstStep returns the first direction of a shortest path from src to
// target, or one of the BFS* constants.
//
// The C marks rooms with a ROOM_BFS_MARK flag on the world itself and clears
// every room in the world before starting — which is O(rooms) per search
// whatever the answer, and means two searches can never run at once. A local
// set costs the same and does neither.
func (l *Live) FindFirstStep(src, target RoomVnum) int {
	if l.Room(src) == nil || l.Room(target) == nil {
		return BFSError
	}
	if src == target {
		return BFSAlreadyThere
	}

	type step struct {
		room RoomVnum
		dir  Direction
	}

	marked := map[RoomVnum]bool{src: true}
	var queue []step

	// validEdge is VALID_EDGE (graph.c:56): an exit that exists, leads
	// somewhere, is not shut (unless doors are trackable), and does not lead
	// into a NOTRACK room or one already seen.
	validEdge := func(from RoomVnum, dir Direction) (RoomVnum, bool) {
		exit := l.Exit(from, dir)
		if exit == nil || exit.ToRoom == NoRoom {
			return 0, false
		}
		if !TrackThroughDoors && exit.State.Has(ExitClosed) {
			return 0, false
		}
		to := l.Room(exit.ToRoom)
		if to == nil || to.Flags.Has(RoomNoTrack) || marked[exit.ToRoom] {
			return 0, false
		}
		return exit.ToRoom, true
	}

	// The first steps carry the direction they went in; every step after
	// inherits it, so what comes back is the *first* move of the path rather
	// than the last.
	for dir := Direction(0); int(dir) < NumDirections; dir++ {
		if to, ok := validEdge(src, dir); ok {
			marked[to] = true
			queue = append(queue, step{room: to, dir: dir})
		}
	}

	for len(queue) > 0 {
		head := queue[0]
		if head.room == target {
			return int(head.dir)
		}
		for dir := Direction(0); int(dir) < NumDirections; dir++ {
			if to, ok := validEdge(head.room, dir); ok {
				marked[to] = true
				queue = append(queue, step{room: to, dir: head.dir})
			}
		}
		queue = queue[1:]
	}

	return BFSNoPath
}
