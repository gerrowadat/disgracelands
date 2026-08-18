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

// The commands that move objects around, ported from act.item.c.
//
// The messages are the C's, including the ones that read oddly — "You start
// to use $p as a shield" for a shield, "You grab $p" for something held.
// Players know these by heart.

// get, drop, put and give live in carrying.go, which is the other half of
// act.item.c.

func doInventory(c *Context) error {
	c.Send("You are carrying:\r\n")
	if len(c.Character.Carrying) == 0 {
		c.Send(" Nothing.\r\n")
		return nil
	}
	for _, obj := range c.Character.Carrying {
		c.Send("%s\r\n", obj.Name())
	}
	return nil
}

func doEquipment(c *Context) error {
	c.Send("You are using:\r\n")

	var any bool
	for pos := game.WearPosition(0); pos < game.NumWears; pos++ {
		obj := c.Character.Equipment[pos]
		if obj == nil {
			continue
		}
		any = true
		c.Send("%s%s\r\n", pos, obj.Name())
	}
	if !any {
		c.Send(" Nothing.\r\n")
	}
	return nil
}

// doWear puts something on, porting do_wear.
//
// With no position named it finds the first slot the object fits, which is
// what makes `wear all` work in the C and what a player expects from `wear
// ring`.
func doWear(c *Context) error {
	if c.Arg == "" {
		c.Send("Wear what?\r\n")
		return nil
	}

	obj := findObject(c.Character.Carrying, c.Arg)
	if obj == nil {
		c.Send("You don't seem to have %s %s.\r\n", article(c.Arg), c.Arg)
		return nil
	}
	return c.wearAt(obj, findWearPosition(c.Character, obj))
}

// doWield is `wear` restricted to the weapon hand, and says so differently
// when the object is not a weapon.
func doWield(c *Context) error {
	if c.Arg == "" {
		c.Send("Wield what?\r\n")
		return nil
	}

	obj := findObject(c.Character.Carrying, c.Arg)
	if obj == nil {
		c.Send("You don't seem to have %s %s.\r\n", article(c.Arg), c.Arg)
		return nil
	}
	if !obj.WearFlags.Has(game.ItemWearWield) {
		c.Send("You can't wield that.\r\n")
		return nil
	}
	return c.wearAt(obj, game.WearWield)
}

// doRemove takes something off, porting do_remove.
func doRemove(c *Context) error {
	if c.Arg == "" {
		c.Send("Remove what?\r\n")
		return nil
	}

	for pos := game.WearPosition(0); pos < game.NumWears; pos++ {
		obj := c.Character.Equipment[pos]
		if obj == nil || !obj.Matches(c.Arg) {
			continue
		}

		c.World.Unequip(c.Character, pos)
		c.World.ObjectToChar(obj, c.Character)
		c.Send("You stop using %s.\r\n", obj.Name())
		c.announce("%s stops using %s.\r\n", c.Character.Name, obj.Name())
		return nil
	}

	c.Send("You don't seem to be using %s %s.\r\n", article(c.Arg), c.Arg)
	return nil
}

// wearAt puts an object in a slot and says so, porting perform_wear and
// wear_message.
func (c *Context) wearAt(obj *game.Object, pos game.WearPosition) error {
	if pos < 0 {
		c.Send("You can't wear %s.\r\n", obj.Name())
		return nil
	}
	if c.Character.Equipment[pos] != nil {
		c.Send("%s", alreadyWearing[pos])
		return nil
	}
	if !c.World.Equip(obj, c.Character, pos) {
		c.Send("You can't seem to put %s on.\r\n", obj.Name())
		return nil
	}

	c.Send(wearMessages[pos][1]+"\r\n", obj.Name())
	c.announce(wearMessages[pos][0]+"\r\n", c.Character.Name, obj.Name())
	return nil
}

// announce tells the rest of the room.
func (c *Context) announce(format string, args ...any) {
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character {
			other.Tell(format, args...)
		}
	}
}

// findObject picks the first object in a list a typed word names.
func findObject(list []*game.Object, word string) *game.Object {
	for _, obj := range list {
		if obj.Matches(word) {
			return obj
		}
	}
	return nil
}

