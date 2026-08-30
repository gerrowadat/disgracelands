// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const oeditSource = "../../reference/moderncserver/src/oedit.c"

// TestValueSlotsMatchOedit re-parses which slots OasisOLC prompts for, per
// item type, and requires this package's named slots to be the same set.
//
// This is the table-re-parsing rule (CLAUDE.md) applied to a table that is
// not written as one. The C has no data anywhere saying "a wand's spell is
// value 3"; the only statement of it is the shape of oedit_disp_valN_menu's
// switch, so the switch is what this reads. A constant renumbered by hand
// then fails a test instead of quietly making every wand cast the wrong
// spell.
//
// Two documented exceptions, both real divergences rather than slack:
//
//   - A weapon's value 0. oedit prompts "Modifier to Hitroll" for it and
//     its own comment says "This doesn't seem to be used if I remembe
//     right" [sic]. Nothing in the C reads it and nothing here does, so it
//     has no name.
//   - A container's value 3. oedit's value-4 menu has no ITEM_CONTAINER
//     case at all, so a builder is never asked — but make_corpse writes 1
//     there (fight.c:319) to mark a corpse. The slot is real and OasisOLC
//     does not know about it.
func TestValueSlotsMatchOedit(t *testing.T) {
	prompted := parseOeditValueMenus(t)
	if len(prompted) < 8 {
		t.Fatalf("parsed only %d item types out of oedit.c", len(prompted))
	}

	// The slots this package names, by the constants themselves rather than
	// by their numbers — so renumbering a constant moves this list with it
	// and the disagreement shows up against the C rather than here.
	named := map[string][]int{
		"ITEM_WEAPON":    {weaponDiceCount, weaponDiceSize, weaponDamage},
		"ITEM_ARMOR":     {armorACApply},
		"ITEM_LIGHT":     {lightHours},
		"ITEM_WAND":      {chargesLevel, chargesMax, chargesRemaining, chargesSpell},
		"ITEM_STAFF":     {chargesLevel, chargesMax, chargesRemaining, chargesSpell},
		"ITEM_DRINKCON":  {drinkCapacity, drinkFilled, drinkLiquid, drinkPoisoned},
		"ITEM_FOUNTAIN":  {drinkCapacity, drinkFilled, drinkLiquid, drinkPoisoned},
		"ITEM_FOOD":      {foodFilling, foodPoisoned},
		"ITEM_MONEY":     {moneyCoins},
		"ITEM_CONTAINER": {containerCapacity, containerFlagsValue, containerKeyValue, containerCorpseValue},
	}

	// cName -> slots oedit prompts for but this package deliberately does
	// not name, and the other way round.
	unnamedOnPurpose := map[string][]int{"ITEM_WEAPON": {0}}
	unpromptedOnPurpose := map[string][]int{"ITEM_CONTAINER": {3}}

	for cName, want := range named {
		got, ok := prompted[cName]
		if !ok {
			t.Errorf("%s names slots %v here and oedit prompts for none", cName, want)
			continue
		}
		got = slices.DeleteFunc(got, func(slot int) bool {
			return slices.Contains(unnamedOnPurpose[cName], slot)
		})
		want = slices.DeleteFunc(slices.Clone(want), func(slot int) bool {
			return slices.Contains(unpromptedOnPurpose[cName], slot)
		})
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: this package names slots %v, oedit prompts for %v",
				cName, want, got)
		}
	}

	// Scroll and potion use all four and are named nowhere, on purpose:
	// their slots 1-3 are three spells rather than one, which is the case
	// §4.3 says stays raw. If oedit ever stopped prompting for them, the
	// reasoning for leaving them out would have changed.
	for _, cName := range []string{"ITEM_SCROLL", "ITEM_POTION"} {
		if _, ok := named[cName]; ok {
			t.Errorf("%s has named slots; the multi-spell case is meant to stay raw", cName)
		}
		if got := prompted[cName]; !slices.Equal(got, []int{0, 1, 2, 3}) {
			t.Errorf("%s: oedit prompts for %v, want all four", cName, got)
		}
	}
}

// parseOeditValueMenus reads oedit_disp_val1_menu through val4 and returns,
// per ITEM_* name, the value slots a builder is prompted for.
//
// The switches have three shapes and all three matter: a case that sends a
// prompt or calls a sub-menu (the type uses that slot), a case that calls
// another valN menu (a *jump* — ITEM_LIGHT skips slots 0 and 1, ITEM_FOOD
// skips 1 and 2, and both say so in a comment), and a case that falls to
// `default: oedit_disp_menu(d)` by not appearing at all.
func parseOeditValueMenus(t *testing.T) map[string][]int {
	t.Helper()

	src, err := os.ReadFile(oeditSource)
	if err != nil {
		t.Fatalf("reading oedit.c: %v", err)
	}
	text := string(src)

	out := map[string][]int{}
	for menu := 1; menu <= 4; menu++ {
		slot := menu - 1
		body := funcBody(t, text, "void oedit_disp_val"+strconv.Itoa(menu)+"_menu(struct descriptor_data *d)")

		var pending []string
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(cLineComment.ReplaceAllString(line, ""))
			switch {
			case strings.HasPrefix(line, "case ITEM_"):
				pending = append(pending, strings.TrimSuffix(strings.TrimPrefix(line, "case "), ":"))
			case strings.HasPrefix(line, "oedit_disp_val") && strings.Contains(line, "_menu(d);"):
				// A jump to another slot's menu: this slot is unused by
				// these types, and the menu it jumps to lists them itself.
				pending = nil
			case strings.HasPrefix(line, "SEND_TO_Q(") || strings.HasPrefix(line, "oedit_disp_") ||
				strings.HasPrefix(line, "oedit_liquid_type("):
				for _, name := range pending {
					out[name] = append(out[name], slot)
				}
				pending = nil
			case line == "break;":
				// A case with no prompt at all — ITEM_NOTE's "supposed to
				// be language, but it's unused".
				pending = nil
			case strings.HasPrefix(line, "default:"):
				pending = nil
			}
		}
	}
	for _, slots := range out {
		slices.Sort(slots)
	}
	return out
}

