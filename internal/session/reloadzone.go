// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"errors"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// doReloadZone is `reloadzone <vnum>` — reloadmob's own zone-wide
// extension, new capability with no interpreter.c row at all. Everything
// it does is documented on ZoneReloader (commands.go) and
// game.Live.ReloadZone (internal/game/reset.go) — this is just the
// command wrapping, the same shape doReloadMob already has.
func doReloadZone(c *Context) error {
	arg, _ := oneArgument(c.Arg)
	if arg == "" {
		c.Send("Reload which zone?\r\n")
		return nil
	}
	if !isNumber(arg) {
		c.Send("That is not a vnum.\r\n")
		return nil
	}
	vnum := game.ZoneVnum(atoi(arg))

	if c.ZoneReload == nil {
		c.Send("Zone reload is not available.\r\n")
		return nil
	}

	result, err := c.ZoneReload.ReloadZone(c.World, vnum)
	if err != nil {
		switch {
		case errors.Is(err, ErrZoneEngaged):
			c.Send("Zone #%d has a player in it, or something fighting; try again once it is clear.\r\n", vnum)
		default:
			c.Send("Could not reload zone #%d: %s\r\n", vnum, err)
		}
		return nil
	}

	c.Send("Reloaded zone #%d — %d room(s), %d mobile instance(s) refreshed.\r\n",
		vnum, result.Rooms, result.Mobiles)
	return nil
}