// findWearPosition finds the first free slot an object fits, in the C's
// order — so a second ring goes on the left hand and a second necklace in the
// second neck slot.
func findWearPosition(c *game.Character, obj *game.Object) game.WearPosition {
	var fits game.WearPosition = -1

	for pos := game.WearPosition(0); pos < game.NumWears; pos++ {
		if !game.CanWearAt(obj.Def, pos) {
			continue
		}
		if c.Equipment[pos] == nil {
			return pos
		}
		// Remember that it fits somewhere, so an occupied slot produces
		// "you're already wearing something there" rather than "you can't
		// wear that".
		if fits < 0 {
			fits = pos
		}
	}
	return fits
}

// wearMessages is wear_messages (act.item.c:1130): the room's message first,
// then the wearer's. The `$p` of the C becomes a `%s` for the object, and the
// `$n` a `%s` for the wearer.
var wearMessages = [game.NumWears][2]string{
	game.WearLight:       {"%s lights %s and holds it.", "You light %s and hold it."},
	game.WearFingerRight: {"%s slides %s on to their right ring finger.", "You slide %s on to your right ring finger."},
	game.WearFingerLeft:  {"%s slides %s on to their left ring finger.", "You slide %s on to your left ring finger."},
	game.WearNeck1:       {"%s wears %s around their neck.", "You wear %s around your neck."},
	game.WearNeck2:       {"%s wears %s around their neck.", "You wear %s around your neck."},
	game.WearBody:        {"%s wears %s on their body.", "You wear %s on your body."},
	game.WearHead:        {"%s wears %s on their head.", "You wear %s on your head."},
	game.WearLegs:        {"%s puts %s on their legs.", "You put %s on your legs."},
	game.WearFeet:        {"%s wears %s on their feet.", "You wear %s on your feet."},
	game.WearHands:       {"%s puts %s on their hands.", "You put %s on your hands."},
	game.WearArms:        {"%s wears %s on their arms.", "You wear %s on your arms."},
	game.WearShield:      {"%s straps %s around their arm as a shield.", "You start to use %s as a shield."},
	game.WearAbout:       {"%s wears %s about their body.", "You wear %s around your body."},
	game.WearWaist:       {"%s wears %s around their waist.", "You wear %s around your waist."},
	game.WearWristRight:  {"%s puts %s on around their right wrist.", "You put %s on around your right wrist."},
	game.WearWristLeft:   {"%s puts %s on around their left wrist.", "You put %s on around your left wrist."},
	game.WearWield:       {"%s wields %s.", "You wield %s."},
	game.WearHold:        {"%s grabs %s.", "You grab %s."},
}

// alreadyWearing is already_wearing (act.item.c): what to say when the slot
// is taken.
var alreadyWearing = [game.NumWears]string{
	game.WearLight:       "You're already using a light.\r\n",
	game.WearFingerRight: "You're already wearing something on both of your ring fingers.\r\n",
	game.WearFingerLeft:  "You're already wearing something on both of your ring fingers.\r\n",
	game.WearNeck1:       "You can't wear anything else around your neck.\r\n",
	game.WearNeck2:       "You can't wear anything else around your neck.\r\n",
	game.WearBody:        "You're already wearing something on your body.\r\n",
	game.WearHead:        "You're already wearing something on your head.\r\n",
	game.WearLegs:        "You're already wearing something on your legs.\r\n",
	game.WearFeet:        "You're already wearing something on your feet.\r\n",
	game.WearHands:       "You're already wearing something on your hands.\r\n",
	game.WearArms:        "You're already wearing something on your arms.\r\n",
	game.WearShield:      "You're already using a shield.\r\n",
	game.WearAbout:       "You're already wearing something about your body.\r\n",
	game.WearWaist:       "You already have something around your waist.\r\n",
	game.WearWristRight:  "You're already wearing something around both of your wrists.\r\n",
	game.WearWristLeft:   "You're already wearing something around both of your wrists.\r\n",
	game.WearWield:       "You're already wielding a weapon.\r\n",
	game.WearHold:        "You're already holding something.\r\n",
}

// article picks "a" or "an" for a word, as the C's AN macro does.
func article(word string) string {
	if word == "" {
		return "a"
	}
	if strings.ContainsRune("aeiouAEIOU", rune(word[0])) {
		return "an"
	}
	return "a"
}

func capitaliseFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
