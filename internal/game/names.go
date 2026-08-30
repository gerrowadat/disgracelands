// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// One table per domain, holding both names a bit or a value has.
//
// Step 6 of docs/design/idiomatic-go.md. §3.5's complaint was that
// `bitnames.go` and `yamlnames.go` held two parallel []string tables per
// domain which had to agree slot for slot, plus the constants.c the first
// is derived from -- three copies of one fact. Two of those three are now
// one: a domain is a []nameEntry, and the two []string projections
// bitnames.go and yamlnames.go export are built from it once at init.
//
// # What this may not do, and does not
//
// §5 is explicit that a step here may not weaken a test that derives its
// expectation from the C, and warns that this step is "the one most likely
// to eat its own test". So, precisely:
//
//   - `bitnames_test.go`'s TestTheNameTablesMatchTheCSource still re-parses
//     constants.c and still compares it against roomBitNames,
//     affectBitNames and the rest. Those are still real package-level
//     variables holding real []string; they are computed from the paired
//     table rather than written out, which is the only difference. The C
//     remains the authority for every display string.
//   - `yamlnames_test.go`'s TestYamlBitTablesMatchDisplayTables does *not*
//     become vacuous, and that is worth being clear about because it is the
//     obvious thing to assume. The length half of it is now unrepresentable
//     -- one table cannot be a different length from itself -- but the half
//     that matters is not: an entry can still say `{"DARK", ""}`, naming a
//     bit for players and leaving it unnameable in a file. The test goes on
//     checking exactly that, against the pairs.
//
// # The two spellings are not interchangeable
//
// Display is constants.c's own string and is printed to players by
// `identify` and `stat`; it carries spaces and hyphens ("DET-ALIGN",
// "Water (No Swim)") and its case is the C's. Yaml is a lower_snake_case
// identifier a file holds. Renaming either to match the other would change
// either what a player sees or what a world file says -- see
// docs/design/data-format.md §4.1.

// nameEntry is one bit or one value, in both spellings.
type nameEntry struct {
	// Display is constants.c's string, or "*"/"UNUSED"/"" for a slot the C
	// itself does not name.
	Display string
	// Yaml is the file format's identifier, or "" for a slot no file may
	// name -- which sends the bit to flags_raw instead (§4.1).
	Yaml string
}

// displayNamesOf and yamlNamesOf project a paired table into the flat
// []string the SprintBit/NameBits family takes. Called once per table at
// package initialisation, never on a hot path.
func displayNamesOf(entries []nameEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Display
	}
	return out
}

func yamlNamesOf(entries []nameEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Yaml
	}
	return out
}

var roomFlagNames = []nameEntry{
	{"DARK", "dark"},
	{"DEATH", "death"},
	{"NO_MOB", "no_mob"},
	{"INDOORS", "indoors"},
	{"PEACEFUL", "peaceful"},
	{"SOUNDPROOF", "soundproof"},
	{"NO_TRACK", "no_track"},
	{"NO_MAGIC", "no_magic"},
	{"TUNNEL", "tunnel"},
	{"PRIVATE", "private"},
	{"GODROOM", "godroom"},
	{"HOUSE", "house"},
	{"HCRSH", "house_crash"},
	{"ATRIUM", "atrium"},
	{"OLC", "olc"},
	{"*", ""},
	{"GOOD_REGEN", "good_regen"},
	{"CAN_QUIT", "can_quit"},
	{"PKILL", "pkill"},
}

var mobActFlagNames = []nameEntry{
	{"SPEC", "spec"},
	{"SENTINEL", "sentinel"},
	{"SCAVENGER", "scavenger"},
	{"ISNPC", "isnpc"},
	{"AWARE", "aware"},
	{"AGGR", "aggressive"},
	{"STAY-ZONE", "stay_zone"},
	{"WIMPY", "wimpy"},
	{"AGGR_EVIL", "aggr_evil"},
	{"AGGR_GOOD", "aggr_good"},
	{"AGGR_NEUTRAL", "aggr_neutral"},
	{"MEMORY", "memory"},
	{"HELPER", "helper"},
	{"NO_CHARM", "no_charm"},
	{"NO_SUMMN", "no_summon"},
	{"NO_SLEEP", "no_sleep"},
	{"NO_BASH", "no_bash"},
	{"NO_BLIND", "no_blind"},
	{"DEAD", "dead"},
}

