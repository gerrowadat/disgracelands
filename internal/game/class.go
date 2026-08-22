// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import (
	"strings"

	"github.com/gerrowadat/disgracelands/internal/rng"
)

// Classes, matching structs.h:122. The numbers are stored in every player
// record ever written, so they are the format as much as they are an enum.
const (
	ClassUndefined int32 = -1
	ClassMagicUser int32 = 0
	ClassCleric    int32 = 1
	ClassThief     int32 = 2
	ClassWarrior   int32 = 3
	// ClassPaladin is Disgracelands' own fifth class, not stock CircleMUD.
	// See docs/investigations/non-stock-features.md.
	ClassPaladin int32 = 4
)

// ClassNames are the display names from class.c's pc_class_types.
var ClassNames = map[int32]string{
	ClassMagicUser: "Magic User",
	ClassCleric:    "Cleric",
	ClassThief:     "Thief",
	ClassWarrior:   "Warrior",
	ClassPaladin:   "Paladin",
}

// ClassAbbrevs are class.c's class_abbrevs, used by the who-list.
var ClassAbbrevs = map[int32]string{
	ClassMagicUser: "Mu",
	ClassCleric:    "Cl",
	ClassThief:     "Th",
	ClassWarrior:   "Wa",
	ClassPaladin:   "Pa",
}

// ClassShortNames are class.c's `pc_class_snames` (class.c:69), a third table
// of class names alongside `pc_class_types` and `class_abbrevs`.
//
// A local addition, added for `remort`, which is the only thing that reads
// them — both to match what a god types and to print what a character has
// become. Note "mage" rather than "Magic User": these are lower-case and
// short, and `remort bob magic user` does not work.
var ClassShortNames = map[int32]string{
	ClassMagicUser: "mage",
	ClassCleric:    "cleric",
	ClassThief:     "thief",
	ClassWarrior:   "warrior",
	ClassPaladin:   "paladin",
}

// ClassShortNameOrder is the order pc_class_snames is written in, which is
// also the order `remort` lists a character's classes in.
var ClassShortNameOrder = []int32{
	ClassMagicUser, ClassCleric, ClassThief, ClassWarrior, ClassPaladin,
}

// ParseShortClassName returns the class a `remort` argument names, matching
// the C's `strcasecmp` against pc_class_snames — a whole name, not a prefix.
func ParseShortClassName(word string) (int32, bool) {
	for _, class := range ClassShortNameOrder {
		if strings.EqualFold(ClassShortNames[class], word) {
			return class, true
		}
	}
	return 0, false
}

// ClassName returns a class's display name.
func ClassName(c int32) string {
	if n, ok := ClassNames[c]; ok {
		return n
	}
	return "Undefined"
}

// CreationMenu is the class menu shown at character creation, verbatim from
// class.c:93.
//
// Paladin is deliberately absent: it is reached by remorting, not by
// choosing it. See ParseCreationClass for the discrepancy this exposed in
// the C server.
const CreationMenu = "\r\nSelect a class:\r\n" +
	"  [C]leric\r\n" +
	"  [T]hief\r\n" +
	"  [W]arrior\r\n" +
	"  [M]agic-user\r\n"

// ParseCreationClass interprets a class letter typed at character creation.
//
// **This is a deliberate deviation from the C, recorded in
// docs/proposals/go-port-plan.md.** The C's parse_class (class.c:117) accepts
// 'p' and returns CLASS_PALADIN, and character creation calls that same
// function — so typing the unadvertised letter at the creation prompt made a
// Paladin without remorting. The menu never offered it and the class exists
// to reward remorting, so the C's intent and its behaviour disagree.
//
// Creation follows the intent and rejects 'p'. ParseClass below keeps the C's
// behaviour for the places where it is correct: an implementor's `set class`,
// and remorting.
func ParseCreationClass(arg byte) int32 {
	switch lower(arg) {
	case 'm':
		return ClassMagicUser
	case 'c':
		return ClassCleric
	case 'w':
		return ClassWarrior
	case 't':
		return ClassThief
	}
	return ClassUndefined
}

// ParseClass interprets a class letter anywhere it is not character creation:
// `set class`, and remorting. Matches class.c's parse_class exactly,
// including Paladin.
func ParseClass(arg byte) int32 {
	if c := ParseCreationClass(arg); c != ClassUndefined {
		return c
	}
	if lower(arg) == 'p' {
		return ClassPaladin
	}
	return ClassUndefined
}

func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b - 'A' + 'a'
	}
	return b
}

