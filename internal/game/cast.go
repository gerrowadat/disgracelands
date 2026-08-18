// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strings"

// The rules around casting a spell, ported from do_cast (spell_parser.c).
//
// Everything here is a decision about whether a spell may be cast at all; the
// spell's actual effect is the caller's business. Kept in the game package
// rather than the session so the paladin rules and the remort class test can
// be tested without a socket.

// The paladin's standing, from the PSF_* flags. These are Disgracelands'
// own — a paladin whose alignment drops loses their magic, and if it drops
// far enough they lose it permanently.
const (
	// PaladinFallen: cast out. Never casts again, whatever their alignment
	// does afterwards.
	PaladinFallen Flags = 1 << 0
	// PaladinUnworthy: suspended. Recovered by getting alignment back above
	// 600.
	PaladinUnworthy Flags = 1 << 1
)

// Paladin alignment thresholds, from do_cast.
const (
	// paladinRedemption is the alignment at which an unworthy paladin is
	// forgiven.
	paladinRedemption int32 = 600
	// paladinDamnation is the alignment at which they are cast out for good.
	// It is the same number as IS_EVIL's threshold, which is not a
	// coincidence: a paladin who reads as evil is finished.
	paladinDamnation int32 = -350
)

// specFlags and specFlagsValue read and write the record's SpecFlags as a
// bitfield. The record holds it as an int32 because that is its width in the
// player file; the conversions are bit-pattern reinterpretations, done in one
// named place for the same reason the remort vector's are.
func specFlags(v int32) Flags {
	return Flags(uint32(v)) //nolint:gosec // a bit pattern, not an arithmetic conversion
}

func specFlagsValue(f Flags) int32 {
	return int32(uint32(f)) //nolint:gosec // as above; only the low bits are ever set
}

// PaladinVerdict is what the paladin rules decided.
type PaladinVerdict struct {
	// Allowed is whether the spell may proceed.
	Allowed bool
	// Message is what to tell the paladin, or "".
	Message string
	// Broadcast is what to tell the whole game, or "" — being cast out is
	// public.
	Broadcast string
}

// JudgePaladin applies the fallen and unworthy rules, porting the block at
// the head of do_cast.
//
// The order matters and is the C's. A fallen paladin is refused outright. An
// unworthy one is refused unless their alignment has climbed back above 600 —
// but the C's condition also excludes anyone already below -350, who is about
// to be cast out on the next branch anyway. Then a negative alignment either
// damns them or suspends them, and only a positive one above 600 redeems.
func JudgePaladin(rec *PlayerRecord) PaladinVerdict {
	if rec == nil || rec.Class != ClassPaladin {
		return PaladinVerdict{Allowed: true}
	}

	flags := specFlags(rec.SpecFlags)

	if flags.Has(PaladinFallen) {
		return PaladinVerdict{
			Message: "You exchanged that spell for a sinful existence, scum!\r\n",
		}
	}

	if flags.Has(PaladinUnworthy) &&
		rec.Alignment <= paladinRedemption && rec.Alignment > paladinDamnation {
		return PaladinVerdict{
			Message: "You must repent your sins before you may use this spell again!\r\n",
		}
	}

	if rec.Alignment < 0 {
		switch {
		case rec.Alignment < paladinDamnation && !flags.Has(PaladinFallen):
			rec.SpecFlags = specFlagsValue(flags.Set(PaladinFallen).Clear(PaladinUnworthy))
			return PaladinVerdict{
				Message: "Alas! Your evil has been your downfall! You may never again " +
					"bear the holy name of Paladin! Begone, sinner!\r\n",
				Broadcast: "A voice whispers in your ear, 'Rejoice! The sinner " +
					rec.Name + " has been cast out!'",
			}
		case rec.Alignment > paladinDamnation && !flags.Has(PaladinUnworthy):
			rec.SpecFlags = specFlagsValue(flags.Set(PaladinUnworthy))
			return PaladinVerdict{
				Message: "You are unworthy of using that spell. Repent your sins, " +
					"lest you be judged by the boot of God himself!\r\n",
			}
		}
	}

	if rec.Alignment > paladinRedemption && flags.Has(PaladinUnworthy) {
		rec.SpecFlags = specFlagsValue(flags.Clear(PaladinUnworthy))
		return PaladinVerdict{
			Allowed: true,
			Message: "Welcome back, friend! May your sword and its might bear " +
				"great witness to almighty God!",
		}
	}

	return PaladinVerdict{Allowed: true}
}

// KnowsSpell reports whether a character is high enough level in any of their
// classes to cast a spell.
//
// **This is a local rewrite and the reason remorting is worth doing.** Stock
// CircleMUD tests `GET_LEVEL(ch) < SINFO.min_level[GET_CLASS(ch)]` — the one
// class they are now. This walks the remort vector and accepts the spell if
// *any* class they have ever been knows it at their level, so a mage who
// remorted to cleric keeps their spellbook. The stock line is still in the C,
// commented out directly above the replacement.
func KnowsSpell(rec *PlayerRecord, info SpellInfo) bool {
	if rec == nil {
		return false
	}

	vector := remortFlags(rec.RemortVector)
	for class, mask := range classRemortMasks {
		if !vector.Has(remortFlags(mask)) {
			continue
		}
		if rec.Level >= MinLevelFor(info, class) {
			return true
		}
	}
	return false
}

// ParseCastArgument splits `'spell name' target` as do_cast's strtok calls do.
//
// The quotes are not decoration: the C splits on them, so a spell name must
// be enclosed and everything after the closing quote is the target. The error
// for a missing quote is one of the game's better lines.
func ParseCastArgument(arg string) (spell, target string, err string) {
	if strings.TrimSpace(arg) == "" {
		return "", "", "Cast what where?\r\n"
	}

	// strtok(argument, "'") takes everything before the first quote, which
	// the C then discards; the second call takes the spell name.
	first := strings.Index(arg, "'")
	if first < 0 {
		return "", "", "Spell names must be enclosed in the Holy Magic Symbols: '\r\n"
	}
	rest := arg[first+1:]

	second := strings.Index(rest, "'")
	if second < 0 {
		return "", "", "Spell names must be enclosed in the Holy Magic Symbols: '\r\n"
	}

	return strings.TrimSpace(rest[:second]), strings.TrimSpace(rest[second+1:]), ""
}

// TargetQuestion is what to ask when a spell needs a target and none was
// given: "who" for a spell aimed at people, "what" for one aimed at objects.
func TargetQuestion(info SpellInfo) string {
	if info.Targets.HasAny(TargetObjRoom | TargetObjInv | TargetObjWorld | TargetObjEquip) {
		return "Upon what should the spell be cast?\r\n"
	}
	return "Upon who should the spell be cast?\r\n"
}
