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

// Getting about, ported from act.wizard.c.
//
// `goto` and `at` for yourself, `transfer` and `teleport` for other people,
// `invis` for not being seen doing it, and `poofin`/`poofout` for saying
// something when you arrive. The first slice of the wizard commands, and the
// one everything else in that file assumes works.

// findTargetRoom is find_target_room (act.wizard.c:150).
//
// A room number, or the name of somebody or something to go to. It is shared
// by `goto`, `at` and `teleport`, and it is where the four restrictions on
// where a god may go live — all of which a greater god ignores.
func (c *Context) findTargetRoom(arg string) (game.RoomVnum, bool) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		c.Send("You must supply a room number or a name.\r\n")
		return game.NoRoom, false
	}

	var location game.RoomVnum

	// A bare number is a room vnum. The `!strchr(roomstr, '.')` is what stops
	// `3.guard` being read as room 3 — the dot form names the third guard.
	if isNumber(arg) && !strings.Contains(arg, ".") {
		location = game.RoomVnum(atoi(arg))
		if c.World.Room(location) == nil {
			c.Send("No room exists with that number.\r\n")
			return game.NoRoom, false
		}
	} else {
		switch target := c.findAnywhere(arg); {
		case target != nil:
			location = target.Room
		default:
			obj := c.findObjectAnywhere(arg)
			if obj == nil {
				c.Send("Nothing exists by that name.\r\n")
				return game.NoRoom, false
			}
			location = objectRoom(obj)
			if location == game.NoRoom {
				c.Send("That object is currently not in a room.\r\n")
				return game.NoRoom, false
			}
		}
	}

	// A greater god goes anywhere, and the check is *after* the room has been
	// found — so the messages below are only ever seen by a lesser immortal.
	if levelOf(c.Character) >= game.LevelGreaterGod {
		return location, true
	}

	room := c.World.Room(location)
	switch {
	case room.Flags.Has(game.RoomGodRoom):
		c.Send("You are not godly enough to use that room!\r\n")
	case room.Flags.Has(game.RoomPrivate) && len(c.World.Occupants(location)) > 1:
		// Two people in a private room is a conversation. One is not.
		c.Send("There's a private conversation going on in that room.\r\n")
	case room.Flags.Has(game.RoomHouse) && !c.World.HouseCanEnter(c.Character, location):
		c.Send("That's private property -- no trespassing!\r\n")
	default:
		return location, true
	}
	return game.NoRoom, false
}

// findObjectAnywhere is get_obj_vis with FIND_OBJ_WORLD: the first object in
// the world answering to a name.
//
// The C walks `object_list`, which is in creation order. Live.Objects() comes
// out of a map and so is in no order at all, which means two objects of the
// same name can be found in either order. It matters only for `goto sword`
// with several swords about, and the sorting is not worth the cost — noted
// in docs/deviations.md rather than papered over.
func (c *Context) findObjectAnywhere(name string) *game.Object {
	return c.findObject(c.World.Objects(), name)
}

// objectRoom is where an object effectively is: the room it lies in, or the
// room whoever holds it is standing in.
func objectRoom(obj *game.Object) game.RoomVnum {
	switch {
	case obj.Location == game.InRoom:
		return obj.Room
	case obj.Holder != nil:
		return obj.Holder.Room
	}
	return game.NoRoom
}

// doGoto is do_goto (act.wizard.c:249).
func doGoto(c *Context) error {
	location, ok := c.findTargetRoom(c.Arg)
	if !ok {
		return nil
	}

	c.announce("%s %s\r\n", c.Character.Name, poofOut(c.Character))
	if err := c.World.Enter(c.Character, location); err != nil {
		c.Send("You cannot go there.\r\n")
		return nil
	}
	c.announce("%s %s\r\n", c.Character.Name, poofIn(c.Character))

	if room := c.World.Room(location); room != nil {
		sendRoomInfo(c.Session, room)
		c.Send("%s", roomDescription(c.World, room, c.Character, false))
	}
	return nil
}

