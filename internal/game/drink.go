// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strings"

// The sixteen drinks and what each does to you, from constants.c:424 and
// :473.
//
// The table is three numbers per drink — drunkenness, fullness, thirst — and
// they are per four units drunk, which is why every use divides by four.
// Salt water and blood have *negative* thirst values: drinking them makes you
// thirstier, which is the joke and also a real hazard on a long walk.

// The sixteen liquids, from structs.h:426. Only two are named anywhere in the
// code — water, which create water makes, and slime, which it makes instead
// when it goes wrong.
const (
	LiquidWater      int32 = 0
	LiquidBeer       int32 = 1
	LiquidWine       int32 = 2
	LiquidAle        int32 = 3
	LiquidDarkAle    int32 = 4
	LiquidWhisky     int32 = 5
	LiquidLemonade   int32 = 6
	LiquidFirebrt    int32 = 7
	LiquidLocalSpec  int32 = 8
	LiquidSlime      int32 = 9
	LiquidMilk       int32 = 10
	LiquidTea        int32 = 11
	LiquidCoffee     int32 = 12
	LiquidBlood      int32 = 13
	LiquidSaltWater  int32 = 14
	LiquidClearWater int32 = 15
)

// drinkNames are drinks[] (constants.c:424).
var drinkNames = [16]string{
	"water",
	"beer",
	"wine",
	"ale",
	"dark ale",
	"whisky",
	"lemonade",
	"firebreather",
	"local speciality",
	"slime mold juice",
	"milk",
	"tea",
	"coffee",
	"blood",
	"salt water",
	"clear water",
}

// drinkEffects are drink_aff[] (constants.c:473), indexed by drink and then
// by condition in the record's order: drunk, full, thirst.
var drinkEffects = [16][3]int32{
	{0, 1, 10}, // water
	{3, 2, 5},  // beer
	{5, 2, 5},  // wine
	{2, 2, 5},  // ale
	{1, 2, 5},  // dark ale
	{6, 1, 4},  // whisky
	{0, 1, 8},  // lemonade
	{10, 0, 0}, // firebreather: all drunkenness, no use at all
	{3, 3, 3},  // local speciality
	{0, 4, -8}, // slime mold juice: filling, and it makes you thirstier
	{0, 3, 6},  // milk
	{0, 1, 6},  // tea
	{0, 1, 6},  // coffee
	{0, 2, -1}, // blood
	{0, 1, -2}, // salt water
	{0, 0, 13}, // clear water
}

// drinkKeywords are drinknames[] (constants.c:443): the *keyword* form, which
// is a different table from the display names above and shorter than it looks.
//
// Dark ale is keyed as "ale", slime mold juice as "juice", salt water as
// "salt", and clear water as "water" — the same word as plain water. A
// container gets the keyword of whatever is in it appended to its name, which
// is how `drink water` finds a canteen, and it is taken off again when the
// liquid changes.
var drinkKeywords = [16]string{
	"water", "beer", "wine", "ale", "ale", "whisky", "lemonade",
	"firebreather", "local", "juice", "milk", "tea", "coffee",
	"blood", "salt", "water",
}

// DrinkKeyword returns the keyword a liquid contributes to its container's
// name.
func DrinkKeyword(liquid int32) string {
	if liquid < 0 || int(liquid) >= len(drinkKeywords) {
		return ""
	}
	return drinkKeywords[liquid]
}

// NameFromDrinkCon takes the current liquid's keyword off a container,
// porting name_from_drinkcon.
//
// It removes every keyword that *starts with* the liquid's name, which is the
// C's `strn_cmp` over the keyword length — so emptying a bottle of "ale" also
// strips a keyword like "alembic" if some builder gave it one. Reproduced.
func NameFromDrinkCon(o *Object) {
	if o == nil || (o.Type != ItemDrinkCon && o.Type != ItemFountain) {
		return
	}
	liquid := DrinkKeyword(o.Values[2])
	if liquid == "" {
		return
	}

	kept := make([]string, 0, 8)
	for _, word := range strings.Fields(o.Keywords) {
		if strings.HasPrefix(strings.ToLower(word), liquid) {
			continue
		}
		kept = append(kept, word)
	}
	o.Keywords = strings.Join(kept, " ")
}

// NameToDrinkCon appends a liquid's keyword to a container, porting
// name_to_drinkcon.
func NameToDrinkCon(o *Object, liquid int32) {
	if o == nil || (o.Type != ItemDrinkCon && o.Type != ItemFountain) {
		return
	}
	if word := DrinkKeyword(liquid); word != "" {
		o.Keywords = strings.TrimSpace(o.Keywords + " " + word)
	}
}

// DrinkName returns a liquid's name.
func DrinkName(liquid int32) string {
	if liquid < 0 || int(liquid) >= len(drinkNames) {
		return "something"
	}
	return drinkNames[liquid]
}

// DrinkEffect returns what one unit-quarter of a liquid does to each
// condition, in the record's order: drunk, full, thirst.
func DrinkEffect(liquid int32) [3]int32 {
	if liquid < 0 || int(liquid) >= len(drinkEffects) {
		return [3]int32{}
	}
	return drinkEffects[liquid]
}

// DrinkAmount is how much a swallow takes, porting the calculation in
// do_drink.
//
// A drink with any drunkenness in it is measured against how thirsty you
// are — `(25 - thirst) / drunkenness` — so a parched character downs far more
// whisky than a comfortable one, and gets correspondingly drunker. A drink
// with no drunkenness is a plain number(3, 10).
func DrinkAmount(liquid, thirst int32, r interface{ Number(int32, int32) int32 }) int32 {
	effect := DrinkEffect(liquid)
	if effect[CondDrunk] > 0 {
		return (25 - thirst) / effect[CondDrunk]
	}
	return r.Number(3, 10)
}
