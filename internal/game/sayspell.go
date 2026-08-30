// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package game

import "strings"

// syllable is one row of the C's syls[] (spell_parser.c:61-94): a piece of a
// spell's name, and what somebody who cannot understand it hears instead.
type syllable struct{ org, news string }

// syllables is syls[], in the C's order, and the order is the whole of it.
//
// ScrambleSpellName walks the name and takes the *first* row that matches at
// the current offset, so the multi-letter rows have to come before the
// single-letter ones or "ar" would always be read as "a" then "r". Inserting
// a row in the wrong place does not fail anything; it quietly changes what a
// bystander hears. That is why TestSyllableTableMatchesTheCSource re-parses
// this out of spell_parser.c rather than asserting about it, the same way
// the command table is anchored to interpreter.c's line numbers
// (CLAUDE.md, "Table re-parsing").
var syllables = []syllable{
	{" ", " "},
	{"ar", "abra"},
	{"ate", "i"},
	{"cau", "kada"},
	{"blind", "nose"},
	{"bur", "mosa"},
	{"cu", "judi"},
	{"de", "oculo"},
	{"dis", "mar"},
	{"ect", "kamina"},
	{"en", "uns"},
	{"gro", "cra"},
	{"light", "dies"},
	{"lo", "hi"},
	{"magi", "kari"},
	{"mon", "bar"},
	{"mor", "zak"},
	{"move", "sido"},
	{"ness", "lacri"},
	{"ning", "illa"},
	{"per", "duda"},
	{"ra", "gru"},
	{"re", "candus"},
	{"son", "sabru"},
	{"tect", "infra"},
	{"tri", "cula"},
	{"ven", "nofo"},
	{"word of", "inset"},
	{"a", "i"}, {"b", "v"}, {"c", "q"}, {"d", "m"}, {"e", "o"}, {"f", "y"}, {"g", "t"},
	{"h", "p"}, {"i", "u"}, {"j", "y"}, {"k", "t"}, {"l", "r"}, {"m", "w"}, {"n", "b"},
	{"o", "a"}, {"p", "s"}, {"q", "d"}, {"r", "f"}, {"s", "g"}, {"t", "h"}, {"u", "e"},
	{"v", "z"}, {"w", "x"}, {"x", "n"}, {"y", "l"}, {"z", "k"},
}

// ScrambleSpellName is say_spell's substitution loop (spell_parser.c:127-141):
// the magic words a bystander who does not share the caster's class hears
// instead of the spell's name. "magic missile" comes out as "kariq wuggure".
//
// A character no row matches is *dropped*, not copied: the C logs "No entry
// in syllable table" and does `ofs++` without touching the output buffer. No
// spell name in the table reaches that -- every letter and the space have a
// row -- but a capital or an apostrophe would, and silently.
func ScrambleSpellName(name string) string {
	var b strings.Builder
	for ofs := 0; ofs < len(name); {
		matched := false
		for _, syl := range syllables {
			if strings.HasPrefix(name[ofs:], syl.org) {
				b.WriteString(syl.news)
				ofs += len(syl.org)
				matched = true
				break
			}
		}
		if !matched {
			ofs++
		}
	}
	return b.String()
}

// SaySpellFormat picks which of say_spell's four sentences a bystander gets
// (spell_parser.c:143-152). The %s is the spell's name, real or scrambled.
//
// selfTarget says the caster is their own target, atChar that somebody else
// in the room is, and atObj that an object the caster can see is. The order
// is the C's: a character target wins over an object one, and both win over
// nothing.
func SaySpellFormat(selfTarget, atChar, atObj bool) string {
	switch {
	case selfTarget:
		return "$n closes $s eyes and utters the words, '%s'."
	case atChar:
		return "$n stares at $N and utters the words, '%s'."
	case atObj:
		return "$n stares at $p and utters the words, '%s'."
	}
	return "$n utters the words, '%s'."
}
