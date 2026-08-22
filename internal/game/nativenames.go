// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// The identifiers the native data format (docs/proposals/data-format.md §4)
// writes for every flag set, sector, position and item type the world files
// carry — lower_snake_case, and *not* the display strings in bitnames.go and
// object.go. §4.1 is explicit those cannot be reused: they are positional,
// carry spaces and hyphens ("DET-ALIGN", "LIQ CONTAINER"), and are printed to
// players by `identify`, so renaming one to suit a file format would change
// what a spell inspection shows.
//
// Every table here is the same length, in the same bit or value order, as
// its bitnames.go/object.go counterpart — nativenames_test.go asserts that
// directly, re-deriving nothing from the C, because the two Go tables are
// the ones that must never drift apart. A "" entry marks a slot with no
// name in *either* table (constants.c's own gaps, e.g. the room bits' "*"
// placeholder and the affect bits' UNUSED slot): NameBits/ParseBitNames
// treat that as unnamed rather than inventing an identifier for it, so it
// round-trips through flags_raw instead (§4.1).

// nativeRoomFlagNames match roomBitNames bit for bit.
var nativeRoomFlagNames = []string{
	"dark", "death", "no_mob", "indoors", "peaceful", "soundproof", "no_track",
	"no_magic", "tunnel", "private", "godroom", "house", "house_crash",
	"atrium", "olc", "", "good_regen", "can_quit", "pkill",
}

// nativeExitDoorNames name the raw 0/1/2 door byte the file always held —
// §4.2's "door: pickproof # absent = no door; regular | pickproof". Index 0
// (no door) has no name because the key is simply absent.
var nativeExitDoorNames = []string{"", "regular", "pickproof"}

// nativeDoorStateNames are the 'D' reset command's third argument
// (game.DoorOpen/DoorClosed/DoorLocked), used by §4.4's `state:` field.
var nativeDoorStateNames = []string{"open", "closed", "locked"}

// nativeSectorNames match sectorNames value for value.
var nativeSectorNames = []string{
	"inside", "city", "field", "forest", "hills", "mountains", "water_swim",
	"water_noswim", "flying", "underwater",
}

// nativePositionNames match positionNames value for value.
var nativePositionNames = []string{
	"dead", "mortally_wounded", "incapacitated", "stunned", "sleeping",
	"resting", "sitting", "fighting", "standing",
}

// nativeSexNames match genderNames value for value.
var nativeSexNames = []string{"neutral", "male", "female"}

// nativeMobActFlagNames match actionBitNames bit for bit. "isnpc" is never
// written by the native writer — it is force-set on every mobile regardless
// of the file, the same rule MobIsNPC documents — but it is still named so a
// file that spells it out explicitly loads without complaint rather than
// tripping the "unknown name" error.
var nativeMobActFlagNames = []string{
	"spec", "sentinel", "scavenger", "isnpc", "aware", "aggressive",
	"stay_zone", "wimpy", "aggr_evil", "aggr_good", "aggr_neutral", "memory",
	"helper", "no_charm", "no_summon", "no_sleep", "no_bash", "no_blind",
	"dead",
}

// nativeAffectFlagNames match affectBitNames bit for bit. Used for both a
// mobile's `affected:` list and a player's `flags.affected:` list — both are
// the same AFF_* bitfield.
var nativeAffectFlagNames = []string{
	"blind", "invisible", "detect_align", "detect_invis", "detect_magic",
	"sense_life", "waterwalk", "sanctuary", "group", "curse", "infravision",
	"poison", "protect_evil", "protect_good", "sleep", "no_track", "",
	"holy_shield", "sneak", "hide", "silence", "charm",
}

// nativeItemExtraFlagNames match extraBitNames bit for bit.
var nativeItemExtraFlagNames = []string{
	"glow", "hum", "no_rent", "no_donate", "no_invis", "invisible", "magic",
	"no_drop", "bless", "anti_good", "anti_evil", "anti_neutral", "anti_mage",
	"anti_cleric", "anti_thief", "anti_warrior", "no_sell", "no_locate",
}

// nativeWearPositionNames name the 18 WearPosition slots (object.go), not
// the coarser 15-bit wear *flags* a prototype's `wear:` field carries —
// two different vocabularies over similar-sounding ground: a flag says an
// object *may* go on a finger, a position says *which* finger it is
// actually wearing it on. This is what a reset's `slot:` field names (§4.4:
// "wear position 16 in a reset becomes wield" — WearWield is index 16).
var nativeWearPositionNames = []string{
	"light", "finger_right", "finger_left", "neck1", "neck2", "body", "head",
	"legs", "feet", "hands", "arms", "shield", "about", "waist",
	"wrist_right", "wrist_left", "wield", "hold",
}

