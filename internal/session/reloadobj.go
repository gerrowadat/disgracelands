// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import "github.com/gerrowadat/disgracelands/internal/game"

// doReloadObject is `reloadobj <vnum>` — new capability, not a port of
// anything in interpreter.c: it re-reads one object's definition from
// the world data on disk and applies it to the running server's
// prototype, without a restart and without touching any already-spawned
// instance. Everything it does is documented on ObjectReloader
// (internal/session/commands.go) and game.Live.ReloadObject
// (internal/game/reset.go) — this is just the command wrapping.
func doReloadObject(c *Context) error {
	arg, _ := oneArgument(c.Arg)
	if arg == "" {
		c.Send("Reload which object?\r\n")
		return nil
	}
	if !isNumber(arg) {
		c.Send("That is not a vnum.\r\n")
		return nil
	}
	vnum := game.ObjVnum(atoi(arg))

	if c.ObjectReload == nil {
		c.Send("Object reload is not available.\r\n")
		return nil
	}

	if err := c.ObjectReload.ReloadObject(c.World, vnum); err != nil {
		c.Send("Could not reload object #%d: %s\r\n", vnum, err)
		return nil
	}

	c.Send("Reloaded object #%d. Instances already in the world keep what they are; new ones will use the fresh definition.\r\n", vnum)
	return nil
}
