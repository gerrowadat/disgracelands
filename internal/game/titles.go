// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

// Default character titles, ported from class.c's title_male (line 2324) and
// title_female (line 2511).
//
// These were transcribed mechanically from the C rather than by hand, and
// titles_test.go re-parses that same C source and compares, so a difference
// is a test failure rather than something to be noticed years later by a
// player who remembers what their title used to say.
//
// The tables are uneven and that is faithful: the magic-user and paladin
// lists run to level 30, while the cleric, thief and warrior lists stop at 20
// (21 for a female thief) and every level above that falls through to the
// class default. Disgracelands' own paladin titles are here too, in the same
// voice as the rest of the local additions.

// Level constants from structs.h:485. LVL_IMMORT is the lowest immortal
// level; LVL_IMPL the highest.
const (
	LevelImmortal    int32 = 31
	LevelGod         int32 = 32
	LevelGreaterGod  int32 = 33
	LevelImplementor int32 = 34
)

// classTitles is one class's list, with the fallback used for levels it does
// not name.
type classTitles struct {
	byLevel      map[int32]string
	defaultTitle string
}

// Title returns the default title for a class, level and sex, porting
// set_title(ch, NULL) (limits.c:223).
//
// A player who has set their own title keeps it; this is only what they are
// given at creation and on each level gained.
func Title(class, level, sex int32) string {
	// The C checks these before it looks at the class at all, so an
	// implementor of any class is "the Implementor".
	if level <= 0 || level > LevelImplementor {
		if sex == SexFemale {
			return "the Woman"
		}
		return "the Man"
	}
	if level == LevelImplementor {
		if sex == SexFemale {
			return "the Implementress"
		}
		return "the Implementor"
	}

	table := titlesMale
	if sex == SexFemale {
		table = titlesFemale
	}
	titles, ok := table[class]
	if !ok {
		// The C's fall-through for a class with no titles defined.
		return "the Classless"
	}
	if t, ok := titles.byLevel[level]; ok {
		return t
	}
	return titles.defaultTitle
}

