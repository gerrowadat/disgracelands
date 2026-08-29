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
	var addMana, addMove int32

	// casterMana is the two spellcasting classes' identical mana roll, capped
	// at ten. The C computes (int)(1.5 * GET_LEVEL(ch)), which truncates
	// towards zero for the positive levels this can be called with, so
	// integer arithmetic gives the same answer.
	level := rec.Level
	casterMana := func() int32 {
		n := randRange(r, level, level*3/2)
		if n > 10 {
			n = 10
		}
		return n
	}

	// **The draws happen in the C's own order: hit points, then mana, then
	// movement** (class.c:1868-1901). Every class does all three, including
	// the two whose mana is thrown away again a few lines further down —
	// `if (GET_LEVEL(ch) > 1)` guards the *addition*, not the roll.
	//
	// This port hoisted the mana roll out of the switch and took it last, on
	// the reasonable-looking grounds that every class computes it
	// identically. It does; the draw order is what differs. Rolling movement
	// before mana hands each of them the other's number, which is how a
	// magic-user got 83 movement where the C gives 84 (#188).
	switch rec.Class {
	case ClassMagicUser:
		addHit += randRange(r, 3, 8)
		addMana = casterMana()
		addMove = randRange(r, 0, 2)
	case ClassCleric:
		addHit += randRange(r, 5, 10)
		addMana = casterMana()
		addMove = randRange(r, 0, 2)
	case ClassThief:
		addHit += randRange(r, 7, 13)
		addMana = casterMana()
		addMove = randRange(r, 1, 3)
	case ClassWarrior:
		addHit += randRange(r, 10, 15)
		addMana = casterMana()
		addMove = randRange(r, 1, 3)
	case ClassPaladin:
		addHit += randRange(r, 10, 14)
		addMana = casterMana()
		addMove = randRange(r, 1, 3)
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
	// max_hit alone (class.c:1809). do_start does not touch max_mana or
	// max_move: init_char set those, and this is the one place where
	// WipeMud-src's do_start differs and resets all three.
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

// Remort makes rec a fresh level-one member of newClass while keeping
// everything they have already been.
//
// **This is a deliberate gameplay change, not a port of anything.** The C's
// do_remort (act.wizard.c:355) sets a bit in the remort vector and stops:
// class, level, hit points, mana and experience are all left exactly as they
// were, so a level 30 warrior remorted to mage was a level 30 warrior who
// could also cast, and an implementor had to follow up with `set <name> class
// mage`, `set <name> level 1` and the rest by hand. Issue #262 is that that
// homework should be the command's job. docs/deviations.md carries the entry.
//
// The mechanic it has to preserve is the one the whole port has carried since
// Phase 3: the IS_<CLASS> macros (utils.h:508) are
//
//	(GET_CLASS(ch) == CLASS_X) || (GET_REMORT_VECTOR(ch) & mask_X)
//
// — your *current* class counts for free, and the vector records the others.
// So changing the class field without setting the outgoing class's bit would
// take away the very abilities remorting exists to keep. Setting that bit
// first is the whole trick, and it is why this cannot be `set class` plus
// `set level`.
//
// What it deliberately does *not* do, each of which do_start does and each of
// which would be wrong here:
//
//   - **No re-roll.** do_start calls roll_real_abils (class.c:1808). A
//     character's abilities are the thing they have had since creation, and
//     silently rolling new ones during what is presented as a reward would be
//     the most destructive possible reading of "set up the new class".
//   - **No skill wipe.** do_start *assigns* StartingSkills; this merges them,
//     and only upwards. Everything already practised survives, which is the
//     point of remorting rather than rerolling.
//   - **Practices are kept.** AdvanceLevel adds one level's worth on top of
//     whatever they had saved. Taking them away would be taking away
//     something earned, and having some to spend is what "ready to get going
//     right away" means.
//   - **Played time, gold, equipment and conditions are untouched.**
//
// Maximum hit points, mana and movement go back to init_char's starting
// figures and then take one level's advance, which is what makes this a
// level-one character rather than a level-one character with a level 30
// body. They are set on the *real* values and then re-totalled, so anything
// worn is re-applied rather than baked into the baseline — the mistake that
// would otherwise make every remort a small permanent stat gain.
func Remort(rec *PlayerRecord, newClass int32, r *rng.Rand) {
	// Keep what they have been. The outgoing class first: that is the bit
	// that stops the class change from taking abilities away.
	//
	// ApplyNewCharacterDefaults already sets a character's own class bit at
	// creation, so for most characters this is belt and braces. It is not
	// redundant: `set <name> class <x>` changes the class field and does not
	// touch the vector, so a character an implementor has moved between
	// classes by hand can reach here with the bit missing — and that is
	// precisely the character for whom losing it would be silent.
	vector := RemortFlagsOf(rec)
	if mask := RemortMask(rec.Class); mask != 0 {
		vector = vector.Set(mask)
	}
	// And the incoming one, which the C also sets. Redundant while they are
	// that class, and not redundant the moment they remort again.
	if mask := RemortMask(newClass); mask != 0 {
		vector = vector.Set(mask)
	}
	SetRemortFlags(rec, vector)

	rec.Class = newClass
	rec.Level = 1
	rec.Points.Exp = 1
	rec.Title = Title(newClass, rec.Level, rec.Sex)

	// A level-one body. On the real values, because RecomputeAffects below
	// rebuilds the totals from these plus equipment and spells; writing the
	// totals directly would make the next recompute lose the difference.
	rec.RealMaxHit = baseMaxHit
	rec.RealMaxMana = baseMaxMana
	rec.RealMaxMove = baseMaxMove
	rec.Points.MaxHit = baseMaxHit
	rec.Points.MaxMana = baseMaxMana
	rec.Points.MaxMove = baseMaxMove

	// The new class's starting skills, merged upwards. A thief who remorts
	// to warrior keeps their sneak; a warrior who remorts to thief gains it.
	for num, pct := range StartingSkills(newClass) {
		if rec.Skills == nil {
			rec.Skills = make(map[int32]int32, len(StartingSkills(newClass)))
		}
		if rec.Skills[num] < pct {
			rec.Skills[num] = pct
		}
	}

	AdvanceLevel(rec, r)

	// AdvanceLevel works on the totals, so fold its gains back into the real
	// figures before re-applying equipment.
	rec.RealMaxHit = rec.Points.MaxHit
	rec.RealMaxMana = rec.Points.MaxMana
	rec.RealMaxMove = rec.Points.MaxMove
	RecomputeAffects(rec)

	rec.Points.Hit = rec.Points.MaxHit
	rec.Points.Mana = rec.Points.MaxMana
	rec.Points.Move = rec.Points.MaxMove
}