// nativeWearFlagNames match wearBitNames bit for bit.
var nativeWearFlagNames = []string{
	"take", "finger", "neck", "body", "head", "legs", "feet", "hands", "arms",
	"shield", "about", "waist", "wrist", "wield", "hold",
}

// nativeApplyTypeNames match applyTypeNames value for value.
var nativeApplyTypeNames = []string{
	"none", "str", "dex", "int", "wis", "con", "cha", "class", "level", "age",
	"char_weight", "char_height", "max_mana", "max_hit", "max_move", "gold",
	"exp", "armor", "hitroll", "damroll", "saving_para", "saving_rod",
	"saving_petri", "saving_breath", "saving_spell",
}

// nativeItemTypeNames match game.ItemTypeNames value for value. Index 17
// ("LIQ CONTAINER" in the display table) is spelled "drink_container" here,
// following §4.3's worked example literally rather than §4.1's passing
// mention of "liq_container" — the two disagree in the proposal itself, and
// §4.3 is the concrete schema.
var nativeItemTypeNames = []string{
	"undefined", "light", "scroll", "wand", "staff", "weapon", "fire_weapon",
	"missile", "treasure", "armor", "potion", "worn", "other", "trash",
	"trap", "container", "note", "drink_container", "key", "food", "money",
	"pen", "boat", "fountain",
}

// nativeAttackTypeNames match the AttackHit..AttackStab constants
// (violence.go) value for value — a weapon's fourth value, per §4.3's
// `damage_type:`.
var nativeAttackTypeNames = []string{
	"hit", "sting", "whip", "slash", "bite", "bludgeon", "crush", "pound",
	"claw", "maul", "thrash", "pierce", "blast", "punch", "stab",
}

// nativeLiquidNames name the LIQ_* constants (structs.h:427) a drink
// container's third value holds, in drink.go's drinkEffects/drinkKeywords
// order — 16 entries, LIQ_WATER (0) through LIQ_CLEARWATER (15).
var nativeLiquidNames = []string{
	"water", "beer", "wine", "ale", "dark_ale", "whisky", "lemonade",
	"firebreather", "local_specialty", "slime_mold_juice", "milk", "tea",
	"coffee", "blood", "salt_water", "clear_water",
}

// nativeShopFlagNames match shopstate.go's ShopWillFight/ShopUsesBank.
var nativeShopFlagNames = []string{"will_fight", "uses_bank"}

// nativeShopTradeNames name shop.h's TRADE_NO* bits (shop.go's TradeWith),
// which §4.5 deliberately keeps as a "who is refused" list rather than
// inverting it to "who is served" — see the `refuses:` field.
var nativeShopTradeNames = []string{
	"good", "evil", "neutral", "magic_user", "cleric", "thief", "warrior",
}

// Accessors, for internal/persist/world/native — these tables are this
// package's data, but the format that spells them out lives one level up.
func NativeRoomFlagNames() []string      { return nativeRoomFlagNames }
func NativeExitDoorNames() []string      { return nativeExitDoorNames }
func NativeDoorStateNames() []string     { return nativeDoorStateNames }
func NativeSectorNames() []string        { return nativeSectorNames }
func NativePositionNames() []string      { return nativePositionNames }
func NativeSexNames() []string           { return nativeSexNames }
func NativeMobActFlagNames() []string    { return nativeMobActFlagNames }
func NativeAffectFlagNames() []string    { return nativeAffectFlagNames }
func NativeItemExtraFlagNames() []string { return nativeItemExtraFlagNames }
func NativeWearFlagNames() []string      { return nativeWearFlagNames }
func NativeWearPositionNames() []string  { return nativeWearPositionNames }
func NativeApplyTypeNames() []string     { return nativeApplyTypeNames }
func NativeItemTypeNames() []string      { return nativeItemTypeNames }
func NativeAttackTypeNames() []string    { return nativeAttackTypeNames }
func NativeLiquidNames() []string        { return nativeLiquidNames }
func NativeShopFlagNames() []string      { return nativeShopFlagNames }
func NativeShopTradeNames() []string     { return nativeShopTradeNames }
