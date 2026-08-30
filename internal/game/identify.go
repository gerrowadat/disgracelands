// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"fmt"
	"strings"
	"time"
)

// Identify, ported from spell_identify (spells.c:289).
//
// It is the least magical spell in the game: it prints the object's record,
// field by field, in the layout a debugger would use. That is the whole
// appeal — everything else in the game hides its numbers, and this shows them
// with the flag names still in capitals.

// IdentifyObject is the report for an object.
func IdentifyObject(obj *Object) string {
	if obj == nil {
		return ""
	}

	var out strings.Builder
	fmt.Fprintf(&out, "You feel informed:\r\n")
	fmt.Fprintf(&out, "Object '%s', Item type: %s\r\n",
		obj.Name(), SprintType(obj.Type, ItemTypeNames))

	if obj.PermAffect != 0 {
		fmt.Fprintf(&out, "Item will give you following abilities:  %s\r\n",
			SprintBit(uint64(obj.PermAffect), affectBitNames))
	}
	fmt.Fprintf(&out, "Item is: %s\r\n", SprintBit(obj.ExtraFlags.Raw(), extraBitNames))
	fmt.Fprintf(&out, "Weight: %d, Value: %d, Rent: %d, Min Level: %d\r\n",
		obj.TotalWeight(), obj.Cost, obj.RentPerDay(), obj.MinLevel())

	switch obj.Type {
	case ItemScroll, ItemPotion:
		fmt.Fprintf(&out, "This %s casts: ", SprintType(obj.Type, ItemTypeNames))
		// Values 1, 2 and 3 are up to three spells, and a scroll with none of
		// them prints the line with nothing after it.
		for _, value := range obj.Values[1:] {
			if value >= 1 {
				fmt.Fprintf(&out, " %s", SpellName(value))
			}
		}
		out.WriteString("\r\n")

	case ItemWand, ItemStaff:
		fmt.Fprintf(&out, "This %s casts:  %s\r\n",
			SprintType(obj.Type, ItemTypeNames), SpellName(obj.Values[3]))
		fmt.Fprintf(&out, "It has %d maximum charge%s and %d remaining.\r\n",
			obj.Values[1], plural(obj.Values[1]), obj.Values[2])

	case ItemWeapon:
		// The average is the die's mean times the number of dice, and the C
		// computes `(size + 1) / 2.0` — which is right for a die but reads
		// like an off-by-one until you remember a d6 averages 3.5.
		fmt.Fprintf(&out, "Damage Dice is '%dD%d'", obj.Values[1], obj.Values[2])
		fmt.Fprintf(&out, " for an average per-round damage of %.1f.\r\n",
			(float64(obj.Values[2]+1)/2.0)*float64(obj.Values[1]))

	case ItemArmor:
		fmt.Fprintf(&out, "AC-apply is %d\r\n", obj.Values[0])
	}

	var found bool
	for _, a := range obj.Affects {
		if a.Location == ApplyNone || a.Modifier == 0 {
			continue
		}
		if !found {
			out.WriteString("Can affect you as :\r\n")
			found = true
		}
		fmt.Fprintf(&out, "   Affects: %s By %d\r\n",
			SprintType(a.Location, applyTypeNames), a.Modifier)
	}
	return out.String()
}

// IdentifyCharacter is the report for a person, which is shorter and rather
// more revealing: it prints another player's exact hit points and every one
// of their abilities.
func IdentifyCharacter(victim *Character, now time.Time) string {
	if victim == nil || victim.Record == nil {
		return ""
	}
	rec := victim.Record

	var out strings.Builder
	fmt.Fprintf(&out, "Name: %s\r\n", victim.Name)

	if !victim.IsNPC() {
		age := AgeOf(rec, now)
		fmt.Fprintf(&out, "%s is %d years, %d months, %d days and %d hours old.\r\n",
			victim.Name, age.Year, age.Month, age.Day, age.Hours)
	}
	fmt.Fprintf(&out, "Height %d cm, Weight %d pounds\r\n", rec.Height, rec.Weight)
	fmt.Fprintf(&out, "Level: %d, Hits: %d, Mana: %d\r\n",
		rec.Level, rec.Points.Hit, rec.Points.Mana)
	fmt.Fprintf(&out, "AC: %d, Hitroll: %d, Damroll: %d\r\n",
		ComputeArmorClass(rec, nil), rec.Points.HitRoll, rec.Points.DamRoll)
	fmt.Fprintf(&out, "Str: %d/%d, Int: %d, Wis: %d, Dex: %d, Con: %d, Cha: %d\r\n",
		rec.Abilities.Strength, rec.Abilities.StrengthPercentile,
		rec.Abilities.Intelligence, rec.Abilities.Wisdom,
		rec.Abilities.Dexterity, rec.Abilities.Constitution,
		rec.Abilities.Charisma)
	return out.String()
}

// RentPerDay is what leaving the object in a rent room costs.
func (o *Object) RentPerDay() int32 {
	if o == nil || o.Def == nil {
		return 0
	}
	return o.Def.RentPerDay
}

func plural(n int32) string {
	if n == 1 {
		return ""
	}
	return "s"
}
