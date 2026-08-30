// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"strings"
	"testing"
)

func TestParseCastArgument(t *testing.T) {
	for _, tc := range []struct {
		in     string
		spell  string
		target string
		err    string
	}{
		{in: "'magic missile' orc", spell: "magic missile", target: "orc"},
		{in: "'magic missile'", spell: "magic missile"},
		{in: "'armor'", spell: "armor"},
		{in: "  'cure light'  welmar  ", spell: "cure light", target: "welmar"},
		{in: "", err: "Cast what where?\r\n"},
		{in: "magic missile", err: "Spell names must be enclosed in the Holy Magic Symbols: '\r\n"},

		// The four strtok answers a quote-finding parser gets wrong, all of
		// them from reference/tools/castparse.c rather than from reading
		// (#358). The first three are why the empty-spell-name behaviour
		// could not be fixed on its own (#365).
		//
		// `''` is not an empty spell name: strtok skips a *run* of
		// delimiters, so the two quotes collapse and there is no second
		// token at all.
		{in: "''", err: "Spell names must be enclosed in the Holy Magic Symbols: '\r\n"},
		{in: "'''", err: "Spell names must be enclosed in the Holy Magic Symbols: '\r\n"},
		// `'  '` *is* handed over, because a space is not a delimiter.
		// It reaches find_skill_num as two spaces and is answered as armor.
		{in: "'  '", spell: "  "},
		{in: "' '", spell: " "},
		// The empty quotes vanish and the target becomes the spell name.
		{in: "'' fido", spell: " fido"},
		// No closing quote is not an error: the second strtok has no
		// delimiter left and returns the rest of the line.
		{in: "'magic missile", spell: "magic missile"},
		{in: "'mag mis fido", spell: "mag mis fido"},

		// Not "   ": Context.Arg is trimmed by session.split, so an
		// all-whitespace argument cannot reach here, and the C answer for
		// it ("must be enclosed", where this port says "Cast what where?")
		// is unreachable. ParseCastArgument's doc comment says so.
	} {
		spell, target, err := ParseCastArgument(tc.in)
		if err != tc.err {
			t.Errorf("ParseCastArgument(%q) error = %q, want %q", tc.in, err, tc.err)
			continue
		}
		if err != "" {
			continue
		}
		if spell != tc.spell || target != tc.target {
			t.Errorf("ParseCastArgument(%q) = %q, %q; want %q, %q",
				tc.in, spell, target, tc.spell, tc.target)
		}
	}
}

// TestARemortedCharacterKeepsTheirSpellbook. This is the local rewrite and
// the reason remorting is worth doing: stock CircleMUD tests only the class
// you are now.
func TestARemortedCharacterKeepsTheirSpellbook(t *testing.T) {
	missile, ok := Spell(SpellMagicMissile)
	if !ok {
		t.Fatal("magic missile is not in the table")
	}

	// A cleric who has never been a mage does not know it.
	cleric := &PlayerRecord{
		Class: ClassCleric, Level: 30,
		RemortVector: NewSet(ClassCleric),
	}
	if KnowsSpell(cleric, missile) {
		t.Error("a plain cleric knows magic missile")
	}

	// One who remorted through mage does, at the mage's level.
	remorted := &PlayerRecord{
		Class: ClassCleric, Level: 30,
		RemortVector: NewSet(ClassCleric, ClassMagicUser),
	}
	if !KnowsSpell(remorted, missile) {
		t.Error("a cleric who remorted through mage does not know magic missile")
	}

	// And level still applies: a mage learns it at 1, so this is not a
	// useful boundary. Take one that is learned late instead.
	fireball, ok := Spell(SpellFireball)
	if !ok {
		t.Fatal("fireball is not in the table")
	}
	need := MinLevelFor(fireball, ClassMagicUser)

	low := &PlayerRecord{
		Class: ClassCleric, Level: need - 1,
		RemortVector: NewSet(ClassCleric, ClassMagicUser),
	}
	if KnowsSpell(low, fireball) {
		t.Errorf("a level %d character knows fireball, which needs %d", need-1, need)
	}
	low.Level = need
	if !KnowsSpell(low, fireball) {
		t.Errorf("a level %d character does not know fireball", need)
	}
}

