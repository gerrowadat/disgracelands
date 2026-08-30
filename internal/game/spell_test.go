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

const (
	spellParserSource = "../../reference/moderncserver/src/spell_parser.c"
	spellsHeader      = "../../reference/moderncserver/src/spells.h"
	classSourceLevels = "../../reference/moderncserver/src/class.c"
)

// TestSpellNumbersMatchTheHeader.
//
// These are stored in every player record, so a wrong one silently writes a
// skill into another skill's slot. That is not hypothetical: three of the six
// skill numbers in this package were wrong until this test was written,
// because they had been read off a comment in do_start rather than out of
// spells.h.
func TestSpellNumbersMatchTheHeader(t *testing.T) {
	src, err := os.ReadFile(spellsHeader)
	if err != nil {
		t.Fatalf("reading spells.h: %v", err)
	}

	want := map[string]int32{}
	for _, m := range regexp.MustCompile(`#define\s+((?:SPELL|SKILL)_\w+)\s+(\d+)`).
		FindAllStringSubmatch(string(src), -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatalf("unparseable number for %s", m[1])
		}
		want[m[1]] = int32(n)
	}
	if len(want) < 70 {
		t.Fatalf("parsed only %d constants from spells.h", len(want))
	}

	// The ones this package names. Anything not listed is simply not ported
	// yet; anything listed must be right.
	for cName, got := range map[string]int32{
		"SKILL_BACKSTAB":       SkillBackstab,
		"SKILL_BASH":           SkillBash,
		"SKILL_HIDE":           SkillHide,
		"SKILL_KICK":           SkillKick,
		"SKILL_PICK_LOCK":      SkillPickLock,
		"SKILL_RESCUE":         SkillRescue,
		"SKILL_SNEAK":          SkillSneak,
		"SKILL_STEAL":          SkillSteal,
		"SKILL_TRACK":          SkillTrack,
		"SPELL_ARMOR":          SpellArmor,
		"SPELL_MAGIC_MISSILE":  SpellMagicMissile,
		"SPELL_CURE_LIGHT":     SpellCureLight,
		"SPELL_HEAL":           SpellHeal,
		"SPELL_SANCTUARY":      SpellSanctuary,
		"SPELL_POISON":         SpellPoison,
		"SPELL_SLEEP":          SpellSleep,
		"SPELL_HOLY_SHIELD":    SpellHolyShield,
		"SPELL_SILENCE":        SpellSilence,
		"SPELL_WORD_OF_RECALL": SpellWordOfRecall,
	} {
		if want[cName] != got {
			t.Errorf("%s is %d here, %d in spells.h", cName, got, want[cName])
		}
	}
}

// TestTheThiefStartingSkillsAreTheRightSlots. do_start names six skills, and
// three of them were being written into bash, kick and steal's slots.
func TestTheThiefStartingSkillsAreTheRightSlots(t *testing.T) {
	skills := StartingSkills(ClassThief)

	for _, tc := range []struct {
		number  int32
		percent int32
		name    string
	}{
		{SkillSneak, 10, "sneak"},
		{SkillHide, 5, "hide"},
		{SkillSteal, 15, "steal"},
		{SkillBackstab, 10, "backstab"},
		{SkillPickLock, 10, "pick lock"},
		{SkillTrack, 10, "track"},
	} {
		if got := skills[tc.number]; got != tc.percent {
			t.Errorf("%s (slot %d) is %d%%, want %d%%", tc.name, tc.number, got, tc.percent)
		}
	}

	// And nothing landed anywhere else.
	if len(skills) != 6 {
		t.Errorf("a new thief has %d skills, want 6: %v", len(skills), skills)
	}
	for _, wrong := range []int32{SkillBash, SkillKick, SkillRescue} {
		if _, ok := skills[wrong]; ok {
			t.Errorf("a new thief has a percentage in slot %d, which is not one of theirs", wrong)
		}
	}
}

