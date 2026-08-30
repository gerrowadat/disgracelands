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

// ParseCastArgument splits `'spell name' target` the way do_cast's three
// strtok calls do (spell_parser.c:604) — which is **not** "find the first
// quote, then the second".
//
//	s = strtok(argument, "'");   if (!s) "Cast what where?"
//	s = strtok(NULL, "'");       if (!s) "Spell names must be enclosed..."
//	t = strtok(NULL, "\0");
//
// Two properties of strtok decide four answers a quote-finding parser gets
// wrong, and this port got all four wrong until #358: it **skips a run of
// delimiters** rather than one, and **only the quote is a delimiter** — a
// space is not.
//
//	cast ''              -> "must be enclosed". The two quotes are one
//	                        skipped run, so there is no second token.
//	cast '  '            -> the spell name is "  ", which find_skill_num
//	                        tokenises away and answers as if it were empty:
//	                        armor, the first spell in the table (#365).
//	cast '' fido         -> the spell name is " fido". The empty quotes
//	                        vanish and the target becomes the spell.
//	cast 'magic missile  -> works. The second strtok has no delimiter left
//	                        and returns the rest of the line, so the quote
//	                        is only needed at the front.
//
// Checked against reference/tools/castparse.c, which is those three calls
// and nothing else, rather than reasoned about — the reasoning is what went
// wrong before.
//
// The target is trimmed here where the C trims it a few lines later
// (`one_argument(strcpy(arg, t), t); skip_spaces(&t)`), which is the same
// string by the time anything looks at it. The spell name is *not* trimmed,
// because "  " and "" are different arguments to a function that treats
// them identically only by accident.
func ParseCastArgument(arg string) (spell, target string, err string) {
	// The C's `argument` still carries the space any_one_arg stopped on —
	// do_cast does no skip_spaces before its strtok — while this port's
	// Context.Arg is trimmed at the dispatcher (session.split). Put it back,
	// because strtok's *first* token is everything before the first quote
	// and whether that token exists at all is what "Cast what where?" turns
	// on. One space is enough however many were typed: token 1's content is
	// discarded and only its existence is read.
	//
	// The one case that leaves behind: `cast` and `cast   ` are the same
	// empty Arg here, where the C answers "Cast what where?" to the first
	// and "must be enclosed" to the second. Two refusals differing by a
	// sentence, over trailing spaces nobody can see, and closing it would
	// mean carrying an untrimmed argument through every command for this
	// one. Left alone deliberately.
	if arg == "" {
		return "", "", "Cast what where?\r\n"
	}
	line := " " + arg

	// strtok(line, "'"): skip a run of leading delimiters, then take
	// everything up to the next one.
	rest := strings.TrimLeft(line, "'")
	if rest == "" {
		return "", "", "Cast what where?\r\n"
	}
	quote := strings.Index(rest, "'")
	if quote < 0 {
		// The first token ran to the end of the line, so the second call has
		// nothing to return. This is `cast magic missile`, unquoted.
		return "", "", "Spell names must be enclosed in the Holy Magic Symbols: '\r\n"
	}
	rest = rest[quote+1:]

	// strtok(NULL, "'"): the same again, and the *run* is what makes
	// `cast ''` an error rather than an empty spell name.
	rest = strings.TrimLeft(rest, "'")
	if rest == "" {
		return "", "", "Spell names must be enclosed in the Holy Magic Symbols: '\r\n"
	}
	if quote = strings.Index(rest, "'"); quote < 0 {
		// No closing quote: the rest of the line is the spell name, and
		// strtok(NULL, "\0") then has nothing to give as a target.
		return rest, "", ""
	}
	return rest[:quote], strings.TrimSpace(rest[quote+1:]), ""
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
