// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"strings"
	"testing"
	"time"
)

// Identify is spell 201, which is *above* MAX_SPELLS — so `cast 'identify'`
// answers "Cast what?!?" in the C as well, and the spell is reachable only
// from a scroll or a potion. Until `use`, `quaff` and `recite` exist the
// report is tested here rather than through a session.

// TestIdentifyingAWeapon.
func TestIdentifyingAWeapon(t *testing.T) {
	sword := &Object{
		Keywords: "sword long", ShortDesc: "a long sword",
		Type:       ItemWeapon,
		Weight:     10,
		Cost:       350,
		ExtraFlags: NewSet(ItemMagic, ItemBless),
		PermAffect: NewSet(AffectDetectInvis),
		Values:     [NumObjValues]int32{0, 2, 6, 3},
		Affects: []ObjAffect{
			{Location: ApplyHitRoll, Modifier: 2},
			{Location: ApplyNone, Modifier: 5}, // skipped: no location
			{Location: ApplyDamRoll, Modifier: 0},
		},
	}

	got := IdentifyObject(sword)
	for _, want := range []string{
		"You feel informed:",
		"Object 'a long sword', Item type: WEAPON",
		"Item will give you following abilities:  DET-INVIS",
		"Item is: MAGIC BLESS",
		"Weight: 10, Value: 350, Rent: 0, Min Level: 0",
		// (6 + 1) / 2 * 2 = 7.0, which is right for a die and looks like an
		// off-by-one until you remember a d6 averages 3.5.
		"Damage Dice is '2D6' for an average per-round damage of 7.0.",
		"Can affect you as :",
		"   Affects: HITROLL By 2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report is missing %q:\n%s", want, got)
		}
	}

	// An apply with no location and one with no modifier are both skipped,
	// so only the hitroll line appears.
	if n := strings.Count(got, "   Affects:"); n != 1 {
		t.Errorf("%d affect lines, want 1:\n%s", n, got)
	}
}

// TestIdentifyingAnUnremarkableObject. An object with no flags at all prints
// "NOBITS", which is sprintbit's answer for an empty bitfield and is exactly
// as informative as it sounds.
func TestIdentifyingAnUnremarkableObject(t *testing.T) {
	got := IdentifyObject(&Object{
		Keywords: "rock", ShortDesc: "a rock", Type: ItemOther, Weight: 3,
	})

	if !strings.Contains(got, "Item is: NOBITS") {
		t.Errorf("an unflagged object does not print NOBITS:\n%s", got)
	}
	if strings.Contains(got, "Can affect you as") {
		t.Errorf("an object with no applies claims to have some:\n%s", got)
	}
	if strings.Contains(got, "Item will give you") {
		t.Errorf("an object with no permanent affect claims to have one:\n%s", got)
	}
}

// TestIdentifyingAWand shows its charges, and the spell it holds.
func TestIdentifyingAWand(t *testing.T) {
	wand := &Object{
		Keywords: "wand", ShortDesc: "a wand", Type: ItemWand,
		Values: [NumObjValues]int32{10, 3, 1, SpellMagicMissile},
	}

	got := IdentifyObject(wand)
	for _, want := range []string{
		"This WAND casts:  magic missile",
		"It has 3 maximum charges and 1 remaining.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report is missing %q:\n%s", want, got)
		}
	}

	// One charge is singular.
	wand.Values[1] = 1
	if got := IdentifyObject(wand); !strings.Contains(got, "1 maximum charge and") {
		t.Errorf("one charge is pluralised:\n%s", got)
	}
}

// TestIdentifyingAPerson prints another character's sheet, exact hit points
// and all.
func TestIdentifyingAPerson(t *testing.T) {
	born := time.Date(2001, 6, 1, 0, 0, 0, 0, time.UTC)
	welmar := newCharacter("Welmar")
	welmar.Record.Birth = born
	welmar.Record.Level = 12
	welmar.Record.Height = 180
	welmar.Record.Weight = 170
	welmar.Record.Points.Hit = 95
	welmar.Record.Points.Mana = 40
	welmar.Record.Points.HitRoll = 3
	welmar.Record.Points.DamRoll = 4

	// One mud year is a little over eleven real days.
	got := IdentifyCharacter(welmar, born.Add(time.Duration(3*SecondsPerMudYear)*time.Second))

	for _, want := range []string{
		"Name: Welmar",
		// Everyone starts at seventeen, so three mud years on is twenty.
		"Welmar is 20 years, 0 months, 0 days and 0 hours old.",
		"Height 180 cm, Weight 170 pounds",
		"Level: 12, Hits: 95, Mana: 40",
		"Hitroll: 3, Damroll: 4",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the report is missing %q:\n%s", want, got)
		}
	}
}

// TestSprintBit, which has three behaviours worth pinning: the trailing
// space, NOBITS, and what happens above the end of the table.
func TestSprintBit(t *testing.T) {
	// Raw bit positions rather than the ExtraFlag constants, on purpose:
	// SprintBit maps a *bit position* to a name, so a table written in the
	// domain's own constants would agree with itself whatever those
	// constants were. What is under test is that bit 0 prints GLOW.
	for _, tc := range []struct {
		flags uint64
		want  string
	}{
		{0, "NOBITS "},
		{1 << 0, "GLOW "},
		{1<<0 | 1<<1, "GLOW HUM "},
		{1 << 7, "NO_DROP "},
		// Bit 18 is past the end of the eighteen-entry table.
		{1 << 18, "UNDEFINED "},
		{1<<0 | 1<<18, "GLOW UNDEFINED "},
	} {
		if got := SprintBit(tc.flags, ExtraBitNames()); got != tc.want {
			t.Errorf("SprintBit(%d) = %q, want %q", tc.flags, got, tc.want)
		}
	}
}

// TestSprintTypeMatchesTheTables, including the out-of-range answer.
func TestSprintTypeMatchesTheTables(t *testing.T) {
	if got := SprintType(ItemWeapon.Number(), ItemTypeNames); got != "WEAPON" {
		t.Errorf("item type 5 is %q", got)
	}
	if got := SprintType(ApplyHitRoll.Number(), ApplyTypeNames()); got != "HITROLL" {
		t.Errorf("apply 18 is %q", got)
	}
	if got := SprintType(99, ApplyTypeNames()); got != "UNDEFINED" {
		t.Errorf("apply 99 is %q", got)
	}
}
