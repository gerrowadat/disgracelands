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

// classCSource is the C the title tables were transcribed from. It is in the
// repository, so a missing file is a broken checkout and a failure, not a
// reason to skip.
const classCSource = "../../reference/moderncserver/src/class.c"

// TestTitlesMatchTheCSource re-parses title_male and title_female out of
// class.c and compares every entry against the transcribed tables.
//
// Three hundred-odd strings copied by hand would be three hundred-odd chances
// to introduce a typo nobody notices until a player asks why their title
// changed. Parsing the original instead means the tables are checked against
// their source on every run.
func TestTitlesMatchTheCSource(t *testing.T) {
	src, err := os.ReadFile(classCSource)
	if err != nil {
		t.Fatalf("reading the C source the tables came from: %v", err)
	}
	text := string(src)

	for _, tc := range []struct {
		function string
		sex      Sex
	}{
		{"title_male", SexMale},
		{"title_female", SexFemale},
	} {
		want := parseTitleFunction(t, text, tc.function)
		if len(want) != 5 {
			t.Fatalf("%s: parsed %d classes, want 5 — the parser has lost track of the source",
				tc.function, len(want))
		}

		for class, byLevel := range want {
			if len(byLevel) < 20 {
				t.Errorf("%s: class %d parsed only %d levels", tc.function, class, len(byLevel))
			}
			for level, title := range byLevel {
				if got := Title(class, level, tc.sex); got != title {
					t.Errorf("Title(%d, %d, %d) = %q, want %q (%s)",
						class, level, tc.sex, got, title, tc.function)
				}
			}
		}
	}
}

// parseTitleFunction pulls the case labels out of one of the two title
// functions, returning class -> level -> title. Levels covered by the class's
// `default:` are filled in for every level the switch does not name, which is
// how the uneven tables get checked as well as the even ones.
func parseTitleFunction(t *testing.T, src, function string) map[Class]map[int32]string {
	t.Helper()

	start := strings.Index(src, "const char *"+function+"(int chclass, int level)\n{")
	if start < 0 {
		t.Fatalf("could not find %s in %s", function, classCSource)
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
	levels := map[string]int32{
		"LVL_IMMORT": LevelImmortal,
		"LVL_GOD":    LevelGod,
		"LVL_GRGOD":  LevelGreaterGod,
		"LVL_IMPL":   LevelImplementor,
	}

	var (
		classCase   = regexp.MustCompile(`^\s*case (CLASS_\w+):\s*$`)
		levelCase   = regexp.MustCompile(`^\s*case\s+(\w+):\s*return "(.*)";\s*$`)
		defaultCase = regexp.MustCompile(`^\s*default:\s*return "(.*)";\s*$`)
	)

	out := map[Class]map[int32]string{}
	defaults := map[Class]string{}
	current := Class(-1)

	for _, line := range strings.Split(body, "\n") {
		if m := classCase.FindStringSubmatch(line); m != nil {
			class, ok := classes[m[1]]
			if !ok {
				t.Fatalf("%s: unknown class %s", function, m[1])
			}
			current = class
			out[current] = map[int32]string{}
			continue
		}
		if current < 0 {
			continue
		}
		if m := levelCase.FindStringSubmatch(line); m != nil {
			level, ok := levels[m[1]]
			if !ok {
				n, err := strconv.Atoi(m[1])
				if err != nil {
					t.Fatalf("%s: unparseable case label %q", function, m[1])
				}
				level = int32(n)
			}
			out[current][level] = m[2]
			continue
		}
		if m := defaultCase.FindStringSubmatch(line); m != nil {
			defaults[current] = m[1]
		}
	}

	// Fill in the levels the switch does not name, so the fallback is checked
	// too — including the 21-to-30 gap in the cleric, thief and warrior lists.
	for class, byLevel := range out {
		fallback, ok := defaults[class]
		if !ok {
			t.Errorf("%s: class %d has no default title", function, class)
			continue
		}
		for level := int32(1); level < LevelImplementor; level++ {
			if _, named := byLevel[level]; !named {
				byLevel[level] = fallback
			}
		}
	}
	return out
}

// TestImplementorsAndOddLevelsBypassTheClassTables covers the checks the C
// makes before it ever looks at the class.
func TestImplementorsAndOddLevelsBypassTheClassTables(t *testing.T) {
	for _, tc := range []struct {
		level int32
		sex   Sex
		want  string
	}{
		{LevelImplementor, SexMale, "the Implementor"},
		{LevelImplementor, SexFemale, "the Implementress"},
		{0, SexMale, "the Man"},
		{0, SexFemale, "the Woman"},
		{-1, SexMale, "the Man"},
		{LevelImplementor + 1, SexFemale, "the Woman"},
	} {
		for _, class := range []Class{ClassMagicUser, ClassCleric, ClassThief, ClassWarrior, ClassPaladin} {
			if got := Title(class, tc.level, tc.sex); got != tc.want {
				t.Errorf("Title(%d, %d, %d) = %q, want %q", class, tc.level, tc.sex, got, tc.want)
			}
		}
	}

	// A sex the game does not otherwise use falls to the male table, as the C
	// does — set_title only special-cases SEX_FEMALE.
	if got := Title(ClassWarrior, 1, SexNeutral); got != Title(ClassWarrior, 1, SexMale) {
		t.Errorf("a neutral character got %q, want the male table's title", got)
	}

	// And a class with no table at all.
	if got := Title(99, 1, SexMale); got != "the Classless" {
		t.Errorf("Title(99, 1, male) = %q, want %q", got, "the Classless")
	}
}
