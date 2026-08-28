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

	"github.com/gerrowadat/disgracelands/internal/rng"
)

// What a character *is* when they first enter the world, against the C.
//
// do_start decides every number a level 1 character has, and it does it by
// drawing from the generator in an order no one function makes obvious: the
// ability rolls are twenty-four draws whose order the sort throws away, a
// warrior who rolls 18 strength takes a twenty-fifth, and advance_level draws
// hit points, then **mana**, then movement, for every class — including the
// two that then throw the mana away, because the `GET_LEVEL(ch) > 1` guard is
// on the addition and not on the roll (class.c:1868-1909).
//
// The oracle is of **reference/moderncserver** and not of WipeMud-src, and
// the two disagree here: WipeMud's advance_level takes no mana draw for a
// thief or a warrior and has no paladin case at all, and its do_start sets
// max_mana and max_move as well as max_hit. moderncserver is the server that
// was played (reference/README.md) and is the one this is a test of. Reading
// the wrong tree is how the mana draw came to be deleted for two classes on
// 2026-08-28; this comment is here so the next reader does not repeat it.
//
// The test runs a *sequence* of characters from one seeded stream and prints
// a following draw rather than checking one character, because that is the
// only way a missing or extra draw is visible at all — a port one draw out
// agrees perfectly about the character it went wrong on. Same argument as
// randoracle's alternating mode.

type startedCharacter struct {
	str, strAdd, intel, wis, dex, con, cha int32
	maxHit, maxMana, maxMove, practices    int32
	next                                   int32
}

func TestCharacterCreationMatchesTheCOracle(t *testing.T) {
	oracle := buildStartOracle(t)

	classes := []struct {
		name string
		id   int32
	}{
		{"magic-user", ClassMagicUser},
		{"cleric", ClassCleric},
		{"thief", ClassThief},
		{"warrior", ClassWarrior},
		// Unreachable at creation — Paladin is remort-only
		// (docs/deviations.md) — but do_start is what remorting runs, so
		// its draws matter and the C has a case for it.
		{"paladin", ClassPaladin},
	}
	// Six seeds, the same set randoracle uses, and enough characters each
	// that a one-draw slip has nowhere to hide.
	seeds := []uint64{1, 42, 1000, 123456789, 2147483646, 987654321}
	const perSeed = 200

	for _, class := range classes {
		for _, seed := range seeds {
			want := runStartOracle(t, oracle, seed, class.id, perSeed)
			if len(want) != perSeed {
				t.Fatalf("%s seed %d: the oracle gave %d characters, want %d",
					class.name, seed, len(want), perSeed)
			}

			r := rng.NewRand(rng.NewCircle(seed))
			for i, w := range want {
				rec := &PlayerRecord{Class: class.id, Sex: SexMale}
				Start(rec, r)
				got := startedCharacter{
					str: rec.Abilities.Strength, strAdd: rec.Abilities.StrengthPercentile,
					intel: rec.Abilities.Intelligence, wis: rec.Abilities.Wisdom,
					dex: rec.Abilities.Dexterity, con: rec.Abilities.Constitution,
					cha:    rec.Abilities.Charisma,
					maxHit: rec.Points.MaxHit, maxMana: rec.Points.MaxMana,
					maxMove: rec.Points.MaxMove, practices: rec.SpellsToLearn,
					next: r.Number(0, 999999),
				}
				if got != w {
					t.Fatalf("%s seed %d character %d:\n got %+v\nwant %+v",
						class.name, seed, i, got, w)
				}
			}
		}
	}
}

func buildStartOracle(t *testing.T) string {
	t.Helper()

	gcc, err := exec.LookPath("gcc")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("gcc not found in CI; the creation comparison must run")
		}
		t.Skip("gcc not found; skipping the creation comparison")
	}

	src := filepath.Join(repoRoot(t), "reference", "tools", "startoracle.c")
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("%s not found: %v", src, err)
	}

	bin := filepath.Join(t.TempDir(), "startoracle")
	build := exec.Command(gcc, "-O2", "-Wall", "-Werror", "-o", bin, src)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compiling the oracle: %v\n%s", err, out)
	}
	return bin
}

func runStartOracle(t *testing.T, bin string, seed uint64, class int32, count int) []startedCharacter {
	t.Helper()

	out, err := exec.Command(bin,
		strconv.FormatUint(seed, 10),
		strconv.Itoa(int(class)),
		strconv.Itoa(count)).Output()
	if err != nil {
		t.Fatalf("running the oracle: %v", err)
	}

	var chars []startedCharacter
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 12 {
			t.Fatalf("oracle line %q has %d fields, want 12", line, len(fields))
		}
		var n [12]int32
		for i, f := range fields {
			v, err := strconv.Atoi(f)
			if err != nil {
				t.Fatalf("unparseable oracle field %q in %q", f, line)
			}
			n[i] = int32(v)
		}
		chars = append(chars, startedCharacter{
			str: n[0], strAdd: n[1], intel: n[2], wis: n[3], dex: n[4],
			con: n[5], cha: n[6], maxHit: n[7], maxMana: n[8], maxMove: n[9],
			practices: n[10], next: n[11],
		})
	}
	return chars
}
