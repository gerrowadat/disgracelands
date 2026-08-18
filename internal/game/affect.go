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

// APPLY_* locations, from structs.h:392.
const (
	ApplyNone         int32 = 0
	ApplyStr          int32 = 1
	ApplyDex          int32 = 2
	ApplyInt          int32 = 3
	ApplyWis          int32 = 4
	ApplyCon          int32 = 5
	ApplyCha          int32 = 6
	ApplyClass        int32 = 7 // reserved in the C too
	ApplyLevel        int32 = 8 // reserved in the C too
	ApplyAge          int32 = 9
	ApplyCharWeight   int32 = 10
	ApplyCharHeight   int32 = 11
	ApplyMana         int32 = 12
	ApplyHit          int32 = 13
	ApplyMove         int32 = 14
	ApplyGold         int32 = 15 // reserved in the C too
	ApplyExp          int32 = 16 // reserved in the C too
	ApplyAC           int32 = 17
	ApplyHitRoll      int32 = 18
	ApplyDamRoll      int32 = 19
	ApplySaveParalyse int32 = 20
	ApplySaveRod      int32 = 21
	ApplySavePetrify  int32 = 22
	ApplySaveBreath   int32 = 23
	ApplySaveSpell    int32 = 24
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
// from their real ones plus every affect, porting affect_total.
//
// Equipment is not in here yet — the C removes and reapplies worn objects'
// affects around the same reset, and object affects arrive with the rest of
// the equipment rules.
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

	for _, a := range rec.Affects {
		flags = flags.Set(a.Bits)
		applyModifier(rec, a.Location, a.Modifier)
	}

	rec.AffectFlags = flags

	// The C clamps abilities to 25 after totalling, since several affects
	// stack and nothing else stops them.
	clamp := func(v *int32) {
		*v = max(0, min(25, *v))
	}
	clamp(&rec.Abilities.Strength)
	clamp(&rec.Abilities.Intelligence)
	clamp(&rec.Abilities.Wisdom)
	clamp(&rec.Abilities.Dexterity)
	clamp(&rec.Abilities.Constitution)
	clamp(&rec.Abilities.Charisma)
}

// applyModifier adds one affect's modifier to whatever it names, porting the
// switch in affect_modify.
func applyModifier(rec *PlayerRecord, location, modifier int32) {
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

// SnapshotReal records the current values as the unaffected ones.
//
// Called when a character is created and when one is loaded from disk. The
// C's char_file_u stores real_abils, not aff_abils — the saved numbers are
// what the character rolled, with no spell on them — so treating the loaded
// record as the real values and then applying its saved affects is exactly
// what the C's load does.
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
