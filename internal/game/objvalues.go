// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// Object values, by item type.
//
// An object's four value slots mean something different for every item type
// and the C reads them through GET_OBJ_VAL(obj, N) at every use, so nothing
// anywhere states what slot 1 of a wand is. This file states it once, which
// is step 3 of docs/proposals/idiomatic-go.md.
//
// container.go has done this for containers since the port, and §3.3 calls
// it "the exception that shows the rule". This is that exception applied to
// the rest.
//
// # Where the layout comes from
//
// Not from the C's source, which contains no such specification as data:
// from OasisOLC's per-type oedit_disp_valN_menu switch
// (reference/moderncserver/src/oedit.c:294-441), which is the closest thing
// to one that exists. internal/persist/world/yaml/values.go transcribed it
// first, for the on-disk typed form, and §4.3 is explicit that a second
// game-side transcription would be a second authority disagreeing with the
// first. So this is the only transcription now: values.go is written in
// terms of these types rather than repeating the slot numbers.
//
// # What this file does not decide
//
// Whether the *yaml* may use its typed form is values.go's question and
// stays there. That rule -- emit the typed form only when every slot the
// type does not use is genuinely zero, so a corpse's -1 fourth value falls
// back to raw rather than being rounded away -- is about round-tripping a
// builder's file unchanged. The game has no such scruple: it reads a
// light's value 2 whatever junk is in value 0, exactly as the C does. The
// accessors below therefore answer for any object of the right *type* and
// never report "this does not fit".

// DamageType is a weapon's kind of blow: the 0-14 offset stored in a
// weapon's fourth value, named by yamlAttackTypeNames and by attackVerbs.
//
// It is *not* the number FightMessages.Pick, DamageMessage and
// AttackTypeName take. Those want the TypeHit-scaled form (300-314), which
// is a union of this domain with SpellID -- misc/messages holds records for
// both. AttackType() promotes one into the other, and that promotion is the
// only place the two scales meet.
//
// The union itself is still an int32, because it is a union and not an
// enumeration; §4.5 is where it gets a type. §3.2's table of nine
// enumerations missed both of these, which is what step 3 needing a field
// type for a weapon's damage turned up.
type DamageType int

// Number is the damage type's stored number: what a weapon's fourth value
// holds and what yamlAttackTypeNames is indexed by.
func (d DamageType) Number() int32 { return int32(d) } //nolint:gosec // fifteen damage types; the format's width

// AttackType promotes a weapon's own damage type to the TypeHit-scaled
// number the message tables are keyed by (fight.c:1035-1042's w_type).
func (d DamageType) AttackType() int32 { return TypeHit + d.Number() }

// The value slots each item type uses, as oedit.c's menus have them.
//
// Named per type rather than shared, because the same index means unrelated
// things: value 2 is a light's hours, a container's key, a drink's liquid
// and a wand's charges remaining.
const (
	// A weapon: value 0 is a hitroll modifier oedit's own comment says
	// "doesn't seem to be used", values 1 and 2 are the damage dice as
	// count and size, and value 3 is the DamageType.
	weaponDiceCount = 1
	weaponDiceSize  = 2
	weaponDamage    = 3

	// Armor: value 0 is the AC apply, the other three unused.
	armorACApply = 0

	// A light source: value 2 is hours remaining. Values 0 and 1 are
	// unused ("jump to 2", says oedit's comment) and so is value 3, whose
	// menu has no LIGHT case at all.
	lightHours = 2

	// A wand or a staff: level, maximum charges, charges left, and the
	// spell. A scroll or a potion shares the level slot but carries three
	// spells in 1..3 instead, which is why it has no grouped accessor.
	chargesLevel     = 0
	chargesMax       = 1
	chargesRemaining = 2
	chargesSpell     = 3

	// A drink container or a fountain: capacity, how much is in it, which
	// liquid, and whether it is poisoned. All four are read.
	drinkCapacity = 0
	drinkFilled   = 1
	drinkLiquid   = 2
	drinkPoisoned = 3

	// Food: value 0 is how much hunger it settles, value 3 whether it is
	// poisoned. Values 1 and 2 are unused.
	foodFilling  = 0
	foodPoisoned = 3

	// Money: value 0 is the number of coins.
	moneyCoins = 0
)

