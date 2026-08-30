// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"strconv"
	"strings"
)

// The spell table, ported from mag_assign_spells (spell_parser.c) and
// init_spell_levels (class.c).
//
// The C builds this at boot by calling spello() sixty-odd times and then
// spell_level() another seventy-eight, which is a table written as
// executable statements. Here it is a table. spell_test.go re-parses both C
// functions and compares every field of every entry, so the transcription is
// checked rather than trusted.

// TargetFlag is one of the TAR_* targeting flags, from spells.h:167, and
// TargetFlags is a spell's set of them. Two of them are checks rather than
// targets — TargetSelfOnly and TargetNotSelf qualify one of the others.
//
// Bit indices, not masks: docs/design/idiomatic-go.md §4.1, and §4.1.1
// for the trap. spell_test.go re-parses spells.h and compares every
// entry's raw bits, which is what keeps these numbers the C's (§5).
type TargetFlag int

// TargetFlags is a set of TargetFlag.
type TargetFlags = Set[TargetFlag]

const (
	TargetIgnore    TargetFlag = 0
	TargetCharRoom  TargetFlag = 1
	TargetCharWorld TargetFlag = 2
	TargetFightSelf TargetFlag = 3
	TargetFightVict TargetFlag = 4
	TargetSelfOnly  TargetFlag = 5
	TargetNotSelf   TargetFlag = 6
	TargetObjInv    TargetFlag = 7
	TargetObjRoom   TargetFlag = 8
	TargetObjWorld  TargetFlag = 9
	TargetObjEquip  TargetFlag = 10
)

// Spell routines, from spells.h:21. A spell may have several — heal both
// restores points and removes blindness — and the C runs each in turn.
// RoutineFlag is one of the MAG_* routines, and RoutineFlags is a spell's
// set of them.
type RoutineFlag int

// RoutineFlags is a set of RoutineFlag.
type RoutineFlags = Set[RoutineFlag]

const (
	MagDamage    RoutineFlag = 0
	MagAffects   RoutineFlag = 1
	MagUnaffects RoutineFlag = 2
	MagPoints    RoutineFlag = 3
	MagAlterObjs RoutineFlag = 4
	MagGroups    RoutineFlag = 5
	MagMasses    RoutineFlag = 6
	MagAreas     RoutineFlag = 7
	MagSummons   RoutineFlag = 8
	MagCreations RoutineFlag = 9
	// MagManual means the spell has a function of its own rather than being
	// assembled from the routines above.
	MagManual RoutineFlag = 10
)

// SpellID identifies a spell or a skill. The two are one domain: the C
// declares a skill with skillo(), which is spello() with every number
// zero, so a skill is a row of spell_info[] with a name and nothing else.
// spellTable is keyed by this and holds both.
//
// The numbers are stored in every player record -- as the keys of
// PlayerRecord.Skills, and in a wand or staff's object values -- so they
// are the file format as much as they are an enumeration
// (docs/design/idiomatic-go.md §2.1). They are not contiguous: the
// spells run 0-58, the skills 131-140 with 136 unused, and the breath
// weapons 201-206.
//
// The zero value is SpellReservedDbc, which is stock CircleMUD's own
// placeholder for "not a spell" -- call_magic (spell_parser.c:229) rejects
// anything below 1 outright. So an unset SpellID is inert rather than
// meaning some real spell, which is the opposite of Class's zero value and
// the same as ItemType's.
type SpellID int

// Number is the spell's stored number, for the player and world formats
// and for the "#N" placeholder SpellNameOrNumber writes.
func (s SpellID) Number() int32 { return int32(s) } //nolint:gosec // spell numbers top out at 206; the format's width

// SpellInfo is one row of the C's spell_info[].
type SpellInfo struct {
	Name string

	// ManaMax is what the spell costs when first learned, ManaMin the floor
	// however high the caster gets, and ManaChange how much each level off
	// the minimum reduces it by.
	ManaMax    int32
	ManaMin    int32
	ManaChange int32

	// MinPosition is the lowest position from which it can be cast.
	MinPosition Position
	// Targets is what it may be aimed at.
	Targets TargetFlags
	// Violent marks a spell that starts a fight.
	Violent bool
	// Routines is what it actually does.
	Routines RoutineFlags
	// WearOff is what the victim is told when an affect expires.
	WearOff string

	// MinLevel is the level each class learns it at. A class absent from the
	// map never learns it: the C fills every slot with LVL_IMMORT first and
	// only lowers the ones init_spell_levels names.
	MinLevel map[Class]int32
}

