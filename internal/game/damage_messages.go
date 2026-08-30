// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strconv"

// The ordinary weapon-swing message, ported from dam_message and
// skill_message's weapon-type half (fight.c). damage()'s dispatch
// (fight.c:855-871), restricted to a weapon attack (the only kind an
// ordinary combat-round swing ever is):
//
//	if the swing missed (dam == 0) or it just killed the victim:
//	    prefer a registered FightMessages entry for the attack type;
//	    fall back to the compiled severity table if there is none
//	else:
//	    always the compiled severity table
//
// Every other attack (kick, bash, backstab, a spell) goes through
// skill_message alone, with its own registered entries, and is not
// covered here — see internal/server/violence.go's own doc comment.

// TypeHit is TYPE_HIT (spells.h:139): the base every weapon attack type
// (AttackHit..AttackStab) is stored as an offset from. hit() adds it when
// building w_type (fight.c:1035-1042) and skill_message/fight_messages
// look attack types up at that scale — the real data/misc/messages'
// weapon-type records are numbered 300-314, not 0-14 — while dam_message
// immediately subtracts it back off to index dam_weapons[]/
// attack_hit_text[] (`w_type -= TYPE_HIT`, fight.c:668). DamageMessage
// does the same subtraction internally, so callers pass the same
// TypeHit-scaled value to both.
const TypeHit int32 = 300

// attackVerbs are attack_hit_text[] (fight.c:68-85): a weapon's singular
// and plural verb, indexed by AttackHit..AttackStab (i.e. by attackType
// minus TypeHit — see DamageMessage/replaceWeaponVerb).
var attackVerbs = [15]struct{ singular, plural string }{
	{"hit", "hits"},
	{"sting", "stings"},
	{"whip", "whips"},
	{"slash", "slashes"},
	{"bite", "bites"},
	{"bludgeon", "bludgeons"},
	{"crush", "crushes"},
	{"pound", "pounds"},
	{"claw", "claws"},
	{"maul", "mauls"},
	{"thrash", "thrashes"},
	{"pierce", "pierces"},
	{"blast", "blasts"},
	{"punch", "punches"},
	{"stab", "stabs"},
}

// damageTier is one row of dam_weapons[] (fight.c:591-661): the three
// audiences' messages, with #w/#W standing for the weapon's verb.
type damageTier struct{ room, char, victim string }

// damageTiers are dam_weapons[], verbatim.
var damageTiers = [10]damageTier{
	{
		"$n tries to #w $N, but misses.",
		"You try to #w $N, but miss.",
		"$n tries to #w you, but misses.",
	},
	{
		"$n tickles $N as $e #W $M.",
		"You tickle $N as you #w $M.",
		"$n tickles you as $e #W you.",
	},
	{
		"$n barely #W $N.",
		"You barely #w $N.",
		"$n barely #W you.",
	},
	{
		"$n #W $N.",
		"You #w $N.",
		"$n #W you.",
	},
	{
		"$n #W $N hard.",
		"You #w $N hard.",
		"$n #W you hard.",
	},
	{
		"$n #W $N very hard.",
		"You #w $N very hard.",
		"$n #W you very hard.",
	},
	{
		"$n #W $N extremely hard.",
		"You #w $N extremely hard.",
		"$n #W you extremely hard.",
	},
	{
		"$n massacres $N to small fragments with $s #w.",
		"You massacre $N to small fragments with your #w.",
		"$n massacres you to small fragments with $s #w.",
	},
	{
		"$n OBLITERATES $N with $s deadly #w!!",
		"You OBLITERATE $N with your deadly #w!!",
		"$n OBLITERATES you with $s deadly #w!!",
	},
	{
		"$n PULVERISES $N with $s AMAZING #w!!",
		"You PULVERISE $N with your AMAZING #w!!",
		"$n PULVERISES you with $s AMAZING #w!!",
	},
}

// damageTierFor picks the row, porting dam_message's own six-branch ladder
// (fight.c:670-679) verbatim — including that a miss (dam 0) is tier 0,
// the same row the "but misses" text lives in. A hit for real damage never
// reaches tier 0: hit() clamps every landed blow to at least 1
// (internal/game/fight.go's Attack, `max(1, dam)`), so dam == 0 here can
// only mean the swing missed.
func damageTierFor(dam int32) damageTier {
	switch {
	case dam == 0:
		return damageTiers[0]
	case dam <= 2:
		return damageTiers[1]
	case dam <= 4:
		return damageTiers[2]
	case dam <= 6:
		return damageTiers[3]
	case dam <= 10:
		return damageTiers[4]
	case dam <= 14:
		return damageTiers[5]
	case dam <= 19:
		return damageTiers[6]
	case dam <= 23:
		return damageTiers[7]
	case dam <= 50:
		return damageTiers[8]
	default:
		return damageTiers[9]
	}
}

