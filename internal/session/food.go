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

// Eating and drinking, ported from do_eat and do_drink (act.item.c).
//
// Hunger and thirst have ticked down since the mud-hour tick landed, and
// starving quarters every kind of regeneration — so until now the only cure
// was to log out.

// stomachFull is the fullness above which nothing more will go in. The C
// tests `> 20` against a maximum of 24, so there is room for a little more
// after you are told you are full.
const stomachFull int32 = 20

// tooDrunkToDrink is the drunkenness above which you miss your mouth.
const tooDrunkToDrink int32 = 10

func doEat(c *Context) error   { return c.eat(false) }
func doTaste(c *Context) error { return c.eat(true) }

// eat, porting do_eat. taste is the same command with a subcommand flag: a
// nibble rather than a meal.
func (c *Context) eat(taste bool) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}

	arg := strings.TrimSpace(c.Arg)
	if arg == "" {
		if taste {
			c.Send("Taste what?\r\n")
		} else {
			c.Send("Eat what?\r\n")
		}
		return nil
	}

	food := c.findObject(c.Character.Carrying, arg)
	if food == nil {
		c.Send("You don't seem to have %s %s.\r\n", article(arg), arg)
		return nil
	}

	// Tasting a drink container is sipping it, which the C routes across
	// rather than refusing.
	if taste && (food.Type == game.ItemDrinkCon || food.Type == game.ItemFountain) {
		return c.drink(true)
	}

	if food.Type != game.ItemFood && c.Character.Level() < game.LevelGod {
		c.Send("You can't eat THAT!\r\n")
		return nil
	}
	if rec.Conditions[game.CondFull] > stomachFull {
		c.Send("You are too full to eat more!\r\n")
		return nil
	}

	if taste {
		c.Send("You nibble a little bit of %s.\r\n", food.Name())
		c.announce("%s tastes a little bit of %s.\r\n", c.Character.Name, food.Name())
	} else {
		c.Send("You eat %s.\r\n", food.Name())
		c.announce("%s eats %s.\r\n", c.Character.Name, food.Name())
	}

	// Value 0 is how filling it is. A taste is worth one whatever the food.
	amount := food.Values[0]
	if taste {
		amount = 1
	}
	game.GainCondition(rec, game.CondFull, amount)

	if rec.Conditions[game.CondFull] > stomachFull {
		c.Send("You are full.\r\n")
	}

	// Value 3 is the poison flag, and an immortal is immune.
	if food.Values[3] != 0 && c.Character.Level() < game.LevelImmortal {
		c.Send("Oops, that tasted rather strange!\r\n")
		c.announce("%s coughs and utters some strange sounds.\r\n", c.Character.Name)
		game.JoinAffect(rec, game.Affect{
			Type:     game.SpellPoison,
			Duration: amount * 2,
			Bits:     game.NewSet(game.AffectPoison),
		}, false, false)
	}

	if !taste {
		c.World.ExtractObject(food)
		return nil
	}
	// A taste takes one unit off; when it runs out the food is gone.
	food.Values[0]--
	if food.Values[0] <= 0 {
		c.Send("There's nothing left now.\r\n")
		c.World.ExtractObject(food)
	}
	return nil
}

func doDrink(c *Context) error { return c.drink(false) }
func doSip(c *Context) error   { return c.drink(true) }

