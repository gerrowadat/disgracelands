// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"github.com/gerrowadat/disgracelands/internal/rng"
	"strings"
	"testing"
)

// TestPaladinIsNotSelectableAtCreation is the deviation from the C, asserted
// so it cannot be undone by someone "fixing" ParseCreationClass to match
// parse_class.
//
// The C's parse_class accepts 'p' and creation calls it, so typing the
// unadvertised letter made a Paladin without remorting. The menu never
// offered it. See docs/design/go-port-plan.md.
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
	for arg, want := range map[byte]Class{
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
	r := rng.NewRand(rng.NewCircle(1))
	for _, class := range []Class{ClassMagicUser, ClassCleric, ClassThief, ClassWarrior, ClassPaladin} {
		for i := 0; i < 200; i++ {
			a := RollAbilities(class, r)
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
	r := rng.NewRand(rng.NewCircle(42))

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
		class   Class
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
			a := RollAbilities(tc.class, r)
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
	r := rng.NewRand(rng.NewCircle(7))
	for _, class := range []Class{ClassMagicUser, ClassCleric, ClassThief, ClassPaladin} {
		for i := 0; i < 300; i++ {
			if a := RollAbilities(class, r); a.StrengthPercentile != 0 {
				t.Fatalf("class %d rolled an exceptional-strength percentile of %d",
					class, a.StrengthPercentile)
			}
		}
	}

	// And a warrior only rolls one on an 18.
	for i := 0; i < 2000; i++ {
		a := RollAbilities(ClassWarrior, r)
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

// setPrimeAbility and RollAbilities have to agree about which statistic is a
// class's prime requisite. RollAbilities hands table[0] — the best of the six
// rolls — to it, and Remort pins that same statistic for the class being left
// behind (#262's "set player prime-stat-from-previous-class 18").
//
// Two independent lists of one fact is the shape that drifts, so this derives
// the answer from the roll rather than restating it: for every roll, the
// prime statistic must be at least as high as all five others, because it was
// given the maximum.
func TestPrimeAbilityIsTheOneTheRollFavours(t *testing.T) {
	classes := []struct {
		class Class
		name  string
	}{
		{ClassMagicUser, "intelligence"},
		{ClassCleric, "wisdom"},
		{ClassThief, "dexterity"},
		{ClassWarrior, "strength"},
		{ClassPaladin, "charisma"},
	}
	for _, c := range classes {
		if got := PrimeAbilityName(c.class); got != c.name {
			t.Errorf("PrimeAbilityName(%d) = %q, want %q", c.class, got, c.name)
		}

		r := rng.NewRand(rng.NewCircle(4242))
		for i := 0; i < 500; i++ {
			a := RollAbilities(c.class, r)
			all := map[string]int32{
				"strength":     a.Strength,
				"intelligence": a.Intelligence,
				"wisdom":       a.Wisdom,
				"dexterity":    a.Dexterity,
				"constitution": a.Constitution,
				"charisma":     a.Charisma,
			}
			prime := all[c.name]
			for name, v := range all {
				if v > prime {
					t.Fatalf("%s: roll %d gave %s=%d over the prime %s=%d; "+
						"PrimeAbilityName and RollAbilities disagree about the prime requisite",
						c.name, i, name, v, c.name, prime)
				}
			}
		}
	}
}

// setPrimeAbility writes the field PrimeAbilityName names, and clears
// percentile strength with it the way `set str` does.
func TestSetPrimeAbility(t *testing.T) {
	for _, tc := range []struct {
		class Class
		read  func(Abilities) int32
	}{
		{ClassMagicUser, func(a Abilities) int32 { return a.Intelligence }},
		{ClassCleric, func(a Abilities) int32 { return a.Wisdom }},
		{ClassThief, func(a Abilities) int32 { return a.Dexterity }},
		{ClassWarrior, func(a Abilities) int32 { return a.Strength }},
		{ClassPaladin, func(a Abilities) int32 { return a.Charisma }},
	} {
		a := Abilities{StrengthPercentile: 91}
		setPrimeAbility(&a, tc.class, 18)
		if got := tc.read(a); got != 18 {
			t.Errorf("class %d: prime statistic = %d, want 18", tc.class, got)
		}
	}

	// Warrior alone: 18/00, not 18 plus whatever percentile was there.
	a := Abilities{StrengthPercentile: 91}
	setPrimeAbility(&a, ClassWarrior, 18)
	if a.StrengthPercentile != 0 {
		t.Errorf("strength percentile = %d, want 0: `set str 18` clears it", a.StrengthPercentile)
	}
}
