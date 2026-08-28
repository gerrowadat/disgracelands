// Copyright (C) 2026 Dave O'Connor. Part of Disgracelands, a derivative work
// of CircleMUD (Copyright (C) 1993-2001 Jeremy Elson, the Trustees of the
// Johns Hopkins University and the CircleMUD Group), itself based on DikuMUD
// (Copyright (C) 1990, 1991). Use of this file is governed by the CircleMUD
// and DikuMUD licenses; see LICENSE. Non-commercial use only.

package main

// The text half of the corpus: misc/socials, misc/messages, misc/xnames,
// text/help/ and text/'s plain prose.
//
// All five of these are line-oriented ASCII files in the C, and all five
// went through `dlctl import` untranscoded until docs/design/
// data-format.md §11.1 — so every one of them carries CP1252 bytes here.
// That gap "was real but inert against every fixture in this repo",
// because stock CircleMUD's text is pure ASCII and ASCII is valid UTF-8
// unchanged. This is the fixture where it would not have been inert.

// textFiles returns the classic text tree, keyed by path under the lib
// directory root.
func textFiles() map[string]string {
	return map[string]string{
		"misc/socials":  torturedSocials,
		"misc/messages": torturedMessages,
		"misc/xnames":   torturedXnames,

		"text/help/index":       "torture.hlp\n$\n",
		"text/help/torture.hlp": torturedHelp,
		"text/help/screen":      helpScreen,

		"text/greetings":  greetings,
		"text/credits":    credits,
		"text/motd":       motd,
		"text/imotd":      "The immortal message of the day, in CP1252: caf\xe9.\n",
		"text/news":       "No news.\n",
		"text/wizlist":    "Torturer\n",
		"text/immlist":    "Torturer\n",
		"text/policies":   "Be nice.\n",
		"text/handbook":   "Be nicer.\n",
		"text/info":       "Nothing to see here.\n",
		"text/background": "This directory exists to be difficult.\n",
	}
}

// torturedSocials covers every shape act.social.c's parser distinguishes:
// a social with no target messages at all (char_found is '#', so the
// remaining five lines are not in the file), one with all eight,
// min_victim_position at both ends of the enum (0 is POS_DEAD, 8 is
// POS_STANDING), the hide flag set, a command as long as anything in the
// real archive reaches, and CP1252 in the message text.
//
// There are no comment lines in it, and that is a fact about the format
// rather than an oversight: boot_social_messages has no comment syntax at
// all. Its first non-blank line is a header, so a '*' line here would be
// read as a social named '*' with the wrong field count and fail the
// load. misc/messages, three constants further down this file, does have
// comments - which is exactly the kind of asymmetry between two
// neighbouring files that gets assumed away.
const torturedSocials = "" +
	"smile 0 0\n" +
	"You smile.\n" +
	"$n smiles.\n" +
	"You smile at $M.\n" +
	"$n smiles at $N.\n" +
	"$n smiles at you.\n" +
	"They are not here.\n" +
	"You smile at yourself.\n" +
	"$n smiles at $mself.\n" +
	"\n" +
	"ponder 0 0\n" +
	"You ponder.\n" +
	"$n ponders.\n" +
	"#\n" +
	"\n" +
	"shove 1 8\n" +
	"Shove whom?\n" +
	"#\n" +
	"You shove $M.\n" +
	"$n shoves $N.\n" +
	"$n shoves you.\n" +
	"They are not here to be shoved.\n" +
	"You shove yourself. Well done.\n" +
	"$n shoves $mself, which is quite a trick.\n" +
	"\n" +
	"cafe 0 0\n" +
	"You order a caf\xe9 \x97 \xa32.50, and worth it.\n" +
	"$n orders a caf\xe9. It smells of \x93proper\x94 coffee.\n" +
	"You buy $M a caf\xe9; $e says \x93merci\x94 and takes it in $s own hands.\n" +
	"$n buys $N a caf\xe9.\n" +
	"$n buys you a caf\xe9.\n" +
	"Nobody by that name is at the caf\xe9.\n" +
	"You buy yourself a caf\xe9.\n" +
	"$n buys $mself a caf\xe9.\n" +
	"\n" +
	"congratulate 0 0\n" +
	"Congratulate whom?\n" +
	"#\n" +
	"You congratulate $M.\n" +
	"$n congratulates $N.\n" +
	"$n congratulates you.\n" +
	"They are not here.\n" +
	"You congratulate yourself.\n" +
	"$n congratulates $mself.\n" +
	"\n" +
	"$\n"