var titlesMale = map[int32]classTitles{
	ClassMagicUser: {
		defaultTitle: "the Mage",
		byLevel: map[int32]string{
			1:  "the Apprentice of Magic",
			2:  "the Spell Student",
			3:  "the Scholar of Magic",
			4:  "the Delver in Spells",
			5:  "the Medium of Magic",
			6:  "the Scribe of Magic",
			7:  "the Seer",
			8:  "the Sage",
			9:  "the Illusionist",
			10: "the Abjurer",
			11: "the Invoker",
			12: "the Enchanter",
			13: "the Conjurer",
			14: "the Magician",
			15: "the Creator",
			16: "the Savant",
			17: "the Magus",
			18: "the Wizard",
			19: "the Warlock",
			20: "the Sorcerer",
			21: "the Necromancer",
			22: "the Thaumaturge",
			23: "the Student of the Occult",
			24: "the Disciple of the Uncanny",
			25: "the Minor Elemental",
			26: "the Greater Elemental",
			27: "the Crafter of Magics",
			28: "the Shaman",
			29: "the Keeper of Talismans",
			30: "the Archmage",
			31: "the Immortal Warlock",
			32: "the Avatar of Magic",
			33: "the God of Magic",
		},
	},
	ClassCleric: {
		defaultTitle: "the Cleric",
		byLevel: map[int32]string{
			1:  "the Believer",
			2:  "the Attendant",
			3:  "the Acolyte",
			4:  "the Novice",
			5:  "the Missionary",
			6:  "the Adept",
			7:  "the Deacon",
			8:  "the Vicar",
			9:  "the Priest",
			10: "the Minister",
			11: "the Canon",
			12: "the Levite",
			13: "the Curate",
			14: "the Monk",
			15: "the Healer",
			16: "the Chaplain",
			17: "the Expositor",
			18: "the Bishop",
			19: "the Arch Bishop",
			20: "the Patriarch",
			31: "the Immortal Cardinal",
			32: "the Inquisitor",
			33: "the God of good and evil",
		},
	},
	ClassThief: {
		defaultTitle: "the Thief",
		byLevel: map[int32]string{
			1:  "the Pilferer",
			2:  "the Footpad",
			3:  "the Filcher",
			4:  "the Pick-Pocket",
			5:  "the Sneak",
			6:  "the Pincher",
			7:  "the Cut-Purse",
			8:  "the Snatcher",
			9:  "the Sharper",
			10: "the Rogue",
			11: "the Robber",
			12: "the Magsman",
			13: "the Highwayman",
			14: "the Burglar",
			15: "the Thief",
			16: "the Knifer",
			17: "the Quick-Blade",
			18: "the Killer",
			19: "the Brigand",
			20: "the Cut-Throat",
			31: "the Immortal Assasin",
			32: "the Demi God of thieves",
			33: "the God of thieves and tradesmen",
		},
	},
	ClassWarrior: {
		defaultTitle: "the Warrior",
		byLevel: map[int32]string{
			1:  "the Swordpupil",
			2:  "the Recruit",
			3:  "the Sentry",
			4:  "the Fighter",
			5:  "the Soldier",
			6:  "the Warrior",
			7:  "the Veteran",
			8:  "the Swordsman",
			9:  "the Fencer",
			10: "the Combatant",
			11: "the Hero",
			12: "the Myrmidon",
			13: "the Swashbuckler",
			14: "the Mercenary",
			15: "the Swordmaster",
			16: "the Lieutenant",
			17: "the Champion",
			18: "the Dragoon",
			19: "the Cavalier",
			20: "the Knight",
			31: "the Immortal Warlord",
			32: "the Extirpator",
			33: "the God of war",
		},
	},
	ClassPaladin: {
		defaultTitle: "the Paladin",
		byLevel: map[int32]string{
			1:  "the Firm Believer",
			2:  "the Initiate",
			3:  "the Tryer",
			4:  "the Paladin",
			5:  "the Soldier of Good",
			6:  "the Honest Fighter",
			7:  "the Bearer of Wrath",
			8:  "the Bringer of Goodly Smiting",
			9:  "the Guardian of the Weak",
			10: "the Enemy of Evil",
			11: "the Humble Hero",
			12: "the Dispenser of Justice",
			13: "the Inquisitor",
			14: "the Expositor",
			15: "the Friend of the Gentle",
			16: "the Greater Paladin",
			17: "the Champion of Good Causes",
			18: "Evilbane",
			19: "the Cavalier",
			20: "the Shining Knight",
			21: "the Righteous Fighter",
			22: "the Smiter of All Evil",
			23: "the Divine Heart",
			24: "the Mighty Paladin",
			25: "the Dispenser-with of Naughty Things",
			26: "the Beating-downer of Evil Thoughts",
			27: "the Guardian of Light",
			28: "the Mighty Redeemer of Sinners",
			29: "the Stern Telling-offer",
			30: "the Firm Rodgerer of Satan's Eye-socket",
			31: "the Immortal Paladin",
			32: "the Grand Inquisitor",
			33: "the God of Justice",
		},
	},
}

