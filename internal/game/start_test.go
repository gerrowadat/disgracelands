// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"math/rand/v2"
	"testing"
)

// TestStartGivesEveryClassSomethingToLiveOn is the regression test for a live
// bug: a freshly created character prompted `0H 0M 0V` because Create built a
// record by hand and never ran do_start.
func TestStartGivesEveryClassSomethingToLiveOn(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for _, class := range []int32{ClassMagicUser, ClassCleric, ClassThief, ClassWarrior, ClassPaladin} {
		rec := &PlayerRecord{Name: "Test", Class: class}
		Start(rec, rng)

		if rec.Level != 1 {
			t.Errorf("class %d: level %d, want 1", class, rec.Level)
		}
		if rec.Points.Exp != 1 {
			t.Errorf("class %d: exp %d, want 1", class, rec.Points.Exp)
		}
		if rec.Points.Hit <= 0 || rec.Points.Hit != rec.Points.MaxHit {
			t.Errorf("class %d: hit %d/%d", class, rec.Points.Hit, rec.Points.MaxHit)
		}
		if rec.Points.Move <= 0 || rec.Points.Move != rec.Points.MaxMove {
			t.Errorf("class %d: move %d/%d", class, rec.Points.Move, rec.Points.MaxMove)
		}
		// Mana is not gained at level one — advance_level guards it on
		// level > 1 — so every new character has none, whatever their class.
		// This is why the C's prompt reads 0M for a brand new mage.
		if rec.Points.MaxMana != 0 || rec.Points.Mana != 0 {
			t.Errorf("class %d: mana %d/%d, want 0/0 at level one",
				class, rec.Points.Mana, rec.Points.MaxMana)
		}
		if rec.Conditions != StartingConditions {
			t.Errorf("class %d: conditions %v, want %v", class, rec.Conditions, StartingConditions)
		}
	}
}

// TestFirstLevelGainsMatchTheCsRanges checks the per-class gain from
// advance_level (class.c:1860) on top of do_start's base of ten, including
// the constitution bonus from con_app[] and the MAX(1, ...) floors.
func TestFirstLevelGainsMatchTheCsRanges(t *testing.T) {
	for _, tc := range []struct {
		class            int32
		name             string
		lowHit, highHit  int32
		lowMove, highMov int32
	}{
		{ClassMagicUser, "magic user", 3, 8, 0, 2},
		{ClassCleric, "cleric", 5, 10, 0, 2},
		{ClassThief, "thief", 7, 13, 1, 3},
		{ClassWarrior, "warrior", 10, 15, 1, 3},
		{ClassPaladin, "paladin", 10, 14, 1, 3},
	} {
		rng := rand.New(rand.NewPCG(uint64(tc.class), 99)) //nolint:gosec // a test seed, not a secret
		sawLowHit, sawHighHit, sawMoveFloor := false, false, false

		for i := 0; i < 5000; i++ {
			rec := &PlayerRecord{Class: tc.class}
			Start(rec, rng)

			// The constitution bonus is part of the roll, so the window moves
			// with whatever constitution came up.
			con := conApplyHitPoints[abilityIndex(rec.Abilities.Constitution)]
			low, high := max(1, tc.lowHit+con), max(1, tc.highHit+con)

			hit := rec.Points.MaxHit - baseMaxHit
			if hit < low || hit > high {
				t.Fatalf("%s: gained %d hit points with con %d, want %d..%d",
					tc.name, hit, rec.Abilities.Constitution, low, high)
			}
			sawLowHit = sawLowHit || hit == low
			sawHighHit = sawHighHit || hit == high

			move := rec.Points.MaxMove
			if move < max(1, tc.lowMove) || move > tc.highMov {
				t.Fatalf("%s: %d movement points, want %d..%d",
					tc.name, move, max(1, tc.lowMove), tc.highMov)
			}
			sawMoveFloor = sawMoveFloor || move == 1
		}

		// The C's number(lo, hi) is inclusive at both ends. If randRange were
		// exclusive at the top, or off by one at the bottom, the range check
		// above would still pass.
		if !sawLowHit || !sawHighHit {
			t.Errorf("%s: over 5000 rolls the gain never reached both ends of its window (low %v, high %v)",
				tc.name, sawLowHit, sawHighHit)
		}
		// A magic-user or cleric rolling number(0, 2) must still end up with
		// one movement point, not none: MAX(1, add_move).
		if tc.lowMove == 0 && !sawMoveFloor {
			t.Errorf("%s: never saw the MAX(1, add_move) floor take effect", tc.name)
		}
	}
}

