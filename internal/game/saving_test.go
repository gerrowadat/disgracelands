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

// TestSavingThrowsMatchTheCSource re-parses saving_throws out of class.c and
// compares all 1,125 entries.
//
// Five classes by five save types by forty-odd levels, written in the C as
// nested switch statements over thirteen hundred lines. Transcribed by hand
// that is a table with a mistake in it.
func TestSavingThrowsMatchTheCSource(t *testing.T) {
	src, err := os.ReadFile(classCSource)
	if err != nil {
		t.Fatalf("reading class.c: %v", err)
	}

	lines := strings.Split(string(src), "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "byte saving_throws(int class_num") &&
			!strings.HasSuffix(strings.TrimSpace(line), ";") {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatal("could not find the saving_throws definition")
	}

	// Walk to the matching close brace.
	depth, started, end := 0, false, -1
	for i := start; i < len(lines) && end < 0; i++ {
		for _, ch := range lines[i] {
			switch ch {
			case '{':
				depth++
				started = true
			case '}':
				depth--
				if started && depth == 0 {
					end = i
				}
			}
			if end >= 0 {
				break
			}
		}
	}
	if end < 0 {
		t.Fatal("saving_throws is not terminated")
	}

	body := stripCComments(strings.Join(lines[start:end+1], "\n"))

	classes := map[string]Class{
		"CLASS_MAGIC_USER": ClassMagicUser,
		"CLASS_CLERIC":     ClassCleric,
		"CLASS_THIEF":      ClassThief,
		"CLASS_WARRIOR":    ClassWarrior,
		"CLASS_PALADIN":    ClassPaladin,
	}
	saves := map[string]SaveType{
		"SAVING_PARA": SaveParalyse, "SAVING_ROD": SaveRod,
		"SAVING_PETRI": SavePetrify, "SAVING_BREATH": SaveBreath,
		"SAVING_SPELL": SaveSpell,
	}

	var (
		classCase = regexp.MustCompile(`^\s*case (CLASS_\w+):\s*$`)
		saveCase  = regexp.MustCompile(`^\s*case (SAVING_\w+):`)
		levelCase = regexp.MustCompile(`^\s*case\s+(\d+):\s*return\s+(\d+);\s*$`)
		breakLine = regexp.MustCompile(`^\s*break;\s*$`)
	)

	var current []Class
	save := SaveType(-1)
	checked := 0

	for _, line := range strings.Split(body, "\n") {
		if m := classCase.FindStringSubmatch(line); m != nil {
			// Warrior and paladin stack their cases in the C, so the class
			// list accumulates until a level switch begins.
			current = append(current, classes[m[1]])
			save = -1
			continue
		}
		if m := saveCase.FindStringSubmatch(line); m != nil {
			save = saves[m[1]]
			continue
		}
		if m := levelCase.FindStringSubmatch(line); m != nil && len(current) > 0 && save >= 0 {
			level, _ := strconv.Atoi(m[1])
			want, _ := strconv.Atoi(m[2])
			for _, class := range current {
				if got := SavingThrow(class, save, int32(level)); got != int32(want) {
					t.Errorf("saving_throws(class %d, save %d, level %d) = %d, the C gives %d",
						class, save, level, got, want)
				}
				checked++
			}
			continue
		}
		if breakLine.MatchString(line) && save == SaveSpell {
			current = nil
			save = -1
		}
	}

	if checked < 1000 {
		t.Fatalf("checked only %d saving throws", checked)
	}
	t.Logf("checked %d saving throws against the C", checked)
}

// TestLowerIsBetter, which the whole table reads backwards on.
func TestLowerIsBetter(t *testing.T) {
	low := SavingThrow(ClassMagicUser, SaveSpell, 1)
	high := SavingThrow(ClassMagicUser, SaveSpell, 30)
	if high >= low {
		t.Errorf("a level-30 mage's spell save is %d and a level-1's is %d; lower should be better",
			high, low)
	}
}

// TestAMobileUsesTheWarriorTables, "according to some book" as the C's
// comment has it.
func TestAMobileUsesTheWarriorTables(t *testing.T) {
	rec := &PlayerRecord{Class: ClassMagicUser, Level: 10}

	r := newRNG()
	// Not a distributional test: just that the class used is the warrior's.
	if got, want := SavingThrow(99, SaveSpell, 10), SavingThrow(ClassWarrior, SaveSpell, 10); got != want {
		t.Errorf("an unknown class saves on %d, want the warrior's %d", got, want)
	}

	// And a mobile with a mage's class still rolls against the warrior table,
	// which MakesSavingThrow decides by the isNPC flag.
	saves := 0
	for i := 0; i < 2000; i++ {
		if MakesSavingThrow(rec, true, SaveSpell, 0, r) {
			saves++
		}
	}
	if saves == 0 || saves == 2000 {
		t.Errorf("a mobile saved %d times in 2000, which is not a roll", saves)
	}
}

// TestABetterBonusIsANegativeOne. The C's comment apologises for this: the
// modifier is applied to the target, so negative is an improvement.
func TestABetterBonusIsANegativeOne(t *testing.T) {
	r := newRNG()
	plain := &PlayerRecord{Class: ClassWarrior, Level: 10}
	blessed := &PlayerRecord{Class: ClassWarrior, Level: 10}
	blessed.SavingThrows[SaveSpell] = -50

	count := func(rec *PlayerRecord) int {
		n := 0
		for i := 0; i < 4000; i++ {
			if MakesSavingThrow(rec, false, SaveSpell, 0, r) {
				n++
			}
		}
		return n
	}

	if count(blessed) <= count(plain) {
		t.Error("a negative saving-throw bonus did not make saves more likely")
	}
}

// TestAPerfectSaveIsNotAutomatic. `MAX(1, save)` means a target of zero
// still has to beat a roll of at least 1.
func TestAPerfectSaveIsNotAutomatic(t *testing.T) {
	r := newRNG()
	rec := &PlayerRecord{Class: ClassWarrior, Level: 10}
	rec.SavingThrows[SaveSpell] = -1000

	failures := 0
	for i := 0; i < 5000; i++ {
		if !MakesSavingThrow(rec, false, SaveSpell, 0, r) {
			failures++
		}
	}
	if failures == 0 {
		t.Error("a character with an enormous bonus never failed a save")
	}
}

func TestPracticeParameters(t *testing.T) {
	for _, tc := range []struct {
		class   Class
		learned int32
		noun    string
	}{
		{ClassMagicUser, 95, "spell"},
		{ClassCleric, 95, "spell"},
		{ClassThief, 85, "skill"},
		{ClassWarrior, 80, "skill"},
		{ClassPaladin, 90, "spell"},
	} {
		if got := LearnedLevel(tc.class); got != tc.learned {
			t.Errorf("class %d is learned at %d%%, want %d%%", tc.class, got, tc.learned)
		}
		if got := PracticeNoun(tc.class); got != tc.noun {
			t.Errorf("class %d calls them %ss, want %ss", tc.class, got, tc.noun)
		}
	}
}

// TestPracticeGainIsBoundedByTheClass. A thief gains at most 12 per session
// whatever their intelligence, which is why a thief practises so much more
// than a mage.
func TestPracticeGainIsBoundedByTheClass(t *testing.T) {
	clever := Abilities{Intelligence: 25}
	dim := Abilities{Intelligence: 3}

	mage := &PlayerRecord{Class: ClassMagicUser, Abilities: clever}
	if got := PracticeGain(mage); got != 60 {
		t.Errorf("a clever mage gains %d per practice, want int_app's 60", got)
	}
	dimMage := &PlayerRecord{Class: ClassMagicUser, Abilities: dim}
	if got := PracticeGain(dimMage); got != 25 {
		t.Errorf("a dim mage gains %d, want the class minimum of 25", got)
	}

	thief := &PlayerRecord{Class: ClassThief, Abilities: clever}
	if got := PracticeGain(thief); got != 12 {
		t.Errorf("a clever thief gains %d, want the class maximum of 12", got)
	}
}

func TestHowGood(t *testing.T) {
	for percent, want := range map[int32]string{
		-1: " error)", 0: " (not learned)", 1: " (awful)", 10: " (awful)",
		11: " (bad)", 20: " (bad)", 21: " (poor)", 40: " (poor)",
		41: " (average)", 55: " (average)", 56: " (fair)", 70: " (fair)",
		71: " (good)", 80: " (good)", 81: " (very good)", 85: " (very good)",
		86: " (superb)", 100: " (superb)",
	} {
		if got := HowGood(percent); got != want {
			t.Errorf("HowGood(%d) = %q, want %q", percent, got, want)
		}
	}
}
