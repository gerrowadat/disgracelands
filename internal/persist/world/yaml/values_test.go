// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"context"
	"testing"

	"github.com/gerrowadat/disgracelands/internal/game"
	"github.com/gerrowadat/disgracelands/internal/persist/world"
	"github.com/gerrowadat/disgracelands/internal/persist/world/classic"
)

// TestTypedValuesRoundTripsRealObjects loads every object in the real
// corpus and, for every one whose type gets a typed form (§4.3), checks
// that decoding to the typed struct and back reproduces the original
// values exactly. Objects with junk in an unused slot are expected to fall
// back to the raw form (ok=false) rather than losing that junk, and this
// asserts that fallback happens rather than a silent wrong answer.
func TestTypedValuesRoundTripsRealObjects(t *testing.T) {
	src, err := classic.New(world.Config{Dir: "../../../../examples/stock/binary/world"})
	if err != nil {
		t.Fatalf("open classic source: %v", err)
	}
	defer func() { _ = src.Close() }()

	w, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	typed, raw := 0, 0
	for _, o := range w.Objects {
		// unusedNonzero, when !ok, distinguishes "the type doesn't get a
		// typed form at all" from "junk in a slot the type doesn't use" —
		// both fall back to the same always-accepted raw form, so this test
		// doesn't need to tell them apart.
		result, _, ok := TypedValues(o.Type, o.Values)
		if !ok {
			raw++
			continue
		}
		typed++

		var back [game.NumObjValues]int32
		var backOK bool
		switch v := result.(type) {
		case WeaponValues:
			back, backOK = ValuesFromWeapon(v)
		case ArmorValues:
			back, backOK = ValuesFromArmor(v), true
		case ContainerValues:
			back, backOK = ValuesFromContainer(v), true
		case DrinkValues:
			back, backOK = ValuesFromDrink(v)
		case LightValues:
			back, backOK = ValuesFromLight(v), true
		case ChargesValues:
			back, backOK = ValuesFromCharges(v)
		default:
			t.Fatalf("object #%d: unexpected typed value %T", o.Vnum, result)
		}
		if !backOK {
			t.Errorf("object #%d: typed form %+v did not convert back", o.Vnum, result)
			continue
		}
		if back != o.Values {
			t.Errorf("object #%d (type %d): round trip mismatch: got %v, want %v",
				o.Vnum, o.Type, back, o.Values)
		}
	}
	t.Logf("%d objects took a typed form, %d stayed raw (unsupported type, unused-slot junk, or an unnamed value)", typed, raw)
}

// TestDrinkPoisonOutsideZeroOrOneFallsBackToRaw guards a value the typed
// schema reads but cannot carry back. `poisoned` is a bool because the C
// tests value 3 as a truth value and nothing more, but the file holds an
// int and a builder's file holds one that is neither 0 nor 1 — folding it
// to 1 rewrites world data on the way through the converter. §4.3's raw
// fallback is the existing answer for a value the typed form would lose,
// and this is a case of it, not an exception to it.
func TestDrinkPoisonOutsideZeroOrOneFallsBackToRaw(t *testing.T) {
	liquid, ok := game.ValueByName("blood", game.YamlLiquidNames())
	if !ok {
		t.Fatal("no liquid named blood")
	}

	for _, tc := range []struct {
		name     string
		poison   int32
		wantRaw  bool
		wantBool bool
	}{
		{"unpoisoned", 0, false, false},
		{"poisoned", 1, false, true},
		{"poisoned with a number that is not one", 5, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := [game.NumObjValues]int32{6, 6, liquid, tc.poison}
			typed, unusedNonzero, ok := TypedValues(game.ItemDrinkCon, values)
			if tc.wantRaw {
				if ok {
					t.Fatalf("expected the raw fallback, got typed %+v", typed)
				}
				if !unusedNonzero {
					t.Error("expected unusedNonzero, so the caller writes the raw form")
				}
				return
			}
			if !ok {
				t.Fatalf("expected a typed form, got the raw fallback")
			}
			drink, isDrink := typed.(DrinkValues)
			if !isDrink {
				t.Fatalf("expected DrinkValues, got %T", typed)
			}
			if drink.Poisoned != tc.wantBool {
				t.Errorf("poisoned: got %v, want %v", drink.Poisoned, tc.wantBool)
			}
			back, backOK := ValuesFromDrink(drink)
			if !backOK || back != values {
				t.Errorf("round trip: got %v (ok=%v), want %v", back, backOK, values)
			}
		})
	}
}
