// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"github.com/gerrowadat/disgracelands/internal/rng"
	"testing"
)

// newRNG is the C server's own generator on a fixed seed, so a failing test
// can be reproduced and so these roll the numbers the C would.
func newRNG() *rng.Rand { return rng.NewRand(rng.NewCircle(20)) }

// TestRemortMasksAreOneShiftedByTheClass is the invariant the remort vector's
// type rests on, and it is now the only thing holding it up.
//
// The vector is a Set[Class] (apply.go), which means bit N *is* class N.
// That is true because pc_class_remort_masks (class.c:82) assigns 1, 2, 4, 8
// and 16 to the five classes in their own numeric order — a coincidence of
// the C's making, not a rule it states anywhere. If somebody ever renumbered
// a class or reassigned a mask, every remort check in the game would quietly
// be about a different class, and nothing else would notice.
//
// This test used to compare the table against a second set of constants
// (RemortMagicUser and friends). Those are gone: they were another spelling
// of the class numbers, which is the duplication §3.5 objects to. What
// replaced them is the rule itself, asserted directly.
func TestRemortMasksAreOneShiftedByTheClass(t *testing.T) {
	for _, class := range []Class{ClassMagicUser, ClassCleric, ClassThief, ClassWarrior, ClassPaladin} {
		mask, ok := classRemortMasks[class]
		if !ok {
			t.Errorf("class %d has no remort mask", class)
			continue
		}
		if want := int32(1) << uint(class); mask != want {
			t.Errorf("class %d has mask %d, want %d (1 << %d) — Set[Class] assumes bit N is class N",
				class, mask, want, class)
		}
		// And the set built from the class agrees with the stored mask.
		if got := NewSet(class).Raw(); got != uint64(mask) { //nolint:gosec // five small positive masks
			t.Errorf("NewSet(%d) is %#x, the C's mask is %#x", class, got, mask)
		}
	}
	if classRemortMasks[ClassPaladin] != 16 {
		t.Errorf("paladin's mask is %d, want 16", classRemortMasks[ClassPaladin])
	}
}

// TestANewMortalIsNotStartedYet is the distinction the C draws and this port
// got wrong once: init_char leaves an ordinary new character at level zero,
// and do_start does not run until they choose to enter the world.
func TestANewMortalIsNotStartedYet(t *testing.T) {
	r := newRNG()
	rec := &PlayerRecord{Name: "Welmar", Class: ClassThief, Sex: SexFemale}
	InitChar(rec, r, false)

	if rec.Level != 0 {
		t.Errorf("level %d after InitChar, want 0 — do_start has not run yet", rec.Level)
	}
	if rec.Points.MaxHit != 0 {
		t.Errorf("max hit %d, want 0 until do_start sets it", rec.Points.MaxHit)
	}
	if len(rec.Skills) != 0 {
		t.Errorf("a mortal knows %d skills at init_char, want none", len(rec.Skills))
	}

	// But these are all set here, not by do_start.
	if rec.Points.MaxMana != baseMaxMana || rec.Points.MaxMove != baseMaxMove {
		t.Errorf("mana/move are %d/%d, want %d/%d",
			rec.Points.MaxMana, rec.Points.MaxMove, baseMaxMana, baseMaxMove)
	}
	if rec.Points.Armor != baseArmor {
		t.Errorf("armour %d, want %d", rec.Points.Armor, baseArmor)
	}
	if rec.Hometown != 1 {
		t.Errorf("hometown %d, want 1 — init_char sets it literally", rec.Hometown)
	}
	if rec.LoadRoom != NoRoom {
		t.Errorf("load room %d, want NoRoom", rec.LoadRoom)
	}
	// Female: weight 100..160, height 150..180.
	if rec.Weight < 100 || rec.Weight > 160 || rec.Height < 150 || rec.Height > 180 {
		t.Errorf("a woman was rolled %d lbs and %d cm", rec.Weight, rec.Height)
	}

	// Now start them, as entering the world does.
	Start(rec, r)
	if rec.Level != 1 {
		t.Errorf("level %d after Start, want 1", rec.Level)
	}
	// do_start does not touch mana or movement, so the init_char values
	// survive — with movement raised by the level gained. This is why a new
	// character in the C prompts with a hundred mana and never zero.
	if rec.Points.MaxMana != baseMaxMana {
		t.Errorf("max mana %d after Start, want %d untouched", rec.Points.MaxMana, baseMaxMana)
	}
	if rec.Points.MaxMove <= baseMaxMove {
		t.Errorf("max move %d after Start, want more than %d", rec.Points.MaxMove, baseMaxMove)
	}
	if rec.Points.Hit != rec.Points.MaxHit || rec.Points.MaxHit < baseMaxHit {
		t.Errorf("hit %d/%d after Start", rec.Points.Hit, rec.Points.MaxHit)
	}
	if rec.Title != "the Pilferess" {
		t.Errorf("title %q, want the level-one female thief's", rec.Title)
	}
}