// Spell returns a spell's row, and whether it exists.
func Spell(number SpellID) (SpellInfo, bool) {
	info, ok := spellTable[number]
	return info, ok
}

// SpellName returns a spell's name, or "!UNUSED!" as the C does.
func SpellName(number SpellID) string {
	if info, ok := spellTable[number]; ok {
		return info.Name
	}
	return "!UNUSED!"
}

// SpellNumberByName finds a spell by name, porting find_skill_num
// (spell_parser.c) — so `cast 'magic mis'` works, and so does
// `cast 'mag mis'`.
//
// **An empty name matches the first spell in the table**, which is armor,
// and that is the C's answer rather than a shrug. is_abbrev rejects an
// empty arg1, so rule 1 declines; the word loop then does not run at all,
// leaving `ok` TRUE and `first2` empty, so `ok && !*first2` holds on the
// very first entry.
//
// This was refused here until #365, on the reasoning that the C cannot
// reach it: do_cast gets the spell name from `strtok(argument, "'")`
// followed by `strtok(NULL, "'")` (spell_parser.c:604), and strtok skips
// leading delimiters, so `cast ”` never hands an empty name over. That is
// right about `cast ”` and wrong about the general claim. Only the quote
// is a delimiter — a space is not — so:
//
//	cast ''        -> "Spell names must be enclosed..."   (the C, and here)
//	cast '  '      -> find_skill_num("  ")  ->  armor     (the C)
//	cast ' '       -> find_skill_num(" ")   ->  armor     (the C)
//
// and find_skill_num cannot tell "  " from "" because any_one_arg
// tokenises the whitespace away before either rule looks at it. Checked by
// running do_cast's own strtok pair rather than by reading it, which is
// how the earlier reasoning went wrong.
//
// So the refusal moved rather than being deleted: this function is
// find_skill_num and answers as it does, and
// SpellNumberFromNameOrNumber — which is a *format* lookup rather than a
// player's typing — refuses an empty name itself, so that an empty
// `spell:` in a yaml file stays an error instead of silently becoming
// armor.
func SpellNumberByName(name string) (SpellID, bool) {
	name = strings.ToLower(strings.TrimSpace(name))

	// An exact match wins outright; otherwise the lowest-numbered match,
	// so the answer does not depend on map iteration order. The C has no
	// exact-match preference — it takes the first index either rule
	// matches — and the two agree because no spell name in the table is
	// reachable from another's whole name by either rule. That is checked
	// by a test rather than asserted here, because it is a property of the
	// name table and the table can change.
	best := SpellID(-1)
	for number, info := range spellTable {
		if info.Name == name {
			return number, true
		}
		if matchesSkillName(name, info.Name) && (best < 0 || number < best) {
			best = number
		}
	}
	if best < 0 {
		return 0, false
	}
	return best, true
}

// matchesSkillName is find_skill_num's two matching rules for one table
// entry (spell_parser.c). Both arguments are already lower-cased.
//
// The first rule is `is_abbrev(name, spell_info[index].name)`: the whole
// typed string against the whole spell name, so "magic mis" reaches
// "magic missile".
//
// The second is the one this port was missing for the whole of its life,
// and it is the one a caster actually types. It walks both strings a word
// at a time and requires each typed word to abbreviate the spell-name word
// in the *same position*, so "mag mis" reaches "magic missile", "b h"
// reaches "burning hands" and "det inv" reaches "detect invisibility".
// 1,145 of the 1,549 per-word abbreviations of the game's own 71 spell
// names were refused without it; see docs/investigations/partial-matching.md
// and #355.
func matchesSkillName(typed, spell string) bool {
	if isAbbrev(typed, spell) {
		return true
	}

	// The C's loop stops when *either* string runs out, and its verdict is
	// `ok && !*first2` — the typed string, not the spell name. So a query
	// with fewer words than the name matches ("cure" reaches "cure light"
	// through this branch as well as through is_abbrev) and one with more
	// words does not, however well the words it does have line up.
	typedWords, spellWords := strings.Fields(typed), strings.Fields(spell)
	if len(typedWords) > len(spellWords) {
		return false
	}
	for i, word := range typedWords {
		if !isAbbrev(word, spellWords[i]) {
			return false
		}
	}
	return true
}

