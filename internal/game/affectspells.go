// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "github.com/gerrowadat/disgracelands/internal/rng"

// The twenty spells that leave an affect, ported from mag_affects (magic.c).

// NoEffect is the C's NOEFFECT: what a caster is told when a spell lands on
// somebody it cannot help or hurt.
const NoEffect = "Nothing seems to happen.\r\n"

// AffectSpell is what mag_affects decided.
type AffectSpell struct {
	// Affects are the modifiers to apply, at most MaxSpellAffects of them.
	Affects []Affect
	// AccumDuration and AccumModifier say how a repeat casting merges with
	// what is already there.
	AccumDuration bool
	AccumModifier bool

	// ToVictim and ToRoom are what to say, or "".
	ToVictim string
	ToRoom   string

	// Refused is set when the spell simply does not happen. Failure carries
	// its own message, which is not always NOEFFECT — blindness says "You
	// fail." to the caster, and sleep says nothing at all.
	Refused         bool
	RefusalToCaster string

	// SleepsVictim is set by the sleep spell, which changes position as well
	// as applying an affect.
	SleepsVictim bool
}

// AffectsOfSpell computes the affect a spell leaves, porting the switch in
// mag_affects.
//
// caster and victim are both needed: several spells scale on the caster's
// level and several check the victim's state. savedThrow is the result of
// mag_savingthrow, computed by the caller so the roll happens once.
func AffectsOfSpell(spell int32, caster, victim *PlayerRecord, victimIsNPC bool,
	victimFlags Flags, level int32, savedThrow bool, r *rng.Rand,
) AffectSpell {
	var out AffectSpell
	add := func(a Affect) {
		a.Type = spell
		out.Affects = append(out.Affects, a)
	}

	switch spell {
	case SpellChillTouch:
		duration := int32(4)
		if savedThrow {
			duration = 1
		}
		add(Affect{Location: ApplyStr, Modifier: -1, Duration: duration})
		out.AccumDuration = true
		out.ToVictim = "You feel your strength wither!"

	case SpellArmor:
		add(Affect{Location: ApplyAC, Modifier: -20, Duration: 24})
		out.AccumDuration = true
		out.ToVictim = "You feel someone protecting you."

	case SpellBless:
		add(Affect{Location: ApplyHitRoll, Modifier: 2, Duration: 6})
		add(Affect{Location: ApplySaveSpell, Modifier: -1, Duration: 6})
		out.AccumDuration = true
		out.ToVictim = "You feel righteous."

	// A local spell: ten to hit and ten to damage for two ticks.
	case SpellHolySmite:
		add(Affect{Location: ApplyHitRoll, Modifier: 10, Duration: 2})
		add(Affect{Location: ApplyDamRoll, Modifier: 10, Duration: 2})
		out.ToVictim = "You feel the power of righteous smiting come upon you."

	// Also local, and the reason do_cast has a silence check at the top.
	case SpellSilence:
		add(Affect{Bits: AffectSilence, Duration: 2})

	case SpellBlindness:
		if victimFlags.Has(MobNoBlind) || savedThrow {
			return AffectSpell{Refused: true, RefusalToCaster: "You fail.\r\n"}
		}
		add(Affect{Location: ApplyHitRoll, Modifier: -4, Duration: 2, Bits: AffectBlind})
		add(Affect{Location: ApplyAC, Modifier: 40, Duration: 2, Bits: AffectBlind})
		out.ToRoom = "%s seems to be blinded!"
		out.ToVictim = "You have been blinded!"

	case SpellCurse:
		if savedThrow {
			return AffectSpell{Refused: true, RefusalToCaster: NoEffect}
		}
		duration := 1 + caster.Level/2
		add(Affect{Location: ApplyHitRoll, Modifier: -1, Duration: duration, Bits: AffectCurse})
		add(Affect{Location: ApplyDamRoll, Modifier: -1, Duration: duration, Bits: AffectCurse})
		out.AccumDuration, out.AccumModifier = true, true
		out.ToRoom = "%s briefly glows red!"
		out.ToVictim = "You feel very uncomfortable."

	case SpellDetectAlign:
		add(Affect{Bits: AffectDetectAlign, Duration: 12 + level})
		out.AccumDuration = true
		out.ToVictim = "Your eyes tingle."

	case SpellDetectInvis:
		add(Affect{Bits: AffectDetectInvis, Duration: 12 + level})
		out.AccumDuration = true
		out.ToVictim = "Your eyes tingle."

	case SpellDetectMagic:
		add(Affect{Bits: AffectDetectMagic, Duration: 12 + level})
		out.AccumDuration = true
		out.ToVictim = "Your eyes tingle."

	case SpellInfravision:
		add(Affect{Bits: AffectInfravision, Duration: 12 + level})
		out.AccumDuration = true
		out.ToVictim = "Your eyes glow red."
		out.ToRoom = "%s's eyes glow red."

	case SpellInvisible:
		add(Affect{
			Location: ApplyAC, Modifier: -40,
			Duration: 12 + caster.Level/4, Bits: AffectInvisible,
		})
		out.AccumDuration = true
		out.ToVictim = "You vanish."
		out.ToRoom = "%s slowly fades out of existence."

	case SpellPoison:
		if savedThrow {
			return AffectSpell{Refused: true, RefusalToCaster: NoEffect}
		}
		add(Affect{
			Location: ApplyStr, Modifier: -2,
			Duration: caster.Level, Bits: AffectPoison,
		})
		out.ToVictim = "You feel very sick."
		out.ToRoom = "%s gets violently ill!"

	case SpellProtFromEvil:
		add(Affect{Bits: AffectProtectEvil, Duration: 24})
		out.AccumDuration = true
		out.ToVictim = "You feel invulnerable!"

	// Local: what turns an evil mobile away, per mobile_activity.
	case SpellHolyShield:
		add(Affect{Bits: AffectHolyShield, Duration: 4})
		out.AccumDuration = true
		out.ToVictim = "You feel yourself protected by righteousness!"

	case SpellSanctuary:
		add(Affect{Bits: AffectSanctuary, Duration: 4})
		out.AccumDuration = true
		out.ToVictim = "A white aura momentarily surrounds you."
		out.ToRoom = "%s is surrounded by a white aura."

	case SpellSleep:
		// The C refuses outright between two players when player-killing is
		// off, and against a NOSLEEP mobile, and on a successful save —
		// silently in all three cases.
		if victimFlags.Has(MobNoSleep) || savedThrow {
			return AffectSpell{Refused: true}
		}
		add(Affect{Bits: AffectSleep, Duration: 4 + caster.Level/4})
		out.SleepsVictim = true

	case SpellStrength:
		// A character already at 18/00 gains nothing, and the C returns
		// without a message at all.
		if victim != nil && victim.Abilities.StrengthPercentile == 100 {
			return AffectSpell{Refused: true}
		}
		// `1 + (level > 18)` in the C: a boolean promoted to an int, so two
		// points above level 18 and one below.
		modifier := int32(1)
		if level > 18 {
			modifier = 2
		}
		add(Affect{
			Location: ApplyStr, Modifier: modifier,
			Duration: caster.Level/2 + 4,
		})
		out.AccumDuration, out.AccumModifier = true, true
		out.ToVictim = "You feel stronger!"

	case SpellSenseLife:
		add(Affect{Bits: AffectSenseLife, Duration: caster.Level})
		out.AccumDuration = true
		out.ToVictim = "Your feel your awareness improve."

	case SpellWaterwalk:
		add(Affect{Bits: AffectWaterwalk, Duration: 24})
		out.AccumDuration = true
		out.ToVictim = "You feel webbing between your toes."

	default:
		return AffectSpell{Refused: true}
	}

	return out
}

