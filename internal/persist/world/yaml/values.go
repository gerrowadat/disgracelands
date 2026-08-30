// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package yaml

import (
	"strconv"
	"strings"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Typed object values, §4.3 of docs/design/data-format.md. Which of the
// four Values slots a given object type reads, and what each one means, is
// not written down anywhere in the C source as data — it is the shape of
// OasisOLC's per-type oedit_disp_valN_menu switch (reference/moderncserver/
// src/oedit.c:294-441), transcribed here as the authority for the slot
// layout because it is the closest thing to a specification that exists.
//
// The rule from §4.3: emit the typed form only when every slot the type
// does not use is genuinely zero. container.go's containerCorpseValue is
// the sharpest example of why — a corpse is a container whose fourth value
// is -1, not 0, and -1 must fall back to the raw form rather than being
// silently rounded away.
//
// Only the five types §4.3 works through by example get a typed form here.
// Every other type — including the multi-spell scroll/potion case, which
// the proposal never works out in the same detail — always uses the raw
// `values:` form. That is a deliberate scope boundary, not an oversight.

// WeaponValues is a weapon's dice and damage type (oedit.c:360-431).
// Values[0] ("modifier to hitroll... doesn't seem to be used") is the slot
// this type does not use; the typed form requires it to be zero.
type WeaponValues struct {
	// Dice is "NdS" — §4.3's `dice: 3d5` — with no bonus term, because a
	// weapon's value slots have none: unlike game.Dice (used for a mobile's
	// hit/damage dice, which do carry one), Values[1]/Values[2] here are
	// just the count and size.
	Dice       string `yaml:"dice"`
	DamageType string `yaml:"damage_type"`
}

// ContainerValues is a container's capacity, flags and key (oedit.c:322-
// 373, container.go's containerCapacity/containerFlagsValue/
// containerKeyValue). Values[3] (containerCorpseValue) is the unused slot;
// a corpse's -1 there is exactly what sends it to the raw form instead.
type ContainerValues struct {
	Capacity  int32 `yaml:"capacity"`
	Closeable bool  `yaml:"closeable,omitempty"`
	Pickproof bool  `yaml:"pickproof,omitempty"`
	Closed    bool  `yaml:"closed,omitempty"`
	Locked    bool  `yaml:"locked,omitempty"`
	Key       int32 `yaml:"key"`
}

// DrinkValues is a drink container or fountain's capacity, fill, liquid and
// poison state (oedit.c:325-437). No unused slot: all four are read.
type DrinkValues struct {
	Capacity int32  `yaml:"capacity"`
	Current  int32  `yaml:"current"`
	Liquid   string `yaml:"liquid"`
	Poisoned bool   `yaml:"poisoned,omitempty"`
}

// LightValues is a light source's remaining hours (oedit.c:301-393).
// Values[0] and Values[1] are unused ("jump to 2" — the comment's own
// words); Values[3] is unused too, since oedit's value-4 menu has no LIGHT
// case at all and falls through to the generic menu.
type LightValues struct {
	Hours int32 `yaml:"hours"`
}

// ChargesValues is a wand or staff's spell, level and charge count
// (oedit.c:307-438). Scroll/potion share the slot layout for level and
// spell but carry three spells instead of a charge count, which is the
// multi-spell case this file deliberately does not attempt.
type ChargesValues struct {
	Spell     string `yaml:"spell"`
	Level     int32  `yaml:"level"`
	Max       int32  `yaml:"max"`
	Remaining int32  `yaml:"remaining"`
}

// ArmorValues is armor's AC apply amount (oedit.c:319-320). Values[1..3] are
// unused.
type ArmorValues struct {
	ACApply int32 `yaml:"ac_apply"`
}

// TypedValues decodes obj.Values into whichever typed form its Type
// supports, or reports ok=false when a slot the type does not use is
// nonzero — the "junk in the unused slot" case §4.3 says must fall back to
// the raw form rather than silently discard what is there.
func TypedValues(objType game.ItemType, values [game.NumObjValues]int32) (typed any, unusedNonzero bool, ok bool) {
	switch objType {
	case game.ItemWeapon:
		if values[0] != 0 {
			return nil, true, false
		}
		attackName, known := game.NameByValue(values[3], game.YamlAttackTypeNames())
		if !known {
			return nil, false, false
		}
		return WeaponValues{
			Dice: formatDice(values[1], values[2]), DamageType: attackName,
		}, false, true

	case game.ItemArmor:
		if values[1] != 0 || values[2] != 0 || values[3] != 0 {
			return nil, true, false
		}
		return ArmorValues{ACApply: values[0]}, false, true

	case game.ItemContainer:
		if values[3] != 0 {
			return nil, true, false
		}
		flags := game.SetFromRaw[game.ContainerFlag](uint64(uint32(values[1]))) //nolint:gosec // four-bit container flag field, reinterpreted not truncated
		return ContainerValues{
			Capacity:  values[0],
			Closeable: flags.Has(game.ContCloseable),
			Pickproof: flags.Has(game.ContPickproof),
			Closed:    flags.Has(game.ContClosed),
			Locked:    flags.Has(game.ContLocked),
			Key:       values[2],
		}, false, true

	case game.ItemDrinkCon:
		// The C reads value 3 as a truth value and nothing else
		// (act.item.c's drink handler tests it with a bare if), so `poisoned`
		// is the honest name for it — but the *file* can hold any int, a
		// builder's file does, and folding 5 down to 1 is a silent edit to
		// world data whether or not the game can tell the difference. The
		// raw form is §4.3's answer for exactly this: a value the typed
		// schema cannot carry back unchanged is not typed.
		if values[3] != 0 && values[3] != 1 {
			return nil, true, false
		}
		liquid, known := game.NameByValue(values[2], game.YamlLiquidNames())
		if !known {
			return nil, false, false
		}
		return DrinkValues{
			Capacity: values[0], Current: values[1], Liquid: liquid,
			Poisoned: values[3] != 0,
		}, false, true

	case game.ItemLight:
		if values[0] != 0 || values[1] != 0 || values[3] != 0 {
			return nil, true, false
		}
		return LightValues{Hours: values[2]}, false, true

	case game.ItemWand, game.ItemStaff:
		return ChargesValues{
			Level: values[0], Max: values[1], Remaining: values[2],
			Spell: formatSpellNumber(values[3]),
		}, false, true
	}
	return nil, false, false
}

// ValuesFromWeapon, ValuesFromArmor, ValuesFromContainer, ValuesFromDrink,
// ValuesFromLight and ValuesFromCharges are TypedValues' inverses, used by
// the reader when a zone file spells out a typed block instead of raw
// values. Each reports ok=false for a name ParseBitNames/ValueByName
// wouldn't recognise, so the caller can report it as a load error rather
// than silently write zero.

func ValuesFromWeapon(v WeaponValues) (values [game.NumObjValues]int32, ok bool) {
	num, size, dok := parseDice(v.Dice)
	attack, aok := game.ValueByName(v.DamageType, game.YamlAttackTypeNames())
	if !dok || !aok {
		return values, false
	}
	values[1], values[2], values[3] = num, size, attack
	return values, true
}

func ValuesFromArmor(v ArmorValues) [game.NumObjValues]int32 {
	return [game.NumObjValues]int32{v.ACApply, 0, 0, 0}
}

func ValuesFromContainer(v ContainerValues) [game.NumObjValues]int32 {
	var flags game.ContainerFlagSet
	if v.Closeable {
		flags = flags.With(game.ContCloseable)
	}
	if v.Pickproof {
		flags = flags.With(game.ContPickproof)
	}
	if v.Closed {
		flags = flags.With(game.ContClosed)
	}
	if v.Locked {
		flags = flags.With(game.ContLocked)
	}
	return [game.NumObjValues]int32{v.Capacity, int32(flags.Raw()), v.Key, 0} //nolint:gosec // four-bit field
}

func ValuesFromDrink(v DrinkValues) (values [game.NumObjValues]int32, ok bool) {
	liquid, lok := game.ValueByName(v.Liquid, game.YamlLiquidNames())
	if !lok {
		return values, false
	}
	poisoned := int32(0)
	if v.Poisoned {
		poisoned = 1
	}
	return [game.NumObjValues]int32{v.Capacity, v.Current, liquid, poisoned}, true
}

func ValuesFromLight(v LightValues) [game.NumObjValues]int32 {
	return [game.NumObjValues]int32{0, 0, v.Hours, 0}
}

func ValuesFromCharges(v ChargesValues) (values [game.NumObjValues]int32, ok bool) {
	spell, sok := parseSpellNumber(v.Spell)
	if !sok {
		return values, false
	}
	return [game.NumObjValues]int32{v.Level, v.Max, v.Remaining, spell}, true
}

// RawValues decodes obj.Values into the untyped [4]int32 form every object
// type accepts, per §4.3's "always accepted, always preserved exactly".
func RawValues(values [game.NumObjValues]int32) [game.NumObjValues]int32 { return values }

// formatDice and parseDice render/parse a weapon's "NdS" notation — no
// bonus term, see WeaponValues.
func formatDice(num, size int32) string {
	return strconv.Itoa(int(num)) + "d" + strconv.Itoa(int(size))
}

func parseDice(s string) (num, size int32, ok bool) {
	i := strings.IndexByte(s, 'd')
	if i < 0 {
		return 0, 0, false
	}
	n, err1 := strconv.Atoi(s[:i])
	sz, err2 := strconv.Atoi(s[i+1:])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return int32(n), int32(sz), true //nolint:gosec // world-data-scale dice values
}

// parseSpellNumber and formatSpellNumber are game.SpellNumberFromNameOrNumber
// and game.SpellNameOrNumber under this file's own naming — kept as thin
// wrappers rather than replaced at every call site, and shared (not
// duplicated) with internal/persist/player/yaml, which needs exactly the
// same name<->number rule for a player's skills.
//
// They return and take the stored int32 rather than a game.SpellID, because
// their callers are object value slots -- a wand's spell lives in
// Values[3] -- and those are still [4]int32 until step 3 types them.
func parseSpellNumber(s string) (int32, bool) {
	n, ok := game.SpellNumberFromNameOrNumber(s)
	return n.Number(), ok
}
func formatSpellNumber(n int32) string { return game.SpellNameOrNumber(game.SpellID(n)) }
