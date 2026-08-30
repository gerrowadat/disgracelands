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
// `wear <thing>` finds the slot; `wear <thing> <place>` names it; `wear all`
// puts on everything wearable. The level check here is local to this tree —
// the C carries it between `<DoC>` markers — and it is worded differently
// from the one inside perform_wear, so which message you get depends on
// whether you named the object or said `all`.
func doWear(c *Context) error {
	arg1, arg2, _ := twoArguments(c.Arg)
	if arg1 == "" {
		c.Send("Wear what?\r\n")
		return nil
	}

	mode, arg1 := findAllDots(arg1)
	if arg2 != "" && mode != findIndiv {
		c.Send("You can't specify the same body location for more than one item!\r\n")
		return nil
	}

	switch mode {
	case findAll:
		var worn bool
		for _, obj := range everything(c.Character.Carrying, mode, arg1) {
			if pos := findWearPosition(obj); pos >= 0 {
				worn = true
				if err := c.wearAt(obj, pos); err != nil {
					return err
				}
			}
		}
		if !worn {
			c.Send("You don't seem to have anything wearable.\r\n")
		}
		return nil

	case findAllDot:
		if arg1 == "" {
			c.Send("Wear all of what?\r\n")
			return nil
		}
		matched := everything(c.Character.Carrying, mode, arg1)
		if len(matched) == 0 {
			c.Send("You don't seem to have any %ss.\r\n", arg1)
			return nil
		}
		for _, obj := range matched {
			pos := findWearPosition(obj)
			if pos < 0 {
				c.Send("You can't wear %s.\r\n", obj.Name())
				continue
			}
			if err := c.wearAt(obj, pos); err != nil {
				return err
			}
		}
		return nil
	}

	obj := c.findObject(c.Character.Carrying, arg1)
	if obj == nil {
		named := namedWithoutCount(arg1)
		c.Send("You don't seem to have %s %s.\r\n", article(named), named)
		return nil
	}
	if c.Character.Level() < obj.MinLevel() {
		c.Send("You are not experienced enough to use that.\r\n")
		return nil
	}

	// A named place is looked up in its own list, and a place that is not a
	// body part is complained about rather than ignored.
	if arg2 != "" {
		pos, ok := wearPositionNamed(arg2)
		if !ok {
			c.Send("'%s'?  What part of your body is THAT?\r\n", arg2)
			return nil
		}
		return c.wearAt(obj, pos)
	}

	pos := findWearPosition(obj)
	if pos < 0 {
		c.Send("You can't wear %s.\r\n", obj.Name())
		return nil
	}
	return c.wearAt(obj, pos)
}

// doWield is `wear` restricted to the weapon hand, and says so differently
// when the object is not a weapon.
func doWield(c *Context) error {
	if c.Arg == "" {
		c.Send("Wield what?\r\n")
		return nil
	}

	obj := c.findObject(c.Character.Carrying, c.Arg)
	if obj == nil {
		named := namedWithoutCount(c.Arg)
		c.Send("You don't seem to have %s %s.\r\n", article(named), named)
		return nil
	}
	if !obj.WearFlags.Has(game.ItemWearWield) {
		c.Send("You can't wield that.\r\n")
		return nil
	}
	return c.wearAt(obj, game.WearWield)
}

// doGrab takes something in hand, porting do_grab.
//
// A light goes in the light slot rather than the hold slot, which is how a
// torch lights the room while a wand does not.
func doGrab(c *Context) error {
	if c.Arg == "" {
		c.Send("Hold what?\r\n")
		return nil
	}

	obj := c.findObject(c.Character.Carrying, c.Arg)
	if obj == nil {
		named := namedWithoutCount(c.Arg)
		c.Send("You don't seem to have %s %s.\r\n", article(named), named)
		return nil
	}
	if obj.Type == game.ItemLight {
		return c.wearAt(obj, game.WearLight)
	}
	if !obj.WearFlags.Has(game.ItemWearHold) {
		c.Send("You can't hold that.\r\n")
		return nil
	}
	return c.wearAt(obj, game.WearHold)
}

