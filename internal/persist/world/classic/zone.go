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

// parseZone reads a whole zone file. Mirrors load_zones() in db.c.
//
// Zone files have a different shape from the other three: one record per
// file, no "#vnum" dispatch loop, and a command list terminated by 'S'.
func (l *loader) parseZone(r *reader, filename string) (*game.ZoneDef, error) {
	zone := &game.ZoneDef{}

	line, ok := r.getLine()
	if !ok {
		return nil, fmt.Errorf("%s: empty zone file", filename)
	}
	if !strings.HasPrefix(line, "#") {
		return nil, fmt.Errorf("%s: expected '#<vnum>' on the first line, got %q", r.where(""), line)
	}
	vnum, ok := scanInt(line[1:])
	if !ok {
		return nil, fmt.Errorf("%s: zone number %q is not a number", r.where(""), line)
	}
	zone.Vnum = game.ZoneVnum(vnum)

	what := fmt.Sprintf("zone #%d", zone.Vnum)

	// The zone name is a plain line with a '~' terminator, read with
	// get_line rather than fread_string — so it is a single line and cannot
	// span several.
	if line, ok = r.getLine(); !ok {
		return nil, fmt.Errorf("%s: file ended before the zone name", r.where(what))
	}
	zone.Name = trimTilde(line)

	// " %hd %hd %d %d ": bottom vnum, top vnum, lifespan, reset mode.
	if line, ok = r.getLine(); !ok {
		return nil, fmt.Errorf("%s: file ended before the numeric line", r.where(what))
	}
	nums, err := requireInts(line, 4, "bottom, top, lifespan and reset mode")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", r.where(what), err)
	}
	zone.Bottom = game.RoomVnum(nums[0])
	zone.Top = game.RoomVnum(nums[1])
	zone.Lifespan = nums[2]
	zone.ResetMode = nums[3]

	if zone.Bottom > zone.Top {
		return nil, fmt.Errorf("%s: bottom vnum %d is above top vnum %d", r.where(what), zone.Bottom, zone.Top)
	}

	for {
		line, ok := r.getLine()
		if !ok {
			return nil, fmt.Errorf("%s: file ended before the zone's 'S' terminator", r.where(what))
		}
		// The C loader skips leading whitespace before reading the opcode,
		// so an indented command is valid.
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		opcode := trimmed[0]

		// '*' is a comment. get_line already drops lines that *start* with
		// '*', but an indented one survives to here.
		if opcode == '*' {
			continue
		}
		if opcode == 'S' || opcode == '$' {
			return zone, nil
		}

		cmd := game.ResetCommand{Command: opcode, Line: r.lineNo}
		want := cmd.NumArgs() + 1 // the if-flag, plus the opcode's arguments
		args := make([]int32, want)
		if got := scanInts(trimmed[1:], args); got != want {
			return nil, fmt.Errorf("%s: command %q on line %d wants %d numbers, got %d: %q",
				r.where(what), string(opcode), r.lineNo, want, got, line)
		}
		cmd.IfFlag = args[0]
		cmd.Arg1 = args[1]
		cmd.Arg2 = args[2]
		if want == 4 {
			cmd.Arg3 = args[3]
		}
		zone.Commands = append(zone.Commands, cmd)
	}
}
