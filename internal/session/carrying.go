// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package session

import "github.com/gerrowadat/disgracelands/internal/game"

// Moving objects between the floor, your hands, a container and somebody
// else: do_get, do_drop, do_put and do_give from act.item.c.
//
// Each of these is a two-layer arrangement in the C — a `perform_` function
// that does the thing to one object and says so, and an ACMD that works out
// which objects the typed words meant — and the argument-working-out is the
// larger half. `get all.coin corpse` and `put 3 potion bag` both reach the
// same one-object routine by very different roads.
//
// The one rule that is not obvious anywhere in the C: which checks apply
// depends on *where* the object was. Something taken from the floor is
// checked against the take flag and your carrying weight; the same object
// taken out of a bag you are already holding is checked against neither,
// because you were carrying its weight either way. That is why you can pull a
// two-tonne anvil out of your own backpack.

func doGet(c *Context) error {
	arg1, arg2, rest := twoArguments(c.Arg)
	arg3, _ := oneArgument(rest)

	switch {
	case arg1 == "":
		c.Send("Get what?\r\n")
	case arg2 == "":
		// `get sword`.
		c.getFromRoom(arg1, 1)
	case isNumber(arg1) && arg3 == "":
		// `get 5 coins`.
		c.getFromRoom(arg2, atoi(arg1))
	default:
		// `get sword bag`, or `get 5 potion bag`.
		amount := int32(1)
		what, where := arg1, arg2
		if isNumber(arg1) {
			amount, what, where = atoi(arg1), arg2, arg3
		}
		c.getFromContainers(what, where, amount)
	}
	return nil
}

// getFromRoom takes things off the floor, porting get_from_room.
func (c *Context) getFromRoom(arg string, howmany int32) {
	mode, arg := findAllDots(arg)

	if mode == findIndiv {
		matches := matchingObjects(c.World.RoomObjects(c.Character.Room), arg, howmany)
		if len(matches) == 0 {
			c.Send("You don't see %s %s here.\r\n", article(arg), arg)
			return
		}
		for _, obj := range matches {
			c.getObjectFromRoom(obj)
		}
		return
	}

	if mode == findAllDot && arg == "" {
		c.Send("Get all of what?\r\n")
		return
	}

	var found bool
	for _, obj := range everything(c.World.RoomObjects(c.Character.Room), mode, arg) {
		found = true
		c.getObjectFromRoom(obj)
	}
	if found {
		return
	}
	if mode == findAll {
		c.Send("There doesn't seem to be anything here.\r\n")
	} else {
		c.Send("You don't see any %ss here.\r\n", arg)
	}
}

// getObjectFromRoom picks one thing up, porting perform_get_from_room.
func (c *Context) getObjectFromRoom(obj *game.Object) {
	if !c.canTake(obj) {
		return
	}
	c.World.ObjectToChar(obj, c.Character)
	c.Send("You get %s.\r\n", obj.Name())
	c.announce("%s gets %s.\r\n", c.Character.Name, obj.Name())
	c.checkMoney(obj)
}

// getFromContainers works out which containers `get <thing> <where>` meant,
// porting the second half of do_get.
func (c *Context) getFromContainers(what, where string, howmany int32) {
	mode, where := findAllDots(where)

	if mode == findIndiv {
		cont, onGround := c.findContainer(where)
		switch {
		case cont == nil:
			// "don't have", not "don't see": the C's message assumes you were
			// looking in your own pockets even when you were not.
			c.Send("You don't have %s %s.\r\n", article(where), where)
		case !cont.IsContainer():
			c.Send("%s is not a container.\r\n", capitaliseFirst(cont.Name()))
		default:
			c.getFromContainer(cont, what, onGround, howmany)
		}
		return
	}

	if mode == findAllDot && where == "" {
		c.Send("Get from all of what?\r\n")
		return
	}

	// Carried containers first, then the ones on the floor — and a named
	// thing that is not a container is complained about only for `all.x`,
	// never for a bare `all`.
	var found bool
	for _, onGround := range []bool{false, true} {
		list := c.Character.Carrying
		if onGround {
			list = c.World.RoomObjects(c.Character.Room)
		}
		for _, cont := range everything(list, mode, where) {
			switch {
			case cont.IsContainer():
				found = true
				c.getFromContainer(cont, what, onGround, howmany)
			case mode == findAllDot:
				found = true
				c.Send("%s is not a container.\r\n", capitaliseFirst(cont.Name()))
			}
		}
	}
	if found {
		return
	}
	if mode == findAll {
		c.Send("You can't seem to find any containers.\r\n")
	} else {
		c.Send("You can't seem to find any %ss here.\r\n", where)
	}
}

