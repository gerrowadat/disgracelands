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

// constantsCSource is the C the ability tables were transcribed from.
const constantsCSource = "../../reference/moderncserver/src/constants.c"

// TestAbilityTablesMatchTheCSource re-parses constants.c and compares every
// number in every table.
//
// Six tables, a bit over three hundred values. Copied by hand that is three
// hundred chances to make a mistake that surfaces years later as one weapon
// doing slightly the wrong damage, so they are checked against their source on
// every run instead.
func TestAbilityTablesMatchTheCSource(t *testing.T) {
	src, err := os.ReadFile(constantsCSource)
	if err != nil {
		t.Fatalf("reading the C source the tables came from: %v", err)
	}
	text := string(src)

	t.Run("str_app", func(t *testing.T) {
		want := parseCTable(t, text, "str_app", 4)
		if len(want) != len(strApply) {
			t.Fatalf("parsed %d rows, have %d", len(want), len(strApply))
		}
		for i, row := range want {
			got := strApply[i]
			check(t, "str_app", i, "tohit", got.ToHit, row[0])
			check(t, "str_app", i, "todam", got.ToDamage, row[1])
			check(t, "str_app", i, "carry_w", got.CarryWeight, row[2])
			check(t, "str_app", i, "wield_w", got.WieldWeight, row[3])
		}
	})

	t.Run("dex_app", func(t *testing.T) {
		want := parseCTable(t, text, "dex_app", 3)
		if len(want) != len(dexApply) {
			t.Fatalf("parsed %d rows, have %d", len(want), len(dexApply))
		}
		for i, row := range want {
			got := dexApply[i]
			check(t, "dex_app", i, "reaction", got.Reaction, row[0])
			check(t, "dex_app", i, "miss_att", got.MissileAttack, row[1])
			check(t, "dex_app", i, "defensive", got.Defensive, row[2])
		}
	})

	t.Run("dex_app_skill", func(t *testing.T) {
		want := parseCTable(t, text, "dex_app_skill", 5)
		if len(want) != len(dexApplySkill) {
			t.Fatalf("parsed %d rows, have %d", len(want), len(dexApplySkill))
		}
		for i, row := range want {
			got := dexApplySkill[i]
			check(t, "dex_app_skill", i, "p_pocket", got.PickPockets, row[0])
			check(t, "dex_app_skill", i, "p_locks", got.PickLocks, row[1])
			check(t, "dex_app_skill", i, "traps", got.Traps, row[2])
			check(t, "dex_app_skill", i, "sneak", got.Sneak, row[3])
			check(t, "dex_app_skill", i, "hide", got.Hide, row[4])
		}
	})

	for _, tc := range []struct {
		cName string
		have  []int32
	}{
		{"int_app", intApplyLearn[:]},
		{"wis_app", wisApplyPractices[:]},
	} {
		t.Run(tc.cName, func(t *testing.T) {
			want := parseCTable(t, text, tc.cName, 1)
			if len(want) != len(tc.have) {
				t.Fatalf("parsed %d rows, have %d", len(want), len(tc.have))
			}
			for i, row := range want {
				check(t, tc.cName, i, "value", tc.have[i], row[0])
			}
		})
	}

	t.Run("con_app", func(t *testing.T) {
		// Only the hitp column is ported; shock belongs to the resurrection
		// rules, which nothing reads yet.
		want := parseCTable(t, text, "con_app", 2)
		if len(want) != len(conApplyHitPoints) {
			t.Fatalf("parsed %d rows, have %d", len(want), len(conApplyHitPoints))
		}
		for i, row := range want {
			check(t, "con_app", i, "hitp", conApplyHitPoints[i], row[0])
		}
	})
}

func check(t *testing.T, table string, row int, field string, got, want int32) {
	t.Helper()
	if got != want {
		t.Errorf("%s[%d].%s = %d, want %d", table, row, field, got, want)
	}
}