// WeaponValues is a weapon's damage.
type WeaponValues struct {
	// Dice is the damage roll. Its Bonus is always zero: a weapon's value
	// slots have no bonus term, unlike a mobile's hit and damage dice.
	Dice Dice
	// Damage is the kind of blow, which picks the verb in a fight message.
	Damage DamageType
}

// ArmorValues is what armor takes off the wearer's armour class.
type ArmorValues struct {
	// ACApply is the C's own name and its own sign convention: a positive
	// number here *improves* AC, because apply_ac multiplies it by -1
	// (handler.c, and see equip.go for the by-position multiplier).
	ACApply int32
}

// LightValues is how long a light has left.
type LightValues struct {
	// Hours is what remains. Zero is a burnt-out light and **-1 is an
	// eternal one**: LitLight tests for non-zero rather than positive, and
	// the burnout timer is guarded on > 0. See light.go.
	Hours int32
}

// ChargesValues is a wand or a staff.
type ChargesValues struct {
	Level     int32
	Max       int32
	Remaining int32
	Spell     SpellID
}

// DrinkValues is a drink container or a fountain.
type DrinkValues struct {
	Capacity int32
	Filled   int32
	Liquid   Liquid
	// Poisoned is value 3, which the C reads as a truth value and nothing
	// else -- act.item.c's drink handler tests it with a bare if. The file
	// can hold any int and a builder's file does; that matters to the
	// writer, not here.
	Poisoned bool
}

// ContainerValues is a container's four slots as one value.
//
// The per-slot accessors in container.go predate this and stay: they are
// what the rules actually call, and Capacity()/ContainerFlags()/
// ContainerKey() read better at a use site than a struct field would.
// This exists for the writer, which wants all four at once. All six
// accessors are named <Thing>Values rather than <Thing> because Object
// already has a Container *field* -- the object this one is inside -- and
// a method cannot share a name with it.
type ContainerValues struct {
	Capacity int32
	Flags    ContainerFlagSet
	// Key is value 2 exactly as stored, which is *not* what ContainerKey()
	// answers. That accessor maps anything <= 0 to NoObject, because to the
	// rules "no key" is one thing; but 0 and -1 are two different bytes on
	// disk and the stock world contains both. Normalising here turned every
	// -1 into a 0 and failed the compatibility corpus on seventeen stock
	// objects, which is the difference between a rules accessor and a value
	// this struct exists to round-trip. §4.4 is where the sentinel itself
	// gets dealt with.
	Key ObjVnum
	// Corpse is value 3's corpse marker (1, not -1: make_corpse,
	// fight.c:319). It is the reason a container's typed yaml form has to
	// be able to refuse: see values.go.
	Corpse bool
}

// WeaponValues returns a weapon's values, and false for anything else.
func (o *Object) WeaponValues() (WeaponValues, bool) {
	if o == nil || o.Type != ItemWeapon {
		return WeaponValues{}, false
	}
	return WeaponValues{
		Dice:   Dice{Number: o.Values[weaponDiceCount], Size: o.Values[weaponDiceSize]},
		Damage: DamageType(o.Values[weaponDamage]),
	}, true
}

// ArmorValues returns armor's values, and false for anything else.
func (o *Object) ArmorValues() (ArmorValues, bool) {
	if o == nil || o.Type != ItemArmor {
		return ArmorValues{}, false
	}
	return ArmorValues{ACApply: o.Values[armorACApply]}, true
}

// LightValues returns a light source's values, and false for anything else.
func (o *Object) LightValues() (LightValues, bool) {
	if o == nil || o.Type != ItemLight {
		return LightValues{}, false
	}
	return LightValues{Hours: o.Values[lightHours]}, true
}

