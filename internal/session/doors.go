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

// Doors, ported from do_gen_door, do_doorcmd and ok_pick (act.movement.c).
//
// The C runs all five commands through one function with a table of
// preconditions per subcommand, which is why the refusals are so uniform:
// "But it's already closed!", "But it's currently open!", "Oh.. it wasn't
// locked, after all..", "It seems to be locked." Same arrangement here.
//
// The part worth getting right is that a door has two sides. Opening one
// opens the matching exit in the room beyond — but only if that exit points
// back, because two rooms can have one-way exits at each other.

// doorAction is one of the five things that can be done to a door.
type doorAction int

const (
	doorOpen doorAction = iota
	doorClose
	doorUnlock
	doorLock
	doorPick
)

// doorVerbs are cmd_door (act.movement.c:296).
var doorVerbs = [...]string{"open", "close", "unlock", "lock", "pick"}

// doorNeeds are flags_door: what state the door must be in for each action.
var doorNeeds = [...]struct{ open, closed, locked, unlocked bool }{
	doorOpen:   {closed: true, unlocked: true},
	doorClose:  {open: true},
	doorUnlock: {closed: true, locked: true},
	doorLock:   {closed: true, unlocked: true},
	doorPick:   {closed: true, locked: true},
}

func doOpen(c *Context) error   { return c.door(doorOpen) }
func doClose(c *Context) error  { return c.door(doorClose) }
func doLock(c *Context) error   { return c.door(doorLock) }
func doUnlock(c *Context) error { return c.door(doorUnlock) }
func doPick(c *Context) error   { return c.door(doorPick) }

// door performs one of the five actions, porting do_gen_door.
//
// The C looks for an object before it looks for a door, so a container in the
// room beats a door with the same keyword — and everything below is written
// once for both, which is why closing a chest and closing a gate refuse in
// exactly the same words.
func (c *Context) door(action doorAction) error {
	verb := doorVerbs[action]

	if c.Arg == "" {
		c.Send("%s what?\r\n", capitaliseFirst(verb))
		return nil
	}

	if cont, _ := c.findContainer(firstWord(c.Arg)); cont != nil {
		return c.containerDoor(cont, action)
	}

	dir, exit := c.findDoor(c.Arg)
	if exit == nil {
		c.Send("There doesn't seem to be %s %s here.\r\n", article(c.Arg), firstWord(c.Arg))
		return nil
	}

	if !exit.State.Has(game.ExitIsDoor) {
		c.Send("You can't %s that!\r\n", verb)
		return nil
	}

	needs := doorNeeds[action]
	closed := exit.State.Has(game.ExitClosed)
	locked := exit.State.Has(game.ExitLocked)

	switch {
	case needs.open && closed:
		c.Send("But it's already closed!\r\n")
		return nil
	case needs.closed && !closed:
		c.Send("But it's currently open!\r\n")
		return nil
	case needs.locked && !locked:
		c.Send("Oh.. it wasn't locked, after all..\r\n")
		return nil
	case needs.unlocked && locked:
		c.Send("It seems to be locked.\r\n")
		return nil
	}

	// Locking and unlocking need the key; picking does not, and immortals
	// need neither.
	if action == doorLock || action == doorUnlock {
		if !c.hasKey(exit.Key) && c.Character.Level() < game.LevelGod {
			c.Send("You don't seem to have the proper key.\r\n")
			return nil
		}
	}

	if action == doorPick && !c.canPick(exit) {
		return nil
	}

	c.applyDoor(dir, exit, action)
	return nil
}

// containerDoor is the same five actions applied to a container, which the C
// reaches through the same function by way of the DOOR_IS_* macros.
//
// A container that is not closeable "can't be opened" rather than "isn't a
// container", because the macro that decides answers one question for both:
// DOOR_IS_OPENABLE is the container type *and* the closeable flag.
func (c *Context) containerDoor(cont *game.Object, action doorAction) error {
	verb := doorVerbs[action]

	if !cont.IsContainer() || !cont.ContainerFlags().Has(game.ContCloseable) {
		c.Send("You can't %s that!\r\n", verb)
		return nil
	}

	needs := doorNeeds[action]
	switch {
	case needs.open && cont.ContainerClosed():
		c.Send("But it's already closed!\r\n")
		return nil
	case needs.closed && !cont.ContainerClosed():
		c.Send("But it's currently open!\r\n")
		return nil
	case needs.locked && !cont.ContainerLocked():
		c.Send("Oh.. it wasn't locked, after all..\r\n")
		return nil
	case needs.unlocked && cont.ContainerLocked():
		c.Send("It seems to be locked.\r\n")
		return nil
	}

	if action == doorLock || action == doorUnlock {
		if !c.hasKey(cont.ContainerKey()) && c.Character.Level() < game.LevelGod {
			c.Send("You don't seem to have the proper key.\r\n")
			return nil
		}
	}
	if action == doorPick && !c.canPickContainer(cont) {
		return nil
	}

	switch action {
	case doorOpen:
		cont.ClearContainerFlag(game.ContClosed)
	case doorClose:
		cont.SetContainerFlag(game.ContClosed)
	case doorLock:
		cont.SetContainerFlag(game.ContLocked)
	case doorUnlock, doorPick:
		cont.ClearContainerFlag(game.ContLocked)
	}

	switch action {
	case doorPick:
		c.Send("The lock quickly yields to your skills.\r\n")
		c.announce("%s skillfully picks the lock on %s.\r\n", c.Character.Name, cont.Name())
	case doorLock, doorUnlock:
		c.Send("*Click*\r\n")
		c.announce("%s %ss %s.\r\n", c.Character.Name, verb, cont.Name())
	default:
		c.Send("Okay.\r\n")
		c.announce("%s %ss %s.\r\n", c.Character.Name, verb, cont.Name())
	}
	return nil
}

