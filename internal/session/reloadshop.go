// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import "github.com/gerrowadat/disgracelands/internal/game"

// doReloadShop is `reloadshop <vnum>` — new capability, not a port of
// anything in interpreter.c: it re-reads one shop's configuration from
// the world data on disk and applies it to the running server, without a
// restart. Everything it does is documented on ShopReloader
// (internal/session/commands.go) and game.Live.ReloadShop
// (internal/game/shopstate.go) — this is just the command wrapping, the
// same shape doReloadObject already has.
func doReloadShop(c *Context) error {
	arg, _ := oneArgument(c.Arg)
	if arg == "" {
		c.Send("Reload which shop?\r\n")
		return nil
	}
	if !isNumber(arg) {
		c.Send("That is not a vnum.\r\n")
		return nil
	}
	vnum := game.ShopVnum(atoi(arg))

	if c.ShopReload == nil {
		c.Send("Shop reload is not available.\r\n")
		return nil
	}

	if err := c.ShopReload.ReloadShop(c.World, vnum); err != nil {
		c.Send("Could not reload shop #%d: %s\r\n", vnum, err)
		return nil
	}

	c.Send("Reloaded shop #%d.\r\n", vnum)
	return nil
}