// torturedMessages is misc/messages: two records, one per numeric space
// the file mixes (a weapon type and a spell), with '#' standing in for a
// message in some slots and CP1252 text in others. All twelve lines must
// be present in every record regardless of how many are '#', which is the
// rule that differs from the socials file above.
const torturedMessages = "" +
	"* misc/messages. Comments are only legal between records.\n" +
	"\n" +
	"* Magic Missile (spells.h SPELL_MAGIC_MISSILE is 5).\n" +
	"M\n" +
	" 5\n" +
	"Your magic missile finishes $N off \x97 messily.\n" +
	"$n's magic missile finishes you off.\n" +
	"$n's magic missile finishes $N off.\n" +
	"Your magic missile misses $N.\n" +
	"$n's magic missile misses you.\n" +
	"$n's magic missile misses $N.\n" +
	"Your magic missile hits $N.\n" +
	"$n's magic missile hits you.\n" +
	"$n's magic missile hits $N.\n" +
	"#\n" +
	"#\n" +
	"#\n" +
	"\n" +
	"* TYPE_HIT (structs.h) is 132: an ordinary weapon swing.\n" +
	"M\n" +
	" 132\n" +
	"You batter $N into a caf\xe9-coloured smear.\n" +
	"$n batters you into a smear.\n" +
	"$n batters $N into a smear.\n" +
	"#\n" +
	"#\n" +
	"#\n" +
	"You hit $N.\n" +
	"$n hits you.\n" +
	"$n hits $N.\n" +
	"Your godly blow annihilates $N.\n" +
	"$n's godly blow annihilates you.\n" +
	"$n's godly blow annihilates $N.\n" +
	"\n" +
	"$\n"

// torturedXnames is misc/xnames: the disallowed-name substrings. The
// hostile cases are a non-ASCII entry (which is what an accented name on
// a real list looks like) and an entry long enough that a fixed-width
// reader would truncate it.
const torturedXnames = "" +
	"torture\n" +
	"caf\xe9\n" +
	"\xfcbermensch\n" +
	"averyverylongdisallowedsubstringindeed\n"

// torturedHelp is text/help/torture.hlp. Two of these entries slug to the
// same file name and one slugs to nothing at all, which are the two cases
// game.HelpSlug's disambiguation and positional fallback exist for and
// which the real 216-entry archive does not contain.
const torturedHelp = "" +
	"TORTURE\n" +
	"\n" +
	"This help database exists to break the slug writer.\n" +
	"\n" +
	"See also: CAFE\n" +
	"#\n" +
	"CAFE CAF\xe9\n" +
	"\n" +
	"An entry whose keywords are not ASCII. \xa9 2001.\n" +
	"#\n" +
	"SLUG COLLISION\n" +
	"\n" +
	"HelpSlug joins an entry's keywords with a space, lowercases them and\n" +
	"collapses every non-alphanumeric run to one dash. So this entry and\n" +
	"the next one both slug to \"slug-collision\", from two different\n" +
	"keyword lines, and the writer has to disambiguate them.\n" +
	"#\n" +
	"SLUG-COLLISION\n" +
	"\n" +
	"The second half of the collision above. One of us gets a numeric\n" +
	"suffix on our file name; which one is decided by file order.\n" +
	"#\n" +
	"! ^\n" +
	"\n" +
	"A keyword line of pure punctuation, which slugs to the empty string.\n" +
	"The writer falls back to a positional name for this one.\n" +
	"#\n" +
	"LONG\n" +
	"\n" +
	"An entry with a body long enough to be worth wrapping, a line that is\n" +
	"deliberately blank in the middle of it:\n" +
	"\n" +
	"and a line that ends in trailing whitespace:   \n" +
	"#\n" +
	"$\n"

const helpScreen = "" +
	"                       The Torture Chamber\n" +
	"\n" +
	"Nothing here is meant to be played. Everything here is meant to break\n" +
	"a converter.\n"

const greetings = "" +
	"                      The Torture Chamber\n" +
	"\n" +
	"           Based on CircleMUD 3.0, created by Jeremy Elson,\n" +
	"                originally DikuMUD by Hans Henrik Staerfeldt,\n" +
	"                Katja Nyboe, Tom Madsen, Michael Seifert\n" +
	"                and Sebastian Hammer.\n" +
	"\n"

const credits = "" +
	"CircleMUD 3.0, created by Jeremy Elson.\n" +
	"Original DikuMUD by Hans Henrik Staerfeldt, Katja Nyboe,\n" +
	"Tom Madsen, Michael Seifert and Sebastian Hammer.\n"

const motd = "" +
	"This is examples/torture. Every file in it is deliberately awkward.\n" +
	"See examples/torture/README.md for which file is awkward how.\n"
