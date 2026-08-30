// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

func affectedCharacter() *PlayerRecord {
	rec := &PlayerRecord{
		Name: "Welmar", Class: ClassMagicUser, Level: 20,
		Abilities: Abilities{
			Strength: 12, Intelligence: 18, Wisdom: 12,
			Dexterity: 12, Constitution: 12, Charisma: 12,
		},
		Points: Points{
			Hit: 100, MaxHit: 100, Mana: 100, MaxMana: 100,
			Armor: 100, HitRoll: 0, DamRoll: 0,
		},
	}
	SnapshotReal(rec)
	return rec
}

// TestAnAffectAppliesAndReverses. The whole point of keeping real and
// affected values apart is that removing an affect restores exactly what was
// there, whatever order things happened in.
func TestAnAffectAppliesAndReverses(t *testing.T) {
	rec := affectedCharacter()

	AddAffect(rec, Affect{Type: SpellArmor, Location: ApplyAC, Modifier: -20, Duration: 24})
	if rec.Points.Armor != 80 {
		t.Errorf("armour is %d, want 80", rec.Points.Armor)
	}

	AddAffect(rec, Affect{Type: SpellBless, Location: ApplyHitRoll, Modifier: 2, Duration: 6})
	if rec.Points.HitRoll != 2 {
		t.Errorf("hitroll is %d, want 2", rec.Points.HitRoll)
	}

	// Removed out of order, and both restore correctly.
	RemoveAffectsOf(rec, SpellArmor)
	if rec.Points.Armor != 100 || rec.Points.HitRoll != 2 {
		t.Errorf("after removing armour: armour %d, hitroll %d; want 100 and 2",
			rec.Points.Armor, rec.Points.HitRoll)
	}

	RemoveAffectsOf(rec, SpellBless)
	if rec.Points.Armor != 100 || rec.Points.HitRoll != 0 {
		t.Errorf("after removing bless: armour %d, hitroll %d; want 100 and 0",
			rec.Points.Armor, rec.Points.HitRoll)
	}
	if len(rec.Affects) != 0 {
		t.Errorf("%d affects remain", len(rec.Affects))
	}
}

// TestFlagsComeAndGoWithTheirAffects.
func TestFlagsComeAndGoWithTheirAffects(t *testing.T) {
	rec := affectedCharacter()

	AddAffect(rec, Affect{Type: SpellSanctuary, Bits: AffectSanctuary, Duration: 4})
	if !rec.AffectFlags.Has(AffectSanctuary) {
		t.Error("sanctuary did not set its flag")
	}

	RemoveAffectsOf(rec, SpellSanctuary)
	if rec.AffectFlags.Has(AffectSanctuary) {
		t.Error("sanctuary's flag survived the affect")
	}
}

// TestAMobilesOwnFlagsSurviveASpellWearingOff. The C's comment explains the
// exploit: otherwise a player could sanctuary a sanctuary-carrying mobile and
// wait for it to fade, stripping it of what its file said it always had.
func TestAMobilesOwnFlagsSurviveASpellWearingOff(t *testing.T) {
	rec := &PlayerRecord{Level: 20, AffectFlags: AffectSanctuary}
	SnapshotReal(rec)

	// A spell cannot be laid on top of a prototype flag at all.
	spell := AffectSpell{Affects: []Affect{{Type: SpellSanctuary, Bits: AffectSanctuary, Duration: 4}}}
	if CanAffect(spell, rec, true, SpellSanctuary) {
		t.Error("a mobile that already has sanctuary could be sanctuaried")
	}

	// And if one somehow got on and wore off, the mobile keeps its own.
	AddAffect(rec, Affect{Type: SpellSanctuary, Bits: AffectSanctuary, Duration: 1})
	AgeAffects(rec)
	AgeAffects(rec)
	if !rec.AffectFlags.Has(AffectSanctuary) {
		t.Error("a mobile lost its prototype sanctuary when a spell wore off")
	}
}

