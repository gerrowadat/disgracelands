// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// TestPaladinIsNotSelectableAtCreation is the deviation from the C, asserted
// so it cannot be undone by someone "fixing" ParseCreationClass to match
// parse_class.
//
// The C's parse_class accepts 'p' and creation calls it, so typing the
// unadvertised letter made a Paladin without remorting. The menu never
// offered it. See docs/proposals/go-port-plan.md.
func TestPaladinIsNotSelectableAtCreation(t *testing.T) {
	for _, arg := range []byte{'p', 'P'} {
		if got := ParseCreationClass(arg); got != ClassUndefined {
			t.Errorf("ParseCreationClass(%q) = %d, want undefined; Paladin is remort-only",
				arg, got)
		}
	}
	// But it remains reachable where the C is right to allow it.
	for _, arg := range []byte{'p', 'P'} {
		if got := ParseClass(arg); got != ClassPaladin {
			t.Errorf("ParseClass(%q) = %d, want Paladin; remort and `set class` need it", arg, got)
		}
	}
	// And the menu must not advertise it.
	if strings.Contains(strings.ToLower(CreationMenu), "paladin") {
		t.Error("the creation menu advertises Paladin")
	}
}

func TestCreationClassLetters(t *testing.T) {
	for arg, want := range map[byte]int32{
		'm': ClassMagicUser, 'M': ClassMagicUser,
		'c': ClassCleric, 'C': ClassCleric,
		't': ClassThief, 'T': ClassThief,
		'w': ClassWarrior, 'W': ClassWarrior,
		'x': ClassUndefined, ' ': ClassUndefined,
	} {
		if got := ParseCreationClass(arg); got != want {
			t.Errorf("ParseCreationClass(%q) = %d, want %d", arg, got, want)
		}
	}
	// Every letter the menu offers must parse, or the menu lies.
	for _, line := range strings.Split(CreationMenu, "\n") {
		i := strings.Index(line, "[")
		if i < 0 || i+2 >= len(line) {
			continue
		}
		letter := line[i+1]
		if ParseCreationClass(letter) == ClassUndefined {
			t.Errorf("the menu offers [%c] but it does not parse", letter)
		}
	}
}

func TestRolledAbilitiesAreInRange(t *testing.T) {
	// 4d6 drop lowest is 3..18.
	rng := rand.New(rand.NewPCG(1, 2))
	for _, class := range []int32{ClassMagicUser, ClassCleric, ClassThief, ClassWarrior, ClassPaladin} {
		for i := 0; i < 200; i++ {
			a := RollAbilities(class, rng)
			for name, v := range map[string]int32{
				"Str": a.Strength, "Int": a.Intelligence, "Wis": a.Wisdom,
				"Dex": a.Dexterity, "Con": a.Constitution, "Cha": a.Charisma,
			} {
				if v < 3 || v > 18 {
					t.Fatalf("class %d rolled %s = %d, want 3..18", class, name, v)
				}
			}
		}
	}
}

// TestEachClassGetsItsBestRollWhereItMatters checks the per-class dealing
// order from roll_real_abils: the highest of the six scores goes to the
// statistic that class is built around.
func TestEachClassGetsItsBestRollWhereItMatters(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 42))

	best := func(a Abilities) int32 {
		m := a.Strength
		for _, v := range []int32{a.Intelligence, a.Wisdom, a.Dexterity, a.Constitution, a.Charisma} {
			if v > m {
				m = v
			}
		}
		return m
	}

	for _, tc := range []struct {
		class   int32
		primary func(Abilities) int32
		name    string
	}{
		{ClassMagicUser, func(a Abilities) int32 { return a.Intelligence }, "intelligence"},
		{ClassCleric, func(a Abilities) int32 { return a.Wisdom }, "wisdom"},
		{ClassThief, func(a Abilities) int32 { return a.Dexterity }, "dexterity"},
		{ClassWarrior, func(a Abilities) int32 { return a.Strength }, "strength"},
		{ClassPaladin, func(a Abilities) int32 { return a.Charisma }, "charisma"},
	} {
		for i := 0; i < 50; i++ {
			a := RollAbilities(tc.class, rng)
			if tc.primary(a) != best(a) {
				t.Errorf("class %d: %s is %d but the best roll was %d",
					tc.class, tc.name, tc.primary(a), best(a))
				break
			}
		}
	}
}

// TestOnlyWarriorsRollExceptionalStrength is a property of the C worth
// keeping: the percentile field is meaningless for every other class, which
// is why a converted record may carry a stale value in it.
func TestOnlyWarriorsRollExceptionalStrength(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 7))
	for _, class := range []int32{ClassMagicUser, ClassCleric, ClassThief, ClassPaladin} {
		for i := 0; i < 300; i++ {
			if a := RollAbilities(class, rng); a.StrengthPercentile != 0 {
				t.Fatalf("class %d rolled an exceptional-strength percentile of %d",
					class, a.StrengthPercentile)
			}
		}
	}

	// And a warrior only rolls one on an 18.
	for i := 0; i < 2000; i++ {
		a := RollAbilities(ClassWarrior, rng)
		if a.Strength != 18 && a.StrengthPercentile != 0 {
			t.Fatalf("a warrior with strength %d has a percentile of %d",
				a.Strength, a.StrengthPercentile)
		}
	}
}

func TestParseSex(t *testing.T) {
	for arg, want := range map[byte]int32{
		'm': SexMale, 'M': SexMale,
		'f': SexFemale, 'F': SexFemale,
		'x': -1,
	} {
		if got := ParseSex(arg); got != want {
			t.Errorf("ParseSex(%q) = %d, want %d", arg, got, want)
		}
	}
}
