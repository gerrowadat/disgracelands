// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// The commands that change what a character is doing, ported from
// act.movement.c's do_stand/do_sit/do_rest/do_sleep/do_wake and
// act.offensive.c's do_flee.
//
// Positions already drove the combat multiplier and the regeneration rates;
// until now nothing could change one. Every branch of every switch below is
// the C's, including the "default" arms that only a floating character
// reaches and the several different ways of refusing to do something while
// fighting.

func doStand(c *Context) error {
	switch c.Character.Position {
	case game.PosStanding:
		c.Send("You are already standing.\r\n")
	case game.PosSitting:
		c.Send("You stand up.\r\n")
		c.announce("%s clambers to their feet.\r\n", c.Character.Name)
		// Sitting after a bash, and possibly still in the fight.
		if c.Character.Fighting != nil {
			c.Character.Position = game.PosFighting
		} else {
			c.Character.Position = game.PosStanding
		}
	case game.PosResting:
		c.Send("You stop resting, and stand up.\r\n")
		c.announce("%s stops resting, and clambers on their feet.\r\n", c.Character.Name)
		c.Character.Position = game.PosStanding
	case game.PosSleeping:
		c.Send("You have to wake up first!\r\n")
	case game.PosFighting:
		c.Send("Do you not consider fighting as standing?\r\n")
	default:
		c.Send("You stop floating around, and put your feet on the ground.\r\n")
		c.announce("%s stops floating around, and puts their feet on the ground.\r\n", c.Character.Name)
		c.Character.Position = game.PosStanding
	}
	return nil
}

func doSit(c *Context) error {
	switch c.Character.Position {
	case game.PosStanding:
		c.Send("You sit down.\r\n")
		c.announce("%s sits down.\r\n", c.Character.Name)
		c.Character.Position = game.PosSitting
	case game.PosSitting:
		c.Send("You're sitting already.\r\n")
	case game.PosResting:
		c.Send("You stop resting, and sit up.\r\n")
		c.announce("%s stops resting.\r\n", c.Character.Name)
		c.Character.Position = game.PosSitting
	case game.PosSleeping:
		c.Send("You have to wake up first.\r\n")
	case game.PosFighting:
		c.Send("Sit down while fighting? Are you MAD?\r\n")
	default:
		c.Send("You stop floating around, and sit down.\r\n")
		c.announce("%s stops floating around, and sits down.\r\n", c.Character.Name)
		c.Character.Position = game.PosSitting
	}
	return nil
}

func doRest(c *Context) error {
	switch c.Character.Position {
	case game.PosStanding:
		c.Send("You sit down and rest your tired bones.\r\n")
		c.announce("%s sits down and rests.\r\n", c.Character.Name)
		c.Character.Position = game.PosResting
	case game.PosSitting:
		c.Send("You rest your tired bones.\r\n")
		c.announce("%s rests.\r\n", c.Character.Name)
		c.Character.Position = game.PosResting
	case game.PosResting:
		c.Send("You are already resting.\r\n")
	case game.PosSleeping:
		c.Send("You have to wake up first.\r\n")
	case game.PosFighting:
		c.Send("Rest while fighting?  Are you MAD?\r\n")
	default:
		c.Send("You stop floating around, and stop to rest your tired bones.\r\n")
		c.announce("%s stops floating around, and rests.\r\n", c.Character.Name)
		// The C sets POS_SITTING here, not POS_RESTING — in the one branch
		// whose message says "rests". Reproduced.
		c.Character.Position = game.PosSitting
	}
	return nil
}

func doSleep(c *Context) error {
	switch c.Character.Position {
	case game.PosStanding, game.PosSitting, game.PosResting:
		c.Send("You go to sleep.\r\n")
		c.announce("%s lies down and falls asleep.\r\n", c.Character.Name)
		c.Character.Position = game.PosSleeping
	case game.PosSleeping:
		c.Send("You are already sound asleep.\r\n")
	case game.PosFighting:
		c.Send("Sleep while fighting?  Are you MAD?\r\n")
	default:
		c.Send("You stop floating around, and lie down to sleep.\r\n")
		c.announce("%s stops floating around, and lie down to sleep.\r\n", c.Character.Name)
		c.Character.Position = game.PosSleeping
	}
	return nil
}