// TestImmortalsGainHolylightAndStopEating covers advance_level's tail, which
// is the only place either is set.
func TestImmortalsGainHolylightAndStopEating(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 12)) //nolint:gosec // a test seed, not a secret

	mortal := &PlayerRecord{Class: ClassWarrior, Level: LevelImmortal - 1, Conditions: StartingConditions}
	AdvanceLevel(mortal, rng)
	if mortal.Conditions != StartingConditions {
		t.Errorf("a level %d character's conditions changed to %v", mortal.Level, mortal.Conditions)
	}
	if mortal.Preferences.Has(PrefHolylight) {
		t.Error("a mortal was given holylight")
	}

	immortal := &PlayerRecord{Class: ClassWarrior, Level: LevelImmortal, Conditions: StartingConditions}
	AdvanceLevel(immortal, rng)
	if immortal.Conditions != [3]int32{-1, -1, -1} {
		t.Errorf("immortal conditions are %v, want all -1", immortal.Conditions)
	}
	if !immortal.Preferences.Has(PrefHolylight) {
		t.Error("an immortal did not get holylight")
	}
}

// TestPracticesFollowTheRemortAwareClassTest is the local deviation from
// stock CircleMUD that is easiest to lose: IS_MAGIC_USER consults the remort
// vector, so a warrior who remorted through cleric practises like a cleric.
func TestPracticesFollowTheRemortAwareClassTest(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 14)) //nolint:gosec // a test seed, not a secret

	// Wisdom 18 gives a bonus of 5: a caster gains MAX(2, 5) = 5, a
	// non-caster MIN(2, MAX(1, 5)) = 2.
	abils := Abilities{Wisdom: 18}

	plain := &PlayerRecord{Class: ClassWarrior, Level: 5, Abilities: abils}
	AdvanceLevel(plain, rng)
	if plain.SpellsToLearn != 2 {
		t.Errorf("a plain warrior gained %d practices, want 2", plain.SpellsToLearn)
	}

	remorted := &PlayerRecord{
		Class: ClassWarrior, Level: 5, Abilities: abils,
		RemortVector: int32(RemortCleric),
	}
	AdvanceLevel(remorted, rng)
	if remorted.SpellsToLearn != 5 {
		t.Errorf("a warrior who remorted through cleric gained %d practices, want 5",
			remorted.SpellsToLearn)
	}
}

// TestThievesStartWithTheirSkills and nobody else does.
func TestThievesStartWithTheirSkills(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	for _, class := range []int32{ClassMagicUser, ClassCleric, ClassWarrior, ClassPaladin} {
		rec := &PlayerRecord{Class: class}
		Start(rec, rng)
		if len(rec.Skills) != 0 {
			t.Errorf("class %d starts with %d skills, want none", class, len(rec.Skills))
		}
	}

	rec := &PlayerRecord{Class: ClassThief}
	Start(rec, rng)
	for skill, want := range map[int32]int32{131: 10, 132: 10, 133: 5, 134: 15, 135: 10, 139: 10} {
		if got := rec.Skills[skill]; got != want {
			t.Errorf("thief skill %d = %d, want %d", skill, got, want)
		}
	}
}

// TestManaIsCappedAtTen guards advance_level's cap, which only bites at the
// levels where number(level, level*3/2) can exceed it.
func TestManaIsCappedAtTen(t *testing.T) {
	rng := rand.New(rand.NewPCG(8, 9))
	for level := int32(1); level <= 34; level++ {
		rec := &PlayerRecord{Class: ClassMagicUser, Level: level}
		for i := 0; i < 200; i++ {
			before := rec.Points.MaxMana
			AdvanceLevel(rec, rng)
			if gained := rec.Points.MaxMana - before; gained > 10 {
				t.Fatalf("level %d gained %d mana, want at most 10", level, gained)
			}
		}
	}
}