// poofIn and poofOut are POOFIN and POOFOUT with the C's defaults.
//
// Both are stored *outside* `player_special_data_saved` (structs.h:899), so
// they are not in the player file and do not survive a reboot. Every god who
// ever set one had to set it again the next time the server came up, and
// nobody seems to have minded.
func poofIn(c *game.Character) string {
	if c.Record != nil && c.Record.PoofIn != "" {
		return c.Record.PoofIn
	}
	return "appears with an ear-splitting bang."
}

func poofOut(c *game.Character) string {
	if c.Record != nil && c.Record.PoofOut != "" {
		return c.Record.PoofOut
	}
	return "disappears in a puff of smoke."
}

// doAt is do_at (act.wizard.c:216): run one command somewhere else.
func doAt(c *Context) error {
	where, command := halfChop(c.Arg)
	if where == "" {
		c.Send("You must supply a room number or a name.\r\n")
		return nil
	}
	if command == "" {
		c.Send("What do you want to do there?\r\n")
		return nil
	}

	location, ok := c.findTargetRoom(where)
	if !ok {
		return nil
	}

	original := c.Character.Room
	if err := c.World.Enter(c.Character, location); err != nil {
		c.Send("You cannot go there.\r\n")
		return nil
	}

	c.runAs(c.Character, command)

	// "check if the char is still there" — a command that moved you, or
	// killed you, leaves you where it put you rather than snapping back.
	if c.Character.Room == location {
		if err := c.World.Enter(c.Character, original); err != nil {
			c.Send("You cannot get back.\r\n")
		}
	}
	return nil
}

// runAs runs a command line as somebody, which is command_interpreter(ch, ...)
// — the same seam `order` uses.
func (c *Context) runAs(who *game.Character, line string) {
	word, arg := split(line)
	cmd := lookupFor(word, levelOf(who))
	if cmd == nil {
		who.Tell("Huh?!?\r\n")
		return
	}
	nested := *c
	nested.Character = who
	nested.Arg = arg
	nested.Social = cmd.Social
	if who != c.Character {
		nested.Session = nil
	}
	if err := cmd.Run(&nested); err != nil {
		who.Tell("Huh?!?\r\n")
	}
}

// doTransfer is do_trans (act.wizard.c:276): bring somebody to you.
func doTransfer(c *Context) error {
	name, _ := oneArgument(c.Arg)
	if name == "" {
		c.Send("Whom do you wish to transfer?\r\n")
		return nil
	}

	if !strings.EqualFold(name, "all") {
		victim := c.findAnywhere(name)
		switch {
		case victim == nil:
			c.Send("%s", noPerson)
		case victim == c.Character:
			c.Send("That doesn't make much sense, does it?\r\n")
		case levelOf(c.Character) < levelOf(victim) && !victim.IsNPC():
			c.Send("Go transfer someone your own size.\r\n")
		default:
			c.bringHere(victim, "%s has transferred you!\r\n")
		}
		return nil
	}

	// `transfer all`, which is a greater god's privilege.
	if levelOf(c.Character) < game.LevelGreaterGod {
		c.Send("I think not.\r\n")
		return nil
	}
	for _, victim := range c.World.Players() {
		if victim == c.Character || levelOf(victim) >= levelOf(c.Character) {
			continue
		}
		c.bringHere(victim, "%s has transferred you!\r\n")
	}
	c.Send("Okay.\r\n")
	return nil
}

// bringHere moves somebody to the actor's room with the mushroom cloud.
func (c *Context) bringHere(victim *game.Character, told string) {
	announce(c.World, victim.Room, victim, "%s disappears in a mushroom cloud.\r\n", victim.Name)
	if err := c.World.Enter(victim, c.Character.Room); err != nil {
		c.Send("They cannot be moved there.\r\n")
		return
	}
	announce(c.World, victim.Room, victim, "%s arrives from a puff of smoke.\r\n", victim.Name)
	victim.Tell(told, c.Character.Name)
	if room := c.World.Room(victim.Room); room != nil {
		victim.Tell("%s", roomDescription(c.World, room, victim, false))
	}
}