// drink, porting do_drink.
func (c *Context) drink(sip bool) error {
	rec := c.Character.Record
	if c.Character.IsNPC() || rec == nil {
		return nil
	}

	arg := strings.TrimSpace(c.Arg)
	if arg == "" {
		c.Send("Drink from what?\r\n")
		return nil
	}

	// Carried first, then the floor — a fountain is drunk from where it
	// stands, a bottle has to be held.
	onGround := false
	vessel := c.findObject(c.Character.Carrying, arg)
	if vessel == nil {
		vessel = c.findObject(c.World.RoomObjects(c.Character.Room), arg)
		onGround = true
	}
	if vessel == nil {
		c.Send("You can't find it!\r\n")
		return nil
	}

	if vessel.Type != game.ItemDrinkCon && vessel.Type != game.ItemFountain {
		c.Send("You can't drink from that!\r\n")
		return nil
	}
	if onGround && vessel.Type == game.ItemDrinkCon {
		c.Send("You have to be holding that to drink from it.\r\n")
		return nil
	}

	// Both of these only bite while you are not actually thirsty, which is
	// the C being merciful: a parched drunk can still drink.
	if rec.Conditions[game.CondDrunk] > tooDrunkToDrink && rec.Conditions[game.CondThirst] > 0 {
		c.Send("You can't seem to get close enough to your mouth.\r\n")
		c.announce("%s tries to drink but misses their mouth!\r\n", c.Character.Name)
		return nil
	}
	if rec.Conditions[game.CondFull] > stomachFull && rec.Conditions[game.CondThirst] > 0 {
		c.Send("Your stomach can't contain anymore!\r\n")
		return nil
	}

	// Value 1 is how much is left, value 2 which liquid it is.
	if vessel.Values[1] <= 0 {
		c.Send("It's empty.\r\n")
		return nil
	}
	liquid := vessel.Values[2]
	name := game.DrinkName(liquid)

	var amount int32
	if sip {
		c.Send("It tastes like %s.\r\n", name)
		c.announce("%s sips from %s.\r\n", c.Character.Name, vessel.Name())
		amount = 1
	} else {
		c.Send("You drink the %s.\r\n", name)
		c.announce("%s drinks %s from %s.\r\n", c.Character.Name, name, vessel.Name())
		amount = game.DrinkAmount(liquid, rec.Conditions[game.CondThirst], c.RNG)
	}

	amount = min(amount, vessel.Values[1])

	// The vessel gets lighter, but never below nothing. The C changes the
	// weight here and the contents at the very end, after the poison; the
	// order does not matter but the emptying does — the last mouthful takes
	// the liquid's keyword off the container's name with it.
	vessel.Weight = max(0, vessel.Weight-amount)

	// The table is per four units, which is why every term divides by four.
	effect := game.DrinkEffect(liquid)
	for _, cond := range []game.Condition{game.CondDrunk, game.CondFull, game.CondThirst} {
		game.GainCondition(rec, cond, effect[cond]*amount/4)
	}

	if rec.Conditions[game.CondDrunk] > tooDrunkToDrink {
		c.Send("You feel drunk.\r\n")
	}
	if rec.Conditions[game.CondThirst] > stomachFull {
		c.Send("You don't feel thirsty any more.\r\n")
	}
	if rec.Conditions[game.CondFull] > stomachFull {
		c.Send("You are full.\r\n")
	}

	// Value 3 is the poison flag on the liquid.
	if vessel.Values[3] != 0 {
		c.Send("Oops, it tasted rather strange!\r\n")
		c.announce("%s chokes and utters some strange sounds.\r\n", c.Character.Name)
		game.JoinAffect(rec, game.Affect{
			Type:     game.SpellPoison,
			Duration: amount * 3,
			Bits:     game.NewSet(game.AffectPoison),
		}, false, false)
	}

	vessel.Values[1] -= amount
	if vessel.Values[1] == 0 {
		// The last of it. The liquid's keyword comes off the container's
		// name, so an empty bottle stops answering to `water`, and it stops
		// being poisoned along with it.
		game.NameFromDrinkCon(vessel)
		vessel.Values[2] = 0
		vessel.Values[3] = 0
	}
	return nil
}

func doPour(c *Context) error { return c.pour(false) }
func doFill(c *Context) error { return c.pour(true) }

