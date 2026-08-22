// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package native

import (
	"strconv"

	"github.com/gerrowadat/disgracelands/internal/game"
)

// Enhanced mobiles, §4.7: the 'E' format's trailing `Key: value` lines
// become a closed, typed `abilities:` map. game.MobDef.Especs keeps them
// raw (Key/Value strings) because interpreting them needs the ability
// tables, which is exactly what this file is — the closed set is
// BareHandAttack, Str, StrAdd, Int, Wis, Dex, Con, Cha, transcribed from
// the espec keys the C reader actually recognises.

// Abilities is the typed form of a mobile's espec block.
type Abilities struct {
	BareHandAttack int32 `yaml:"bare_hand_attack,omitempty"`
	Str            int32 `yaml:"str,omitempty"`
	StrAdd         int32 `yaml:"str_add,omitempty"`
	Int            int32 `yaml:"int,omitempty"`
	Wis            int32 `yaml:"wis,omitempty"`
	Dex            int32 `yaml:"dex,omitempty"`
	Con            int32 `yaml:"con,omitempty"`
	Cha            int32 `yaml:"cha,omitempty"`
}

// especKeys map the C's espec key spelling to Abilities' native field name.
var especKeys = map[string]string{
	"BareHandAttack": "bare_hand_attack",
	"Str":            "str",
	"StrAdd":         "str_add",
	"Int":            "int",
	"Wis":            "wis",
	"Dex":            "dex",
	"Con":            "con",
	"Cha":            "cha",
}

// AbilitiesFromEspecs converts a mobile's raw espec lines into the typed
// form, per §4.7's "an unrecognised key is a load error" (the closed-set
// rule §4.1 states generally): unknown reports every Key that isn't in the
// closed set, or whose Value isn't an integer.
func AbilitiesFromEspecs(especs []game.Espec) (abilities Abilities, unknown []string) {
	for _, e := range especs {
		name, known := especKeys[e.Key]
		if !known {
			unknown = append(unknown, e.Key)
			continue
		}
		n, err := strconv.Atoi(e.Value)
		if err != nil {
			unknown = append(unknown, e.Key)
			continue
		}
		switch name {
		case "bare_hand_attack":
			abilities.BareHandAttack = int32(n) //nolint:gosec // world-data-scale
		case "str":
			abilities.Str = int32(n) //nolint:gosec // world-data-scale
		case "str_add":
			abilities.StrAdd = int32(n) //nolint:gosec // world-data-scale
		case "int":
			abilities.Int = int32(n) //nolint:gosec // world-data-scale
		case "wis":
			abilities.Wis = int32(n) //nolint:gosec // world-data-scale
		case "dex":
			abilities.Dex = int32(n) //nolint:gosec // world-data-scale
		case "con":
			abilities.Con = int32(n) //nolint:gosec // world-data-scale
		case "cha":
			abilities.Cha = int32(n) //nolint:gosec // world-data-scale
		}
	}
	return abilities, unknown
}

// EspecsFromAbilities is AbilitiesFromEspecs' inverse, in the C's own key
// order (bitnames_test.go-style tables are re-derivable; espec order is
// not data, so a fixed order here just keeps writer output deterministic).
func EspecsFromAbilities(a Abilities) []game.Espec {
	var out []game.Espec
	add := func(key string, v int32) {
		if v != 0 {
			out = append(out, game.Espec{Key: key, Value: strconv.Itoa(int(v))})
		}
	}
	add("BareHandAttack", a.BareHandAttack)
	add("Str", a.Str)
	add("StrAdd", a.StrAdd)
	add("Int", a.Int)
	add("Wis", a.Wis)
	add("Dex", a.Dex)
	add("Con", a.Con)
	add("Cha", a.Cha)
	return out
}

// IsZero reports whether a carries no abilities at all — the "simple/
// enhanced distinction disappears: a mobile either has an abilities block
// or it does not" rule from §4.7.
func (a Abilities) IsZero() bool {
	return a == Abilities{}
}
