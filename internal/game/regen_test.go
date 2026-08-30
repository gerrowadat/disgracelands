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
	"time"
)

// regenContext is a Regenerator for tests.
type regenContext struct {
	npc       bool
	pos       Position
	poisoned  bool
	goodRegen bool
}

func (r regenContext) IsNPC() bool        { return r.npc }
func (r regenContext) Position() Position { return r.pos }
func (r regenContext) Poisoned() bool     { return r.poisoned }
func (r regenContext) GoodRegen() bool    { return r.goodRegen }

// TestRegenerationMatchesTheCExhaustively compares hit_gain, mana_gain and
// move_gain against the C across every combination of the things that change
// them.
//
// These are integer formulas with four truncating divisions each, and they
// decide how fast everybody in the game heals. An off-by-one anywhere in them
// would be invisible in play for months and wrong for all of it, so this walks
// the whole input space rather than sampling it: 84 ages by 9 positions by
// every flag, three formulas each. The oracle emits the lot in one run — a
// process per comparison would take minutes.
func TestRegenerationMatchesTheCExhaustively(t *testing.T) {
	oracle := buildRegenOracle(t)

	out, err := exec.Command(oracle, "sweep").Output()
	if err != nil {
		t.Fatalf("running the oracle sweep: %v", err)
	}

	now := time.Now()
	compared := 0

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 9 {
			t.Fatalf("oracle line %q has %d fields, want 9", line, len(fields))
		}
		var n [9]int32
		for i, f := range fields {
			v, err := strconv.Atoi(f)
			if err != nil {
				t.Fatalf("unparseable oracle field %q in %q", f, line)
			}
			n[i] = int32(v)
		}
		age, pos := n[0], Position(n[1])
		caster, starving, poisoned, good := n[2] == 1, n[3] == 1, n[4] == 1, n[5] == 1
		wantHit, wantMana, wantMove := n[6], n[7], n[8]

		rec := &PlayerRecord{
			Birth:      now.Add(-time.Duration(age-startingAge) * SecondsPerMudYear * time.Second),
			Level:      10,
			Class:      ClassWarrior,
			Conditions: [3]int32{0, 24, 24},
		}
		if caster {
			rec.Class = ClassMagicUser
		}
		if starving {
			rec.Conditions[CondFull] = 0
		}

		// If the record does not produce the age the C was given, the two
		// sides are describing different characters and nothing below means
		// anything.
		if got := Age(rec, now); got != age {
			t.Fatalf("Age() gave %d, want %d", got, age)
		}

		ctx := regenContext{pos: pos, poisoned: poisoned, goodRegen: good}
		for _, f := range []struct {
			name      string
			got, want int32
		}{
			{"hit", HitGain(rec, ctx, now), wantHit},
			{"mana", ManaGain(rec, ctx, now), wantMana},
			{"move", MoveGain(rec, ctx, now), wantMove},
		} {
			if f.got != f.want {
				t.Fatalf("%s_gain(age %d, %s, caster %v, starving %v, poisoned %v, goodregen %v) = %d, the C gives %d",
					f.name, age, pos, caster, starving, poisoned, good, f.got, f.want)
			}
			compared++
		}
	}

	if compared == 0 {
		t.Fatal("the oracle sweep produced nothing")
	}
	t.Logf("compared %d regeneration values against the C", compared)
}

// TestMobileRegenerationMatchesTheC. A mobile regains its level per tick and
// skips every adjustment except poison and the room.
func TestMobileRegenerationMatchesTheC(t *testing.T) {
	oracle := buildRegenOracle(t)

	for _, level := range []int32{0, 1, 5, 34, 100} {
		for _, poisoned := range []bool{false, true} {
			for _, good := range []bool{false, true} {
				rec := &PlayerRecord{Level: level}
				ctx := regenContext{npc: true, pos: PosStanding, poisoned: poisoned, goodRegen: good}

				for _, f := range []struct {
					name string
					got  int32
				}{
					{"hit", HitGain(rec, ctx, time.Now())},
					{"mana", ManaGain(rec, ctx, time.Now())},
					{"move", MoveGain(rec, ctx, time.Now())},
				} {
					want := runRegenOracle(t, oracle, "npc", f.name, itoa(level),
						boolArg(poisoned), boolArg(good))
					if f.got != want {
						t.Errorf("npc %s_gain(level %d, poisoned %v, goodregen %v) = %d, the C gives %d",
							f.name, level, poisoned, good, f.got, want)
					}
				}
			}
		}
	}
}

