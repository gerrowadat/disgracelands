// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "testing"

func TestDamageTierBoundaries(t *testing.T) {
	for _, tc := range []struct {
		dam  int32
		want int
	}{
		{0, 0}, {1, 1}, {2, 1}, {3, 2}, {4, 2}, {5, 3}, {6, 3},
		{7, 4}, {10, 4}, {11, 5}, {14, 5}, {15, 6}, {19, 6},
		{20, 7}, {23, 7}, {24, 8}, {50, 8}, {51, 9}, {1000, 9},
	} {
		got := damageTierFor(tc.dam)
		if got != damageTiers[tc.want] {
			t.Errorf("damageTierFor(%d) = tier %+v, want tier %d (%+v)", tc.dam, got, tc.want, damageTiers[tc.want])
		}
	}
}

// The C's own text, transcribed once and checked here so a later edit
// cannot silently drift from dam_weapons[]/attack_hit_text[] (fight.c).
func TestAttackVerbsMatchTheC(t *testing.T) {
	want := [15]struct{ singular, plural string }{
		{"hit", "hits"}, {"sting", "stings"}, {"whip", "whips"},
		{"slash", "slashes"}, {"bite", "bites"}, {"bludgeon", "bludgeons"},
		{"crush", "crushes"}, {"pound", "pounds"}, {"claw", "claws"},
		{"maul", "mauls"}, {"thrash", "thrashes"}, {"pierce", "pierces"},
		{"blast", "blasts"}, {"punch", "punches"}, {"stab", "stabs"},
	}
	if attackVerbs != want {
		t.Errorf("attackVerbs = %+v, want %+v", attackVerbs, want)
	}
}

func TestReplaceWeaponVerb(t *testing.T) {
	for _, tc := range []struct {
		format string
		attack int32
		want   string
	}{
		{"You #w $N.", TypeHit + AttackSlash, "You slash $N."},
		{"$n #W $N.", TypeHit + AttackSlash, "$n slashes $N."},
		{"literal # sign", TypeHit + AttackHit, "literal # sign"},
		{"#w and #W together", TypeHit + AttackStab, "stab and stabs together"},
		{"trailing #", TypeHit + AttackHit, "trailing #"},
	} {
		if got := replaceWeaponVerb(tc.format, tc.attack); got != tc.want {
			t.Errorf("replaceWeaponVerb(%q, %d) = %q, want %q", tc.format, tc.attack, got, tc.want)
		}
	}
}

func TestReplaceWeaponVerbOutOfRangeFallsBackToHit(t *testing.T) {
	if got, want := replaceWeaponVerb("You #w $N.", 999), "You hit $N."; got != want {
		t.Errorf("replaceWeaponVerb with an out-of-range attack type = %q, want %q", got, want)
	}
}

func TestDamageMessageOmitsColour(t *testing.T) {
	// No CCYEL/CCRED/CCNRM anywhere — nothing in this port emits colour
	// yet (docs/deviations.md), so the C's wrapping is simply absent.
	got := DamageMessage(5, TypeHit+AttackSlash, AudienceChar)
	if want := "You slash $N."; got != want {
		t.Errorf("DamageMessage = %q, want %q", got, want)
	}
}

func TestDamageMessageAllAudiences(t *testing.T) {
	if got, want := DamageMessage(0, TypeHit+AttackHit, AudienceRoom), "$n tries to hit $N, but misses."; got != want {
		t.Errorf("room = %q, want %q", got, want)
	}
	if got, want := DamageMessage(0, TypeHit+AttackHit, AudienceChar), "You try to hit $N, but miss."; got != want {
		t.Errorf("char = %q, want %q", got, want)
	}
	if got, want := DamageMessage(0, TypeHit+AttackHit, AudienceVictim), "$n tries to hit you, but misses."; got != want {
		t.Errorf("victim = %q, want %q", got, want)
	}
}
