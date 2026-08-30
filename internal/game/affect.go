// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// Affects: the timed modifiers a spell leaves on a character, ported from
// handler.c's affect_to_char/affect_from_char/affect_join/affect_total and
// limits.c's affect_update.
//
// The C's arrangement is worth understanding before changing anything here.
// A character's abilities exist twice — real_abils, what they rolled, and
// aff_abils, what they currently have. affect_total recomputes the second
// from the first by removing every affect, resetting to real, and applying
// them all again. That is not an optimisation problem, it is the only way to
// get the arithmetic right when affects can be added and removed in any
// order.

// Apply is where an affect's modifier lands — an APPLY_* location from
// structs.h:392. The numbers are in every player record and every object
// file, so they are the format as much as an enumeration.
//
// The zero value *is* a member, and a meaningful one: ApplyNone is how the
// C says "this affect slot modifies nothing", which is what makes an empty
// slot in a two-element array distinguishable from one that changes
// strength by zero.
type Apply int

// Number is the location's stored number, for the file formats and
// apply_types[]. The narrowing point, as Class.Number is.
func (a Apply) Number() int32 { return int32(a) } //nolint:gosec // twenty-five locations; the format's width

// APPLY_* locations, from structs.h:392.
const (
	ApplyNone         Apply = 0
	ApplyStr          Apply = 1
	ApplyDex          Apply = 2
	ApplyInt          Apply = 3
	ApplyWis          Apply = 4
	ApplyCon          Apply = 5
	ApplyCha          Apply = 6
	ApplyClass        Apply = 7 // reserved in the C too
	ApplyLevel        Apply = 8 // reserved in the C too
	ApplyAge          Apply = 9
	ApplyCharWeight   Apply = 10
	ApplyCharHeight   Apply = 11
	ApplyMana         Apply = 12
	ApplyHit          Apply = 13
	ApplyMove         Apply = 14
	ApplyGold         Apply = 15 // reserved in the C too
	ApplyExp          Apply = 16 // reserved in the C too
	ApplyAC           Apply = 17
	ApplyHitRoll      Apply = 18
	ApplyDamRoll      Apply = 19
	ApplySaveParalyse Apply = 20
	ApplySaveRod      Apply = 21
	ApplySavePetrify  Apply = 22
	ApplySaveBreath   Apply = 23
	ApplySaveSpell    Apply = 24
)

// MaxSpellAffects is MAX_SPELL_AFFECTS: how many separate modifiers one spell
// may apply. Bless is two, blindness is two, and nothing in the stock game
// needs more.
const MaxSpellAffects = 2

// AffectedBySpell reports whether a character is already under a spell.
func AffectedBySpell(rec *PlayerRecord, spell int32) bool {
	for _, a := range rec.Affects {
		if a.Type == spell {
			return true
		}
	}
	return false
}

// AddAffect puts an affect on a character, porting affect_to_char.
//
// It does not apply the modifier itself: RecomputeAffects does, from the
// whole list. Adding one and adjusting the total separately is how the C gets
// into trouble when an affect is removed in a different order from the one it
// was added in.
func AddAffect(rec *PlayerRecord, a Affect) {
	rec.Affects = append(rec.Affects, a)
	RecomputeAffects(rec)
}

// RemoveAllAffects strips every affect off a character, porting the local
// spell_dispel_magic (spells.c:85, between `<DoC>` markers).
//
// No saving throw, no exceptions, and it does not care who cast what: the
// C walks the whole list calling affect_remove. Cast on yourself it takes
// your own blessings with it.
func RemoveAllAffects(rec *PlayerRecord) {
	if rec == nil || len(rec.Affects) == 0 {
		return
	}
	rec.Affects = nil
	RecomputeAffects(rec)
}

// RemoveAffectsOf takes off every affect a spell put on, porting
// affect_from_char.
func RemoveAffectsOf(rec *PlayerRecord, spell int32) bool {
	kept := rec.Affects[:0]
	var removed bool
	for _, a := range rec.Affects {
		if a.Type == spell {
			removed = true
			continue
		}
		kept = append(kept, a)
	}
	rec.Affects = kept

	if removed {
		RecomputeAffects(rec)
	}
	return removed
}

// JoinAffect adds an affect, merging it with an existing one from the same
// spell, porting affect_join.
//
// accumDuration adds the durations together, accumModifier the modifiers.
// Neither is the common case: most spells simply refuse to stack, which
// mag_affects checks before calling this at all.
func JoinAffect(rec *PlayerRecord, a Affect, accumDuration, accumModifier bool) {
	for i := range rec.Affects {
		existing := &rec.Affects[i]
		if existing.Type != a.Type || existing.Location != a.Location {
			continue
		}

		if accumDuration {
			a.Duration += existing.Duration
		}
		if accumModifier {
			a.Modifier += existing.Modifier
		}

		// Replace it rather than adding a second: the C removes the old and
		// adds the merged one.
		rec.Affects = append(rec.Affects[:i], rec.Affects[i+1:]...)
		AddAffect(rec, a)
		return
	}

	AddAffect(rec, a)
}

