// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fighter is a Fighter for tests.
type fighter struct {
	npc       bool
	pos       Position
	wielded   *Object
	sanctuary bool
}

func (f fighter) IsNPC() bool        { return f.npc }
func (f fighter) Position() Position { return f.pos }
func (f fighter) Wielded() *Object   { return f.wielded }
func (f fighter) Sanctuary() bool    { return f.sanctuary }

// TestComputeTHAC0MatchesTheC across a wide sweep of the inputs.
//
// The two ability adjustments are `int -= double` in the C, so each truncates
// separately. Doing the arithmetic in integers, or folding the two terms
// together before subtracting, gives different answers for most inputs — and
// they would still be plausible-looking to-hit numbers, which is why this is
// compared rather than read across.
func TestComputeTHAC0MatchesTheC(t *testing.T) {
	oracle := buildFightOracle(t)

	out, err := exec.Command(oracle, "sweep-thaco").Output()
	if err != nil {
		t.Fatalf("running the oracle sweep: %v", err)
	}

	compared := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 8 {
			t.Fatalf("oracle line %q has %d fields, want 8", line, len(fields))
		}
		var n [8]int32
		for i, f := range fields {
			v, err := strconv.Atoi(f)
			if err != nil {
				t.Fatalf("unparseable oracle field %q", f)
			}
			n[i] = int32(v)
		}

		rec := &PlayerRecord{
			Class: n[0], Level: n[1],
			Abilities: Abilities{
				Strength: n[2], StrengthPercentile: n[3],
				Intelligence: n[5], Wisdom: n[6],
			},
			Points: Points{HitRoll: n[4]},
		}

		if got := ComputeTHAC0(rec, fighter{pos: PosStanding}); got != n[7] {
			t.Fatalf("compute_thaco(class %d, level %d, str %d/%d, hitroll %d, int %d, wis %d) = %d, the C gives %d",
				n[0], n[1], n[2], n[3], n[4], n[5], n[6], got, n[7])
		}
		compared++
	}

	if compared == 0 {
		t.Fatal("the oracle sweep produced nothing")
	}
	t.Logf("compared %d to-hit numbers against the C", compared)
}

// TestTheAbilityAdjustmentsTruncateSeparately is the specific trap, asserted
// on its own so a failure says what broke rather than which of 40,000 sweep
// rows disagreed.
func TestTheAbilityAdjustmentsTruncateSeparately(t *testing.T) {
	oracle := buildFightOracle(t)

	// Intelligence 18 and wisdom 18 each give (18-13)/1.5 = 3.333. Truncating
	// after each subtraction takes 3 then 3; folding them together first
	// would take 6.666 and truncate once, which is also 6 — so the case that
	// separates them is one where the halves land differently.
	for _, tc := range []struct{ intel, wis int32 }{
		{18, 18}, {14, 14}, {15, 16}, {25, 3}, {3, 25}, {13, 13}, {12, 12},
	} {
		rec := &PlayerRecord{
			Class: ClassWarrior, Level: 10,
			Abilities: Abilities{Strength: 12, Intelligence: tc.intel, Wisdom: tc.wis},
		}
		got := ComputeTHAC0(rec, fighter{pos: PosStanding})

		want := runFightOracle(t, oracle, "compute",
			itoa(ClassWarrior), "10", "12", "0", "0", itoa(tc.intel), itoa(tc.wis), "0")

		if got != want {
			t.Errorf("int %d, wis %d: got %d, the C gives %d", tc.intel, tc.wis, got, want)
		}
	}
}

// TestTHAC0TableMatchesTheC at every class and level.
func TestTHAC0TableMatchesTheC(t *testing.T) {
	oracle := buildFightOracle(t)

	for _, class := range []int32{ClassMagicUser, ClassCleric, ClassThief, ClassWarrior, ClassPaladin} {
		for level := int32(0); level <= LevelImplementor; level++ {
			want := runFightOracle(t, oracle, "thaco", itoa(class), itoa(level))
			if got := THAC0(class, level); got != want {
				t.Errorf("THAC0(%d, %d) = %d, the C gives %d", class, level, got, want)
			}
		}
	}

	// Warriors and paladins share a table in the C, by falling through.
	for level := int32(0); level <= LevelImplementor; level++ {
		if THAC0(ClassWarrior, level) != THAC0(ClassPaladin, level) {
			t.Errorf("warrior and paladin differ at level %d", level)
		}
	}

	// An unreachable level does not fall through into another class's table.
	for _, level := range []int32{-1, 35, 1000} {
		if got := THAC0(ClassWarrior, level); got != 100 {
			t.Errorf("THAC0(warrior, %d) = %d, want 100", level, got)
		}
	}
}