// TestAffectsExpireOnTheTick, and the wear-off message is sent once.
func TestAffectsExpireOnTheTick(t *testing.T) {
	rec := affectedCharacter()

	AddAffect(rec, Affect{Type: SpellArmor, Location: ApplyAC, Modifier: -20, Duration: 2})

	if expired := AgeAffects(rec); len(expired) != 0 {
		t.Fatalf("armour expired early: %+v", expired)
	}
	if rec.Points.Armor != 80 {
		t.Errorf("armour is %d after one tick, want 80 still", rec.Points.Armor)
	}

	AgeAffects(rec)
	expired := AgeAffects(rec)
	if len(expired) != 1 || expired[0].Spell != SpellArmor {
		t.Fatalf("expired %+v, want armour", expired)
	}
	if expired[0].Message != "You feel less protected." {
		t.Errorf("the wear-off message was %q", expired[0].Message)
	}
	if rec.Points.Armor != 100 {
		t.Errorf("armour is %d after expiry, want 100", rec.Points.Armor)
	}
}

// TestAPermanentAffectDoesNotAge.
func TestAPermanentAffectDoesNotAge(t *testing.T) {
	rec := affectedCharacter()
	AddAffect(rec, Affect{Type: SpellSanctuary, Bits: AffectSanctuary, Duration: -1})

	for i := 0; i < 50; i++ {
		if expired := AgeAffects(rec); len(expired) != 0 {
			t.Fatalf("a permanent affect expired: %+v", expired)
		}
	}
	if !rec.AffectFlags.Has(AffectSanctuary) {
		t.Error("the permanent affect's flag went away")
	}
}

// TestAccumulatingDurations, which is what lets a caster top up armour rather
// than being told it had no effect.
func TestAccumulatingDurations(t *testing.T) {
	rec := affectedCharacter()
	a := Affect{Type: SpellArmor, Location: ApplyAC, Modifier: -20, Duration: 24}

	JoinAffect(rec, a, true, false)
	JoinAffect(rec, a, true, false)

	if len(rec.Affects) != 1 {
		t.Fatalf("%d affects, want one merged", len(rec.Affects))
	}
	if rec.Affects[0].Duration != 48 {
		t.Errorf("duration is %d, want 48", rec.Affects[0].Duration)
	}
	// The modifier did not accumulate, so the armour bonus is unchanged.
	if rec.Points.Armor != 80 {
		t.Errorf("armour is %d, want 80", rec.Points.Armor)
	}
}

// TestAccumulatingModifiers, which curse and strength do.
func TestAccumulatingModifiers(t *testing.T) {
	rec := affectedCharacter()
	a := Affect{Type: SpellStrength, Location: ApplyStr, Modifier: 2, Duration: 14}

	JoinAffect(rec, a, true, true)
	JoinAffect(rec, a, true, true)

	if rec.Abilities.Strength != 16 {
		t.Errorf("strength is %d, want 12 + 2 + 2", rec.Abilities.Strength)
	}
}

// TestAbilitiesAreClamped, which is what stops repeated strength spells
// running away.
//
// A player's ceiling is 18, not 25, and the overflow above it goes into the
// strength percentile rather than being thrown away — so twenty castings of
// strength leave a character at 18/100 rather than at 58. A mobile's ceiling
// is 25 and has no percentile at all.
func TestAbilitiesAreClamped(t *testing.T) {
	rec := affectedCharacter()
	for i := 0; i < 20; i++ {
		AddAffect(rec, Affect{Type: SpellStrength, Location: ApplyStr, Modifier: 2, Duration: 14})
	}
	if rec.Abilities.Strength != 18 || rec.Abilities.StrengthPercentile != 100 {
		t.Errorf("strength is %d/%d, want 18/100",
			rec.Abilities.Strength, rec.Abilities.StrengthPercentile)
	}

	// Intelligence has no percentile, so it simply stops at 18.
	rec = affectedCharacter()
	for i := 0; i < 20; i++ {
		AddAffect(rec, Affect{Type: SpellArmor, Location: ApplyInt, Modifier: 2, Duration: 14})
	}
	if rec.Abilities.Intelligence != 18 {
		t.Errorf("intelligence is %d, want 18", rec.Abilities.Intelligence)
	}

	// A mobile stops at 25.
	rec = affectedCharacter()
	rec.Mobile = true
	for i := 0; i < 20; i++ {
		AddAffect(rec, Affect{Type: SpellStrength, Location: ApplyStr, Modifier: 2, Duration: 14})
	}
	if rec.Abilities.Strength != 25 {
		t.Errorf("a mobile's strength is %d, want 25", rec.Abilities.Strength)
	}
}

