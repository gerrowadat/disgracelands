// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// What wearing something does, ported from apply_ac, invalid_align and
// invalid_class (handler.c and class.c).
//
// Until now equipment was only a place to put objects: a suit of plate mail
// went on and nothing changed. Two separate mechanisms make it matter, and
// the C keeps them apart on purpose.
//
//   - An ITEM_ARMOR's value 0 is armour class, and equip_char subtracts it
//     from the wearer there and then. It is a permanent change to the
//     character's own armour figure, undone by unequip_char.
//   - The `A` lines — hitroll, damroll, an ability, a saving throw — are
//     applies, and affect_total recomputes them from scratch every time
//     anything changes.
//
// Mixing the two up is how a character ends up with armour class that drifts
// every time they change their shield.

// ArmorClassOf is apply_ac (handler.c:441): what a piece of armour is worth in the
// slot it is worn in.
//
// The multiplier belongs to the *slot*, not to the object, so the same value 0
// is worth three times as much on the body as on a wrist. The comments in the
// C call these percentages — 30%, 20%, 10% — which they were before somebody
// multiplied the whole scale by ten.
func ArmorClassOf(obj *Object, pos WearPosition) int32 {
	armor, ok := obj.ArmorValues()
	if !ok {
		return 0
	}
	switch pos {
	case WearBody:
		return 3 * armor.ACApply
	case WearHead, WearLegs:
		return 2 * armor.ACApply
	default:
		return armor.ACApply
	}
}

// Zaps reports whether an object refuses to be worn by this character,
// porting invalid_align and invalid_class (handler.c:488, class.c:130).
//
// The class test uses the IS_<CLASS> macros, which in this tree consult the
// remort vector — so an anti-thief object rejects anybody who has *ever* been
// a thief, not merely somebody who is one now. That is the local rewrite
// working exactly as intended and it is worth knowing before somebody
// remorts out of a class to wear their old guild's forbidden armour.
//
// objsave.c:210 has a second copy of invalid_class that tests GET_CLASS
// instead, so rent applies the stricter rule and wearing applies the looser
// one. Only this one is reachable yet.
func Zaps(rec *PlayerRecord, obj *Object) bool {
	if rec == nil || obj == nil {
		return false
	}

	switch {
	case obj.ExtraFlags.Has(ItemAntiEvil) && IsEvil(rec):
		return true
	case obj.ExtraFlags.Has(ItemAntiGood) && IsGood(rec):
		return true
	case obj.ExtraFlags.Has(ItemAntiNeutral) && IsNeutral(rec):
		return true
	case obj.ExtraFlags.Has(ItemAntiMagicUser) && IsMagicUser(rec):
		return true
	case obj.ExtraFlags.Has(ItemAntiCleric) && IsCleric(rec):
		return true
	case obj.ExtraFlags.Has(ItemAntiWarrior) && IsWarrior(rec):
		return true
	case obj.ExtraFlags.Has(ItemAntiThief) && IsThief(rec):
		return true
	}
	return false
}
