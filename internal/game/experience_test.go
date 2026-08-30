// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const classCSourceForExp = "../../reference/moderncserver/src/class.c"

// TestLevelExperienceMatchesTheCSource re-parses level_exp out of class.c and
// compares every entry, the same arrangement the title and ability tables
// use. 160 numbers, and one of them wrong is a level that arrives at the
// wrong time forever.
func TestLevelExperienceMatchesTheCSource(t *testing.T) {
	src, err := os.ReadFile(classCSourceForExp)
	if err != nil {
		t.Fatalf("reading the C source the table came from: %v", err)
	}

	want := parseLevelExp(t, string(src))
	if len(want) != 5 {
		t.Fatalf("parsed %d classes, want 5", len(want))
	}

	for class, byLevel := range want {
		if len(byLevel) != 32 {
			t.Errorf("class %d has %d levels, want 32 (0 through 31)", class, len(byLevel))
		}
		for level, exp := range byLevel {
			if got := LevelExperience(class, level); got != exp {
				t.Errorf("LevelExperience(%d, %d) = %d, want %d", class, level, got, exp)
			}
		}
	}
}

func parseLevelExp(t *testing.T, src string) map[Class]map[int32]int32 {
	t.Helper()

	start := strings.Index(src, "int level_exp(int chclass, int level)\n{")
	if start < 0 {
		t.Fatal("could not find level_exp in the C source")
	}
	body := src[start:]
	if end := strings.Index(body, "\n}\n"); end >= 0 {
		body = body[:end]
	}

	classes := map[string]Class{
		"CLASS_MAGIC_USER": ClassMagicUser,
		"CLASS_CLERIC":     ClassCleric,
		"CLASS_THIEF":      ClassThief,
		"CLASS_WARRIOR":    ClassWarrior,
		"CLASS_PALADIN":    ClassPaladin,
	}

	classCase := regexp.MustCompile(`^\s*case (CLASS_\w+):\s*$`)
	levelCase := regexp.MustCompile(`^\s*case\s+(\w+):\s*return (\d+);\s*$`)

	out := map[Class]map[int32]int32{}
	current := Class(-1)

	for _, line := range strings.Split(body, "\n") {
		if m := classCase.FindStringSubmatch(line); m != nil {
			class, ok := classes[m[1]]
			if !ok {
				t.Fatalf("unknown class %s", m[1])
			}
			current = class
			out[current] = map[int32]int32{}
			continue
		}
		if current < 0 {
			continue
		}
		if m := levelCase.FindStringSubmatch(line); m != nil {
			level := int32(LevelImmortal)
			if m[1] != "LVL_IMMORT" {
				n, err := strconv.Atoi(m[1])
				if err != nil {
					t.Fatalf("unparseable level %q", m[1])
				}
				level = int32(n)
			}
			exp, err := strconv.Atoi(m[2])
			if err != nil {
				t.Fatalf("unparseable experience %q", m[2])
			}
			out[current][level] = int32(exp)
		}
	}
	return out
}

// TestExperienceTablesRiseMonotonically. A table that went backwards
// anywhere would let a character level and immediately un-level, and a
// row-by-row comparison against a C table with the same mistake in it would
// not notice.
func TestExperienceTablesRiseMonotonically(t *testing.T) {
	for _, class := range []Class{ClassMagicUser, ClassCleric, ClassThief, ClassWarrior, ClassPaladin} {
		previous := int32(-1)
		for level := int32(0); level <= LevelImplementor; level++ {
			exp := LevelExperience(class, level)
			if exp <= previous {
				t.Errorf("class %d: level %d needs %d, level %d needed %d",
					class, level, exp, level-1, previous)
			}
			previous = exp
		}
	}
}

// TestImmortalLevelsArePricedOffTheCeiling, which is what lets immortal
// levels be added without touching a table.
func TestImmortalLevelsArePricedOffTheCeiling(t *testing.T) {
	for _, level := range []int32{LevelGod, LevelGreaterGod, LevelImplementor} {
		want := expMax - (LevelImplementor-level)*1000
		if got := LevelExperience(ClassWarrior, level); got != want {
			t.Errorf("LevelExperience(warrior, %d) = %d, want %d", level, got, want)
		}
	}
	if LevelExperience(ClassWarrior, LevelImplementor) != expMax {
		t.Error("an implementor should need exactly EXP_MAX")
	}
}

// TestAnOutOfRangeLevelIsUnreachable rather than free. The C logs a SYSERR
// and returns zero, which would make every "have I earned this level?"
// comparison succeed.
func TestAnOutOfRangeLevelIsUnreachable(t *testing.T) {
	for _, level := range []int32{-1, LevelImplementor + 1, 1000} {
		if got := LevelExperience(ClassWarrior, level); got != expMax {
			t.Errorf("LevelExperience(warrior, %d) = %d, want it to be unreachable", level, got)
		}
	}
	if got := LevelExperience(99, 5); got != expMax {
		t.Errorf("an unknown class got %d, want it to be unreachable", got)
	}
}

func TestGainingALevel(t *testing.T) {
	r := newRNG()
	rec := &PlayerRecord{Name: "Welmar", Class: ClassWarrior, Sex: SexFemale, Level: 1}
	Start(rec, r)

	need := LevelExperience(ClassWarrior, 2)
	before := rec.Points.MaxHit

	// The local cap allows only a tenth of the band per kill, so however big
	// the kill, it takes at least eleven of them — ten tenths is short by
	// exactly the experience do_start already granted.
	var kills int
	for kills = 1; kills <= 30; kills++ {
		out := GainExperience(rec, need, r)
		if out.Levels > 0 {
			if out.Levels != 1 {
				t.Errorf("gained %d levels at once, want 1", out.Levels)
			}
			break
		}
	}

	if rec.Level != 2 {
		t.Fatalf("level is %d after %d kills of %d experience each, want 2", rec.Level, kills, need)
	}
	if kills < 11 {
		t.Errorf("levelled after %d kills; the tenth-of-a-band cap should need at least 11", kills)
	}
	if rec.Points.MaxHit <= before {
		t.Errorf("max hit is %d, was %d — advance_level did not run", rec.Points.MaxHit, before)
	}
	if rec.Title != Title(ClassWarrior, 2, SexFemale) {
		t.Errorf("title is %q, want it reset for the new level", rec.Title)
	}
}

