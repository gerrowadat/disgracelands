// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package classic

import (
	"fmt"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// parseRoom reads one room record, positioned just after its "#vnum" line.
// Mirrors parse_room() and setup_dir() in db.c.
func (l *loader) parseRoom(r *reader, vnum game.RoomVnum) (*game.RoomDef, error) {
	what := fmt.Sprintf("room #%d", vnum)
	room := &game.RoomDef{Vnum: vnum}

	var err error
	if room.Name, err = r.readString(what + " name"); err != nil {
		return nil, err
	}
	if room.Description, err = r.readString(what + " description"); err != nil {
		return nil, err
	}

	line, ok := r.getLine()
	if !ok {
		return nil, fmt.Errorf("%s: file ended before the flags/sector line", r.where(what))
	}

	// " %d %s %d ": a zone number the C loader reads and discards (the zone
	// files own that relationship now), the room flags, and the sector type.
	fields := splitFields(line)
	if len(fields) < 3 {
		return nil, fmt.Errorf("%s: malformed flags/sector line %q, want '<zone> <flags> <sector>'", r.where(what), line)
	}
	flags, unknown := game.ParseFlags(fields[1])
	room.Flags = flags
	if len(unknown) > 0 {
		l.warnf("%s: room flags %q contain characters that are neither letters nor digits (%q); the C loader ignores them", r.where(what), fields[1], string(unknown))
	}
	if flags.ExceedsCRange() {
		l.warnf("%s: room flags %q use bits above %d, which the C server cannot represent", r.where(what), fields[1], game.CFlagLimit)
	}
	sector, ok := scanInt(fields[2])
	if !ok {
		return nil, fmt.Errorf("%s: sector type %q is not a number", r.where(what), fields[2])
	}
	room.SectorType = sector

	for {
		line, ok := r.getLine()
		if !ok {
			return nil, fmt.Errorf("%s: file ended before the room's 'S' terminator", r.where(what))
		}
		switch line[0] {
		case 'D':
			dirNum, ok := scanInt(line[1:])
			if !ok {
				return nil, fmt.Errorf("%s: exit line %q has no direction number", r.where(what), line)
			}
			dir, inRange := game.DirectionFromInt(dirNum)
			// The record's three lines have to be consumed either way, or
			// everything after it misparses.
			exit, err := l.parseExit(r, vnum, dirNum)
			if err != nil {
				return nil, err
			}
			if !inRange {
				// The C loader indexes dir_option[] with this value
				// unchecked, so an out-of-range direction is a buffer
				// overrun there. Report it and drop the exit.
				l.warnf("%s: exit direction D%d is out of range (0-%d); ignoring it (the C loader would corrupt memory here)",
					r.where(what), dirNum, game.NumDirections-1)
				continue
			}
			if room.Exits[dir] != nil {
				l.warnf("%s: duplicate exit D%d (%s); the later one wins, as in the C loader", r.where(what), dirNum, dir)
			}
			room.Exits[dir] = exit

		case 'E':
			keywords, err := r.readString(what + " extra description keyword")
			if err != nil {
				return nil, err
			}
			desc, err := r.readString(what + " extra description")
			if err != nil {
				return nil, err
			}
			room.ExtraDescs = append(room.ExtraDescs, game.ExtraDesc{
				Keywords: keywords, Description: desc,
			})

		case 'S':
			// The C loader builds its extra-description list by prepending,
			// so at runtime it is in reverse file order. Anything that walks
			// the list and stops at the first keyword match therefore sees
			// the *last* matching description in the file. Reverse here to
			// keep that observable behaviour identical.
			reverseExtras(room.ExtraDescs)
			return room, nil

		default:
			return nil, fmt.Errorf("%s: expected D, E or S, got %q", r.where(what), line)
		}
	}
}

// parseExit reads the three lines of a direction record. dirNum is the raw
// scanned direction, used only for error messages: it may be out of range,
// and the record still has to be consumed so the rest of the file parses.
func (l *loader) parseExit(r *reader, vnum game.RoomVnum, dirNum int32) (*game.ExitDef, error) {
	what := fmt.Sprintf("room #%d exit D%d", vnum, dirNum)
	exit := &game.ExitDef{}

	var err error
	if exit.Description, err = r.readString(what + " description"); err != nil {
		return nil, err
	}
	if exit.Keywords, err = r.readString(what + " keywords"); err != nil {
		return nil, err
	}

	line, ok := r.getLine()
	if !ok {
		return nil, fmt.Errorf("%s: file ended before the exit's numeric line", r.where(what))
	}
	nums, err := requireInts(line, 3, what)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", r.where(what), err)
	}

	exit.DoorFlag = nums[0]
	exit.Key = game.ObjVnum(nums[1])
	exit.ToRoom = game.RoomVnum(nums[2])

	// The C loader treats any value other than 1 or 2 as "no door" without
	// comment, so a typo'd 3 silently becomes a doorless exit.
	if exit.DoorFlag < 0 || exit.DoorFlag > 2 {
		l.warnf("%s: door flag %d is not 0, 1 or 2; treated as no door", r.where(what), exit.DoorFlag)
		exit.DoorFlag = 0
	}
	return exit, nil
}

func reverseExtras(e []game.ExtraDesc) {
	for i, j := 0, len(e)-1; i < j; i, j = i+1, j-1 {
		e[i], e[j] = e[j], e[i]
	}
}

// trimTilde removes a trailing '~' and anything after it, matching the way
// load_zones() takes the terminator off a zone name.
func trimTilde(s string) string {
	if i := strings.IndexByte(s, '~'); i >= 0 {
		return s[:i]
	}
	return s
}
