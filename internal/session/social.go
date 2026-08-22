// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import "github.com/gerrowadat/disgracelands/internal/game"

// do_action, ported from act.social.c.
//
// One function behind a hundred commands. Which social ran is decided by
// which table entry was matched, and everything it says comes out of
// `data/misc/socials` — so this is the smallest command in the game and a
// third of what a player types.

// doAction runs a social, porting do_action.
func doAction(c *Context) error {
	social := c.Social
	if social == nil {
		// The C's answer when a command points at do_action and the file has
		// no entry for it. Two of the table's socials are in exactly that
		// state.
		c.Send("That action is not supported.\r\n")
		return nil
	}

	// A social that takes no target ignores its argument entirely — the C
	// only calls one_argument when char_found is set.
	var name string
	if social.TakesTarget() {
		name, _ = oneArgument(c.Arg)
	}

	if name == "" {
		c.Send("%s", c.act(social.CharNoArg, game.ActArgs{Actor: c.Character}, c.Character))
		c.toRoom(social.OthersNoArg, game.ActArgs{Actor: c.Character})
		return nil
	}

	victim := c.findInRoom(name)
	switch {
	case victim == nil:
		c.Send("%s", c.act(social.NotFound, game.ActArgs{Actor: c.Character}, c.Character))

	case victim == c.Character:
		c.Send("%s", c.act(social.CharAuto, game.ActArgs{Actor: c.Character}, c.Character))
		c.toRoom(social.OthersAuto, game.ActArgs{Actor: c.Character})

	case victim.Position < social.MinVictimPosition:
		// The one message do_action does not take from the file.
		c.Send("%s", c.act("$N is not in a proper position for that.",
			game.ActArgs{Actor: c.Character, Victim: victim}, c.Character))

	default:
		args := game.ActArgs{Actor: c.Character, Victim: victim}
		c.Send("%s", c.act(social.CharFound, args, c.Character))
		victim.Tell("%s", c.act(social.VictFound, args, victim))
		for _, other := range c.World.Occupants(c.Character.Room) {
			if other != c.Character && other != victim {
				other.Tell("%s", c.act(social.OthersFound, args, other))
			}
		}
	}
	return nil
}

// act renders a message for one audience. An empty format produces nothing at
// all, which is how a social with a blank line in the file stays silent.
func (c *Context) act(format string, args game.ActArgs, to *game.Character) string {
	if format == "" {
		return ""
	}
	return c.World.Act(format, args, to)
}

// toRoom renders a message once per listener, because the codes resolve
// differently for each of them.
func (c *Context) toRoom(format string, args game.ActArgs) {
	if format == "" {
		return
	}
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character {
			other.Tell("%s", c.World.Act(format, args, other))
		}
	}
}