// RecomputeAffects rebuilds a character's current abilities and modifiers
// from their real ones plus everything worn and every affect, porting
// affect_total.
//
// The C does this by walking the equipment and the affect list *twice* —
// once subtracting every modifier and once adding it back — because it has
// nowhere to keep the unaffected figures. That works only as long as nothing
// changes between the two passes, which is why a shield swapped while
// blessed used to drift. Here the real values are kept and the totals are
// rebuilt from them, which cannot drift.
//
// Equipment comes from rec.Worn, which points at the character's own array.
// A record with no character behind it — one being loaded, or a test — has
// nil there and is totalled without equipment.
func RecomputeAffects(rec *PlayerRecord) {
	// Start from what they rolled.
	rec.Abilities = rec.RealAbilities

	rec.Points.Armor = rec.RealArmor
	rec.Points.HitRoll = rec.RealHitRoll
	rec.Points.DamRoll = rec.RealDamRoll
	rec.SavingThrows = rec.RealSavingThrows
	rec.Points.MaxHit = rec.RealMaxHit
	rec.Points.MaxMana = rec.RealMaxMana
	rec.Points.MaxMove = rec.RealMaxMove

	flags := rec.BaseAffectFlags

	// Equipment first, then spells — the order the C applies them in, and it
	// matters only because of the clamping below.
	if rec.Worn != nil {
		for _, obj := range rec.Worn {
			if obj == nil {
				continue
			}
			flags = flags.Union(obj.PermAffect)
			for _, a := range obj.Affects {
				applyModifier(rec, a.Location, a.Modifier)
			}
		}
	}

	for _, a := range rec.Affects {
		flags = flags.Union(a.Bits)
		applyModifier(rec, a.Location, a.Modifier)
	}

	rec.AffectFlags = flags

	// The ceiling is 25 for a mobile and *18* for a player, which is the
	// wart: an immortal rolled with 25s across the board loses them the first
	// time anything recomputes. Strength is the exception — anything above 18
	// is converted into the percentile rather than thrown away. See
	// docs/weirdnumbers.md.
	ceiling := int32(18)
	if rec.Mobile {
		ceiling = 25
	}
	clamp := func(v *int32) {
		*v = max(0, min(ceiling, *v))
	}
	clamp(&rec.Abilities.Intelligence)
	clamp(&rec.Abilities.Wisdom)
	clamp(&rec.Abilities.Dexterity)
	clamp(&rec.Abilities.Constitution)
	clamp(&rec.Abilities.Charisma)

	rec.Abilities.Strength = max(0, rec.Abilities.Strength)
	if rec.Mobile {
		rec.Abilities.Strength = min(ceiling, rec.Abilities.Strength)
		return
	}
	if rec.Abilities.Strength > 18 {
		rec.Abilities.StrengthPercentile = min(100,
			rec.Abilities.StrengthPercentile+(rec.Abilities.Strength-18)*10)
		rec.Abilities.Strength = 18
	}
}

// applyModifier adds one affect's modifier to whatever it names, porting the
// switch in affect_modify.
func applyModifier(rec *PlayerRecord, location Apply, modifier int32) {
	switch location {
	case ApplyStr:
		rec.Abilities.Strength += modifier
	case ApplyDex:
		rec.Abilities.Dexterity += modifier
	case ApplyInt:
		rec.Abilities.Intelligence += modifier
	case ApplyWis:
		rec.Abilities.Wisdom += modifier
	case ApplyCon:
		rec.Abilities.Constitution += modifier
	case ApplyCha:
		rec.Abilities.Charisma += modifier
	case ApplyAge:
		// The C adjusts player.time.birth, which moves the birthday rather
		// than the age. Not ported: nothing in the stock game uses it, and
		// silently rewriting a character's birth date is worse than doing
		// nothing.
	case ApplyCharWeight:
		rec.Weight += modifier
	case ApplyCharHeight:
		rec.Height += modifier
	case ApplyMana:
		rec.Points.MaxMana += modifier
	case ApplyHit:
		rec.Points.MaxHit += modifier
	case ApplyMove:
		rec.Points.MaxMove += modifier
	case ApplyAC:
		rec.Points.Armor += modifier
	case ApplyHitRoll:
		rec.Points.HitRoll += modifier
	case ApplyDamRoll:
		rec.Points.DamRoll += modifier
	case ApplySaveParalyse:
		rec.SavingThrows[0] += modifier
	case ApplySaveRod:
		rec.SavingThrows[1] += modifier
	case ApplySavePetrify:
		rec.SavingThrows[2] += modifier
	case ApplySaveBreath:
		rec.SavingThrows[3] += modifier
	case ApplySaveSpell:
		rec.SavingThrows[4] += modifier
	}
}

// ExpiredAffect is one affect that ran out, so the caller can say so.
type ExpiredAffect struct {
	Spell int32
	// Message is the spell's wear-off text, or "".
	Message string
}