// isAbbrev is is_abbrev (interpreter.c:1057): arg1 is a non-empty prefix
// of arg2, case-insensitively. Both arguments here are already lower-cased,
// so this is the prefix test and the emptiness rule and nothing else — the
// emptiness rule being the part worth naming, since it is what stops every
// spell in the table matching a typed word that is not there.
func isAbbrev(arg1, arg2 string) bool {
	return arg1 != "" && strings.HasPrefix(arg2, arg1)
}

// SpellNameOrNumber names a spell/skill number via SpellName, or formats it
// as "#N" when the table does not cover it — a placeholder the yaml data
// format's writers (world and player) can round-trip losslessly for a
// number nothing names, while still preferring a real name whenever one
// exists. Shared here rather than duplicated per package, since both need
// exactly the same rule: a wand's charge spell and a player's learned
// skill are both just a spellTable number underneath.
func SpellNameOrNumber(n SpellID) string {
	if name := SpellName(n); name != "!UNUSED!" {
		return name
	}
	return "#" + strconv.Itoa(int(n))
}

// SpellNumberFromNameOrNumber is SpellNameOrNumber's inverse: a name is
// looked up via SpellNumberByName, which matches an exact name outright
// before it ever falls back to a prefix — so a name SpellNameOrNumber
// produced is always matched exactly, never by coincidental abbreviation
// against some other entry sharing a prefix. "#N" parses back to N.
//
// The empty name is refused here and not in SpellNumberByName, and the
// split is the point. This is what the yaml readers call — the player
// file's skills and affects, a wand's charge spell, a damage message's
// subject — where an empty name is a malformed file and should say so.
// SpellNumberByName is what a player's typing reaches, where the C's own
// answer is armor (#365). A format that inherited that would turn a blank
// `spell:` into armor without a word.
func SpellNumberFromNameOrNumber(s string) (SpellID, bool) {
	if strings.TrimSpace(s) == "" {
		return 0, false
	}
	if rest, ok := strings.CutPrefix(s, "#"); ok {
		n, err := strconv.Atoi(rest)
		if err != nil {
			return 0, false
		}
		return SpellID(n), true
	}
	return SpellNumberByName(s)
}

// MinLevelFor is the level a class learns a spell at, or LevelImmortal if it
// never does.
func MinLevelFor(info SpellInfo, class Class) int32 {
	if level, ok := info.MinLevel[class]; ok {
		return level
	}
	return LevelImmortal
}

// ManaCost is what casting costs, porting mag_manacost (spell_parser.c:100).
//
// The C's expression indexes min_level[0] and min_level[1] — magic-user and
// cleric — and takes the lower, regardless of what the caster actually is.
// So a paladin's costs are computed from the mage's and cleric's learning
// levels, which is either a local shortcut or an oversight and is reproduced
// either way.
func ManaCost(info SpellInfo, level int32) int32 {
	base := min(MinLevelFor(info, ClassMagicUser), MinLevelFor(info, ClassCleric))
	return max(info.ManaMax-info.ManaChange*(level-base), info.ManaMin)
}

