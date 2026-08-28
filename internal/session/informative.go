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

	victim := c.findInRoom(c.Arg)
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

	// A container is opened up as well as described. do_examine's own
	// generic_find(FIND_OBJ_INV | FIND_OBJ_ROOM | FIND_CHAR_ROOM |
	// FIND_OBJ_EQUIP) (act.informative.c:711) — a *second* search after
	// look_at_target's, on the same argument, which is why the C's comment
	// there says "look_at_target() eats the number" and it is handed an
	// untouched copy.
	if _, obj, _ := c.genericFind(c.Arg,
		findObjInv|findObjRoom|findCharRoom|findObjEquip); obj != nil {
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
	if name == "" {
		c.Send("Look at what?\r\n")
		return nil
	}

	// generic_find comes first, on the argument **as typed**
	// (act.informative.c:589). It takes its own copy of the string before
	// calling get_number (handler.c:1377-1383), so the count is still on the
	// caller's argument afterwards — which is why look_at_target can strip it
	// again below for the extra-description walk, and why the C's comment
	// there reads "Strip off 'number.' from 2.foo and friends."
	//
	// This port stripped the count *first* and handed the bare word to the
	// search, so `2.sword` was searched for as "sword" and always found the
	// first one (#194). One shared count across all four lists is the other
	// half of the same call: with one sword worn and one carried, `1.sword`
	// is the worn one and `2.sword` the carried one.
	victim, obj, _ := c.genericFind(name,
		findObjInv|findObjRoom|findObjEquip|findCharRoom)
	if victim != nil {
		return c.lookAtCharacter(victim)
	}

	// Then the count, for the extra descriptions. generic_find gives up on a
	// zero count *before it searches anything at all* —
	// `if (!(number = get_number(&name))) return (0);` (handler.c:1387) — and
	// look_at_target then reads it itself and says "Look at what?"
	// (act.informative.c:603). The order matters for one case: `0.me` is not
	// you, because get_char_room_vis's own "self" branch is never reached.
	// `2.me` is, since that branch ignores the count it sits behind. Both
	// checked against the C, not reasoned about.
	fnum, name := game.GetNumber(name)
	if fnum == 0 {
		c.Send("Look at what?\r\n")
		return nil
	}

	// The extra descriptions, in the C's own order: the room, then worn
	// equipment, then the inventory, then what is lying on the floor
	// (act.informative.c:610-639). This is what makes `look at atm` show the
	// note stuck to the ATM rather than the ATM's own line from the room.
	//
	// `i` counts *matches across all four lists*, not within one, so
	// `2.plaque` in a room with one plaque and a plaque on the floor is the
	// one on the floor. That shared counter is the whole reason these are one
	// walk rather than four independent searches.
	if desc, found := c.findExtraDescription(name, fnum); found {
		// The C pages the room's own extra description and sends the other
		// three directly (page_string against send_to_char). Both end up on
		// the screen; the difference only shows for a description longer than
		// a page, and none in the shipped world is.
		c.Send("%s", ensureNewline(desc))
		// An object matched by *both* an extra description and its own name
		// gets its modifiers appended — show_obj_modifiers, then a newline
		// (act.informative.c:643-646). The description itself is not repeated.
		if obj != nil {
			c.Send("%s\r\n", objectModifiers(c.Character, obj))
		}
		return nil
	}

	if obj != nil {
		return c.showObject(obj)
	}

	c.Send("You do not see that here.\r\n")
	return nil
}

// findExtraDescription walks the four lists look_at_target searches, in its
// order, returning the fnum'th match.
func (c *Context) findExtraDescription(name string, fnum int) (string, bool) {
	var seen int
	match := func(list []game.ExtraDesc) (string, bool) {
		for _, extra := range list {
			if !game.MatchesAnyKeyword(extra.Keywords, name) {
				continue
			}
			seen++
			if seen == fnum {
				return extra.Description, true
			}
		}
		return "", false
	}

	if room := c.World.Room(c.Character.Room); room != nil {
		if desc, ok := match(room.ExtraDescs); ok {
			return desc, true
		}
	}
	for pos := game.WearPosition(0); pos < game.NumWears; pos++ {
		obj := c.Character.Equipment[pos]
		if obj == nil || obj.Def == nil || !c.World.CanSeeObj(c.Character, obj) {
			continue
		}
		if desc, ok := match(obj.Def.ExtraDescs); ok {
			return desc, true
		}
	}
	for _, list := range [][]*game.Object{
		c.Character.Carrying,
		c.World.RoomObjects(c.Character.Room),
	} {
		for _, obj := range list {
			if obj.Def == nil || !c.World.CanSeeObj(c.Character, obj) {
				continue
			}
			if desc, ok := match(obj.Def.ExtraDescs); ok {
				return desc, true
			}
		}
	}
	return "", false
}

// showObject is show_obj_to_char with SHOW_OBJ_ACTION (act.informative.c:60),
// which is what look_at_target falls back to when nothing matched an extra
// description.
//
// The two odd strings are the C's and are odd on purpose: a drink container
// says "It looks like a drink container." and everything else "You see
// nothing special.." — with two full stops, because show_obj_modifiers
// appends to the same line and the sentence was written to run into it.
func (c *Context) showObject(obj *game.Object) error {
	switch obj.Type {
	case game.ItemNote:
		// A note shows what is written on it and nothing else, which is the
		// whole of how mail is read (act.informative.c:117). It returns
		// before the modifiers, so a glowing note does not say so.
		if text := obj.ActionDescription(); text != "" {
			c.Send("There is something written on it:\r\n\r\n%s", ensureNewline(text))
		} else {
			c.Send("It's blank.\r\n")
		}
		return nil
	case game.ItemDrinkCon:
		c.Send("It looks like a drink container.")
	default:
		c.Send("You see nothing special..")
	}
	c.Send("%s\r\n", objectModifiers(c.Character, obj))
	return nil
}

// lookAtCharacter describes somebody, porting look_at_char.
func (c *Context) lookAtCharacter(victim *game.Character) error {
	if victim.Record != nil && victim.Record.Description != "" {
		c.Send("%s", ensureNewline(victim.Record.Description))
	} else {
		// `$m`, not the name: the C is `act("You see nothing special about
		// $m.", FALSE, i, 0, ch, TO_VICT)` (act.informative.c:242), and $m is
		// the *objective pronoun* of the person being looked at. So it is
		// "about him.", "about her." or "about it." — never a name, and never
		// "about a large dog." This port printed the name, which reads
		// naturally enough that nothing questioned it until a parity scenario
		// finally looked at somebody.
		c.Send("%s", c.World.Act("You see nothing special about $m.",
			game.ActArgs{Actor: victim, Victim: c.Character}, c.Character))
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