// AgeAffects counts down every affect and removes the ones that expire,
// porting affect_update (limits.c).
//
// A duration of -1 is permanent — the C uses it for affects that come from
// somewhere other than a spell — and is left alone.
func AgeAffects(rec *PlayerRecord) []ExpiredAffect {
	var expired []ExpiredAffect
	kept := rec.Affects[:0]

	for _, a := range rec.Affects {
		switch {
		case a.Duration >= 1:
			a.Duration--
			kept = append(kept, a)
		case a.Duration == -1:
			// Permanent.
			kept = append(kept, a)
		default:
			// The wear-off message is only sent for the last affect of a
			// spell, which is why bless says "you feel less righteous" once
			// and not twice.
			message := ""
			if info, ok := Spell(a.Type); ok && info.WearOff != "" &&
				countAffects(kept, a.Type) == 0 && countAffects(rec.Affects, a.Type) == 1 {
				message = info.WearOff
			}
			expired = append(expired, ExpiredAffect{Spell: a.Type, Message: message})
		}
	}

	rec.Affects = kept
	if len(expired) > 0 {
		RecomputeAffects(rec)
	}
	return expired
}

func countAffects(list []Affect, spell int32) int {
	n := 0
	for _, a := range list {
		if a.Type == spell {
			n++
		}
	}
	return n
}

// BaseRecord returns rec with every figure RecomputeAffects derives replaced
// by the unaffected value it derives from: the port's char_to_store
// (db.c:2292), which is what a live character must be written through.
//
// The C strips a character before writing them — unequip everything, then
// `while (ch->affected) affect_remove(...)`, then `ch->aff_abils =
// ch->real_abils` — and puts it all back afterwards, with the comment that
// says exactly why: "remove the affections so that the raw values are
// stored; otherwise the effects are doubled when the char logs back in"
// (db.c:2319-2324). Saving a blessed character's blessed numbers makes them
// the base the next login blesses again.
//
// Here the raw values are already kept beside the derived ones, so the same
// result is a copy with the derived fields overwritten. Nothing is mutated
// and nothing is put back, which is also what makes this safe to call on a
// snapshot taken off the world goroutine.
//
// Armour class, hitroll and damroll are not saved at all but reset, matching
// the C's own `st->points.armor = 100; st->points.hitroll = 0;
// st->points.damroll = 0` (db.c:2354-2356) — and store_to_char forces the
// same three on the way back in (db.c:2260-2262), so the C never round-trips
// them either. RealArmor cannot be written in their place: equip_char adjusts
// it directly rather than through an affect (docs/deviations.md, "Affects are
// recomputed from stored real values"), so a character saved while wearing
// armour would have the armour folded into their base and folded in again on
// the next login.
//
// Only players reach this. char_to_store is guarded by save_char's
// `if (IS_NPC(ch)) return` (db.c:2206), as Server.Save is.
func BaseRecord(rec PlayerRecord) PlayerRecord {
	rec.Abilities = rec.RealAbilities
	rec.Points.MaxHit = rec.RealMaxHit
	rec.Points.MaxMana = rec.RealMaxMana
	rec.Points.MaxMove = rec.RealMaxMove
	rec.SavingThrows = rec.RealSavingThrows
	rec.AffectFlags = rec.BaseAffectFlags

	rec.Points.Armor = baseArmor
	rec.Points.HitRoll = 0
	rec.Points.DamRoll = 0
	return rec
}

// SnapshotReal records the current values as the unaffected ones.
//
// Called when a character is created and at the end of every player store's
// Load. The C's char_file_u stores real_abils, not aff_abils — the saved
// numbers are what the character rolled, with no spell on them — so treating
// the loaded record as the real values and then applying its saved affects is
// exactly what the C's load does (store_to_char's `ch->real_abils =
// ch->aff_abils = st->abilities`, db.c:2245-2246).
//
// It must be called exactly once per record, before anything recomputes.
// Never is the worse failure and was a real one: a record loaded with the
// Real fields left at zero holds correct figures right up until the first
// RecomputeAffects — a spell landing, a shield going on — which resets the
// live values from a base of nothing and leaves the character with no hit
// points, no mana, no movement and no abilities. Twice is the other
// direction: called after RecomputeAffects it folds the affects into the base
// they were applied to, which is the doubling BaseRecord's comment describes.
func SnapshotReal(rec *PlayerRecord) {
	rec.RealAbilities = rec.Abilities
	rec.RealArmor = rec.Points.Armor
	rec.RealHitRoll = rec.Points.HitRoll
	rec.RealDamRoll = rec.Points.DamRoll
	rec.RealMaxHit = rec.Points.MaxHit
	rec.RealMaxMana = rec.Points.MaxMana
	rec.RealMaxMove = rec.Points.MaxMove
	rec.RealSavingThrows = rec.SavingThrows
	rec.BaseAffectFlags = rec.AffectFlags
}
