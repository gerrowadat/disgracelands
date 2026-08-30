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
// PaladinFlag is one of them, and PaladinFlags is a paladin's set.
// Bit indices, not masks: docs/design/idiomatic-go.md §4.1, and §4.1.1
// for the trap. The numbers are the player file's, in the spare long
// SpecFlags lives in.
type PaladinFlag int

// PaladinFlags is a set of PaladinFlag.
type PaladinFlags = Set[PaladinFlag]

const (
	// PaladinFallen: cast out. Never casts again, whatever their alignment
	// does afterwards.
	PaladinFallen PaladinFlag = 0
	// PaladinUnworthy: suspended. Recovered by getting alignment back above
	// 600.
	PaladinUnworthy PaladinFlag = 1
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
func specFlags(v int32) PaladinFlags {
	return SetFromRaw[PaladinFlag](uint64(uint32(v))) //nolint:gosec // a bit pattern, not an arithmetic conversion
}

func specFlagsValue(f PaladinFlags) int32 {
	return int32(uint32(f.Raw())) //nolint:gosec // as above; only the low bits are ever set
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
			rec.SpecFlags = specFlagsValue(flags.With(PaladinFallen).Without(PaladinUnworthy))
			return PaladinVerdict{
				Message: "Alas! Your evil has been your downfall! You may never again " +
					"bear the holy name of Paladin! Begone, sinner!\r\n",
				Broadcast: "A voice whispers in your ear, 'Rejoice! The sinner " +
					rec.Name + " has been cast out!'",
			}
		case rec.Alignment > paladinDamnation && !flags.Has(PaladinUnworthy):
			rec.SpecFlags = specFlagsValue(flags.With(PaladinUnworthy))
			return PaladinVerdict{
				Message: "You are unworthy of using that spell. Repent your sins, " +
					"lest you be judged by the boot of God himself!\r\n",
			}
		}
	}

	if rec.Alignment > paladinRedemption && flags.Has(PaladinUnworthy) {
		rec.SpecFlags = specFlagsValue(flags.Without(PaladinUnworthy))
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

	for class := range classRemortMasks {
		if !rec.RemortVector.Has(class) {
			continue
		}
		if rec.Level >= MinLevelFor(info, class) {
			return true
		}
	}
	return false
}

// ParseCastArgument splits `'spell name' target` as do_cast's three strtok
// calls do (spell_parser.c:603-611).
//
// The quotes are not decoration: the C splits on them, so a spell name must
// be enclosed and everything after the closing quote is the target. The error
// for a missing quote is one of the game's better lines.
//
// **arg is what the interpreter passes, leading space and all**, and that
// is load-bearing rather than incidental. command_interpreter does
// `line = any_one_arg(argument, arg)` (interpreter.c:1019), and any_one_arg
// skips spaces at the *start* and returns a pointer to the character after
// the word it copied — which is the space before the rest. So do_cast is
// handed " 'magic missile' fido"; and because strtok skips leading
// *delimiters*, the first call returns that space (do_cast's own comment
// calls it "blank") and the second returns the spell name.
//
// Read with the wrong idea of the input — no leading space — the same
// function appears to return the spell name from the first call and the
// target from the second, i.e. to cast the target. That prediction is
// obviously false and is still easy to argue away; it was read that way
// twice here. reference/tools/castoracle.c is the C, compiled, and
// castoracle_test.go sweeps it, which is why the version below is right
// and the readings were not.
func ParseCastArgument(arg string) (spell, target string, err string) {
	// An empty argument is do_cast's own first refusal: strtok returns
	// NULL when there is nothing to tokenise at all.
	if arg == "" {
		return "", "", "Cast what where?\r\n"
	}

	// **The space is put back.** The C's `argument` keeps the space after
	// the command word, and strtok's first call is defined in terms of it:
	// with the space, that call returns the space (do_cast calls it
	// "blank") and the second returns the spell name. This port's
	// interpreter trims the argument (session.split), so without this the
	// first call would consume the *spell name* as the blank and the
	// second would return the target — which is the very confusion the
	// header above describes, arrived at from the other direction.
	//
	// Any leading whitespace does equally well, because the first call
	// discards whatever it returns. What matters is only that there is
	// something before the opening quote that is not itself a quote.
	_, rest, _ := strtokQuote(" " + arg)

	// strtok(NULL, "'"): the spell name. NULL when there is no closing
	// quote — or, and this is the case a plain `strings.Index` port gets
	// wrong, when what is between the quotes is nothing at all, because
	// strtok skips the delimiters it starts on. So `cast ''` is refused
	// here with "Spell names must be enclosed", rather than handed on as
	// an empty spell name to be refused a step later with "Cast what?!?".
	name, after, ok := strtokQuote(rest)
	if !ok {
		return "", "", "Spell names must be enclosed in the Holy Magic Symbols: '\r\n"
	}

	// strtok(NULL, "\0"): everything left, delimiters and all.
	//
	// TrimSpace on both halves is this port's own. On the target it is
	// invisible — the C's `t` keeps the leading space any_one_arg left on
	// it, and every consumer trims it anyway. On the *spell name* it is a
	// deliberate deviation with a reachable consequence, and
	// docs/deviations.md has it: `cast '   '` reaches find_skill_num with
	// three spaces, which has no words, so the C's word loop never runs,
	// `ok && !*first2` holds on the first entry of spell_info[], and the C
	// casts whichever spell sits lowest in the table. This trims to "" and
	// SpellNumberByName refuses it.
	return strings.TrimSpace(name), strings.TrimSpace(after), ""
}

// strtokQuote is one `strtok(s, "'")` call: skip the quotes it starts on,
// take everything up to the next one, and report what is left after it.
// ok is false where strtok returns NULL, which is when nothing but quotes
// remains.
//
// It threads the remainder through the return rather than keeping a
// position, because the C's strtok keeps that position in a static and
// reproducing a global here would buy nothing.
func strtokQuote(s string) (token, rest string, ok bool) {
	s = strings.TrimLeft(s, "'")
	if s == "" {
		return "", "", false
	}
	if end := strings.Index(s, "'"); end >= 0 {
		return s[:end], s[end+1:], true
	}
	return s, "", true
}

// TargetQuestion is what to ask when a spell needs a target and none was
// given: "who" for a spell aimed at people, "what" for one aimed at objects.
func TargetQuestion(info SpellInfo) string {
	if info.Targets.HasAny(TargetObjRoom, TargetObjInv, TargetObjWorld, TargetObjEquip) {
		return "Upon what should the spell be cast?\r\n"
	}
	return "Upon who should the spell be cast?\r\n"
}

// SpecFlagsOf and SetSpecFlags read and write the paladin state bits, which
// live in another of the player file's spare longs. `redeem` is the only thing
// outside this file that touches them.
func SpecFlagsOf(rec *PlayerRecord) PaladinFlags {
	if rec == nil {
		return PaladinFlags{}
	}
	return specFlags(rec.SpecFlags)
}

// SetSpecFlags writes them back.
func SetSpecFlags(rec *PlayerRecord, f PaladinFlags) {
	if rec != nil {
		rec.SpecFlags = specFlagsValue(f)
	}
}