func TestComputeArmorClassMatchesTheC(t *testing.T) {
	oracle := buildFightOracle(t)

	for _, armor := range []int32{-200, -100, -50, 0, 50, 100} {
		for dex := int32(0); dex <= 25; dex++ {
			for _, awake := range []bool{true, false} {
				pos := PosStanding
				if !awake {
					pos = PosSleeping
				}

				rec := &PlayerRecord{
					Points:    Points{Armor: armor},
					Abilities: Abilities{Dexterity: dex},
				}
				got := ComputeArmorClass(rec, fighter{pos: pos})
				want := runFightOracle(t, oracle, "ac", itoa(armor), itoa(dex), boolArg(awake))

				if got != want {
					t.Errorf("armour %d, dex %d, awake %v: got %d, the C gives %d",
						armor, dex, awake, got, want)
				}
			}
		}
	}
}

// TestThePositionMultiplierIsIntegerDivision. The C's comment beside it lists
// 1.33x for a sitting victim through 3.00x for a mortally wounded one; the
// arithmetic is integer division, so the real multipliers are 1, 1, 2, 2, 2
// and 3. The comment has been wrong since 1993 and the code is what players
// experienced.
func TestThePositionMultiplierIsIntegerDivision(t *testing.T) {
	oracle := buildFightOracle(t)

	want := map[Position]int32{
		PosMortallyWounded: 3,
		PosIncapacitated:   2,
		PosStunned:         2,
		PosSleeping:        2,
		PosResting:         1,
		PosSitting:         1,
		PosFighting:        1,
		PosStanding:        1,
	}

	for pos, multiplier := range want {
		fromC := runFightOracle(t, oracle, "multiplier", itoa(int32(pos)))
		if fromC != multiplier {
			t.Errorf("the C's multiplier at %s is %d, this test expected %d", pos, fromC, multiplier)
		}
	}

	// And the Go side produces the same, through a real swing: a victim who
	// cannot defend themselves takes the multiplied figure.
	r := newRNG()
	attacker := &PlayerRecord{
		Class: ClassWarrior, Level: 30,
		Abilities: Abilities{Strength: 18, Intelligence: 25, Wisdom: 25},
		Points:    Points{HitRoll: 50, DamRoll: 10},
	}
	victim := &PlayerRecord{Class: ClassWarrior, Level: 1, Points: Points{Armor: 100}}

	for pos, multiplier := range want {
		var seenMax int32
		for i := 0; i < 400; i++ {
			swing := Attack(attacker, victim, fighter{pos: PosStanding}, fighter{pos: pos}, r)
			if swing.Hit && swing.Damage > seenMax {
				seenMax = swing.Damage
			}
		}
		// Damage is strength todam (2 at 18) + damroll (10) + number(0, 2),
		// so the unmultiplied maximum is 14.
		if want := 14 * multiplier; seenMax != want {
			t.Errorf("at %s the best hit was %d, want %d (multiplier %d)",
				pos, seenMax, want, multiplier)
		}
	}
}