// TestSpellTableMatchesTheCSource re-parses mag_assign_spells and compares
// every field of every entry.
func TestSpellTableMatchesTheCSource(t *testing.T) {
	want := parseSpello(t)
	if len(want) < 60 {
		t.Fatalf("parsed only %d spello calls", len(want))
	}

	for name, row := range want {
		number, ok := spellNumberFor(t, name)
		if !ok {
			continue
		}
		got, ok := spellTable[number]
		if !ok {
			t.Errorf("%s is in the C's table and not in this one", name)
			continue
		}

		if got.Name != row.name {
			t.Errorf("%s: name is %q, want %q", name, got.Name, row.name)
		}
		if got.ManaMax != row.manaMax || got.ManaMin != row.manaMin || got.ManaChange != row.manaChange {
			t.Errorf("%s: mana is %d/%d/%d, want %d/%d/%d", name,
				got.ManaMax, got.ManaMin, got.ManaChange,
				row.manaMax, row.manaMin, row.manaChange)
		}
		if int32(got.MinPosition) != row.minPosition {
			t.Errorf("%s: min position is %d, want %d", name, got.MinPosition, row.minPosition)
		}
		// Raw(), not a re-derivation: row.targets is the bit vector this
		// test parsed out of spells.h, so comparing the set's own bits is
		// what keeps the C the authority for these numbers rather than
		// the Go constants (docs/proposals/idiomatic-go.md §5).
		if got.Targets.Raw() != row.targets {
			t.Errorf("%s: targets are %d, want %d", name, got.Targets.Raw(), row.targets)
		}
		if got.Violent != row.violent {
			t.Errorf("%s: violent is %v, want %v", name, got.Violent, row.violent)
		}
		if got.Routines.Raw() != row.routines {
			t.Errorf("%s: routines are %d, want %d", name, got.Routines.Raw(), row.routines)
		}
	}
}

// TestSpellLevelsMatchTheCSource covers init_spell_levels, which is a
// separate list of seventy-eight assignments in another file.
func TestSpellLevelsMatchTheCSource(t *testing.T) {
	src, err := os.ReadFile(classSourceLevels)
	if err != nil {
		t.Fatalf("reading class.c: %v", err)
	}

	classes := map[string]int32{
		"CLASS_MAGIC_USER": ClassMagicUser,
		"CLASS_CLERIC":     ClassCleric,
		"CLASS_THIEF":      ClassThief,
		"CLASS_WARRIOR":    ClassWarrior,
		"CLASS_PALADIN":    ClassPaladin,
	}

	found := 0
	for _, m := range regexp.MustCompile(`spell_level\s*\(\s*(\w+)\s*,\s*(\w+)\s*,\s*(\d+)\s*\)`).
		FindAllStringSubmatch(stripCComments(string(src)), -1) {
		number, ok := spellNumberFor(t, m[1])
		if !ok {
			continue
		}
		class, ok := classes[m[2]]
		if !ok {
			t.Errorf("unknown class %s", m[2])
			continue
		}
		level, err := strconv.Atoi(m[3])
		if err != nil {
			t.Fatalf("unparseable level %q", m[3])
		}
		found++

		info, ok := spellTable[number]
		if !ok {
			t.Errorf("%s has a class level and no table entry", m[1])
			continue
		}
		if got := MinLevelFor(info, class); got != int32(level) {
			t.Errorf("%s for class %d is level %d, want %d", m[1], class, got, level)
		}
	}

	if found < 70 {
		t.Fatalf("checked only %d class levels", found)
	}
	t.Logf("checked %d class levels against the C", found)
}

