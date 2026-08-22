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

// do_enter, do_leave and do_order, ported from act.movement.c:697, :731 and
// act.offensive.c:396.
//
// `enter` and `leave` are movement by *description* rather than by compass:
// walk in through the door, walk back out into the open. Neither takes a
// direction and both work by looking at the room flags of what is next door.

// doEnter is do_enter (act.movement.c:697).
func doEnter(c *Context) error {
	name, _ := oneArgument(c.Arg)

	if name != "" {
		// A named door. The C compares the *whole* keyword string with
		// str_cmp rather than matching one of its words, so a door whose
		// keyword is "gate portal" is entered by typing the whole of that and
		// not by typing "gate".
		for dir := game.Direction(0); int(dir) < game.NumDirections; dir++ {
			exit := c.World.Exit(c.Character.Room, dir)
			if exit != nil && exit.Keywords != "" && strings.EqualFold(exit.Keywords, name) {
				c.moveCharacter(c.Character, dir)
				return nil
			}
		}
		c.Send("There is no %s here.\r\n", name)
		return nil
	}

	room := c.World.Room(c.Character.Room)
	if room != nil && room.Flags.Has(game.RoomIndoors) {
		c.Send("You are already indoors.\r\n")
		return nil
	}

	// The first open exit into somewhere indoors, in compass order — so
	// `enter` with two buildings next to you always picks the northerly one.
	for dir := game.Direction(0); int(dir) < game.NumDirections; dir++ {
		if dest := c.openExitTo(dir); dest != nil && dest.Flags.Has(game.RoomIndoors) {
			c.moveCharacter(c.Character, dir)
			return nil
		}
	}
	c.Send("You can't seem to find anything to enter.\r\n")
	return nil
}

// doLeave is do_leave (act.movement.c:731): the mirror of enter with no
// argument.
func doLeave(c *Context) error {
	room := c.World.Room(c.Character.Room)
	if room == nil || !room.Flags.Has(game.RoomIndoors) {
		c.Send("You are outside.. where do you want to go?\r\n")
		return nil
	}

	for dir := game.Direction(0); int(dir) < game.NumDirections; dir++ {
		if dest := c.openExitTo(dir); dest != nil && !dest.Flags.Has(game.RoomIndoors) {
			c.moveCharacter(c.Character, dir)
			return nil
		}
	}
	c.Send("I see no obvious exits to the outside.\r\n")
	return nil
}

// openExitTo returns the room through an exit, or nil when there is no exit,
// nowhere behind it, or the door is shut.
func (c *Context) openExitTo(dir game.Direction) *game.RoomDef {
	exit := c.World.Exit(c.Character.Room, dir)
	if exit == nil || exit.ToRoom == game.NoRoom || exit.State.Has(game.ExitClosed) {
		return nil
	}
	return c.World.Room(exit.ToRoom)
}

// doOrder is do_order (act.offensive.c:396).
//
// Telling a charmed follower what to do. Anybody can be *told*; only a
// charmed follower of yours actually does it, and everybody else in the room
// watches them look blank.
func doOrder(c *Context) error {
	name, message := halfChop(c.Arg)
	who := c.Character

	if name == "" || message == "" {
		c.Send("Order who to do what?\r\n")
		return nil
	}

	victim := c.World.FindInRoom(who, who.Room, name)
	toFollowers := isPrefixOf(name, "followers")
	if victim == nil && !toFollowers {
		c.Send("That person isn't here.\r\n")
		return nil
	}
	if victim == who {
		c.Send("You obviously suffer from skitzofrenia.\r\n")
		return nil
	}
	// Somebody else's puppet does not get to issue orders of their own.
	if who.Charmed() {
		c.Send("Your superior would not aprove of you giving orders.\r\n")
		return nil
	}

	if victim != nil {
		victim.Tell("%s orders you to '%s'\r\n", who.Name, message)
		c.announceExcept(victim, "%s gives %s an order.\r\n", who.Name, victim.Name)

		if victim.Master != who || !victim.Charmed() {
			// Note who this is sent about: the *victim*, to everyone
			// including the person who gave the order.
			for _, other := range c.World.Occupants(who.Room) {
				if other != victim {
					other.Tell("%s has an indifferent look.\r\n", victim.Name)
				}
			}
			return nil
		}
		c.Send("Okay.\r\n")
		c.runOrder(victim, message)
		return nil
	}

	// `order followers <something>`: everybody charmed and present.
	c.announce("%s issues the order '%s'.\r\n", who.Name, message)

	found := false
	for _, follower := range append([]*game.Character(nil), who.Followers...) {
		if follower.Room == who.Room && follower.Charmed() {
			found = true
			c.runOrder(follower, message)
		}
	}
	if found {
		c.Send("Okay.\r\n")
	} else {
		c.Send("Nobody here is a loyal subject of yours!\r\n")
	}
	return nil
}

// runOrder is command_interpreter(vict, message): the ordered character runs
// the command as if they had typed it.
//
// A mobile has no session, so the Context it gets has none either — which is
// exactly the case Context.Send already handles by writing to the character's
// client, and a mobile's client is nobody.
func (c *Context) runOrder(victim *game.Character, line string) {
	word, arg := split(line)
	cmd := lookup(word)
	if cmd == nil {
		victim.Tell("Huh?!?\r\n")
		return
	}
	ordered := *c
	ordered.Character = victim
	ordered.Session = nil
	ordered.Arg = arg
	ordered.Social = cmd.Social
	if err := cmd.Run(&ordered); err != nil {
		victim.Tell("Huh?!?\r\n")
	}
}
