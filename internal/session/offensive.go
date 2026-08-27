// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import "github.com/gerrowadat/disgracelands/internal/game"

// Starting a fight: do_hit, do_kill and do_assist (act.offensive.c).
//
// `hit` does not put you into a fight and wait for the round — it swings
// immediately and costs three rounds of lag, which is why opening with `hit`
// and then `kick` is slower than it looks. `kill` is the same command for
// everybody below implementor, and for an implementor it is something else
// entirely: an instant, unanswerable killing.

func doHit(c *Context) error    { return c.attack(false) }
func doMurder(c *Context) error { return c.attack(true) }

// attack, porting do_hit. The murder subcommand differs in one place: it is
// the only way to start a fight with another player while player-killing is
// off.
func (c *Context) attack(murder bool) error {
	name, _ := oneArgument(c.Arg)
	if name == "" {
		c.Send("Hit who?\r\n")
		return nil
	}

	victim := c.findInRoom(name)
	switch {
	case victim == nil:
		c.Send("They don't seem to be here.\r\n")
		return nil
	case victim == c.Character:
		c.Send("You hit yourself...OUCH!.\r\n")
		c.announce("%s hits %sself, and says OUCH!\r\n", c.Character.Name, c.Character.Objective())
		return nil
	case c.Character.Charmed() && c.Character.Master == victim:
		c.Send("%s is just such a good friend, you simply can't hit %s.\r\n",
			victim.Name, victim.Objective())
		return nil
	}

	if !pkAllowed {
		if !victim.IsNPC() && !c.Character.IsNPC() && !murder {
			c.Send("Use 'murder' to hit another player.\r\n")
			return nil
		}
		// A charmed pet cannot be ordered to attack a player. The C returns
		// in silence, so the order simply does nothing.
		if c.Character.Charmed() && c.Character.Master != nil &&
			!c.Character.Master.IsNPC() && !victim.IsNPC() {
			return nil
		}
	}

	if c.Character.Position != game.PosStanding || victim == c.Character.Fighting {
		c.Send("You do the best you can!\r\n")
		return nil
	}
	if c.medusaLooksAt(victim) {
		return nil
	}

	c.Violence.Swing(c.World, c.Character, victim)
	// PULSE_VIOLENCE + 2 in the C, where the pulse is 2 seconds and the unit
	// is pulses — so it is a round and a fraction rather than three rounds.
	// Rounded to the round here, which is the unit Wait takes.
	c.Character.Wait(1, c.roundLength())
	return nil
}

// doKill, porting do_kill.
//
// For anybody below implementor this *is* `hit` — the C hands the argument
// straight over. For an implementor it is the god-killing: no roll, no fight,
// no chance to run.
func doKill(c *Context) error {
	if c.Character.Level() < game.LevelImplementor || c.Character.IsNPC() {
		return c.attack(false)
	}

	name, _ := oneArgument(c.Arg)
	if name == "" {
		c.Send("Kill who?\r\n")
		return nil
	}

	victim := c.findInRoom(name)
	if victim == nil {
		c.Send("They aren't here.\r\n")
		return nil
	}
	if victim == c.Character {
		c.Send("Your mother would be so sad.. :(\r\n")
		return nil
	}

	c.Send("You chop %s to pieces!  Ah!  The blood!\r\n", victim.Objective())
	victim.Tell("%s chops you to pieces!\r\n", c.Character.Name)
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != victim {
			other.Tell("%s brutally slays %s!\r\n", c.Character.Name, victim.Name)
		}
	}

	// raw_kill, and not damage() with a big number. The difference is
	// visible: damage() announces the new position first — "$n is dead!
	// R.I.P." to the room, "You are dead!  Sorry..." to the victim — and
	// raw_kill announces nothing but the death cry. Routing this through
	// Damage printed both, and then die() printed the R.I.P. a second time.
	c.Violence.RawKill(c.World, victim)
	return nil
}

// doAssist, porting do_assist: join whatever fight somebody else is in.
func doAssist(c *Context) error {
	if c.Character.Fighting != nil {
		c.Send("You're already fighting!  How can you assist someone else?\r\n")
		return nil
	}

	name, _ := oneArgument(c.Arg)
	if name == "" {
		c.Send("Whom do you wish to assist?\r\n")
		return nil
	}

	helpee := c.findInRoom(name)
	if helpee == nil {
		c.Send("No-one by that name here.\r\n")
		return nil
	}
	if helpee == c.Character {
		c.Send("You can't help yourself any more than this!\r\n")
		return nil
	}

	// Whoever they are fighting — or, if they are not fighting anybody,
	// whoever is fighting *them*.
	opponent := helpee.Fighting
	if opponent == nil {
		for _, other := range c.World.Occupants(c.Character.Room) {
			if other.Fighting == helpee {
				opponent = other
				break
			}
		}
	}

	switch {
	case opponent == nil:
		c.Send("But nobody is fighting %s!\r\n", helpee.Objective())
		return nil
	case !pkAllowed && !opponent.IsNPC():
		c.Send("Use 'murder' if you really want to attack %s.\r\n", opponent.Name)
		return nil
	}
	if c.medusaLooksAt(opponent) {
		return nil
	}

	c.Send("You join the fight!\r\n")
	helpee.Tell("%s assists you!\r\n", c.Character.Name)
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != helpee {
			other.Tell("%s assists %s.\r\n", c.Character.Name, helpee.Name)
		}
	}

	c.Violence.Swing(c.World, c.Character, opponent)
	return nil
}