// CanAffect applies the two guards mag_affects makes after building the
// affect, porting the block after the switch.
//
// The first is subtle and worth keeping: a mobile that has an affect *from
// its prototype* cannot be given it by a spell, because otherwise a player
// could sanctuary a sanctuary-carrying mobile and wait for it to wear off,
// stripping it of something its file said it always had.
func CanAffect(spell AffectSpell, target *PlayerRecord, targetIsNPC bool, spellNumber int32) bool {
	if targetIsNPC && !AffectedBySpell(target, spellNumber) {
		for _, a := range spell.Affects {
			if a.Bits != 0 && target.AffectFlags.HasAny(a.Bits) {
				return false
			}
		}
	}

	// Already under it and it does not stack.
	if AffectedBySpell(target, spellNumber) &&
		!spell.AccumDuration && !spell.AccumModifier {
		return false
	}
	return true
}

// ApplyAffectSpell puts the affects on, porting the loop at the end of
// mag_affects. Only entries that actually do something are applied, which is
// how a spell with one affect and a two-element array does not leave an empty
// one behind.
func ApplyAffectSpell(spell AffectSpell, target *PlayerRecord) {
	for _, a := range spell.Affects {
		if a.Bits == 0 && a.Location == ApplyNone {
			continue
		}
		JoinAffect(target, a, spell.AccumDuration, spell.AccumModifier)
	}
}

