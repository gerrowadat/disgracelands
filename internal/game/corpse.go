// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "fmt"

// Corpses, ported from make_corpse (fight.c) and the object half of
// point_update (limits.c:457).
//
// The corpse timers themselves (config.c:76, mud hours) live in
// GameTuning.NPCCorpseTime/PlayerCorpseTime (tuning.go) — a runtime setting,
// not a constant, since docs/deviations.md's "the archive wins" fidelity
// default was reopened for this field by name.

// Corpses carry ItemNoDonate (object.go) so that nobody can drop one into the
// donation room and have it teleport somewhere public with the contents still
// in it.

// corpseIdentifier is what make_corpse puts in value 3 to mark a container as
// a corpse. Value 0 is the capacity, deliberately zero: you cannot put things
// *into* a corpse, only take them out.
const corpseIdentifier = 1

// IsCorpse reports whether an object is one, porting IS_CORPSE.
func IsCorpse(o *Object) bool {
	return o != nil && o.Type == ItemContainer && o.Values[containerCorpseValue] == corpseIdentifier
}

// MakeCorpse kills a character into a container, porting make_corpse.
//
// Everything they were carrying and wearing goes inside, along with their
// gold, and the corpse is left in the room. The character's inventory and
// equipment are empty afterwards — that is the point, and it is why this is
// one function rather than a sequence a caller could half-finish.
func (l *Live) MakeCorpse(c *Character) *Object {
	corpse := l.NewBareObject()

	corpse.Keywords = "corpse"
	corpse.ShortDesc = fmt.Sprintf("the corpse of %s", c.Name)
	corpse.Description = fmt.Sprintf("The corpse of %s is lying here.", c.Name)

	corpse.Type = ItemContainer
	corpse.WearFlags = NewSet(ItemWearTake)
	corpse.ExtraFlags = NewSet(ItemNoDonate)
	// Capacity zero: nothing more goes in. The corpse marker is value 3,
	// which OasisOLC never prompts for — see objvalues.go.
	corpse.Values = ValuesOfContainer(ContainerValues{Corpse: true})

	corpse.Weight = c.CarriedWeight()
	if c.Record != nil {
		corpse.Weight += c.Record.Weight
	}

	tuning := Tuning()
	corpse.Timer = tuning.PlayerCorpseTime
	if c.IsNPC() {
		corpse.Timer = tuning.NPCCorpseTime
	}

	// Inventory first, then equipment, which is the order the C does it and
	// therefore the order things appear inside.
	for _, o := range append([]*Object(nil), c.Carrying...) {
		l.ObjectToObject(o, corpse)
	}
	for pos := WearPosition(0); pos < NumWears; pos++ {
		if o := l.Unequip(c, pos); o != nil {
			l.ObjectToObject(o, corpse)
		}
	}

	if c.Record != nil && c.Record.Points.Gold > 0 {
		l.ObjectToObject(l.MakeMoney(c.Record.Points.Gold), corpse)
		c.Record.Points.Gold = 0
	}

	l.ObjectToRoom(corpse, c.Room)
	return corpse
}

// MakeMoney builds a pile of coins, porting create_money (handler.c).
func (l *Live) MakeMoney(amount int32) *Object {
	money := l.NewBareObject()
	money.Type = ItemMoney
	money.WearFlags = NewSet(ItemWearTake)
	money.SetCoins(amount)

	switch amount {
	case 1:
		money.Keywords = "coin gold"
		money.ShortDesc = "a gold coin"
		money.Description = "One miserable gold coin is lying here."
	default:
		// The C never says how many coins a pile is: it names it from
		// money_desc's table, so a hundred coins and a hundred and ninety
		// both look like "a small pile of gold coins" until you pick them up.
		money.Keywords = "coins gold"
		money.ShortDesc = MoneyDescription(amount)
		money.Description = capitaliseFirst(MoneyDescription(amount)) + " is lying here."
	}
	return money
}

// CarriedWeight is what a character is carrying and wearing, porting
// IS_CARRYING_W plus the equipment the C counts separately.
func (c *Character) CarriedWeight() int32 {
	if c == nil {
		return 0
	}
	var total int32
	for _, o := range c.Carrying {
		total += o.TotalWeight()
	}
	for _, o := range c.Equipment {
		total += o.TotalWeight()
	}
	return total
}

// DecayResult is one corpse that rotted away, so the caller can say so in the
// right place.
type DecayResult struct {
	// Corpse is the object that decayed.
	Corpse *Object
	// Room is where it was, or NoRoom if somebody was carrying it.
	Room RoomVnum
	// CarriedBy is who was holding it, or nil.
	CarriedBy *Character
}

// DecayObjects counts down every corpse's timer and destroys the ones that
// reach zero, porting the object half of point_update.
//
// Contents are not destroyed with the corpse: ExtractObject moves them to
// wherever the corpse was, which is what stops a decaying corpse taking
// somebody's equipment with it.
func (l *Live) DecayObjects() []DecayResult {
	var decayed []DecayResult

	for _, o := range l.Objects() {
		if !IsCorpse(o) || o.Timer <= 0 {
			continue
		}
		o.Timer--
		if o.Timer > 0 {
			continue
		}

		result := DecayResult{Corpse: o, Room: NoRoom}
		switch o.Location {
		case InRoom:
			result.Room = o.Room
		case CarriedBy, WornBy:
			result.CarriedBy = o.Holder
		}
		decayed = append(decayed, result)

		l.ExtractObject(o)
	}
	return decayed
}