// RollAbilities rolls a character's statistics, porting roll_real_abils
// (class.c:1726).
//
// Six scores are rolled as 4d6-drop-lowest, sorted descending, and then dealt
// out in a per-class order that gives each class its best roll where it needs
// it. The sort in the C is an insertion into a table using a three-way XOR
// swap; the effect is a descending sort and that is what is reproduced here.
//
// Only a warrior with an 18 strength rolls the exceptional-strength
// percentile, which is why that field is meaningless for everyone else.
func RollAbilities(class int32, r *rng.Rand) Abilities {
	roll := func() int32 {
		var dice [4]int32
		lowest := int32(7)
		total := int32(0)
		for i := range dice {
			dice[i] = randN(r, 6) + 1
			total += dice[i]
			if dice[i] < lowest {
				lowest = dice[i]
			}
		}
		return total - lowest
	}

	var table [6]int32
	for i := range table {
		table[i] = roll()
	}
	// Descending, matching what the C's swap loop achieves.
	for i := 0; i < len(table); i++ {
		for j := i + 1; j < len(table); j++ {
			if table[j] > table[i] {
				table[i], table[j] = table[j], table[i]
			}
		}
	}

	var a Abilities
	switch class {
	case ClassMagicUser:
		a.Intelligence, a.Wisdom, a.Dexterity = table[0], table[1], table[2]
		a.Strength, a.Constitution, a.Charisma = table[3], table[4], table[5]
	case ClassCleric:
		a.Wisdom, a.Intelligence, a.Strength = table[0], table[1], table[2]
		a.Dexterity, a.Constitution, a.Charisma = table[3], table[4], table[5]
	case ClassThief:
		a.Dexterity, a.Strength, a.Constitution = table[0], table[1], table[2]
		a.Intelligence, a.Wisdom, a.Charisma = table[3], table[4], table[5]
	case ClassWarrior:
		a.Strength, a.Dexterity, a.Constitution = table[0], table[1], table[2]
		a.Wisdom, a.Intelligence, a.Charisma = table[3], table[4], table[5]
		if a.Strength == 18 {
			a.StrengthPercentile = randN(r, 101)
		}
	case ClassPaladin:
		// Unreachable at creation — Paladin is remort-only — but the ordering
		// is ported because remorting needs it.
		a.Charisma, a.Wisdom, a.Strength = table[0], table[1], table[2]
		a.Constitution, a.Dexterity, a.Intelligence = table[3], table[4], table[5]
	}
	return a
}

// randN returns a random value in [0, n).
//
// The C writes this as number(0, n - 1), and the draw goes through the same
// helper so that a run on the "circle" generator consumes the sequence in
// exactly the order the C server would. See internal/rng.
func randN(r *rng.Rand, n int32) int32 { return r.Number(0, n-1) }

// Sexes, matching structs.h.
const (
	SexNeutral int32 = 0
	SexMale    int32 = 1
	SexFemale  int32 = 2
)

// ParseSex interprets the letter typed at the sex prompt, as
// interpreter.c's nanny does.
func ParseSex(arg byte) int32 {
	switch lower(arg) {
	case 'm':
		return SexMale
	case 'f':
		return SexFemale
	}
	return -1
}

// StartingConditions are the hunger, thirst and drunkenness a new character
// begins with, from do_start (class.c:1802). Order is drunk, full, thirsty.
var StartingConditions = [3]int32{0, 24, 24}

// baseMaxHit is what do_start sets before the first advance_level.
const baseMaxHit int32 = 10

// Skill numbers, from spells.h:103.
//
// These are stored in every player record, so they are the file format and
// not an enum. An earlier version of this file had three of them wrong —
// sneak, steal and track were taken from a comment rather than from
// spells.h, which put them on top of bash, kick and steal respectively. Read
// the header, not the comment.
const (
	SkillBackstab int32 = 131
	SkillBash     int32 = 132
	SkillHide     int32 = 133
	SkillKick     int32 = 134
	SkillPickLock int32 = 135
	// 136 is undefined in the C, and left so here.
	SkillRescue int32 = 137
	SkillSneak  int32 = 138
	SkillSteal  int32 = 139
	SkillTrack  int32 = 140
)

// StartingSkills are the skills a class begins knowing, from do_start.
// Only the thief has any; the numbers are the C's.
func StartingSkills(class int32) map[int32]int32 {
	if class != ClassThief {
		return nil
	}
	// Skill numbers from spells.h: SNEAK, HIDE, STEAL, BACKSTAB, PICK_LOCK,
	// TRACK. Named constants arrive with the skill tables in Phase 4; until
	// then the numbers carry their names in this comment rather than being
	// invented elsewhere.
	return map[int32]int32{
		SkillSneak:    10,
		SkillHide:     5,
		SkillSteal:    15,
		SkillBackstab: 10,
		SkillPickLock: 10,
		SkillTrack:    10,
	}
}

