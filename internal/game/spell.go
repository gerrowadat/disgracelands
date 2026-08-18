// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strings"

// The spell table, ported from mag_assign_spells (spell_parser.c) and
// init_spell_levels (class.c).
//
// The C builds this at boot by calling spello() sixty-odd times and then
// spell_level() another seventy-eight, which is a table written as
// executable statements. Here it is a table. spell_test.go re-parses both C
// functions and compares every field of every entry, so the transcription is
// checked rather than trusted.

// Targeting flags, from spells.h:167. Two of them are checks rather than
// targets — TargetSelfOnly and TargetNotSelf qualify one of the others.
const (
	TargetIgnore    Flags = 1 << 0
	TargetCharRoom  Flags = 1 << 1
	TargetCharWorld Flags = 1 << 2
	TargetFightSelf Flags = 1 << 3
	TargetFightVict Flags = 1 << 4
	TargetSelfOnly  Flags = 1 << 5
	TargetNotSelf   Flags = 1 << 6
	TargetObjInv    Flags = 1 << 7
	TargetObjRoom   Flags = 1 << 8
	TargetObjWorld  Flags = 1 << 9
	TargetObjEquip  Flags = 1 << 10
)

// Spell routines, from spells.h:21. A spell may have several — heal both
// restores points and removes blindness — and the C runs each in turn.
const (
	MagDamage    Flags = 1 << 0
	MagAffects   Flags = 1 << 1
	MagUnaffects Flags = 1 << 2
	MagPoints    Flags = 1 << 3
	MagAlterObjs Flags = 1 << 4
	MagGroups    Flags = 1 << 5
	MagMasses    Flags = 1 << 6
	MagAreas     Flags = 1 << 7
	MagSummons   Flags = 1 << 8
	MagCreations Flags = 1 << 9
	// MagManual means the spell has a function of its own rather than being
	// assembled from the routines above.
	MagManual Flags = 1 << 10
)

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
	Targets Flags
	// Violent marks a spell that starts a fight.
	Violent bool
	// Routines is what it actually does.
	Routines Flags
	// WearOff is what the victim is told when an affect expires.
	WearOff string

	// MinLevel is the level each class learns it at. A class absent from the
	// map never learns it: the C fills every slot with LVL_IMMORT first and
	// only lowers the ones init_spell_levels names.
	MinLevel map[int32]int32
}

// Spell returns a spell's row, and whether it exists.
func Spell(number int32) (SpellInfo, bool) {
	info, ok := spellTable[number]
	return info, ok
}

// SpellName returns a spell's name, or "!UNUSED!" as the C does.
func SpellName(number int32) string {
	if info, ok := spellTable[number]; ok {
		return info.Name
	}
	return "!UNUSED!"
}

// SpellNumberByName finds a spell by name, matching a prefix as the C's
// find_skill_num does — so `cast 'magic mis'` works.
func SpellNumberByName(name string) (int32, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return 0, false
	}

	// An exact match wins outright; otherwise the lowest-numbered prefix
	// match, so the answer does not depend on map iteration order.
	best := int32(-1)
	for number, info := range spellTable {
		if info.Name == name {
			return number, true
		}
		if strings.HasPrefix(info.Name, name) && (best < 0 || number < best) {
			best = number
		}
	}
	if best < 0 {
		return 0, false
	}
	return best, true
}

