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

// Class is a player class, matching structs.h:122. The numbers are stored
// in every player record ever written, so they are the format as much as
// they are an enum — docs/proposals/idiomatic-go.md §4.2 and §2.1.
//
// Note that the zero value is ClassMagicUser rather than ClassUndefined,
// which is the C's numbering and is unchanged by this type: a zero-valued
// Class has always read as a magic user, and ClassUndefined is -1.
type Class int

// Classes, matching structs.h:122.
const (
	ClassUndefined Class = -1
	ClassMagicUser Class = 0
	ClassCleric    Class = 1
	ClassThief     Class = 2
	ClassWarrior   Class = 3
	// ClassPaladin is Disgracelands' own fifth class, not stock CircleMUD.
	// See docs/investigations/non-stock-features.md.
	ClassPaladin Class = 4
)

// RemortClasses is the set of classes a character has passed through: the
// Disgracelands remort vector, whose bits are `1 << class`.
type RemortClasses = Set[Class]

// Number is the class's stored number: what every player record holds and
// what the value-indexed name tables (pc_class_types, the yaml class names)
// are keyed by.
//
// It exists so the narrowing happens in one place with one reasoning,
// which is the same job Set.Raw does for the flag domains
// (docs/proposals/idiomatic-go.md §4.1). Class is an `int` and the
// formats are 8- and 32-bit, so without it every boundary would carry its
// own G115 suppression; with it there is one, here, and it is trivially
// true — there are five classes.
func (c Class) Number() int32 { return int32(c) } //nolint:gosec // five classes; the format's width, not an arithmetic conversion

// ClassNames are the display names from class.c's pc_class_types.
var ClassNames = map[Class]string{
	ClassMagicUser: "Magic User",
	ClassCleric:    "Cleric",
	ClassThief:     "Thief",
	ClassWarrior:   "Warrior",
	ClassPaladin:   "Paladin",
}

