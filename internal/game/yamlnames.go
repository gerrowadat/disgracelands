// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// The identifiers the yaml data format (docs/design/data-format.md §4)
// writes for every flag set, sector, position and item type the world files
// carry — lower_snake_case, and *not* the display strings in bitnames.go and
// object.go. §4.1 is explicit those cannot be reused: they are positional,
// carry spaces and hyphens ("DET-ALIGN", "LIQ CONTAINER"), and are printed to
// players by `identify`, so renaming one to suit a file format would change
// what a spell inspection shows.
//
// Every table here is the same length, in the same bit or value order, as
// its bitnames.go/object.go counterpart — yamlnames_test.go asserts that
// directly, re-deriving nothing from the C, because the two Go tables are
// the ones that must never drift apart. A "" entry marks a slot with no
// name in *either* table (constants.c's own gaps, e.g. the room bits' "*"
// placeholder and the affect bits' UNUSED slot): NameBits/ParseBitNames
// treat that as unnamed rather than inventing an identifier for it, so it
// round-trips through flags_raw instead (§4.1).

// yamlRoomFlagNames match roomBitNames bit for bit.
var yamlRoomFlagNames = []string{
	"dark", "death", "no_mob", "indoors", "peaceful", "soundproof", "no_track",
	"no_magic", "tunnel", "private", "godroom", "house", "house_crash",
	"atrium", "olc", "", "good_regen", "can_quit", "pkill",
}

// yamlExitDoorNames name the raw 0/1/2 door byte the file always held —
// §4.2's "door: pickproof # absent = no door; regular | pickproof". Index 0
// (no door) has no name because the key is simply absent.
var yamlExitDoorNames = []string{"", "regular", "pickproof"}

// yamlDoorStateNames are the 'D' reset command's third argument
// (game.DoorOpen/DoorClosed/DoorLocked), used by §4.4's `state:` field.
var yamlDoorStateNames = []string{"open", "closed", "locked"}

// yamlSectorNames match sectorNames value for value.
var yamlSectorNames = []string{
	"inside", "city", "field", "forest", "hills", "mountains", "water_swim",
	"water_noswim", "flying", "underwater",
}

// yamlPositionNames match positionNames value for value.
var yamlPositionNames = []string{
	"dead", "mortally_wounded", "incapacitated", "stunned", "sleeping",
	"resting", "sitting", "fighting", "standing",
}

// yamlSexNames match genderNames value for value.
var yamlSexNames = []string{"neutral", "male", "female"}

// yamlMobActFlagNames match actionBitNames bit for bit. "isnpc" is never
// written by the yaml writer — it is force-set on every mobile regardless
// of the file, the same rule MobIsNPC documents — but it is still named so a
// file that spells it out explicitly loads without complaint rather than
// tripping the "unknown name" error.
var yamlMobActFlagNames = []string{
	"spec", "sentinel", "scavenger", "isnpc", "aware", "aggressive",
	"stay_zone", "wimpy", "aggr_evil", "aggr_good", "aggr_neutral", "memory",
	"helper", "no_charm", "no_summon", "no_sleep", "no_bash", "no_blind",
	"dead",
}

// yamlAffectFlagNames match affectBitNames bit for bit. Used for both a
// mobile's `affected:` list and a player's `flags.affected:` list — both are
// the same AFF_* bitfield.
var yamlAffectFlagNames = []string{
	"blind", "invisible", "detect_align", "detect_invis", "detect_magic",
	"sense_life", "waterwalk", "sanctuary", "group", "curse", "infravision",
	"poison", "protect_evil", "protect_good", "sleep", "no_track", "",
	"holy_shield", "sneak", "hide", "silence", "charm",
}

// yamlItemExtraFlagNames match extraBitNames bit for bit.
var yamlItemExtraFlagNames = []string{
	"glow", "hum", "no_rent", "no_donate", "no_invis", "invisible", "magic",
	"no_drop", "bless", "anti_good", "anti_evil", "anti_neutral", "anti_mage",
	"anti_cleric", "anti_thief", "anti_warrior", "no_sell", "no_locate",
}

// yamlWearPositionNames name the 18 WearPosition slots (object.go), not
// the coarser 15-bit wear *flags* a prototype's `wear:` field carries —
// two different vocabularies over similar-sounding ground: a flag says an
// object *may* go on a finger, a position says *which* finger it is
// actually wearing it on. This is what a reset's `slot:` field names (§4.4:
// "wear position 16 in a reset becomes wield" — WearWield is index 16).
var yamlWearPositionNames = []string{
	"light", "finger_right", "finger_left", "neck1", "neck2", "body", "head",
	"legs", "feet", "hands", "arms", "shield", "about", "waist",
	"wrist_right", "wrist_left", "wield", "hold",
}

// yamlWearFlagNames match wearBitNames bit for bit.
var yamlWearFlagNames = []string{
	"take", "finger", "neck", "body", "head", "legs", "feet", "hands", "arms",
	"shield", "about", "waist", "wrist", "wield", "hold",
}

// yamlApplyTypeNames match applyTypeNames value for value.
var yamlApplyTypeNames = []string{
	"none", "str", "dex", "int", "wis", "con", "cha", "class", "level", "age",
	"char_weight", "char_height", "max_mana", "max_hit", "max_move", "gold",
	"exp", "armor", "hitroll", "damroll", "saving_para", "saving_rod",
	"saving_petri", "saving_breath", "saving_spell",
}

