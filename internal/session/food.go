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

	food := findObject(c.Character.Carrying, arg)
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
			Bits:     game.AffectPoison,
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
	vessel := findObject(c.Character.Carrying, arg)
	if vessel == nil {
		vessel = findObject(c.World.RoomObjects(c.Character.Room), arg)
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
	vessel.Values[1] -= amount

	// The vessel gets lighter, but never below nothing.
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
			Bits:     game.AffectPoison,
		}, false, false)
	}
	return nil
}

// doPour, porting do_pour: empty a container onto the ground.
func doPour(c *Context) error {
	arg := strings.TrimSpace(c.Arg)
	if arg == "" {
		c.Send("What do you want to pour out?\r\n")
		return nil
	}

	vessel := findObject(c.Character.Carrying, arg)
	if vessel == nil {
		c.Send("You can't find it!\r\n")
		return nil
	}
	if vessel.Type != game.ItemDrinkCon {
		c.Send("You can't pour from that!\r\n")
		return nil
	}
	if vessel.Values[1] <= 0 {
		c.Send("It's empty.\r\n")
		return nil
	}

	c.Send("You empty the %s from %s.\r\n", game.DrinkName(vessel.Values[2]), vessel.Name())
	c.announce("%s empties %s.\r\n", c.Character.Name, vessel.Name())

	vessel.Weight = max(0, vessel.Weight-vessel.Values[1])
	vessel.Values[1] = 0
	vessel.Values[2] = 0
	vessel.Values[3] = 0
	return nil
}