// TestANaturalTwentyAlwaysHitsAndAOneAlwaysMisses, whatever the numbers say.
func TestANaturalTwentyAlwaysHitsAndAOneAlwaysMisses(t *testing.T) {
	r := newRNG()

	// Hopeless attacker against excellent armour: only the natural 20 lands.
	hopeless := &PlayerRecord{Class: ClassMagicUser, Level: 1,
		Abilities: Abilities{Strength: 3, Intelligence: 3, Wisdom: 3}}
	armoured := &PlayerRecord{Class: ClassWarrior, Level: 30,
		Abilities: Abilities{Dexterity: 25}, Points: Points{Armor: -100}}

	var hits, twenties int
	for i := 0; i < 4000; i++ {
		swing := Attack(hopeless, armoured, fighter{pos: PosStanding}, fighter{pos: PosStanding}, r)
		if swing.Roll == 20 {
			twenties++
			if !swing.Hit {
				t.Fatal("a natural 20 missed")
			}
		}
		if swing.Hit {
			hits++
		}
	}
	if twenties == 0 {
		t.Fatal("no natural 20 was rolled in 4000 swings")
	}
	if hits != twenties {
		t.Errorf("%d hits from %d natural 20s; nothing else should have landed", hits, twenties)
	}

	// Unmissable attacker against no armour: only the natural 1 misses.
	deadly := &PlayerRecord{Class: ClassWarrior, Level: 34,
		Abilities: Abilities{Strength: 18, StrengthPercentile: 100, Intelligence: 25, Wisdom: 25},
		Points:    Points{HitRoll: 100}}
	naked := &PlayerRecord{Class: ClassMagicUser, Level: 1, Points: Points{Armor: 100}}

	var misses, ones int
	for i := 0; i < 4000; i++ {
		swing := Attack(deadly, naked, fighter{pos: PosStanding}, fighter{pos: PosStanding}, r)
		if swing.Roll == 1 {
			ones++
			if swing.Hit {
				t.Fatal("a natural 1 hit")
			}
		}
		if !swing.Hit {
			misses++
		}
	}
	if ones == 0 {
		t.Fatal("no natural 1 was rolled in 4000 swings")
	}
	if misses != ones {
		t.Errorf("%d misses from %d natural 1s; nothing else should have missed", misses, ones)
	}
}

// TestASleepingVictimIsAlwaysHit, which beats even a natural 1.
func TestASleepingVictimIsAlwaysHit(t *testing.T) {
	r := newRNG()

	hopeless := &PlayerRecord{Class: ClassMagicUser, Level: 1,
		Abilities: Abilities{Strength: 3, Intelligence: 3, Wisdom: 3}}
	armoured := &PlayerRecord{Class: ClassWarrior, Level: 30,
		Abilities: Abilities{Dexterity: 25}, Points: Points{Armor: -100}}

	for i := 0; i < 2000; i++ {
		swing := Attack(hopeless, armoured, fighter{pos: PosStanding}, fighter{pos: PosSleeping}, r)
		if !swing.Hit {
			t.Fatalf("a sleeping victim was missed on a roll of %d", swing.Roll)
		}
	}
}

// TestAWieldedWeaponReplacesBareHands, using its own damage dice.
func TestAWieldedWeaponReplacesBareHands(t *testing.T) {
	r := newRNG()
	l := objectWorld()

	attacker := &PlayerRecord{Class: ClassWarrior, Level: 30,
		Abilities: Abilities{Strength: 12, Intelligence: 25, Wisdom: 25},
		Points:    Points{HitRoll: 100}}
	victim := &PlayerRecord{Class: ClassMagicUser, Level: 1, Points: Points{Armor: 100}}

	// Bare handed: strength 12 gives no bonus, so damage is number(0, 2),
	// floored at 1.
	var bareMax int32
	for i := 0; i < 500; i++ {
		if s := Attack(attacker, victim, fighter{pos: PosStanding}, fighter{pos: PosStanding}, r); s.Hit && s.Damage > bareMax {
			bareMax = s.Damage
		}
	}
	if bareMax != 2 {
		t.Errorf("bare-handed maximum is %d, want 2", bareMax)
	}

	// A weapon doing 3d6: 3 to 18.
	sword := l.NewObject(100)
	sword.Values[1] = 3
	sword.Values[2] = 6
	wielding := fighter{pos: PosStanding, wielded: sword}

	var low, high int32 = 100, 0
	for i := 0; i < 2000; i++ {
		if s := Attack(attacker, victim, wielding, fighter{pos: PosStanding}, r); s.Hit {
			low = min(low, s.Damage)
			high = max(high, s.Damage)
		}
	}
	if low != 3 || high != 18 {
		t.Errorf("a 3d6 weapon did %d..%d, want 3..18", low, high)
	}
}

// TestAMobileUsesItsOwnDamageDice rather than a player's bare hands.
func TestAMobileUsesItsOwnDamageDice(t *testing.T) {
	r := newRNG()

	mob := &PlayerRecord{Class: ClassWarrior, Level: 10,
		DamageDice: 2, DamageSize: 8,
		Abilities: Abilities{Strength: 12, Intelligence: 25, Wisdom: 25},
		Points:    Points{HitRoll: 100}}
	victim := &PlayerRecord{Class: ClassMagicUser, Level: 1, Points: Points{Armor: 100}}

	var low, high int32 = 100, 0
	for i := 0; i < 2000; i++ {
		if s := Attack(mob, victim, fighter{npc: true, pos: PosStanding}, fighter{pos: PosStanding}, r); s.Hit {
			low = min(low, s.Damage)
			high = max(high, s.Damage)
		}
	}
	if low != 2 || high != 16 {
		t.Errorf("a 2d8 mobile did %d..%d, want 2..16", low, high)
	}
}

