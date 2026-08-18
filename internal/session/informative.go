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

// The rest of act.informative.c: looking at things, sizing people up, and
// asking what time it is.

// doConsider, porting do_consider.
//
// The thresholds are uneven on purpose — ten levels below you is a chicken,
// one above needs luck, and more than ten above means "You ARE mad!". The
// wording is the whole feature.
func doConsider(c *Context) error {
	if strings.TrimSpace(c.Arg) == "" {
		c.Send("Consider killing who?\r\n")
		return nil
	}

	victim := c.World.FindInRoom(c.Character.Room, c.Arg)
	if victim == nil {
		c.Send("Consider killing who?\r\n")
		return nil
	}
	if victim == c.Character {
		c.Send("Easy!  Very easy indeed!\r\n")
		return nil
	}
	if !victim.IsNPC() {
		c.Send("Would you like to borrow a cross and a shovel?\r\n")
		return nil
	}

	c.Send("%s", game.ConsiderVerdict(victim.Level()-c.Character.Level()))
	return nil
}

// doTime, porting do_time.
func doTime(c *Context) error {
	now := c.World.MudTime()
	c.Send("%s", now.Clock())
	c.Send("%s", now.Date())
	return nil
}

// doWeather, porting do_weather. Outdoors only, and it says so.
func doWeather(c *Context) error {
	room := c.World.Room(c.Character.Room)
	if room != nil && room.Flags.Has(game.RoomIndoors) {
		c.Send("You have no feeling about the weather at all.\r\n")
		return nil
	}
	c.Send("The sky is cloudless and you feel a mild breeze.\r\n")
	return nil
}

// doExamine is `look` with the container and corpse contents shown, porting
// do_examine.
func doExamine(c *Context) error {
	if strings.TrimSpace(c.Arg) == "" {
		c.Send("Examine what?\r\n")
		return nil
	}

	if err := c.lookAtTarget(c.Arg); err != nil {
		return err
	}

	// A container is opened up as well as described.
	if obj := c.findVisibleObject(c.Arg); obj != nil {
		if obj.Type == game.ItemContainer {
			c.Send("When you look inside, you see:\r\n")
			c.showContents(obj)
		}
	}
	return nil
}

// lookAtTarget describes one thing, porting look_at_target.
//
// The C's search order is the order here, and it is player-visible: your own
// equipment before the room's floor means `look sword` shows the one in your
// hand rather than the one on the ground.
func (c *Context) lookAtTarget(arg string) error {
	name := strings.TrimSpace(arg)

	if victim := c.World.FindInRoom(c.Character.Room, name); victim != nil {
		return c.lookAtCharacter(victim)
	}
	if obj := c.findVisibleObject(name); obj != nil {
		if obj.Description != "" && obj.Location == game.InRoom {
			c.Send("%s\r\n", obj.Description)
		} else {
			c.Send("You see nothing special about %s.\r\n", obj.Name())
		}
		return nil
	}

	// An extra description on the room — the C looks these up last, which is
	// why an object named "fountain" wins over a fountain in the room's
	// description.
	if room := c.World.Room(c.Character.Room); room != nil {
		for _, extra := range room.ExtraDescs {
			if game.MatchesAnyKeyword(extra.Keywords, name) {
				c.Send("%s", ensureNewline(extra.Description))
				return nil
			}
		}
	}

	c.Send("You do not see that here.\r\n")
	return nil
}

// lookAtCharacter describes somebody, porting look_at_char.
func (c *Context) lookAtCharacter(victim *game.Character) error {
	if victim.Record != nil && victim.Record.Description != "" {
		c.Send("%s", ensureNewline(victim.Record.Description))
	} else {
		c.Send("You see nothing special about %s.\r\n", victim.Name)
	}

	c.Send("%s", game.HealthDiagnosis(victim.Name, victim.Record))

	if victim != c.Character {
		victim.Tell("%s looks at you.\r\n", c.Character.Name)
	}

	// What they are wearing, which is how you size somebody up.
	var wearing bool
	for pos := game.WearPosition(0); pos < game.NumWears; pos++ {
		obj := victim.Equipment[pos]
		if obj == nil {
			continue
		}
		if !wearing {
			c.Send("\r\n%s is using:\r\n", victim.Name)
			wearing = true
		}
		c.Send("%s%s\r\n", pos, obj.Name())
	}
	return nil
}

// findVisibleObject looks for an object the way the C does: equipment first,
// then inventory, then the floor.
func (c *Context) findVisibleObject(name string) *game.Object {
	for _, obj := range c.Character.Equipment {
		if obj != nil && obj.Matches(name) {
			return obj
		}
	}
	if obj := findObject(c.Character.Carrying, name); obj != nil {
		return obj
	}
	return findObject(c.World.RoomObjects(c.Character.Room), name)
}

// showContents lists what is inside a container.
func (c *Context) showContents(container *game.Object) {
	if len(container.Contents) == 0 {
		c.Send("Nothing.\r\n")
		return
	}
	for _, obj := range container.Contents {
		c.Send("%s\r\n", obj.Name())
	}
}