// AdvanceLevel applies one level's worth of gains, porting advance_level
// (class.c:1860).
//
// It is called for the level the character is *currently* at — do_start calls
// it at level one — so the caller raises the level first and then calls this.
//
// The C also snoop-checks and saves the character here. Both are the caller's
// business in this port: saving from inside a game rule would mean the world
// goroutine touching the disk. See internal/server for where it happens.
func AdvanceLevel(rec *PlayerRecord, r *rng.Rand) {
	addHit := HitPointBonus(rec.Abilities.Constitution)
	var addMove int32

	switch rec.Class {
	case ClassMagicUser:
		addHit, addMove = addHit+randRange(r, 3, 8), randRange(r, 0, 2)
	case ClassCleric:
		addHit, addMove = addHit+randRange(r, 5, 10), randRange(r, 0, 2)
	case ClassThief:
		addHit, addMove = addHit+randRange(r, 7, 13), randRange(r, 1, 3)
	case ClassWarrior:
		addHit, addMove = addHit+randRange(r, 10, 15), randRange(r, 1, 3)
	case ClassPaladin:
		addHit, addMove = addHit+randRange(r, 10, 14), randRange(r, 1, 3)
	}

	// Every class rolls mana the same way, capped at ten. The C computes
	// (int)(1.5 * level), which truncates towards zero for the positive
	// levels this can be called with, so integer arithmetic gives the same
	// answer.
	level := rec.Level
	addMana := randRange(r, level, level*3/2)
	if addMana > 10 {
		addMana = 10
	}

	// MAX(1, ...) on both: a low-constitution magic-user can roll a negative
	// hit-point gain and a magic-user or cleric can roll zero movement, and
	// the C refuses to let either of those cost them a level's progress.
	rec.Points.MaxHit += max(1, addHit)
	rec.Points.MaxMove += max(1, addMove)

	// No mana at level one. This is why a new character prompts with 0M and
	// always has: the C guards the mana gain on level > 1 while hit points
	// and movement are gained unconditionally.
	if rec.Level > 1 {
		rec.Points.MaxMana += addMana
	}

	// Practices. The class test is the remort-aware IS_<CLASS> macro, not a
	// plain comparison, so a character who remorted through cleric keeps a
	// cleric's practice rate.
	bonus := Practices(rec.Abilities.Wisdom)
	if IsMagicUser(rec) || IsCleric(rec) {
		rec.SpellsToLearn += max(2, bonus)
	} else {
		rec.SpellsToLearn += min(2, max(1, bonus))
	}

	if rec.Level >= LevelImmortal {
		// Immortals do not eat, drink or get drunk, and they see in the dark.
		rec.Conditions = [3]int32{-1, -1, -1}
		rec.Preferences = rec.Preferences.Set(PrefHolylight)
	}
}

// Start initialises a character on their first entry into the world, porting
// do_start (class.c:1802).
//
// This is not what runs when a character is created — InitChar is. The C
// splits the two and calls them at different moments: init_char at the class
// prompt, do_start only once the player has read the message of the day and
// chosen to enter the game, and only `if (GET_LEVEL(ch) == 0)`
// (interpreter.c:1684). The split matters, because the first character on an
// empty roster is given level 34 by init_char and therefore never runs this
// at all.
//
// The C's trailing bookkeeping — the mudlog line, played time, logon time and
// the siteok-everyone flag — is left to the caller, which is the only place
// that knows the clock and the server's configuration.
func Start(rec *PlayerRecord, r *rng.Rand) {
	rec.Level = 1
	rec.Points.Exp = 1
	rec.Abilities = RollAbilities(rec.Class, r)
	rec.Title = Title(rec.Class, rec.Level, rec.Sex)
	rec.Points.MaxHit = baseMaxHit
	rec.Skills = StartingSkills(rec.Class)

	AdvanceLevel(rec, r)

	rec.Points.Hit = rec.Points.MaxHit
	rec.Points.Mana = rec.Points.MaxMana
	rec.Points.Move = rec.Points.MaxMove

	// Everything set above is what this character has without any spell on
	// them, which is what an affect adds to.
	SnapshotReal(rec)

	// After advance_level, not before: an immortal created at level one would
	// otherwise have the -1s overwritten. The C has the same ordering.
	rec.Conditions = StartingConditions
}

// randRange returns a value in [lo, hi]. It is number() by another name, kept
// because the call sites read better with it.
func randRange(r *rng.Rand, lo, hi int32) int32 { return r.Number(lo, hi) }