// TestAClassThatNeverLearnsASpellCannotCastIt. The C fills every slot with
// LVL_IMMORT before init_spell_levels lowers the ones it names, so a class
// absent from the list is not merely unlisted — it is barred.
func TestAClassThatNeverLearnsASpellCannotCastIt(t *testing.T) {
	info, ok := Spell(SpellMagicMissile)
	if !ok {
		t.Fatal("magic missile is not in the table")
	}

	if got := MinLevelFor(info, ClassMagicUser); got != 1 {
		t.Errorf("a magic-user learns magic missile at %d, want 1", got)
	}
	if got := MinLevelFor(info, ClassWarrior); got != LevelImmortal {
		t.Errorf("a warrior learns magic missile at %d, want it barred", got)
	}
}

func TestSpellNumberByName(t *testing.T) {
	for name, want := range map[string]int32{
		"magic missile": SpellMagicMissile,
		"magic mis":     SpellMagicMissile,
		"armor":         SpellArmor,
		"heal":          SpellHeal,
	} {
		got, ok := SpellNumberByName(name)
		if !ok || got != want {
			t.Errorf("SpellNumberByName(%q) = %d, %v; want %d", name, got, ok, want)
		}
	}

	if _, ok := SpellNumberByName("frobnicate"); ok {
		t.Error("an invented spell was found")
	}
	if _, ok := SpellNumberByName(""); ok {
		t.Error("an empty name was found")
	}

	// A prefix that matches several is resolved deterministically, not by map
	// order — the same query must give the same answer every run.
	first, _ := SpellNumberByName("cure")
	for i := 0; i < 50; i++ {
		if again, _ := SpellNumberByName("cure"); again != first {
			t.Fatalf("`cure` resolved to %d and then %d", first, again)
		}
	}
}

// TestManaCostFallsWithLevel, down to the floor.
func TestManaCostFallsWithLevel(t *testing.T) {
	info, ok := Spell(SpellMagicMissile)
	if !ok {
		t.Fatal("magic missile is not in the table")
	}

	// 25 max, 10 min, 3 per level above the learning level of 1.
	if got := ManaCost(info, 1); got != 25 {
		t.Errorf("at level 1 it costs %d, want 25", got)
	}
	if got := ManaCost(info, 2); got != 22 {
		t.Errorf("at level 2 it costs %d, want 22", got)
	}
	if got := ManaCost(info, 30); got != 10 {
		t.Errorf("at level 30 it costs %d, want the floor of 10", got)
	}
	// It never goes below the floor however high the caster.
	if got := ManaCost(info, LevelImplementor); got != 10 {
		t.Errorf("at level 34 it costs %d, want 10", got)
	}
}

// spelloRow is one parsed spello() call.
type spelloRow struct {
	name        string
	manaMax     int32
	manaMin     int32
	manaChange  int32
	minPosition int32
	targets     uint64
	violent     bool
	routines    uint64
}