// ChargesValues returns a wand or staff's values, and false for anything else --
// including a scroll or a potion, which share the level slot but carry
// three spells rather than a charge count.
func (o *Object) ChargesValues() (ChargesValues, bool) {
	if o == nil || (o.Type != ItemWand && o.Type != ItemStaff) {
		return ChargesValues{}, false
	}
	return ChargesValues{
		Level:     o.Values[chargesLevel],
		Max:       o.Values[chargesMax],
		Remaining: o.Values[chargesRemaining],
		Spell:     SpellID(o.Values[chargesSpell]),
	}, true
}

// DrinkValues returns a drink container or fountain's values, and false for
// anything else.
func (o *Object) DrinkValues() (DrinkValues, bool) {
	if o == nil || (o.Type != ItemDrinkCon && o.Type != ItemFountain) {
		return DrinkValues{}, false
	}
	return DrinkValues{
		Capacity: o.Values[drinkCapacity],
		Filled:   o.Values[drinkFilled],
		Liquid:   Liquid(o.Values[drinkLiquid]),
		Poisoned: o.Values[drinkPoisoned] != 0,
	}, true
}

// ContainerValues returns a container's values, and false for anything else.
func (o *Object) ContainerValues() (ContainerValues, bool) {
	if !o.IsContainer() {
		return ContainerValues{}, false
	}
	return ContainerValues{
		Capacity: o.Capacity(),
		Flags:    o.ContainerFlags(),
		Key:      ObjVnum(o.Values[containerKeyValue]),
		Corpse:   o.Values[containerCorpseValue] == corpseIdentifier,
	}, true
}

// ValuesOfWeapon, ValuesOfArmor and the rest are the accessors' inverses:
// the four slots a typed form encodes to. They take the values rather than
// an object because the world reader builds a prototype's slots before
// there is an object, and because the writer works from a definition.
//
// Slots the type does not use are zero. That is right for building a new
// object; it is *not* a claim that an existing object's unused slots are
// zero, which is exactly the distinction values.go's unusedNonzero check
// exists to make.

// ValuesOfWeapon encodes a weapon's values.
func ValuesOfWeapon(v WeaponValues) (values [NumObjValues]int32) {
	values[weaponDiceCount] = v.Dice.Number
	values[weaponDiceSize] = v.Dice.Size
	values[weaponDamage] = v.Damage.Number()
	return values
}

// ValuesOfArmor encodes armor's values.
func ValuesOfArmor(v ArmorValues) (values [NumObjValues]int32) {
	values[armorACApply] = v.ACApply
	return values
}

// ValuesOfLight encodes a light source's values.
func ValuesOfLight(v LightValues) (values [NumObjValues]int32) {
	values[lightHours] = v.Hours
	return values
}

// ValuesOfCharges encodes a wand or staff's values.
func ValuesOfCharges(v ChargesValues) (values [NumObjValues]int32) {
	values[chargesLevel] = v.Level
	values[chargesMax] = v.Max
	values[chargesRemaining] = v.Remaining
	values[chargesSpell] = v.Spell.Number()
	return values
}

// ValuesOfDrink encodes a drink container's values.
func ValuesOfDrink(v DrinkValues) (values [NumObjValues]int32) {
	values[drinkCapacity] = v.Capacity
	values[drinkFilled] = v.Filled
	values[drinkLiquid] = v.Liquid.Number()
	if v.Poisoned {
		values[drinkPoisoned] = 1
	}
	return values
}

// ValuesOfContainer encodes a container's values.
func ValuesOfContainer(v ContainerValues) (values [NumObjValues]int32) {
	values[containerCapacity] = v.Capacity
	values[containerFlagsValue] = int32(v.Flags.Raw()) //nolint:gosec // four bits
	values[containerKeyValue] = int32(v.Key)
	if v.Corpse {
		values[containerCorpseValue] = corpseIdentifier
	}
	return values
}