// TestTheFirstCharacterIsAnImplementorAndSkipsDoStart covers the other half:
// init_char promotes them, and because their level is no longer zero the
// server must not run do_start on them — which would put them back to level
// one with a rolled set of statistics.
func TestTheFirstCharacterIsAnImplementorAndSkipsDoStart(t *testing.T) {
	rec := &PlayerRecord{Name: "Zod", Class: ClassWarrior, Sex: SexMale}
	InitChar(rec, newRNG(), true)

	if rec.Level != LevelImplementor {
		t.Errorf("level %d, want %d", rec.Level, LevelImplementor)
	}
	if rec.Points.Exp != implementorExp {
		t.Errorf("exp %d, want %d", rec.Points.Exp, implementorExp)
	}
	if rec.Points.MaxHit != 500 || rec.Points.Hit != 500 {
		t.Errorf("hit %d/%d, want 500/500", rec.Points.Hit, rec.Points.MaxHit)
	}
	if rec.Points.MaxMana != baseMaxMana || rec.Points.MaxMove != baseMaxMove {
		t.Errorf("mana/move %d/%d, want %d/%d",
			rec.Points.MaxMana, rec.Points.MaxMove, baseMaxMana, baseMaxMove)
	}
	if rec.Title != "the Implementor" {
		t.Errorf("title %q, want %q", rec.Title, "the Implementor")
	}
	if rec.Conditions != [3]int32{-1, -1, -1} {
		t.Errorf("conditions %v, want all -1 — gods do not eat", rec.Conditions)
	}
	if len(rec.Skills) != MaxSkills {
		t.Errorf("knows %d skills, want all %d", len(rec.Skills), MaxSkills)
	}
	for skill, level := range rec.Skills {
		if level != 100 {
			t.Fatalf("skill %d is at %d%%, want 100%%", skill, level)
		}
	}
	if rec.Abilities.Strength != 25 || rec.Abilities.StrengthPercentile != 100 {
		t.Errorf("strength %d/%d, want 25/100",
			rec.Abilities.Strength, rec.Abilities.StrengthPercentile)
	}
}

// TestNewCharacterDefaultsAreTheLocalOnes covers the block Disgracelands
// added at the class prompt, which stock CircleMUD does not have.
func TestNewCharacterDefaultsAreTheLocalOnes(t *testing.T) {
	rec := &PlayerRecord{Class: ClassCleric}
	ApplyNewCharacterDefaults(rec)

	if rec.RemortVector != NewSet(ClassCleric) {
		t.Errorf("remort vector %d, want a cleric's %d",
			rec.RemortVector, classRemortMasks[ClassCleric])
	}
	// A new cleric must read as a cleric through the remort-aware macro.
	if !IsCleric(rec) {
		t.Error("a new cleric does not test as one")
	}

	for name, bit := range map[string]PrefFlag{
		"colour (low)":  PrefColour1,
		"colour (high)": PrefColour2,
		"display hp":    PrefDisplayHP,
		"display mana":  PrefDisplayMana,
		"display move":  PrefDisplayMove,
		"autoexit":      PrefAutoExit,
	} {
		if !rec.Preferences.Has(bit) {
			t.Errorf("a new character does not have %s set", name)
		}
	}
	if !rec.PlayerFlags.Has(PlayerSiteOK) {
		t.Error("a new character is not site-cleared")
	}
}