// MinLevelFor is the level a class learns a spell at, or LevelImmortal if it
// never does.
func MinLevelFor(info SpellInfo, class int32) int32 {
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
	SpellReservedDbc     int32 = 0
	SpellArmor           int32 = 1
	SpellTeleport        int32 = 2
	SpellBless           int32 = 3
	SpellBlindness       int32 = 4
	SpellBurningHands    int32 = 5
	SpellCallLightning   int32 = 6
	SpellCharm           int32 = 7
	SpellChillTouch      int32 = 8
	SpellClone           int32 = 9
	SpellColorSpray      int32 = 10
	SpellControlWeather  int32 = 11
	SpellCreateFood      int32 = 12
	SpellCreateWater     int32 = 13
	SpellCureBlind       int32 = 14
	SpellCureCritic      int32 = 15
	SpellCureLight       int32 = 16
	SpellCurse           int32 = 17
	SpellDetectAlign     int32 = 18
	SpellDetectInvis     int32 = 19
	SpellDetectMagic     int32 = 20
	SpellDetectPoison    int32 = 21
	SpellDispelEvil      int32 = 22
	SpellEarthquake      int32 = 23
	SpellEnchantWeapon   int32 = 24
	SpellEnergyDrain     int32 = 25
	SpellFireball        int32 = 26
	SpellHarm            int32 = 27
	SpellHeal            int32 = 28
	SpellInvisible       int32 = 29
	SpellLightningBolt   int32 = 30
	SpellLocateObject    int32 = 31
	SpellMagicMissile    int32 = 32
	SpellPoison          int32 = 33
	SpellProtFromEvil    int32 = 34
	SpellRemoveCurse     int32 = 35
	SpellSanctuary       int32 = 36
	SpellShockingGrasp   int32 = 37
	SpellSleep           int32 = 38
	SpellStrength        int32 = 39
	SpellSummon          int32 = 40
	SpellVentriloquate   int32 = 41
	SpellWordOfRecall    int32 = 42
	SpellRemovePoison    int32 = 43
	SpellSenseLife       int32 = 44
	SpellAnimateDead     int32 = 45
	SpellDispelGood      int32 = 46
	SpellGroupArmor      int32 = 47
	SpellGroupHeal       int32 = 48
	SpellGroupRecall     int32 = 49
	SpellInfravision     int32 = 50
	SpellWaterwalk       int32 = 51
	SpellHolySmite       int32 = 52
	SpellHolyShield      int32 = 53
	SpellDispelMagic     int32 = 54
	SpellOuchie          int32 = 55
	SpellFullHeal        int32 = 56
	SpellSilence         int32 = 57
	SpellImmolate        int32 = 58
	SpellIdentify        int32 = 201
	SpellFireBreath      int32 = 202
	SpellGasBreath       int32 = 203
	SpellFrostBreath     int32 = 204
	SpellAcidBreath      int32 = 205
	SpellLightningBreath int32 = 206
	SpellTypeSpell       int32 = 0
	SpellTypePotion      int32 = 1
	SpellTypeWand        int32 = 2
	SpellTypeStaff       int32 = 3
	SpellTypeScroll      int32 = 4
)

