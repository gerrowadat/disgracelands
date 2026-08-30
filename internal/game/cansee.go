// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// Who can see whom, and what. The CAN_SEE family from utils.h:426-460.
//
// The C builds this out of six macros nested three deep, and taking them apart
// is worth doing once because the nesting hides two things. First, **holylight
// appears twice and means different things**: `LIGHT_OK` does not consult it
// at all, and `IMM_CAN_SEE` lets it bypass the entire test rather than answer
// any part of it — so a god with holylight sees through darkness, invisibility
// and hiding alike, in one step. Second, **the invisibility-level test is
// outside `IMM_CAN_SEE`**, so it is the one thing holylight does *not* defeat:
// a god cannot see an equal-or-higher god who is `invis`, whatever else they
// have on.
//
// A note on the room used. `LIGHT_OK` asks about the *subject's* room, never
// the object's, and CAN_SEE is asked about people in other rooms — by `where`,
// by the world-wide spell targets. So standing in the dark stops you seeing
// somebody standing in daylight two rooms away. That is the C's, and it is
// what makes `where` useless to a mortal in an unlit cave.

// LightOk ports LIGHT_OK (utils.h:426): can this character see anything at all,
// where they are?
//
// Note it takes infravision alone, *not* CanSeeInDark — holylight is handled a
// level up. See docs/weirdnumbers.md; the two questions are not the same and
// the difference only shows for a blind god.
func (l *Live) LightOk(sub *Character) bool {
	if sub == nil {
		return false
	}
	if sub.HasAffect(AffectBlind) {
		return false
	}
	return !l.RoomIsDark(sub.Room) || sub.HasAffect(AffectInfravision)
}

// InvisOk ports INVIS_OK (utils.h:429): is the object's concealment defeated?
//
// Two pairs, and each is "not concealed, or I have the counter to it":
// invisibility against detect-invisible, hiding against sense-life. Sneaking
// is not here — AFF_SNEAK conceals *movement*, not the person, so a sneaking
// character standing in front of you is plainly visible.
func InvisOk(sub, obj *Character) bool {
	if obj.HasAffect(AffectInvisible) && !sub.HasAffect(AffectDetectInvis) {
		return false
	}
	if obj.HasAffect(AffectHide) && !sub.HasAffect(AffectSenseLife) {
		return false
	}
	return true
}

// hasHolylight is the `!IS_NPC(sub) && PRF_FLAGGED(sub, PRF_HOLYLIGHT)` that
// appears in three macros. The NPC guard is not decoration: player_specials is
// not allocated for a mobile, so the C would be dereferencing a null pointer
// without it.
func hasHolylight(ch *Character) bool {
	return ch != nil && !ch.IsNPC() && ch.Record != nil &&
		ch.Record.Preferences.Has(PrefHolylight)
}

// CanSee ports CAN_SEE (utils.h:441): can sub see obj?
//
// The three tests, in the C's order:
//
//	SELF          -- you always see yourself, whatever you are affected by
//	invis level   -- the only test holylight does not defeat
//	IMM_CAN_SEE   -- light and concealment, or holylight in one step
func (l *Live) CanSee(sub, obj *Character) bool {
	if sub == nil || obj == nil {
		return false
	}
	if sub == obj {
		return true
	}

	// GET_INVIS_LEV is a player_specials field, so a mobile's is read as 0 —
	// the C writes the `IS_NPC(obj) ? 0 :` out longhand for exactly that
	// reason. A mobile can never be invis-levelled.
	if !obj.IsNPC() && obj.Record != nil && sub.RealLevel() < obj.Record.InvisLevel {
		return false
	}

	// IMM_CAN_SEE: the mortal test, or holylight instead of it.
	if l.LightOk(sub) && InvisOk(sub, obj) {
		return true
	}
	return hasHolylight(sub)
}

// InvisOkObj ports INVIS_OK_OBJ (utils.h:448). Objects have no equivalent of
// hiding — only ITEM_INVISIBLE — so this is one test rather than two.
func InvisOkObj(sub *Character, obj *Object) bool {
	return !obj.ExtraFlags.Has(ItemInvisible) || sub.HasAffect(AffectDetectInvis)
}

// CanSeeObj ports CAN_SEE_OBJ (utils.h:459).
//
// The part worth keeping in mind is CAN_SEE_OBJ_CARRIER (utils.h:452):
// **an object held by somebody you cannot see is itself invisible.** That is
// what stops an invisible thief's sword being listed in the room, and it means
// object visibility is not a property of the object alone.
func (l *Live) CanSeeObj(sub *Character, obj *Object) bool {
	if sub == nil || obj == nil {
		return false
	}
	if l.LightOk(sub) && InvisOkObj(sub, obj) && l.canSeeCarrier(sub, obj) {
		return true
	}
	return hasHolylight(sub)
}

// canSeeCarrier ports CAN_SEE_OBJ_CARRIER. An object lying in a room or inside
// a container has no holder and passes.
//
// The C tests `carried_by` and `worn_by` as two separate members; here both
// are placements with a Holder, and HolderOf answers for either — which is
// the same collapse, now made by the type rather than by an enum test.
func (l *Live) canSeeCarrier(sub *Character, obj *Object) bool {
	if holder := obj.HolderOf(); holder != nil {
		return l.CanSee(sub, holder)
	}
	return true
}

// Pers ports PERS (utils.h:470): what `who` is called when `to` looks at them.
//
// "someone" is the C's word and it is deliberately the same for everybody, so
// that two invisible people are indistinguishable rather than merely unnamed.
func (l *Live) Pers(who, to *Character) string {
	if who == nil {
		return ""
	}
	if to != nil && !l.CanSee(to, who) {
		return "someone"
	}
	return who.Name
}

// Objs ports OBJS (utils.h:472).
func (l *Live) Objs(obj *Object, to *Character) string {
	if obj == nil {
		return ""
	}
	if to != nil && !l.CanSeeObj(to, obj) {
		return "something"
	}
	return obj.Name()
}
