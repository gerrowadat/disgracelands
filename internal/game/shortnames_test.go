// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestClassShortNamesMatchTheCSource re-parses pc_class_snames.
//
// A third table of class names, and the one `remort` matches a god's typing
// against — so a wrong entry means a class nobody can remort into. Checked
// against the C rather than asserted, like the other name tables.
func TestClassShortNamesMatchTheCSource(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "reference", "moderncserver", "src", "class.c"))
	if err != nil {
		t.Fatalf("reading class.c: %v", err)
	}

	block := regexp.MustCompile(`(?s)pc_class_snames\[\] = \{(.*?)\};`).FindSubmatch(src)
	if block == nil {
		t.Fatal("pc_class_snames not found in class.c")
	}

	var want []string
	for _, m := range regexp.MustCompile(`"([^"\\]*)"`).FindAllStringSubmatch(string(block[1]), -1) {
		if m[1] == "" {
			continue // the "\n" terminator, whose escape the pattern drops
		}
		want = append(want, m[1])
	}

	if len(want) != len(ClassShortNameOrder) {
		t.Fatalf("the C has %d short names, ClassShortNameOrder has %d: %v",
			len(want), len(ClassShortNameOrder), want)
	}
	for i, class := range ClassShortNameOrder {
		if got := ClassShortNames[class]; got != want[i] {
			t.Errorf("class %d: short name %q, the C says %q", class, got, want[i])
		}
	}
}

// TestParseShortClassName, including that it is a whole name rather than a
// prefix — the C compares with strcasecmp, not a prefix match.
func TestParseShortClassName(t *testing.T) {
	for word, want := range map[string]Class{
		"mage": ClassMagicUser, "MAGE": ClassMagicUser,
		"cleric": ClassCleric, "thief": ClassThief,
		"warrior": ClassWarrior, "Paladin": ClassPaladin,
	} {
		got, ok := ParseShortClassName(word)
		if !ok || got != want {
			t.Errorf("ParseShortClassName(%q) = (%d, %v), want (%d, true)", word, got, ok, want)
		}
	}
	for _, word := range []string{"", "mag", "magic user", "m", "wizard", "war"} {
		if _, ok := ParseShortClassName(word); ok {
			t.Errorf("ParseShortClassName(%q) matched; it is a whole name", word)
		}
	}
}

// TestRemortMask. Paladin has a mask in the C's table and no IS_ macro reads
// it, which is why remorting into paladin lists nothing new.
func TestRemortMask(t *testing.T) {
	for _, class := range []Class{ClassMagicUser, ClassCleric, ClassThief, ClassWarrior} {
		if got, want := RemortMask(class), NewSet(class); got != want {
			t.Errorf("RemortMask(%d) = %v, want %v", class, got.Members(), want.Members())
		}
	}
	if RemortMask(ClassPaladin).Empty() {
		t.Error("paladin has a mask in the C's table and should have one here")
	}
	if !RemortMask(99).Empty() {
		t.Error("an unknown class has no mask")
	}
}

// TestRemortVectorRoundTrips through the accessors internal/session uses.
func TestRemortVectorRoundTrips(t *testing.T) {
	rec := &PlayerRecord{}
	want := NewSet(ClassThief, ClassWarrior)
	SetRemortClasses(rec, want)
	if got := RemortClassesOf(rec); got != want {
		t.Errorf("round trip gave %v", got.Members())
	}
	// The stored bits are still the C's: thief is 4 and warrior 8.
	if got := rec.RemortVector.Raw(); got != 4|8 {
		t.Errorf("the stored vector is %#x, want %#x", got, 4|8)
	}
}

// TestARemortedCharacterCountsAsBothClasses is the point of the whole
// mechanic: the IS_<CLASS> macros read the vector, not the class field.
func TestARemortedCharacterCountsAsBothClasses(t *testing.T) {
	rec := &PlayerRecord{Class: ClassWarrior}
	SetRemortClasses(rec, NewSet(ClassWarrior))

	if IsThief(rec) {
		t.Error("a plain warrior counted as a thief")
	}
	SetRemortClasses(rec, RemortClassesOf(rec).With(ClassThief))
	if !IsThief(rec) || !IsWarrior(rec) {
		t.Error("a warrior who remorted through thief should count as both")
	}
}