// Spell numbers, from spells.h:35. Stored in every player record, so these
// are the file format as much as they are constants.
const (
	SpellReservedDbc     SpellID = 0
	SpellArmor           SpellID = 1
	SpellTeleport        SpellID = 2
	SpellBless           SpellID = 3
	SpellBlindness       SpellID = 4
	SpellBurningHands    SpellID = 5
	SpellCallLightning   SpellID = 6
	SpellCharm           SpellID = 7
	SpellChillTouch      SpellID = 8
	SpellClone           SpellID = 9
	SpellColorSpray      SpellID = 10
	SpellControlWeather  SpellID = 11
	SpellCreateFood      SpellID = 12
	SpellCreateWater     SpellID = 13
	SpellCureBlind       SpellID = 14
	SpellCureCritic      SpellID = 15
	SpellCureLight       SpellID = 16
	SpellCurse           SpellID = 17
	SpellDetectAlign     SpellID = 18
	SpellDetectInvis     SpellID = 19
	SpellDetectMagic     SpellID = 20
	SpellDetectPoison    SpellID = 21
	SpellDispelEvil      SpellID = 22
	SpellEarthquake      SpellID = 23
	SpellEnchantWeapon   SpellID = 24
	SpellEnergyDrain     SpellID = 25
	SpellFireball        SpellID = 26
	SpellHarm            SpellID = 27
	SpellHeal            SpellID = 28
	SpellInvisible       SpellID = 29
	SpellLightningBolt   SpellID = 30
	SpellLocateObject    SpellID = 31
	SpellMagicMissile    SpellID = 32
	SpellPoison          SpellID = 33
	SpellProtFromEvil    SpellID = 34
	SpellRemoveCurse     SpellID = 35
	SpellSanctuary       SpellID = 36
	SpellShockingGrasp   SpellID = 37
	SpellSleep           SpellID = 38
	SpellStrength        SpellID = 39
	SpellSummon          SpellID = 40
	SpellVentriloquate   SpellID = 41
	SpellWordOfRecall    SpellID = 42
	SpellRemovePoison    SpellID = 43
	SpellSenseLife       SpellID = 44
	SpellAnimateDead     SpellID = 45
	SpellDispelGood      SpellID = 46
	SpellGroupArmor      SpellID = 47
	SpellGroupHeal       SpellID = 48
	SpellGroupRecall     SpellID = 49
	SpellInfravision     SpellID = 50
	SpellWaterwalk       SpellID = 51
	SpellHolySmite       SpellID = 52
	SpellHolyShield      SpellID = 53
	SpellDispelMagic     SpellID = 54
	SpellOuchie          SpellID = 55
	SpellFullHeal        SpellID = 56
	SpellSilence         SpellID = 57
	SpellImmolate        SpellID = 58
	SpellIdentify        SpellID = 201
	SpellFireBreath      SpellID = 202
	SpellGasBreath       SpellID = 203
	SpellFrostBreath     SpellID = 204
	SpellAcidBreath      SpellID = 205
	SpellLightningBreath SpellID = 206
)

