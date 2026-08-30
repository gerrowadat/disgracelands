// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"github.com/gerrowadat/disgracelands/internal/colour"
	"github.com/gerrowadat/disgracelands/internal/game"
)

// doColour is do_color (act.informative.c:1645).
//
// The setting is two preference bits read as one two-bit number, which is why
// the C writes the assignment as arithmetic: the bits are not adjacent in the
// flags word and cannot be masked out together.
//
// Matching is `search_block(arg, ctypes, FALSE)`, and the FALSE means *prefix*
// — so `c` is Complete, `s` is Sparse, and `o` is Off. `n` is Normal. There is
// no ambiguity to resolve because the four initials differ.
func doColour(c *Context) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}
	arg, _ := oneArgument(c.Arg)

	current := colour.LevelOf(
		rec.Preferences.Has(game.PrefColour1), rec.Preferences.Has(game.PrefColour2))

	if arg == "" {
		c.Send("Your current color level is %s.\r\n", colour.Names[current])
		return nil
	}

	level, ok := colour.ParseLevel(arg)
	if !ok {
		c.Send("Usage: color { Off | Sparse | Normal | Complete }\r\n")
		return nil
	}

	one, two := colour.Bits(level)
	rec.Preferences = rec.Preferences.Without(game.PrefColour1, game.PrefColour2)
	if one {
		rec.Preferences = rec.Preferences.With(game.PrefColour1)
	}
	if two {
		rec.Preferences = rec.Preferences.With(game.PrefColour2)
	}

	// The confirmation colours the word "color" itself, at C_SPR — so it is
	// red for anybody who asked for any colour at all, and the reset is at
	// C_OFF so it goes out even for somebody who has just switched colour off.
	// That is the C being careful: without the unconditional reset, turning
	// colour off would leave the terminal red.
	c.SendAt(colour.Sparse, "Your {{red}}color")
	c.SendAt(colour.Off, "{{/}}")
	c.Send(" is now %s.\r\n", colour.Names[level])
	return nil
}
