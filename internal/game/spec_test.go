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
	"testing"
)

const specAssignSource = "../../reference/moderncserver/src/spec_assign.c"

// TestSpecialAssignmentsMatchTheCSource re-parses spec_assign.c.
//
// Two hundred and five rows of vnum-and-name, transcribed. Copied by hand that
// is two hundred chances to give a mobile the wrong job — a guildmaster who
// does not teach, or a janitor that eats corpses — and every one of them would
// look plausible in the file.
func TestSpecialAssignmentsMatchTheCSource(t *testing.T) {
	src, err := os.ReadFile(specAssignSource)
	if err != nil {
		t.Fatalf("reading the C the table came from: %v", err)
	}

	// Strip comments first: several assignments are commented out, and one of
	// the comments contains what looks like a call.
	text := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(src), "")
	text = regexp.MustCompile(`//[^\n]*`).ReplaceAllString(text, "")

	for _, tc := range []struct {
		macro string
		have  []SpecAssignment
	}{
		{"ASSIGNMOB", MobileSpecials},
		{"ASSIGNOBJ", ObjectSpecials},
		{"ASSIGNROOM", RoomSpecials},
	} {
		call := regexp.MustCompile(tc.macro + `\(\s*(\d+)\s*,\s*([a-z_]+)\s*\)`)
		found := call.FindAllStringSubmatch(text, -1)

		if len(found) != len(tc.have) {
			t.Errorf("%s: parsed %d from the C, have %d", tc.macro, len(found), len(tc.have))
			continue
		}
		for i, m := range found {
			vnum, err := strconv.ParseInt(m[1], 10, 32)
			if err != nil {
				t.Fatalf("%s row %d: %v", tc.macro, i, err)
			}
			if got := tc.have[i]; int64(got.Vnum) != vnum || got.Name != m[2] {
				t.Errorf("%s row %d is {%d, %q}, want {%d, %q}",
					tc.macro, i, got.Vnum, got.Name, vnum, m[2])
			}
		}
	}
}

// TestEverySpecialNameIsOneTheCDefines. A typo in a name is a special that
// silently never runs, which is indistinguishable from one that is not
// implemented yet.
func TestEverySpecialNameIsOneTheCDefines(t *testing.T) {
	src, err := os.ReadFile("../../reference/moderncserver/src/spec_procs.c")
	if err != nil {
		t.Fatalf("reading spec_procs.c: %v", err)
	}
	// The C's own definitions, plus the ones that live in other files.
	defined := map[string]bool{
		"postmaster":  true, // mail.c
		"gen_board":   true, // boards.c
		"shop_keeper": true,
	}
	for _, m := range regexp.MustCompile(`SPECIAL\(([a-z_]+)\)`).FindAllStringSubmatch(string(src), -1) {
		defined[m[1]] = true
	}
	for _, name := range []string{"receptionist", "cryogenicist", "pet_shops", "bank"} {
		defined[name] = true
	}

	all := append(append(append([]SpecAssignment{}, MobileSpecials...), ObjectSpecials...), RoomSpecials...)
	for _, a := range all {
		if !defined[a.Name] {
			t.Errorf("vnum %d is assigned %q, which the C never defines", a.Vnum, a.Name)
		}
	}
}

// TestGuildGuardsUseTheRemortVector, which is the local rewrite and the whole
// reason the guild guard is worth its own table.
func TestGuildGuardsUseTheRemortVector(t *testing.T) {
	// The mage guild: room 3017, south.
	mage := &PlayerRecord{Class: ClassMagicUser, RemortVector: NewSet(ClassMagicUser)}
	warrior := &PlayerRecord{Class: ClassWarrior, RemortVector: NewSet(ClassWarrior)}

	if GuildBars(mage, 3017, South) {
		t.Error("a magic-user was barred from the magic-user guild")
	}
	if !GuildBars(warrior, 3017, South) {
		t.Error("a warrior walked into the magic-user guild")
	}

	// The point of the rewrite: a warrior who has *been* a magic-user is let
	// through. Stock CircleMUD compares GET_CLASS and would turn them away.
	exMage := &PlayerRecord{
		Class:        ClassWarrior,
		RemortVector: NewSet(ClassWarrior, ClassMagicUser),
	}
	if GuildBars(exMage, 3017, South) {
		t.Error("a warrior who was once a magic-user was barred")
	}

	// A door with no guild row is not guarded at all.
	if GuildBars(warrior, 3017, North) || GuildBars(warrior, 9999, South) {
		t.Error("a guard blocked a door it is not standing at")
	}

	// The -999 rows block everybody, which is what "all" means there.
	if !GuildBars(mage, 5065, West) {
		t.Error("the Brass Dragon guard let somebody through")
	}
}

// TestMobileAttackSpellLadder, including the level below the bottom of it.
func TestMobileAttackSpellLadder(t *testing.T) {
	for level, want := range map[int32]int32{
		4: SpellMagicMissile, 5: SpellMagicMissile,
		6: SpellChillTouch, 7: SpellChillTouch,
		8: SpellBurningHands, 9: SpellBurningHands,
		10: SpellShockingGrasp, 11: SpellShockingGrasp,
		12: SpellLightningBolt, 13: SpellLightningBolt,
		14: SpellColorSpray, 17: SpellColorSpray,
		18: SpellFireball, 30: SpellFireball,
		// Below the bottom of the switch, where the C falls through to the
		// default: a level-two mobile mage throws fireballs.
		2: SpellFireball,
	} {
		if got := MobileAttackSpell(level); got != want {
			t.Errorf("level %d throws %s, want %s", level, SpellName(got), SpellName(want))
		}
	}
}

// TestAssigningSpecialsToAWorldThatDoesNotHaveThem counts rather than fails:
// the shipped stock data is Midgaard and a third of this tree's assignments
// point at the archived world.
func TestAssigningSpecialsToAWorldThatDoesNotHaveThem(t *testing.T) {
	l := objectWorld()

	attached, missing := l.AssignSpecials()
	if attached+missing != len(MobileSpecials)+len(ObjectSpecials)+len(RoomSpecials) {
		t.Errorf("%d attached + %d missing does not account for the whole table",
			attached, missing)
	}
	if missing == 0 {
		t.Error("a world of three rooms claimed every special assignment")
	}
}
