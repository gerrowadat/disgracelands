// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import (
	"github.com/gerrowadat/disgracelands/internal/game"
)

// Magic out of objects, ported from mag_objectmagic (spell_parser.c) and
// do_use (act.other.c).
//
// Four item types and four different things they do with a spell. A wand is
// pointed at one target, a staff hits everyone in the room, a scroll is read
// and dissolves, a potion is drunk. Between them they are how a mortal reaches
// the spells above `MAX_SPELLS` — `identify` among them, which is why `cast
// 'identify'` answers "Cast what?!?" and a scroll of it works.

// Default levels for an item that does not say what level it casts at, from
// spells.h:11.
const (
	defaultStaffLevel int32 = 12
	defaultWandLevel  int32 = 12
)

func doUse(c *Context) error    { return c.useItem("use") }
func doQuaff(c *Context) error  { return c.useItem("quaff") }
func doRecite(c *Context) error { return c.useItem("recite") }

// useItem, porting do_use.
//
// The item has to be *held* — the C looks at WEAR_HOLD first for all three
// commands. Quaffing and reciting fall back to the inventory; `use` does not,
// so a wand in your pack is a wand you cannot use.
func (c *Context) useItem(verb string) error {
	name, arg := halfChop(c.Arg)
	if name == "" {
		c.Send("What do you want to %s?\r\n", verb)
		return nil
	}

	item := c.Character.Equipment[game.WearHold]
	if item == nil || !item.Matches(name) {
		if verb == "use" {
			c.Send("You don't seem to be holding %s %s.\r\n", article(name), name)
			return nil
		}
		if item = findObject(c.Character.Carrying, name); item == nil {
			c.Send("You don't seem to have %s %s.\r\n", article(name), name)
			return nil
		}
	}

	switch verb {
	case "quaff":
		if item.Type != game.ItemPotion {
			c.Send("You can only quaff potions.\r\n")
			return nil
		}
	case "recite":
		if item.Type != game.ItemScroll {
			c.Send("You can only recite scrolls.\r\n")
			return nil
		}
	case "use":
		if item.Type != game.ItemWand && item.Type != game.ItemStaff {
			c.Send("You can't seem to figure out how to use it.\r\n")
			return nil
		}
	}

	c.objectMagic(item, arg)
	return nil
}

// objectMagic runs the item, porting mag_objectmagic.
func (c *Context) objectMagic(obj *game.Object, arg string) {
	// The target, found the way generic_find does for all four types at
	// once: somebody in the room, or an object carried, worn or lying here.
	var victim *game.Character
	var target *game.Object
	if arg != "" {
		victim = c.World.FindInRoom(c.Character.Room, arg)
		if victim == nil {
			target = c.findVisibleObject(arg)
		}
	}

	switch obj.Type {
	case game.ItemStaff:
		c.useStaff(obj)
	case game.ItemWand:
		c.useWand(obj, arg, victim, target)
	case game.ItemScroll:
		c.reciteScroll(obj, arg, victim, target)
	case game.ItemPotion:
		c.quaffPotion(obj)
	}
}

// useStaff, porting the ITEM_STAFF case: everybody in the room *except* the
// wielder.
func (c *Context) useStaff(obj *game.Object) {
	c.Send("You tap %s three times on the ground.\r\n", obj.Name())
	if desc := obj.ActionDescription(); desc != "" {
		c.announce("%s", ensureNewline(desc))
	} else {
		c.announce("%s taps %s three times on the ground.\r\n", c.Character.Name, obj.Name())
	}

	if obj.Values[2] <= 0 {
		c.Send("It seems powerless.\r\n")
		c.announce("Nothing seems to happen.\r\n")
		return
	}
	obj.Values[2]--
	c.Character.Wait(1, c.roundLength())

	level := obj.Values[0]
	if level == 0 {
		level = defaultStaffLevel
	}
	number := obj.Values[3]

	info, ok := game.Spell(number)
	if !ok {
		return
	}

	// An area or mass spell on a staff would hit the room once per person if
	// it were run per person, so the C runs it with *no* target once for each
	// person present instead — which is the same number of castings and the
	// C's own comment says why: "Problem: Area/mass spells on staves can
	// cause crashes."
	if info.Routines.HasAny(game.MagAreas | game.MagMasses) {
		for range c.World.Occupants(c.Character.Room) {
			c.castAtLevel(info, number, nil, nil, level, game.SaveRod)
		}
		return
	}

	for _, victim := range append([]*game.Character(nil), c.World.Occupants(c.Character.Room)...) {
		if victim != c.Character {
			c.castAtLevel(info, number, victim, nil, level, game.SaveRod)
		}
	}
}