// parseCTable pulls the brace-delimited rows out of one table definition.
func parseCTable(t *testing.T, src, name string, columns int) [][]int32 {
	t.Helper()

	// The declarations differ in their struct type but all read
	// `... <type> <name>[] = {`.
	start := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\[\]\s*=\s*\{`).FindStringIndex(src)
	if start == nil {
		t.Fatalf("could not find %s in %s", name, constantsCSource)
	}
	body := src[start[1]:]
	end := strings.Index(body, "\n};")
	if end < 0 {
		t.Fatalf("%s is not terminated", name)
	}
	body = body[:end]

	// Comments carry the index and sometimes a stray brace-free number; strip
	// them before looking for rows.
	body = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(body, "")

	var out [][]int32
	for _, row := range regexp.MustCompile(`\{([^}]*)\}`).FindAllStringSubmatch(body, -1) {
		fields := strings.Split(row[1], ",")
		if len(fields) != columns {
			t.Fatalf("%s row %d has %d columns, want %d: %q", name, len(out), len(fields), columns, row[1])
		}
		var values []int32
		for _, f := range fields {
			n, err := strconv.Atoi(strings.TrimSpace(f))
			if err != nil {
				t.Fatalf("%s row %d: unparseable value %q", name, len(out), f)
			}
			values = append(values, int32(n))
		}
		out = append(out, values)
	}

	// A single-column table has no inner braces, so its values are bare.
	if columns == 1 && len(out) == 0 {
		for _, f := range strings.Split(body, ",") {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			n, err := strconv.Atoi(f)
			if err != nil {
				t.Fatalf("%s: unparseable value %q", name, f)
			}
			out = append(out, []int32{int32(n)})
		}
	}

	if len(out) == 0 {
		t.Fatalf("parsed no rows from %s", name)
	}
	return out
}

// TestStrengthIndexHandlesExceptionalStrength covers STRENGTH_APPLY_INDEX,
// which is the reason str_app has 31 rows for 26 possible scores.
func TestStrengthIndexHandlesExceptionalStrength(t *testing.T) {
	for _, tc := range []struct {
		strength, percentile int32
		want                 int
		why                  string
	}{
		{18, 0, 18, "a plain 18 is row 18, not an exceptional band"},
		{18, 1, 26, "18/01 is the bottom of the first band"},
		{18, 50, 26, "18/50 is the top of the first band"},
		{18, 51, 27, ""},
		{18, 75, 27, ""},
		{18, 76, 28, ""},
		{18, 90, 28, ""},
		{18, 91, 29, ""},
		{18, 99, 29, ""},
		{18, 100, 30, "18/00 is the best there is"},
		{17, 100, 17, "a percentile on any score but 18 is meaningless"},
		{25, 100, 25, "and that includes the godly scores"},
		{0, 0, 0, ""},
		{30, 0, 25, "out of range is clamped rather than read off the end"},
		{-1, 0, 0, ""},
	} {
		if got := strengthIndex(tc.strength, tc.percentile); got != tc.want {
			t.Errorf("strengthIndex(%d, %d) = %d, want %d %s",
				tc.strength, tc.percentile, got, tc.want, tc.why)
		}
	}
}

// TestExceptionalStrengthBeatsAPlainEighteen: the bands exist to be better
// than a plain 18 and worse than a 19, and a transcription that shuffled the
// rows would still pass a row-by-row comparison against the C if it shuffled
// the C's rows too.
func TestExceptionalStrengthBeatsAPlainEighteen(t *testing.T) {
	plain := Strength(18, 0)
	for _, percentile := range []int32{1, 51, 76, 91, 100} {
		band := Strength(18, percentile)
		if band.ToDamage <= plain.ToDamage {
			t.Errorf("18/%02d does %d damage, a plain 18 does %d", percentile, band.ToDamage, plain.ToDamage)
		}
		if band.CarryWeight <= plain.CarryWeight {
			t.Errorf("18/%02d carries %d, a plain 18 carries %d", percentile, band.CarryWeight, plain.CarryWeight)
		}
	}

	// And the bands are ordered among themselves.
	previous := plain
	for _, percentile := range []int32{1, 51, 76, 91, 100} {
		band := Strength(18, percentile)
		if band.ToDamage < previous.ToDamage || band.CarryWeight < previous.CarryWeight {
			t.Errorf("18/%02d is weaker than the band below it", percentile)
		}
		previous = band
	}
}

// TestAbilityAccessorsAreBounded, because the C indexes these unchecked and a
// spell or an implementor could in principle produce a score outside the
// table.
func TestAbilityAccessorsAreBounded(t *testing.T) {
	for _, score := range []int32{-1000, -1, 0, 18, 25, 26, 1000} {
		Strength(score, 0)
		Strength(score, 100)
		Dexterity(score)
		DexteritySkills(score)
		LearnPercent(score)
		Practices(score)
		HitPointBonus(score)
	}
}