// getFromContainer takes things out of one container, porting
// get_from_container.
func (c *Context) getFromContainer(cont *game.Object, arg string, onGround bool, howmany int32) {
	mode, arg := findAllDots(arg)

	if cont.ContainerClosed() {
		c.Send("%s is closed.\r\n", capitaliseFirst(cont.Name()))
		return
	}

	if mode == findIndiv {
		matches := matchingObjects(cont.Contents, arg, howmany)
		if len(matches) == 0 {
			c.Send("There doesn't seem to be %s %s in %s.\r\n", article(arg), arg, cont.Name())
			return
		}
		for _, obj := range matches {
			c.getObjectFromContainer(obj, cont, onGround)
		}
		return
	}

	if mode == findAllDot && arg == "" {
		c.Send("Get all of what?\r\n")
		return
	}

	var found bool
	for _, obj := range everything(cont.Contents, mode, arg) {
		found = true
		c.getObjectFromContainer(obj, cont, onGround)
	}
	if found {
		return
	}
	if mode == findAll {
		c.Send("%s seems to be empty.\r\n", capitaliseFirst(cont.Name()))
	} else {
		c.Send("You can't seem to find any %ss in %s.\r\n", arg, cont.Name())
	}
}

// getObjectFromContainer takes one thing out, porting
// perform_get_from_container.
//
// A container on the floor is treated as the floor — take flag, weight, the
// lot. One you are carrying skips all of that and only counts items, because
// the weight was already yours.
func (c *Context) getObjectFromContainer(obj, cont *game.Object, onGround bool) {
	if onGround && !c.canTake(obj) {
		return
	}
	if c.handsFull() {
		c.Send("%s: you can't hold any more items.\r\n", capitaliseFirst(obj.Name()))
		return
	}

	c.World.ObjectToChar(obj, c.Character)
	c.Send("You get %s from %s.\r\n", obj.Name(), cont.Name())
	c.announce("%s gets %s from %s.\r\n", c.Character.Name, obj.Name(), cont.Name())
	c.checkMoney(obj)
}

// checkMoney turns a picked-up pile of coins into gold, porting
// get_check_money.
//
// The object is destroyed rather than carried, which is why gold never shows
// up in an inventory. The C runs this after the "You get $p." line, so a
// player sees themselves pick up a pile and then be told what was in it.
func (c *Context) checkMoney(obj *game.Object) {
	if obj.Type != game.ItemMoney || obj.Values[0] <= 0 || c.Character.Record == nil {
		return
	}

	value := obj.Values[0]
	c.World.ExtractObject(obj)
	c.Character.Record.Points.Gold += value

	if value == 1 {
		c.Send("There was 1 coin.\r\n")
		return
	}
	c.Send("There were %d coins.\r\n", value)
}

// doDrop puts things on the floor, porting do_drop.
//
// The C's do_drop is also `junk` and `donate` by way of a subcommand, and both
// of those need places to put things that do not exist yet; this is the drop
// case only.
func doDrop(c *Context) error {
	arg, rest := oneArgument(c.Arg)

	switch {
	case arg == "":
		c.Send("What do you want to drop?\r\n")

	case isNumber(arg):
		multi := atoi(arg)
		word, _ := oneArgument(rest)
		switch {
		case word == "coins" || word == "coin":
			c.dropGold(multi)
		case multi <= 0:
			c.Send("Yeah, that makes sense.\r\n")
		case word == "":
			c.Send("What do you want to drop %d of?\r\n", multi)
		default:
			matches := matchingObjects(c.Character.Carrying, word, multi)
			if len(matches) == 0 {
				c.Send("You don't seem to have any %ss.\r\n", word)
				return nil
			}
			for _, obj := range matches {
				c.dropObject(obj)
			}
		}

	default:
		mode, word := findAllDots(arg)
		switch {
		case mode == findAll && len(c.Character.Carrying) == 0:
			c.Send("You don't seem to be carrying anything.\r\n")
		case mode == findAllDot && word == "":
			c.Send("What do you want to drop all of?\r\n")
		case mode == findIndiv:
			obj := findObject(c.Character.Carrying, word)
			if obj == nil {
				c.Send("You don't seem to have %s %s.\r\n", article(word), word)
				return nil
			}
			c.dropObject(obj)
		default:
			matched := everything(c.Character.Carrying, mode, word)
			if len(matched) == 0 && mode == findAllDot {
				c.Send("You don't seem to have any %ss.\r\n", word)
			}
			for _, obj := range matched {
				c.dropObject(obj)
			}
		}
	}
	return nil
}