// doRemove takes something off, porting do_remove.
func doRemove(c *Context) error {
	arg, _ := oneArgument(c.Arg)
	if arg == "" {
		c.Send("Remove what?\r\n")
		return nil
	}

	// `remove all` and `remove all.ring` are the C's, and this port had
	// neither: it looked for an object whose name was the literal word "all"
	// and refused. do_remove is the last of the nine find_all_dots callers to
	// get them (act.item.c:1453).
	mode, rest := findAllDots(arg)
	switch mode {
	case findAll:
		var found bool
		for pos := game.WearPosition(0); pos < game.NumWears; pos++ {
			if c.Character.Equipment[pos] != nil {
				c.performRemove(pos)
				found = true
			}
		}
		if !found {
			c.Send("You're not using anything.\r\n")
		}

	case findAllDot:
		if rest == "" {
			c.Send("Remove all of what?\r\n")
			return nil
		}
		var found bool
		for pos := game.WearPosition(0); pos < game.NumWears; pos++ {
			obj := c.Character.Equipment[pos]
			if obj == nil || !c.World.CanSeeObj(c.Character, obj) || !obj.Matches(rest) {
				continue
			}
			c.performRemove(pos)
			found = true
		}
		if !found {
			// The C pluralises by sticking an "s" on whatever was typed, so
			// `remove all.boots` answers "any bootss". Left alone: it is what
			// a player of the real game saw.
			c.Send("You don't seem to be using any %ss.\r\n", rest)
		}

	default:
		pos, ok := c.equipmentPosition(arg)
		if !ok {
			named := namedWithoutCount(arg)
			c.Send("You don't seem to be using %s %s.\r\n", article(named), named)
			return nil
		}
		c.performRemove(pos)
	}
	return nil
}

// equipmentPosition is get_obj_pos_in_equip_vis (handler.c:1254): which slot
// holds the N'th visible worn thing of that name.
//
// It checks CAN_SEE_OBJ, which is worth saying out loud because
// generic_find's own equipment loop does not — see docs/weirdnumbers.md.
// Two searches over the same array, in the same file, disagreeing about
// whether an invisible worn object can be found.
func (c *Context) equipmentPosition(arg string) (game.WearPosition, bool) {
	n, word := game.GetNumber(arg)
	if n == 0 {
		return 0, false
	}
	for pos := game.WearPosition(0); pos < game.NumWears; pos++ {
		obj := c.Character.Equipment[pos]
		if obj == nil || !c.World.CanSeeObj(c.Character, obj) || !obj.Matches(word) {
			continue
		}
		n--
		if n == 0 {
			return pos, true
		}
	}
	return 0, false
}

// performRemove is perform_remove (act.item.c:1424).
//
// Two refusals this port did not have, and both are things a player runs
// into: a cursed item cannot come off at all, and an item cannot come off
// into hands that are already full. The second is why `remove all` is not
// simply a loop that always succeeds — a character wearing more than they can
// carry takes some of it off and keeps the rest on.
func (c *Context) performRemove(pos game.WearPosition) {
	obj := c.Character.Equipment[pos]
	if obj == nil {
		return
	}
	switch {
	case obj.ExtraFlags.Has(game.ItemNoDrop):
		c.Send("You can't remove %s, it must be CURSED!\r\n", obj.Name())
	case c.handsFull():
		// Capitalised because act() capitalises whatever it ends up with —
		// `SEND_TO_Q(CAP(lbuf), ...)` (comm.c:2410) — and "$p" is the first
		// thing in this format, so a short description beginning "a long
		// sword" reaches the player as "A long sword:".
		c.Send("%s: you can't carry that many items!\r\n", capitaliseFirst(obj.Name()))
	default:
		c.World.Unequip(c.Character, pos)
		c.World.ObjectToChar(obj, c.Character)
		c.Send("You stop using %s.\r\n", obj.Name())
		c.announce("%s stops using %s.\r\n", c.Character.Name, obj.Name())
	}
}