// Unaffection is what one curing spell takes off, and what it says when it
// works: mag_unaffects' own switch (magic.c:910-929), which maps the *cure*
// to the affliction before touching anything.
//
// That mapping was the whole of #299. The port passed mag_unaffects the
// cure's own spell number, so `cure blind` removed affects of type 14 --
// a number nothing ever applies -- and left the blindness where it was.
// Cure blind, remove poison and remove curse each did nothing at all to a
// character, and each said "Nothing seems to happen."
type Unaffection struct {
	// Affliction is the spell whose affects come off.
	Affliction int32
	// ToVictim is sent to the character being cured, and ToRoom to
	// everybody else in the room with a %s for their name.
	ToVictim, ToRoom string
	// Silent says the C sends no NOEFFECT when there was nothing to
	// remove. It is true only for heal, which is MAG_POINTS |
	// MAG_UNAFFECTS: without it, healing somebody who is not blind would
	// print "Nothing seems to happen." immediately after healing them
	// (`if (spellnum != SPELL_HEAL)`, magic.c:932).
	Silent bool
}

// UnaffectionOf returns what a MAG_UNAFFECTS spell removes, and whether the
// C's switch has a case for it at all.
//
// The false return is not a can't-happen. `full heal` is MAG_POINTS |
// MAG_UNAFFECTS in the archived spell table (spell_parser.c:1007-1008) and
// mag_unaffects has no case for it, so on the real server every full heal
// fell through to the default: a SYSERR in the syslog, and no unaffection
// and no message for anybody in the room. Reproduced here as doing nothing,
// silently -- see docs/weirdnumbers.md.
func UnaffectionOf(spell int32) (Unaffection, bool) {
	switch spell {
	case SpellCureBlind:
		return Unaffection{
			Affliction: SpellBlindness,
			ToVictim:   "Your vision returns!",
			ToRoom:     "There's a momentary gleam in %s's eyes.",
		}, true
	case SpellHeal:
		return Unaffection{
			Affliction: SpellBlindness,
			ToVictim:   "Your vision returns!",
			ToRoom:     "There's a momentary gleam in %s's eyes.",
			Silent:     true,
		}, true
	case SpellRemovePoison:
		return Unaffection{
			Affliction: SpellPoison,
			ToVictim:   "A warm feeling runs through your body!",
			ToRoom:     "%s looks better.",
		}, true
	case SpellRemoveCurse:
		// No room line in the C, which is not an oversight worth fixing:
		// a curse lifting is between the caster and the cursed.
		return Unaffection{
			Affliction: SpellCurse,
			ToVictim:   "You don't feel so unlucky.",
		}, true
	}
	return Unaffection{}, false
}