var titlesFemale = map[int32]classTitles{
	ClassMagicUser: {
		defaultTitle: "the Witch",
		byLevel: map[int32]string{
			1:  "the Apprentice of Magic",
			2:  "the Spell Student",
			3:  "the Scholar of Magic",
			4:  "the Delveress in Spells",
			5:  "the Medium of Magic",
			6:  "the Scribess of Magic",
			7:  "the Seeress",
			8:  "the Sage",
			9:  "the Illusionist",
			10: "the Abjuress",
			11: "the Invoker",
			12: "the Enchantress",
			13: "the Conjuress",
			14: "the Witch",
			15: "the Creator",
			16: "the Savant",
			17: "the Craftess",
			18: "the Wizard",
			19: "the War Witch",
			20: "the Sorceress",
			21: "the Necromancress",
			22: "the Thaumaturgess",
			23: "the Student of the Occult",
			24: "the Disciple of the Uncanny",
			25: "the Minor Elementress",
			26: "the Greater Elementress",
			27: "the Crafter of Magics",
			28: "Shaman",
			29: "the Keeper of Talismans",
			30: "Archwitch",
			31: "the Immortal Enchantress",
			32: "the Empress of Magic",
			33: "the Goddess of Magic",
		},
	},
	ClassCleric: {
		defaultTitle: "the Cleric",
		byLevel: map[int32]string{
			1:  "the Believer",
			2:  "the Attendant",
			3:  "the Acolyte",
			4:  "the Novice",
			5:  "the Missionary",
			6:  "the Adept",
			7:  "the Deaconess",
			8:  "the Vicaress",
			9:  "the Priestess",
			10: "the Lady Minister",
			11: "the Canon",
			12: "the Levitess",
			13: "the Curess",
			14: "the Nunne",
			15: "the Healess",
			16: "the Chaplain",
			17: "the Expositress",
			18: "the Bishop",
			19: "the Arch Lady of the Church",
			20: "the Matriarch",
			31: "the Immortal Priestess",
			32: "the Inquisitress",
			33: "the Goddess of good and evil",
		},
	},
	ClassThief: {
		defaultTitle: "the Thief",
		byLevel: map[int32]string{
			1:  "the Pilferess",
			2:  "the Footpad",
			3:  "the Filcheress",
			4:  "the Pick-Pocket",
			5:  "the Sneak",
			6:  "the Pincheress",
			7:  "the Cut-Purse",
			8:  "the Snatcheress",
			9:  "the Sharpress",
			10: "the Rogue",
			11: "the Robber",
			12: "the Magswoman",
			13: "the Highwaywoman",
			14: "the Burglaress",
			15: "the Thief",
			16: "the Knifer",
			17: "the Quick-Blade",
			18: "the Murderess",
			19: "the Brigand",
			20: "the Cut-Throat",
			31: "the Immortal Assasin",
			32: "the Demi Goddess of thieves",
			33: "the Goddess of thieves and tradesmen",
			34: "the Implementress",
		},
	},
	ClassWarrior: {
		defaultTitle: "the Warrior",
		byLevel: map[int32]string{
			1:  "the Swordpupil",
			2:  "the Recruit",
			3:  "the Sentress",
			4:  "the Fighter",
			5:  "the Soldier",
			6:  "the Warrior",
			7:  "the Veteran",
			8:  "the Swordswoman",
			9:  "the Fenceress",
			10: "the Combatess",
			11: "the Heroine",
			12: "the Myrmidon",
			13: "the Swashbuckleress",
			14: "the Mercenaress",
			15: "the Swordmistress",
			16: "the Lieutenant",
			17: "the Lady Champion",
			18: "the Lady Dragoon",
			19: "the Cavalier",
			20: "the Lady Knight",
			31: "the Immortal Lady of War",
			32: "the Queen of Destruction",
			33: "the Goddess of war",
		},
	},
	ClassPaladin: {
		defaultTitle: "the Paladin",
		byLevel: map[int32]string{
			1:  "the Firm Believer",
			2:  "the Initiate",
			3:  "the Tryer",
			4:  "the Paladin",
			5:  "the Soldier of Good",
			6:  "the Honest Fighter",
			7:  "the Bearer of Wrath",
			8:  "the Bringer of Goodly Smiting",
			9:  "the Guardian of the Weak",
			10: "the Enemy of Evil",
			11: "the Humble Heroine",
			12: "the Dispenser of Justice",
			13: "the Inquisitress",
			14: "the Expositress",
			15: "the Friend of the Gentle",
			16: "the Greater Paladin",
			17: "the Champion of Good Causes",
			18: "Evilbane",
			19: "the Cavalier",
			20: "the Shining Knight",
			21: "the Righteous Fighter",
			22: "the Smiter of All Evil",
			23: "the Divine Heart",
			24: "the Mighty Paladin",
			25: "the Dispenser-with of Naughty Things",
			26: "the Beating-downer of Evil Thoughts",
			27: "the Guardian of Light",
			28: "the Mighty Redeemer of Sinners",
			29: "the Stern Telling-offer",
			30: "the Snapper-of-Satan's-Neck-With-Her-Thighs",
			31: "the Immortal Paladin",
			32: "the Grand Inquisitress",
			33: "the Goddess of Justice",
		},
	},
}