// doTeleport is do_teleport (act.wizard.c:325): send somebody somewhere.
func doTeleport(c *Context) error {
	name, where, _ := twoArguments(c.Arg)
	if name == "" {
		c.Send("Whom do you wish to teleport?\r\n")
		return nil
	}
	victim := c.findAnywhere(name)
	switch {
	case victim == nil:
		c.Send("%s", noPerson)
		return nil
	case victim == c.Character:
		c.Send("Use 'goto' to teleport yourself.\r\n")
		return nil
	case levelOf(victim) >= levelOf(c.Character):
		c.Send("Maybe you shouldn't do that.\r\n")
		return nil
	case where == "":
		c.Send("Where do you wish to send this person?\r\n")
		return nil
	}

	target, ok := c.findTargetRoom(where)
	if !ok {
		return nil
	}

	c.Send("Okay.\r\n")
	announce(c.World, victim.Room, victim, "%s disappears in a puff of smoke.\r\n", victim.Name)
	if err := c.World.Enter(victim, target); err != nil {
		c.Send("They cannot be moved there.\r\n")
		return nil
	}
	announce(c.World, target, victim, "%s arrives from a puff of smoke.\r\n", victim.Name)
	victim.Tell("%s has teleported you!\r\n", c.Character.Name)
	if room := c.World.Room(target); room != nil {
		victim.Tell("%s", roomDescription(c.World, room, victim, false))
	}
	return nil
}

// doInvis is do_invis (act.wizard.c:1589).
func doInvis(c *Context) error {
	if c.Character.IsNPC() {
		c.Send("You can't do that!\r\n")
		return nil
	}
	arg, _ := oneArgument(c.Arg)

	if arg == "" {
		// No argument toggles: visible if you are hidden, hidden at your own
		// level if you are not.
		if invisLevel(c.Character) > 0 {
			c.becomeVisible()
		} else {
			c.becomeInvisible(levelOf(c.Character))
		}
		return nil
	}

	level := atoi(arg)
	switch {
	case level > levelOf(c.Character):
		c.Send("You can't go invisible above your own level.\r\n")
	case level < 1:
		c.becomeVisible()
	default:
		c.becomeInvisible(level)
	}
	return nil
}

// becomeInvisible is perform_immort_invis (act.wizard.c:1568).
//
// The two messages are sent to the people whose view of you *changes*, which
// is a narrower set than "everybody here": going from invis 34 to invis 32
// tells the level 32s and 33s that you have appeared and says nothing to
// anybody else.
func (c *Context) becomeInvisible(level int32) {
	was := invisLevel(c.Character)
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other == c.Character {
			continue
		}
		switch {
		case levelOf(other) >= was && levelOf(other) < level:
			other.Tell("You blink and suddenly realize that %s is gone.\r\n", c.Character.Name)
		case levelOf(other) < was && levelOf(other) >= level:
			other.Tell("You suddenly realize that %s is standing beside you.\r\n", c.Character.Name)
		}
	}

	if c.Character.Record != nil {
		c.Character.Record.InvisLevel = level
	}
	c.Send("Your invisibility level is %d.\r\n", level)
}

// becomeVisible is perform_immort_vis (act.wizard.c:1554).
func (c *Context) becomeVisible() {
	if invisLevel(c.Character) == 0 {
		c.Send("You are already fully visible.\r\n")
		return
	}
	if c.Character.Record != nil {
		c.Character.Record.InvisLevel = 0
	}
	c.Send("You are now fully visible.\r\n")
	c.announce("You suddenly realize that %s is standing beside you.\r\n", c.Character.Name)
}

func invisLevel(who *game.Character) int32 {
	if who == nil || who.Record == nil {
		return 0
	}
	return who.Record.InvisLevel
}

// doPoofIn and doPoofOut are do_poofset (act.wizard.c:1638).
func doPoofIn(c *Context) error  { return c.setPoof(&c.Character.Record.PoofIn) }
func doPoofOut(c *Context) error { return c.setPoof(&c.Character.Record.PoofOut) }

func (c *Context) setPoof(field *string) error {
	if c.Character.Record == nil {
		return nil
	}
	*field = strings.TrimSpace(c.Arg)
	c.Send("Okay.\r\n")
	return nil
}

// noPerson is NOPERSON (config.c).
const noPerson = "No-one by that name here.\r\n"