var spellTable = map[int32]SpellInfo{
	// The skills. The C declares these with skillo(), which is spello() with
	// every number zero — so a skill has a name and a slot in the same table
	// and nothing else. Their class levels come from init_spell_levels along
	// with the spells'.
	SkillBackstab: {
		Name:        "backstab",
		MinPosition: PosDead,
		MinLevel: map[int32]int32{
			ClassThief: 3,
		},
	},
	SkillBash: {
		Name:        "bash",
		MinPosition: PosDead,
		MinLevel: map[int32]int32{
			ClassPaladin: 12,
			ClassWarrior: 12,
		},
	},
	SkillHide: {
		Name:        "hide",
		MinPosition: PosDead,
		MinLevel: map[int32]int32{
			ClassThief: 5,
		},
	},
	SkillKick: {
		Name:        "kick",
		MinPosition: PosDead,
		MinLevel: map[int32]int32{
			ClassPaladin: 2,
			ClassWarrior: 1,
		},
	},
	SkillPickLock: {
		Name:        "pick lock",
		MinPosition: PosDead,
		MinLevel: map[int32]int32{
			ClassThief: 2,
		},
	},
	SkillRescue: {
		Name:        "rescue",
		MinPosition: PosDead,
		MinLevel: map[int32]int32{
			ClassPaladin: 2,
			ClassWarrior: 3,
		},
	},
	SkillSneak: {
		Name:        "sneak",
		MinPosition: PosDead,
		MinLevel: map[int32]int32{
			ClassThief: 1,
		},
	},
	SkillSteal: {
		Name:        "steal",
		MinPosition: PosDead,
		MinLevel: map[int32]int32{
			ClassThief: 4,
		},
	},
	SkillTrack: {
		Name:        "track",
		MinPosition: PosDead,
		MinLevel: map[int32]int32{
			ClassThief:   6,
			ClassWarrior: 9,
		},
	},

	// The spells.
	SpellAnimateDead: {
		Name:    "animate dead",
		ManaMax: 35, ManaMin: 10, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     TargetObjRoom,
		Violent:     false,
		Routines:    MagSummons,
	},
	SpellArmor: {
		Name:    "armor",
		ManaMax: 30, ManaMin: 15, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "You feel less protected.",
		MinLevel: map[int32]int32{
			ClassCleric:    1,
			ClassMagicUser: 4,
			ClassPaladin:   9,
		},
	},
	SpellBless: {
		Name:    "bless",
		ManaMax: 35, ManaMin: 5, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetObjInv,
		Violent:     false,
		Routines:    MagAffects | MagAlterObjs,
		WearOff:     "You feel less righteous.",
		MinLevel: map[int32]int32{
			ClassCleric:  5,
			ClassPaladin: 5,
		},
	},
	SpellBlindness: {
		Name:    "blindness",
		ManaMax: 35, ManaMin: 25, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetNotSelf,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "You feel a cloak of blindness dissolve.",
		MinLevel: map[int32]int32{
			ClassCleric:    6,
			ClassMagicUser: 9,
		},
	},
	SpellBurningHands: {
		Name:    "burning hands",
		ManaMax: 30, ManaMin: 10, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
		MinLevel: map[int32]int32{
			ClassMagicUser: 5,
		},
	},
	SpellCallLightning: {
		Name:    "call lightning",
		ManaMax: 40, ManaMin: 25, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
		MinLevel: map[int32]int32{
			ClassCleric: 15,
		},
	},
	SpellCharm: {
		Name:    "charm person",
		ManaMax: 75, ManaMin: 50, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetNotSelf,
		Violent:     true,
		Routines:    MagManual,
		WearOff:     "You feel more self-confident.",
		MinLevel: map[int32]int32{
			ClassMagicUser: 16,
		},
	},
	SpellChillTouch: {
		Name:    "chill touch",
		ManaMax: 30, ManaMin: 10, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage | MagAffects,
		WearOff:     "You feel your strength return.",
		MinLevel: map[int32]int32{
			ClassMagicUser: 3,
		},
	},
	SpellClone: {
		Name:    "clone",
		ManaMax: 80, ManaMin: 65, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     TargetSelfOnly,
		Violent:     false,
		Routines:    MagSummons,
		MinLevel: map[int32]int32{
			ClassMagicUser: 30,
		},
	},
	SpellColorSpray: {
		Name:    "color spray",
		ManaMax: 30, ManaMin: 15, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
		MinLevel: map[int32]int32{
			ClassMagicUser: 11,
		},
	},
	SpellControlWeather: {
		Name:    "control weather",
		ManaMax: 75, ManaMin: 25, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     TargetIgnore,
		Violent:     false,
		Routines:    MagManual,
		MinLevel: map[int32]int32{
			ClassCleric: 17,
		},
	},
	SpellCreateFood: {
		Name:    "create food",
		ManaMax: 30, ManaMin: 5, ManaChange: 4,
		MinPosition: PosStanding,
		Targets:     TargetIgnore,
		Violent:     false,
		Routines:    MagCreations,
		MinLevel: map[int32]int32{
			ClassCleric:  2,
			ClassPaladin: 15,
		},
	},
	SpellCreateWater: {
		Name:    "create water",
		ManaMax: 30, ManaMin: 5, ManaChange: 4,
		MinPosition: PosStanding,
		Targets:     TargetObjInv | TargetObjEquip,
		Violent:     false,
		Routines:    MagManual,
		MinLevel: map[int32]int32{
			ClassCleric:  2,
			ClassPaladin: 15,
		},
	},
	SpellCureBlind: {
		Name:    "cure blind",
		ManaMax: 30, ManaMin: 5, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagUnaffects,
		MinLevel: map[int32]int32{
			ClassCleric:  4,
			ClassPaladin: 13,
		},
	},
	SpellCureCritic: {
		Name:    "cure critic",
		ManaMax: 30, ManaMin: 10, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagPoints,
		MinLevel: map[int32]int32{
			ClassCleric: 9,
		},
	},
	SpellCureLight: {
		Name:    "cure light",
		ManaMax: 30, ManaMin: 10, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagPoints,
		MinLevel: map[int32]int32{
			ClassCleric:  1,
			ClassPaladin: 9,
		},
	},
	SpellCurse: {
		Name:    "curse",
		ManaMax: 80, ManaMin: 50, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetObjInv,
		Violent:     true,
		Routines:    MagAffects | MagAlterObjs,
		WearOff:     "You feel more optimistic.",
		MinLevel: map[int32]int32{
			ClassMagicUser: 14,
		},
	},
	SpellDetectAlign: {
		Name:    "detect alignment",
		ManaMax: 20, ManaMin: 10, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetSelfOnly,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "You feel less aware.",
		MinLevel: map[int32]int32{
			ClassCleric:  4,
			ClassPaladin: 1,
		},
	},
	SpellDetectInvis: {
		Name:    "detect invisibility",
		ManaMax: 20, ManaMin: 10, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetSelfOnly,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "Your eyes stop tingling.",
		MinLevel: map[int32]int32{
			ClassCleric:    6,
			ClassMagicUser: 2,
		},
	},
	SpellDetectMagic: {
		Name:    "detect magic",
		ManaMax: 20, ManaMin: 10, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetSelfOnly,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "The detect magic wears off.",
		MinLevel: map[int32]int32{
			ClassMagicUser: 2,
		},
	},
	SpellDetectPoison: {
		Name:    "detect poison",
		ManaMax: 15, ManaMin: 5, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetObjInv | TargetObjRoom,
		Violent:     false,
		Routines:    MagManual,
		WearOff:     "The detect poison wears off.",
		MinLevel: map[int32]int32{
			ClassCleric:    3,
			ClassMagicUser: 10,
		},
	},
	SpellDispelEvil: {
		Name:    "dispel evil",
		ManaMax: 40, ManaMin: 25, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
		MinLevel: map[int32]int32{
			ClassCleric:  14,
			ClassPaladin: 20,
		},
	},
	SpellDispelGood: {
		Name:    "dispel good",
		ManaMax: 40, ManaMin: 25, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
		MinLevel: map[int32]int32{
			ClassCleric: 14,
		},
	},
	SpellDispelMagic: {
		Name:    "dispel magic",
		ManaMax: 100, ManaMin: 70, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     false,
		Routines:    MagManual,
	},
	SpellEarthquake: {
		Name:    "earthquake",
		ManaMax: 40, ManaMin: 25, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetIgnore,
		Violent:     true,
		Routines:    MagAreas,
		MinLevel: map[int32]int32{
			ClassCleric: 12,
		},
	},
	SpellEnchantWeapon: {
		Name:    "enchant weapon",
		ManaMax: 150, ManaMin: 100, ManaChange: 10,
		MinPosition: PosStanding,
		Targets:     TargetObjInv,
		Violent:     false,
		Routines:    MagManual,
		MinLevel: map[int32]int32{
			ClassMagicUser: 26,
		},
	},
	SpellEnergyDrain: {
		Name:    "energy drain",
		ManaMax: 40, ManaMin: 25, ManaChange: 1,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage | MagManual,
		MinLevel: map[int32]int32{
			ClassMagicUser: 13,
		},
	},
	SpellGroupArmor: {
		Name:    "group armor",
		ManaMax: 50, ManaMin: 30, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     TargetIgnore,
		Violent:     false,
		Routines:    MagGroups,
		MinLevel: map[int32]int32{
			ClassCleric: 9,
		},
	},
	SpellFireball: {
		Name:    "fireball",
		ManaMax: 40, ManaMin: 30, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
		MinLevel: map[int32]int32{
			ClassMagicUser: 15,
		},
	},
	SpellOuchie: {
		Name:    "ouchie",
		ManaMax: 40, ManaMin: 30, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
	},
	SpellImmolate: {
		Name:    "immolate",
		ManaMax: 40, ManaMin: 30, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
	},
	SpellGroupHeal: {
		Name:    "group heal",
		ManaMax: 80, ManaMin: 60, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     TargetIgnore,
		Violent:     false,
		Routines:    MagGroups,
		MinLevel: map[int32]int32{
			ClassCleric: 22,
		},
	},
	SpellHarm: {
		Name:    "harm",
		ManaMax: 75, ManaMin: 45, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
		MinLevel: map[int32]int32{
			ClassCleric: 19,
		},
	},
	SpellHeal: {
		Name:    "heal",
		ManaMax: 60, ManaMin: 40, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagPoints | MagUnaffects,
		MinLevel: map[int32]int32{
			ClassCleric: 16,
		},
	},
	SpellFullHeal: {
		Name:    "full heal",
		ManaMax: 200, ManaMin: 100, ManaChange: 5,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagPoints | MagUnaffects,
	},
	SpellHolyShield: {
		Name:    "holy shield",
		ManaMax: 80, ManaMin: 40, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetSelfOnly,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "You feel your holy protection fade.",
		MinLevel: map[int32]int32{
			ClassPaladin: 5,
		},
	},
	SpellHolySmite: {
		Name:    "holy smite",
		ManaMax: 80, ManaMin: 40, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetSelfOnly,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "You feel less smitey.",
		MinLevel: map[int32]int32{
			ClassPaladin: 26,
		},
	},
	SpellInfravision: {
		Name:    "infravision",
		ManaMax: 25, ManaMin: 10, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetSelfOnly,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "Your night vision seems to fade.",
		MinLevel: map[int32]int32{
			ClassCleric:    7,
			ClassMagicUser: 3,
		},
	},
	SpellInvisible: {
		Name:    "invisibility",
		ManaMax: 35, ManaMin: 25, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetObjInv | TargetObjRoom,
		Violent:     false,
		Routines:    MagAffects | MagAlterObjs,
		WearOff:     "You feel yourself exposed.",
		MinLevel: map[int32]int32{
			ClassMagicUser: 4,
		},
	},
	SpellLightningBolt: {
		Name:    "lightning bolt",
		ManaMax: 30, ManaMin: 15, ManaChange: 1,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
		MinLevel: map[int32]int32{
			ClassMagicUser: 9,
		},
	},
	SpellLocateObject: {
		Name:    "locate object",
		ManaMax: 25, ManaMin: 20, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     TargetObjWorld,
		Violent:     false,
		Routines:    MagManual,
		MinLevel: map[int32]int32{
			ClassMagicUser: 6,
		},
	},
	SpellMagicMissile: {
		Name:    "magic missile",
		ManaMax: 25, ManaMin: 10, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
		MinLevel: map[int32]int32{
			ClassMagicUser: 1,
		},
	},
	SpellPoison: {
		Name:    "poison",
		ManaMax: 50, ManaMin: 20, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetNotSelf | TargetObjInv,
		Violent:     true,
		Routines:    MagAffects | MagAlterObjs,
		WearOff:     "You feel less sick.",
		MinLevel: map[int32]int32{
			ClassCleric:    8,
			ClassMagicUser: 14,
		},
	},
	SpellProtFromEvil: {
		Name:    "protection from evil",
		ManaMax: 40, ManaMin: 10, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetSelfOnly,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "You feel less protected.",
		MinLevel: map[int32]int32{
			ClassCleric: 8,
		},
	},
	SpellRemoveCurse: {
		Name:    "remove curse",
		ManaMax: 45, ManaMin: 25, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetObjInv | TargetObjEquip,
		Violent:     false,
		Routines:    MagUnaffects | MagAlterObjs,
		MinLevel: map[int32]int32{
			ClassCleric: 26,
		},
	},
	SpellRemovePoison: {
		Name:    "remove poison",
		ManaMax: 40, ManaMin: 8, ManaChange: 4,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetObjInv | TargetObjRoom,
		Violent:     false,
		Routines:    MagUnaffects | MagAlterObjs,
		MinLevel: map[int32]int32{
			ClassCleric:  10,
			ClassPaladin: 13,
		},
	},
	SpellSanctuary: {
		Name:    "sanctuary",
		ManaMax: 110, ManaMin: 85, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "The white aura around your body fades.",
		MinLevel: map[int32]int32{
			ClassCleric:  15,
			ClassPaladin: 22,
		},
	},
	SpellSenseLife: {
		Name:    "sense life",
		ManaMax: 20, ManaMin: 10, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom | TargetSelfOnly,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "You feel less aware of your surroundings.",
		MinLevel: map[int32]int32{
			ClassMagicUser: 17,
		},
	},
	SpellShockingGrasp: {
		Name:    "shocking grasp",
		ManaMax: 30, ManaMin: 15, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom | TargetFightVict,
		Violent:     true,
		Routines:    MagDamage,
		MinLevel: map[int32]int32{
			ClassMagicUser: 7,
		},
	},
	SpellSilence: {
		Name:    "silence",
		ManaMax: 100, ManaMin: 70, ManaChange: 3,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "Outside noises return to you once more.",
	},
	SpellSleep: {
		Name:    "sleep",
		ManaMax: 40, ManaMin: 25, ManaChange: 5,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom,
		Violent:     true,
		Routines:    MagAffects,
		WearOff:     "You feel less tired.",
		MinLevel: map[int32]int32{
			ClassMagicUser: 8,
		},
	},
	SpellStrength: {
		Name:    "strength",
		ManaMax: 35, ManaMin: 30, ManaChange: 1,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "You feel weaker.",
		MinLevel: map[int32]int32{
			ClassMagicUser: 6,
		},
	},
	SpellSummon: {
		Name:    "summon",
		ManaMax: 75, ManaMin: 50, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     TargetCharWorld | TargetNotSelf,
		Violent:     false,
		Routines:    MagManual,
		MinLevel: map[int32]int32{
			ClassCleric: 10,
		},
	},
	SpellTeleport: {
		Name:    "teleport",
		ManaMax: 75, ManaMin: 50, ManaChange: 3,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagManual,
	},
	SpellWaterwalk: {
		Name:    "waterwalk",
		ManaMax: 40, ManaMin: 20, ManaChange: 2,
		MinPosition: PosStanding,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagAffects,
		WearOff:     "Your feet seem less buoyant.",
	},
	SpellWordOfRecall: {
		Name:    "word of recall",
		ManaMax: 20, ManaMin: 10, ManaChange: 2,
		MinPosition: PosFighting,
		Targets:     TargetCharRoom,
		Violent:     false,
		Routines:    MagManual,
		MinLevel: map[int32]int32{
			ClassCleric:  12,
			ClassPaladin: 24,
		},
	},
	SpellIdentify: {
		Name:    "identify",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosDead,
		Targets:     TargetCharRoom | TargetObjInv | TargetObjRoom,
		Violent:     false,
		Routines:    MagManual,
	},
	SpellFireBreath: {
		Name:    "fire breath",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosSitting,
		Targets:     TargetIgnore,
		Violent:     true,
		Routines:    0,
	},
	SpellGasBreath: {
		Name:    "gas breath",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosSitting,
		Targets:     TargetIgnore,
		Violent:     true,
		Routines:    0,
	},
	SpellFrostBreath: {
		Name:    "frost breath",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosSitting,
		Targets:     TargetIgnore,
		Violent:     true,
		Routines:    0,
	},
	SpellAcidBreath: {
		Name:    "acid breath",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosSitting,
		Targets:     TargetIgnore,
		Violent:     true,
		Routines:    0,
	},
	SpellLightningBreath: {
		Name:    "lightning breath",
		ManaMax: 0, ManaMin: 0, ManaChange: 0,
		MinPosition: PosSitting,
		Targets:     TargetIgnore,
		Violent:     true,
		Routines:    0,
	},
}

// Practice parameters, from prac_params (class.c:176). Indexed by class:
// what counts as learned, the most and least a session teaches, and whether
// the class calls them spells or skills.
var practiceParams = map[int32]struct {
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
func LearnedLevel(class int32) int32 {
	if p, ok := practiceParams[class]; ok {
		return p.Learned
	}
	return practiceParams[ClassWarrior].Learned
}

// PracticeNoun is "spell" or "skill", whichever the class calls them.
func PracticeNoun(class int32) string {
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