// TestGrafMatchesTheCAtEveryAge, including the band boundaries where the
// interpolation changes slope and the 60..79 band that divides by 20 rather
// than 15.
func TestGrafMatchesTheCAtEveryAge(t *testing.T) {
	oracle := buildRegenOracle(t)

	for age := int32(0); age <= 120; age++ {
		out := runRegenOracleLines(t, oracle, "graf", itoa(age))
		if len(out) != 3 {
			t.Fatalf("the oracle gave %d values for age %d, want 3", len(out), age)
		}
		for i, got := range []int32{
			graf(age, 8, 12, 20, 32, 16, 10, 4),
			graf(age, 4, 8, 12, 16, 12, 10, 8),
			graf(age, 16, 20, 24, 20, 16, 12, 10),
		} {
			if got != out[i] {
				t.Errorf("graf curve %d at age %d = %d, the C gives %d", i, age, got, out[i])
			}
		}
	}
}

// TestPoisonAppliesBeforeTheRoom. The order matters: a poisoned character in
// a good-regeneration room gets half of what they otherwise would, not a
// quarter and not the full amount.
func TestPoisonAppliesBeforeTheRoom(t *testing.T) {
	now := time.Now()
	rec := &PlayerRecord{Class: ClassWarrior, Conditions: [3]int32{0, 24, 24}}

	plain := HitGain(rec, regenContext{pos: PosStanding}, now)
	poisoned := HitGain(rec, regenContext{pos: PosStanding, poisoned: true}, now)
	both := HitGain(rec, regenContext{pos: PosStanding, poisoned: true, goodRegen: true}, now)

	if poisoned != plain/4 {
		t.Errorf("poison gave %d, want a quarter of %d", poisoned, plain)
	}
	if both != poisoned*2 {
		t.Errorf("a poisoned character in a good-regen room got %d, want twice %d", both, poisoned)
	}
}

// TestAgeStartsAtSeventeen, which is the C's rule for everybody.
func TestAgeStartsAtSeventeen(t *testing.T) {
	now := time.Now()

	if got := Age(&PlayerRecord{Birth: now}, now); got != startingAge {
		t.Errorf("a character born now is %d, want %d", got, startingAge)
	}
	if got := Age(&PlayerRecord{}, now); got != startingAge {
		t.Errorf("a character with no birth date is %d, want %d", got, startingAge)
	}
	// A clock that went backwards must not produce a negative age.
	if got := Age(&PlayerRecord{Birth: now.Add(time.Hour)}, now); got != startingAge {
		t.Errorf("a character born in the future is %d, want %d", got, startingAge)
	}

	oneYear := now.Add(-SecondsPerMudYear * time.Second)
	if got := Age(&PlayerRecord{Birth: oneYear}, now); got != startingAge+1 {
		t.Errorf("a character one mud year old is %d, want %d", got, startingAge+1)
	}
}

// TestRemortedCharactersHealLikeCasters is the local rule again, in a place
// it is easy to miss: hit_gain halves for a magic-user or cleric, and the
// test is the remort-aware macro.
func TestRemortedCharactersHealLikeCasters(t *testing.T) {
	now := time.Now()
	ctx := regenContext{pos: PosStanding}

	warrior := &PlayerRecord{Class: ClassWarrior, Conditions: [3]int32{0, 24, 24}}
	remorted := &PlayerRecord{
		Class: ClassWarrior, Conditions: [3]int32{0, 24, 24},
		RemortVector: int32(RemortCleric),
	}

	plain := HitGain(warrior, ctx, now)
	after := HitGain(remorted, ctx, now)
	if after != plain/2 {
		t.Errorf("a warrior who remorted through cleric heals %d, want half of %d", after, plain)
	}

	// And gains mana at a caster's rate, which is the compensation.
	if ManaGain(remorted, ctx, now) != ManaGain(warrior, ctx, now)*2 {
		t.Error("a remorted warrior does not gain mana at a caster's rate")
	}
}