// doWake wakes the character, or somebody else in the room.
func doWake(c *Context) error {
	if arg := strings.TrimSpace(c.Arg); arg != "" {
		return c.wakeSomebody(arg)
	}

	if c.Character.Record != nil && c.Character.Record.AffectFlags.Has(game.AffectSleep) {
		c.Send("You can't wake up!\r\n")
		return nil
	}
	if c.Character.Position > game.PosSleeping {
		c.Send("You are already awake...\r\n")
		return nil
	}

	c.Send("You awaken, and sit up.\r\n")
	c.announce("%s awakens.\r\n", c.Character.Name)
	c.Character.Position = game.PosSitting
	return nil
}

func (c *Context) wakeSomebody(arg string) error {
	if c.Character.Position == game.PosSleeping {
		c.Send("Maybe you should wake yourself up first.\r\n")
		return nil
	}

	victim := c.World.FindInRoom(c.Character.Room, arg)
	switch {
	case victim == nil:
		c.Send("No-one by that name here.\r\n")
	case victim == c.Character:
		// The C falls through to waking yourself, which cannot happen: the
		// sleeping case was refused above and an awake character is told they
		// already are.
		c.Send("You are already awake...\r\n")
	case victim.Position.Awake():
		c.Send("%s is already awake.\r\n", victim.Name)
	case victim.Record != nil && victim.Record.AffectFlags.Has(game.AffectSleep):
		c.Send("You can't wake %s up!\r\n", victim.Name)
	case victim.Position < game.PosSleeping:
		c.Send("%s is in pretty bad shape!\r\n", victim.Name)
	default:
		c.Send("You wake %s up.\r\n", victim.Name)
		victim.Tell("You are awakened by %s.\r\n", c.Character.Name)
		victim.Position = game.PosSitting
	}
	return nil
}

// doFlee runs away, porting do_flee (act.offensive.c).
//
// Six attempts at a random direction, and if none of them is a way out the
// character is stuck. Fleeing a fight costs experience — and a great deal of
// it: the opponent's missing hit points times their level.
func doFlee(c *Context) error {
	if c.Character.Position < game.PosFighting {
		c.Send("You are in pretty bad shape, unable to flee!\r\n")
		return nil
	}

	for attempt := 0; attempt < 6; attempt++ {
		roll := c.RNG.Number(0, game.NumDirections-1)
		if roll < 0 || roll >= game.NumDirections {
			continue
		}
		dir := game.Direction(roll)

		exit := c.World.Exit(c.Character.Room, dir)
		if exit == nil || exit.ToRoom == game.NoRoom || exit.State.Has(game.ExitClosed) {
			continue
		}
		destination := c.World.Room(exit.ToRoom)
		if destination == nil || destination.Flags.Has(game.RoomDeathTrap) {
			continue
		}

		c.announce("%s panics, and attempts to flee!\r\n", c.Character.Name)

		wasFighting := c.Character.Fighting
		from := c.Character.Room
		if err := c.World.Enter(c.Character, exit.ToRoom); err != nil {
			c.Send("You try to flee, but can't!\r\n")
			return nil
		}
		c.World.StopFighting(c.Character)
		if wasFighting != nil && wasFighting.Fighting == c.Character {
			c.World.StopFighting(wasFighting)
		}

		for _, other := range c.World.Occupants(from) {
			other.Tell("%s has fled!\r\n", c.Character.Name)
		}
		c.Send("You flee head over heels.\r\n")

		c.penaliseFlight(wasFighting)
		// do_flee's look passes ignore_brief = 0 (act.offensive.c:372), unlike
		// `look` typed by hand.
		return lookAtRoom(c, false)
	}

	c.Send("PANIC!  You couldn't escape!\r\n")
	return nil
}

// penaliseFlight takes the experience a flight costs.
//
// The figure is the opponent's *missing* hit points times their level, so
// fleeing a fight you were winning costs more than fleeing one you were
// losing — which is the point. The local rule from solo_gain applies here
// too: no experience changes hands between two players.
func (c *Context) penaliseFlight(wasFighting *game.Character) {
	if wasFighting == nil || c.Character.IsNPC() || c.Character.Record == nil {
		return
	}
	if !wasFighting.IsNPC() && !c.Character.IsNPC() {
		return
	}
	if wasFighting.Record == nil {
		return
	}

	loss := (wasFighting.Record.Points.MaxHit - wasFighting.Record.Points.Hit) *
		wasFighting.Record.Level
	if loss <= 0 {
		return
	}
	game.GainExperience(c.Character.Record, -loss, c.RNG)
}
