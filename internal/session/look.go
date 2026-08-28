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

// `look`, ported from act.informative.c.
//
// do_look is a dispatcher, not a command: four different functions answer to
// it depending on what follows the word, and this port had collapsed all four
// into one. `look in <container>` in particular reached look_at_target, which
// describes a thing, rather than look_in_obj, which opens it — so a corpse
// answered "You see nothing special about the corpse of a fido." and there
// was no way to find out what was in it.
//
// The session-parity suite is what has an opinion about all of this now
// (test/parity, objects.session and combat.session): the C server is the
// expectation, and the shapes below are what it actually printed.

// findObjectAndWhere is generic_find restricted to FIND_OBJ_EQUIP |
// FIND_OBJ_INV | FIND_OBJ_ROOM, which is what look_in_obj and look_at_target
// ask for. See genericFind, which is the whole of it.
func (c *Context) findObjectAndWhere(arg string) (*game.Object, findWhere) {
	_, obj, where := c.genericFind(arg, findObjEquip|findObjInv|findObjRoom)
	return obj, where
}

// lookInObject is look_in_obj (act.informative.c:500): `look in <thing>`.
//
// Three kinds of thing answer to it — a container, a drink container and a
// fountain — and everything else gets "There's nothing inside that!" whether
// or not it looks like it should have an inside.
func (c *Context) lookInObject(arg string) error {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		c.Send("Look in what?\r\n")
		return nil
	}

	obj, where := c.findObjectAndWhere(arg)
	if obj == nil {
		// AN() picks the article off the first letter alone (utils.h:133),
		// so it says "an hour" wrong and "a onion" wrong, and it says the
		// player's own typo back at them: `look in nothing` is "There
		// doesn't seem to be a nothing here."
		c.Send("There doesn't seem to be %s %s here.\r\n", article(arg), arg)
		return nil
	}

	switch obj.Type {
	case game.ItemContainer:
		if obj.ContainerClosed() {
			c.Send("It is closed.\r\n")
			return nil
		}
		// fname, not Name(): the *first keyword*, not the short description,
		// so a "leather bag" listed as "a small leather bag" heads its
		// contents with "bag". The trailing space before the newline is the
		// C's own (act.informative.c:519).
		c.Send("%s", fname(obj.Keywords))
		switch where {
		case foundInInventory:
			c.Send(" (carried): \r\n")
		case foundInRoom:
			c.Send(" (here): \r\n")
		case foundInEquipment:
			c.Send(" (used): \r\n")
		}
		c.listObjects(obj.Contents)
	case game.ItemDrinkCon, game.ItemFountain:
		c.showLiquid(obj)
	default:
		c.Send("There's nothing inside that!\r\n")
	}
	return nil
}

// showLiquid is look_in_obj's drink-container branch.
//
// The middle case is marked `/* BUG */` in the C itself, and it is left
// alone: a container whose contents exceed its capacity, or whose capacity is
// zero, reports "Its contents seem somewhat murky." rather than dividing by
// it. Reproduced because the guard is what stops the division, and because
// "murky" is what a player of the real game saw.
func (c *Context) showLiquid(obj *game.Object) {
	capacity, filled, liquid := obj.Values[0], obj.Values[1], obj.Values[2]
	switch {
	case filled <= 0:
		c.Send("It is empty.\r\n")
	case capacity <= 0 || filled > capacity:
		c.Send("Its contents seem somewhat murky.\r\n")
	default:
		c.Send("It's %sfull of a %s liquid.\r\n",
			game.Fullness(filled*3/capacity), game.LiquidColour(liquid))
	}
}

// listObjects is list_obj_to_char with SHOW_OBJ_SHORT and show set
// (act.informative.c:129), which is what every container's contents go
// through.
//
// The leading space on " Nothing." is the C's, and so is the fact that it
// appears at all only when `show` is true — a room's floor listing passes
// false and stays silent when there is nothing on it.
func (c *Context) listObjects(list []*game.Object) {
	var found bool
	for _, obj := range list {
		if !c.World.CanSeeObj(c.Character, obj) {
			continue
		}
		c.Send("%s%s\r\n", obj.Name(), objectModifiers(c.Character, obj))
		found = true
	}
	if !found {
		c.Send(" Nothing.\r\n")
	}
}

// objectModifiers is show_obj_modifiers (act.informative.c:104): the little
// tags an object trails when it is invisible, blessed, magical, glowing or
// humming.
//
// Two of the five are conditional on the *viewer* rather than the object —
// blue for blessed and yellow for magical need detect alignment and detect
// magic up — which is why this takes the character as well as the object.
func objectModifiers(viewer *game.Character, obj *game.Object) string {
	var b strings.Builder
	if obj.ExtraFlags.Has(game.ItemInvisible) {
		b.WriteString(" (invisible)")
	}
	if obj.ExtraFlags.Has(game.ItemBless) && viewer.HasAffect(game.AffectDetectAlign) {
		b.WriteString(" ..It glows blue!")
	}
	if obj.ExtraFlags.Has(game.ItemMagic) && viewer.HasAffect(game.AffectDetectMagic) {
		b.WriteString(" ..It glows yellow!")
	}
	if obj.ExtraFlags.Has(game.ItemGlow) {
		b.WriteString(" ..It has a soft glowing aura!")
	}
	if obj.ExtraFlags.Has(game.ItemHum) {
		b.WriteString(" ..It emits a faint humming sound!")
	}
	return b.String()
}

// lookInDirection is look_in_direction (act.informative.c:534).
//
// The door lines are an if/else rather than two ifs, so a closed door says
// only "The gate is closed." and never also that it is a door.
func (c *Context) lookInDirection(dir game.Direction) error {
	room := c.World.Room(c.Character.Room)
	if room == nil || room.Exits[dir] == nil {
		c.Send("Nothing special there...\r\n")
		return nil
	}
	exit := room.Exits[dir]
	if exit.Description != "" {
		c.Send("%s", ensureNewline(exit.Description))
	} else {
		c.Send("You see nothing special.\r\n")
	}

	if exit.Keywords == "" {
		return nil
	}
	switch {
	case exit.State.Has(game.ExitClosed):
		c.Send("The %s is closed.\r\n", fname(exit.Keywords))
	case exit.State.Has(game.ExitIsDoor):
		c.Send("The %s is open.\r\n", fname(exit.Keywords))
	}
	return nil
}

// fname is handler.c:698: the leading run of letters in a keyword list.
//
// It stops at the first non-alphabetic character rather than at whitespace,
// which is not the same thing — a keyword list beginning "bag2 leather"
// yields "bag", not "bag2". Nothing in the shipped world exercises the
// difference; it is reproduced because the C's loop is written that way and
// a world file is free to.
func fname(namelist string) string {
	end := 0
	for end < len(namelist) && isLetter(namelist[end]) {
		end++
	}
	return namelist[:end]
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