// ClassAbbrevs are class.c's class_abbrevs, used by the who-list.
var ClassAbbrevs = map[Class]string{
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
var ClassShortNames = map[Class]string{
	ClassMagicUser: "mage",
	ClassCleric:    "cleric",
	ClassThief:     "thief",
	ClassWarrior:   "warrior",
	ClassPaladin:   "paladin",
}

// ClassShortNameOrder is the order pc_class_snames is written in, which is
// also the order `remort` lists a character's classes in.
var ClassShortNameOrder = []Class{
	ClassMagicUser, ClassCleric, ClassThief, ClassWarrior, ClassPaladin,
}

// ParseShortClassName returns the class a `remort` argument names, matching
// the C's `strcasecmp` against pc_class_snames — a whole name, not a prefix.
func ParseShortClassName(word string) (Class, bool) {
	for _, class := range ClassShortNameOrder {
		if strings.EqualFold(ClassShortNames[class], word) {
			return class, true
		}
	}
	return 0, false
}

// ClassName returns a class's display name.
func ClassName(c Class) string {
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
// docs/design/go-port-plan.md.** The C's parse_class (class.c:117) accepts
// 'p' and returns CLASS_PALADIN, and character creation calls that same
// function — so typing the unadvertised letter at the creation prompt made a
// Paladin without remorting. The menu never offered it and the class exists
// to reward remorting, so the C's intent and its behaviour disagree.
//
// Creation follows the intent and rejects 'p'. ParseClass below keeps the C's
// behaviour for the places where it is correct: an implementor's `set class`,
// and remorting.
func ParseCreationClass(arg byte) Class {
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
func ParseClass(arg byte) Class {
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
func RollAbilities(class Class, r *rng.Rand) Abilities {
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

// Sex is a character's sex, matching structs.h. Like Class, the numbers are
// in every player record and every mob file, so they are the format as well
// as an enumeration — docs/proposals/idiomatic-go.md §4.2.
type Sex int

// Sexes, matching structs.h.
const (
	SexNeutral Sex = 0
	SexMale    Sex = 1
	SexFemale  Sex = 2
	// SexUndefined is what ParseSex answers for a letter that is neither
	// 'm' nor 'f'. It was a bare -1; naming it is the point of §3.4.
	SexUndefined Sex = -1
)

// Number is the sex's stored number, for the file formats and the
// value-indexed name tables — the same narrowing point Class.Number is, and
// for the same reason.
func (s Sex) Number() int32 { return int32(s) } //nolint:gosec // three sexes; the format's width, not an arithmetic conversion

// ParseSex interprets the letter typed at the sex prompt, as
// interpreter.c's nanny does.
func ParseSex(arg byte) Sex {
	switch lower(arg) {
	case 'm':
		return SexMale
	case 'f':
		return SexFemale
	}
	return SexUndefined
}

// Race is a character's race. Unlike Class and Sex it enumerates nothing
// here: this server has no races, no race name table and no rule that
// consults one, and `internal/game` never reads PlayerRecord.Race at all.
// It is a number the ascii player format reserves a tag for and that this
// server therefore has to preserve across a round-trip, and nothing else.
//
// The type exists for the other half of §3.2's argument. Race sat between
// Class and Level as a bare int32, so `rec.Race = rec.Level` compiled and
// so did the transposition of any pair of them; that is the same hazard
// §3.2 cites Title(class, level, sex int32) for, one struct along. Naming
// the domain costs one declaration and is also the only place a reader
// can be told the field means nothing here, which is the more useful half.
//
// The archived binary format has no race at all -- structs.h:972's
// char_file_u goes name, description, title, sex, chclass, level -- so a
// character saved through it never had one to lose. See
// docs/proposals/idiomatic-go.md §4.2.
type Race int

// Number is the race's stored number, for the player formats. There is no
// name table to key, because there are no names.
func (r Race) Number() int32 { return int32(r) } //nolint:gosec // the format's width, not an arithmetic conversion

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
	SkillBackstab SpellID = 131
	SkillBash     SpellID = 132
	SkillHide     SpellID = 133
	SkillKick     SpellID = 134
	SkillPickLock SpellID = 135
	// 136 is undefined in the C, and left so here.
	SkillRescue SpellID = 137
	SkillSneak  SpellID = 138
	SkillSteal  SpellID = 139
	SkillTrack  SpellID = 140
)

// StartingSkills are the skills a class begins knowing, from do_start.
// Only the thief has any; the numbers are the C's.
func StartingSkills(class Class) map[SpellID]int32 {
	if class != ClassThief {
		return nil
	}
	return map[SpellID]int32{
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
		rec.Preferences = rec.Preferences.With(PrefHolylight)
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
	// **Merged, not assigned.** The C's do_start has no memset: SET_SKILL
	// (utils.h:325) writes one array element, and the six thief lines are
	// the only writes in the function, so everything already practised
	// survives it. At creation the array is calloc'd and the distinction
	// cannot show — but do_start is also what `advance <name> 1` runs to
	// demote somebody (doAdvance), and there it is the difference between
	// losing a level and losing every skill ever practised. Assigning here
	// was silently the second of those.
	for num, pct := range StartingSkills(rec.Class) {
		if rec.Skills == nil {
			rec.Skills = make(map[SpellID]int32, 6)
		}
		if rec.Skills[num] < pct {
			rec.Skills[num] = pct
		}
	}

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
// were. Issue #262 is that the rest should be the command's job, and
// docs/deviations.md carries the entry.
//
// **The steps are not invented.** They are the sequence whoever ran the game
// typed by hand after every remort, recovered from their own notes (#262):
//
//	set player class whatever
//	set player lessons 0
//	advance player 1
//	set player maxmana 100
//	set player maxmove 100
//	(hp's should be okay)
//	set player prime-stat-from-previous-class 18
//
// Each line below is one of those, in that order, and the order matters:
// `advance 1` re-rolls the abilities, so pinning the old class's prime stat
// has to come after it or the roll would overwrite it.
//
// The mechanic all of it has to preserve is the one the port has carried
// since Phase 3: the IS_<CLASS> macros (utils.h:508) are
//
//	(GET_CLASS(ch) == CLASS_X) || (GET_REMORT_VECTOR(ch) & mask_X)
//
// — your *current* class counts for free, and the vector records the others.
// So changing the class field without setting the outgoing class's bit would
// take away the very abilities remorting exists to keep.
func Remort(rec *PlayerRecord, newClass Class, r *rng.Rand) {
	oldClass := rec.Class

	// Keep what they have been. The outgoing class's bit is what stops the
	// class change from taking abilities away.
	//
	// The hand-run procedure did not need this line, because it ran the
	// *old* remort command first and that set the incoming class's bit,
	// while the outgoing one was already set — by ApplyNewCharacterDefaults
	// at creation, or by whichever earlier remort put them in that class.
	// Setting it explicitly is what makes the command self-contained, and it
	// is not merely belt and braces: `set <name> class` moves the class
	// field without touching the vector, so a character an implementor has
	// moved by hand can reach here with the bit missing.
	vector := RemortClassesOf(rec)
	vector = vector.Union(RemortMask(oldClass))
	vector = vector.Union(RemortMask(newClass))
	SetRemortClasses(rec, vector)

	// set player class whatever
	rec.Class = newClass

	// set player lessons 0
	//
	// "lessons" is do_set's other name for practices (act.wizard.c:2397-8,
	// both rows write the same field). Thirty levels of banked practices
	// spent instantly on a new class's spell list is what this is stopping.
	rec.SpellsToLearn = 0

	// advance player 1
	//
	// Demoting through `advance` runs do_start (doAdvance's own branch, and
	// class.c:1802), which is where the level, the experience, the title,
	// the re-rolled abilities, max_hit back to 10, the class's starting
	// skills and one AdvanceLevel all come from. Calling the same function
	// rather than restating its effects is the point: whatever `advance
	// <name> 1` does to a character, remorting does, because that is what
	// was being typed.
	Start(rec, r)

	// set player maxmana 100
	// set player maxmove 100
	//
	// do_start deliberately does not touch either (it sets max_hit alone),
	// so without these two a remorted character keeps a level-30 mana and
	// movement pool on a level-1 body — which is exactly why the notes have
	// them. 100 and 100, as written down, rather than init_char's 100 and
	// 82: the movement figure is a choice somebody made, and the notes are
	// the evidence of which choice.
	//
	// "(hp's should be okay)" — do_start's max_hit and AdvanceLevel between
	// them leave hit points right, so there is no third line here.
	rec.Points.MaxMana = remortMaxMana
	rec.Points.MaxMove = remortMaxMove

	// set player prime-stat-from-previous-class 18
	//
	// The reward, and the least obvious line in the notes. do_start re-rolls
	// for the *new* class, so the new class's prime requisite gets the best
	// of the six rolls and the old one gets whatever is left — and the whole
	// point of the remort vector is that the character goes on being their
	// old class too. Without this they would keep the old class's spells and
	// lose the statistic that made them worth casting. 18 is the maximum a
	// roll can produce.
	//
	// Percentile strength is cleared with it, because `set str 18` clears it
	// (wizset.go, and the C's do_set does the same): 18/00, not 18 plus
	// whatever the new roll happened to leave behind.
	setPrimeAbility(&rec.Abilities, oldClass, maxRolledAbility)

	// Everything above is what this character is without anything worn or
	// cast on them, so it becomes the real baseline; then equipment and
	// affects go back on top. Doing it in that order is what stops a remort
	// from baking a worn item's bonus permanently into the character.
	SnapshotReal(rec)
	RecomputeAffects(rec)

	rec.Points.Hit = rec.Points.MaxHit
	rec.Points.Mana = rec.Points.MaxMana
	rec.Points.Move = rec.Points.MaxMove
}

// remortMaxMana and remortMaxMove are the figures the hand-run remort
// procedure set (#262). The mana one is init_char's own; the movement one is
// not — init_char gives 82 — and the difference is deliberate rather than a
// slip in the notes, so it is named here rather than reusing baseMaxMove.
const (
	remortMaxMana int32 = 100
	remortMaxMove int32 = 100
)

// maxRolledAbility is the highest a rolled statistic can be: 4d6 drop the
// lowest, so 6+6+6.
const maxRolledAbility int32 = 18

// setPrimeAbility sets a class's prime requisite — the statistic
// roll_real_abils (class.c) hands the best of the six rolls to, which is the
// first field in that class's arm of the switch.
//
// Paladin's is charisma, which is worth knowing and easy to get wrong: it is
// the only class whose ordering was ported without ever being reachable at
// creation, because paladin is remort-only.
func setPrimeAbility(a *Abilities, class Class, value int32) {
	switch class {
	case ClassMagicUser:
		a.Intelligence = value
	case ClassCleric:
		a.Wisdom = value
	case ClassThief:
		a.Dexterity = value
	case ClassWarrior:
		a.Strength = value
		// As `set str` does: 18/00, not 18 plus a percentile from the roll
		// that has just been overwritten.
		a.StrengthPercentile = 0
	case ClassPaladin:
		a.Charisma = value
	}
}

// PrimeAbilityName names a class's prime requisite, for a message that has to
// say which statistic it pinned and why. Same table as setPrimeAbility, which
// is the one that does it.
func PrimeAbilityName(class Class) string {
	switch class {
	case ClassMagicUser:
		return "intelligence"
	case ClassCleric:
		return "wisdom"
	case ClassThief:
		return "dexterity"
	case ClassWarrior:
		return "strength"
	case ClassPaladin:
		return "charisma"
	}
	return "nothing"
}