// dropObject drops one thing, porting perform_drop's SCMD_DROP case.
func (c *Context) dropObject(obj *game.Object) {
	if obj.ExtraFlags.Has(game.ItemNoDrop) {
		c.Send("You can't drop %s, it must be CURSED!\r\n", obj.Name())
		return
	}
	c.Send("You drop %s.\r\n", obj.Name())
	c.announce("%s drops %s.\r\n", c.Character.Name, obj.Name())
	c.World.ObjectToRoom(obj, c.Character.Room)
}

// dropGold drops coins, porting perform_drop_gold.
//
// The wait state is the interesting part, and the C says why: it is there "to
// prevent coin-bombing", where a player drops their gold a coin at a time to
// flood everyone else's screen.
func (c *Context) dropGold(amount int32) {
	rec := c.Character.Record
	if rec == nil {
		return
	}

	switch {
	case amount <= 0:
		c.Send("Heh heh heh.. we are jolly funny today, eh?\r\n")
		return
	case rec.Points.Gold < amount:
		c.Send("You don't have that many coins!\r\n")
		return
	}

	c.Character.Wait(1, c.roundLength())

	c.Send("You drop some gold.\r\n")
	c.announce("%s drops %s.\r\n", c.Character.Name, game.MoneyDescription(amount))
	c.World.ObjectToRoom(c.World.MakeMoney(amount), c.Character.Room)
	rec.Points.Gold -= amount
}

// doPut, porting do_put.
func doPut(c *Context) error {
	arg1, arg2, rest := twoArguments(c.Arg)
	arg3, _ := oneArgument(rest)

	// `put 3 potion bag` names a count first; `put potion bag` does not.
	theObj, theCont := arg1, arg2
	howmany := int32(1)
	if arg3 != "" && isNumber(arg1) {
		howmany, theObj, theCont = atoi(arg1), arg2, arg3
	}

	objMode, theObj := findAllDots(theObj)
	contMode, theCont := findAllDots(theCont)

	switch {
	case theObj == "" && objMode == findIndiv:
		c.Send("Put what in what?\r\n")
		return nil
	case contMode != findIndiv:
		c.Send("You can only put things into one container at a time.\r\n")
		return nil
	case theCont == "":
		it := "them"
		if objMode == findIndiv {
			it = "it"
		}
		c.Send("What do you want to put %s in?\r\n", it)
		return nil
	}

	cont, _ := c.findContainer(theCont)
	switch {
	case cont == nil:
		c.Send("You don't see %s %s here.\r\n", article(theCont), theCont)
		return nil
	case !cont.IsContainer():
		c.Send("%s is not a container.\r\n", capitaliseFirst(cont.Name()))
		return nil
	case cont.ContainerClosed():
		c.Send("You'd better open it first!\r\n")
		return nil
	}

	if objMode == findIndiv {
		matches := matchingObjects(c.Character.Carrying, theObj, howmany)
		switch {
		case len(matches) == 0:
			c.Send("You aren't carrying %s %s.\r\n", article(theObj), theObj)
		case matches[0] == cont:
			c.Send("You attempt to fold it into itself, but fail.\r\n")
		default:
			for _, obj := range matches {
				c.putObject(obj, cont)
			}
		}
		return nil
	}

	var found bool
	for _, obj := range everything(c.Character.Carrying, objMode, theObj) {
		if obj == cont {
			continue
		}
		found = true
		c.putObject(obj, cont)
	}
	if found {
		return nil
	}
	if objMode == findAll {
		c.Send("You don't seem to have anything to put in it.\r\n")
	} else {
		c.Send("You don't seem to have any %ss.\r\n", theObj)
	}
	return nil
}