func parseSpello(t *testing.T) map[string]spelloRow {
	t.Helper()

	src, err := os.ReadFile(spellParserSource)
	if err != nil {
		t.Fatalf("reading spell_parser.c: %v", err)
	}
	body := stripCComments(string(src))
	start := strings.Index(body, "void mag_assign_spells(void)")
	if start < 0 {
		t.Fatal("could not find mag_assign_spells")
	}
	body = body[start:]

	positions := map[string]int32{
		"POS_DEAD": 0, "POS_MORTALLYW": 1, "POS_INCAP": 2, "POS_STUNNED": 3,
		"POS_SLEEPING": 4, "POS_RESTING": 5, "POS_SITTING": 6,
		"POS_FIGHTING": 7, "POS_STANDING": 8, "0": 0,
	}
	targets := map[string]uint64{
		"TAR_IGNORE": 1 << 0, "TAR_CHAR_ROOM": 1 << 1, "TAR_CHAR_WORLD": 1 << 2,
		"TAR_FIGHT_SELF": 1 << 3, "TAR_FIGHT_VICT": 1 << 4, "TAR_SELF_ONLY": 1 << 5,
		"TAR_NOT_SELF": 1 << 6, "TAR_OBJ_INV": 1 << 7, "TAR_OBJ_ROOM": 1 << 8,
		"TAR_OBJ_WORLD": 1 << 9, "TAR_OBJ_EQUIP": 1 << 10, "0": 0,
	}
	routines := map[string]uint64{
		"MAG_DAMAGE": 1 << 0, "MAG_AFFECTS": 1 << 1, "MAG_UNAFFECTS": 1 << 2,
		"MAG_POINTS": 1 << 3, "MAG_ALTER_OBJS": 1 << 4, "MAG_GROUPS": 1 << 5,
		"MAG_MASSES": 1 << 6, "MAG_AREAS": 1 << 7, "MAG_SUMMONS": 1 << 8,
		"MAG_CREATIONS": 1 << 9, "MAG_MANUAL": 1 << 10, "0": 0,
	}

	bits := func(expr string, table map[string]uint64) uint64 {
		var out uint64
		for _, part := range strings.Split(expr, "|") {
			part = strings.TrimSpace(part)
			if part == "" || part == "FALSE" {
				continue
			}
			v, ok := table[part]
			if !ok {
				t.Fatalf("unknown flag %q", part)
			}
			out |= v
		}
		return out
	}
	number := func(s string) int32 {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			t.Fatalf("unparseable number %q", s)
		}
		return int32(n)
	}

	out := map[string]spelloRow{}
	for _, m := range regexp.MustCompile(`(?s)\bspello\s*\(([^;]*?)\)\s*;`).
		FindAllStringSubmatch(body, -1) {
		args := splitTopLevel(m[1])
		if len(args) != 10 || strings.TrimSpace(args[0]) == "skill" {
			continue
		}

		pos, ok := positions[strings.TrimSpace(args[5])]
		if !ok {
			t.Fatalf("unknown position %q", args[5])
		}

		out[strings.TrimSpace(args[0])] = spelloRow{
			name:        strings.Trim(strings.TrimSpace(args[1]), `"`),
			manaMax:     number(args[2]),
			manaMin:     number(args[3]),
			manaChange:  number(args[4]),
			minPosition: pos,
			targets:     bits(args[6], targets),
			violent:     strings.TrimSpace(args[7]) == "TRUE",
			routines:    bits(args[8], routines),
		}
	}
	return out
}

// spellNumberFor maps a C constant name to the Go number, by reading
// spells.h — so the test does not depend on the same table it is checking.
func spellNumberFor(t *testing.T, cName string) (int32, bool) {
	t.Helper()

	if cachedSpellNumbers == nil {
		src, err := os.ReadFile(spellsHeader)
		if err != nil {
			t.Fatalf("reading spells.h: %v", err)
		}
		cachedSpellNumbers = map[string]int32{}
		for _, m := range regexp.MustCompile(`#define\s+((?:SPELL|SKILL)_\w+)\s+(\d+)`).
			FindAllStringSubmatch(string(src), -1) {
			n, _ := strconv.Atoi(m[2])
			cachedSpellNumbers[m[1]] = int32(n)
		}
	}
	n, ok := cachedSpellNumbers[cName]
	return n, ok
}

var cachedSpellNumbers map[string]int32

// splitTopLevel splits an argument list on commas outside brackets and
// strings.
func splitTopLevel(s string) []string {
	var out []string
	var cur strings.Builder
	depth, inString := 0, false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case inString:
			cur.WriteByte(ch)
			if ch == '\\' && i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			} else if ch == '"' {
				inString = false
			}
		case ch == '"':
			inString = true
			cur.WriteByte(ch)
		case ch == '(':
			depth++
			cur.WriteByte(ch)
		case ch == ')':
			depth--
			cur.WriteByte(ch)
		case ch == ',' && depth == 0:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(ch)
		}
	}
	out = append(out, cur.String())
	return out
}

func stripCComments(s string) string {
	return regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(s, "")
}