// TestBlessAppliesBothOfItsAffects.
func TestBlessAppliesBothOfItsAffects(t *testing.T) {
	caster := affectedCharacter()
	victim := affectedCharacter()

	result := AffectsOfSpell(SpellBless, caster, victim, false, 0, 20, false, newRNG())
	if result.Refused || len(result.Affects) != 2 {
		t.Fatalf("bless gave %+v", result)
	}
	ApplyAffectSpell(result, victim)

	if victim.Points.HitRoll != 2 {
		t.Errorf("hitroll is %d, want 2", victim.Points.HitRoll)
	}
	if victim.SavingThrows[4] != -1 {
		t.Errorf("spell saving throw is %d, want -1", victim.SavingThrows[4])
	}
}

// TestSpellsThatRefuse, each with its own message or silence.
func TestSpellsThatRefuse(t *testing.T) {
	caster := affectedCharacter()
	victim := affectedCharacter()
	r := newRNG()

	// Blindness against a save says "You fail." to the caster.
	result := AffectsOfSpell(SpellBlindness, caster, victim, false, 0, 20, true, r)
	if !result.Refused || result.RefusalToCaster != "You fail.\r\n" {
		t.Errorf("blindness against a save gave %+v", result)
	}
	// And against a NOBLIND mobile, whatever the roll.
	result = AffectsOfSpell(SpellBlindness, caster, victim, true, MobNoBlind, 20, false, r)
	if !result.Refused {
		t.Error("blindness landed on a NOBLIND mobile")
	}

	// Curse and poison say NOEFFECT.
	for _, spell := range []int32{SpellCurse, SpellPoison} {
		result = AffectsOfSpell(spell, caster, victim, false, 0, 20, true, r)
		if !result.Refused || result.RefusalToCaster != NoEffect {
			t.Errorf("spell %d against a save gave %+v", spell, result)
		}
	}

	// Sleep refuses silently.
	result = AffectsOfSpell(SpellSleep, caster, victim, false, 0, 20, true, r)
	if !result.Refused || result.RefusalToCaster != "" {
		t.Errorf("sleep against a save gave %+v", result)
	}

	// Strength on somebody already at 18/00 refuses silently too.
	strong := affectedCharacter()
	strong.Abilities.StrengthPercentile = 100
	result = AffectsOfSpell(SpellStrength, caster, strong, false, 0, 20, false, r)
	if !result.Refused || result.RefusalToCaster != "" {
		t.Errorf("strength on an 18/00 character gave %+v", result)
	}
}

// TestStrengthGivesTwoPointsAboveLevelEighteen. The C writes this as
// `1 + (level > 18)` — a boolean promoted to an integer.
func TestStrengthGivesTwoPointsAboveLevelEighteen(t *testing.T) {
	caster := affectedCharacter()
	victim := affectedCharacter()
	r := newRNG()

	for _, tc := range []struct{ level, want int32 }{
		{1, 1}, {18, 1}, {19, 2}, {34, 2},
	} {
		result := AffectsOfSpell(SpellStrength, caster, victim, false, 0, tc.level, false, r)
		if result.Refused || len(result.Affects) != 1 {
			t.Fatalf("strength at level %d gave %+v", tc.level, result)
		}
		if got := result.Affects[0].Modifier; got != tc.want {
			t.Errorf("strength at level %d gives %d, want %d", tc.level, got, tc.want)
		}
	}
}

// TestChillTouchDurationDependsOnTheSave.
func TestChillTouchDurationDependsOnTheSave(t *testing.T) {
	caster := affectedCharacter()
	victim := affectedCharacter()
	r := newRNG()

	saved := AffectsOfSpell(SpellChillTouch, caster, victim, false, 0, 20, true, r)
	failed := AffectsOfSpell(SpellChillTouch, caster, victim, false, 0, 20, false, r)

	if saved.Affects[0].Duration != 1 {
		t.Errorf("a saved chill touch lasts %d, want 1", saved.Affects[0].Duration)
	}
	if failed.Affects[0].Duration != 4 {
		t.Errorf("an unsaved chill touch lasts %d, want 4", failed.Affects[0].Duration)
	}
}