// yamlItemTypeNames match game.ItemTypeNames value for value. Index 17
// ("LIQ CONTAINER" in the display table) is spelled "drink_container" here,
// following §4.3's worked example literally rather than §4.1's passing
// mention of "liq_container" — the two disagree in the proposal itself, and
// §4.3 is the concrete schema.
var yamlItemTypeNames = []string{
	"undefined", "light", "scroll", "wand", "staff", "weapon", "fire_weapon",
	"missile", "treasure", "armor", "potion", "worn", "other", "trash",
	"trap", "container", "note", "drink_container", "key", "food", "money",
	"pen", "boat", "fountain",
}

// yamlAttackTypeNames match the AttackHit..AttackStab constants
// (violence.go) value for value — a weapon's fourth value, per §4.3's
// `damage_type:`.
var yamlAttackTypeNames = []string{
	"hit", "sting", "whip", "slash", "bite", "bludgeon", "crush", "pound",
	"claw", "maul", "thrash", "pierce", "blast", "punch", "stab",
}

// yamlLiquidNames name the LIQ_* constants (structs.h:427) a drink
// container's third value holds, in drink.go's drinkEffects/drinkKeywords
// order — 16 entries, LIQ_WATER (0) through LIQ_CLEARWATER (15).
var yamlLiquidNames = []string{
	"water", "beer", "wine", "ale", "dark_ale", "whisky", "lemonade",
	"firebreather", "local_specialty", "slime_mold_juice", "milk", "tea",
	"coffee", "blood", "salt_water", "clear_water",
}

// yamlPlayerFlagNames match playerBitNames bit for bit — the PLR_* flags
// a player record's `flags.act:` list uses. PlayerBanned (bit 17) has no
// name here because it has none in playerBitNames either: it is a local
// addition that was never given a slot in the C's own player_bits[]
// (bitnames.go's comment on it), so SprintBit already renders it
// "UNDEFINED" today — the yaml format leaves it exactly as unnamed as the
// C does, rather than inventing the identifier the original source never
// had. It still round-trips, via flags_raw.
var yamlPlayerFlagNames = []string{
	"killer", "thief", "frozen", "dont_set", "writing", "mailing",
	"crash_save", "siteok", "no_shout", "no_title", "deleted", "load_room",
	"no_wizlist", "no_delete", "invis_start", "cryo", "not_dead_yet",
}

// yamlPreferenceNames match preferenceBitNames bit for bit — a player's
// `flags.prefs:` list. PrefClearScreen (bit 22, OasisOLC's own addition) is
// unnamed for the same reason PlayerBanned is above.
var yamlPreferenceNames = []string{
	"brief", "compact", "deaf", "no_tell", "display_hp", "display_mana",
	"display_move", "autoexit", "no_hassle", "quest", "summonable",
	"no_repeat", "holylight", "color_1", "color_2", "no_wiz", "log_1",
	"log_2", "no_auction", "no_gossip", "no_grats", "room_flags",
}

// yamlClassNames match ClassNames value for value (ClassMagicUser through
// ClassPaladin) — a player's `identity.class:` field. The same table also
// names RemortVector's bits: classRemortMasks (create.go) assigns class N
// bit 1<<N, so a class name at index N is simultaneously its remort-vector
// bit's name, and `identity.remort:` reads through NameBits/ParseBitNames
// against this table rather than a second one.
var yamlClassNames = []string{
	"magic_user", "cleric", "thief", "warrior", "paladin",
}

// yamlShopFlagNames match shopstate.go's ShopWillFight/ShopUsesBank.
var yamlShopFlagNames = []string{"will_fight", "uses_bank"}

// yamlShopTradeNames name shop.h's TRADE_NO* bits (shop.go's TradeWith),
// which §4.5 deliberately keeps as a "who is refused" list rather than
// inverting it to "who is served" — see the `refuses:` field.
var yamlShopTradeNames = []string{
	"good", "evil", "neutral", "magic_user", "cleric", "thief", "warrior",
}

// Accessors, for internal/persist/world/yaml — these tables are this
// package's data, but the format that spells them out lives one level up.
func YamlRoomFlagNames() []string      { return yamlRoomFlagNames }
func YamlExitDoorNames() []string      { return yamlExitDoorNames }
func YamlDoorStateNames() []string     { return yamlDoorStateNames }
func YamlSectorNames() []string        { return yamlSectorNames }
func YamlPositionNames() []string      { return yamlPositionNames }
func YamlSexNames() []string           { return yamlSexNames }
func YamlMobActFlagNames() []string    { return yamlMobActFlagNames }
func YamlAffectFlagNames() []string    { return yamlAffectFlagNames }
func YamlItemExtraFlagNames() []string { return yamlItemExtraFlagNames }
func YamlWearFlagNames() []string      { return yamlWearFlagNames }
func YamlWearPositionNames() []string  { return yamlWearPositionNames }
func YamlApplyTypeNames() []string     { return yamlApplyTypeNames }
func YamlItemTypeNames() []string      { return yamlItemTypeNames }
func YamlAttackTypeNames() []string    { return yamlAttackTypeNames }
func YamlLiquidNames() []string        { return yamlLiquidNames }
func YamlShopFlagNames() []string      { return yamlShopFlagNames }
func YamlShopTradeNames() []string     { return yamlShopTradeNames }
func YamlPlayerFlagNames() []string    { return yamlPlayerFlagNames }
func YamlPreferenceNames() []string    { return yamlPreferenceNames }
func YamlClassNames() []string         { return yamlClassNames }
