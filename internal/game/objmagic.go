// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "fmt"

// The spells that happen to objects: mag_alter_objs (magic.c:946) and the
// manual spells from spells.c that take an object rather than a person.
//
// Every one of them is written as "if it is not already so, make it so, and
// say something" — and the refusals are all silent. A cursed object cursed
// again produces "Nothing seems to happen", which is indistinguishable from
// the spell failing, and that is the C's behaviour and the reason a mage
// cannot tell whether their curse landed.

// AlterObject applies an object-altering spell, porting mag_alter_objs.
//
// It returns the message to print, with the object already named, or "" if
// nothing happened. The C prints the same line to the caster and to the room,
// which is why there is only one.
func AlterObject(spell SpellID, obj *Object, casterLevel int32) string {
	if obj == nil {
		return ""
	}

	switch spell {
	case SpellBless:
		// Weight is the limit, and it scales with the caster: five pounds
		// per level. A twentieth-level cleric cannot bless a suit of plate.
		if !obj.ExtraFlags.Has(ItemBless) && obj.TotalWeight() <= 5*casterLevel {
			obj.ExtraFlags = obj.ExtraFlags.With(ItemBless)
			return capitaliseFirst(obj.Name()) + " glows briefly."
		}

	case SpellCurse:
		if !obj.ExtraFlags.Has(ItemNoDrop) {
			obj.ExtraFlags = obj.ExtraFlags.With(ItemNoDrop)
			// A cursed weapon also loses a point of damage size — value 2 is
			// the die, so a 2d6 sword becomes 2d5. Curse it repeatedly and
			// the die goes to nothing, except that the flag stops it.
			if weapon, ok := obj.WeaponValues(); ok {
				weapon.Dice.Size--
				obj.SetWeaponDice(weapon.Dice)
			}
			return capitaliseFirst(obj.Name()) + " briefly glows red."
		}

	case SpellInvisible:
		if !obj.ExtraFlags.HasAny(ItemNoInvis, ItemInvisible) {
			obj.ExtraFlags = obj.ExtraFlags.With(ItemInvisible)
			return capitaliseFirst(obj.Name()) + " vanishes."
		}

	case SpellPoison:
		if obj.Consumable() && !obj.Poisoned() {
			obj.SetPoisoned(true)
			return capitaliseFirst(obj.Name()) + " steams briefly."
		}

	case SpellRemoveCurse:
		if obj.ExtraFlags.Has(ItemNoDrop) {
			obj.ExtraFlags = obj.ExtraFlags.Without(ItemNoDrop)
			if weapon, ok := obj.WeaponValues(); ok {
				weapon.Dice.Size++
				obj.SetWeaponDice(weapon.Dice)
			}
			return capitaliseFirst(obj.Name()) + " briefly glows blue."
		}

	case SpellRemovePoison:
		if obj.Consumable() && obj.Poisoned() {
			obj.SetPoisoned(false)
			return capitaliseFirst(obj.Name()) + " steams briefly."
		}
	}
	return ""
}

// consumable reports whether an object is something that can be poisoned:
// the two liquid types and food.
// CreateFoodVnum is the object mag_creations makes for `create food`
// (magic.c:1016). It is a bare 10 in the C, with no constant and no comment.
const CreateFoodVnum ObjVnum = 10

// FillWithWater fills a container, porting spell_create_water (spells.c:43).
//
// The odd branch is the first one: casting it on a container holding anything
// that is not water turns the contents to *slime* rather than filling it.
// Emptying the bottle first is the only way to get water into it, and the
// spell gives no hint of that.
func FillWithWater(obj *Object) string {
	if obj == nil || obj.Type != ItemDrinkCon {
		return ""
	}

	contents, _ := obj.DrinkValues()

	if contents.Liquid != LiquidWater && contents.Filled != 0 {
		NameFromDrinkCon(obj)
		obj.SetDrinkLiquid(LiquidSlime)
		NameToDrinkCon(obj, LiquidSlime)
		return ""
	}

	water := max(contents.Capacity-contents.Filled, 0)
	if water <= 0 {
		return ""
	}
	// `>= 0` and not `> 0`: the C takes the keyword off even when the
	// container was already empty, which is a no-op there and is why this
	// reads oddly rather than wrongly.
	if contents.Filled >= 0 {
		NameFromDrinkCon(obj)
	}
	obj.SetDrinkLiquid(LiquidWater)
	obj.SetDrinkFilled(contents.Filled + water)
	NameToDrinkCon(obj, LiquidWater)
	obj.Weight += water

	return capitaliseFirst(obj.Name()) + " is filled."
}

// EnchantWeapon puts hitroll and damroll on a weapon, porting
// spell_enchant_weapon (spells.c:394).
//
// The bonus is a bare `1 + (level >= 18)` — a C boolean used as a number —
// so it is +1 until the caster is level 18 and +2 after, with nothing in
// between and no further growth. Damroll crosses over at 20 rather than 18,
// for no reason the code gives.
//
// It refuses silently on anything already magical or carrying any apply at
// all, which is why enchanting a ring that gives +1 strength does nothing and
// says nothing.
func EnchantWeapon(obj *Object, caster *PlayerRecord, level int32) string {
	if obj == nil || caster == nil {
		return ""
	}
	if obj.Type != ItemWeapon || obj.ExtraFlags.Has(ItemMagic) {
		return ""
	}
	for _, a := range obj.Affects {
		if a.Location != ApplyNone {
			return ""
		}
	}

	obj.ExtraFlags = obj.ExtraFlags.With(ItemMagic)
	obj.Affects = []ObjAffect{
		{Location: ApplyHitRoll, Modifier: 1 + boolToInt(level >= 18)},
		{Location: ApplyDamRoll, Modifier: 1 + boolToInt(level >= 20)},
	}

	// The weapon takes the caster's side, and says so in their colour.
	switch {
	case IsGood(caster):
		obj.ExtraFlags = obj.ExtraFlags.With(ItemAntiEvil)
		return capitaliseFirst(obj.Name()) + " glows blue."
	case IsEvil(caster):
		obj.ExtraFlags = obj.ExtraFlags.With(ItemAntiGood)
		return capitaliseFirst(obj.Name()) + " glows red."
	}
	return capitaliseFirst(obj.Name()) + " glows yellow."
}

func boolToInt(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// PoisonReport is what detect poison says about an object, porting the object
// half of spell_detect_poison (spells.c:429).
func PoisonReport(obj *Object) string {
	if obj == nil {
		return ""
	}
	if !obj.Consumable() {
		return "You sense that it should not be consumed.\r\n"
	}
	if obj.Poisoned() {
		return fmt.Sprintf("You sense that %s has been contaminated.\r\n", obj.Name())
	}
	return fmt.Sprintf("You sense that %s is safe for consumption.\r\n", obj.Name())
}