// TestTheLocalPerKillCap is the Disgracelands rule: no single kill awards
// more than a tenth of the band to the next level, so a level-one character
// cannot be dragged up by somebody else's kill.
func TestTheLocalPerKillCap(t *testing.T) {
	r := newRNG()
	rec := &PlayerRecord{Class: ClassWarrior, Level: 1}
	Start(rec, r)

	band := LevelExperience(ClassWarrior, 2) - LevelExperience(ClassWarrior, 1)

	out := GainExperience(rec, 1_000_000, r)
	if !out.Capped {
		t.Error("an enormous award was not reported as capped")
	}
	if out.Applied != band/10 {
		t.Errorf("awarded %d, want a tenth of the %d band", out.Applied, band)
	}
	if rec.Level != 1 {
		t.Errorf("one kill took a character to level %d", rec.Level)
	}

	// And an award below the limit is untouched.
	small := GainExperience(rec, 10, r)
	if small.Capped || small.Applied != 10 {
		t.Errorf("a small award was altered: %+v", small)
	}
}

// TestTheStockPerKillCapAlsoApplies, at high level where a tenth of the band
// is the larger of the two limits.
func TestTheStockPerKillCapAlsoApplies(t *testing.T) {
	r := newRNG()
	rec := &PlayerRecord{Class: ClassWarrior, Level: 25}

	band := LevelExperience(ClassWarrior, 26) - LevelExperience(ClassWarrior, 25)
	if band/10 <= MaxExpGainPerKill {
		t.Skipf("at level 25 the local cap (%d) still bites first", band/10)
	}

	out := GainExperience(rec, 10_000_000, r)
	if out.Applied != MaxExpGainPerKill {
		t.Errorf("awarded %d, want the stock cap of %d", out.Applied, MaxExpGainPerKill)
	}
}

// TestExperienceLossIsCappedAndFloored.
func TestExperienceLossIsCappedAndFloored(t *testing.T) {
	r := newRNG()

	rec := &PlayerRecord{Class: ClassWarrior, Level: 10, Points: Points{Exp: 1_000_000}}
	GainExperience(rec, -10_000_000, r)
	if want := int32(1_000_000 - MaxExpLossPerDeath); rec.Points.Exp != want {
		t.Errorf("experience is %d, want %d after one capped loss", rec.Points.Exp, want)
	}

	// It never goes negative.
	poor := &PlayerRecord{Class: ClassWarrior, Level: 10, Points: Points{Exp: 100}}
	GainExperience(poor, -MaxExpLossPerDeath, r)
	if poor.Points.Exp != 0 {
		t.Errorf("experience is %d, want 0", poor.Points.Exp)
	}

	// Losing experience does not cost a level: the C only ever adds levels
	// here, and de-levelling is something an implementor does deliberately.
	if poor.Level != 10 {
		t.Errorf("losing experience dropped the character to level %d", poor.Level)
	}
}

// TestMortalProgressStopsBelowImmortal. The C's bound is
// `>= LVL_IMMORT - 1`, so a level-30 character gains nothing further.
func TestMortalProgressStopsBelowImmortal(t *testing.T) {
	r := newRNG()

	for _, level := range []int32{LevelImmortal - 1, LevelImmortal, LevelImplementor} {
		rec := &PlayerRecord{Class: ClassWarrior, Level: level, Points: Points{Exp: 1000}}
		out := GainExperience(rec, 1_000_000, r)
		if out.Applied != 0 || rec.Points.Exp != 1000 {
			t.Errorf("a level %d character gained %d experience", level, out.Applied)
		}
	}

	// And a character who has not started yet.
	unborn := &PlayerRecord{Class: ClassWarrior, Level: 0}
	if out := GainExperience(unborn, 5000, r); out.Applied != 0 {
		t.Errorf("a level-zero character gained %d experience", out.Applied)
	}
}

// TestSeveralLevelsAtOnce: the C loops rather than stepping once, which is
// how "You rise 3 levels!" happens.
func TestSeveralLevelsAtOnce(t *testing.T) {
	r := newRNG()
	rec := &PlayerRecord{Class: ClassWarrior, Level: 1}
	Start(rec, r)

	// Bypass the per-kill caps by setting the experience directly, which is
	// what an implementor's `set exp` does.
	rec.Points.Exp = LevelExperience(ClassWarrior, 5) - 1
	out := GainExperience(rec, 1, r)

	if out.Levels != 4 {
		t.Errorf("rose %d levels, want 4", out.Levels)
	}
	if rec.Level != 5 {
		t.Errorf("level is %d, want 5", rec.Level)
	}
}

func TestExpToLevel(t *testing.T) {
	rec := &PlayerRecord{Class: ClassThief, Level: 3, Points: Points{Exp: 0}}
	if got, want := ExpToLevel(rec), LevelExperience(ClassThief, 4); got != want {
		t.Errorf("ExpToLevel = %d, want %d", got, want)
	}

	// Never negative, for a character carrying more than they need.
	rec.Points.Exp = LevelExperience(ClassThief, 10)
	if got := ExpToLevel(rec); got != 0 {
		t.Errorf("ExpToLevel = %d, want 0", got)
	}
}