// TestEveryHitDoesAtLeastOne, however weak the attacker.
func TestEveryHitDoesAtLeastOne(t *testing.T) {
	r := newRNG()

	// Strength 3 is -4 to damage, so the raw figure is negative.
	feeble := &PlayerRecord{Class: ClassMagicUser, Level: 1,
		Abilities: Abilities{Strength: 3, Intelligence: 25, Wisdom: 25},
		Points:    Points{HitRoll: 100, DamRoll: -10}}
	victim := &PlayerRecord{Class: ClassMagicUser, Level: 1, Points: Points{Armor: 100}}

	for i := 0; i < 1000; i++ {
		if s := Attack(feeble, victim, fighter{pos: PosStanding}, fighter{pos: PosStanding}, r); s.Hit && s.Damage < 1 {
			t.Fatalf("a hit did %d damage", s.Damage)
		}
	}
}

// TestAttackingAnImmortalDoublesTheDamage.
//
// This is a local deviation from stock CircleMUD and a startling one: the
// comment above it still reads "You can't damage an immortal!", which is what
// `dam = 0` did before somebody changed the line. Reproduced because it is
// what players fought against.
func TestAttackingAnImmortalDoublesTheDamage(t *testing.T) {
	immortal := &PlayerRecord{Class: ClassWarrior, Level: LevelImplementor}
	mortal := &PlayerRecord{Class: ClassWarrior, Level: 30}

	if got := ApplyDamage(50, immortal, fighter{pos: PosStanding}); got != 100 {
		t.Errorf("50 damage to an implementor became %d, want 100", got)
	}
	if got := ApplyDamage(50, mortal, fighter{pos: PosStanding}); got != 50 {
		t.Errorf("50 damage to a level-30 mortal became %d, want 50", got)
	}
	// A mobile of immortal level is not covered by the check.
	if got := ApplyDamage(50, immortal, fighter{npc: true, pos: PosStanding}); got != 50 {
		t.Errorf("50 damage to an immortal-level mobile became %d, want 50", got)
	}
}

func TestSanctuaryHalvesDamage(t *testing.T) {
	victim := &PlayerRecord{Class: ClassWarrior, Level: 10}
	sanct := fighter{pos: PosStanding, sanctuary: true}

	if got := ApplyDamage(50, victim, sanct); got != 25 {
		t.Errorf("50 damage under sanctuary became %d, want 25", got)
	}
	// The C's guard is `dam >= 2`, so one point of damage is not halved to
	// zero.
	if got := ApplyDamage(1, victim, sanct); got != 1 {
		t.Errorf("1 damage under sanctuary became %d, want 1", got)
	}
	if got := ApplyDamage(3, victim, sanct); got != 1 {
		t.Errorf("3 damage under sanctuary became %d, want 1", got)
	}
}

func TestDamageIsClampedAtBothEnds(t *testing.T) {
	victim := &PlayerRecord{Class: ClassWarrior, Level: 10}
	plain := fighter{pos: PosStanding}

	if got := ApplyDamage(5000, victim, plain); got != maxDamagePerBlow {
		t.Errorf("5000 damage became %d, want %d", got, maxDamagePerBlow)
	}
	if got := ApplyDamage(-50, victim, plain); got != 0 {
		t.Errorf("negative damage became %d, want 0", got)
	}
}

func buildFightOracle(t *testing.T) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("gcc not found in CI; the combat comparison must run")
		}
		t.Skip("gcc not found; skipping the combat comparison")
	}

	src := filepath.Join(repoRoot(t), "reference", "tools", "fightoracle.c")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("%s not found: %v", src, err)
	}

	bin := filepath.Join(t.TempDir(), "fightoracle")
	build := exec.Command(gcc, "-O2", "-Wall", "-Werror", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compiling the oracle: %v\n%s", err, out)
	}
	return bin
}

func runFightOracle(t *testing.T, bin string, args ...string) int32 {
	t.Helper()

	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		t.Fatalf("running the oracle with %v: %v", args, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("unparseable oracle output %q: %v", out, err)
	}
	return int32(n)
}