// TestAlreadyAffectedAndNotAccumulating is refused.
func TestAlreadyAffectedAndNotAccumulating(t *testing.T) {
	rec := affectedCharacter()
	AddAffect(rec, Affect{Type: SpellHolySmite, Location: ApplyHitRoll, Modifier: 10, Duration: 2})

	spell := AffectSpell{
		Affects: []Affect{{Type: SpellHolySmite, Location: ApplyHitRoll, Modifier: 10, Duration: 2}},
	}
	if CanAffect(spell, rec, false, SpellHolySmite) {
		t.Error("a non-accumulating spell was allowed to stack")
	}

	// One that does accumulate is allowed.
	spell.AccumDuration = true
	if !CanAffect(spell, rec, false, SpellHolySmite) {
		t.Error("an accumulating spell was refused")
	}
}

// TestBaseRecordUndoesWhatAffectsDid: BaseRecord and SnapshotReal are the
// two halves of a save/load round trip, and the round trip has to be a fixed
// point. A record written through BaseRecord, read back and snapshotted, and
// recomputed must land on exactly the figures it started with — otherwise
// every logout moves the character, which is the doubling char_to_store's own
// comment warns about (db.c:2319-2324).
func TestBaseRecordUndoesWhatAffectsDid(t *testing.T) {
	rec := affectedCharacter()
	AddAffect(rec, Affect{Type: SpellArmor, Location: ApplyAC, Modifier: -20, Duration: 24})
	AddAffect(rec, Affect{Type: SpellBless, Location: ApplyHitRoll, Modifier: 2, Duration: 6})
	AddAffect(rec, Affect{Type: SpellStrength, Location: ApplyStr, Modifier: 1, Duration: 6})
	live := *rec

	// What a save writes.
	saved := BaseRecord(*rec)
	if saved.Points.MaxHit != rec.RealMaxHit || saved.Points.MaxMana != rec.RealMaxMana ||
		saved.Points.MaxMove != rec.RealMaxMove {
		t.Errorf("saved pools are %+v, want the real ones (%d/%d/%d)",
			saved.Points, rec.RealMaxHit, rec.RealMaxMana, rec.RealMaxMove)
	}
	if saved.Abilities != rec.RealAbilities {
		t.Errorf("saved abilities are %+v, want %+v", saved.Abilities, rec.RealAbilities)
	}
	// Not the real values but the C's own constants, because equip_char
	// adjusts RealArmor directly: db.c:2354-2356.
	if saved.Points.Armor != 100 || saved.Points.HitRoll != 0 || saved.Points.DamRoll != 0 {
		t.Errorf("saved armour/hitroll/damroll are %d/%d/%d, want 100/0/0",
			saved.Points.Armor, saved.Points.HitRoll, saved.Points.DamRoll)
	}

	// BaseRecord takes a copy: the live character is untouched, which is
	// what lets a background save work from a snapshot.
	if rec.Points.Armor != live.Points.Armor || rec.Abilities != live.Abilities {
		t.Error("BaseRecord mutated the record it was given")
	}

	// What a load then does with it.
	reloaded := saved
	SnapshotReal(&reloaded)
	RecomputeAffects(&reloaded)

	if reloaded.Points != live.Points {
		t.Errorf("round trip moved the points:\n got %+v\nwant %+v", reloaded.Points, live.Points)
	}
	if reloaded.Abilities != live.Abilities {
		t.Errorf("round trip moved the abilities:\n got %+v\nwant %+v", reloaded.Abilities, live.Abilities)
	}
	if reloaded.AffectFlags != live.AffectFlags {
		t.Errorf("round trip moved the affect flags: got %v, want %v",
			reloaded.AffectFlags, live.AffectFlags)
	}
}

// TestBaseRecordDoesNotSaveWornArmour. equip_char subtracts an ITEM_ARMOR's
// class from RealArmor directly rather than going through an affect — the one
// place this port keeps the C's method (docs/deviations.md) — so RealArmor on
// an equipped character already has the armour folded in. Writing it would
// fold it in a second time on the next login, which is why BaseRecord writes
// the C's flat 100 instead (db.c:2354).
func TestBaseRecordDoesNotSaveWornArmour(t *testing.T) {
	rec := affectedCharacter()
	rec.RealArmor = 70 // as if wearing a suit of plate worth 30
	RecomputeAffects(rec)
	if rec.Points.Armor != 70 {
		t.Fatalf("armour is %d, want 70", rec.Points.Armor)
	}

	if got := BaseRecord(*rec).Points.Armor; got != 100 {
		t.Errorf("saved armour is %d, want 100 — worn armour must not reach the file", got)
	}
}