// putObject puts one thing in a container, porting perform_put.
//
// The capacity check counts the container's own weight against its capacity,
// because the C keeps a container's weight and its contents' weight in the
// same field; see docs/weirdnumbers.md.
//
// The curse spreads: putting a cursed object into a bag curses the bag. The C
// carries a comment from its author admitting this is strange and blaming the
// lack of auto-equip on rent, and it has been in every CircleMUD since.
func (c *Context) putObject(obj, cont *game.Object) {
	if cont.TotalWeight()+obj.TotalWeight() > cont.Capacity() {
		c.Send("%s won't fit in %s.\r\n", capitaliseFirst(obj.Name()), cont.Name())
		return
	}

	c.World.ObjectToObject(obj, cont)
	c.announce("%s puts %s in %s.\r\n", c.Character.Name, obj.Name(), cont.Name())

	if obj.ExtraFlags.Has(game.ItemNoDrop) && !cont.ExtraFlags.Has(game.ItemNoDrop) {
		cont.ExtraFlags = cont.ExtraFlags.Set(game.ItemNoDrop)
		c.Send("You get a strange feeling as you put %s in %s.\r\n", obj.Name(), cont.Name())
		return
	}
	c.Send("You put %s in %s.\r\n", obj.Name(), cont.Name())
}

// doGive, porting do_give.
func doGive(c *Context) error {
	arg, rest := oneArgument(c.Arg)

	switch {
	case arg == "":
		c.Send("Give what to who?\r\n")
		return nil

	case isNumber(arg):
		// `give 20 coins zod`, or `give 3 potion zod`.
		amount := atoi(arg)
		word, after := oneArgument(rest)
		switch word {
		case "coins", "coin":
			who, _ := oneArgument(after)
			if victim := c.giveFindVictim(who); victim != nil {
				c.giveGold(victim, amount)
			}
		case "":
			c.Send("What do you want to give %d of?\r\n", amount)
		default:
			victim := c.giveFindVictim(after)
			if victim == nil {
				return nil
			}
			matches := matchingObjects(c.Character.Carrying, word, amount)
			if len(matches) == 0 {
				c.Send("You don't seem to have any %ss.\r\n", word)
				return nil
			}
			for _, obj := range matches {
				c.giveObject(victim, obj)
			}
		}
		return nil
	}

	// `give sword zod`, `give all zod`, `give all.potion zod`.
	who, _ := oneArgument(rest)
	victim := c.giveFindVictim(who)
	if victim == nil {
		return nil
	}

	mode, word := findAllDots(arg)
	switch {
	case mode == findIndiv:
		obj := findObject(c.Character.Carrying, word)
		if obj == nil {
			c.Send("You don't seem to have %s %s.\r\n", article(word), word)
			return nil
		}
		c.giveObject(victim, obj)
	case mode == findAllDot && word == "":
		c.Send("All of what?\r\n")
	case len(c.Character.Carrying) == 0:
		c.Send("You don't seem to be holding anything.\r\n")
	default:
		for _, obj := range everything(c.Character.Carrying, mode, word) {
			c.giveObject(victim, obj)
		}
	}
	return nil
}

// giveFindVictim finds who to give something to, porting give_find_vict. It
// says why not, so a nil return means the player has already been told.
func (c *Context) giveFindVictim(arg string) *game.Character {
	if arg == "" {
		c.Send("To who?\r\n")
		return nil
	}
	victim := c.World.FindInRoom(c.Character.Room, arg)
	if victim == nil {
		c.Send("No-one by that name here.\r\n")
		return nil
	}
	if victim == c.Character {
		c.Send("What's the point of that?\r\n")
		return nil
	}
	return victim
}

// giveObject hands one thing over, porting perform_give.
func (c *Context) giveObject(victim *game.Character, obj *game.Object) {
	if obj.ExtraFlags.Has(game.ItemNoDrop) {
		c.Send("You can't let go of %s!!  Yeech!\r\n", obj.Name())
		return
	}
	if handsFull(victim) {
		c.Send("%s seems to have %s hands full.\r\n", victim.Name, victim.Possessive())
		return
	}
	if obj.TotalWeight()+victim.CarriedWeight() > carryWeight(victim) {
		c.Send("%s can't carry that much weight.\r\n", capitaliseFirst(victim.Subject()))
		return
	}

	c.World.ObjectToChar(obj, victim)
	c.Send("You give %s to %s.\r\n", obj.Name(), victim.Name)
	victim.Tell("%s gives you %s.\r\n", c.Character.Name, obj.Name())
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != victim {
			other.Tell("%s gives %s to %s.\r\n", c.Character.Name, obj.Name(), victim.Name)
		}
	}
}