var affectFlagNames = []nameEntry{
	{"BLIND", "blind"},
	{"INVIS", "invisible"},
	{"DET-ALIGN", "detect_align"},
	{"DET-INVIS", "detect_invis"},
	{"DET-MAGIC", "detect_magic"},
	{"SENSE-LIFE", "sense_life"},
	{"WATWALK", "waterwalk"},
	{"SANCT", "sanctuary"},
	{"GROUP", "group"},
	{"CURSE", "curse"},
	{"INFRA", "infravision"},
	{"POISON", "poison"},
	{"PROT-EVIL", "protect_evil"},
	{"PROT-GOOD", "protect_good"},
	{"SLEEP", "sleep"},
	{"NO_TRACK", "no_track"},
	{"UNUSED", ""},
	{"HOLY-SHIELD", "holy_shield"},
	{"SNEAK", "sneak"},
	{"HIDE", "hide"},
	{"SILENCE", "silence"},
	{"CHARM", "charm"},
}

var itemExtraFlagNames = []nameEntry{
	{"GLOW", "glow"},
	{"HUM", "hum"},
	{"NO_RENT", "no_rent"},
	{"NO_DONATE", "no_donate"},
	{"NO_INVIS", "no_invis"},
	{"INVISIBLE", "invisible"},
	{"MAGIC", "magic"},
	{"NO_DROP", "no_drop"},
	{"BLESS", "bless"},
	{"ANTI_GOOD", "anti_good"},
	{"ANTI_EVIL", "anti_evil"},
	{"ANTI_NEUTRAL", "anti_neutral"},
	{"ANTI_MAGE", "anti_mage"},
	{"ANTI_CLERIC", "anti_cleric"},
	{"ANTI_THIEF", "anti_thief"},
	{"ANTI_WARRIOR", "anti_warrior"},
	{"NO_SELL", "no_sell"},
	{"NO_LOCATE", "no_locate"},
}

var wearFlagNames = []nameEntry{
	{"TAKE", "take"},
	{"FINGER", "finger"},
	{"NECK", "neck"},
	{"BODY", "body"},
	{"HEAD", "head"},
	{"LEGS", "legs"},
	{"FEET", "feet"},
	{"HANDS", "hands"},
	{"ARMS", "arms"},
	{"SHIELD", "shield"},
	{"ABOUT", "about"},
	{"WAIST", "waist"},
	{"WRIST", "wrist"},
	{"WIELD", "wield"},
	{"HOLD", "hold"},
}

var playerFlagNames = []nameEntry{
	{"KILLER", "killer"},
	{"THIEF", "thief"},
	{"FROZEN", "frozen"},
	{"DONTSET", "dont_set"},
	{"WRITING", "writing"},
	{"MAILING", "mailing"},
	{"CSH", "crash_save"},
	{"SITEOK", "siteok"},
	{"NOSHOUT", "no_shout"},
	{"NOTITLE", "no_title"},
	{"DELETED", "deleted"},
	{"LOADRM", "load_room"},
	{"NO_WIZL", "no_wizlist"},
	{"NO_DEL", "no_delete"},
	{"INVST", "invis_start"},
	{"CRYO", "cryo"},
	{"DEAD", "not_dead_yet"},
}

var preferenceNames = []nameEntry{
	{"BRIEF", "brief"},
	{"COMPACT", "compact"},
	{"DEAF", "deaf"},
	{"NO_TELL", "no_tell"},
	{"D_HP", "display_hp"},
	{"D_MANA", "display_mana"},
	{"D_MOVE", "display_move"},
	{"AUTOEX", "autoexit"},
	{"NO_HASS", "no_hassle"},
	{"QUEST", "quest"},
	{"SUMN", "summonable"},
	{"NO_REP", "no_repeat"},
	{"LIGHT", "holylight"},
	{"C1", "color_1"},
	{"C2", "color_2"},
	{"NO_WIZ", "no_wiz"},
	{"L1", "log_1"},
	{"L2", "log_2"},
	{"NO_AUC", "no_auction"},
	{"NO_GOS", "no_gossip"},
	{"NO_GTZ", "no_grats"},
	{"RMFLG", "room_flags"},
}

var sectorTypeNames = []nameEntry{
	{"Inside", "inside"},
	{"City", "city"},
	{"Field", "field"},
	{"Forest", "forest"},
	{"Hills", "hills"},
	{"Mountains", "mountains"},
	{"Water (Swim)", "water_swim"},
	{"Water (No Swim)", "water_noswim"},
	{"In Flight", "flying"},
	{"Underwater", "underwater"},
}