func TestGainCondition(t *testing.T) {
	rec := &PlayerRecord{Conditions: [3]int32{5, 24, 24}}

	// Clamped at the top.
	GainCondition(rec, CondFull, 10)
	if rec.Conditions[CondFull] != MaxCondition {
		t.Errorf("full is %d, want it clamped to %d", rec.Conditions[CondFull], MaxCondition)
	}

	// And at the bottom, with the message the C sends on the way past zero.
	rec.Conditions[CondFull] = 1
	if change := GainCondition(rec, CondFull, -5); change.Message != "You are hungry.\r\n" {
		t.Errorf("reaching zero food said %q", change.Message)
	}
	if rec.Conditions[CondFull] != 0 {
		t.Errorf("full is %d, want 0", rec.Conditions[CondFull])
	}

	// Sobering up is only announced to somebody who was drunk.
	rec.Conditions[CondDrunk] = 2
	if change := GainCondition(rec, CondDrunk, -2); change.Message != "You are now sober.\r\n" {
		t.Errorf("sobering up said %q", change.Message)
	}
	if change := GainCondition(rec, CondDrunk, -1); change.Message != "" {
		t.Errorf("a character who was already sober was told %q", change.Message)
	}
}

// PLR_WRITING suppresses the message and nothing else: `if (GET_COND(ch,
// condition) || PLR_FLAGGED(ch, PLR_WRITING)) return;` (limits.c:394) sits
// *after* the condition has already been changed and clamped. Somebody in
// the line editor goes on getting hungry; they are simply not interrupted
// to be told so.
func TestWritingSuppressesTheConditionMessageButNotTheChange(t *testing.T) {
	rec := &PlayerRecord{Conditions: [3]int32{5, 24, 24}}
	rec.PlayerFlags = rec.PlayerFlags.With(PlayerWriting)

	rec.Conditions[CondFull] = 1
	change := GainCondition(rec, CondFull, -5)
	if change.Message != "" {
		t.Errorf("somebody in the editor was told %q", change.Message)
	}
	if rec.Conditions[CondFull] != 0 {
		t.Errorf("full is %d, want 0 — the change happens either way",
			rec.Conditions[CondFull])
	}

	// Out of the editor, the same step does say so.
	rec.PlayerFlags = rec.PlayerFlags.Without(PlayerWriting)
	rec.Conditions[CondFull] = 1
	if change := GainCondition(rec, CondFull, -5); change.Message != "You are hungry.\r\n" {
		t.Errorf("outside the editor, reaching zero food said %q", change.Message)
	}
}

// TestImmortalsNeverGetHungry: a condition of -1 means "does not apply", and
// the C returns before touching it.
func TestImmortalsNeverGetHungry(t *testing.T) {
	rec := &PlayerRecord{Conditions: [3]int32{-1, -1, -1}}
	for _, cond := range []Condition{CondDrunk, CondFull, CondThirst} {
		if change := GainCondition(rec, cond, -1); change.Changed {
			t.Errorf("condition %d changed on an immortal", cond)
		}
		if rec.Conditions[cond] != -1 {
			t.Errorf("condition %d is now %d, want -1", cond, rec.Conditions[cond])
		}
	}
}

func itoa(n int32) string { return strconv.Itoa(int(n)) }

func boolArg(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func buildRegenOracle(t *testing.T) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("gcc not found in CI; the regeneration comparison must run")
		}
		t.Skip("gcc not found; skipping the regeneration comparison")
	}

	src := filepath.Join(repoRoot(t), "reference", "tools", "regenoracle.c")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("%s not found: %v", src, err)
	}

	bin := filepath.Join(t.TempDir(), "regenoracle")
	build := exec.Command(gcc, "-O2", "-Wall", "-Werror", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compiling the oracle: %v\n%s", err, out)
	}
	return bin
}

func runRegenOracle(t *testing.T, bin string, args ...string) int32 {
	t.Helper()
	out := runRegenOracleLines(t, bin, args...)
	if len(out) != 1 {
		t.Fatalf("the oracle gave %d values for %v, want 1", len(out), args)
	}
	return out[0]
}

func runRegenOracleLines(t *testing.T, bin string, args ...string) []int32 {
	t.Helper()

	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		t.Fatalf("running the oracle with %v: %v", args, err)
	}

	var values []int32
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			t.Fatalf("unparseable oracle output %q: %v", line, err)
		}
		values = append(values, int32(n))
	}
	return values
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