// TestAPaladinFallsAndStaysFallen. Being cast out is permanent: recovering
// alignment afterwards does nothing.
func TestAPaladinFallsAndStaysFallen(t *testing.T) {
	rec := &PlayerRecord{Name: "Welmar", Class: ClassPaladin, Alignment: -400}

	verdict := JudgePaladin(rec)
	if verdict.Allowed {
		t.Error("a paladin at -400 alignment was allowed to cast")
	}
	if !strings.Contains(verdict.Message, "never again") {
		t.Errorf("the message was %q", verdict.Message)
	}
	if !strings.Contains(verdict.Broadcast, "has been cast out") {
		t.Errorf("the broadcast was %q", verdict.Broadcast)
	}
	if !specFlags(rec.SpecFlags).Has(PaladinFallen) {
		t.Error("the paladin was not marked fallen")
	}

	// Redemption is not available to the fallen.
	rec.Alignment = 1000
	verdict = JudgePaladin(rec)
	if verdict.Allowed {
		t.Error("a fallen paladin recovered by raising their alignment")
	}
	if !strings.Contains(verdict.Message, "sinful existence") {
		t.Errorf("the message was %q", verdict.Message)
	}
}

// TestAPaladinBecomesUnworthyAndCanRepent.
func TestAPaladinBecomesUnworthyAndCanRepent(t *testing.T) {
	rec := &PlayerRecord{Name: "Welmar", Class: ClassPaladin, Alignment: -100}

	verdict := JudgePaladin(rec)
	if verdict.Allowed {
		t.Error("a paladin at -100 alignment was allowed to cast")
	}
	if !strings.Contains(verdict.Message, "unworthy") {
		t.Errorf("the message was %q", verdict.Message)
	}
	if !specFlags(rec.SpecFlags).Has(PaladinUnworthy) {
		t.Error("the paladin was not marked unworthy")
	}
	if specFlags(rec.SpecFlags).Has(PaladinFallen) {
		t.Error("an unworthy paladin was also marked fallen")
	}

	// Still refused while their alignment is merely non-terrible.
	rec.Alignment = 100
	if JudgePaladin(rec).Allowed {
		t.Error("an unworthy paladin cast at alignment 100")
	}

	// Above 600 they are forgiven.
	rec.Alignment = 700
	verdict = JudgePaladin(rec)
	if !verdict.Allowed {
		t.Error("an unworthy paladin at alignment 700 was not forgiven")
	}
	if !strings.Contains(verdict.Message, "Welcome back") {
		t.Errorf("the message was %q", verdict.Message)
	}
	if specFlags(rec.SpecFlags).Has(PaladinUnworthy) {
		t.Error("the unworthy flag was not cleared")
	}
}

// TestAGoodPaladinIsLeftAlone, and so is everybody who is not a paladin.
func TestAGoodPaladinIsLeftAlone(t *testing.T) {
	good := &PlayerRecord{Name: "Welmar", Class: ClassPaladin, Alignment: 800}
	if verdict := JudgePaladin(good); !verdict.Allowed {
		t.Error("a good paladin was refused")
	}

	// A paladin with a middling positive alignment is fine too — the rules
	// only bite below zero.
	middling := &PlayerRecord{Name: "Welmar", Class: ClassPaladin, Alignment: 100}
	if verdict := JudgePaladin(middling); !verdict.Allowed {
		t.Error("a paladin at alignment 100 was refused")
	}

	for _, class := range []Class{ClassMagicUser, ClassCleric, ClassThief, ClassWarrior} {
		rec := &PlayerRecord{Class: class, Alignment: -1000}
		if verdict := JudgePaladin(rec); !verdict.Allowed {
			t.Errorf("class %d at alignment -1000 was refused a spell", class)
		}
		if rec.SpecFlags != 0 {
			t.Errorf("class %d was given a paladin flag", class)
		}
	}
}

// TestSpellDamageUsesTheBetterDiceForAMagicUser. The same spell rolls d8 for
// a mage and d6 for anybody else, which is most of a mage's advantage early
// on — and the test is the remort-aware macro.
func TestSpellDamageUsesTheBetterDiceForAMagicUser(t *testing.T) {
	r := newRNG()

	mage := &PlayerRecord{Class: ClassMagicUser, RemortVector: NewSet(ClassMagicUser)}
	cleric := &PlayerRecord{Class: ClassCleric, RemortVector: NewSet(ClassCleric)}
	victim := &PlayerRecord{Class: ClassWarrior, Level: 10}

	var mageMax, clericMax int32
	for i := 0; i < 3000; i++ {
		mageMax = max(mageMax, SpellDamage(SpellMagicMissile, mage, victim, 10, r))
		clericMax = max(clericMax, SpellDamage(SpellMagicMissile, cleric, victim, 10, r))
	}

	// 1d8+1 against 1d6+1.
	if mageMax != 9 {
		t.Errorf("a mage's best magic missile was %d, want 9", mageMax)
	}
	if clericMax != 7 {
		t.Errorf("a cleric's best magic missile was %d, want 7", clericMax)
	}

	// A cleric who remorted through mage rolls the mage's dice.
	remorted := &PlayerRecord{
		Class:        ClassCleric,
		RemortVector: NewSet(ClassCleric, ClassMagicUser),
	}
	var remortedMax int32
	for i := 0; i < 3000; i++ {
		remortedMax = max(remortedMax, SpellDamage(SpellMagicMissile, remorted, victim, 10, r))
	}
	if remortedMax != 9 {
		t.Errorf("a remorted cleric's best magic missile was %d, want a mage's 9", remortedMax)
	}
}