// canPickContainer is ok_pick against a container's lock.
func (c *Context) canPickContainer(cont *game.Object) bool {
	switch {
	case cont.ContainerKey() == game.NoObject:
		c.Send("Odd - you can't seem to find a keyhole.\r\n")
	case cont.ContainerFlags().Has(game.ContPickproof):
		c.Send("It resists your attempts to pick it.\r\n")
	case c.RNG.Number(1, 101) > c.skill(game.SkillPickLock):
		c.Send("You failed to pick the lock.\r\n")
	default:
		return true
	}
	return false
}

// applyDoor changes the door's state on both sides and says so.
func (c *Context) applyDoor(dir game.Direction, exit *game.ExitDef, action doorAction) {
	set := func(e *game.ExitDef) {
		switch action {
		case doorOpen:
			e.State = e.State.Without(game.ExitClosed)
		case doorClose:
			e.State = e.State.With(game.ExitClosed)
		case doorLock:
			e.State = e.State.With(game.ExitLocked)
		case doorUnlock, doorPick:
			e.State = e.State.Without(game.ExitLocked)
		}
	}

	set(exit)

	// The other side, if there is one pointing back at us.
	far := c.farSide(dir, exit)
	if far != nil {
		set(far)
	}

	name := doorName(exit)
	switch action {
	case doorPick:
		c.Send("The lock quickly yields to your skills.\r\n")
		c.announce("%s skillfully picks the lock on the %s.\r\n", c.Character.Name, name)
	case doorLock, doorUnlock:
		c.Send("*Click*\r\n")
		c.announce("%s %ss the %s.\r\n", c.Character.Name, doorVerbs[action], name)
	default:
		// config.c:99: OK is "Okay.", not "Ok.".
		c.Send("Okay.\r\n")
		c.announce("%s %ss the %s.\r\n", c.Character.Name, doorVerbs[action], name)
	}

	// The room beyond is told too, but only for opening and closing — a lock
	// makes no noise through a wall.
	if far != nil && (action == doorOpen || action == doorClose) {
		past := "opened"
		if action == doorClose {
			past = "closed"
		}
		for _, other := range c.World.Occupants(exit.ToRoom) {
			other.Tell("The %s is %s from the other side.\r\n", doorName(far), past)
		}
	}
}

// farSide returns the exit in the next room that points back at this one, or
// nil. Two rooms can have one-way exits at each other, in which case the
// doors are separate things and operating one does not touch the other.
func (c *Context) farSide(dir game.Direction, exit *game.ExitDef) *game.ExitDef {
	if exit.ToRoom == game.NoRoom {
		return nil
	}
	beyond := c.World.Room(exit.ToRoom)
	if beyond == nil {
		return nil
	}
	back := beyond.Exits[dir.Reverse()]
	if back == nil || back.ToRoom != c.Character.Room {
		return nil
	}
	return back
}

// findDoor locates a door by direction or by keyword, porting find_door.
func (c *Context) findDoor(arg string) (game.Direction, *game.ExitDef) {
	room := c.World.Room(c.Character.Room)
	if room == nil {
		return 0, nil
	}

	fields := strings.Fields(strings.ToLower(arg))
	if len(fields) == 0 {
		return 0, nil
	}

	// `open north` names the direction outright.
	if dir, ok := game.ParseDirection(fields[0]); ok {
		return dir, room.Exits[dir]
	}
	// `open door east` names the door and then the direction.
	if len(fields) > 1 {
		if dir, ok := game.ParseDirection(fields[1]); ok {
			if exit := room.Exits[dir]; exit != nil && matchesDoor(exit, fields[0]) {
				return dir, exit
			}
			return dir, nil
		}
	}
	// `open gate` searches the room's doors for one with that keyword.
	for dir := game.Direction(0); dir < game.NumDirections; dir++ {
		if exit := room.Exits[dir]; exit != nil && matchesDoor(exit, fields[0]) {
			return dir, exit
		}
	}
	return 0, nil
}

// canPick reports whether a pick attempt succeeds, porting ok_pick.
func (c *Context) canPick(exit *game.ExitDef) bool {
	switch {
	case exit.Key == game.NoObject:
		c.Send("Odd - you can't seem to find a keyhole.\r\n")
	case exit.State.Has(game.ExitPickproof):
		c.Send("It resists your attempts to pick it.\r\n")
	case c.RNG.Number(1, 101) > c.skill(game.SkillPickLock):
		c.Send("You failed to pick the lock.\r\n")
	default:
		return true
	}
	return false
}

// hasKey reports whether the character is carrying the right key.
func (c *Context) hasKey(key game.ObjVnum) bool {
	if key == game.NoObject {
		return false
	}
	for _, obj := range c.Character.Carrying {
		if obj.Vnum() == key {
			return true
		}
	}
	// A key held in the hand counts too.
	if held := c.Character.Equipment[game.WearHold]; held != nil && held.Vnum() == key {
		return true
	}
	return false
}

// skill returns a character's percentage in a skill.
func (c *Context) skill(number game.SpellID) int32 {
	if c.Character.Record == nil {
		return 0
	}
	return c.Character.Record.Skills[number]
}

// doorName is what to call a door in a message: its first keyword, or
// "door".
func doorName(exit *game.ExitDef) string {
	if name := firstWord(exit.Keywords); name != "" {
		return name
	}
	return "door"
}

func matchesDoor(exit *game.ExitDef, word string) bool {
	return exit.Keywords != "" && game.MatchesAnyKeyword(exit.Keywords, word)
}

func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
