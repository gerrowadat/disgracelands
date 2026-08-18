// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// The sixteen drinks and what each does to you, from constants.c:424 and
// :473.
//
// The table is three numbers per drink — drunkenness, fullness, thirst — and
// they are per four units drunk, which is why every use divides by four.
// Salt water and blood have *negative* thirst values: drinking them makes you
// thirstier, which is the joke and also a real hazard on a long walk.

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