// replaceWeaponVerb substitutes #w/#W for the weapon's singular/plural
// verb, porting replace_string (fight.c:562-587) — a plain text pre-pass
// applied before the result reaches Act, not one of Act's own $-codes.
// Any other character after a literal '#' (there is none in dam_weapons[]
// today) is dropped, matching the C's own default case, which emits
// nothing for it either. attackType is TypeHit-scaled (300-314), matching
// what callers already carry for the fight_messages lookup; the
// `w_type -= TYPE_HIT` conversion (fight.c:668) happens here.
func replaceWeaponVerb(format string, attackType int32) string {
	offset := attackType - TypeHit
	verbs := attackVerbs[0]
	if offset >= 0 && int(offset) < len(attackVerbs) {
		verbs = attackVerbs[offset]
	}

	var out []rune
	runes := []rune(format)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '#' || i+1 >= len(runes) {
			out = append(out, runes[i])
			continue
		}
		switch runes[i+1] {
		case 'w':
			out = append(out, []rune(verbs.singular)...)
			i++
		case 'W':
			out = append(out, []rune(verbs.plural)...)
			i++
		default:
			out = append(out, '#')
		}
	}
	return string(out)
}

// DamageMessage renders the compiled severity-table message for one
// audience — room, attacker or victim — porting dam_message
// (fight.c:591-620) minus the colour codes: nothing in this port emits
// colour yet (docs/deviations.md), so the C's CCYEL/CCRED wrapping is
// omitted, the same simplification every other command already ships
// with. attackType is TypeHit-scaled (300-314) — the same value
// FightMessages.Pick is called with, not the 0-14 AttackHit..AttackStab
// offset on its own.
func DamageMessage(dam, attackType int32, audience Audience) string {
	tier := damageTierFor(dam)
	var format string
	switch audience {
	case AudienceRoom:
		format = tier.room
	case AudienceChar:
		format = tier.char
	case AudienceVictim:
		format = tier.victim
	}
	return replaceWeaponVerb(format, attackType)
}

// Audience is which of dam_message's/a MsgSet's three recipients a message
// is being rendered for.
type Audience int

const (
	AudienceRoom Audience = iota
	AudienceChar
	AudienceVictim
)

// WeaponAttackType is hit()'s own weapon-type resolution (fight.c:1035-
// 1042), for the part that matters to messages: TypeHit plus a wielded
// weapon's own type (its fourth value), or bare hands (TypeHit +
// AttackHit) if nothing is wielded — the TypeHit-scaled value both
// FightMessages.Pick and DamageMessage expect.
//
// The C has a third case this does not: an NPC with `mob_specials.
// attack_type` set uses that instead of bare hands when unarmed. Nothing
// in this tree parses a mobile's attack type from the world format yet
// (confirmed: no AttackType field exists on MobDef, and nothing reads
// one) — an unarmed mobile always resolves to bare hands here, a
// documented simplification (docs/deviations.md) rather than a silent
// gap, until the world format carries that field.
func WeaponAttackType(wielded *Object) int32 {
	if weapon, ok := wielded.WeaponValues(); ok {
		return weapon.Damage.AttackType()
	}
	// AttackHit is zero, so bare hands are TypeHit itself. Written out
	// because the C writes it out, and because "TypeHit + AttackHit" is
	// the shape every other caller of this scale has.
	return TypeHit + AttackHit
}

// AttackTypeName names a misc/messages record's attack type for the
// yaml format, for step 6c's step-and-a-half question: the number
// spans two spaces the C never had to name together, because it never
// wrote either one out symbolically at all. TypeHit and above is a
// weapon type, named by YamlAttackTypeNames() the same way a weapon's
// own fourth value already is (internal/persist/world/yaml/values.go);
// below it is a spell or skill number, named by SpellNameOrNumber — which
// already covers SkillBackstab/SkillBash/SkillKick alongside every real
// spell, since spellTable is one table for both (confirmed: init_spell_
// levels assigns skill levels through the same mechanism spello() uses
// for spells). Either way, a number neither table names round-trips as
// "#N" rather than being lost — SpellNameOrNumber's own convention,
// reused rather than inventing a second one for the other half.
func AttackTypeName(attackType int32) string {
	if attackType >= TypeHit {
		if name, ok := NameByValue(attackType-TypeHit, YamlAttackTypeNames()); ok {
			return name
		}
		return "#" + strconv.Itoa(int(attackType))
	}
	// Below TypeHit the number is a SpellID; at or above it, it is a
	// weapon attack type. AttackType is a union of two domains rather than
	// one enumeration, which is why it stays an int32 and converts here --
	// see docs/design/idiomatic-go.md §4.5.
	return SpellNameOrNumber(SpellID(attackType))
}

// AttackTypeFromName is AttackTypeName's inverse. A weapon-type name is
// tried first — the two spaces cannot collide (spell/skill names are
// never single hit-verbs like "slash", and neither table is consulted for
// entries the other already claimed) — before falling back to
// SpellNumberFromNameOrNumber, which itself already understands "#N".
func AttackTypeFromName(name string) (int32, bool) {
	if offset, ok := ValueByName(name, YamlAttackTypeNames()); ok {
		return TypeHit + offset, true
	}
	spell, ok := SpellNumberFromNameOrNumber(name)
	return spell.Number(), ok
}