var positionTypeNames = []nameEntry{
	{"Dead", "dead"},
	{"Mortally wounded", "mortally_wounded"},
	{"Incapacitated", "incapacitated"},
	{"Stunned", "stunned"},
	{"Sleeping", "sleeping"},
	{"Resting", "resting"},
	{"Sitting", "sitting"},
	{"Fighting", "fighting"},
	{"Standing", "standing"},
}

var sexNames = []nameEntry{
	{"neutral", "neutral"},
	{"male", "male"},
	{"female", "female"},
}

var applyNames = []nameEntry{
	{"NONE", "none"},
	{"STR", "str"},
	{"DEX", "dex"},
	{"INT", "int"},
	{"WIS", "wis"},
	{"CON", "con"},
	{"CHA", "cha"},
	{"CLASS", "class"},
	{"LEVEL", "level"},
	{"AGE", "age"},
	{"CHAR_WEIGHT", "char_weight"},
	{"CHAR_HEIGHT", "char_height"},
	{"MAXMANA", "max_mana"},
	{"MAXHIT", "max_hit"},
	{"MAXMOVE", "max_move"},
	{"GOLD", "gold"},
	{"EXP", "exp"},
	{"ARMOR", "armor"},
	{"HITROLL", "hitroll"},
	{"DAMROLL", "damroll"},
	{"SAVING_PARA", "saving_para"},
	{"SAVING_ROD", "saving_rod"},
	{"SAVING_PETRI", "saving_petri"},
	{"SAVING_BREATH", "saving_breath"},
	{"SAVING_SPELL", "saving_spell"},
}

var itemTypeNames = []nameEntry{
	{"UNDEFINED", "undefined"},
	{"LIGHT", "light"},
	{"SCROLL", "scroll"},
	{"WAND", "wand"},
	{"STAFF", "staff"},
	{"WEAPON", "weapon"},
	{"FIRE WEAPON", "fire_weapon"},
	{"MISSILE", "missile"},
	{"TREASURE", "treasure"},
	{"ARMOR", "armor"},
	{"POTION", "potion"},
	{"WORN", "worn"},
	{"OTHER", "other"},
	{"TRASH", "trash"},
	{"TRAP", "trap"},
	{"CONTAINER", "container"},
	{"NOTE", "note"},
	{"LIQ CONTAINER", "drink_container"},
	{"KEY", "key"},
	{"FOOD", "food"},
	{"MONEY", "money"},
	{"PEN", "pen"},
	{"BOAT", "boat"},
	{"FOUNTAIN", "fountain"},
}

var wearPositionNames = []nameEntry{
	{"<used as light>      ", "light"},
	{"<worn on finger>     ", "finger_right"},
	{"<worn on finger>     ", "finger_left"},
	{"<worn around neck>   ", "neck1"},
	{"<worn around neck>   ", "neck2"},
	{"<worn on body>       ", "body"},
	{"<worn on head>       ", "head"},
	{"<worn on legs>       ", "legs"},
	{"<worn on feet>       ", "feet"},
	{"<worn on hands>      ", "hands"},
	{"<worn on arms>       ", "arms"},
	{"<worn as shield>     ", "shield"},
	{"<worn about body>    ", "about"},
	{"<worn around waist>  ", "waist"},
	{"<worn around wrist>  ", "wrist_right"},
	{"<worn around wrist>  ", "wrist_left"},
	{"<wielded>            ", "wield"},
	{"<held>               ", "hold"},
}

var classNames = []nameEntry{
	{"Magic User", "magic_user"},
	{"Cleric", "cleric"},
	{"Thief", "thief"},
	{"Warrior", "warrior"},
	{"Paladin", "paladin"},
}

var liquidNames = []nameEntry{
	{"water", "water"},
	{"beer", "beer"},
	{"wine", "wine"},
	{"ale", "ale"},
	{"dark ale", "dark_ale"},
	{"whisky", "whisky"},
	{"lemonade", "lemonade"},
	{"firebreather", "firebreather"},
	{"local speciality", "local_specialty"},
	{"slime mold juice", "slime_mold_juice"},
	{"milk", "milk"},
	{"tea", "tea"},
	{"coffee", "coffee"},
	{"blood", "blood"},
	{"salt water", "salt_water"},
	{"clear water", "clear_water"},
}