var spellTable = map[SpellID]SpellInfo{
	// The skills. The C declares these with skillo(), which is spello() with
	// every number zero — so a skill has a name and a slot in the same table
	// and nothing else. Their class levels come from init_spell_levels along
	// with the spells'.
	SkillBackstab: {
		Name:        "backstab",
		MinPosition: PosDead,
		MinLevel: map[Class]int32{
			ClassThief: 3,
		},
	},
	SkillBash: {
		Name:        "bash",
		MinPosition: PosDead,
		MinLevel: map[Class]int32{
			ClassPaladin: 12,
			ClassWarrior: 12,
		},
	},
	SkillHide: {
		Name:        "hide",
		MinPosition: PosDead,
		MinLevel: map[Class]int32{
			ClassThief: 5,
		},
	},
	SkillKick: {
		Name:        "kick",
		MinPosition: PosDead,
		MinLevel: map[Class]int32{
			ClassPaladin: 2,
			ClassWarrior: 1,
		},
	},
	SkillPickLock: {
		Name:        "pick lock",
		MinPosition: PosDead,
		MinLevel: map[Class]int32{
			ClassThief: 2,
		},
	},
	SkillRescue: {
		Name:        "rescue",
		MinPosition: PosDead,
		MinLevel: map[Class]int32{
			ClassPaladin: 2,
			ClassWarrior: 3,
		},
	},
	SkillSneak: {
		Name:        "sneak",
		MinPosition: PosDead,
		MinLevel: map[Class]int32{
			ClassThief: 1,
		},
	},
	SkillSteal: {
		Name:        "steal",
		MinPosition: PosDead,
		MinLevel: map[Class]int32{
			ClassThief: 4,
		},
	},
	SkillTrack: {
		Name:        "track",
		MinPosition: PosDead,
		MinLevel: map[Class]int32{
			ClassThief:   6,
			ClassWarrior: 9,
		},
	},

	// The spells.
	SpellAnimateDead: {
		Name:    "animate dead",
		ManaMax: 35, ManaMin: 10, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetObjRoom),
		Violent:     false,
		Routines:    NewSet(MagSummons),
	},
	SpellArmor: {
		Name:    "armor",
		ManaMax: 30, ManaMin: 15, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "You feel less protected.",
		MinLevel: map[Class]int32{
			ClassCleric:    1,
			ClassMagicUser: 4,
			ClassPaladin:   9,
		},
	},
	SpellBless: {
		Name:    "bless",
		ManaMax: 35, ManaMin: 5, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetObjInv),
		Violent:     false,
		Routines:    NewSet(MagAffects, MagAlterObjs),
		WearOff:     "You feel less righteous.",
		MinLevel: map[Class]int32{
			ClassCleric:  5,
			ClassPaladin: 5,
		},
	},
	SpellBlindness: {
		Name:    "blindness",
		ManaMax: 35, ManaMin: 25, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetNotSelf),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "You feel a cloak of blindness dissolve.",
		MinLevel: map[Class]int32{
			ClassCleric:    6,
			ClassMagicUser: 9,
		},
	},
	SpellBurningHands: {
		Name:    "burning hands",
		ManaMax: 30, ManaMin: 10, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
		MinLevel: map[Class]int32{
			ClassMagicUser: 5,
		},
	},
	SpellCallLightning: {
		Name:    "call lightning",
		ManaMax: 40, ManaMin: 25, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
		MinLevel: map[Class]int32{
			ClassCleric: 15,
		},
	},
	SpellCharm: {
		Name:    "charm person",
		ManaMax: 75, ManaMin: 50, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetNotSelf),
		Violent:     true,
		Routines:    NewSet(MagManual),
		WearOff:     "You feel more self-confident.",
		MinLevel: map[Class]int32{
			ClassMagicUser: 16,
		},
	},
	SpellChillTouch: {
		Name:    "chill touch",
		ManaMax: 30, ManaMin: 10, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage, MagAffects),
		WearOff:     "You feel your strength return.",
		MinLevel: map[Class]int32{
			ClassMagicUser: 3,
		},
	},
	SpellClone: {
		Name:    "clone",
		ManaMax: 80, ManaMin: 65, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetSelfOnly),
		Violent:     false,
		Routines:    NewSet(MagSummons),
		MinLevel: map[Class]int32{
			ClassMagicUser: 30,
		},
	},
	SpellColorSpray: {
		Name:    "color spray",
		ManaMax: 30, ManaMin: 15, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
		MinLevel: map[Class]int32{
			ClassMagicUser: 11,
		},
	},
	SpellControlWeather: {
		Name:    "control weather",
		ManaMax: 75, ManaMin: 25, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetIgnore),
		Violent:     false,
		Routines:    NewSet(MagManual),
		MinLevel: map[Class]int32{
			ClassCleric: 17,
		},
	},
	SpellCreateFood: {
		Name:    "create food",
		ManaMax: 30, ManaMin: 5, ManaChange: 4,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetIgnore),
		Violent:     false,
		Routines:    NewSet(MagCreations),
		MinLevel: map[Class]int32{
			ClassCleric:  2,
			ClassPaladin: 15,
		},
	},
	SpellCreateWater: {
		Name:    "create water",
		ManaMax: 30, ManaMin: 5, ManaChange: 4,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetObjInv, TargetObjEquip),
		Violent:     false,
		Routines:    NewSet(MagManual),
		MinLevel: map[Class]int32{
			ClassCleric:  2,
			ClassPaladin: 15,
		},
	},
	SpellCureBlind: {
		Name:    "cure blind",
		ManaMax: 30, ManaMin: 5, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagUnaffects),
		MinLevel: map[Class]int32{
			ClassCleric:  4,
			ClassPaladin: 13,
		},
	},
	SpellCureCritic: {
		Name:    "cure critic",
		ManaMax: 30, ManaMin: 10, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagPoints),
		MinLevel: map[Class]int32{
			ClassCleric: 9,
		},
	},
	SpellCureLight: {
		Name:    "cure light",
		ManaMax: 30, ManaMin: 10, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagPoints),
		MinLevel: map[Class]int32{
			ClassCleric:  1,
			ClassPaladin: 9,
		},
	},
	SpellCurse: {
		Name:    "curse",
		ManaMax: 80, ManaMin: 50, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetObjInv),
		Violent:     true,
		Routines:    NewSet(MagAffects, MagAlterObjs),
		WearOff:     "You feel more optimistic.",
		MinLevel: map[Class]int32{
			ClassMagicUser: 14,
		},
	},
	SpellDetectAlign: {
		Name:    "detect alignment",
		ManaMax: 20, ManaMin: 10, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetSelfOnly),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "You feel less aware.",
		MinLevel: map[Class]int32{
			ClassCleric:  4,
			ClassPaladin: 1,
		},
	},
	SpellDetectInvis: {
		Name:    "detect invisibility",
		ManaMax: 20, ManaMin: 10, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetSelfOnly),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "Your eyes stop tingling.",
		MinLevel: map[Class]int32{
			ClassCleric:    6,
			ClassMagicUser: 2,
		},
	},
	SpellDetectMagic: {
		Name:    "detect magic",
		ManaMax: 20, ManaMin: 10, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetSelfOnly),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "The detect magic wears off.",
		MinLevel: map[Class]int32{
			ClassMagicUser: 2,
		},
	},
	SpellDetectPoison: {
		Name:    "detect poison",
		ManaMax: 15, ManaMin: 5, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetObjInv, TargetObjRoom),
		Violent:     false,
		Routines:    NewSet(MagManual),
		WearOff:     "The detect poison wears off.",
		MinLevel: map[Class]int32{
			ClassCleric:    3,
			ClassMagicUser: 10,
		},
	},
	SpellDispelEvil: {
		Name:    "dispel evil",
		ManaMax: 40, ManaMin: 25, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
		MinLevel: map[Class]int32{
			ClassCleric:  14,
			ClassPaladin: 20,
		},
	},
	SpellDispelGood: {
		Name:    "dispel good",
		ManaMax: 40, ManaMin: 25, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
		MinLevel: map[Class]int32{
			ClassCleric: 14,
		},
	},
	SpellDispelMagic: {
		Name:    "dispel magic",
		ManaMax: 100, ManaMin: 70, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     false,
		Routines:    NewSet(MagManual),
	},
	SpellEarthquake: {
		Name:    "earthquake",
		ManaMax: 40, ManaMin: 25, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetIgnore),
		Violent:     true,
		Routines:    NewSet(MagAreas),
		MinLevel: map[Class]int32{
			ClassCleric: 12,
		},
	},
	SpellEnchantWeapon: {
		Name:    "enchant weapon",
		ManaMax: 150, ManaMin: 100, ManaChange: 10,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetObjInv),
		Violent:     false,
		Routines:    NewSet(MagManual),
		MinLevel: map[Class]int32{
			ClassMagicUser: 26,
		},
	},
	SpellEnergyDrain: {
		Name:    "energy drain",
		ManaMax: 40, ManaMin: 25, ManaChange: 1,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage, MagManual),
		MinLevel: map[Class]int32{
			ClassMagicUser: 13,
		},
	},
	SpellGroupArmor: {
		Name:    "group armor",
		ManaMax: 50, ManaMin: 30, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetIgnore),
		Violent:     false,
		Routines:    NewSet(MagGroups),
		MinLevel: map[Class]int32{
			ClassCleric: 9,
		},
	},
	SpellFireball: {
		Name:    "fireball",
		ManaMax: 40, ManaMin: 30, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
		MinLevel: map[Class]int32{
			ClassMagicUser: 15,
		},
	},
	SpellOuchie: {
		Name:    "ouchie",
		ManaMax: 40, ManaMin: 30, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
	},
	SpellImmolate: {
		Name:    "immolate",
		ManaMax: 40, ManaMin: 30, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
	},
	SpellGroupHeal: {
		Name:    "group heal",
		ManaMax: 80, ManaMin: 60, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetIgnore),
		Violent:     false,
		Routines:    NewSet(MagGroups),
		MinLevel: map[Class]int32{
			ClassCleric: 22,
		},
	},
	SpellHarm: {
		Name:    "harm",
		ManaMax: 75, ManaMin: 45, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
		MinLevel: map[Class]int32{
			ClassCleric: 19,
		},
	},
	SpellHeal: {
		Name:    "heal",
		ManaMax: 60, ManaMin: 40, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagPoints, MagUnaffects),
		MinLevel: map[Class]int32{
			ClassCleric: 16,
		},
	},
	SpellFullHeal: {
		Name:    "full heal",
		ManaMax: 200, ManaMin: 100, ManaChange: 5,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagPoints, MagUnaffects),
	},
	SpellHolyShield: {
		Name:    "holy shield",
		ManaMax: 80, ManaMin: 40, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetSelfOnly),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "You feel your holy protection fade.",
		MinLevel: map[Class]int32{
			ClassPaladin: 5,
		},
	},
	SpellHolySmite: {
		Name:    "holy smite",
		ManaMax: 80, ManaMin: 40, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetSelfOnly),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "You feel less smitey.",
		MinLevel: map[Class]int32{
			ClassPaladin: 26,
		},
	},
	SpellInfravision: {
		Name:    "infravision",
		ManaMax: 25, ManaMin: 10, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetSelfOnly),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "Your night vision seems to fade.",
		MinLevel: map[Class]int32{
			ClassCleric:    7,
			ClassMagicUser: 3,
		},
	},
	SpellInvisible: {
		Name:    "invisibility",
		ManaMax: 35, ManaMin: 25, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetObjInv, TargetObjRoom),
		Violent:     false,
		Routines:    NewSet(MagAffects, MagAlterObjs),
		WearOff:     "You feel yourself exposed.",
		MinLevel: map[Class]int32{
			ClassMagicUser: 4,
		},
	},
	SpellLightningBolt: {
		Name:    "lightning bolt",
		ManaMax: 30, ManaMin: 15, ManaChange: 1,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
		MinLevel: map[Class]int32{
			ClassMagicUser: 9,
		},
	},
	SpellLocateObject: {
		Name:    "locate object",
		ManaMax: 25, ManaMin: 20, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetObjWorld),
		Violent:     false,
		Routines:    NewSet(MagManual),
		MinLevel: map[Class]int32{
			ClassMagicUser: 6,
		},
	},
	SpellMagicMissile: {
		Name:    "magic missile",
		ManaMax: 25, ManaMin: 10, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
		MinLevel: map[Class]int32{
			ClassMagicUser: 1,
		},
	},
	SpellPoison: {
		Name:    "poison",
		ManaMax: 50, ManaMin: 20, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetNotSelf, TargetObjInv),
		Violent:     true,
		Routines:    NewSet(MagAffects, MagAlterObjs),
		WearOff:     "You feel less sick.",
		MinLevel: map[Class]int32{
			ClassCleric:    8,
			ClassMagicUser: 14,
		},
	},
	SpellProtFromEvil: {
		Name:    "protection from evil",
		ManaMax: 40, ManaMin: 10, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetSelfOnly),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "You feel less protected.",
		MinLevel: map[Class]int32{
			ClassCleric: 8,
		},
	},
	SpellRemoveCurse: {
		Name:    "remove curse",
		ManaMax: 45, ManaMin: 25, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetObjInv, TargetObjEquip),
		Violent:     false,
		Routines:    NewSet(MagUnaffects, MagAlterObjs),
		MinLevel: map[Class]int32{
			ClassCleric: 26,
		},
	},
	SpellRemovePoison: {
		Name:    "remove poison",
		ManaMax: 40, ManaMin: 8, ManaChange: 4,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetObjInv, TargetObjRoom),
		Violent:     false,
		Routines:    NewSet(MagUnaffects, MagAlterObjs),
		MinLevel: map[Class]int32{
			ClassCleric:  10,
			ClassPaladin: 13,
		},
	},
	SpellSanctuary: {
		Name:    "sanctuary",
		ManaMax: 110, ManaMin: 85, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "The white aura around your body fades.",
		MinLevel: map[Class]int32{
			ClassCleric:  15,
			ClassPaladin: 22,
		},
	},
	SpellSenseLife: {
		Name:    "sense life",
		ManaMax: 20, ManaMin: 10, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom, TargetSelfOnly),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "You feel less aware of your surroundings.",
		MinLevel: map[Class]int32{
			ClassMagicUser: 17,
		},
	},
	SpellShockingGrasp: {
		Name:    "shocking grasp",
		ManaMax: 30, ManaMin: 15, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom, TargetFightVict),
		Violent:     true,
		Routines:    NewSet(MagDamage),
		MinLevel: map[Class]int32{
			ClassMagicUser: 7,
		},
	},
	SpellSilence: {
		Name:    "silence",
		ManaMax: 100, ManaMin: 70, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "Outside noises return to you once more.",
	},
	SpellSleep: {
		Name:    "sleep",
		ManaMax: 40, ManaMin: 25, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom),
		Violent:     true,
		Routines:    NewSet(MagAffects),
		WearOff:     "You feel less tired.",
		MinLevel: map[Class]int32{
			ClassMagicUser: 8,
		},
	},
	SpellStrength: {
		Name:    "strength",
		ManaMax: 35, ManaMin: 30, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "You feel weaker.",
		MinLevel: map[Class]int32{
			ClassMagicUser: 6,
		},
	},
	SpellSummon: {
		Name:    "summon",
		ManaMax: 75, ManaMin: 50, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharWorld, TargetNotSelf),
		Violent:     false,
		Routines:    NewSet(MagManual),
		MinLevel: map[Class]int32{
			ClassCleric: 10,
		},
	},
	SpellTeleport: {
		Name:    "teleport",
		ManaMax: 75, ManaMin: 50, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagManual),
	},
	SpellWaterwalk: {
		Name:    "waterwalk",
		ManaMax: 40, ManaMin: 20, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagAffects),
		WearOff:     "Your feet seem less buoyant.",
	},
	SpellWordOfRecall: {
		Name:    "word of recall",
		ManaMax: 20, ManaMin: 10, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     NewSet(TargetCharRoom),
		Violent:     false,
		Routines:    NewSet(MagManual),
		MinLevel: map[Class]int32{
			ClassCleric:  12,
			ClassPaladin: 24,
		},
	},
	SpellIdentify: {
		Name:    "identify",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosDead,
		Targets:     NewSet(TargetCharRoom, TargetObjInv, TargetObjRoom),
		Violent:     false,
		Routines:    NewSet(MagManual),
	},
	SpellFireBreath: {
		Name:    "fire breath",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosSitting,
		Targets:     NewSet(TargetIgnore),
		Violent:     true,
		Routines:    RoutineFlags{},
	},
	SpellGasBreath: {
		Name:    "gas breath",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosSitting,
		Targets:     NewSet(TargetIgnore),
		Violent:     true,
		Routines:    RoutineFlags{},
	},
	SpellFrostBreath: {
		Name:    "frost breath",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosSitting,
		Targets:     NewSet(TargetIgnore),
		Violent:     true,
		Routines:    RoutineFlags{},
	},
	SpellAcidBreath: {
		Name:    "acid breath",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosSitting,
		Targets:     NewSet(TargetIgnore),
		Violent:     true,
		Routines:    RoutineFlags{},
	},
	SpellLightningBreath: {
		Name:    "lightning breath",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosSitting,
		Targets:     NewSet(TargetIgnore),
		Violent:     true,
		Routines:    RoutineFlags{},
	},
}