// wearAt puts an object in a slot and says so, porting perform_wear,
// wear_message and the equipping half of equip_char.
func (c *Context) wearAt(obj *game.Object, pos game.WearPosition) error {
	if pos < 0 {
		c.Send("You can't wear %s.\r\n", obj.Name())
		return nil
	}
	if !obj.FitsAt(pos) {
		c.Send("You can't wear %s there.\r\n", obj.Name())
		return nil
	}

	// The right hand, the first neck slot and the right wrist step aside for
	// the left one rather than refusing. Only these three, and only by one.
	switch pos {
	case game.WearFingerRight, game.WearNeck1, game.WearWristRight:
		if c.Character.Equipment[pos] != nil {
			pos++
		}
	}

	if c.Character.Level() < obj.MinLevel() {
		c.Send("You aren't experienced enough to use %s.\r\n", obj.Name())
		return nil
	}
	if c.Character.Equipment[pos] != nil {
		c.Send("%s", alreadyWearing[pos])
		return nil
	}

	// The message comes before the object moves, which is the C's order and
	// is why a zapping object announces itself as worn and then bites.
	c.Send(wearMessages[pos][1]+"\r\n", obj.Name())
	c.announce(wearMessages[pos][0]+"\r\n", c.Character.Name, obj.Name())

	// Alignment and class. An anti-good sword lets a paladin put it on and
	// then throws it back at them.
	if game.Zaps(c.Character.Record, obj) {
		c.Send("You are zapped by %s and instantly let go of it.\r\n", obj.Name())
		c.announce("%s is zapped by %s and instantly lets go of it.\r\n",
			c.Character.Name, obj.Name())
		c.World.ObjectToChar(obj, c.Character)
		return nil
	}

	if !c.World.Equip(obj, c.Character, pos) {
		c.Send("You can't seem to put %s on.\r\n", obj.Name())
	}
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

// findObject picks the object in a list a typed word names, porting
// get_obj_in_list_vis (handler.c:1124).
//
// Filtered on CAN_SEE_OBJ and honouring a `2.` prefix, both of which the C
// does here and neither of which this port did. A viewer who cannot see an
// object cannot pick it up by name either — which is what stops an invisible
// object being findable by somebody without detect invisible.
func (c *Context) findObject(list []*game.Object, word string) *game.Object {
	return c.World.NewSearch(c.Character, word).ObjectIn(list)
}

// findWearPosition picks the slot an unqualified `wear` uses, porting
// find_eq_pos with no argument.
//
// It is a run of `if`s with no `else` and no break, so the *last* one that
// matches wins rather than the first: something wearable on a finger and
// around the neck goes around the neck. Nothing here looks at whether the
// slot is free — perform_wear does that, and only it knows about the second
// ring finger.
//
// The list is also short of three slots. Light, wield and hold are not in it,
// so `wear torch` refuses and only `hold` and `wield` reach them.
func findWearPosition(obj *game.Object) game.WearPosition {
	where := game.WearPosition(-1)
	for _, fit := range []struct {
		flag game.WearFlag
		pos  game.WearPosition
	}{
		{game.ItemWearFinger, game.WearFingerRight},
		{game.ItemWearNeck, game.WearNeck1},
		{game.ItemWearBody, game.WearBody},
		{game.ItemWearHead, game.WearHead},
		{game.ItemWearLegs, game.WearLegs},
		{game.ItemWearFeet, game.WearFeet},
		{game.ItemWearHands, game.WearHands},
		{game.ItemWearArms, game.WearArms},
		{game.ItemWearShield, game.WearShield},
		{game.ItemWearAbout, game.WearAbout},
		{game.ItemWearWaist, game.WearWaist},
		{game.ItemWearWrist, game.WearWristRight},
	} {
		if obj.WearFlags.Has(fit.flag) {
			where = fit.pos
		}
	}
	return where
}

// wearPlaceNames are find_eq_pos's keywords[], in slot order. The empty
// entries are the C's "!RESERVED!", which cannot be matched because
// search_block refuses any argument beginning with `!`.
var wearPlaceNames = [game.NumWears]string{
	game.WearFingerRight: "finger",
	game.WearNeck1:       "neck",
	game.WearBody:        "body",
	game.WearHead:        "head",
	game.WearLegs:        "legs",
	game.WearFeet:        "feet",
	game.WearHands:       "hands",
	game.WearArms:        "arms",
	game.WearShield:      "shield",
	game.WearAbout:       "about",
	game.WearWaist:       "waist",
	game.WearWristRight:  "wrist",
}

// wearPositionNamed resolves `wear ring finger`, matching a prefix as
// search_block does.
func wearPositionNamed(arg string) (game.WearPosition, bool) {
	for pos, name := range wearPlaceNames {
		if name != "" && strings.HasPrefix(name, strings.ToLower(arg)) {
			return game.WearPosition(pos), true
		}
	}
	return -1, false
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
