// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import "github.com/gerrowadat/disgracelands/internal/game"

// doInsult is do_insult (act.social.c), the one social that is a function
// rather than a row in the socials file.
//
// It is there because it picks its message: one of three at random, and the
// first of those branches four ways on the two sexes. Nothing in the socials
// file format can express that, which is why this command exists at all.
//
// The messages are 1993's and are reproduced as they are.
func doInsult(c *Context) error {
	name, _ := oneArgument(c.Arg)
	if name == "" {
		c.Send("I'm sure you don't want to insult *everybody*...\r\n")
		return nil
	}

	victim := c.findInRoom(name)
	if victim == nil {
		c.Send("Can't hear you!\r\n")
		return nil
	}
	if victim == c.Character {
		c.Send("You feel insulted.\r\n")
		return nil
	}

	c.Send("You insult %s.\r\n", victim.Name)

	var line string
	switch c.RNG.Number(0, 2) {
	case 0:
		// The only place in the game where both parties' sexes pick the
		// wording. A neuter insulter counts as a woman, because the C tests
		// `GET_SEX(ch) == SEX_MALE` and takes the else.
		switch {
		case c.Character.Sex() == game.SexMale && victim.Sex() == game.SexMale:
			line = "$n accuses you of fighting like a woman!"
		case c.Character.Sex() == game.SexMale:
			line = "$n says that women can't fight."
		case victim.Sex() == game.SexMale:
			line = "$n accuses you of having the smallest... (brain?)"
		default:
			line = "$n tells you that you'd lose a beauty contest against a troll."
		}
	case 1:
		line = "$n calls your mother a bitch!"
	default:
		line = "$n tells you to get lost!"
	}

	args := game.ActArgs{Actor: c.Character, Victim: victim}
	victim.Tell("%s", c.World.Act(line, args, victim))
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != victim {
			other.Tell("%s", c.World.Act("$n insults $N.", args, other))
		}
	}
	return nil
}