// funcBody returns the text between a C function's signature and the first
// line that is a lone closing brace.
func funcBody(t *testing.T, src, signature string) string {
	t.Helper()
	i := strings.Index(src, signature)
	if i < 0 {
		t.Fatalf("no %s in oedit.c", signature)
	}
	body := src[i+len(signature):]
	if end := strings.Index(body, "\n}\n"); end >= 0 {
		return body[:end]
	}
	t.Fatalf("unterminated %s", signature)
	return ""
}

// TestTypedValuesRoundTripThroughTheGrouped accessors, which is the property
// values.go depends on now that it encodes through them.
func TestTypedValuesRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name   string
		obj    *Object
		encode func(*Object) [NumObjValues]int32
	}{
		{"weapon", &Object{Type: ItemWeapon, Values: [NumObjValues]int32{0, 3, 5, AttackSlash}},
			func(o *Object) [NumObjValues]int32 { v, _ := o.WeaponValues(); return ValuesOfWeapon(v) }},
		{"armor", &Object{Type: ItemArmor, Values: [NumObjValues]int32{7, 0, 0, 0}},
			func(o *Object) [NumObjValues]int32 { v, _ := o.ArmorValues(); return ValuesOfArmor(v) }},
		{"light", &Object{Type: ItemLight, Values: [NumObjValues]int32{0, 0, -1, 0}},
			func(o *Object) [NumObjValues]int32 { v, _ := o.LightValues(); return ValuesOfLight(v) }},
		{"wand", &Object{Type: ItemWand, Values: [NumObjValues]int32{20, 3, 2, SpellMagicMissile.Number()}},
			func(o *Object) [NumObjValues]int32 { v, _ := o.ChargesValues(); return ValuesOfCharges(v) }},
		{"drink", &Object{Type: ItemDrinkCon, Values: [NumObjValues]int32{20, 12, LiquidBeer.Number(), 1}},
			func(o *Object) [NumObjValues]int32 { v, _ := o.DrinkValues(); return ValuesOfDrink(v) }},
		// A key of -1 is "no key" on disk and a key of 0 is a different
		// byte; both must survive, which normalising through ContainerKey()
		// did not (seventeen stock objects).
		{"container, no key", &Object{Type: ItemContainer, Values: [NumObjValues]int32{100, 5, -1, 0}},
			func(o *Object) [NumObjValues]int32 { v, _ := o.ContainerValues(); return ValuesOfContainer(v) }},
		{"container, key zero", &Object{Type: ItemContainer, Values: [NumObjValues]int32{100, 5, 0, 0}},
			func(o *Object) [NumObjValues]int32 { v, _ := o.ContainerValues(); return ValuesOfContainer(v) }},
		{"corpse", &Object{Type: ItemContainer, Values: [NumObjValues]int32{0, 0, 0, corpseIdentifier}},
			func(o *Object) [NumObjValues]int32 { v, _ := o.ContainerValues(); return ValuesOfContainer(v) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.encode(tc.obj); got != tc.obj.Values {
				t.Errorf("round trip gave %v, want %v", got, tc.obj.Values)
			}
		})
	}
}

// TestTheAccessorsRefuseTheWrongType, since every one of them is reached
// from a switch on Type and a silent zero value would be the worst answer.
func TestTheAccessorsRefuseTheWrongType(t *testing.T) {
	obj := &Object{Type: ItemArmor, Values: [NumObjValues]int32{1, 2, 3, 4}}
	if _, ok := obj.WeaponValues(); ok {
		t.Error("armor answered WeaponValues")
	}
	if _, ok := obj.DrinkValues(); ok {
		t.Error("armor answered DrinkValues")
	}
	if _, ok := obj.ChargesValues(); ok {
		t.Error("armor answered ChargesValues")
	}
	// A scroll is not a wand, however alike their slots look.
	scroll := &Object{Type: ItemScroll, Values: [NumObjValues]int32{20, 1, 2, 3}}
	if _, ok := scroll.ChargesValues(); ok {
		t.Error("a scroll answered ChargesValues; its slots 1-3 are three spells")
	}
	// And nil is nothing.
	var none *Object
	if _, ok := none.WeaponValues(); ok {
		t.Error("a nil object answered WeaponValues")
	}
}

// TestDamageTypePromotesToTheMessageScale. The two scales are the whole
// reason DamageType exists as its own type: fifteen weapon verbs live at
// 0-14, and the table the messages are looked up in is keyed at 300-314.
func TestDamageTypePromotesToTheMessageScale(t *testing.T) {
	if got := DamageType(AttackSlash).AttackType(); got != TypeHit+AttackSlash {
		t.Errorf("slash promotes to %d, want %d", got, TypeHit+AttackSlash)
	}
	if got := DamageType(AttackHit).Number(); got != AttackHit {
		t.Errorf("Number() is %d, want the unpromoted %d", got, AttackHit)
	}
	// Every named attack type has a name in the yaml table at the
	// unpromoted scale, which is the scale a weapon's value 3 is in.
	for attack := AttackHit; attack <= AttackStab; attack++ {
		if _, ok := NameByValue(DamageType(attack).Number(), YamlAttackTypeNames()); !ok {
			t.Errorf("attack type %d has no yaml name", attack)
		}
	}
}