// pour, porting do_pour. One function and two subcommands: `pour <thing>
// out`, `pour <thing> into <thing>`, and `fill <thing> from <fountain>`.
//
// The arithmetic in the second half is the interesting part. It fills the
// destination to its capacity outright, subtracts that much from the source,
// and then — if the source has gone *negative*, which is how it finds out
// there was not enough — adds the shortfall back to both. It arrives at the
// right answer by overshooting and correcting.
func (c *Context) pour(filling bool) error {
	arg1, arg2, _ := twoArguments(c.Arg)

	var from, to *game.Object

	if !filling {
		if arg1 == "" {
			c.Send("From what do you want to pour?\r\n")
			return nil
		}
		if from = c.findObject(c.Character.Carrying, arg1); from == nil {
			c.Send("You can't find it!\r\n")
			return nil
		}
		if from.Type != game.ItemDrinkCon {
			c.Send("You can't pour from that!\r\n")
			return nil
		}
	} else {
		if arg1 == "" {
			c.Send("What do you want to fill?  And what are you filling it from?\r\n")
			return nil
		}
		if to = c.findObject(c.Character.Carrying, arg1); to == nil {
			c.Send("You can't find it!\r\n")
			return nil
		}
		if to.Type != game.ItemDrinkCon {
			c.Send("You can't fill %s!\r\n", to.Name())
			return nil
		}
		if arg2 == "" {
			c.Send("What do you want to fill %s from?\r\n", to.Name())
			return nil
		}
		// Filling is from something standing in the room, and only from a
		// fountain: you cannot fill a waterskin from a bottle.
		if from = c.findObject(c.World.RoomObjects(c.Character.Room), arg2); from == nil {
			c.Send("There doesn't seem to be %s %s here.\r\n", article(arg2), arg2)
			return nil
		}
		if from.Type != game.ItemFountain {
			c.Send("You can't fill something from %s.\r\n", from.Name())
			return nil
		}
	}

	if from.Values[1] == 0 {
		// The C's "The $p is empty." reads oddly because $p is already "a
		// bottle"; it comes out as "The a bottle is empty."
		c.Send("The %s is empty.\r\n", from.Name())
		return nil
	}

	if !filling {
		switch arg2 {
		case "":
			c.Send("Where do you want it?  Out or in what?\r\n")
			return nil
		case "out":
			c.Send("You empty %s.\r\n", from.Name())
			c.announce("%s empties %s.\r\n", c.Character.Name, from.Name())
			emptyDrinkContainer(from)
			return nil
		}
		if to = c.findObject(c.Character.Carrying, arg2); to == nil {
			c.Send("You can't find it!\r\n")
			return nil
		}
		if to.Type != game.ItemDrinkCon && to.Type != game.ItemFountain {
			c.Send("You can't pour anything into that.\r\n")
			return nil
		}
	}

	switch {
	case to == from:
		c.Send("A most unproductive effort.\r\n")
		return nil
	case to.Values[1] != 0 && to.Values[2] != from.Values[2]:
		c.Send("There is already another liquid in it!\r\n")
		return nil
	case to.Values[1] >= to.Values[0]:
		c.Send("There is no room for more.\r\n")
		return nil
	}

	if filling {
		c.Send("You gently fill %s from %s.\r\n", to.Name(), from.Name())
		c.announce("%s gently fills %s from %s.\r\n", c.Character.Name, to.Name(), from.Name())
	} else {
		// The C names the destination with the word the player typed rather
		// than with the object's own name, and forgets the newline. Both
		// reproduced; see docs/weirdnumbers.md.
		c.Send("You pour the %s into the %s.", game.DrinkName(from.Values[2]), arg2)
	}

	if to.Values[1] == 0 {
		game.NameToDrinkCon(to, from.Values[2])
	}
	to.Values[2] = from.Values[2]

	// Fill it to the brim, then find out whether there was that much.
	amount := to.Values[0] - to.Values[1]
	from.Values[1] -= amount
	to.Values[1] = to.Values[0]

	if from.Values[1] < 0 {
		to.Values[1] += from.Values[1]
		amount += from.Values[1]
		emptyDrinkContainer(from)
	}

	// "Then the poison boogie", says the C: poison travels with the liquid
	// and never dilutes.
	if from.Values[3] != 0 {
		to.Values[3] = 1
	}

	from.Weight = max(0, from.Weight-amount)
	to.Weight += amount
	return nil
}

// emptyDrinkContainer empties a vessel and takes the liquid's keyword back
// off its name, which is the pair the C repeats in four places.
func emptyDrinkContainer(o *game.Object) {
	o.Weight = max(0, o.Weight-o.Values[1])
	game.NameFromDrinkCon(o)
	o.Values[1] = 0
	o.Values[2] = 0
	o.Values[3] = 0
}