// Practice parameters, from prac_params (class.c:176). Indexed by class:
// what counts as learned, the most and least a session teaches, and whether
// the class calls them spells or skills.
var practiceParams = map[Class]struct {
	Learned int32
	Max     int32
	Min     int32
	Noun    string
}{
	ClassMagicUser: {95, 100, 25, "spell"},
	ClassCleric:    {95, 100, 25, "spell"},
	ClassThief:     {85, 12, 0, "skill"},
	ClassWarrior:   {80, 12, 0, "skill"},
	ClassPaladin:   {90, 100, 25, "spell"},
}

// LearnedLevel is the percentage at which a class stops being able to
// practise something. A thief tops out at 85 and a warrior at 80 — they
// never become as sure of a skill as a mage does of a spell.
func LearnedLevel(class Class) int32 {
	if p, ok := practiceParams[class]; ok {
		return p.Learned
	}
	return practiceParams[ClassWarrior].Learned
}

// PracticeNoun is "spell" or "skill", whichever the class calls them.
func PracticeNoun(class Class) string {
	if p, ok := practiceParams[class]; ok {
		return p.Noun
	}
	return "skill"
}

// PracticeGain is how much one session teaches, porting the expression in
// SPECIAL(guild).
//
// Intelligence drives it through int_app[].learn, bounded by the class's
// minimum and maximum. A thief's maximum of 12 is the reason a thief
// practises so many more times than a mage.
func PracticeGain(rec *PlayerRecord) int32 {
	p, ok := practiceParams[rec.Class]
	if !ok {
		p = practiceParams[ClassWarrior]
	}
	return min(p.Max, max(p.Min, LearnPercent(rec.Abilities.Intelligence)))
}

// HowGood describes a percentage the way the C does, porting how_good
// (spec_procs.c).
func HowGood(percent int32) string {
	switch {
	case percent < 0:
		// The C's " error)" has an unbalanced bracket. Reproduced: a player
		// who ever saw it would remember it.
		return " error)"
	case percent == 0:
		return " (not learned)"
	case percent <= 10:
		return " (awful)"
	case percent <= 20:
		return " (bad)"
	case percent <= 40:
		return " (poor)"
	case percent <= 55:
		return " (average)"
	case percent <= 70:
		return " (fair)"
	case percent <= 80:
		return " (good)"
	case percent <= 85:
		return " (very good)"
	}
	return " (superb)"
}