// TestEnergyDrainDestroysTheWeak: level 2 or below takes a flat 100.
func TestEnergyDrainDestroysTheWeak(t *testing.T) {
	r := newRNG()
	caster := &PlayerRecord{Class: ClassCleric}

	for _, level := range []int32{1, 2} {
		victim := &PlayerRecord{Level: level}
		if got := SpellDamage(SpellEnergyDrain, caster, victim, 10, r); got != 100 {
			t.Errorf("energy drain on a level %d victim did %d, want 100", level, got)
		}
	}

	victim := &PlayerRecord{Level: 3}
	for i := 0; i < 200; i++ {
		if got := SpellDamage(SpellEnergyDrain, caster, victim, 10, r); got < 1 || got > 10 {
			t.Fatalf("energy drain on a level 3 victim did %d, want 1..10", got)
		}
	}
}

// TestDispelTurnsOnAMatchingCaster, which is the trap in both spells.
func TestDispelTurnsOnAMatchingCaster(t *testing.T) {
	r := newRNG()

	evil := &PlayerRecord{Alignment: -1000, Points: Points{Hit: 250}}
	good := &PlayerRecord{Alignment: 1000, Points: Points{Hit: 250}}
	neutral := &PlayerRecord{Alignment: 0, Points: Points{Hit: 250}}

	// An evil caster casting dispel evil hurts themselves, down to one.
	if got := Dispel(SpellDispelEvil, evil, neutral, r); !got.Backfired || got.Damage != 249 {
		t.Errorf("dispel evil from an evil caster gave %+v, want a backfire of 249", got)
	}
	// A good caster casting dispel good, likewise.
	if got := Dispel(SpellDispelGood, good, neutral, r); !got.Backfired || got.Damage != 249 {
		t.Errorf("dispel good from a good caster gave %+v, want a backfire of 249", got)
	}

	// The gods protect the wrong target.
	if got := Dispel(SpellDispelEvil, neutral, good, r); !got.Protected {
		t.Errorf("dispel evil on a good victim gave %+v, want protection", got)
	}
	if got := Dispel(SpellDispelGood, neutral, evil, r); !got.Protected {
		t.Errorf("dispel good on an evil victim gave %+v, want protection", got)
	}

	// And an ordinary casting does 6d8+6.
	got := Dispel(SpellDispelEvil, good, evil, r)
	if got.Backfired || got.Protected || got.Damage < 12 || got.Damage > 54 {
		t.Errorf("an ordinary dispel evil gave %+v, want 12..54 damage", got)
	}
}

func TestSpellHealing(t *testing.T) {
	r := newRNG()
	victim := &PlayerRecord{Points: Points{Hit: 10, MaxHit: 200}}

	for i := 0; i < 500; i++ {
		// 1d8 + 1 + level/4; at level 20 that is 1..8 plus 1 plus 5, so 7..14.
		if got := SpellHealing(SpellCureLight, victim, 20, r); got.Amount < 7 || got.Amount > 14 {
			t.Fatalf("cure light healed %d at level 20, want 7..14", got.Amount)
		}
	}

	// Full heal is defined by what is missing rather than a roll.
	full := SpellHealing(SpellFullHeal, victim, 20, r)
	if full.Amount != 190 {
		t.Errorf("full heal restored %d, want the 190 missing", full.Amount)
	}
	if !strings.Contains(full.Message, "lovely") {
		t.Errorf("full heal said %q", full.Message)
	}
}

func TestTargetQuestion(t *testing.T) {
	people, _ := Spell(SpellMagicMissile)
	if got := TargetQuestion(people); !strings.Contains(got, "who") {
		t.Errorf("a spell aimed at people asks %q", got)
	}

	things, _ := Spell(SpellEnchantWeapon)
	if got := TargetQuestion(things); !strings.Contains(got, "what") {
		t.Errorf("a spell aimed at objects asks %q", got)
	}
}