// useWand, porting the ITEM_WAND case: one target, pointed at.
func (c *Context) useWand(obj *game.Object, arg string, victim *game.Character, target *game.Object) {
	info, ok := game.Spell(obj.Values[3])
	if !ok {
		return
	}

	switch {
	case victim == c.Character:
		c.Send("You point %s at yourself.\r\n", obj.Name())
		c.announce("%s points %s at %sself.\r\n",
			c.Character.Name, obj.Name(), c.Character.Objective())
	case victim != nil:
		c.Send("You point %s at %s.\r\n", obj.Name(), victim.Name)
		c.announceAction(obj, "%s points %s at %s.\r\n",
			c.Character.Name, obj.Name(), victim.Name)
	case target != nil:
		c.Send("You point %s at %s.\r\n", obj.Name(), target.Name())
		c.announceAction(obj, "%s points %s at %s.\r\n",
			c.Character.Name, obj.Name(), target.Name())
	case info.Routines.HasAny(game.MagAreas | game.MagMasses):
		// A wand of an area spell does not need pointing.
		c.Send("You point %s outward.\r\n", obj.Name())
		c.announce("%s points %s outward.\r\n", c.Character.Name, obj.Name())
	default:
		c.Send("At what should %s be pointed?\r\n", obj.Name())
		return
	}
	_ = arg

	if obj.Values[2] <= 0 {
		c.Send("It seems powerless.\r\n")
		c.announce("Nothing seems to happen.\r\n")
		return
	}
	obj.Values[2]--
	c.Character.Wait(1, c.roundLength())

	level := obj.Values[0]
	if level == 0 {
		level = defaultWandLevel
	}
	c.castAtLevel(info, obj.Values[3], victim, target, level, game.SaveRod)
}

// reciteScroll, porting the ITEM_SCROLL case: up to three spells, and the
// scroll is destroyed whatever happens.
func (c *Context) reciteScroll(obj *game.Object, arg string, victim *game.Character, target *game.Object) {
	if arg != "" && victim == nil && target == nil {
		c.Send("There is nothing to here to affect with %s.\r\n", obj.Name())
		return
	}
	if arg == "" {
		victim = c.Character
	}

	c.Send("You recite %s which dissolves.\r\n", obj.Name())
	if desc := obj.ActionDescription(); desc != "" {
		c.announce("%s", ensureNewline(desc))
	} else {
		c.announce("%s recites %s.\r\n", c.Character.Name, obj.Name())
	}
	c.Character.Wait(1, c.roundLength())

	// Values 1, 2 and 3 are up to three spells, cast in order and stopping at
	// the first that does nothing.
	for _, number := range obj.Values[1:] {
		if !c.castFromItem(number, victim, target, obj.Values[0], game.SaveRod) {
			break
		}
	}
	c.World.ExtractObject(obj)
}

// quaffPotion, porting the ITEM_POTION case: always on the drinker.
func (c *Context) quaffPotion(obj *game.Object) {
	c.Send("You quaff %s.\r\n", obj.Name())
	if desc := obj.ActionDescription(); desc != "" {
		c.announce("%s", ensureNewline(desc))
	} else {
		c.announce("%s quaffs %s.\r\n", c.Character.Name, obj.Name())
	}
	c.Character.Wait(1, c.roundLength())

	for _, number := range obj.Values[1:] {
		if !c.castFromItem(number, c.Character, nil, obj.Values[0], game.SaveRod) {
			break
		}
	}
	c.World.ExtractObject(obj)
}

// castFromItem runs one spell out of an item, returning false when there was
// no spell there — which is what stops a scroll with one spell casting three.
func (c *Context) castFromItem(number int32, victim *game.Character, target *game.Object,
	level int32, save game.SaveType,
) bool {
	if number < 1 {
		return false
	}
	info, ok := game.Spell(number)
	if !ok {
		return false
	}
	return c.castAtLevel(info, number, victim, target, level, save)
}

// castAtLevel is call_magic for an item: no mana, no skill roll, and the
// level is the *item's* rather than the reader's.
func (c *Context) castAtLevel(info game.SpellInfo, number int32, victim *game.Character,
	target *game.Object, level int32, save game.SaveType,
) bool {
	rec := c.Character.Record
	if rec == nil {
		return false
	}

	// castSpell reads the caster's level for damage and duration, so an item
	// casting at its own level has to lend it for the call. The C passes the
	// level as an argument instead, which comes to the same thing and is the
	// one place this port keeps the state where the C keeps the parameter.
	actual := rec.Level
	rec.Level = level
	defer func() { rec.Level = actual }()

	return c.castSpell(info, number, victim, target, save)
}

// announceAction sends an object's action description to the room if it has
// one, and the given message otherwise. Four of mag_objectmagic's messages
// have that shape.
func (c *Context) announceAction(obj *game.Object, format string, args ...any) {
	if desc := obj.ActionDescription(); desc != "" {
		c.announce("%s", ensureNewline(desc))
		return
	}
	c.announce(format, args...)
}