// giveGold hands over coins, porting perform_give_gold.
//
// An immortal's gold is not deducted — the C tests the level twice, once to
// decide whether they can afford it and once to decide whether to charge them,
// so a god can hand out unlimited money without ever running out.
func (c *Context) giveGold(victim *game.Character, amount int32) {
	rec := c.Character.Record
	if rec == nil || victim.Record == nil {
		return
	}
	free := !c.Character.IsNPC() && c.Character.Level() >= game.LevelGod

	switch {
	case amount <= 0:
		c.Send("Heh heh heh ... we are jolly funny today, eh?\r\n")
		return
	case rec.Points.Gold < amount && !free:
		c.Send("You don't have that many coins!\r\n")
		return
	}

	c.Send("Okay.\r\n")
	coins := "coins"
	if amount == 1 {
		coins = "coin"
	}
	victim.Tell("%s gives you %d gold %s.\r\n", c.Character.Name, amount, coins)
	for _, other := range c.World.Occupants(c.Character.Room) {
		if other != c.Character && other != victim {
			other.Tell("%s gives %s to %s.\r\n",
				c.Character.Name, game.MoneyDescription(amount), victim.Name)
		}
	}

	if !free {
		rec.Points.Gold -= amount
	}
	victim.Record.Points.Gold += amount
}

// findContainer locates a container by name the way generic_find does with
// FIND_OBJ_INV|FIND_OBJ_ROOM: inventory first, then the floor. The second
// return says which, because it decides whether taking from it counts as
// taking from the ground.
func (c *Context) findContainer(name string) (obj *game.Object, onGround bool) {
	if obj := findObject(c.Character.Carrying, name); obj != nil {
		return obj, false
	}
	return findObject(c.World.RoomObjects(c.Character.Room), name), true
}

// matchingObjects returns up to howmany objects a word names, in list order.
//
// The C walks the list re-searching from the last match each time, which comes
// to the same thing. A count of zero matches nothing at all — `get 0 sword`
// is silently obeyed, because the C's `while (obj && howmany--)` tests before
// it decrements.
func matchingObjects(list []*game.Object, word string, howmany int32) []*game.Object {
	var out []*game.Object
	for _, obj := range list {
		if len(out) >= int(howmany) {
			break
		}
		if obj.Matches(word) {
			out = append(out, obj)
		}
	}
	return out
}

// everything returns every object a dotmode matches: all of them for `all`,
// the named ones for `all.x`.
//
// The result is a snapshot, because every caller moves the objects it is
// given and the C is careful to remember `next_content` before it does.
func everything(list []*game.Object, mode dotMode, word string) []*game.Object {
	var out []*game.Object
	for _, obj := range list {
		if mode == findAll || obj.Matches(word) {
			out = append(out, obj)
		}
	}
	return out
}

// canTake reports whether a character may pick something up, porting
// can_take_obj, and says why not.
func (c *Context) canTake(obj *game.Object) bool {
	switch {
	case c.handsFull():
		c.Send("%s: you can't carry that many items.\r\n", capitaliseFirst(obj.Name()))
	case c.Character.CarriedWeight()+obj.TotalWeight() > carryWeight(c.Character):
		c.Send("%s: you can't carry that much weight.\r\n", capitaliseFirst(obj.Name()))
	case !obj.Takeable():
		c.Send("%s: you can't take that!\r\n", capitaliseFirst(obj.Name()))
	default:
		return true
	}
	return false
}

// handsFull reports whether a character is carrying as many separate items as
// they can, porting CAN_CARRY_N.
func (c *Context) handsFull() bool { return handsFull(c.Character) }

func handsFull(c *game.Character) bool {
	if c.Record == nil {
		return false
	}
	// CAN_CARRY_N: 5 + dex/2 + level/2 (utils.h:346). The C writes the
	// halvings as `>> 1`, which is the same for the non-negative values this
	// sees; see docs/weirdnumbers.md.
	limit := 5 + c.Record.Abilities.Dexterity/2 + c.Record.Level/2
	return int64(len(c.Carrying)) >= int64(limit)
}

// carryWeight is CAN_CARRY_W: the strength table's carry weight.
func carryWeight(c *game.Character) int32 {
	if c.Record == nil {
		return 0
	}
	return game.Strength(c.Record.Abilities.Strength, c.Record.Abilities.StrengthPercentile).CarryWeight
}
