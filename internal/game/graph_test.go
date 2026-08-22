// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

// The breadth-first search behind `track`.

// gridWorld builds a 4x4 grid of rooms numbered 100 + row*10 + col, joined
// north/south and east/west. Room 100 is the top-left corner; north
// decreases the row.
func gridWorld(t *testing.T) *Live {
	t.Helper()

	rooms := map[RoomVnum]*RoomDef{}
	vnum := func(row, col int) RoomVnum { return RoomVnum(100 + row*10 + col) }

	var list []*RoomDef
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			r := &RoomDef{Vnum: vnum(row, col), Name: "A room"}
			rooms[r.Vnum] = r
			list = append(list, r)
		}
	}
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			r := rooms[vnum(row, col)]
			if row > 0 {
				r.Exits[North] = &ExitDef{ToRoom: vnum(row-1, col)}
			}
			if row < 3 {
				r.Exits[South] = &ExitDef{ToRoom: vnum(row+1, col)}
			}
			if col > 0 {
				r.Exits[West] = &ExitDef{ToRoom: vnum(row, col-1)}
			}
			if col < 3 {
				r.Exits[East] = &ExitDef{ToRoom: vnum(row, col+1)}
			}
		}
	}

	return NewLive(&World{Rooms: list})
}

func TestFindFirstStepOnAGrid(t *testing.T) {
	w := gridWorld(t)

	for _, tc := range []struct {
		name     string
		from, to RoomVnum
		want     int
	}{
		{"one room east", 100, 101, int(East)},
		{"one room south", 100, 110, int(South)},
		{"one room west", 101, 100, int(West)},
		{"one room north", 110, 100, int(North)},
		// The far corner is three east and three south. Both are shortest, so
		// the answer is whichever direction the C's loop reaches first — and
		// the loop runs in compass order, north east south west, so east
		// beats south.
		{"the far corner", 100, 133, int(East)},
		{"straight across", 100, 103, int(East)},
		{"straight down", 100, 130, int(South)},
	} {
		if got := w.FindFirstStep(tc.from, tc.to); got != tc.want {
			t.Errorf("%s: from %d to %d gave %d, want %d", tc.name, tc.from, tc.to, got, tc.want)
		}
	}
}

func TestFindFirstStepEdgeCases(t *testing.T) {
	w := gridWorld(t)

	if got := w.FindFirstStep(100, 100); got != BFSAlreadyThere {
		t.Errorf("searching from a room to itself gave %d, want BFSAlreadyThere", got)
	}
	if got := w.FindFirstStep(99999, 100); got != BFSError {
		t.Errorf("searching from a room that does not exist gave %d, want BFSError", got)
	}
	if got := w.FindFirstStep(100, 99999); got != BFSError {
		t.Errorf("searching to a room that does not exist gave %d, want BFSError", got)
	}
}

// An island with no way in is BFSNoPath rather than an error.
func TestNoPathToAnUnreachableRoom(t *testing.T) {
	w := gridWorld(t)
	island := &RoomDef{Vnum: 900, Name: "An island"}
	w.rooms[island.Vnum] = island

	if got := w.FindFirstStep(100, 900); got != BFSNoPath {
		t.Errorf("searching to an unreachable room gave %d, want BFSNoPath", got)
	}
}

// A NOTRACK room is not walked through, so it can cut a grid in half.
func TestNoTrackRoomsAreNotSearchedThrough(t *testing.T) {
	w := gridWorld(t)

	// Wall off the second row, leaving 130 reachable only the long way — and
	// since every route through row 1 is blocked, not at all.
	for col := 0; col < 4; col++ {
		w.Room(RoomVnum(110 + col)).Flags = w.Room(RoomVnum(110 + col)).Flags.Set(RoomNoTrack)
	}
	if got := w.FindFirstStep(100, 130); got != BFSNoPath {
		t.Errorf("searching across a wall of NOTRACK rooms gave %d, want BFSNoPath", got)
	}
	// The unwalled half is still fine.
	if got := w.FindFirstStep(100, 103); got != int(East) {
		t.Errorf("searching within the reachable half gave %d, want east", got)
	}
}

// track_through_doors is YES on this server, so a shut door does not break a
// trail. With it off, it does.
func TestClosedDoorsDependOnTheSetting(t *testing.T) {
	w := gridWorld(t)

	// Shut every door out of the top row's second column, so the only way
	// down is through row 1.
	for col := 0; col < 4; col++ {
		r := w.Room(RoomVnum(100 + col))
		if e := r.Exits[South]; e != nil {
			e.State = e.State.Set(ExitClosed)
		}
	}

	// The setting is per-world now rather than a package variable, so this
	// needs no saving and restoring — and two tests can run at once.
	if !w.TrackThroughDoors() {
		t.Error("a fresh world should track through doors, as this server did")
	}
	if got := w.FindFirstStep(100, 110); got != int(South) {
		t.Errorf("with track_through_doors on, a shut door gave %d, want south", got)
	}

	w.SetTrackThroughDoors(false)
	if got := w.FindFirstStep(100, 110); got != BFSNoPath {
		t.Errorf("with track_through_doors off, a shut door gave %d, want BFSNoPath", got)
	}
}
