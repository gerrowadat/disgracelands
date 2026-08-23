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

// doReloadMob is `reloadmob <vnum>` — new capability, not a port of
// anything in interpreter.c: it re-reads one mobile's definition from
// the world data on disk and applies it to the running server without a
// restart. Everything it does is documented on MobReloader
// (internal/session/commands.go) and game.Live.ReloadMobile
// (internal/game/reset.go) — this is just the command wrapping.
func doReloadMob(c *Context) error {
	arg, _ := oneArgument(c.Arg)
	if arg == "" {
		c.Send("Reload which mobile?\r\n")
		return nil
	}
	if !isNumber(arg) {
		c.Send("That is not a vnum.\r\n")
		return nil
	}
	vnum := game.MobVnum(atoi(arg))

	if c.MobReload == nil {
		c.Send("Mobile reload is not available.\r\n")
		return nil
	}

	refreshed, err := c.MobReload.ReloadMobile(c.World, vnum)
	if err != nil {
		switch {
		case errors.Is(err, ErrMobEngaged):
			c.Send("Mob #%d is in combat; try again once the fight is over.\r\n", vnum)
		default:
			c.Send("Could not reload mob #%d: %s\r\n", vnum, err)
		}
		return nil
	}

	c.Send("Reloaded mob #%d — %d instance(s) refreshed.\r\n", vnum, refreshed)
	return nil
}
